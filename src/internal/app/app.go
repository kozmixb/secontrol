package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kozmixb/secontrol/assets"
	"golang.org/x/crypto/ssh"
	_ "modernc.org/sqlite"
)

type App struct {
	db     *sql.DB
	aead   cipher.AEAD
	client *http.Client
	active sync.Map
}

const dockerEnvironment = `if [ -d "${HOME}/bin" ]; then export PATH="${HOME}/bin:${PATH}"; fi; if [ -z "${DOCKER_HOST:-}" ]; then login_docker_host=$("${SHELL:-/bin/sh}" -lc 'echo -n "${DOCKER_HOST:-}"' 2>/dev/null || true); if [ -n "$login_docker_host" ]; then export DOCKER_HOST="$login_docker_host"; else docker_uid=$(id -u); for docker_socket in "/run/user/${docker_uid}/docker.sock" "${HOME}/.docker/run/docker.sock"; do if [ -S "$docker_socket" ]; then export XDG_RUNTIME_DIR="/run/user/${docker_uid}"; export DOCKER_HOST="unix://${docker_socket}"; break; fi; done; fi; fi; `

func New(dbPath, dataDir string) (*App, error) {
	key, err := loadKey(dataDir)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	a := &App{db: db, aead: aead, client: &http.Client{Timeout: 20 * time.Second}}
	if err := a.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return a, nil
}

func (a *App) Close() error { return a.db.Close() }

func (a *App) migrate() error {
	_, err := a.db.Exec(`
CREATE TABLE IF NOT EXISTS agents (
 id INTEGER PRIMARY KEY, name TEXT NOT NULL, host TEXT NOT NULL, port INTEGER NOT NULL DEFAULT 22,
 username TEXT NOT NULL, auth_type TEXT NOT NULL, credential BLOB NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending', last_error TEXT NOT NULL DEFAULT '', last_seen DATETIME,
 os TEXT NOT NULL DEFAULT '', kernel TEXT NOT NULL DEFAULT '', uptime_seconds INTEGER NOT NULL DEFAULT 0,
 load1 REAL NOT NULL DEFAULT 0, memory_total INTEGER NOT NULL DEFAULT 0, memory_used INTEGER NOT NULL DEFAULT 0,
 disk_total INTEGER NOT NULL DEFAULT 0, disk_used INTEGER NOT NULL DEFAULT 0, cpu_count INTEGER NOT NULL DEFAULT 0,
 is_vm INTEGER NOT NULL DEFAULT 0, virtualization TEXT NOT NULL DEFAULT '', access_level TEXT NOT NULL DEFAULT 'regular',
 created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS ssh_keys (
 id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, private_key BLOB NOT NULL, public_key TEXT NOT NULL DEFAULT '',
 created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS containers (
 agent_id INTEGER NOT NULL, id TEXT NOT NULL, name TEXT NOT NULL, image TEXT NOT NULL, status TEXT NOT NULL,
 state TEXT NOT NULL, ports TEXT NOT NULL, created TEXT NOT NULL, uptime TEXT NOT NULL DEFAULT '', updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 image_version TEXT NOT NULL DEFAULT '', update_available INTEGER, registry_digest TEXT NOT NULL DEFAULT '', image_checked_at DATETIME,
 compose_project TEXT NOT NULL DEFAULT '', compose_service TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(agent_id,id), FOREIGN KEY(agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS metric_samples (
 id INTEGER PRIMARY KEY, agent_id INTEGER NOT NULL, recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 load1 REAL NOT NULL, memory_used INTEGER NOT NULL, disk_used INTEGER NOT NULL,
 FOREIGN KEY(agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS network_interfaces (
 agent_id INTEGER NOT NULL, name TEXT NOT NULL, state TEXT NOT NULL DEFAULT '', mac TEXT NOT NULL DEFAULT '', addresses TEXT NOT NULL DEFAULT '[]',
 PRIMARY KEY(agent_id,name), FOREIGN KEY(agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS system_services (
 agent_id INTEGER NOT NULL, name TEXT NOT NULL, load_state TEXT NOT NULL DEFAULT '', active_state TEXT NOT NULL DEFAULT '', sub_state TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(agent_id,name), FOREIGN KEY(agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS storage_volumes (
 agent_id INTEGER NOT NULL, filesystem TEXT NOT NULL, fs_type TEXT NOT NULL DEFAULT '', mount_point TEXT NOT NULL,
 total_bytes INTEGER NOT NULL DEFAULT 0, used_bytes INTEGER NOT NULL DEFAULT 0, available_bytes INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(agent_id,filesystem,mount_point), FOREIGN KEY(agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_metrics_agent_time ON metric_samples(agent_id, recorded_at);
`)
	if err != nil {
		return err
	}
	// Upgrade databases created before public keys were stored.
	_, _ = a.db.Exec("ALTER TABLE ssh_keys ADD COLUMN public_key TEXT NOT NULL DEFAULT ''")
	_, _ = a.db.Exec("ALTER TABLE agents ADD COLUMN ssh_key_id INTEGER REFERENCES ssh_keys(id)")
	_, _ = a.db.Exec("ALTER TABLE agents ADD COLUMN is_vm INTEGER NOT NULL DEFAULT 0")
	_, _ = a.db.Exec("ALTER TABLE agents ADD COLUMN virtualization TEXT NOT NULL DEFAULT ''")
	_, _ = a.db.Exec("ALTER TABLE agents ADD COLUMN access_level TEXT NOT NULL DEFAULT 'regular'")
	_, _ = a.db.Exec("ALTER TABLE containers ADD COLUMN uptime TEXT NOT NULL DEFAULT ''")
	_, _ = a.db.Exec("ALTER TABLE containers ADD COLUMN image_version TEXT NOT NULL DEFAULT ''")
	_, _ = a.db.Exec("ALTER TABLE containers ADD COLUMN update_available INTEGER")
	_, _ = a.db.Exec("ALTER TABLE containers ADD COLUMN registry_digest TEXT NOT NULL DEFAULT ''")
	_, _ = a.db.Exec("ALTER TABLE containers ADD COLUMN image_checked_at DATETIME")
	_, _ = a.db.Exec("ALTER TABLE containers ADD COLUMN compose_project TEXT NOT NULL DEFAULT ''")
	_, _ = a.db.Exec("ALTER TABLE containers ADD COLUMN compose_service TEXT NOT NULL DEFAULT ''")
	return nil
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/overview", a.overview)
	mux.HandleFunc("GET /api/containers", a.listFleetContainers)
	mux.HandleFunc("GET /api/storage", a.listFleetStorage)
	mux.HandleFunc("GET /api/agents", a.listAgents)
	mux.HandleFunc("POST /api/agents", a.createAgent)
	mux.HandleFunc("POST /api/agents/test", a.testAgentConnection)
	mux.HandleFunc("GET /api/ssh-keys", a.listSSHKeys)
	mux.HandleFunc("POST /api/ssh-keys", a.createSSHKey)
	mux.HandleFunc("POST /api/ssh-keys/generate", a.generateSSHKey)
	mux.HandleFunc("DELETE /api/ssh-keys/{id}", a.deleteSSHKey)
	mux.HandleFunc("GET /api/agents/{id}", a.getAgent)
	mux.HandleFunc("DELETE /api/agents/{id}", a.deleteAgent)
	mux.HandleFunc("POST /api/agents/{id}/refresh", a.refreshAgent)
	mux.HandleFunc("GET /api/agents/{id}/containers", a.listContainers)
	mux.HandleFunc("GET /api/agents/{id}/system", a.machineSystem)
	mux.HandleFunc("GET /api/agents/{id}/containers/{container}", a.inspectContainer)
	mux.HandleFunc("POST /api/agents/{id}/containers/{container}/actions/{action}", a.containerAction)
	mux.HandleFunc("GET /api/agents/{id}/metrics", a.metrics)
	mux.HandleFunc("GET /api/agents/{id}/logs", a.logs)
	mux.Handle("/", http.FileServer(http.FS(assets.Files)))
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func (a *App) Poll(ctx context.Context, interval time.Duration) {
	a.refreshAll(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.refreshAll(ctx)
		}
	}
}

