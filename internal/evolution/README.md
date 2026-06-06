# Skill Evolution Engine

Skill Evolution Engine 消费 Run、Run Step、Evidence、Observation、User Correction 和 Upstream Update，生成 Candidate、Proposal 与事件；它不直接修改 Stable Skill、不执行发布、不创建 Task，也不直接调用模型。

## 处理链路

1. `TriggerEngine` 将运行结果识别为冻结契约中的 Trigger。
2. `ErrorNormalizer` 清除时间、UUID、动态数字和设备私有路径，生成稳定 signature。
3. `Aggregator` 按 `skill_id + signature` 聚合 Observation。
4. `Scorer` 使用固定权重、运行次数、设备数和证据数计算可解释分数。
5. `ProposalGenerator` 生成问题、证据、作用域、建议文件、风险、测试和预期收益。
6. `Service` 持久化结果并发布 `evolution.candidate.created` 与 `evolution.proposal.review_ready`。

## 升级规则

- 单次瞬时网络错误：最高 25 分，保持 `observed`。
- `false_success`：基础 90 分，单次即可进入 Proposal。
- `security_violation`：基础 100 分，单次即可进入 Proposal。
- 同类错误跨 Run、跨 Device 会增加分数与置信度。
- 含设备私有路径的 Observation 强制进入 `device` scope，公开 DTO 中仅保留 `<path>`。

## 集成边界

- Core 层提供 `Repository`、`EventPublisher`、migration、事务、Audit 和 EventBus。
- AgentDock Skill Runtime 将运行 DTO 映射为 `RunInput`，Evolution 不依赖 Runtime 内部实现。
- Task 模块订阅 `evolution.proposal.review_ready` 创建 Review Task。
- Skill 模块在 Proposal 审批后生成或更新 Skill Package。
- 对外结构必须与公共契约中的 Observation、EvolutionCandidate、EvolutionProposal 保持一致。

## 错误码与审计点

- `EVOLUTION_VALIDATION_ERROR`
- `EVOLUTION_INVALID_TRANSITION`
- `EVOLUTION_NOT_ELIGIBLE`
- `EVOLUTION_REPOSITORY_ERROR`

必须审计的写操作包括：Observation 入库、Candidate 创建或更新、Proposal 创建、状态迁移和事件发布。

## 验收

```bash
go test ./internal/evolution ./tests/evolution
go test ./...
go vet ./...
go build ./...
go test -race ./internal/evolution ./tests/evolution
```

## 回退

Evolution 模块本身不直接修改 Stable Skill。回退时停止相关事件消费者和服务接线；已生成的 Candidate/Proposal 应保留为审计记录，或按状态标记为 `deferred`，禁止物理静默删除。
