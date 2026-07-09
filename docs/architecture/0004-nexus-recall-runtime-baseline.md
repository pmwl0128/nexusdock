# ADR-0004: Nexus, Recall, and Runtime Ownership Baseline

- Status: Accepted
- Date: 2026-07-07

## Decision

Nexus is the personal AgentDock control plane. Recall is a Git-backed content repository for memory, notes, cards, inbox, and sync. AgentDock Runtime owns Task, Skill, and Workflow lifecycles.

Nexus stores its system state in `NEXUS_DATA_DIR`. Recall content is stored in `RECALL_REPO_DIR`. The two directories must be configurable independently.

Runtime data must enter Nexus through AgentDock Runtime APIs. Nexus must not create independent Task, Skill, or Workflow lifecycle tables, registries, state files, or direct filesystem writers.

## Consequences

- Recall repository migration must not move Nexus device registration, administrator sessions, or command history.
- Nexus backup and restore can treat system state and Recall content as separate units.
- Runtime views are allowed to be read-only when AgentDock Runtime lacks controlled write APIs.
- Public contracts must name Runtime resources as Runtime views or Runtime actions, not Nexus-owned Task or Skill systems.
- Legacy names are not part of the design vocabulary for new code.
