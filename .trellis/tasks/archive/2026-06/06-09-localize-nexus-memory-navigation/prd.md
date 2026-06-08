# 中文化 Nexus 与 Memory 导航

## Goal

把 Nexus 控制台和 Memory 工作台的导航文案统一改为中文，让前台界面更像面向中文用户的同一套产品，同时保留 Memory 内部 Dashboard / Explorer / Git / Sync 的二级导航结构。

## What I Already Know

* 用户明确要求“导航改中文”。
* 用户明确确认“保留二级导航吧”。
* Nexus 一级导航位于 `web/src/App.tsx` 的 `NAV` 常量，目前仍有 Home / Inbox / Devices / Memory / Skills / Runs / Settings 等英文标签。
* Memory 工作台内部二级导航位于 `web/src/MemoryWorkspace.tsx` 的 `MEMORY_NAV`，当前结构已是内部二级导航，部分文案已中文化。
* Memory 工作台周边仍有若干导航相关英文状态或提示，如 `Nexus Memory`、`Knowledge workspace`、`Git backed memory`、`Online`、`files`、`dirs`、`Explorer`、`Review Studio`。
* 旧任务 `06-08-unify-memory-nexus-frontend` 已归档，本任务只承接导航中文化，不重开视觉重构。

## Assumptions

* 一级导航中文化不改变 hash route、section id、搜索 target 或 API 契约。
* Memory 内部二级导航继续保留在 Memory 工作台内部，不迁入 Nexus 主侧栏。
* 英文技术标识如 API 字段、命令类型、类型名、代码变量名不属于本任务范围。

## Requirements

* Nexus 一级导航显示中文：总览、待办、设备、记忆、Skills/能力、运行、计划任务、设置等，优先使用中文用户能直接理解的短标签。
* Memory 工作台保留内部二级导航结构，二级导航标签继续使用中文。
* 清理导航与顶部状态附近残留英文，使 Memory 内部工作台品牌、状态、计数和入口提示不再混杂明显英文。
* 不改变路由、数据加载、Git、Sync、编辑器、命令面板等功能行为。
* 不引入新依赖，不调整后端 API。

## Acceptance Criteria

* [x] `web/src/App.tsx` 的主导航标签均为中文。
* [x] `web/src/MemoryWorkspace.tsx` 仍保留 `MEMORY_NAV` 内部二级导航，不把 Memory 子入口移入 Nexus 主导航。
* [x] Memory 顶栏/侧栏/二级导航附近不再显示明显英文状态词，如 `Online`、`files`、`dirs`、`Explorer`、`Review Studio`。
* [x] `npm run build` 在 `web/` 通过。
* [x] 本地浏览器验证 `/ui/` 与 Memory 页面在桌面/移动宽度下可见且导航不溢出。
* [x] 如果构建更新 `internal/httpx/web_dist`，一并纳入变更。

## Definition Of Done

* PRD 已记录用户决策。
* 前端文案修改完成。
* 构建通过。
* 浏览器实际验证关键导航。
* 按 Trellis 检查流程完成质量检查。

## Out Of Scope

* 重新设计 Nexus 或 Memory 布局。
* 修改 Memory 数据模型、Markdown 编辑、Git diff 或同步行为。
* 修改后端 API、运行时配置、DockMini 或 MemoryDock 部署。
* 全量翻译所有开发者 README 或内部代码标识。

## Technical Notes

* 前端规范入口：`.trellis/spec/frontend/index.md`。
* Nexus 主导航：`web/src/App.tsx`。
* Memory 二级导航：`web/src/MemoryWorkspace.tsx`。
* 视觉统一旧任务参考：`.trellis/tasks/archive/2026-06/06-08-unify-memory-nexus-frontend/prd.md`。