func (a *App) refreshAll(ctx context.Context) {
	rows, err := a.db.QueryContext(ctx, "SELECT id FROM agents")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			go a.collect(context.Background(), id)
		}
	}
}

func (a *App) createAgent(w http.ResponseWriter, r *http.Request) {
	var in agentInput
	if err := decodeJSON(r, &in); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	in.Name, in.Host, in.Username = strings.TrimSpace(in.Name), strings.TrimSpace(in.Host), strings.TrimSpace(in.Username)
	if in.Port == 0 {
		in.Port = 22
	}
	if in.Port < 1 || in.Port > 65535 {
		errorJSON(w, 400, "port must be between 1 and 65535")
		return
	}
	if in.Name == "" || in.Host == "" || in.Username == "" || (in.AuthType != "password" && in.AuthType != "key") {
		errorJSON(w, 400, "name, host, username and a valid authentication type are required")
		return
	}
	secret := in.Password
	if in.AuthType == "key" {
		secret = in.PrivateKey
		if in.KeyID > 0 {
			var stored []byte
			if err := a.db.QueryRowContext(r.Context(), "SELECT private_key FROM ssh_keys WHERE id=?", in.KeyID).Scan(&stored); errors.Is(err, sql.ErrNoRows) {
				errorJSON(w, 400, "selected SSH key was not found")
				return
			} else if err != nil {
				errorJSON(w, 500, err.Error())
				return
			}
			plain, err := a.decrypt(stored)
			if err != nil {
				errorJSON(w, 500, "could not decrypt selected SSH key")
				return
			}
			secret = string(plain)
		}
	}
	if secret == "" {
		errorJSON(w, 400, "a password or private key is required")
		return
	}
	encrypted, err := a.encrypt([]byte(secret))
	if err != nil {
		errorJSON(w, 500, "could not protect credential")
		return
	}
	var keyID any
	if in.AuthType == "key" && in.KeyID > 0 {
		keyID = in.KeyID
	}
	result, err := a.db.ExecContext(r.Context(), `INSERT INTO agents(name,host,port,username,auth_type,credential,ssh_key_id) VALUES(?,?,?,?,?,?,?)`,
		in.Name, in.Host, in.Port, in.Username, in.AuthType, encrypted, keyID)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	id, _ := result.LastInsertId()
	go a.collect(context.Background(), id)
	writeJSON(w, 201, map[string]int64{"id": id})
}

