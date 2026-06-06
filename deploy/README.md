# AgentDock Nexus 部署、迁移与回退

本目录记录 AgentDock Nexus 的构建、验收、Memory 迁移与回退流程。

## 前端构建

```bash
cd web
npm ci
npm run build
```

构建产物写入 `internal/httpx/web_dist/`，由 Go 服务内嵌发布。

## 全量验收

```bash
go test ./...
go vet ./...
go build ./...
cd web && npm run build
```

测试覆盖：

- `/health` 与嵌入式 `/ui/`。
- 旧 Memory API 读写兼容。
- Home / Inbox / Devices / Memory / Skills / Runs / Settings 前端入口。
- 移动端抽屉、遮罩和单栏布局产物。
- Path Traversal、隐藏路径、敏感信息泄漏扫描、Symlink Escape 测试夹具。
- 旧 MemoryDock 数据原地读取、搜索和内容摘要不变。

## MemoryDock 迁移

先停止写入，再创建只读备份：

```bash
python3 deploy/memory_migration.py backup /path/to/memory /path/to/backups
python3 deploy/memory_migration.py verify /path/to/backups/memorydock-YYYYmmddTHHMMSSZ
```

Nexus 使用原路径启动后必须验证：

```bash
curl -fsS http://127.0.0.1:18777/health
curl -fsS http://127.0.0.1:18777/v1/memories -H "Authorization: Bearer $MEMORYDOCK_AUTH_TOKEN"
```

确认目录、Search、Bootstrap、Diff 和时间线后，再切换公网 UI。

## 回退

1. 停止 Nexus/MemoryDock 服务。
2. 保留故障现场的独立备份。
3. 校验迁移前备份。
4. 显式确认恢复：

```bash
python3 deploy/memory_migration.py restore \
  /path/to/backups/memorydock-YYYYmmddTHHMMSSZ \
  /path/to/memory-restored \
  --confirmed
```

目标目录非空时默认拒绝。只有在再次确认服务已停止、目标数据已另行备份后才使用 `--replace`。

## 生产完整闭环

正式发布前必须使用真实 DockAir、DockMini、DockVPS 演示：

1. 导入 Skill。
2. 安装到 DockAir 并触发失败。
3. Agent 修复并上报 Observation。
4. 生成 Proposal 与 Review Task。
5. Canary 到 DockMini 并验证。
6. 发布 Stable，同步到 DockVPS。
7. 验证 Secret 脱敏、离线 Outbox、Nexus 重启恢复和回退。

前端无法连接对应 API 时必须明确显示 Compatibility mode，不伪造实时数据。
