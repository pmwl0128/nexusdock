# Quality Guidelines

Backend changes should stay small, contract-aware, and verifiable with local commands.

## Required Checks

Use focused tests while editing, then run the relevant final checks:

```bash
go test ./...
go vet ./...
python3 scripts/check-contracts.py
```

For full repository confidence or frontend-adjacent backend work, also run:

```bash
cd web && npm run build
```

## Required Patterns

- Run `gofmt` on Go files before final verification.
- Keep generated contracts reproducible through `python3 scripts/generate-contracts.py`.
- Let `scripts/check-contracts.py` detect stale generated output, missing schema descriptions, unresolved refs, event mismatches, and v1 compatibility breaks.
- Add or update tests when changing service behavior, repository behavior, migrations, authentication, command state transitions, or contract generation.
- Keep compatibility code explicit: `cmd/memorydock` is a compatibility entrypoint; `cmd/nexus-server` is the Nexus control plane.

## Forbidden Patterns

- Do not hand-edit files under `generated/nexuscontracts/`.
- Do not edit generated `contracts/jsonschema/*.json`, event schemas, or OpenAPI output without updating `scripts/generate-contracts.py` when the generator owns the shape.
- Do not introduce plaintext secrets into tests, docs, fixtures, logs, or responses.
- Do not bypass service-layer validation by writing directly through repositories from HTTP handlers.
- Do not add shell execution or arbitrary command payloads to the device control plane; commands must remain typed and policy-checked.
- Do not add frontend API assumptions that are absent from `contracts/` or the existing API helpers.

## Testing Expectations

- Domain services need unit tests for validation, state transitions, and idempotency.
- SQLite repositories need tests for persistence, optimistic locking, and conflict behavior.
- Cross-domain flows belong under `tests/<domain>/` when they exercise multiple packages.
- Migration changes require migration tests or integration coverage that opens a fresh database and applies all migrations.
- Security-sensitive changes need tests for unauthorized, forbidden, and revoked/expired cases.

## Review Checklist

- Does the change preserve `contracts/` as the single source of truth?
- Are generated files either untouched or regenerated intentionally?
- Are errors stable and non-secret?
- Are database changes migration-backed and backward-compatible?
- Are Makefile, README, Docker, and docs entrypoints still accurate?