func (a *App) testAgentConnection(w http.ResponseWriter, r *http.Request) {
	var in agentInput
	if err := decodeJSON(r, &in); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	in.Host, in.Username = strings.TrimSpace(in.Host), strings.TrimSpace(in.Username)
	if in.Port == 0 {
		in.Port = 22
	}
	if in.Host == "" || in.Username == "" || in.Port < 1 || in.Port > 65535 {
		errorJSON(w, 400, "host, username and a valid SSH port are required")
		return
	}
	secret := in.Password
	if in.AuthType == "key" {
		if in.KeyID < 1 {
			errorJSON(w, 400, "select a saved SSH key")
			return
		}
		var encrypted []byte
		if err := a.db.QueryRowContext(r.Context(), "SELECT private_key FROM ssh_keys WHERE id=?", in.KeyID).Scan(&encrypted); errors.Is(err, sql.ErrNoRows) {
			errorJSON(w, 400, "selected SSH key was not found")
			return
		} else if err != nil {
			errorJSON(w, 500, err.Error())
			return
		}
		plain, err := a.decrypt(encrypted)
		if err != nil {
			errorJSON(w, 500, "could not decrypt selected SSH key")
			return
		}
		secret = string(plain)
	} else if in.AuthType != "password" {
		errorJSON(w, 400, "select a valid authentication type")
		return
	}
	if secret == "" {
		errorJSON(w, 400, "a password or saved SSH key is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	output, err := runSSHCredential(ctx, in.Host, in.Port, in.Username, in.AuthType, []byte(secret), "hostname; uname -srm")
	if err != nil {
		errorJSON(w, 502, err.Error())
		return
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	result := map[string]string{"status": "connected"}
	if len(lines) > 0 {
		result["hostname"] = strings.TrimSpace(lines[0])
	}
	if len(lines) > 1 {
		result["system"] = strings.TrimSpace(lines[1])
	}
	writeJSON(w, 200, result)
}

func (a *App) listSSHKeys(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,name,public_key,
		(SELECT COUNT(*) FROM agents WHERE agents.ssh_key_id=ssh_keys.id),created_at
		FROM ssh_keys ORDER BY name`)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	keys := []SSHKey{}
	for rows.Next() {
		var key SSHKey
		if rows.Scan(&key.ID, &key.Name, &key.PublicKey, &key.UsageCount, &key.CreatedAt) == nil {
			keys = append(keys, key)
		}
	}
	writeJSON(w, 200, keys)
}

func (a *App) createSSHKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string `json:"name"`
		PrivateKey string `json:"privateKey"`
		PublicKey  string `json:"publicKey"`
	}
	if err := decodeJSON(r, &in); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || strings.TrimSpace(in.PrivateKey) == "" {
		errorJSON(w, 400, "key name and private key are required")
		return
	}
	if _, err := ssh.ParsePrivateKey([]byte(in.PrivateKey)); err != nil {
		errorJSON(w, 400, "private key is not a valid unencrypted SSH key")
		return
	}
	signer, _ := ssh.ParsePrivateKey([]byte(in.PrivateKey))
	publicKey := strings.TrimSpace(in.PublicKey)
	if publicKey == "" {
		publicKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	} else if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey)); err != nil {
		errorJSON(w, 400, "public key is not a valid OpenSSH public key")
		return
	}
	encrypted, err := a.encrypt([]byte(in.PrivateKey))
	if err != nil {
		errorJSON(w, 500, "could not protect private key")
		return
	}
	result, err := a.db.ExecContext(r.Context(), "INSERT INTO ssh_keys(name,private_key,public_key) VALUES(?,?,?)", in.Name, encrypted, publicKey)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			errorJSON(w, 409, "a key with this name already exists")
			return
		}
		errorJSON(w, 500, err.Error())
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, 201, map[string]int64{"id": id})
}

func (a *App) generateSSHKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string `json:"name"`
		PrivateKey string `json:"privateKey"`
		PublicKey  string `json:"publicKey"`
	}
	if err := decodeJSON(r, &in); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		errorJSON(w, 400, "key name is required")
		return
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		errorJSON(w, 500, "could not generate SSH key")
		return
	}
	block, err := ssh.MarshalPrivateKey(private, in.Name)
	if err != nil {
		errorJSON(w, 500, "could not encode SSH key")
		return
	}
	privatePEM := pem.EncodeToMemory(block)
	sshPublic, err := ssh.NewPublicKey(public)
	if err != nil {
		errorJSON(w, 500, "could not encode public key")
		return
	}
	publicText := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPublic)))
	encrypted, err := a.encrypt(privatePEM)
	if err != nil {
		errorJSON(w, 500, "could not protect generated key")
		return
	}
	result, err := a.db.ExecContext(r.Context(), "INSERT INTO ssh_keys(name,private_key,public_key) VALUES(?,?,?)", in.Name, encrypted, publicText)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			errorJSON(w, 409, "a key with this name already exists")
			return
		}
		errorJSON(w, 500, err.Error())
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, 201, map[string]any{"id": id, "publicKey": publicText})
}

func (a *App) deleteSSHKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var usage int
	if err := a.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM agents WHERE ssh_key_id=?", id).Scan(&usage); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	if usage > 0 {
		errorJSON(w, 409, fmt.Sprintf("SSH key is used by %d machine(s) and cannot be removed", usage))
		return
	}
	result, err := a.db.ExecContext(r.Context(), "DELETE FROM ssh_keys WHERE id=?", id)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		errorJSON(w, 404, "SSH key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) listAgents(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), agentSelect+" ORDER BY name")
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	agents := []Agent{}
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err == nil {
			agents = append(agents, agent)
		}
	}
	writeJSON(w, 200, agents)
}

func (a *App) getAgent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	agent, err := scanAgent(a.db.QueryRowContext(r.Context(), agentSelect+" WHERE id=?", id))
	if errors.Is(err, sql.ErrNoRows) {
		errorJSON(w, 404, "machine not found")
		return
	}
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, agent)
}

func (a *App) deleteAgent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	result, err := a.db.ExecContext(r.Context(), "DELETE FROM agents WHERE id=?", id)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		errorJSON(w, 404, "machine not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) refreshAgent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := a.collect(ctx, id); err != nil {
		errorJSON(w, 502, err.Error())
		return
	}
	_, _ = a.db.ExecContext(ctx, "UPDATE containers SET image_checked_at=NULL WHERE agent_id=?", id)
	writeJSON(w, 200, map[string]string{"status": "online"})
}

func (a *App) overview(w http.ResponseWriter, r *http.Request) {
	var total, online, containers, running, services, servicesRunning, servicesFailed int
	var storageTotal, storageUsed, storageAvailable int64
	err := a.db.QueryRowContext(r.Context(), `SELECT
		(SELECT COUNT(*) FROM agents),
		(SELECT COUNT(*) FROM agents WHERE status='online'),
		(SELECT COUNT(*) FROM containers),
		(SELECT COUNT(*) FROM containers WHERE state='running'),
		(SELECT COALESCE(SUM(total_bytes),0) FROM storage_volumes WHERE lower(fs_type) NOT IN ('overlay','nfs','nfs4','cifs','smb3')),
		(SELECT COALESCE(SUM(used_bytes),0) FROM storage_volumes WHERE lower(fs_type) NOT IN ('overlay','nfs','nfs4','cifs','smb3')),
		(SELECT COALESCE(SUM(available_bytes),0) FROM storage_volumes WHERE lower(fs_type) NOT IN ('overlay','nfs','nfs4','cifs','smb3')),
		(SELECT COUNT(*) FROM system_services),
		(SELECT COUNT(*) FROM system_services WHERE active_state='active'),
		(SELECT COUNT(*) FROM system_services WHERE active_state='failed' OR sub_state='failed')`).
		Scan(&total, &online, &containers, &running, &storageTotal, &storageUsed, &storageAvailable, &services, &servicesRunning, &servicesFailed)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"machines": total, "online": online, "containers": containers, "running": running,
		"storageTotal": storageTotal, "storageUsed": storageUsed, "storageAvailable": storageAvailable,
		"services": services, "servicesRunning": servicesRunning, "servicesFailed": servicesFailed,
	})
}

func (a *App) listContainers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,name,image,status,state,ports,created,uptime FROM containers WHERE agent_id=? ORDER BY name`, id)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []Container{}
	for rows.Next() {
		var c Container
		if rows.Scan(&c.ID, &c.Name, &c.Image, &c.Status, &c.State, &c.Ports, &c.Created, &c.Uptime) == nil {
			items = append(items, c)
		}
	}
	writeJSON(w, 200, items)
}

func (a *App) listFleetContainers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT
		c.id,c.name,c.image,c.status,c.state,c.ports,c.created,c.uptime,
		c.image_version,c.update_available,c.image_checked_at,c.compose_project,c.compose_service,
		a.id,a.name,a.host,a.access_level
		FROM containers c
		JOIN agents a ON a.id=c.agent_id
		ORDER BY a.name,c.name`)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []FleetContainer{}
	for rows.Next() {
		var item FleetContainer
		var updateAvailable sql.NullBool
		var imageCheckedAt sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Image, &item.Status, &item.State, &item.Ports, &item.Created, &item.Uptime,
			&item.Version, &updateAvailable, &imageCheckedAt, &item.ComposeProject, &item.ComposeService,
			&item.AgentID, &item.AgentName, &item.AgentHost, &item.AgentAccess,
		); err != nil {
			errorJSON(w, 500, err.Error())
			return
		}
		if item.Version == "" {
			item.Version = containerVersion(item.Image)
		}
		if updateAvailable.Valid {
			item.UpdateAvailable = &updateAvailable.Bool
		}
		if imageCheckedAt.Valid {
			item.ImageCheckedAt = &imageCheckedAt.Time
		}
		items = append(items, item)
	}
	writeJSON(w, 200, items)
}

func (a *App) inspectContainer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	containerID := r.PathValue("container")
	if containerID == "" {
		errorJSON(w, 400, "container id is required")
		return
	}
	var image, cachedRegistry string
	var cachedUpdate sql.NullBool
	var checkedAt sql.NullTime
	if err := a.db.QueryRowContext(r.Context(), "SELECT image,registry_digest,update_available,image_checked_at FROM containers WHERE agent_id=? AND id=?", id, containerID).Scan(&image, &cachedRegistry, &cachedUpdate, &checkedAt); errors.Is(err, sql.ErrNoRows) {
		errorJSON(w, 404, "container not found")
		return
	} else if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	checkRegistry := !checkedAt.Valid || time.Since(checkedAt.Time) >= 24*time.Hour
	registryCommand := "true"
	if checkRegistry {
		registryCommand = `docker buildx imagetools inspect "$image" --format '{{json .Manifest.Digest}}' 2>/dev/null || docker manifest inspect --verbose "$image" 2>/dev/null || true`
	}
	command := fmt.Sprintf(dockerEnvironment+`cid=%s; image=%s;
printf 'IMAGE_ID\n'; docker inspect --format '{{.Image}}' "$cid" 2>/dev/null || true;
printf '\nRESTART\n'; docker inspect --format '{{.HostConfig.RestartPolicy.Name}}' "$cid" 2>/dev/null || true;
printf '\nHEALTH\n'; docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$cid" 2>/dev/null || true;
printf '\nCOMPOSE\n'; docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}|{{index .Config.Labels "com.docker.compose.service"}}' "$cid" 2>/dev/null || true;
printf '\nNETWORKS\n'; docker inspect --format '{{range $name, $network := .NetworkSettings.Networks}}{{$name}} {{$network.IPAddress}}{{"\n"}}{{end}}' "$cid" 2>/dev/null || true;
printf '\nMOUNTS\n'; docker inspect --format '{{range .Mounts}}{{.Type}} {{.Destination}}{{"\n"}}{{end}}' "$cid" 2>/dev/null || true;
printf '\nIMAGE_META\n'; docker image inspect --format '{{json .RepoDigests}}|{{.Created}}|{{.Size}}|{{.Os}}/{{.Architecture}}' "$image" 2>/dev/null || true;
printf '\nREGISTRY\n'; %s`, shellQuote(containerID), shellQuote(image), registryCommand)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	out, err := a.runSSH(ctx, id, command)
	if err != nil {
		errorJSON(w, 502, err.Error())
		return
	}
	sections := parseSections(out)
	details := ContainerDetails{
		ImageID:       strings.TrimSpace(sections["IMAGE_ID"]),
		RestartPolicy: strings.TrimSpace(sections["RESTART"]),
		Health:        strings.TrimSpace(sections["HEALTH"]),
		Networks:      nonEmptyLines(sections["NETWORKS"]),
		Mounts:        nonEmptyLines(sections["MOUNTS"]),
	}
	compose := strings.SplitN(strings.TrimSpace(sections["COMPOSE"]), "|", 2)
	if len(compose) == 2 {
		if compose[0] != "<no value>" {
			details.ComposeProject = compose[0]
		}
		if compose[1] != "<no value>" {
			details.ComposeService = compose[1]
		}
	}
	meta := strings.SplitN(strings.TrimSpace(sections["IMAGE_META"]), "|", 4)
	if len(meta) == 4 {
		var digests []string
		_ = json.Unmarshal([]byte(meta[0]), &digests)
		if len(digests) > 0 {
			if at := strings.LastIndex(digests[0], "@"); at >= 0 {
				details.LocalDigest = digests[0][at+1:]
			}
		}
		details.ImageCreated = meta[1]
		details.ImageSize, _ = strconv.ParseInt(meta[2], 10, 64)
		details.Platform = meta[3]
	}
	details.RegistryDigest = cachedRegistry
	if cachedUpdate.Valid {
		available := cachedUpdate.Bool
		details.UpdateAvailable = &available
	}
	if checkRegistry {
		details.RegistryDigest = findDigest([]byte(sections["REGISTRY"]))
		if details.RegistryDigest == "" {
			details.RegistryDigest = a.publicRegistryDigest(r.Context(), image)
		}
	}
	if checkRegistry && details.LocalDigest != "" && details.RegistryDigest != "" {
		available := details.LocalDigest != details.RegistryDigest
		details.UpdateAvailable = &available
	}
	var updateValue any
	if details.UpdateAvailable != nil {
		updateValue = *details.UpdateAvailable
	}
	if checkRegistry {
		_, _ = a.db.ExecContext(r.Context(), `UPDATE containers SET image_version=?,update_available=?,registry_digest=?,image_checked_at=CURRENT_TIMESTAMP,compose_project=?,compose_service=? WHERE agent_id=? AND id=?`,
			containerVersion(image), updateValue, details.RegistryDigest, details.ComposeProject, details.ComposeService, id, containerID)
	} else {
		_, _ = a.db.ExecContext(r.Context(), `UPDATE containers SET image_version=?,compose_project=?,compose_service=? WHERE agent_id=? AND id=?`,
			containerVersion(image), details.ComposeProject, details.ComposeService, id, containerID)
	}
	writeJSON(w, 200, details)
}

func (a *App) containerAction(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	containerID := r.PathValue("container")
	action := r.PathValue("action")
	if action != "start" && action != "stop" && action != "restart" && action != "update" {
		errorJSON(w, 400, "action must be start, stop, restart, or update")
		return
	}
	var exists int
	if err := a.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM containers WHERE agent_id=? AND id=?", id, containerID).Scan(&exists); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	if exists == 0 {
		errorJSON(w, 404, "container not found")
		return
	}
	if action == "update" {
		a.updateComposeContainer(w, r, id, containerID)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	out, err := a.runSSH(ctx, id, dockerEnvironment+fmt.Sprintf("docker %s %s", action, shellQuote(containerID)))
	if err != nil {
		errorJSON(w, 502, err.Error())
		return
	}
	if err := a.collect(ctx, id); err != nil && !strings.Contains(err.Error(), "already in progress") {
		errorJSON(w, 502, fmt.Sprintf("container action succeeded but inventory refresh failed: %v", err))
		return
	}
	status := map[string]string{"start": "started", "stop": "stopped", "restart": "restarted"}[action]
	writeJSON(w, 200, map[string]string{"status": status, "output": strings.TrimSpace(out)})
}

func (a *App) updateComposeContainer(w http.ResponseWriter, r *http.Request, id int64, containerID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	var containerName, image, targetDigest string
	if err := a.db.QueryRowContext(ctx, "SELECT name,image,registry_digest FROM containers WHERE agent_id=? AND id=?", id, containerID).Scan(&containerName, &image, &targetDigest); err != nil {
		errorJSON(w, 404, "container not found")
		return
	}
	inspect := dockerEnvironment + fmt.Sprintf(`docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}{{"\n"}}{{index .Config.Labels "com.docker.compose.service"}}{{"\n"}}{{index .Config.Labels "com.docker.compose.project.working_dir"}}{{"\n"}}{{index .Config.Labels "com.docker.compose.project.config_files"}}' %s`, shellQuote(containerID))
	out, err := a.runSSH(ctx, id, inspect)
	if err != nil {
		errorJSON(w, 502, err.Error())
		return
	}
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(out), "\r\n", "\n"), "\n")
	if len(lines) < 4 || lines[0] == "" || lines[1] == "" || lines[0] == "<no value>" || lines[1] == "<no value>" {
		errorJSON(w, 409, "automatic updates are only supported for Docker Compose services")
		return
	}
	project, service, workingDir := lines[0], lines[1], lines[2]
	composeArgs := []string{"docker compose", "-p", shellQuote(project)}
	if workingDir != "" && workingDir != "<no value>" {
		composeArgs = append(composeArgs, "--project-directory", shellQuote(workingDir))
	}
	if lines[3] != "" && lines[3] != "<no value>" {
		for _, configFile := range strings.Split(lines[3], ",") {
			if configFile = strings.TrimSpace(configFile); configFile != "" {
				composeArgs = append(composeArgs, "-f", shellQuote(configFile))
			}
		}
	}
	compose := strings.Join(composeArgs, " ")
	command := dockerEnvironment + fmt.Sprintf("%s pull %s && %s up -d %s", compose, shellQuote(service), compose, shellQuote(service))
	flusher, ok := w.(http.Flusher)
	if !ok {
		errorJSON(w, 500, "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	var streamMu sync.Mutex
	emit := func(kind, data string) {
		streamMu.Lock()
		defer streamMu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"type": kind, "data": data})
		flusher.Flush()
	}
	emit("command", command)
	emit("status", "Pulling image and recreating Compose service…")
	err = a.runSSHStream(ctx, id, command, func(data []byte) { emit("output", string(data)) })
	if err != nil {
		emit("error", err.Error())
		return
	}
	emit("status", "Refreshing container inventory…")
	if err := a.collect(ctx, id); err != nil && !strings.Contains(err.Error(), "already in progress") {
		emit("error", fmt.Sprintf("container updated but inventory refresh failed: %v", err))
		return
	}
	_, _ = a.db.ExecContext(ctx, `UPDATE containers SET image_version=?,update_available=0,registry_digest=?,image_checked_at=CURRENT_TIMESTAMP,compose_project=?,compose_service=? WHERE agent_id=? AND name=?`,
		containerVersion(image), targetDigest, project, service, id, containerName)
	emit("complete", "Container updated successfully.")
}

