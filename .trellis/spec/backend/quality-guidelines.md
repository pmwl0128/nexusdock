# Quality Guidelines

## 最终检查

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
python3 scripts/check-contracts.py
cd web && npm run build
```

## 必须遵守

- `contracts/` 只描述当前生产能力，生成器必须可重复执行。
- `scripts/check-contracts.py` 必须阻止已退出的产品路径和模型重新进入公共契约。
- 不手改 `generated/nexuscontracts/`、`contracts/openapi/` 或生成的 JSON Schema。
- 不在前端硬编码具体 Skill 或 Env 变量定义；使用设备 Runtime 上报状态。
- 人工界面不暴露 Skill 生命周期、任意 JSON 编排、风险等级、TTL 或重试等底层调度参数。
- 不加入任意 Shell、不绕过 Service 校验、不在日志、测试和文档中放入真实 Secret。
- 服务、仓库、迁移、认证和命令状态变化必须有对应测试。

## 评审问题

- 该功能是否直接服务个人多设备工作流？
- 是否重复了 AgentDock Runtime 或外部 Agent 的职责？
- 契约、真实路由、前端和部署是否一致？
- 最终 diff 是否包含历史架构残留或无关样式？
