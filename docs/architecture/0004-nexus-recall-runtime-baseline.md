# ADR-0004: Nexus, Recall, and Runtime Ownership Baseline

- Status: Accepted
- Date: 2026-07-07

## Decision

NexusDock is the personal AgentDock control plane. Recall is NexusDock's memory module and Git-backed content repository for memory, notes, cards, inbox, embeddings, and sync. AgentDock Runtime owns Task, Skill, and dynamic MCP lifecycles. Workflow templates are a Nexus-global registry consumed by AgentDock.

NexusDock stores control-plane system state in `NEXUS_DATA_DIR`. NexusDock Recall stores content in `RECALL_REPO_DIR`. The two data domains remain configurable independently even though they are served by the same NexusDock process.

Runtime data must enter Nexus through AgentDock Runtime APIs. Nexus must not create independent Task or Skill lifecycle tables, registries, state files, or direct filesystem writers. Workflow templates are the explicit global exception and are not scoped to an AgentDock node.

## Consequences

- Recall repository migration must not move Nexus device registration, administrator sessions, or command history.
- Nexus backup and restore can treat system state and Recall content as separate units.
- Runtime views are allowed to be read-only when AgentDock Runtime lacks controlled write APIs.
- Public contracts must name Runtime resources as Runtime views or Runtime actions, not Nexus-owned Task or Skill systems.
- Legacy names are not part of the design vocabulary for new code.
