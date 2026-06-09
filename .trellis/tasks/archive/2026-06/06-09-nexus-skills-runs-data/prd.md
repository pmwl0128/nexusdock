# 补齐 Nexus 能力目录和运行数据

## Goal

修复 Nexus Web 中能力目录、运行记录和设备指标的空数据体验，让页面从真实 Nexus API 获取 JSON 数据；当节点尚未上报指标或 Skill summary 时，UI 明确显示未上报，而不是误导性地显示空页或 0 / 0。

## What I Already Know

- Web 能力页请求 `/api/v1/skills` 和 `/api/skills`，运行页请求 `/api/v1/runs` 和 `/api/runs`。
- 当前 `memorydock` 服务没有注册这些 API，未命中的路径会落到 UI HTML，前端解析为 `INVALID_JSON` 后走兼容空态。
- `/v1/devices` 已能返回真实 DockMini 设备和 5 个 capability。
- 设备详情的资源指标直接读取最新 heartbeat metrics；当前 DockMini 心跳上报的 `cpu_percent`、`memory_percent`、`disk_percent` 都是 0。
- 设备卡片的 Skill 数量来自 `heartbeat.skills`，当前为 `null`。

## Requirements

- 增加 Skills API，返回 JSON 数组，不再让前端请求拿到 HTML。
- Skills API 优先聚合设备 heartbeat 的 Skill summary；没有 summary 时聚合设备 capability，保证能力目录能显示已知能力。
- 增加 Runs API，聚合设备命令历史为前端现有 `Run` card 可消费的字段。
- 前端设备卡片把未上报 Skill summary 显示成“未上报”，不再显示 `0 / 0`。
- 前端资源指标在 heartbeat metrics 全部为 0 时标记为“未上报”，避免把缺采集误读成真实 0%。

## Acceptance Criteria

- [ ] `/api/v1/skills` 和 `/api/skills` 返回 JSON，而不是 UI HTML。
- [ ] `/api/v1/runs` 和 `/api/runs` 返回 JSON，而不是 UI HTML。
- [ ] 能力页在当前 DockMini 数据下展示非空卡片。
- [ ] 运行页在有设备命令历史时展示记录；没有历史时返回空 JSON 数组。
- [ ] 设备详情指标缺采集时显示“未上报”。
- [ ] Go tests 和 Web build 通过。

## Out of Scope

- 不在本任务中修改 AgentDock 节点侧真实 CPU/内存/磁盘采集逻辑。
- 不新增数据库迁移。
- 不实现完整 Skill 导入/发布工作流，只补当前页面需要的列表数据。

## Technical Notes

- Backend: `internal/httpx/server.go`, `internal/httpx/control_plane.go`, `internal/devices`, `internal/commands`。
- Frontend: `web/src/App.tsx`, `web/src/components/devices/DevicesPage.tsx`。
- Spec: backend directory/error/quality guidelines, frontend index, cross-layer guide。
