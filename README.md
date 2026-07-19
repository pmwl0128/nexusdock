# NexusDock

NexusDock is the personal AgentDock control plane. Recall is NexusDock's memory module: Git-backed Markdown content, notes, cards, embeddings, and sync live inside this service alongside backups, administrator sessions, and Runtime views backed by AgentDock Runtime APIs.

## Product Boundary

Top-level product areas:

- Overview: backup status and high-priority runtime availability signals.
- Recall: NexusDock memory module for unified memory, notes, cards, inbox, Markdown editing, Git review, embeddings, and sync.
- Runtime: explicitly selected AgentDock node task, Skill, and dynamic MCP views through AgentDock Runtime APIs; Workflow templates remain a Nexus-global registry.
- Settings: administrator account, browser sessions, Nexus data health, Recall repository location, and backup status.

Nexus does not own AgentDock Task, Skill, or dynamic MCP lifecycle state. It stores only node connection metadata and encrypted node credentials, then queries or triggers the selected AgentDock through controlled Runtime APIs. Workflow templates are global Nexus data consumed by AgentDock.

## Runtime Structure

```text
cmd/nexusdock          production service entrypoint
internal/recall    NexusDock Recall memory module: Markdown content, notes, cards, embeddings, and Git sync
internal/agentdock encrypted AgentDock node registry and credentials
internal/auth      administrator sessions and device authentication
internal/httpx     Nexus HTTP API, Runtime API facade, and embedded Web UI
web                React Nexus console
```

Production builds use `cmd/nexusdock`. Product vocabulary, public contracts, deployment variables, and UI copy must use NexusDock, Nexus, Recall, and Runtime consistently.

## Data Layout

```text
NEXUS_DATA_DIR/
  nexus.db
  backups/
  secrets/
    agentdock-nodes.key

RECALL_REPO_DIR/
  .git/
  profile.md
  recall/docs/
  recall/managed/
```

AgentDock 节点 Token 使用 `secrets/agentdock-nodes.key` 加密后存入 `nexus.db`。备份和恢复时必须同时保留数据库与该密钥文件；只恢复其中一项会导致已有节点凭据无法解密。

## Local Development

Requirements:

- Go version declared by `go.mod`.
- Node.js 26 and npm.
- Python 3 for contract and repository checks.
- Docker for production-image verification.

```bash
make web-deps
make check
make ci
```

`make check` performs formatting drift, module tidiness, Go tests, `go vet`, contract generation drift, and repository-boundary checks. `make ci` additionally builds the embedded Web UI, runs focused race tests, and builds the production binary.

The embedded UI lives in `internal/httpx/web_dist`. A frontend change is incomplete until `make web-build` has regenerated this directory and the resulting files are committed.

## Administrator Authentication

Browser administrator credentials live only in `NEXUS_DATA_DIR/nexus.db` and use Argon2id. Initialize or recover the administrator from a local terminal so the credential never enters Compose files, shell history, or Git:

```bash
NEXUS_DATA_DIR=./nexus-data ./bin/nexusdock admin init owner
NEXUS_DATA_DIR=./nexus-data ./bin/nexusdock admin recover owner
```

Browser requests use the administrator session cookie. Programmatic `/v1` clients use `Authorization: Bearer $NEXUS_AUTH_TOKEN`; a direct loopback fallback exists only for isolated tests or embedded uses that do not configure the web authentication service. Client-controlled `Host` and forwarding headers never grant local access.

`NEXUS_REQUIRE_AUTH=true` is a startup guard: it refuses to start unless `NEXUS_AUTH_TOKEN` is configured. `NEXUS_TRUSTED_PROXIES` must contain only the reverse proxy addresses that are allowed to supply `X-Forwarded-*` headers. Leave `NEXUS_AUTH_ALLOW_INSECURE_HTTP=false` outside local development so browser login requires HTTPS.

The production image runs as fixed UID/GID `10001:10001`. Compose uses a read-only root filesystem, drops all Linux capabilities, enables `no-new-privileges`, and mounts the Git credential-store file as a read-only Secret. Host bind mounts for `NEXUS_DATA_DIR` and `RECALL_REPO_DIR` must be writable by UID/GID 10001; see `deploy/README.md` for the permission preflight.
