# Error Handling

Backend errors should preserve internal context for logs and tests while exposing stable, non-secret codes to API callers.

## Error Types

- Shared API/control-plane errors use `internal/core.CodedError` and stable codes from `internal/core/errors.go`.
- Domain packages may own local error constants or wrappers in `errors.go` when the errors are not shared outside the package.
- Wrap unexpected infrastructure failures with `%w` so callers can inspect causes.
- Use validation errors for bad caller input, not generic internal errors.

## Service Layer Patterns

- Validate required fields at service entry points before repository writes.
- Keep authorization and state-transition checks in services, not repositories.
- Return idempotent success for already-completed safe operations when the existing service contract does so, such as approving an already-approved device.
- Do not log or return plaintext tokens, cookies, passwords, authorization headers, or secret environment values.

## HTTP Patterns

- Decode JSON through helpers that call `DisallowUnknownFields`.
- Map stable error codes to HTTP status consistently:
  - auth required, invalid token, revoked token: `401`
  - forbidden: `403`
  - validation: `400`
  - not found: `404`
  - version or database conflicts: `409`
  - unknown infrastructure failure: `500`
- Response bodies should include a stable code and a user-facing message; avoid exposing raw SQL, filesystem, or secret-bearing details.
- Limit request bodies where handlers accept arbitrary JSON.

## Contract Boundary

- Contract-visible errors must be represented in `contracts/error-codes.json` and regenerated if they affect generated DTOs.
- Device command, skill run, memory write, and migration errors should leave enough state or audit evidence for operators to determine whether work completed.

## Scenario: Nexus ErrorResponse Contract

### 1. Scope / Trigger

- Trigger: HTTP errors returned by `cmd/nexus-server` are public Nexus API responses consumed by generated clients and the React console.
- This does not apply to MemoryDock compatibility handlers under `internal/httpx`, which intentionally keep their legacy `{ "ok": false, "error": { ... } }` shape for existing Memory clients.

### 2. Signatures

- `cmd/nexus-server.writeError(w http.ResponseWriter, r *http.Request, err error)`
- `cmd/nexus-server.writeErrorStatus(w http.ResponseWriter, r *http.Request, status int, code core.ErrorCode, message string)`
- `middleware.Authenticate(..., onError func(http.ResponseWriter, *http.Request, error), ...)`

### 3. Contracts

- Nexus error response body must match `contracts/jsonschema/ErrorResponse.json`:

```json
{
  "code": "AUTH_REQUIRED",
  "message": "bearer token is required",
  "request_id": "req_..."
}
```

- `request_id` comes from `middleware.RequestIDFromContext(r.Context())`; handlers must receive `*http.Request` when writing errors so the request context is available.
- Nexus control-plane errors must not include the Memory compatibility `ok` flag or nested `error` object.

### 4. Validation & Error Matrix

- missing bearer token -> `401` + `AUTH_REQUIRED`
- missing required scope -> `403` + `FORBIDDEN`
- invalid JSON or multiple JSON documents -> `400` + `VALIDATION_ERROR`
- not found -> `404` + `NOT_FOUND`
- version/database conflict -> `409` + `VERSION_CONFLICT` or `DB_CONFLICT`
- unexpected infrastructure failure -> `500` + `INTERNAL_ERROR`

### 5. Good/Base/Bad Cases

- Good: `/v1/auth/me` without credentials returns top-level `code`, `message`, and `request_id`.
- Base: `/health` may keep its simple success payload because it is not an error response.
- Bad: Nexus API returns `{ "ok": false, "error": { "code": "AUTH_REQUIRED" } }`, causing generated clients and frontend fallback logic to parse different shapes.

### 6. Tests Required

- Add or update `cmd/nexus-server` HTTP tests when changing error helpers.
- Assert status code, stable `code`, non-empty `message`, propagated `request_id`, and absence of compatibility-only fields.

### 7. Wrong vs Correct

#### Wrong

```go
writeJSON(w, http.StatusUnauthorized, map[string]any{
	"ok": false,
	"error": map[string]any{"code": core.CodeAuthRequired, "message": "bearer token is required"},
})
```

#### Correct

```go
writeJSON(w, http.StatusUnauthorized, map[string]any{
	"code": core.CodeAuthRequired,
	"message": "bearer token is required",
	"request_id": middleware.RequestIDFromContext(r.Context()),
})
```

## Common Mistakes

- Returning raw driver errors directly to API clients.
- Treating version conflicts as generic validation failures.
- Accepting unknown JSON fields in control-plane write endpoints.
- Emitting secrets in errors during config, auth, env, or skill flows.
- Reusing MemoryDock compatibility error payloads in Nexus control-plane handlers.
