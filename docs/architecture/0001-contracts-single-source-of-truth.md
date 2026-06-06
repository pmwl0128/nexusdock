# ADR-0001：公共契约唯一权威来源

- 状态：Accepted
- 日期：2026-06-05
- Owner：T0

## 背景

AgentDock Nexus 与 AgentDock 分属不同代码仓库，同时由多个线程并行开发。若各模块手写 DTO，会出现字段含义漂移、枚举不一致、前后端重复定义和节点兼容性不可验证等问题。

## 决策

1. `agentdock-nexus/contracts/` 是公共协议唯一权威来源。
2. REST API 使用 OpenAPI 3.1，Skill 包使用 JSON Schema Draft 2020-12，事件使用独立 JSON Schema。
3. Go DTO 与客户端只能由 `scripts/generate-contracts.py` 生成。
4. `internal/api/dto/` 不定义业务字段，只提供边界说明；业务代码直接消费生成包。
5. 前端、AgentDock 节点和测试不得复制 DTO。
6. 跨模块接口变更先改契约，再重新生成并运行兼容性检查。

## 后果

- 契约评审先于实现合并。
- 生成结果漂移会使 CI 失败。
- 契约仓库重命名不影响 Schema ID；Go module 路径由仓库集成线程统一调整。

