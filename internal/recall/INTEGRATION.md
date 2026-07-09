# T2 Recall 模块集成说明

## 所有权与边界

本目录由 T2 维护。Recall 模块继续以 Markdown 文件和 Git 作为内容权威源，不访问
Nexus 数据库，也不创建 Task、执行设备命令或修改 Skill。

`RecallService` 是 T3、T8 与 API 层唯一应依赖的业务端口。旧 `Store` API 仅作为内部实现保留，
用于兼容现有 Recall HTTP/MCP 行为。

## 对 T0 的契约需求

冻结 `RecallEntry`、`RecallConflict`、`RecallContextPack` 时，应与本目录模型保持以下
枚举和值一致：

- scope：`profile|global|project|device|agent|ops|inbox`
- status：`active|stale|conflicted|unverified|deprecated`
- confidence：`unknown|low|medium|high`
- conflict source：`device_snapshot|skill_run|user_edit|git_merge|agent_repair`

所有新增字段应先作为 optional 字段进入 V1 契约，避免破坏旧 Recall 数据。

## 对 T1 的持久化需求

Markdown 文件仍是 Recall 内容权威源。T1 如需数据库索引，只应持久化冲突与检索索引，
不得把 Recall 正文迁入 SQLite。建议为 `ConflictRepository` 提供 SQLite 实现，字段至少
覆盖 `RecallConflict` 全部成员，并对 conflict ID 建唯一约束。

T2 不直接提交 migration。

## 审计点

`WithMutationObserver` 暴露写操作审计端口。成功应用提案后记录：

- action：`recall.update.applied`
- object：Recall path
- source
- verification run ID
- occurred_at

提案未批准、摘要不匹配、版本冲突和写入失败都不会产生成功审计事件。

## 稳定错误语义

API 层可映射以下错误：

| 场景 | 建议错误码 |
|---|---|
| 非法 scope/status/confidence/path | `RECALL_VALIDATION_ERROR` |
| 提案未批准 | `RECALL_APPROVAL_REQUIRED` |
| 当前内容摘要变化 | `RECALL_VERSION_CONFLICT` |
| 提案内容摘要错误 | `RECALL_DIGEST_MISMATCH` |
| 临时日志写入长期作用域 | `RECALL_TRANSIENT_CONTENT_REJECTED` |
| 迁移目标非空或摘要不一致 | `RECALL_MIGRATION_FAILED` |

## 迁移与回退

`MigrateRepository` 支持：

1. dry-run 盘点；
2. 原路径只读接管校验；
3. 空目标目录的逐文件复制；
4. 迁移前后 SHA-256 摘要验证；
5. UTF-8 Recall 可读性验证。

回退时停止 Nexus，恢复上一个已验证服务指向原 Recall Repository 即可。复制迁移不会删除或修改
源仓库；原地接管校验也不会写文件。

## 验收命令

```bash
go test ./...
go vet ./...
go build ./...
RECALL_LIVE_STORE=/path/to/recall go test ./tests/recall \
  -run TestLiveLegacyRepositoryValidation -v
```
