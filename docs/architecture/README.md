# AgentDock Nexus 架构决策

本目录记录当前个人 AgentDock 控制台需要长期遵守的边界。

- [ADR-0004：Nexus、Recall 与 Runtime 所有权基线](0004-nexus-recall-runtime-baseline.md) 是当前架构基线。
- [ADR-0001：当前生产契约唯一权威来源](0001-contracts-single-source-of-truth.md) 是历史决策，必须通过 ADR-0004 解释。
- [ADR-0002：当前契约兼容与版本策略](0002-contract-compatibility-and-versioning.md) 是历史决策，必须通过 ADR-0004 解释。
- [ADR-0003：设备命令安全边界](0003-device-command-security-boundary.md) 是历史决策，必须通过 ADR-0004 解释。

当前基线：Nexus 是控制台和 API facade；Recall 是 Git Markdown 内容仓库；AgentDock Runtime 拥有 Task、Skill、Workflow 生命周期。公共契约、生成代码、真实路由、六个顶层前端入口和部署必须保持一致。
