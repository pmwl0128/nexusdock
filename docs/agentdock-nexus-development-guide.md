# AgentDock Nexus 项目整体开发文档

## 1. 项目定位

AgentDock Nexus 是 AgentDock 体系的统一控制面。开发时先区分三个边界：

```text
AgentDock       = 节点侧工具运行、Skill Runtime、设备心跳、离线命令租约与执行
AgentDock Nexus = 控制面、Memory、Devices、Skills、Runs、Tasks、Evolution 与 Web UI
Git / Memory    = Memory Markdown 数据的可审计持久化与同步后端
```

Nexus 负责管理和编排，不直接执行节点命令；AgentDock 节点负责执行并回传状态、进度、结果和证据。Web 前端只调用同源 Nexus API，不绕过后端访问 SQLite、Memory 文件或节点本地状态。

## 2. 本地路径与运行入口

当前生产形态把 Nexus 源码与 Memory 数据拆开：

```text
项目源码目录：/Volumes/KIOXIA/Docker/agentdock-nexus/source
Docker Compose 目录：/Volumes/KIOXIA/Docker/memorydock
Memory 数据目录：/Volumes/KIOXIA/Docker/memorydock/memory
本地服务：http://127.0.0.1:18777
本地 health：http://127.0.0.1:18777/health
本地 UI：http://127.0.0.1:18777/ui/
```

修改源码后，仍从 Compose 目录构建和重启容器：

```bash
cd /Volumes/KIOXIA/Docker/memorydock
docker compose up -d --build memorydock
curl -fsS http://127.0.0.1:18777/health
```

不要把 Memory 数据目录移动进源码仓库。部署、备份、回退都应把源码、运行配置和 Memory 数据视为不同资产。

## 3. 仓库结构

```text
cmd/
  memorydock/       # 兼容旧 MemoryDock 的 Memory/API/Web 服务入口
  nexus-server/     # Nexus 控制面服务入口
  nexus-worker/     # 后台任务 Worker
contracts/          # OpenAPI、JSON Schema、事件 Schema、错误码和兼容性签名
generated/          # 由契约生成的 Go 类型
internal/
  api/              # 公共 DTO 引用与中间件
  core/             # SQLite、配置、事件总线、迁移基础设施
  devices/          # 设备注册、审批、心跳、凭据、状态
  commands/         # 设备命令生命周期、租约、进度、结果
  memory/ syncer/   # Markdown Memory 存储、搜索、Git Sync
  skills/           # Skill 导入、Catalog、Provenance、导出
  evolution/        # Observation、Candidate、Proposal、Review 流程
  tasks/ runs/      # Agent Inbox、Run Registry、Evidence
  httpx/            # HTTP API、兼容入口、嵌入式 Web 资源
web/                # React/Vite 前端源码
migrations/         # Nexus SQLite migrations
deploy/             # 迁移、备份、回退和部署验收脚本/说明
docs/               # 架构决策和专项开发文档
```

模块间不要重复定义公共 DTO。跨模块、跨仓库、跨进程的字段和枚举必须回到 `contracts/` 维护。

## 4. 契约优先开发

`contracts/` 是 Nexus Server、Web、AgentDock 节点和外部集成的唯一公共协议来源。

涉及以下内容时，先改契约再改实现：

- REST API、SSE 入口、请求响应 DTO。
- 设备状态、命令状态、风险等级、错误码等公共枚举。
- 事件信封、事件 payload、JSON Schema。
- AgentDock 节点也要消费的生成类型。

标准流程：

```bash
python3 scripts/generate-contracts.py
python3 scripts/check-contracts.py
go test ./generated/nexuscontracts ./internal/api/dto
```

生成文件不得手工修改。若 AgentDock 仓库消费了生成类型，必须同步生成后的权威版本，并验证两边的契约文件摘要一致。

## 5. 后端开发流程

后端实现按 service / repository / HTTP 边界推进：

- Service 层负责领域规则、状态机、鉴权后的业务决策和审计事件。
- Repository 层负责 SQLite 或文件存储细节，不能向 HTTP 层泄漏存储实现。
- `internal/httpx` 只负责路由、请求解析、响应编码、兼容入口和嵌入式 UI。
- Run、Audit、Event、Evidence 必须随写操作同步设计，不能事后补日志。

关键模块边界：

- Devices：注册 Token、设备审批、撤销、凭据轮换、心跳状态。
- Commands：结构化命令创建、租约、开始、续租、进度、结果和终态。
- Memory：Markdown 读写、搜索、Context Pack、冲突、提案、Git Sync。
- Skills：导入校验、Catalog、安装发布状态、Provenance、导出。
- Tasks：Agent Inbox 聚合 needs_agent、needs_user、review、automatic 等任务。
- Runs：统一记录执行步骤、证据、失败层级和验证结果。

禁止在 Web 或 HTTP 层绕过 service 直接改 repository。高风险动作要有后端最终校验，前端禁用按钮只能作为体验辅助。

## 6. 前端开发流程

前端位于 `web/`，使用 React、Vite 和 lucide-react。生产构建产物写入 `internal/httpx/web_dist/`，由 Go 服务内嵌发布。