func nonEmptyLines(value string) []string {
	lines := []string{}
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func findDigest(data []byte) string {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return ""
	}
	var walk func(any) string
	walk = func(current any) string {
		switch typed := current.(type) {
		case string:
			if strings.HasPrefix(typed, "sha256:") {
				return typed
			}
		case map[string]any:
			if digest, ok := typed["digest"].(string); ok && strings.HasPrefix(digest, "sha256:") {
				return digest
			}
			for _, child := range typed {
				if digest := walk(child); digest != "" {
					return digest
				}
			}
		case []any:
			for _, child := range typed {
				if digest := walk(child); digest != "" {
					return digest
				}
			}
		}
		return ""
	}
	return walk(value)
}

func containerVersion(image string) string {
	image = strings.TrimSpace(image)
	if at := strings.LastIndex(image, "@"); at >= 0 {
		digest := image[at+1:]
		if len(digest) > 19 {
			return digest[:19] + "…"
		}
		return digest
	}
	lastSlash := strings.LastIndex(image, "/")
	if colon := strings.LastIndex(image, ":"); colon > lastSlash {
		return image[colon+1:]
	}
	return "latest"
}

func (a *App) metrics(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT recorded_at,load1,memory_used,disk_used FROM metric_samples WHERE agent_id=? AND recorded_at > datetime('now','-24 hours') ORDER BY recorded_at`, id)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var at time.Time
		var load float64
		var mem, disk int64
		if rows.Scan(&at, &load, &mem, &disk) == nil {
			items = append(items, map[string]any{"at": at, "load": load, "memory": mem, "disk": disk})
		}
	}
	writeJSON(w, 200, items)
}

func (a *App) logs(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	container := r.URL.Query().Get("container")
	if container == "" {
		errorJSON(w, 400, "container is required")
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines < 1 || lines > 1000 {
		lines = 200
	}
	out, err := a.runSSH(r.Context(), id, fmt.Sprintf("docker logs --tail %d --timestamps %s 2>&1", lines, shellQuote(container)))
	if err != nil {
		errorJSON(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"logs": out})
}

const agentSelect = `SELECT id,name,host,port,username,auth_type,status,last_error,last_seen,os,kernel,uptime_seconds,load1,memory_total,memory_used,disk_total,disk_used,cpu_count,is_vm,virtualization,access_level,
(SELECT COUNT(*) FROM containers c WHERE c.agent_id=agents.id),
(SELECT COUNT(*) FROM containers c WHERE c.agent_id=agents.id AND c.state='running'),created_at FROM agents`

type scanner interface{ Scan(...any) error }

func scanAgent(s scanner) (Agent, error) {
	var a Agent
	var seen sql.NullTime
	err := s.Scan(&a.ID, &a.Name, &a.Host, &a.Port, &a.Username, &a.AuthType, &a.Status, &a.LastError, &seen, &a.OS, &a.Kernel, &a.UptimeSeconds, &a.Load1, &a.MemoryTotal, &a.MemoryUsed, &a.DiskTotal, &a.DiskUsed, &a.CPUCount, &a.IsVM, &a.Virtualization, &a.AccessLevel, &a.ContainerCount, &a.RunningCount, &a.CreatedAt)
	if seen.Valid {
		a.LastSeen = &seen.Time
	}
	return a, err
}

func (a *App) collect(ctx context.Context, id int64) error {
	if _, loaded := a.active.LoadOrStore(id, true); loaded {
		return errors.New("refresh already in progress")
	}
	defer a.active.Delete(id)
	const command = dockerEnvironment + `printf 'HOST\n'; uname -srm; printf '\nOS\n'; (grep PRETTY_NAME /etc/os-release 2>/dev/null | cut -d= -f2- | tr -d '"') || uname -s; printf '\nVIRT\n'; v=$(systemd-detect-virt 2>/dev/null || true); [ "$v" = "none" ] && v=physical; printf '%s\n' "${v:-unknown}"; printf '\nACCESS\n'; if [ "$(id -u)" -eq 0 ]; then printf 'root\n'; elif (command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1) || id -nG | tr ' ' '\n' | grep -Eq '^(sudo|wheel)$'; then printf 'sudo\n'; else printf 'regular\n'; fi; printf '\nUPTIME\n'; cut -d. -f1 /proc/uptime; printf '\nLOAD\n'; cut -d' ' -f1 /proc/loadavg; printf '\nCPU\n'; getconf _NPROCESSORS_ONLN; printf '\nMEM\n'; awk '/MemTotal/{t=$2*1024}/MemAvailable/{a=$2*1024}END{print t, t-a}' /proc/meminfo; printf '\nDISK\n'; df -B1 / | awk 'NR==2{print $2,$3}'; printf '\nSTORAGE\n'; df -B1 -P -T 2>/dev/null | awk 'NR>1 && $2 !~ /^(tmpfs|devtmpfs|squashfs|overlay)$/ {mount=$7; for(i=8;i<=NF;i++) mount=mount " " $i; print $1 "|" $2 "|" $3 "|" $4 "|" $5 "|" mount}' || true; printf '\nINTERFACES\n'; ip -j address show 2>/dev/null || true; printf '\nSERVICES\n'; systemctl list-units --type=service --all --no-legend --no-pager --plain 2>/dev/null | awk '{description=""; for(i=5;i<=NF;i++) description=description (i==5?"":" ") $i; print $1 "|" $2 "|" $3 "|" $4 "|" description}' || true; printf '\nDOCKER\n'; docker ps -a --no-trunc --format '{{json .}}' 2>/dev/null || true`
	out, err := a.runSSH(ctx, id, command)
	if err != nil {
		_, _ = a.db.Exec("UPDATE agents SET status='offline',last_error=? WHERE id=?", err.Error(), id)
		return err
	}
	sections := parseSections(out)
	var uptime int64
	var load float64
	var cpus int
	fmt.Sscan(sections["UPTIME"], &uptime)
	fmt.Sscan(sections["LOAD"], &load)
	fmt.Sscan(sections["CPU"], &cpus)
	var memTotal, memUsed, diskTotal, diskUsed int64
	fmt.Sscan(sections["MEM"], &memTotal, &memUsed)
	fmt.Sscan(sections["DISK"], &diskTotal, &diskUsed)
	hostParts := strings.Fields(sections["HOST"])
	kernel := sections["HOST"]
	if len(hostParts) > 1 {
		kernel = strings.Join(hostParts[1:], " ")
	}
	virtualization := strings.TrimSpace(sections["VIRT"])
	accessLevel := strings.TrimSpace(sections["ACCESS"])
	if accessLevel != "root" && accessLevel != "sudo" {
		accessLevel = "regular"
	}
	isVM := virtualization != "" && virtualization != "physical" && virtualization != "unknown" && virtualization != "none"
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE agents SET status='online',last_error='',last_seen=CURRENT_TIMESTAMP,os=?,kernel=?,uptime_seconds=?,load1=?,memory_total=?,memory_used=?,disk_total=?,disk_used=?,cpu_count=?,is_vm=?,virtualization=?,access_level=? WHERE id=?`, sections["OS"], kernel, uptime, load, memTotal, memUsed, diskTotal, diskUsed, cpus, isVM, virtualization, accessLevel, id)
	if err != nil {
		return err
	}
	seenContainers := []string{}
	for _, line := range strings.Split(sections["DOCKER"], "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw struct{ ID, Names, Image, Status, State, Ports, CreatedAt, RunningFor string }
		if json.Unmarshal([]byte(line), &raw) == nil {
			seenContainers = append(seenContainers, raw.ID)
			_, _ = tx.ExecContext(ctx, `INSERT INTO containers(agent_id,id,name,image,status,state,ports,created,uptime,image_version) VALUES(?,?,?,?,?,?,?,?,?,?)
				ON CONFLICT(agent_id,id) DO UPDATE SET
				name=excluded.name,image=excluded.image,status=excluded.status,state=excluded.state,ports=excluded.ports,created=excluded.created,uptime=excluded.uptime,updated_at=CURRENT_TIMESTAMP,
				image_version=CASE WHEN containers.image=excluded.image THEN containers.image_version ELSE excluded.image_version END,
				update_available=CASE WHEN containers.image=excluded.image THEN containers.update_available ELSE NULL END,
				registry_digest=CASE WHEN containers.image=excluded.image THEN containers.registry_digest ELSE '' END,
				image_checked_at=CASE WHEN containers.image=excluded.image THEN containers.image_checked_at ELSE NULL END`,
				id, raw.ID, raw.Names, raw.Image, raw.Status, raw.State, raw.Ports, raw.CreatedAt, raw.RunningFor, containerVersion(raw.Image))
		}
	}
	if err := storeSystemInventory(ctx, tx, id, sections); err != nil {
		return err
	}
	if len(seenContainers) == 0 {
		_, _ = tx.ExecContext(ctx, "DELETE FROM containers WHERE agent_id=?", id)
	} else {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(seenContainers)), ",")
		args := make([]any, 0, len(seenContainers)+1)
		args = append(args, id)
		for _, containerID := range seenContainers {
			args = append(args, containerID)
		}
		_, _ = tx.ExecContext(ctx, "DELETE FROM containers WHERE agent_id=? AND id NOT IN ("+placeholders+")", args...)
	}
	_, _ = tx.ExecContext(ctx, `INSERT INTO metric_samples(agent_id,load1,memory_used,disk_used) VALUES(?,?,?,?)`, id, load, memUsed, diskUsed)
	_, _ = tx.ExecContext(ctx, `DELETE FROM metric_samples WHERE recorded_at < datetime('now','-7 days')`)
	return tx.Commit()
}

