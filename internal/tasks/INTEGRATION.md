# T8 Agent Inbox / Tasks 集成说明

## 所有权

T8 只拥有：

- `internal/tasks/`
- `internal/contextpack/`
- `tests/tasks/`

本实现不修改 `contracts/`、`migrations/`、Memory、Device、Skill、Run、Evolution 或 Web 模块。

## 公共契约映射

`Task`、`Link`、`Completion`、`Actor` 与 T0 冻结的 JSON 字段和枚举保持一致。T0 合并后，传输层应增加 generated DTO 与领域模型的显式转换，不允许前端或其他模块复制 Task DTO。

事件：

- `task.created`
- `task.updated`

错误码：

- `TASK_VALIDATION_ERROR`
- `TASK_NOT_FOUND`
- `TASK_FORBIDDEN`
- `TASK_VERSION_CONFLICT`
- `TASK_INVALID_TRANSITION`
- `TASK_ALREADY_CLAIMED`
- `TASK_VERIFICATION_REQUIRED`
- `TASK_REPOSITORY_ERROR`
- `TASK_AUDIT_ERROR`

## T1 SQLite migration 草案

T1 是 `migrations/**` 唯一 Owner。建议由 T1 最终落地以下表和约束：

```sql
CREATE TABLE tasks (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  category TEXT NOT NULL,
  source_type TEXT NOT NULL,
  source_id TEXT NOT NULL,
  object_id TEXT NOT NULL,
  dedup_key TEXT NOT NULL UNIQUE,
  priority TEXT NOT NULL,
  assigned_actor_type TEXT,
  assigned_actor_id TEXT,
  assigned_actor_display_name TEXT,
  completion_criteria_json TEXT NOT NULL,
  risk_constraints_json TEXT NOT NULL,
  completion_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  version INTEGER NOT NULL CHECK (version >= 1)
);

CREATE INDEX tasks_inbox_idx ON tasks(status, priority, created_at DESC);
CREATE INDEX tasks_source_idx ON tasks(source_type, source_id, category, object_id);

CREATE TABLE task_links (
  task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  object_id TEXT NOT NULL,
  relation TEXT NOT NULL,
  PRIMARY KEY(task_id, type, object_id, relation)
);

CREATE INDEX task_links_object_idx ON task_links(type, object_id);

CREATE TABLE task_activities (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  actor_type TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  actor_display_name TEXT,
  action TEXT NOT NULL,
  from_status TEXT,
  to_status TEXT,
  reason TEXT,
  metadata_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX task_activities_task_idx ON task_activities(task_id, created_at);

CREATE TABLE task_idempotency (
  scope TEXT NOT NULL,
  key TEXT NOT NULL,
  task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  result_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY(scope, key)
);
```

SQL Repository 的 `CreateOrGet` 必须在单个事务中依赖 `dedup_key UNIQUE` 完成去重；`Update` 必须使用：

```sql
UPDATE tasks
SET ..., version = version + 1
WHERE id = ? AND version = ?;
```

影响行数为 0 时返回 `TASK_VERSION_CONFLICT`。Task 更新、Activity、Audit 和 Outbox Event 应处于同一事务，避免业务状态已写入但审计或事件丢失。

## 跨线程接口

Context Pack 只通过窄接口读取外部模块：

- T2：`MemoryProvider`
- T3：`DeviceProvider`
- T5：`SkillProvider`
- T1：`RunProvider`
- Auth：`AccessChecker`

外部对象使用 `json.RawMessage` 作为临时边界，避免 T8 复制其他模块字段。T0 generated DTO 合并后，由组合根提供类型安全 Adapter。

## MCP 注册

工具名：`nexus_task`

冻结 action：

- `list`
- `inspect`
- `claim`
- `context`
- `update`
- `complete`
- `cancel`

领域层额外支持：`progress`、`block`、`await_user`、`await_agent`，用于完整 Agent 操作闭环。

## 验收命令

```bash
gofmt -w internal/tasks internal/contextpack tests/tasks
go test ./...
go test -race ./internal/tasks ./internal/contextpack ./tests/tasks
go vet ./...
go build ./...
```

## 回退

T8 代码是新增模块，尚未挂入现有 MemoryDock HTTP 启动路径。回退时撤销 T8 commit 即可，不影响现有 Memory 数据、Web UI 或运行服务。
