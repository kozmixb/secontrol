package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

func (a *App) runSSH(ctx context.Context, id int64, command string) (string, error) {
	var host, user, authType string
	var port int
	var encrypted []byte
	err := a.db.QueryRowContext(ctx, "SELECT host,port,username,auth_type,credential FROM agents WHERE id=?", id).Scan(&host, &port, &user, &authType, &encrypted)
	if err != nil {
		return "", err
	}
	secret, err := a.decrypt(encrypted)
	if err != nil {
		return "", errors.New("could not decrypt credential")
	}
	return runSSHCredential(ctx, host, port, user, authType, secret, command)
}

func (a *App) runSSHStream(ctx context.Context, id int64, command string, output func([]byte)) error {
	var host, user, authType string
	var port int
	var encrypted []byte
	if err := a.db.QueryRowContext(ctx, "SELECT host,port,username,auth_type,credential FROM agents WHERE id=?", id).Scan(&host, &port, &user, &authType, &encrypted); err != nil {
		return err
	}
	secret, err := a.decrypt(encrypted)
	if err != nil {
		return errors.New("could not decrypt credential")
	}
	return runSSHCredentialStream(ctx, host, port, user, authType, secret, command, output)
}

type streamOutputWriter struct {
	write func([]byte)
}

func (w streamOutputWriter) Write(data []byte) (int, error) {
	copyOfData := append([]byte(nil), data...)
	w.write(copyOfData)
	return len(data), nil
}

func runSSHCredentialStream(ctx context.Context, host string, port int, user, authType string, secret []byte, command string, output func([]byte)) error {
	method, err := sshAuthMethod(authType, secret)
	if err != nil {
		return err
	}
	config := &ssh.ClientConfig{User: user, Auth: []ssh.AuthMethod{method}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 12 * time.Second}
	client, err := ssh.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)), config)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	writer := streamOutputWriter{write: output}
	session.Stdout, session.Stderr = writer, writer
	if err := session.Start(command); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	select {
	case <-ctx.Done():
		_ = session.Close()
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("remote command failed: %w", err)
		}
		return nil
	}
}

func sshAuthMethod(authType string, secret []byte) (ssh.AuthMethod, error) {
	if authType == "password" {
		return ssh.Password(string(secret)), nil
	}
	signer, err := ssh.ParsePrivateKey(secret)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	return ssh.PublicKeys(signer), nil
}

func runSSHCredential(ctx context.Context, host string, port int, user, authType string, secret []byte, command string) (string, error) {
	method, err := sshAuthMethod(authType, secret)
	if err != nil {
		return "", err
	}
	config := &ssh.ClientConfig{User: user, Auth: []ssh.AuthMethod{method}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 12 * time.Second}
	client, err := ssh.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)), config)
	if err != nil {
		return "", fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	done := make(chan struct {
		out []byte
		err error
	}, 1)
	go func() {
		b, e := session.CombinedOutput(command)
		done <- struct {
			out []byte
			err error
		}{b, e}
	}()
	select {
	case <-ctx.Done():
		_ = session.Close()
		return "", ctx.Err()
	case result := <-done:
		if result.err != nil {
			return string(result.out), fmt.Errorf("remote command failed: %w", result.err)
		}
		return string(result.out), nil
	}
}
