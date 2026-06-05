# AgentDock Nexus 契约与架构治理

本目录记录跨线程、跨仓库必须共同遵守的架构决策。公共协议的唯一权威来源是 `contracts/`；`generated/` 与 `internal/api/dto/` 只允许由生成器产生或引用，不得手写与契约重复的 DTO。

## 决策索引

- [ADR-0001：公共契约唯一权威来源](0001-contracts-single-source-of-truth.md)
- [ADR-0002：兼容性与版本策略](0002-contract-compatibility-and-versioning.md)
- [ADR-0003：设备命令安全边界](0003-device-command-security-boundary.md)
- [ADR-0004：事件信封与 SSE 传输](0004-event-envelope-and-sse.md)

## T0 所有权

T0 独占维护：

- `contracts/**`
- `docs/architecture/**`
- `internal/api/dto/**`
- `scripts/generate-contracts.py`
- `scripts/check-contracts.py`
- `generated/nexuscontracts/**`

业务线程可读取生成结果，但公共字段、枚举、错误码、事件结构的变更必须先提交 T0 审核。

