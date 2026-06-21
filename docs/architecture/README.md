# AgentDock Nexus 架构决策

本目录记录当前个人多设备控制台需要长期遵守的边界。

- [ADR-0001：当前生产契约唯一权威来源](0001-contracts-single-source-of-truth.md)
- [ADR-0002：当前契约兼容与版本策略](0002-contract-compatibility-and-versioning.md)
- [ADR-0003：设备命令安全边界](0003-device-command-security-boundary.md)

当前生产只有 `cmd/recalldock`。公共契约、生成代码、真实路由、五入口前端和部署必须保持一致。已退出的 Task、Run、Context Pack、Skill Catalog、Evolution、独立 Worker/Server 和 SSE 事件总线不得重新作为当前架构出现。
