# ADR-0001：当前生产契约唯一权威来源

- 状态：Accepted
- 日期：2026-06-05
- 更新：2026-06-14

## 决策

1. `contracts/` 只描述 `cmd/memorydock` 当前真实生产 HTTP 能力。
2. REST API 使用 OpenAPI 3.1，DTO 使用 JSON Schema Draft 2020-12。
3. Go DTO 与设备客户端只能由 `scripts/generate-contracts.py` 生成。
4. `internal/api/dto/` 不重复定义传输字段。
5. 前端、AgentDock 节点和测试不得复制或重新发明公共 DTO。
6. 契约检查必须阻止已退出的 Task、Run、Context Pack、Skill Catalog、Evolution 和 SSE 事件总线重新进入产品。
7. AgentDock Skill Manifest 属于 Runtime 仓库和 Skill 包，不由 Nexus 契约维护。

## 后果

- 契约变更必须与真实路由、前端和节点消费者同时验证。
- 生成结果漂移或产品边界回退会使检查失败。
- 历史协议由 Git 历史保留，不继续放在当前契约目录参与生成。
