# NexusDock

NexusDock is the personal AgentDock control plane. It manages devices, Recall content, backups, administrator sessions, and a Runtime view backed by AgentDock Runtime APIs.

## Product Boundary

Top-level product areas:

- Overview: device health, backup status, and high-priority runtime availability signals.
- Devices: enrollment, approval, heartbeat, capabilities, policy, environment actions, structured commands, and history.
- Recall: unified memory, notes, cards, inbox, Markdown editing, Git review, and sync.
- Runtime: AgentDock Runtime task, skill, workflow, capability, and log views through AgentDock Runtime APIs.
- Settings: administrator account, browser sessions, Nexus data health, Recall repository location, and backup status.

Nexus does not own AgentDock Task, Skill, or Workflow lifecycle state. Those systems belong to AgentDock Runtime. Nexus only queries or triggers them through controlled Runtime APIs.

## Runtime Structure

```text
cmd/nexusdock          production service entrypoint
internal/recall    Recall Markdown content, notes, cards, and Git sync
internal/devices   device enrollment, heartbeat, policy, and structured commands
internal/commands  device command queue
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

RECALL_REPO_DIR/
  .git/
  profile.md
  recall/docs/
  recall/managed/
```
