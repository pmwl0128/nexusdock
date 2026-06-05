# AgentDock Nexus 公共契约

`contracts/` 是 Nexus Server、Web、AgentDock 节点和外部集成的唯一公共协议来源。

## 内容

- `openapi/nexus.yaml`：REST API、SSE 入口及所有公共 DTO。
- `jsonschema/agentdock-skill-v1.json`：`agentdock.yaml` V1。
- `jsonschema/*.json`：独立领域 DTO Schema，供事件和非 HTTP 消费者复用。
- `events/*.json`：事件信封与 10 个冻结事件 Schema。
- `error-codes.json`：稳定错误码目录。
- `compatibility/v1-baseline.json`：冻结兼容性签名。

## 命令

```bash
python3 scripts/generate-contracts.py
python3 scripts/check-contracts.py
go test ./generated/nexuscontracts ./internal/api/dto
```

生成文件不得手改。契约变更必须同时更新说明、重新生成并通过兼容性检查。

