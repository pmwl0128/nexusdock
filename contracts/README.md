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
