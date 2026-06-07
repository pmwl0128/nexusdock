# AgentDock Nexus 设备管理前端闭环开发文档

## 1. 文档信息

| 项目 | 内容 |
| --- | --- |
| 项目名称 | AgentDock Nexus Devices Frontend Closure |
| 所属仓库 | `agentdock-nexus` |
| 前端目录 | `web/` |
| 后端入口 | `cmd/nexus-server/` |
| 公共契约 | `contracts/openapi/nexus.yaml` |
| 文档状态 | 开发实施基线 |
| 基线日期 | 2026-06-07 |

## 2. 项目背景

AgentDock Nexus 已具备设备注册、审批、心跳、凭据、命令租约、命令执行结果和 SQLite 持久化等后端控制面能力，但当前 Web `DevicesPage` 仍以只读设备列表为主，尚未完成从前端发起注册、审批、命令下发、状态追踪和结果查看的完整交互闭环。

本项目用于补齐设备管理前端能力，使管理员能够仅通过 Nexus Web UI 完成以下流程：

1. 创建一次性设备注册 Token。
2. 查看待审批设备并执行审批。
3. 查看设备实时状态、能力、版本、架构、Skill 摘要和最近心跳。
4. 下发受控设备命令。
5. 追踪命令从排队到结束的完整状态。
6. 查看结构化命令结果和错误信息。
7. 在移动端完成同等核心操作。

## 3. 当前实现基线

### 3.1 已具备能力

- Nexus Server 已提供设备与命令控制面 HTTP API。
- 设备和命令数据已接入 SQLite Repository，不再依赖仅存活于进程内的临时状态。
- 设备注册 Token 为一次性凭据，明文只在创建时返回。
- 设备凭据只持久化摘要，不持久化明文 Token。
- 设备状态支持 `pending`、`online`、`degraded`、`offline`、`revoked`。
- 命令状态支持 `queued`、`leased`、`running`、`succeeded`、`failed`、`expired`、`cancelled`。
- 设备命令只允许公共契约中定义的结构化命令类型，不允许任意 Shell。
- 后端已有设备控制面、E2E、安全和迁移测试基础。
- Web 已具备 Home、Inbox、Devices、Memory、Skills、Runs、Settings 页面和移动端基础布局。

### 3.2 当前缺口

- `DevicesPage` 目前只读取设备列表并渲染设备卡片。
- 未提供注册 Token 创建对话框。
- 未提供设备审批、撤销等操作入口。
- 未提供设备详情和命令历史视图。
- 未提供命令创建表单。
- 未提供命令状态轮询和结果查看。
- 前端类型仍需要与公共 OpenAPI 契约完整对齐。
- 缺少覆盖真实 UI 操作的设备管理前端 E2E 测试。

## 4. 目标与非目标

### 4.1 项目目标

- 完成设备注册、审批、状态查看、命令下发和结果查看的 Web 闭环。
- 所有前端请求直接调用 Nexus 同源管理 API。
- 前端不保存 Nexus 管理凭据、设备 Token、密码或其他 Secret。
- 前端展示的数据与后端公共契约、SQLite 状态和设备心跳一致。
- 桌面端和移动端均可完成核心管理操作。

### 4.2 非目标

- 不在前端实现设备命令执行逻辑。
- 不允许用户输入或执行任意 Shell 命令。
- 不修改 AgentDock 节点侧命令租约协议。
- 不绕过 Nexus API 直接访问 SQLite。
- 不将设备注册 Token、设备 Token 或管理凭据写入浏览器持久化存储。
- 不在本项目中重构 Memory、Skills、Runs 等其他页面。

## 5. 系统边界与数据流

```text
管理员浏览器
    │
    │ 同源 HTTP / Session 或管理认证
    ▼
AgentDock Nexus Web + Nexus Server
    │
    ├── Device Service
    ├── Command Service
    ├── SQLite Repository
    ├── Audit / Event / Run Registry
    └── Memory 子系统与兼容入口
             
AgentDock 节点
    ├── enrollment
    ├── heartbeat
    ├── command lease
    ├── command start / renew / progress
    └── command result
```

