# ADR-0004：事件信封与 SSE 传输

- 状态：Accepted
- 日期：2026-06-05
- Owner：T0

## 决策

所有域事件使用统一信封：

- `id`：全局唯一事件 ID。
- `type`：冻结的事件类型。
- `version`：事件 Schema 版本，当前为 `1`。
- `occurred_at`：事实发生时间。
- `producer`：产生事件的服务或节点。
- `subject`：主对象引用。
- `correlation_id`：跨服务关联 ID，可选。
- `causation_id`：直接原因事件 ID，可选。
- `data`：事件特定 payload。

SSE 使用事件 `id` 作为 `id:`，事件 `type` 作为 `event:`，完整信封 JSON 作为 `data:`。消费者必须支持断线后通过 `Last-Event-ID` 恢复。至少一次投递是默认语义，消费者必须按事件 ID 幂等处理。

