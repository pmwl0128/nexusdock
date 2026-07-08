# Error Handling

Nexus 错误必须保留内部诊断信息，同时只向调用方返回稳定、脱敏的错误。

## 规则

- 输入验证、授权和状态转换在 Service 层完成。
- HTTP JSON 通过统一 helper 解码，并拒绝未知字段或多段 JSON。
- `401` 用于缺失或失效凭据，`403` 用于策略拒绝，`400` 用于输入错误，`404` 用于资源不存在，`409` 用于版本或状态冲突，未知基础设施错误返回 `500`。
- 不返回 SQL、绝对私密路径、Token、Cookie、密码、Authorization Header 或 Env Secret。
- 控制面错误使用稳定 `code` 和可读 `message`；浏览器兼容层可由前端 API helper 同时解析顶层与嵌套错误结构。
- 设备和 Env 错误应保留足够的脱敏状态，便于确认操作是否完成。

## 契约与测试

- 公共错误码进入 `contracts/error-codes.json`。
- 修改 `internal/httpx` 错误映射时补充对应 Handler 测试。
- 安全敏感接口至少覆盖无凭据、权限不足、失效凭据和非法输入。
