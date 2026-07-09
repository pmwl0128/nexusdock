# 完善 NexusDock 前端

## Goal

把现有 Nexus 前端从概念展示型首页推进为更实用的控制台体验：用户进入后能快速看到设备、计划任务、运行异常和后续动作，同时保持现有后端 API 兼容，不新增契约依赖。

## What I Already Know

- 前端位于 `web/`，是 Vite + React + TypeScript，入口为 `web/src/main.tsx` 和 `web/src/App.tsx`。
- 当前已有主要页面：Home、Inbox、Devices、Memory、Skills、Runs、计划任务、Settings。
- `DevicesPage` 和 `EnvManagerPage` 已经是较完整的真实控制台页面，包含注册、审批、命令、Env 管理等流程。
- `HomePage` 当前偏概念型，展示里程碑和系统态势，但对真实对象的扫描、失败定位和下一步操作帮助不足。
- `nexus.css` 当前大量压缩在少数长行中，后续维护困难；可以追加格式化样式段降低风险。
- 初始 `npm run build` 已通过。

## Requirements

- 首页必须更像运维/控制台工作台，而不是营销或概念页。
- 不新增后端接口；优先复用已有 `/v1/devices`、`/v1/schedules`、`/api/v1/nexus/overview` 等资源探测逻辑。
- 保留现有 Devices、Memory、Env Manager 的功能入口和行为。
- 移动端不能丢失全局搜索能力；小屏布局不得遮挡或溢出主要操作。
- UI 风格应保持安静、密集、可扫描，避免夸张 hero 和纯装饰元素。
- Secret 或 Token 不得在新增界面中回显。

## Acceptance Criteria

- [x] 首页展示真实设备/计划任务摘要或兼容态说明。
- [x] 首页提供可点击的关键动作入口，直达 Devices、Memory、Settings、Schedules 等页面。
- [x] 全局搜索在移动端可用。
- [x] `npm run build` 通过。
- [x] 本地 dev server 可打开，桌面与移动视口截图无明显重叠、空白或不可读文本。

## Definition of Done

- TypeScript build and Vite production build pass.
- Browser verification covers desktop and mobile viewport.
- Git diff is scoped to the frontend improvement and Trellis task files.

## Technical Approach

- 在 `HomePage` 中复用 `useResource` 拉取设备与计划任务，派生控制台摘要，不增加 API。
- 增加工作台模块：关键动作、真实对象概览、需要关注事项。
- 调整 topbar 搜索容器，让移动端保留输入框而不是隐藏整个搜索。
- 追加格式化 CSS 覆盖与新增类，减少改动压缩 CSS 的风险。

## Out of Scope

- 不新增后端 Nexus API。
- 不拆分 `App.tsx` 的模块结构；本任务先做前端体验闭环，后续可单独重构。
- 不更改 MemoryWorkspace 的独立样式体系。
- 不改设备命令、Env Manager 的业务语义。

## Technical Notes

- Relevant files inspected: `web/package.json`, `web/src/App.tsx`, `web/src/nexus.css`, `web/src/components/devices/DevicesPage.tsx`, `web/src/components/env/EnvManagerPage.tsx`, `web/src/main.tsx`.
- Applicable Trellis specs are currently backend placeholders only; frontend work follows existing local React/CSS patterns plus general UI constraints.
