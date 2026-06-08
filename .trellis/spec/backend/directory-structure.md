# Directory Structure

AgentDock Nexus is a Go single-repo backend with a React/Vite frontend. Keep backend modules narrow, package-local, and aligned to product domains.

## Top-Level Layout

```text
cmd/
  memorydock/       # MemoryDock-compatible Memory API and Web entrypoint
  nexus-server/     # Nexus control-plane HTTP service
  nexus-worker/     # background worker entrypoint
contracts/          # OpenAPI, JSON Schema, event schemas, error code baseline
generated/          # generated Go client/DTO code from contracts
internal/
  api/              # shared API middleware and DTO package markers
  audit/ auth/ runs/
  commands/ devices/
  core/             # database, config, migrations, IDs, event bus, core errors
  evolution/
  httpx/            # Memory-compatible HTTP API and embedded UI server
  memory/ syncer/
  skills/
  tasks/
migrations/         # embedded SQLite migrations
scripts/            # reproducible local verification and code generation
tests/              # cross-package integration and acceptance tests
web/                # React/Vite UI; build output is embedded under internal/httpx/web_dist
```

## Module Organization

- Put domain models, repositories, and services in `internal/<domain>/`.
- Keep HTTP wiring in command packages or `internal/httpx`; domain packages should not depend on HTTP handlers.
- Keep cross-cutting database/config/error primitives in `internal/core`.
- Keep MemoryDock compatibility under `cmd/memorydock` and `internal/httpx`; do not mix compatibility-specific behavior into `cmd/nexus-server` unless the API contract requires it.
- Keep generated code under `generated/` and regenerate it from `scripts/generate-contracts.py`; never hand-edit generated files.
- Keep schema changes in numbered files under `migrations/` and embed them through `migrations/embed.go`.

## Naming Conventions

- Package names are short lowercase domain names: `devices`, `commands`, `memory`, `syncer`.
- Repository interfaces live near the domain service as `repository.go`; SQLite implementations use `sqlite_repository.go`.
- Service implementations use `service.go`, with focused tests in `service_test.go`.
- Domain errors use `errors.go` in the owning package unless the error is shared across packages, in which case use `internal/core`.
- Cross-package integration tests belong under `tests/<domain>/`.

## Examples

- `internal/devices/` shows the expected service/repository/model split.
- `internal/commands/` shows a domain service that depends on another domain through a narrow authorization interface.
- `internal/core/migrations.go` owns migration loading and checksum validation.
- `cmd/nexus-server/app.go` owns HTTP route registration for the control-plane server.
