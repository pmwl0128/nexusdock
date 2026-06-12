# 完善 Nexus 前端真实数据与功能闭环

## Goal

把 Nexus 控制台里已经暴露出来但没有真实数据闭环的入口补上第一轮可用能力，降低“有页面但没数据”的空壳感。重点是让总览和待办使用真实 JSON API，让前端不再把 SPA HTML fallback 静默当成兼容模式，并验证浏览器里主要入口可以读取实时数据。

## What I Already Know

* 用户指出前端很多页面没有数据、功能没有发挥出来，并要求继续。
* 当前运行服务在 `http://127.0.0.1:18777/ui/`。
* 实时 API 当前有数据：`/v1/devices` 有 1 台 DockMini，`/api/v1/skills` 有 5 条，`/api/v1/runs` 有 31 条，`/api/v1/schedules` 有 1 条。
* `/api/v1/nexus/overview`、`/api/v1/tasks`、`/api/tasks`、`/v1/tasks` 当前返回 SPA HTML fallback，不是真 JSON API。
* 前端 `web/src/App.tsx` 已经有 `HomePage`、`InboxPage`、`SkillsPage`、`RunsPage`、`SchedulesPage`，但总览和待办依赖未注册的 API。
* `internal/tasks` 已有 task service/model/repository，但当前 `internal/httpx.Server` 未接入 task service。
* `internal/httpx/catalog_runs.go` 已经从 devices/commands 派生 skills 和 runs 列表。

## Assumptions

* 第一轮先解决最影响使用感的空壳入口，不做整套视觉重设计。
* Task inbox 可以先由运行态聚合生成，优先暴露设备异常、失败命令、失败计划任务等可处理事项；持久化 task service 接入可以后续再做。
* 保持 MemoryDock compatibility server 的 `{items: ...}` 响应风格，避免打破现有前端 helper。

## Requirements

* 后端提供真实 JSON `GET /api/v1/nexus/overview` 与兼容别名。
* 后端提供真实 JSON `GET /v1/tasks`、`GET /api/v1/tasks`、`GET /api/tasks`，至少覆盖当前控制台待办页需要的字段。
* 概览指标应从真实 devices、commands、schedules、skills 数据派生，避免页面固定显示无意义 0。
* 前端待办页使用真实 task API，并继续显示 live/error/empty 状态。
* 前端总览优先用真实 overview API，仍保留 devices/schedules fallback，但不应把 HTML fallback 误导成成功数据。
* 更新或补充测试，覆盖新增 API 返回 JSON、任务聚合和概览计数。

## Acceptance Criteria

* [x] `GET /api/v1/nexus/overview` 返回 JSON，包含 `agent_tasks`、`user_tasks`、`device_alerts`、`skill_candidates`、`memory_conflicts`、`recent_failures`。
* [x] `GET /v1/tasks` 返回 JSON `{items:[...]}`，不会再落到 UI HTML。
* [x] 当前运行态有失败/异常时，待办页能显示对应条目；没有时显示真实空状态而不是兼容模式。
* [x] `npm run build` 成功并刷新 embedded assets。
* [x] Go 测试覆盖新增 HTTP 行为。
* [x] 浏览器验证 `总览 / 待办 / 能力 / 运行 / 计划任务 / 设置` 主要入口无控制台错误，关键页面能显示真实状态。

## Definition of Done

* Tests added/updated.
* Relevant lint/type/build checks pass.
* Embedded frontend assets regenerated when frontend changes.
* Runtime browser check completed against `127.0.0.1:18777`.

## Out of Scope

* 不重做整套 UI 视觉系统。
* 不把 `internal/tasks` 的持久化服务完整接入 MemoryDock runtime。
* 不新增外部依赖。
* 不做复杂任务工作流编辑、claim/complete 操作界面。

## Technical Notes

* Frontend rules: `.trellis/spec/frontend/index.md`
* Backend rules: `.trellis/spec/backend/index.md` plus error/quality/directory guides.
* Cross-layer concern: API fallback HTML vs JSON parsing was the concrete failure boundary.
* Candidate files: `internal/httpx/server.go`, `internal/httpx/control_plane.go`, `internal/httpx/catalog_runs.go`, `internal/httpx/schedules.go`, `web/src/App.tsx`, HTTP tests.
