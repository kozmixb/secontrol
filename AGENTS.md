# SeControl contributor guide

This file is maintained with the project and applies to all repository work.

## Project layout

- `src/` contains the complete Go module and embedded web application.
- `src/cmd/secontrol/` contains the executable entry point.
- `src/internal/app/` contains HTTP, SQLite, encryption, SSH, and collection logic.
- `src/assets/` contains the dashboard assets and their Go embed package.
- `Dockerfile` and `docker-compose.yml` define the supported container workflow.

## Working conventions

- Keep all Go source, module, test, and embedded application files under `src/`.
- Run Go commands from `src/`.
- Use Docker Compose for running and validating the application. Do not replace the persistent `secontrol-data` volume.
- Format changed Go files with `gofmt`.
- Run `go test ./...` before considering a Go change complete.
- Keep the service listening on port `5000` unless the configuration contract is intentionally changed.
- Preserve the agentless model: target machines are accessed over SSH and must not require installed SeControl software.
- Never log, return, or commit passwords, private keys, master keys, or decrypted credentials.
- Never clear, recreate, or remove persisted application data unless the user explicitly requests data clearing.
- Use parameterized SQL and preserve SQLite foreign-key and WAL configuration.
- Treat remote command construction as a security boundary. Validate or shell-quote every user-controlled value.
- Update `README.md`, `.env.example`, and container configuration when public behavior or configuration changes.

## Product direction

SeControl should remain easy to deploy as one container, provide a clear modern operator experience, and leave clean extension points for authentication, SSH host-key pinning, alerts, PostgreSQL, and distributed collection.
