# NexusDock 部署

生产服务是 NexusDock，主入口为 `cmd/nexusdock`。

## 构建

运行 `make build`。该目标构建 Web、执行 Go 测试与静态检查、验证契约，并生成生产二进制。

## 数据保护

Persist:

- `NEXUS_DATA_DIR/nexus.db`, WAL, SHM
- `NEXUS_DATA_DIR/backups`
- `NEXUS_DATA_DIR/secrets/agentdock-nodes.key`
- `RECALL_REPO_DIR` and its Git remote credentials

Nexus 系统状态不得写入 Recall 仓库下的 `.nexus` 目录；系统状态只写入 `NEXUS_DATA_DIR`。

## Compose

```bash
docker compose build nexusdock
docker compose up -d nexusdock
```

生产镜像名使用 `nexusdock:local`。

Compose 应分别挂载 Nexus 数据目录和 Recall 仓库目录。

## 验收

AgentDock 节点通过 Nexus 设置页或 `/v1/runtime/nodes` API 写入数据库。部署配置不再接受单节点 Endpoint 或 Token 环境变量，也不会自动迁移旧配置。

验证 `/health`、认证后的 `/v1/system/status`、`/v1/backup/status`、`/v1/runtime/nodes`、`/v1/runtime/nodes/{nodeID}/tasks`、`/v1/runtime/nodes/{nodeID}/skills`、`/v1/runtime/nodes/{nodeID}/mcp`、`/v1/workflow-templates`，并执行 SQLite `quick_check`。

回退时恢复上一个已验证镜像和部署前数据库快照；数据库异常时禁止反复重启容器。不要让两个 Nexus 实例同时写同一个 SQLite 文件。
