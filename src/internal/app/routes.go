package app

import (
	"net/http"

	"github.com/kozmixb/secontrol/assets"
)

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/overview", a.overview)
	mux.HandleFunc("GET /api/containers", a.listFleetContainers)
	mux.HandleFunc("GET /api/storage", a.listFleetStorage)
	mux.HandleFunc("GET /api/agents", a.listAgents)
	mux.HandleFunc("POST /api/agents", a.createAgent)
	mux.HandleFunc("POST /api/agents/test", a.testAgentConnection)
	mux.HandleFunc("GET /api/ssh-keys", a.listSSHKeys)
	mux.HandleFunc("POST /api/ssh-keys", a.createSSHKey)
	mux.HandleFunc("POST /api/ssh-keys/generate", a.generateSSHKey)
	mux.HandleFunc("PATCH /api/ssh-keys/{id}", a.renameSSHKey)
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
