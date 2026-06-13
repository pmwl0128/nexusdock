# AgentDock Nexus 部署

生产只部署 `cmd/memorydock`，不启动独立 Nexus Server 或 Worker。

## 构建

运行 `make build`。该目标构建 Web、执行 Go 测试与静态检查、验证契约，并生成唯一生产二进制。

## 数据保护

持久化 `/memory`、`/memory/.nexus/control-plane.db` 和 `/memory/.nexus/artifacts`。部署前停止写入并备份 SQLite 主文件、WAL 和 SHM。

## Compose

```bash
docker compose build memorydock
docker compose up -d memorydock
```

## 验收

验证 `/health`、认证后的 `/v1/system/status`、`/v1/devices`、`/v1/artifacts`、`/v1/artifact-fetches`、`/v1/schedules`，并执行 SQLite `quick_check`、`integrity_check`、`foreign_key_check`。

回退时恢复上一个已验证镜像和部署前数据库快照；数据库异常时禁止反复重启容器。
