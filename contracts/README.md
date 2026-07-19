# NexusDock 当前公共契约

`contracts/` describes the current NexusDock HTTP API.

NexusDock owns Recall access, backup status, administrator sessions, and system status. AgentDock Runtime owns Task, Skill, and dynamic MCP lifecycles; NexusDock exposes these only through explicit node-scoped Runtime views/actions. Workflow templates are a Nexus-global registry.

## 当前内容

- `openapi/nexus.yaml`：当前 Nexus HTTP API。
- `jsonschema/*.json`：当前 HTTP DTO 的独立 JSON Schema。
- `error-codes.json`：稳定错误码目录。
- `generated/nexuscontracts/`：由生成器产生的 Go DTO 与客户端。

Nexus 不维护独立 Task、Run、Context Pack、Skill Catalog、Evolution、Worker 或 SSE 事件总线契约。Skill 包格式和任务状态机属于 AgentDock Runtime；Workflow 模板属于 Nexus 全局契约，不挂在单个 Runtime 节点下。

## 验证

```bash
python3 scripts/generate-contracts.py
python3 scripts/check-contracts.py
go test ./generated/nexuscontracts
```

生成文件不得手改。新增接口前必须确认它直接服务个人控制台、Recall、备份或 Runtime facade 的真实工作流。

契约检查会把 `internal/httpx` 的公开路由和查询参数与 OpenAPI 双向比对，并要求每个 OpenAPI `operationId` 都生成一个 Go Client 方法。独立 JSON Schema 只携带自身可达的 `$defs`；缺失、无法解析或未使用的引用都会让 CI 失败。公开错误码同样必须出现在 `error-codes.json`，Runtime 上游私有错误码只能通过 `upstream_code` 透传，不能进入 Nexus 稳定错误码空间。