前端只负责管理侧操作。设备注册、心跳、租约和结果回传由 AgentDock 节点使用设备凭据调用节点侧 API 完成。

Memory 是 Nexus 的一个子系统，不是 Devices 页面绕过 Nexus Server 的独立管理通道。UI 不应直接保存或拼接 MemoryDock 账号密码。

## 6. 功能范围

### 6.1 Devices 列表页

每台设备至少显示：

- 设备名称和设备 ID。
- 当前状态。
- 平台和架构。
- AgentDock 版本。
- 能力列表及启用状态，例如 Memory、Browser、Desktop。
- 已安装或激活 Skill 摘要。
- 最近心跳时间。
- 状态更新时间。

页面级操作：

- 刷新。
- 创建一次性注册 Token。
- 按状态筛选。
- 按名称、设备 ID、平台搜索。

设备级操作：

- 查看详情。
- 审批待审批设备。
- 下发命令。
- 查看命令历史。
- 撤销设备，作为高风险管理操作并要求二次确认。

### 6.2 创建注册 Token

创建表单字段：

- `created_by`：创建主体标识，默认取当前登录主体。
- `ttl_seconds`：有效期，范围 60 至 604800 秒。
- `allowed_command_types`：允许该设备接收的命令类型。
- `max_risk`：最大允许风险等级。

成功后：

- 仅在结果对话框中显示一次明文 Token。
- 同时显示过期时间。
- 提供显式复制按钮。
- 关闭对话框后不允许从前端状态、日志或浏览器存储中恢复 Token。
- 禁止在 URL、Toast、埋点和控制台日志中输出 Token。

### 6.3 设备审批

审批仅对 `pending` 设备开放。

交互要求：

1. 用户点击“批准设备”。
2. 对话框展示设备名称、平台、架构、公钥摘要和注册时间。
3. 用户确认后调用审批接口。
4. 成功后立即刷新设备详情和列表。
5. 页面不得直接把审批成功等同于 `online`；设备必须完成有效心跳后才显示 `online`。

### 6.4 设备状态

状态语义：

| 状态 | 含义 | UI 行为 |
| --- | --- | --- |
| `pending` | 已注册，待审批 | 显示审批按钮，不允许下发命令 |
| `online` | 已审批且心跳正常 | 允许按策略下发命令 |
| `degraded` | 超过 90 秒未收到心跳 | 显示告警，可查看详情，谨慎下发命令 |
| `offline` | 超过 180 秒未收到心跳 | 显示离线，不执行依赖实时连接的乐观提示 |
| `revoked` | 凭据已撤销 | 禁止下发命令和再次审批 |

前端不自行计算最终状态，只展示后端返回状态。前端可使用最近心跳时间增强说明，但不得覆盖后端状态机。

### 6.5 命令下发

允许的命令类型以公共契约为准：

- `health.check`
- `skill.install`
- `skill.run`
- `skill.rollback`
- `memory.sync`
- `service.inspect`
- `service.restart`
- `diagnostics.collect`
- `agentdock.reload`

创建命令时必须提交：

- 命令类型。
- 结构化 Payload。
- 风险等级。
- 幂等键。
- 到期时间或契约要求的 TTL。
- 最大尝试次数。
- 创建主体。

前端根据命令类型渲染白名单表单，不提供自由 Shell 输入框。

高风险命令，例如 `service.restart`，必须显示风险说明并要求二次确认。后端策略仍是最终授权边界，前端禁用按钮不能替代后端校验。

### 6.6 命令生命周期与结果

命令创建成功后，前端进入命令详情视图并追踪：

```text
queued → leased → running → succeeded
                         └→ failed
queued / leased ─────────→ expired
任意允许状态 ────────────→ cancelled
```

轮询策略：

- 命令刚创建后的前 30 秒：每 2 秒查询一次。
- 30 秒后仍未结束：每 5 秒查询一次。
- 页面不可见时降低到每 15 秒一次，恢复可见后立即刷新。
- 到达终态后停止轮询。
- 连续请求失败时使用指数退避，最大间隔 30 秒。

前端查询命令和结果使用：

```http
GET /v1/commands/{commandId}
```

