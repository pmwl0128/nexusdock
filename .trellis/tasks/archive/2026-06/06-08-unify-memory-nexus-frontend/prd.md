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
* 真实环境记忆确认：当前 `/Volumes/KIOXIA/Docker/agentdock-nexus/source` 是真实部署目录，前端构建产物需要随源码变更一起处理；MemoryDock 真实服务由 Docker Compose 管理，当前任务不改运行时配置。
* `.trellis/spec/frontend/index.md` 明确要求：Memory 在 `.nexus-memory-mode` 下应作为 Nexus workbench surface；统一样式应优先作用于 `.nexus-memory-mode`，避免 legacy MemoryDock 独立 UI 风格反向污染控制台。
* `web/src/App.tsx` 在 `section === 'memory'` 时包裹 `.nexus-memory-mode` 并渲染 `MemoryWorkspace`；这是最适合做统一覆写的边界。
* `web/src/MemoryWorkspace.tsx` 现有功能表面包括 Dashboard、Memories Explorer/Editor、Git Review、Sync Center、Command Palette、移动导航与 fullscreen/focus 状态；脑暴方案必须保留这些行为入口。

## Brainstorm Notes

### Candidate Directions

1. **CSS-only Nexus adapter**
   * 在 `nexus.css` 末尾追加 `.nexus-memory-mode ...` 覆写，把 Memory 的背景、侧栏、按钮、面板、输入框、badge、Git/Sync 页面重绘成 Nexus 控制台语汇。
   * 优点：风险最低，不碰 React 行为；最符合“不改 API/功能”的边界。
   * 缺点：会保留 Memory 内部的局部命名和部分布局语义，长期维护仍有两套 class。

2. **Light structural alignment**
   * 保留 Memory 功能逻辑，但少量调整 `MemoryWorkspace.tsx` 容器 class/标题/状态条，让 Dashboard、Explorer、Editor、Git、Sync 更接近 Nexus 的 `section-heading`、`nexus-panel`、`nx-button`、`status-badge` 模式。
   * 优点：视觉和语义更统一，后续维护成本更低。
   * 缺点：需要更仔细验证编辑器、diff、移动导航、fullscreen 状态。

3. **Full component convergence**
   * 把 Memory 页面拆成 Nexus 原生页面组件，尽量复用 `nexus.css` 的通用组件类。
   * 优点：统一最彻底。
   * 缺点：当前任务过大，容易误伤 Memory 编辑/Git/Sync 行为，和 MVP 边界不匹配。

### Recommended MVP

采用 **Light structural alignment**：先用 `.nexus-memory-mode` 作为安全边界重绘外观，再做少量结构/类名补强。这样既能明显统一产品面，又不会把 Memory 工作台重写成另一个项目。

MVP 重点：

* 移除 Memory 的紫蓝渐变、玻璃态、orb/floating nav 主视觉。
* 让 Memory 使用 Nexus 的浅灰内容区、白色面板、深色固定侧栏、10-17px 圆角、低饱和状态色。
* Dashboard 从“MemoryDock hero”收敛成 Nexus workbench overview，避免第二套品牌。
* Explorer/Editor 保持高密度双栏工作台，桌面优先；移动端单栏但保留关键操作。
* Git Review 和 Sync Center 统一为 Nexus panel/card 语言，避免 review-studio 的独立视觉。
* 验证以 `/ui/#memory` 或兼容 query route 为主，覆盖桌面和移动宽度。

### Explicit Non-Goals

* 不新增后端契约或 API。
* 不改变 Markdown 渲染、Git discard、Sync pull/push、Basic Auth 配置逻辑。
* 不做完整设计系统抽象或新组件库。
* 不改 DockMini / MemoryDock / Skill Runtime 配置。
