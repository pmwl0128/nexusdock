# AgentDock Nexus 当前公共契约

`contracts/` describes the current AgentDock Nexus HTTP API.

Nexus owns devices, Recall access, artifact relay, administrator sessions, and system status. AgentDock Runtime owns Task, Skill, and Workflow lifecycles; Nexus exposes these only as Runtime API-backed views/actions.

## 当前内容

- `openapi/nexus.yaml`：当前 Nexus HTTP API。
- `jsonschema/*.json`：当前 HTTP DTO 的独立 JSON Schema。
- `error-codes.json`：稳定错误码目录。
- `generated/nexuscontracts/`：由生成器产生的 Go DTO 与设备客户端。

Nexus 不维护独立 Task、Run、Context Pack、Skill Catalog、Evolution、Worker 或 SSE 事件总线契约。Skill 包格式、任务状态机和 Workflow 生命周期属于 AgentDock Runtime，不属于 Nexus 公共契约。

## 验证

```bash
python3 scripts/generate-contracts.py
python3 scripts/check-contracts.py
go test ./generated/nexuscontracts
```

生成文件不得手改。新增接口前必须确认它直接服务个人控制台、Recall、设备、文件中继或 Runtime facade 的真实工作流。
