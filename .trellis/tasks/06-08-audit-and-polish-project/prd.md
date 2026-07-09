# 审计并规范化项目缺陷

## Goal

对 NexusDock 仓库做一次轻量工程审计，修复不需要产品决策即可确认的规范缺口，让后续开发者和 AI 代理能用真实项目约束执行构建、测试和代码修改。

## What I Already Know

- 当前仓库是 Go 后端 + React/Vite 前端的单仓库项目。
- `go test ./...`、`python3 scripts/check-contracts.py`、`web/npm run build` 当前通过。
- `Makefile` 仍以旧 `memorydock` 二进制作为唯一 build/run 入口，未覆盖 `cmd/nexusdock-server`、`cmd/nexusdock-worker`、契约检查和前端构建。
- `.trellis/spec/backend/*.md` 仍为模板占位，缺少当前代码真实遵循的目录、数据库、错误处理、质量和日志规范。
- `cmd/memorydock` 仍存在并作为 Memory 兼容入口，不应在本任务中删除。

## Requirements

- 保持业务行为不变，仅修复项目入口和规范文档。
- 更新 Makefile，使常用命令覆盖 Go 格式化、测试、vet、构建 Nexus server/worker、构建 Memory 兼容入口、契约检查和前端构建。
- 将后端 Trellis spec 从模板占位改成基于当前代码的真实约束。
- 不修改生成契约或前端资源，除非验证命令真实产生漂移。

## Acceptance Criteria

- [x] `make build` 能构建 Nexus server、Nexus worker 和 Memory 兼容入口。
- [x] `make test` 通过。
- [x] `python3 scripts/check-contracts.py` 通过。
- [x] `cd web && npm run build` 通过。
- [x] `.trellis/spec/backend/*.md` 不再包含模板占位 `(To be filled by the team)`。

## Out of Scope

- 不重构业务代码。
- 不删除 MemoryDock 兼容入口。
- 不改数据库 schema、OpenAPI 或 JSON Schema。
- 不启动或部署生产服务。

## Technical Notes

- 质量命令初始验证：Go 测试、契约检查和前端构建通过。
- `cmd/memorydock/main.go` 仍是兼容 Memory API/Web 的入口。
- `cmd/nexusdock-server` 是 Nexus 控制面服务入口。
- 规范文档应记录当前代码事实，而不是理想化模板。
