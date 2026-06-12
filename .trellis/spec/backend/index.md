# Backend Development Guidelines

These guidelines describe the current AgentDock Nexus backend conventions. They are executable project rules for humans and AI agents working in this repository.

## Scope

Applies to Go code under `cmd/`, `internal/`, `migrations/`, `generated/`, `contracts/`, `scripts/`, and backend-facing tests under `tests/`.

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | Module organization and ownership boundaries | Active |
| [Database Guidelines](./database-guidelines.md) | SQLite, migrations, repositories, transactions | Active |
| [Dashboard Compatibility APIs](./dashboard-compatibility-apis.md) | MemoryDock-compatible Nexus console JSON routes | Active |
| [Error Handling](./error-handling.md) | Domain errors, API responses, validation boundaries | Active |
| [Quality Guidelines](./quality-guidelines.md) | Required checks, generated-code rules, forbidden patterns | Active |
| [Logging Guidelines](./logging-guidelines.md) | `slog` usage and secret-safe logging | Active |

## Pre-Development Checklist

- Read [Directory Structure](./directory-structure.md) before adding or moving packages.
- Read [Database Guidelines](./database-guidelines.md) before changing repositories, migrations, or SQLite behavior.
- Read [Dashboard Compatibility APIs](./dashboard-compatibility-apis.md) before adding or changing `internal/httpx` routes consumed by the Nexus console.
- Read [Error Handling](./error-handling.md) before adding service methods or HTTP handlers.
- Read [Quality Guidelines](./quality-guidelines.md) before changing contracts, generated files, tests, or build scripts.
- Read [Logging Guidelines](./logging-guidelines.md) before adding operational logs.

## Required Verification

Run the narrowest relevant checks while iterating, then finish backend changes with:

```bash
go test ./...
go vet ./...
python3 scripts/check-contracts.py
```

If the change touches embedded UI assets or `web/`, also run:

```bash
cd web && npm run build
```
