# Dashboard Compatibility APIs

`internal/httpx` serves the MemoryDock-compatible UI and a small set of Nexus
console APIs. Dashboard APIs must be explicit JSON routes because otherwise the
SPA fallback can return HTML to the React client and look like a data source
until JSON parsing fails.

## Scenario: Nexus Console Dashboard Data

### 1. Scope / Trigger

- Trigger: adding or changing console data routes under `internal/httpx` that
  back `web/src/App.tsx` overview, inbox, skills, runs, schedules, or search
  surfaces.
- This applies to compatibility aliases as well as primary `/v1/...` routes.
- This does not replace the contract-first `cmd/nexus-server` OpenAPI surface;
  it documents the MemoryDock-compatible runtime routes used by `/ui/`.

### 2. Signatures

- `GET /api/v1/nexus/overview`
- `GET /v1/nexus/overview`
- `GET /v1/tasks`
- `GET /api/v1/tasks`
- `GET /api/tasks`
- `func (s *Server) nexusOverview(w http.ResponseWriter, r *http.Request)`
- `func (s *Server) listDashboardTasks(w http.ResponseWriter, r *http.Request)`
- `func (s *Server) dashboardState(ctx context.Context) (dashboardOverview, []dashboardTask, error)`

### 3. Contracts

- Routes are protected with `withAPIAccess`, so callers may use the configured
  bearer admin token or configured Basic Auth credentials.
- Successful responses must have `Content-Type: application/json` and must never
  return the SPA HTML fallback.
- Overview response fields:
  - `agent_tasks int`
  - `user_tasks int`
  - `device_alerts int`
  - `skill_candidates int`
  - `memory_conflicts int`
  - `recent_failures int`
- Task list response shape is `{ "items": dashboardTask[] }`.
- Dashboard task fields:
  - `id string`
  - `title string`
  - `type string` such as `needs_agent` or `needs_user`
  - `status string`
  - `source string` such as `device_approval`, `device_alert`,
    `memory_conflict`, `run_failure`, or `schedule_failure`
  - `updated_at string` in RFC3339/RFC3339Nano format when available
- Dashboard state may be derived from devices, command runs, skill summaries,
  and the backup schedule status. If no actionable data exists, return a real
  empty state (`{ "items": [] }`) rather than compatibility mode.

### 4. Validation & Error Matrix

- Missing or invalid credentials -> `401` via the compatibility error helper.
- Device or command service validation/authorization errors ->
  `writeControlPlaneError`.
- Device or command service infrastructure errors -> `500 INTERNAL_ERROR` via
  `writeControlPlaneError`.
- Missing schedule status file -> `never_run` schedule state and no failed task.
- Invalid schedule status JSON -> `unknown` schedule state and no failed task.
- Missing control-plane services -> return schedule-derived dashboard state only.

### 5. Good/Base/Bad Cases

- Good: a pending device returns one `device_approval` task and increments
  `user_tasks`.
- Good: a failed command returns a `run_failure` task and increments
  `recent_failures`.
- Good: heartbeat memory conflicts return a `memory_conflict` task and increment
  `memory_conflicts`.
- Base: an online device with successful schedules can return `{ "items": [] }`
  while still reporting real skill candidates or other live counts.
- Bad: `/v1/tasks` is unregistered and falls through to `/ui/` HTML.
- Bad: frontend code treats `INVALID_JSON` from an HTML fallback as live data.

### 6. Tests Required

- HTTP tests must cover every public alias added for dashboard data.
- Tests must unmarshal responses as JSON and assert the expected top-level shape.
- Tests must cover at least one live aggregation source for overview counts and
  task list entries.
- When the frontend source changes, run `npm run build` in `web/` and include
  refreshed `internal/httpx/web_dist` assets.
- Runtime verification should check authenticated JSON responses on
  `127.0.0.1:18777` after deployment when the task Definition of Done names the
  live service.

### 7. Wrong vs Correct

#### Wrong

```go
// Missing route registration lets the SPA handler answer with text/html.
// The frontend then discovers the problem only after JSON parsing fails.
mux.HandleFunc("GET /ui/", uiProtected(s.uiIndex))
```

#### Correct

```go
mux.HandleFunc("GET /v1/tasks", protected(s.listDashboardTasks))
mux.HandleFunc("GET /api/v1/tasks", protected(s.listDashboardTasks))
mux.HandleFunc("GET /api/tasks", protected(s.listDashboardTasks))
```

#### Wrong

```tsx
const resource = useResource<Task[]>(['/api/v1/tasks', '/api/tasks'], [], refreshToken);
```

#### Correct

```tsx
const resource = useResource<Task[]>(['/v1/tasks', '/api/v1/tasks', '/api/tasks'], [], refreshToken);
```