`POST /v1/commands/{commandId}/result` 是设备回传结果的节点侧接口，不是前端查询接口。

结果视图至少显示：

- 命令 ID、设备 ID、命令类型和风险等级。
- 当前状态和状态时间线。
- 创建时间、到期时间、尝试次数。
- 结构化输出。
- 错误码和脱敏错误信息。
- 可关联的 Run、Audit 或 Evidence 标识。

## 7. API 设计基线

### 7.1 管理侧 API

| 功能 | 方法 | 路径 | 前端用途 |
| --- | --- | --- | --- |
| 设备列表 | GET | `/v1/devices` | 列表和刷新 |
| 设备详情 | GET | `/v1/devices/{deviceId}` | 详情抽屉或详情页 |
| 创建注册 Token | POST | `/v1/devices/enrollment-tokens` | 注册 Token 对话框 |
| 审批设备 | POST | `/v1/devices/{deviceId}/approve` | 审批待审批设备 |
| 撤销设备 | POST | `/v1/devices/{deviceId}/revoke` | 高风险撤销操作 |
| 创建命令 | POST | `/v1/devices/{deviceId}/commands` | 下发受控命令 |
| 设备命令列表 | GET | `/v1/devices/{deviceId}/commands` | 命令历史 |
| 命令详情 | GET | `/v1/commands/{commandId}` | 状态和结果追踪 |

### 7.2 节点侧 API

以下接口由 AgentDock 节点调用，Devices 前端不得直接模拟：

| 功能 | 方法 | 路径 |
| --- | --- | --- |
| 注册设备 | POST | `/v1/devices/enroll` |
| 上报心跳 | POST | `/v1/devices/{deviceId}/heartbeat` |
| 轮换设备 Token | POST | `/v1/devices/{deviceId}/token/rotate` |
| 租用命令 | POST | `/v1/devices/{deviceId}/commands/lease` |
| 开始命令 | POST | `/v1/commands/{commandId}/start` |
| 续租 | POST | `/v1/commands/{commandId}/renew` |
| 上报进度 | POST | `/v1/commands/{commandId}/progress` |
| 回传结果 | POST | `/v1/commands/{commandId}/result` |

## 8. 前端技术设计

### 8.1 目录建议

```text
web/src/
├── App.tsx
├── api/
│   ├── client.ts
│   ├── devices.ts
│   ├── commands.ts
│   └── types.ts
├── components/
│   ├── devices/
│   │   ├── DeviceCard.tsx
│   │   ├── DeviceDetailsDrawer.tsx
│   │   ├── EnrollmentTokenDialog.tsx
│   │   ├── ApproveDeviceDialog.tsx
│   │   ├── RevokeDeviceDialog.tsx
│   │   ├── CommandCreateDialog.tsx
│   │   ├── CommandStatusTimeline.tsx
│   │   └── CommandResultPanel.tsx
│   └── common/
├── hooks/
│   ├── useResource.ts
│   ├── useDevices.ts
│   └── useCommandPolling.ts
└── nexus.css
```

### 8.2 状态管理

- 保留轻量 React Hook 模式，不为单一页面引入新的全局状态框架。
- 服务端数据与对话框局部状态分离。
- mutation 成功后按资源粒度刷新，而不是整页重载。
- 所有异步操作必须具有 `idle`、`loading`、`success`、`error` 状态。
- 防止重复点击造成重复审批或重复命令创建。
- 命令创建必须生成稳定幂等键，网络重试时复用同一幂等键。

### 8.3 类型来源

- `contracts/openapi/nexus.yaml` 是接口和 DTO 的唯一事实来源。
- 前端类型不得根据示例响应手工猜测。
- 若暂未自动生成 TypeScript 类型，手写类型也必须逐字段对齐契约，并用契约测试防止漂移。
- 契约变更先更新 OpenAPI，再重新生成或同步前端类型。

### 8.4 错误处理

前端需要区分：

