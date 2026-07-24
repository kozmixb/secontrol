package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kozmixb/secontrol/internal/app"
)

func main() {
	addr := env("SECONTROL_ADDR", ":5000")
	dataDir := env("SECONTROL_DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		panic(err)
	}

	control, err := app.New(filepath.Join(dataDir, "secontrol.db"), dataDir)
	if err != nil {
		panic(err)
	}
	defer control.Close()

	poll, err := time.ParseDuration(env("SECONTROL_POLL_INTERVAL", "60s"))
	if err != nil {
		panic(errors.New("invalid SECONTROL_POLL_INTERVAL"))
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go control.Poll(ctx, poll)

	server := &http.Server{
		Addr:              addr,
		Handler:           control.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		slog.Info("SeControl listening", "address", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
