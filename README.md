# SeControl

SeControl is an agentless infrastructure control plane. It connects to Linux machines over SSH, collects host health, discovers Docker containers, and presents the result in a compact web dashboard on port 5000.

## Run with Docker

```bash
docker compose up --build
```

Open `http://localhost:5000`. Data and the generated credential-encryption key are stored in the explicitly named `secontrol-data` Docker volume. Normal rebuilds, container replacement, and `docker compose down` preserve this volume.

## What the first version includes

- SSH connections using a password or pasted private key
- Required SSH connection test before a machine can be saved
- Reusable encrypted SSH key-pair management in Settings
- GUI-based Ed25519 key generation and public-key import
- Per-key machine usage counts and deletion protection for assigned keys
- Copyable, non-root Linux bootstrap commands for the current user's `authorized_keys`
- AES-GCM encryption for stored credentials
- Linux OS, kernel, CPU count, uptime, load, memory, and root-disk collection
- Network interface inventory and systemd service status, including stopped and failed services
- Fleet-wide mounted filesystem inventory with used, free, and total capacity
- Docker container inventory, on-demand logs, Compose metadata, guarded runtime controls, and one-click Compose image updates
- Rootless Docker discovery through login-shell environments and per-user daemon sockets
- Public OCI/Docker registry digest fallback when remote Buildx or manifest plugins are unavailable
- SQLite-backed daily image-version and update-status caching with forced checks on refresh
- SQLite persistence with WAL mode and seven days of metric samples
- Automatic polling (60 seconds by default) and manual refresh
- Responsive dashboard and JSON API
- Persistent light/dark appearance preference, with dark mode by default
- Content-hashed JavaScript and stylesheet URLs generated during Docker builds
- Health endpoint at `/api/health`

The remote user needs permission to execute `docker ps` and `docker logs`. Rootless installations may keep the Docker CLI in `$HOME/bin`; SeControl includes that path automatically. No agent is installed on the target machine.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `SECONTROL_ADDR` | `:5000` | HTTP listen address |
| `SECONTROL_DATA_DIR` | `./data` | SQLite database and generated master key |
| `SECONTROL_POLL_INTERVAL` | `60s` | Fleet collection interval |
| `SECONTROL_MASTER_KEY` | generated | Optional base64-encoded 32-byte encryption key |

For production, put the service behind TLS and authentication, set a stable `SECONTROL_MASTER_KEY` in your secret manager, restrict network access, and replace the current trust-on-first-use SSH host-key behavior with stored host fingerprints.

## Local development

Requires Go 1.24+:

```bash
cd src
go mod tidy
go run ./cmd/secontrol
```

## API

- `GET /api/overview`
- `GET /api/storage`
- `GET|POST /api/agents`
- `POST /api/agents/test`
- `GET|POST /api/ssh-keys`
- `POST /api/ssh-keys/generate`
- `DELETE /api/ssh-keys/{id}`
- `GET|DELETE /api/agents/{id}`
- `POST /api/agents/{id}/refresh`
- `GET /api/agents/{id}/containers`
- `GET /api/agents/{id}/metrics`
- `GET /api/agents/{id}/system`
- `GET /api/agents/{id}/logs?container=<id>&lines=200`

## Next production steps

The schema and package layout leave room for authentication/RBAC, SSH host-key pinning, alert rules, additional disks and interfaces, container lifecycle actions, terminal access, Postgres, and multi-node scheduling.
