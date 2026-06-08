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

## Common Mistakes

- Returning raw driver errors directly to API clients.
- Treating version conflicts as generic validation failures.
- Accepting unknown JSON fields in control-plane write endpoints.
- Emitting secrets in errors during config, auth, env, or skill flows.
