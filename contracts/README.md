# AgentDock Nexus 当前公共契约

`contracts/` 只描述 Nexus 当前生产服务 `cmd/recalldock` 的真实 HTTP 能力，是 Web、AgentDock 节点和外部集成的协议来源。

## 当前内容

- `openapi/nexus.yaml`：总览所需系统状态、设备控制、记忆、Git 同步、加密文件中继、备份状态和账号会话 API。
- `jsonschema/*.json`：当前 HTTP DTO 的独立 JSON Schema。
- `error-codes.json`：稳定错误码目录。
- `generated/nexuscontracts/`：由生成器产生的 Go DTO 与设备客户端。

Nexus 不维护独立 Task、Run、Context Pack、Skill Catalog、Evolution、Worker 或 SSE 事件总线契约。Skill 包格式和生命周期属于 AgentDock Runtime，不属于 Nexus 公共契约。

## 验证

```bash
python3 scripts/generate-contracts.py
python3 scripts/check-contracts.py
go test ./generated/nexuscontracts
```

生成文件不得手改。新增接口前必须确认它直接服务个人多设备控制台的真实工作流。
