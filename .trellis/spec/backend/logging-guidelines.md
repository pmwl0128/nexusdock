# Logging Guidelines

The backend uses the standard library `log/slog` for operational logs.

## Logger Setup

- Entry points create a JSON logger with `slog.NewJSONHandler(os.Stderr, ...)`.
- Respect configured log levels through project config helpers such as `cfg.LogLevel()`.
- Pass `*slog.Logger` into server or manager constructors instead of using package globals in domain code.

## Levels

- `Debug`: request traces and low-level diagnostic events that are useful during local troubleshooting.
- `Info`: service startup, shutdown, enabled modes, addresses, and successful high-level lifecycle events.
- `Warn`: recoverable degraded behavior where the operator may need to inspect configuration or environment.
- `Error`: startup failures, listener failures, migration failures, and operation failures that prevent requested work from completing.

## Structured Fields

- Prefer key/value fields over formatted strings.
- Use stable field names such as `error`, `addr`, `store_dir`, `auto_sync`, `method`, and `path`.
- Include request IDs or run IDs when they are available from middleware or run context.
- Log service state, not secret-bearing input.

## What To Log

- Startup configuration that is safe to disclose, such as bind address, store directory, and feature flags.
- Migration failure context without dumping SQL payloads.
- Sync, Git, command, device, run, and audit lifecycle transitions when they matter operationally.
- HTTP request method/path at debug level, as `internal/httpx` currently does.

## What Not To Log

- Bearer tokens, device tokens, enrollment tokens, API keys, cookies, passwords, authorization headers, private keys, or secret env values.
- Full request bodies for endpoints that may carry secret or user content.
- Raw command output unless the caller has explicitly requested diagnostics and output is redacted.
- Memory contents by default; log paths and operation summaries instead.