```bash
cd web
npm ci
npm run build
```

前端开发要求：

- API 调用统一走 `web/src/api/client.ts` 或同等封装，保持同源请求、超时、错误对象和取消语义一致。
- 页面必须区分 loading、empty、error、unauthorized、compatibility mode 和 live data。
- 真实 API 已存在时，不允许静默吞错后显示“暂无数据”。
- UI 不保存管理凭据、设备注册 Token、设备凭据或其他敏感材料到 localStorage、URL、console、Toast 或埋点。
- 设备命令只能使用结构化表单，不提供 Shell 输入框。
- 新页面需要桌面和移动端验收；长 ID、SHA、远端路径、错误消息必须可换行，不允许撑出横向滚动。

本轮已补齐：

- 顶部全局搜索已接入 `/v1/devices` 和 `/v1/schedules`，保留导航入口搜索作为兜底。
- Home 概览可在 `/api/v1/nexus/overview` 缺失时从真实 devices/schedules 派生设备异常和最近失败。
- Basic Auth URL 凭据会被前端清理，API client 拒绝跨源请求并区分鉴权失败、真实错误和兼容缺口。
- 新增计划任务页，展示真实 `/v1/schedules` 的状态、归档、SHA、远端路径和历史。

仍需推进：

- Inbox、Skills、Runs 等页面还依赖后端继续补齐真实 API；真实 API 存在后不得继续显示粗略 Compatibility mode。
- 前端测试仍偏存在性检查，需要补真实交互、真实 API 数据和移动断点测试。
- `MemoryWorkspace.tsx`、`DevicesPage.tsx` 和大体量 CSS 需要按页面、弹窗、表单、hooks、样式模块拆分。

## 7. 测试与验收

基础验证：

```bash
go test ./...
go vet ./...
go build ./...
python3 scripts/check-contracts.py
cd web && npm run build
```

部署后验证：

```bash
curl -fsS http://127.0.0.1:18777/health
curl -I http://127.0.0.1:18777/ui/
```

真实数据验收至少覆盖：

- `/v1/devices` 能返回真实设备，Web Devices 页面能展示同一台设备。
- `/v1/schedules` 能返回真实计划任务，Web 计划任务页能展示状态、归档和历史。
- 设备注册、审批、撤销、命令创建、命令历史、命令结果闭环。
- 旧 Memory API、Memory 工作区、搜索、Diff、Git 状态兼容。
- 移动端菜单、弹窗、表单、长文本和命令详情无横向溢出。

前端验收不要只检查 bundle 中是否包含文字；需要通过浏览器或 E2E 实际点击、读取 API 数据、验证错误态和移动断点。

## 8. 部署与回退

生产部署从 Compose 目录操作：

```bash
cd /Volumes/KIOXIA/Docker/memorydock
docker compose up -d --build memorydock
docker compose ps
curl -fsS http://127.0.0.1:18777/health
```

部署原则：

- 源码目录是 `/Volumes/KIOXIA/Docker/agentdock-nexus/source`。
- Memory 数据目录是 `/Volumes/KIOXIA/Docker/memorydock/memory`，部署不应迁移或重建该目录。
- 修改数据库 migration、Memory 数据结构或 Git Sync 行为前，先创建备份并校验。
- 回退前先停止服务、保留故障现场、校验备份，再显式恢复。
- 公开 UI、FRP、Caddy、Cloudflare 等入口只记录路径和流程，不记录真实密钥。

回退和迁移脚本以 `deploy/README.md` 为准。任何破坏性恢复都必须有明确确认和独立备份。

## 9. 安全规则

- 不把真实 Token、密码、私钥、Cookie、设备凭据、云盘凭据、FRP 密钥或 Cloudflare 凭据写入文档、日志、导出包或前端状态。
- 不绕过 Nexus API 直接修改 SQLite、Memory 文件或 AgentDock 节点状态。
- 不在前端或后端提供任意 Shell 命令输入；设备命令必须是公共契约定义的结构化命令。
- 写入、删除、移动、撤销、重启、发布、回退等高风险操作必须有后端校验、审计记录和清晰 UI 确认。
- 导入 Skill 必须经过 manifest、digest、路径、安全扫描和敏感信息检查。
- 错误响应和 Evidence 必须脱敏，不能泄漏真实本机路径、凭据或私有环境内容。

## 10. 开发前检查清单

开始实现前确认：

- 这次变更是否影响公共契约；若影响，先改 `contracts/`。
- 是否需要 migration、备份和回退步骤。
- 是否需要同步 AgentDock 消费端生成类型。
- 是否会触碰已有未提交改动；若无关，不要重排或覆盖。
- 是否需要前端真实数据验收，而不仅是构建通过。
- 是否新增或暴露敏感字段；若有，先设计脱敏和审计。

完成后至少说明：

- 改了哪些模块和入口。
- 跑了哪些测试或为什么未跑。
- 是否影响部署、数据、契约或公开接口。
- 是否还有前端交互、鉴权态、错误态或移动端风险。
