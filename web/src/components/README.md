# Web Components

本目录用于拆分 NexusDock Web UI 组件。

当前入口位于 `web/src/App.tsx`，`RecallWorkspace.tsx` 承载 Recall 工作区能力。新增页面和交互应优先按领域拆分为可复用组件，避免继续扩大入口文件。

当前产品一级入口固定为：

- 总览 / Dashboard
- 召回 / Recall
- 文件 / Files
- 运行时 / Runtime
- 设置 / Settings

组件边界建议：

- `components/Dialog.tsx`：跨页面复用的站内模态框，避免回退到浏览器原生 `prompt` / `confirm`。
- Recall Explorer / Editor / Diff / Timeline 后续应从 `RecallWorkspace.tsx` 继续拆分，但不新增独立 Inbox 或 Task 产品入口。
- Runtime 页面只作为 AgentDock Runtime API-backed view；Files / Backup / Access Settings 后续按领域拆分，不要重新引入独立 Run、Skill Catalog、Worker 或 Context Pack 页面。
