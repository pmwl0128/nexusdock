# ADR-0003：设备命令安全边界

- 状态：Accepted
- 日期：2026-06-05
- Owner：T0

## 决策

Nexus v1 不提供任意 Shell 命令。设备命令 `type` 只允许：

- `health.check`
- `skill.install`
- `skill.run`
- `skill.rollback`
- `memory.sync`
- `service.inspect`
- `service.restart`
- `diagnostics.collect`
- `agentdock.reload`

命令必须包含风险等级、过期时间、幂等键和结构化 payload。节点只执行契约中声明且本地策略允许的命令。租约过期、token 撤销、命令过期或 handler 缺失时不得产生副作用。

命令 payload 的具体字段由对应命令 handler 在业务线程内校验；公共信封禁止携带明文 Secret。