- 认证失败：提示重新登录或刷新 Session。
- 授权失败：展示策略拒绝，不重复自动重试。
- 资源冲突：刷新设备或命令状态后提示用户。
- 设备离线：命令可排队但不得伪造“已执行”。
- 参数错误：定位到具体表单字段。
- 网络错误：保留用户输入并允许重试。
- 服务端错误：显示请求追踪标识，不显示内部堆栈和 Secret。

## 9. 安全要求

### 9.1 Secret 处理

- 注册 Token 明文只显示一次。
- 禁止写入 `localStorage`、`sessionStorage`、IndexedDB、URL、日志和错误上报。
- React 状态在对话框关闭后立即清空。
- 复制操作必须由用户显式触发。
- 所有响应渲染前执行 Secret 字段过滤和长度限制。

### 9.2 命令边界

- 不提供 Shell、脚本、路径或环境变量的任意执行入口。
- 命令类型必须来自契约枚举。
- Payload 必须使用按命令类型定义的结构化 Schema。
- 风险和设备策略由后端最终校验。
- 高风险命令必须二次确认并留下审计记录。

### 9.3 Web 安全

- 所有写操作使用同源认证和 CSRF 防护策略。
- 不使用 `dangerouslySetInnerHTML` 渲染设备输出。
- JSON 输出使用纯文本或安全结构化组件展示。
- 防止设备名称、标签和命令输出造成 XSS。
- 保持现有 Path Traversal、Symlink Escape、隐藏路径和 `.git` 访问防护测试。

## 10. 响应式与可访问性

### 10.1 桌面端

- 设备卡片使用多列网格。
- 详情使用右侧抽屉或独立详情区域。
- 命令时间线和结构化结果可并排展示。

### 10.2 移动端

- 设备卡片单列显示。
- 详情和操作使用全屏抽屉或底部操作面板。
- 操作按钮不得依赖鼠标悬停。
- Token 和命令输出支持横向滚动，但不撑破页面。
- 高风险确认按钮与取消按钮保持足够间距。

### 10.3 可访问性

- 对话框具备焦点锁定和 Escape 关闭。
- 状态不能只通过颜色表达，必须同时显示文本和图标。
- 所有图标按钮提供可读标签。
- 异步成功或失败结果使用可被辅助技术识别的状态提示。

## 11. 开发任务拆分

### T1：API 与类型层

- 补齐 Devices 和 Commands API Client。
- 对齐 OpenAPI DTO。
- 统一错误响应解析。
- 增加请求取消和超时。

### T2：设备列表与详情

- 完善设备卡片字段。
- 增加筛选和搜索。
- 增加设备详情抽屉。
- 增加状态说明和能力展示。

### T3：注册与审批

- 创建 Enrollment Token 对话框。
- 实现一次性 Token 展示和清理。
- 实现审批确认和状态刷新。
- 实现撤销确认和结果反馈。

### T4：命令下发

- 按命令类型实现结构化表单。
- 实现风险提示和二次确认。
- 实现幂等键生成和重试复用。
- 接入命令创建 API。

### T5：命令追踪

- 实现设备命令历史。
- 实现命令状态时间线。
- 实现轮询、退避和终态停止。
- 实现结果、错误和 Evidence 展示。

### T6：测试与发布

- 增加组件测试。
- 增加 API 契约测试。
- 增加前端 E2E 测试。
- 完成移动端回归。
- 构建并嵌入 `internal/httpx/web_dist/`。

## 12. 测试方案

### 12.1 单元与组件测试

- 状态 Badge 映射正确。
- 注册 Token 关闭后明文被清空。
- 命令表单只允许契约定义类型。
- 幂等键在重试中保持不变。
- 终态命令停止轮询。
- 网络失败后用户输入不丢失。

### 12.2 API 契约测试

- 请求字段与 OpenAPI 完全一致。
- 204 响应不尝试解析 JSON。
- 错误响应正确映射到 UI。
- `GET /v1/commands/{commandId}` 能返回可渲染的终态结果。
- 前端不调用节点侧租约、心跳和结果回传接口。

### 12.3 E2E 场景

