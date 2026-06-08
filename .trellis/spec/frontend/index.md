# Frontend Development Guidelines

## Scope

Applies to the React/Vite app under `web/`, including the embedded build output served from `internal/httpx/web_dist`.

## Nexus Console Rules

- Treat `web/src/App.tsx` pages as operational console surfaces, not landing pages. Prefer dense, scannable panels, direct action entry points, and visible live/compatibility state.
- Do not add new backend API dependencies for dashboard polish unless the task explicitly changes contracts. Reuse existing resource helpers and fallback paths first.
- Route API calls through `web/src/api/client.ts`; it must parse both Nexus top-level `ErrorResponse` (`code`, `message`, `request_id`) and MemoryDock compatibility errors (`error.code`, `error.message`) because `/ui/` can be served by both surfaces.
- Mobile navigation must preserve the global search input. Do not hide `.nexus-search-wrap` without providing an equivalent search affordance.
- If `npm run build` is used for verification, include the generated `internal/httpx/web_dist` asset changes with the frontend source change because `web/vite.config.ts` writes there intentionally.

## Quality Check

- Run `npm run build` from `web/`.
- Verify the local `/ui/` route in a browser at desktop and mobile widths for horizontal overflow, unreadable text, and overlapping controls.
- Check browser console warnings/errors when UI behavior or CSS changes.
