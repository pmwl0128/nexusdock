# Directory Structure

AgentDock Nexus 是单一 Go 服务加 React/Vite 前端。生产入口只有 `cmd/memorydock`。

## 当前结构

```text
cmd/memorydock/       # 唯一生产入口与管理 CLI
contracts/            # 当前生产 OpenAPI、JSON Schema、错误码
generated/            # 契约生成的 Go DTO 与客户端
internal/
  artifacts/          # 加密 Artifact Relay / Fetch
  audit/ auth/         # 管理员会话、设备认证、必要审计
  commands/ devices/  # 设备注册、心跳、策略和结构化命令
  config/ core/        # 配置、SQLite、迁移和共享错误
  httpx/               # HTTP 路由与嵌入式 Web UI
  memory/ syncer/      # Markdown 记忆与 Git 同步
migrations/            # SQLite 迁移；历史表可保留但产品代码不再使用
scripts/               # 契约生成、检查和诊断
web/                   # 五入口个人控制台
```

## 边界

- HTTP 路由统一注册在 `internal/httpx`，领域包不依赖 HTTP。
- 设备协议只允许结构化命令，不提供任意 Shell。
- Skill 安装、升级、回滚和运行属于各设备 AgentDock Runtime；Nexus 只展示上报状态并可承载兼容命令协议。
- 不新增独立 Task、Run、Context Pack、Skill Catalog、Evolution、Worker 或第二个 Server。
- 生成文件只能由 `scripts/generate-contracts.py` 更新。
- 数据库变化使用编号迁移；历史表不等于当前产品能力。