1. 创建一次性注册 Token。
2. DockMini、DockAir、DockVPS 分别完成注册。
3. UI 显示三台 `pending` 设备。
4. 逐台审批。
5. 节点发送心跳后 UI 显示 `online`。
6. 分别下发 `health.check`。
7. UI 显示 `queued → leased → running → succeeded`。
8. 查看结构化健康检查结果。
9. 下发允许的 Skill 安装、运行和回退命令并查看结果。
10. 停止一台节点心跳，验证 `degraded` 和 `offline` 状态。
11. 重启 Nexus，验证设备、命令和结果仍可读取。
12. 验证页面和日志中无明文 Secret。

### 12.4 安全测试

- 注册 Token 不进入浏览器持久化存储。
- 设备输出中的 HTML 和脚本不会执行。
- 任意 `shell.exec` 命令被前后端共同拒绝。
- 高风险命令无确认时不能提交。
- Path Traversal 和 Symlink Escape 测试继续通过。

## 13. 构建与部署

### 13.1 构建

```bash
cd web
npm ci
npm run build
```

构建产物写入：

```text
internal/httpx/web_dist/
```

### 13.2 全量验证

```bash
go test ./...
go vet ./...
go build ./...
python3 scripts/check-contracts.py
cd web && npm ci && npm run build
```

### 13.3 部署后验证

- Nexus 健康检查成功。
- Web UI 可打开。
- 设备列表 API 可读取。
- 创建注册 Token 成功。
- 审批设备成功。
- 设备心跳后状态转为 `online`。
- `health.check` 命令完整闭环。
- Nexus 重启后设备和命令状态仍存在。
- 移动端布局可完成核心操作。

## 14. 验收标准

满足以下全部条件方可验收：

- 管理员可从 UI 创建一次性注册 Token。
- Token 明文仅显示一次且不会进入持久化存储或日志。
- 待审批设备可从 UI 审批。
- 审批后必须由真实心跳驱动状态变为 `online`。
- 页面正确显示 `pending`、`online`、`degraded`、`offline`、`revoked`。
- 页面显示设备平台、架构、AgentDock 版本、能力、Skill 摘要和最近心跳。
- 可从 UI 下发公共契约允许的结构化命令。
- 不存在任意 Shell 执行入口。
- 命令状态和结果可持续刷新并在终态停止轮询。
- 可查看设备命令历史和单条命令详情。
- Nexus 重启后设备、命令和结果不丢失。
- 桌面端和移动端核心流程均通过。
- `go test ./...`、`go vet ./...`、`go build ./...`、契约检查和前端构建全部通过。
- E2E、安全和迁移测试通过。

## 15. 回退策略

### 15.1 前端回退

- 保留上一个已验证的 `internal/httpx/web_dist/` 构建版本。
- 回退时仅替换嵌入式前端资源，不回滚 SQLite 数据。
- 回退后执行 UI、设备列表和健康检查验证。

### 15.2 后端回退

- 回退前备份 Nexus SQLite 和 Memory 数据目录。
- 使用兼容当前数据库 Schema 的上一版本二进制。
- 若涉及 migration，严格按照 `deploy/README.md` 的迁移与回退流程执行。
- 不通过删除数据库恢复服务。

### 15.3 设备侧影响

- 前端回退不改变设备凭据。
- 已排队命令继续由 Nexus 和 AgentDock 节点处理。
- 已完成命令结果保留在 SQLite 中。
- 回退期间禁止重复创建相同副作用命令，重试必须复用幂等键。

## 16. 交付物

- Devices 页面完整管理功能。
- 设备详情和命令历史视图。
- 注册、审批、撤销和命令对话框。
- 命令状态时间线和结果面板。
- OpenAPI 对齐的前端类型与 API Client。
- 组件测试、契约测试和 E2E 测试。
- 更新后的嵌入式 Web 构建产物。
- 部署验证记录和回退说明。

## 17. 参考文件

- `contracts/openapi/nexus.yaml`
- `internal/httpx/control_plane.go`
- `internal/devices/`
- `internal/commands/`
- `web/src/App.tsx`
- `web/src/api/client.ts`
- `web/src/api/types.ts`
- `tests/devices/control_plane_test.go`
- `tests/e2e/e2e_test.go`
- `tests/security/security_test.go`
- `deploy/README.md`

