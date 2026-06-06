# T5 → T0：`agentdock.yaml` V1 契约需求

T5 不直接修改 `contracts/**`。T0 冻结 `contracts/jsonschema/agentdock-skill-v1.json` 时应与本目录 `Manifest` 保持一致。

固定根字段：

- `apiVersion`: `agentdock.io/v1`
- `kind`: `Skill`
- `metadata`: `name`、`displayName`、`version`、`description`、`license`、`tags`
- `spec.operations[]`: `id`、`description`、`runner`、`entrypoint`、`args`、`timeoutSeconds`、`inputSchema`、`outputSchema`
- `spec.compatibility`: `os`、`arch`、`agentdock`
- `spec.permissions.network`: `mode`、`hosts`
- `spec.permissions.filesystem`: `read`、`write`
- `spec.permissions.secrets[]`: `name`、`required`

约束：未知字段拒绝；名称、操作 ID、SemVer、相对入口路径、runner 枚举、OS/Arch 枚举、网络声明均做严格校验。YAML alias、anchor、tag 和重复 key 被拒绝，避免不同运行时产生不同解释。
