# MemoryDock

MemoryDock 是一个独立的长期记忆服务，用于给多个 AgentDock / Agent 实例共享项目上下文、偏好、决策、runbook 和会话交接信息。

```text
AgentDock  = Agent 的工具运行层
MemoryDock = Agent 的长期记忆层
Git        = 记忆的可审计持久化与同步后端
```

## 当前功能

- Markdown + YAML-like frontmatter 存储。
- 记忆 CRUD、搜索、项目上下文打包 `pack`、inbox note 追加。
- 路径白名单与路径穿越保护：只允许写入当前规范目录。
- 非 `inbox/` 写入、移动、删除必须显式 `confirmed=true`。
- Git 同步：定时 pull、写入后 debounce commit/push、状态和 diff 查看。
- HTTP API，可由 AgentDock 转发为 `memory_*` MCP 工具。
- Web UI，支持 Basic Auth，账号密码可通过 UI 更新并持久化到配置文件。

## 端口与启动

默认只监听本地：

```text
MEMORYDOCK_HOST=127.0.0.1
MEMORYDOCK_PORT=18777
MEMORYDOCK_STORE_DIR=memory
```

本地运行：

```bash
go run ./cmd/memorydock
```

Docker Compose 运行优先使用项目脚本或自动选择 Compose 命令：

```bash
./scripts/doctor.sh
```

当前服务 health：

```bash
curl -s http://127.0.0.1:18777/health
```

## 认证配置

`/health` 不需要认证。`/v1/*` API 支持以下任一方式通过：

```http
Authorization: Bearer <MEMORYDOCK_AUTH_TOKEN>
```

或 UI Basic Auth 账号密码。

公网部署建议强制开启：

```text
MEMORYDOCK_REQUIRE_AUTH=true
MEMORYDOCK_AUTH_TOKEN=<strong-random-token>
MEMORYDOCK_USERNAME=<non-default-user>
MEMORYDOCK_PASSWORD=<strong-password>
```

如果 `MEMORYDOCK_REQUIRE_AUTH=true`，服务启动时会拒绝默认账号密码 `admin/memorydock`。UI 修改账号密码后，后端会把密码哈希持久化到：

```text
memory/.memorydock/access.json
```

也可以通过环境变量覆盖：

```text
MEMORYDOCK_ACCESS_FILE=/path/to/access.json
MEMORYDOCK_PASSWORD_HASH=<pbkdf2-sha256 hash>
```

配置接口不会返回密码或哈希，只返回 `enabled` 和 `username`。

## API 示例

写入 inbox note：

```bash
curl -s http://127.0.0.1:18777/v1/notes/append \
  -H 'content-type: application/json' \
  -H 'Authorization: Bearer <token>' \
  -d '{"content":"今天决定把 AgentDock 的长期记忆拆为 MemoryDock。"}'
```

搜索：

```bash
curl -s http://127.0.0.1:18777/v1/memories/search \
  -H 'content-type: application/json' \
  -H 'Authorization: Bearer <token>' \
  -d '{"query":"AgentDock","max_results":20}'
```

打包项目上下文：

```bash
curl -s http://127.0.0.1:18777/v1/memories/pack \
  -H 'content-type: application/json' \
  -H 'Authorization: Bearer <token>' \
  -d '{"project":"agentdock"}'
```

同步状态：

```bash
curl -s http://127.0.0.1:18777/v1/sync/status \
  -H 'Authorization: Bearer <token>'
```

## 当前记忆目录规范

只保留以下顶层结构，不再使用旧的 `shared/`、`journal/`：

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

目录语义：

- `profile.md`：用户长期偏好、协作规则、写入纪律。
- `devices/`：设备摘要，只记录稳定路径、端口、运行形态等事实。
- `projects/<project>/project.md`：项目总览和长期原则。
- `projects/<project>/environment.md`：项目真实运行环境。
- `projects/<project>/runbooks/`：项目专属 runbook。
- `ops/`：跨项目运维、反代、基础设施流程。
- `inbox/`：临时收件箱，后续应整理到长期目录或删除。

## 前端构建与嵌入式资源

Web UI 构建产物嵌入 Go 二进制：

```bash
cd web
npm run build
```

构建输出目录是：

```text
internal/httpx/web_dist
```

该目录作为 Go embed 静态资源提交到 Git。修改前端后必须重新执行 `npm run build` 并提交 `internal/httpx/web_dist`。`web/tsconfig.tsbuildinfo` 是 TypeScript 缓存文件，不应提交。

## 部署验收

每次修改部署或认证配置后执行：

```bash
./scripts/doctor.sh
go test ./...
go build ./...
cd web && npm run build
curl -s http://127.0.0.1:18777/health
```

`doctor.sh` 会检查 Compose 命令、认证策略、记忆目录 Git 状态、端口监听、本地 health、公网 health（可选 `MEMORYDOCK_PUBLIC_HEALTH_URL`）、嵌入式前端资源和构建缓存追踪状态。

## 安全策略

- 写入 `inbox/` 之外必须 `confirmed=true`。
- 移动、删除必须 `confirmed=true`。
- 路径穿越、绝对路径、隐藏目录、`.git` 路径会被拒绝。
- 默认不自动同步；生产使用时显式设置 `MEMORYDOCK_AUTO_SYNC=true`。
- 不建议把 token、password、SSH 私钥路径、真实本机绝对路径等敏感信息写入长期记忆。
