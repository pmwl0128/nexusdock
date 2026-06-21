# AgentDock Nexus

AgentDock Nexus 是面向个人多设备环境的 AgentDock 控制台，集中管理长期召回、设备、加密文件中继、备份状态和基础账号安全。

## 产品边界

一级入口只有五个：总览、设备、召回、文件、设置。

- 总览：设备异常、近期文件传输和备份状态。
- 设备：注册、审批、心跳、能力与 Skill 上报、Env、命令和历史。
- 召回：Markdown 召回库、Git 变更审阅和同步。
- 文件：Artifact 发送、Delivery 落盘和反向 Fetch 状态。
- 设置：管理员账号、浏览器会话、SQLite 健康和备份信息。

独立 Task/Inbox、Run Registry、Run Evidence、Context Pack、Skill Evolution、独立 Worker、独立 Nexus Server 和 Agent/System Token 管理 UI 不属于当前产品。

## 运行结构

```text
cmd/recalldock      唯一生产服务入口
internal/recall     召回文件与 Git 同步（内部包名，非公开 API）
internal/devices    设备注册、心跳与策略
internal/commands   设备命令队列
internal/artifacts  ADR1 加密 Artifact Relay / Fetch
internal/auth       浏览器会话与设备鉴权
internal/httpx      HTTP API 与嵌入式 Web UI
web                 React 前端
```

生产数据库仍保留历史表以避免破坏性迁移；未使用表不再由产品代码读写。
