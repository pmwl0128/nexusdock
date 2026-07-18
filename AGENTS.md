# NexusDock 开发约束

## 产品边界

NexusDock 是个人多设备 AgentDock 控制台。一级产品区域保持为总览、Recall、Runtime 和设置；Runtime 必须显式选择 AgentDock 节点。不要恢复已退出的独立 Task、Run、Skill Registry、设备控制面、SSE/EventBus 或旧单节点 Runtime 路由。

## 代码原则

- 修改前先理解现有目录、数据模型、接口契约、错误处理和测试方式。
- 主流程保持连贯，优先具体类型和清楚的数据流；不要为形式上的分层提前增加 interface、helper 或通用抽象。
- 动态 JSON 只停留在 HTTP/Runtime 边界，进入核心流程后尽快转成明确结构。
- 错误必须显式返回并包含定位上下文，不吞错，不只记录日志后继续。
- 非显然业务约束、兼容原因和安全边界使用中文注释说明“为什么”。
- 测试描述真实业务行为，优先覆盖状态变化、错误路径、安全边界和曾经出现过的问题。

## 认证与秘密

- 浏览器管理员账号只存储在 Nexus SQLite 中，通过本地 `nexusdock admin` 命令初始化或重置。
- `NEXUS_AUTH_TOKEN` 仅用于程序化 API；不要恢复 `NEXUS_USERNAME`、`NEXUS_PASSWORD`、`NEXUS_PASSWORD_HASH` 或 `NEXUS_ACCESS_FILE`。
- AgentDock 节点 Token 必须通过节点密钥加密保存，API、日志、测试快照和提交中不得出现明文或密文。

## 验证与提交

- 常规验证执行 `make check`；完整交付执行 `make ci`。
- 修改公共 API 后必须更新生成器并执行 `make contracts`。
- 修改前端后必须提交 `internal/httpx/web_dist` 中对应的嵌入式产物。
- 提交标题使用 `type(scope): 中文说明`；默认从最新 `develop` 创建任务分支，验证后归并 `develop`。
