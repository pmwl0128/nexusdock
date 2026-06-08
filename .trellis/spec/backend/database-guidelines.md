# Database Guidelines

The backend uses SQLite through `database/sql` and `modernc.org/sqlite`. Database code must be deterministic, migration-backed, and safe to run on local developer machines and long-lived MemoryDock deployments.

## SQLite Setup

- Open databases through `internal/core.OpenSQLite`; it applies absolute paths, directory creation, foreign keys, busy timeout, WAL mode for file databases, and connection pool limits.
- Use `:memory:` only in tests or explicitly isolated flows; `OpenSQLite` forces a single connection for in-memory databases.
- Do not open SQLite directly from feature packages unless you are extending `internal/core`.

## Repositories

- Domain packages define a repository interface and a SQLite implementation.
- Repository methods should accept `context.Context` and return domain models, not transport DTOs.
- Preserve optimistic locking where a model has `Version`; check expected versions in the service layer before repository updates when the caller supplies one.
- Translate SQLite constraint/lock conflicts into stable domain or `core.CodedError` values instead of leaking raw driver text to API callers.

## Transactions

- Use `core.SQLTxManager` or package-local transaction helpers for multi-step writes.
- Roll back on every error path and convert commit conflicts with `core.IsSQLiteConflict`.
- Keep callbacks small and free of network or filesystem side effects; emit events after the database state is committed unless the existing code explicitly documents otherwise.

## Migrations

- Add schema changes as numbered SQL files under `migrations/`.
- Never edit an applied migration casually; `MigrationRunner` stores checksums and treats checksum drift as `DB_CONFLICT`.
- Add new migrations with a strictly increasing numeric prefix such as `0003_feature_name.sql`.
- Let `core.NewMigrationRunner(...).Run(ctx)` create and validate `schema_migrations`.
- Preserve backup hooks for deployed databases before applying pending migrations.

## Naming Conventions

- Use snake_case for table and column names.
- Use stable text enum values that match contract names where those values cross API boundaries.
- Store times as RFC 3339 text unless a package already owns a different documented representation.
- Store secret digests, never plaintext credentials or tokens.

## Common Mistakes

- Updating `contracts/` without matching migration/service behavior.
- Adding a new required column without a migration path for existing rows.
- Relying on SQLite defaults instead of explicit service-level validation.
- Returning raw SQLite errors from HTTP handlers.
