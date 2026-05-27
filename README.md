# MemoryDock

MemoryDock 是一个独立的长期记忆服务，用于给多个 AgentDock / Agent 实例共享项目上下文、偏好、决策、runbook 和会话交接信息。

它的定位是：

```text
AgentDock  = Agent 的工具运行层
MemoryDock = Agent 的长期记忆层
Git        = 记忆的可审计持久化与同步后端
```

## MVP 功能

- Markdown + YAML-like frontmatter 存储。
- 记忆 CRUD。
- 文本和路径搜索。
- 项目上下文打包 `pack`。
- inbox note 追加。
- Git 自动同步：定时 pull，写入后 debounce commit/push。
- HTTP API，后续可由 AgentDock 转发为 `memory_*` MCP 工具。

## 启动

```bash
go run ./cmd/memorydock
```

默认配置：

```text
MEMORYDOCK_HOST=127.0.0.1
MEMORYDOCK_PORT=18777
MEMORYDOCK_STORE_DIR=memory
MEMORYDOCK_AUTO_SYNC=false
MEMORYDOCK_PULL_INTERVAL_SECONDS=120
MEMORYDOCK_PUSH_DEBOUNCE_SECONDS=10
```

如果设置 `MEMORYDOCK_AUTH_TOKEN`，所有 `/v1/*` API 都需要：

```http
Authorization: Bearer <token>
```

## API 示例

写入 inbox note：

```bash
curl -s http://127.0.0.1:18777/v1/notes/append \
  -H 'content-type: application/json' \
  -d '{"content":"今天决定把 AgentDock 的长期记忆拆为 MemoryDock。"}'
```

搜索：

```bash
curl -s http://127.0.0.1:18777/v1/memories/search \
  -H 'content-type: application/json' \
  -d '{"query":"AgentDock","max_results":20}'
```

打包项目上下文：

```bash
curl -s http://127.0.0.1:18777/v1/memories/pack \
  -H 'content-type: application/json' \
  -d '{"project":"agentdock"}'
```

同步状态：

```bash
curl -s http://127.0.0.1:18777/v1/sync/status
```

## 推荐目录

```text
memory/
  shared/
    profile.md
    projects/
      agentdock/
        overview.md
        conventions.md
        environment.md
        session-handoff.md
        decisions/
        runbooks/
  devices/
  inbox/
  journal/
```

## 安全策略

- 写入 `inbox/` 之外必须 `confirmed=true`。
- 默认不自动同步；生产使用时显式设置 `MEMORYDOCK_AUTO_SYNC=true`。
- 不建议把 token、password、SSH 私钥路径、真实本机绝对路径等敏感信息写入 shared 记忆。

