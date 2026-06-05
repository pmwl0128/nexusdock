# ADR-0002：兼容性与版本策略

- 状态：Accepted
- 日期：2026-06-05
- Owner：T0

## 决策

公共 API 当前版本为 `v1`，遵循以下规则：

1. 冻结后可新增 optional 字段。
2. 不得删除字段、收紧已有字段约束、修改字段含义或修改既有枚举值。
3. 新增枚举值属于潜在兼容风险，必须明确消费者的 unknown-value 行为并通过 T0 审核。
4. Breaking Change 必须发布新 API/Schema major 版本。
5. 所有写接口支持 `Idempotency-Key`；同一 actor、operation 与 key 必须返回同一逻辑结果。
6. 资源更新使用 `version` 做乐观并发控制，冲突返回 `VERSION_CONFLICT`。
7. `contracts/compatibility/v1-baseline.json` 保存冻结签名；`scripts/check-contracts.py` 检查删除、必填项增加、类型变化和枚举收窄。

## 时间格式

所有时间字段使用 RFC 3339 UTC 字符串。持续时间统一使用整数秒或毫秒，字段名必须带单位。

