# AgentDock Nexus

AgentDock Nexus is the personal AgentDock control plane. It manages devices, Recall content, encrypted file relay, backups, administrator sessions, and a Runtime view backed by AgentDock Runtime APIs.

## Product Boundary

Top-level product areas:

- Overview: device health, file transfer status, backup status, and high-priority runtime availability signals.
- Devices: enrollment, approval, heartbeat, capabilities, policy, environment actions, structured commands, and history.
- Recall: unified memory, notes, cards, inbox, Markdown editing, Git review, and sync.
- Files: encrypted Artifact delivery and reverse Fetch state.
- Runtime: AgentDock Runtime task, skill, workflow, capability, and log views through AgentDock Runtime APIs.
- Settings: administrator account, browser sessions, Nexus data health, Recall repository location, backup status, and deployment diagnostics.

Nexus does not own AgentDock Task, Skill, or Workflow lifecycle state. Those systems belong to AgentDock Runtime. Nexus only queries or triggers them through controlled Runtime APIs.

## Runtime Structure

```text
cmd/nexus          production service entrypoint
cmd/recalldock     deprecated compatibility wrapper
internal/recall    Recall Markdown content, notes, cards, and Git sync
internal/devices   device enrollment, heartbeat, policy, and structured commands
internal/commands  device command queue
internal/artifacts encrypted Artifact Relay and Fetch
internal/auth      administrator sessions and device authentication
internal/httpx     Nexus HTTP API, Runtime API facade, and embedded Web UI
web                React Nexus console
```

Production builds use `cmd/nexus`. `cmd/recalldock` is kept only as a deprecated compatibility wrapper for existing local commands. New product vocabulary, public contracts, deployment variables, and UI copy must use Nexus, Recall, and Runtime.

## Data Layout

```text
NEXUS_DATA_DIR/
  nexus.db
  artifacts/
  backups/

RECALL_REPO_DIR/
  .git/
  profile.md
  recall/docs/
  recall/managed/
```
