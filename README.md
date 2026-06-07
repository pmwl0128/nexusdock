# AgentDock Nexus

AgentDock Nexus 是 AgentDock 的统一控制面，用于集中管理多设备、长期记忆、Skill、运行记录、任务收件箱和 Skill Evolution。

仓库地址：

```text
https://github.com/uvwt/agentdock-nexus
```

```text
AgentDock       = 节点侧工具运行、Skill Runtime 与离线命令执行
AgentDock Nexus = 控制面、Memory、Devices、Skills、Runs、Tasks、Evolution 与 Web UI
Git             = Memory 的可审计持久化与同步后端
```

## 核心能力

- Nexus Core：SQLite、迁移、认证、审计、事件与 Run Registry。
- Devices：设备注册、心跳、能力上报、Token 轮换、撤销与命令生命周期。
- Memory：Markdown 存储、搜索、Context Pack、冲突、提案、审批与 Git Sync。
- Skills：Catalog、导入、扫描、Provenance、导出与版本化发布。
- Evolution：Observation 聚合、候选评分、Proposal、Review 与状态机。
- Agent Inbox：统一聚合设备异常、Memory 冲突、Skill 失败和 Evolution Proposal。
- Web UI：Home、Inbox、Devices、Memory、Skills、Runs、Settings，支持移动端。
- 公共契约：OpenAPI、JSON Schema、事件 Schema 与生成 Go 类型。

## 仓库结构

```text
cmd/
  memorydock/       # 兼容旧 MemoryDock 的 Memory/API/Web 服务入口
  nexus-server/     # Nexus 控制面服务
  nexus-worker/     # 后台任务 Worker
contracts/          # OpenAPI、JSON Schema、事件和错误码
internal/
  core/             # 数据库、配置、事件总线、迁移
  auth/ audit/ runs/
  devices/ commands/
  memory/ syncer/
  skills/ evolution/ tasks/
  httpx/            # HTTP API 与嵌入式 Web 资源
web/                # React/Vite 前端
migrations/         # Nexus SQLite migrations
deploy/             # 迁移、回退和部署验收文档
```

## 本地开发

要求：

- Go 1.26 或更高版本
- Node.js 18 或更高版本
- npm

后端验证：

```bash
go test ./...
go vet ./...
go build ./...
python3 scripts/check-contracts.py
```

前端构建：

```bash
cd web
npm ci
npm run build
```

前端产物写入并提交到：

```text
internal/httpx/web_dist/
```

## 启动服务

### Memory 兼容入口

现有 MemoryDock 部署可继续使用原环境变量与入口：

```bash
MEMORYDOCK_HOST=127.0.0.1 \
MEMORYDOCK_PORT=18777 \
MEMORYDOCK_STORE_DIR=memory \
go run ./cmd/memorydock
```

健康检查：

```bash
curl -fsS http://127.0.0.1:18777/health
```

Web UI：

```text
http://127.0.0.1:18777/ui/
```

### Nexus 控制面

```bash
go run ./cmd/nexus-server
```

Nexus Server 的数据库、监听地址和认证配置以 `internal/core/config.go` 及部署环境为准。生产部署前必须使用非默认凭据，并确认数据库与 Memory 数据已备份。

## Memory 兼容与迁移

AgentDock Nexus 保留旧 MemoryDock 的 Memory API、目录结构、Web 工作区和环境变量，以便原地升级。

标准 Memory 目录：

```text
memory/
  profile.md
  devices/
    <device>.md
  projects/
    <project>/
      project.md
      environment.md
      runbooks/
        <runbook>.md
  ops/
    <runbook-or-operation>.md
  inbox/
    <temporary-note>.md
```

迁移前创建备份并校验：

```bash
python3 deploy/memory_migration.py backup /path/to/memory /path/to/backups
python3 deploy/memory_migration.py verify /path/to/backups/memorydock-YYYYmmddTHHMMSSZ
```

完整迁移与回退流程见 [`deploy/README.md`](deploy/README.md)。

## 认证与安全

Memory 兼容入口继续支持 Bearer Token 与 UI Basic Auth：

```http
Authorization: Bearer <MEMORYDOCK_AUTH_TOKEN>
```

生产环境至少配置：

```text
MEMORYDOCK_REQUIRE_AUTH=true
MEMORYDOCK_AUTH_TOKEN=<strong-random-token>
MEMORYDOCK_USERNAME=<non-default-user>
MEMORYDOCK_PASSWORD=<strong-password>
```

安全规则：

- 写入 `inbox/` 之外必须显式确认。
- 移动和删除必须显式确认。
- 拒绝绝对路径、路径穿越、隐藏目录和 `.git` 路径。
- Skill 导入必须经过 Manifest、Digest、路径和敏感信息检查。
- 设备命令、Skill Run 和写操作必须留下状态、证据与审计记录。
- 不把 Token、密码、私钥或真实私有路径写入公开文档和导出包。

## 公共契约

`contracts/` 是 Nexus Server、Web、AgentDock 节点与外部集成的唯一公共协议来源。

```bash
python3 scripts/generate-contracts.py
python3 scripts/check-contracts.py
go test ./generated/nexuscontracts ./internal/api/dto
```

生成文件不得手工修改。契约变更必须重新生成并通过兼容性检查。

AgentDock 消费端的生成类型必须与本仓库保持一致：

```text
agentdock/generated/nexuscontracts/types.gen.go
```

## 开发文档

- [项目整体开发文档](docs/agentdock-nexus-development-guide.md)
- [设备管理前端闭环开发文档](docs/agentdock-nexus-devices-frontend-closure.md)

## 部署验收

每次部署或升级至少执行：

```bash
go test ./...
go vet ./...
go build ./...
python3 scripts/check-contracts.py
cd web && npm ci && npm run build
curl -fsS http://127.0.0.1:18777/health
```

生产验收还必须覆盖：

- 数据库 migration 与备份回退。
- 旧 Memory 数据无损读取。
- 设备注册、心跳、离线 Outbox 和命令状态闭环。
- Skill 导入、安装、执行、验证、Canary 与回退。
- Evolution Proposal 到 Agent Inbox Review 的闭环。
- Secret 脱敏、Audit Event 和 Run Evidence。

## 相关仓库

- AgentDock：<https://github.com/uvwt/agentdock>
- AgentDock Nexus：<https://github.com/uvwt/agentdock-nexus>
