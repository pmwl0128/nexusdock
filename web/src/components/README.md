# Web Components

本目录用于拆分 AgentDock Nexus Web UI 组件。

当前入口位于 `web/src/App.tsx`，`MemoryWorkspace.tsx` 保留旧 MemoryDock 工作区兼容能力。新增页面和交互应优先按领域拆分为可复用组件，避免继续扩大入口文件。

建议的组件边界：

- Dashboard / Overview
- Inbox
- Devices
- Skills
- Runs
- Memory Explorer / Editor / Diff / Timeline
- Git Sync
- Access Settings
