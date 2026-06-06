# T5 交付说明：Skill 规范、导入、导出与 Catalog

## 所有权与边界

本线程仅修改 `internal/skills/**`。未修改 `contracts/**`、`migrations/**`、Runtime、Device、Task、Evolution 或前端。`agentdock.yaml` V1 的 T0 契约需求记录在 `catalog/CONTRACT_REQUIREMENTS.md`。

## 交付内容

- `catalog/manifest.go`：`agentdock.yaml` V1 parser、严格 validator、稳定 YAML serializer 和版本约束。
- `importer/`：local directory、ZIP、Git、generic `SKILL.md` 导入；原目录结构保持；只补 `agentdock.yaml`；原子发布。
- `provenance/`：来源类型、脱敏 URI、上游版本/commit、SHA-256 digest、license 和导入时间。
- `importer/scanner.go`：dangerous shell、hidden binary、path traversal、symlink escape、secret leakage、undeclared network、download-and-execute 检测。
- `catalog/catalog.go`：summary、operations、trust、maturity、compatibility、releases 和 installed devices 查询模型。
- `exporter/`：generic、AgentDock、target adapter 三种导出；拒绝 Secret、私有绝对路径、symlink 和特殊文件泄漏。
- `interfaces.go`：`SkillCatalog`、`SkillImporter`、`SkillExporter`、`SkillPackageValidator` 稳定接口。

## 错误码

导入：`SKILL_INVALID_SOURCE`、`SKILL_INVALID_MANIFEST`、`SKILL_UNSAFE_PACKAGE`、`SKILL_PACKAGE_CONFLICT`、`SKILL_IMPORT_IO`。

导出：`SKILL_EXPORT_INVALID_PACKAGE`、`SKILL_EXPORT_UNSAFE_PACKAGE`、`SKILL_EXPORT_ADAPTER_MISSING`、`SKILL_EXPORT_IO`。

Manifest validation 使用结构化 issue code，例如 `UNSUPPORTED_VERSION`、`INVALID_ENTRYPOINT`、`DUPLICATE_OPERATION`、`MISSING_NETWORK_HOSTS`。

## Migration

T5 不直接提交数据库 migration。当前实现使用 Skill repository 文件布局：

```text
<store>/<skill>/<version>/...
<store>/_metadata/<skill>/<version>/provenance.json
```

若 T1 将 Catalog 元数据落 SQLite，需保留 `skill_name + version` 唯一约束，并持久化 provenance digest、source、license、imported_at；包内容仍留在 Skill repository。

## 审计点

调用层应记录 `skill.import.started/blocked/completed` 与 `skill.export.started/blocked/completed`，包含 actor、request_id、skill、version、source_type、digest、scan finding codes、export format。审计中不得记录凭证化 Source URI、Secret 内容或导出文件正文。

## 验收命令

```bash
go test ./internal/skills/...
go test -race ./internal/skills/...
go vet ./internal/skills/...
go test ./...
```

## 回退

本线程全部新增在 `internal/skills/**`，未改现有 MemoryDock 服务入口和数据。回退分支提交即可；已导入的 Skill package 是独立版本目录，可在确认未被引用后删除对应 `<skill>/<version>` 和 `_metadata/<skill>/<version>`，不影响 Memory 数据。