func parseSections(out string) map[string]string {
	result := map[string]string{}
	current := ""
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		trim := strings.TrimSpace(line)
		switch trim {
		case "HOST", "OS", "VIRT", "ACCESS", "UPTIME", "LOAD", "CPU", "MEM", "DISK", "STORAGE", "INTERFACES", "SERVICES", "DOCKER",
			"IMAGE_ID", "RESTART", "HEALTH", "COMPOSE", "NETWORKS", "MOUNTS", "IMAGE_META", "REGISTRY":
			current = trim
		default:
			if current != "" {
				if result[current] != "" {
					result[current] += "\n"
				}
				result[current] += trim
			}
		}
	}
	return result
}

func loadKey(dir string) ([]byte, error) {
	if encoded := os.Getenv("SECONTROL_MASTER_KEY"); encoded != "" {
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(key) != 32 {
			return nil, errors.New("SECONTROL_MASTER_KEY must be base64 for exactly 32 bytes")
		}
		return key, nil
	}
	path := filepath.Join(dir, "master.key")
	if key, err := os.ReadFile(path); err == nil && len(key) == 32 {
		return key, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, err
	}
	return key, nil
}
func (a *App) encrypt(plain []byte) ([]byte, error) {
	nonce := make([]byte, a.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return a.aead.Seal(nonce, nonce, plain, nil), nil
}
func (a *App) decrypt(data []byte) ([]byte, error) {
	n := a.aead.NonceSize()
	if len(data) < n {
		return nil, errors.New("invalid ciphertext")
	}
	return a.aead.Open(nil, data[:n], data[n:], nil)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		errorJSON(w, 400, "invalid machine id")
		return 0, false
	}
	return id, true
}
func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func errorJSON(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
