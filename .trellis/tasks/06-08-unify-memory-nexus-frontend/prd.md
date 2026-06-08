# 统一 Memory 与 Nexus 前台界面风格

## Goal

重构前台视觉语言，让 Nexus 控制台和 Memory 工作台看起来属于同一个产品面。Memory 仍然保留文件树、Markdown 编辑、Git diff、同步等高效工作区能力，但不再使用独立的玻璃态/紫蓝渐变风格，而是服从 Nexus 的运维控制台外壳、色彩、按钮、面板和密度。

## What I Already Know

* 用户希望前台整体重构，UI 由 Codex 决定。
* 用户明确提到“记忆和 nexus 界面风格统一化”。
* 前端位于 `web/`，React/Vite，主要入口是 `web/src/App.tsx` 和 `web/src/MemoryWorkspace.tsx`。
* Nexus 风格在 `web/src/nexus.css`：深色侧栏、浅色内容区、控制台式卡片、12-17px 半径、低饱和状态色。
* Memory 工作台风格在 `web/src/styles.css`：独立玻璃态、紫蓝渐变、浮动底部导航、大半径面板，和 Nexus 明显割裂。
* 任务是前台 UI 重构，不应引入新的后端 API 契约。

## Assumptions

* 统一方向以 Nexus 控制台为主，Memory 作为 Nexus 的子工作区，而不是并列的第二套产品。
* 保留现有 Memory 交互和 API 路由，优先通过 CSS 和少量结构调整统一观感。
* 桌面和移动都要可用；移动端必须保留 Nexus 顶部搜索能力。

## Requirements

* Nexus 和 Memory 使用同一套颜色 token、按钮、输入框、面板、状态标识和阴影层级。
* Memory 页面取消独立玻璃态/营销感背景，改成 Nexus 控制台背景和工作台面板。
* Memory 的顶部栏、侧边导航、文件树、编辑器、Git、Sync、Dashboard 面板与 Nexus 的视觉密度一致。
* Memory 入口从 Nexus 返回时保持清晰，不制造第二套品牌识别。
* 不改变后端 API、路由契约和 Memory 功能行为。
* 不引入新的图标库或 UI 框架；继续使用 lucide-react。

## Acceptance Criteria

* [ ] `web/src/styles.css` 的 Memory 风格与 `web/src/nexus.css` 的 Nexus 风格一致，不再主导使用紫蓝渐变/玻璃态/orb 背景。
* [ ] Memory 导航、按钮、输入框和面板在桌面与移动宽度下不出现文字重叠或水平溢出。
* [ ] `npm run build` 在 `web/` 通过。
* [ ] 本地浏览器验证 `/ui/` 桌面与移动宽度下页面可见、布局合理、控制台无明显错误。
* [ ] 如果 build 更新 `internal/httpx/web_dist`，一并纳入变更。

## Definition Of Done

* 前端源码修改完成。
* 构建通过。
* 浏览器实际验证桌面和移动关键页面。
* 按 Trellis 检查流程完成质量检查。

## Out Of Scope

* 新增或修改后端 API。
* 重做 Memory 的数据模型、Markdown 解析器或 Git/sync 行为。
* 引入新设计系统依赖。
* 改动 DockMini Runtime、Skill Runtime 或 MemoryDock 运行时配置。

## Technical Notes

* 前端规范：`.trellis/spec/frontend/index.md`。
* Nexus 主要 CSS：`web/src/nexus.css`。
* Memory 主要 CSS：`web/src/styles.css`。
* Memory 组件：`web/src/MemoryWorkspace.tsx`。
* Nexus app shell：`web/src/App.tsx`。
