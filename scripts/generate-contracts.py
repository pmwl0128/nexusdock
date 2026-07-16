#!/usr/bin/env python3
"""Generate Nexus OpenAPI, JSON Schemas, event schemas and Go client DTOs.

The generator intentionally uses only the Python standard library so every
AgentDock development host can reproduce the checked-in output.
"""

from __future__ import annotations

import json
import pathlib
import subprocess
from typing import Any


ROOT = pathlib.Path(__file__).resolve().parents[1]
CONTRACTS = ROOT / "contracts"
GENERATED = ROOT / "generated" / "nexuscontracts"


def scalar(kind: str | list[str], description: str, **extra: Any) -> dict[str, Any]:
    return {"type": kind, "description": description, **extra}


def enum(description: str, values: list[str]) -> dict[str, Any]:
    return scalar("string", description, enum=values)


def ref(name: str) -> dict[str, Any]:
    return {"$ref": f"#/components/schemas/{name}"}


def array(description: str, items: dict[str, Any]) -> dict[str, Any]:
    return scalar("array", description, items=items)


def obj(
    description: str,
    properties: dict[str, Any],
    required: tuple[str, ...] = (),
    *,
    additional: bool | dict[str, Any] = False,
) -> dict[str, Any]:
    value: dict[str, Any] = {
        "type": "object",
        "description": description,
        "additionalProperties": additional,
        "properties": properties,
    }
    if required:
        value["required"] = list(required)
    return value


ID = scalar("string", "全局唯一标识符。", format="uuid")
TIMESTAMP = scalar("string", "RFC 3339 UTC 时间。", format="date-time")
VERSION = scalar("integer", "资源乐观锁版本，从 1 开始。", minimum=1)

ERROR_CODES = [
    "AUTH_REQUIRED",
    "FORBIDDEN",
    "INVALID_TOKEN",
    "TOKEN_REVOKED",
    "VERSION_CONFLICT",
    "DB_CONFLICT",
    "INTERNAL_ERROR",
    "INVALID_JSON",
    "VALIDATION_ERROR",
    "NOT_FOUND",
    "ALREADY_EXISTS",
    "RATE_LIMITED",
    "IDEMPOTENCY_CONFLICT",
    "COMMAND_EXPIRED",
    "LEASE_EXPIRED",
    "UNSUPPORTED_COMMAND",
    "SKILL_BLOCKED",
    "SCHEMA_MISMATCH",
    "AGENTDOCK_NODE_STORE_UNAVAILABLE",
    "AGENTDOCK_NODE_LIST_FAILED",
    "AGENTDOCK_NODE_NOT_FOUND",
    "AGENTDOCK_NODE_EXISTS",
    "AGENTDOCK_NODE_DISABLED",
    "INVALID_AGENTDOCK_NODE",
    "AGENTDOCK_NODE_OPERATION_FAILED",
    "AGENTDOCK_NODE_CREDENTIALS_UNAVAILABLE",
    "AGENTDOCK_RUNTIME_UNREACHABLE",
    "AGENTDOCK_RUNTIME_BAD_RESPONSE",
]


def build_schemas() -> dict[str, dict[str, Any]]:
    schemas: dict[str, dict[str, Any]] = {}
    schemas["JsonObject"] = obj("通用结构化对象。", {}, additional=True)
    schemas["ErrorDetail"] = obj(
        "字段级错误详情。",
        {
            "field": scalar("string", "字段路径。"),
            "reason": scalar("string", "稳定原因标识。"),
            "message": scalar("string", "可读错误说明。"),
        },
        ("reason", "message"),
    )
    schemas["ErrorResponse"] = obj(
        "统一错误响应。",
        {
            "code": scalar("string", "稳定错误码。"),
            "message": scalar("string", "面向调用方的错误说明。"),
            "request_id": scalar("string", "请求关联 ID。"),
            "details": array("可选字段级错误。", ref("ErrorDetail")),
        },
        ("code", "message", "request_id"),
    )
    schemas["LegacyErrorEnvelope"] = obj(
        "Nexus 与浏览器接口使用的错误信封。",
        {
            "ok": scalar("boolean", "固定为 false。"),
            "error": obj(
                "错误详情。",
                {
                    "code": scalar("string", "稳定错误码。"),
                    "message": scalar("string", "可读错误说明。"),
                },
                ("code", "message"),
            ),
        },
        ("ok", "error"),
    )
    schemas["HealthResponse"] = obj(
        "服务健康状态。",
        {
            "ok": scalar("boolean", "服务是否健康。"),
            "service": scalar("string", "服务名称。"),
        },
        ("ok", "service"),
    )
    schemas["SystemStatus"] = obj(
        "Nexus 系统与数据存储状态。",
        {
            "ok": scalar("boolean", "系统是否健康。"),
            "service": scalar("string", "服务名称，固定为 nexusdock。"),
            "database": scalar("string", "SQLite 健康状态。"),
            "schema_version": scalar("integer", "数据库 Schema 版本。", minimum=0),
            "nexus_data_dir": scalar("string", "Nexus 系统状态目录。"),
            "recall_repo_dir": scalar("string", "Recall Git Markdown 仓库目录。"),
        },
        ("ok", "service", "database", "schema_version", "nexus_data_dir", "recall_repo_dir"),
    )
    schemas["BackupHistory"] = obj(
        "一次备份执行的脱敏结果。",
        {
            "schema_version": scalar("integer", "状态文件版本。", minimum=0),
            "state": enum("备份状态。", ["never_run", "queued", "running", "success", "failed", "unknown", "disabled"]),
            "message": scalar("string", "状态说明。"),
            "started_at": TIMESTAMP,
            "completed_at": TIMESTAMP,
            "host": scalar("string", "执行主机。"),
            "archive": scalar("string", "归档文件名。"),
            "archive_size": scalar("integer", "归档字节数。", minimum=0),
            "sha256": scalar("string", "归档 SHA-256。"),
            "remote_path": scalar("string", "脱敏后的远端路径。"),
        },
        ("state",),
    )
    schemas["BackupStatus"] = obj(
        "AgentDock 与 Nexus 的单一备份状态。",
        {
            "id": scalar("string", "稳定备份标识。"),
            "title": scalar("string", "备份名称。"),
            "description": scalar("string", "备份内容说明。"),
            "provider": scalar("string", "计划执行提供方。"),
            "host": scalar("string", "执行主机。"),
            "enabled": scalar("boolean", "备份是否启用。"),
            "schedule": scalar("string", "可读计划。"),
            "schedule_type": scalar("string", "计划类型。"),
            "state": enum("备份状态。", ["never_run", "queued", "running", "success", "failed", "unknown", "disabled"]),
            "last_started_at": TIMESTAMP,
            "last_completed_at": TIMESTAMP,
            "next_run_at": TIMESTAMP,
            "message": scalar("string", "状态说明。"),
            "archive": scalar("string", "最近归档文件名。"),
            "archive_size": scalar("integer", "最近归档字节数。", minimum=0),
            "sha256": scalar("string", "最近归档 SHA-256。"),
            "remote_path": scalar("string", "脱敏后的远端路径。"),
            "history": array("最近备份历史。", ref("BackupHistory")),
        },
        ("id", "title", "provider", "host", "enabled", "schedule", "state", "next_run_at", "history"),
    )
    schemas["RecallEntry"] = obj(
        "Markdown 召回条目。",
        {
            "path": scalar("string", "召回相对路径。"),
            "content": scalar("string", "Markdown 或文本内容。"),
            "size_bytes": scalar("integer", "内容字节数。", minimum=0),
            "modified_at": TIMESTAMP,
        },
        ("path",),
    )
    schemas["PrivateNoteSummary"] = obj(
        "私密笔记安全元数据；不包含正文或正文片段。",
        {
            "path": scalar("string", "notes/ 下的私密笔记相对路径。"),
            "encrypted_path": scalar("string", "对应 age 密文相对路径。"),
            "category": scalar("string", "私密笔记分类。"),
            "title": scalar("string", "私密笔记标题。"),
            "summary": scalar("string", "人工维护的安全简介。"),
            "tags": array("安全标签。", scalar("string", "标签。")),
            "updated_at": TIMESTAMP,
            "contains_secret": scalar("boolean", "正文是否被标记为含敏感信息。"),
            "score": scalar("integer", "元数据检索匹配分数。", minimum=0),
        },
        ("path", "encrypted_path", "contains_secret"),
    )
    schemas["PrivateNoteSearchRequest"] = obj(
        "私密笔记元数据检索请求。",
        {
            "query": scalar("string", "仅匹配标题、简介、标签、分类和路径的查询。"),
            "max_results": scalar("integer", "最大结果数。", minimum=1, maximum=100),
        },
        ("query",),
    )
    schemas["PrivateNoteSearchResponse"] = obj(
        "私密笔记元数据检索结果。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "action": scalar("string", "固定为 search。"),
            "query": scalar("string", "原始查询。"),
            "root": scalar("string", "Nexus 私密笔记根目录。"),
            "results": array("仅含安全元数据的结果。", ref("PrivateNoteSummary")),
            "count": scalar("integer", "结果数。", minimum=0),
            "metadata_only": scalar("boolean", "固定为 true。"),
            "policy": scalar("string", "检索安全策略说明。"),
        },
        ("ok", "action", "query", "root", "results", "count", "metadata_only"),
    )
    schemas["PrivateNoteReadRequest"] = obj(
        "显式读取私密笔记正文的请求。",
        {
            "path": scalar("string", "notes/ 下的私密笔记相对路径。"),
            "max_bytes": scalar("integer", "最大返回字节数。", minimum=1, maximum=1048576),
        },
        ("path",),
    )
    schemas["PrivateNoteReadResponse"] = obj(
        "显式私密笔记明文读取结果。",
        {
            "action": scalar("string", "固定为 read。"),
            "root": scalar("string", "Nexus 私密笔记根目录。"),
            "path": scalar("string", "明文相对路径。"),
            "encrypted_path": scalar("string", "age 密文相对路径。"),
            "content": scalar("string", "私密笔记明文，仅显式读取接口返回。"),
            "truncated": scalar("boolean", "正文是否被截断。"),
            "contains_secret": scalar("boolean", "正文是否被标记为含敏感信息。"),
        },
        ("action", "root", "path", "encrypted_path", "content", "truncated", "contains_secret"),
    )
    schemas["PrivateNoteWriteRequest"] = obj(
        "创建或覆盖私密笔记请求。",
        {
            "path": scalar("string", "可选的 notes/ 相对路径。"),
            "category": scalar("string", "未传 path 时使用的分类。"),
            "title": scalar("string", "标题，也可用于生成路径。"),
            "summary": scalar("string", "可安全检索的人工简介。"),
            "tags": array("可安全检索的标签。", scalar("string", "标签。")),
            "content": scalar("string", "私密笔记正文。"),
            "confirmed": scalar("boolean", "真实写入必须为 true。"),
            "overwrite": scalar("boolean", "是否覆盖已有笔记。"),
        },
        ("content", "confirmed"),
    )
    schemas["PrivateNoteWriteResponse"] = obj(
        "私密笔记明文与 age 密文原子写入结果。",
        {
            "action": scalar("string", "固定为 write。"),
            "root": scalar("string", "Nexus 私密笔记根目录。"),
            "path": scalar("string", "明文相对路径。"),
            "encrypted_path": scalar("string", "age 密文相对路径。"),
            "written": scalar("boolean", "明文是否写入。"),
            "encrypted": scalar("boolean", "密文是否写入。"),
            "algorithm": scalar("string", "加密算法。"),
        },
        ("action", "root", "path", "encrypted_path", "written", "encrypted", "algorithm"),
    )
    schemas["PrivateNoteDeleteRequest"] = obj(
        "同时删除私密笔记明文和 age 密文的请求。",
        {
            "path": scalar("string", "notes/ 下的私密笔记相对路径。"),
            "confirmed": scalar("boolean", "真实删除必须为 true。"),
        },
        ("path", "confirmed"),
    )
    schemas["PrivateNoteDeleteResponse"] = obj(
        "私密笔记明文与 age 密文删除结果。",
        {
            "action": scalar("string", "固定为 delete。"),
            "root": scalar("string", "Nexus 私密笔记根目录。"),
            "path": scalar("string", "明文相对路径。"),
            "encrypted_path": scalar("string", "age 密文相对路径。"),
            "deleted_plaintext": scalar("boolean", "明文是否删除。"),
            "deleted_encrypted": scalar("boolean", "密文是否删除。"),
        },
        ("action", "root", "path", "encrypted_path", "deleted_plaintext", "deleted_encrypted"),
    )
    schemas["PrivateNoteStatusRequest"] = obj(
        "读取私密笔记状态或安全元数据列表。",
        {"action": enum("状态动作。", ["check", "list"])},
        ("action",),
    )
    schemas["PrivateNoteStatusResponse"] = obj(
        "私密笔记加密和 Git 忽略状态。",
        {
            "action": scalar("string", "执行的状态动作。"),
            "root": scalar("string", "Nexus 私密笔记根目录。"),
            "notes": array("私密笔记安全元数据。", ref("PrivateNoteSummary")),
            "count": scalar("integer", "列表项数。", minimum=0),
            "notes_count": scalar("integer", "明文笔记数。", minimum=0),
            "missing_encrypted": array("缺失的 age 密文路径。", scalar("string", "密文路径。")),
            "encrypted_backup_ok": scalar("boolean", "每条明文是否都有密文。"),
            "plaintext_git_ignored": scalar("boolean", "notes/ 是否由仓库规则忽略。"),
            "keys_git_ignored": scalar("boolean", ".keys/ 是否由仓库规则忽略。"),
        },
        ("action", "root", "encrypted_backup_ok", "plaintext_git_ignored", "keys_git_ignored"),
    )
    schemas["PrivateNoteMaintenanceRequest"] = obj(
        "私密笔记加密维护请求。",
        {"action": enum("维护动作。", ["init", "init-encryption", "sync-encrypted", "encrypt-all"])},
        ("action",),
    )
    schemas["PrivateNoteMaintenanceResponse"] = obj(
        "私密笔记加密初始化或全量重加密结果。",
        {
            "action": scalar("string", "执行的维护动作。"),
            "root": scalar("string", "Nexus 私密笔记根目录。"),
            "recipient": scalar("string", "age 公钥接收者。"),
            "identity_created": scalar("boolean", "是否新建 identity。"),
            "encrypted_count": scalar("integer", "生成的密文数量。", minimum=0),
            "algorithm": scalar("string", "加密算法。"),
        },
        ("action", "root", "algorithm"),
    )
    schemas["AgentDockNode"] = obj(
        "Nexus 管理的一台 AgentDock Runtime 节点。",
        {
            "id": scalar("string", "稳定节点 ID。", pattern="^[a-z0-9][a-z0-9_-]{0,63}$"),
            "name": scalar("string", "节点显示名称。", minLength=1, maxLength=100),
            "endpoint": scalar("string", "AgentDock HTTP/HTTPS Origin。", format="uri"),
            "enabled": scalar("boolean", "节点是否允许 Runtime 请求。"),
            "timeout_seconds": scalar("integer", "请求超时秒数。", minimum=1, maximum=300),
            "token_configured": scalar("boolean", "节点 Token 是否已配置；不会返回 Token 原值。"),
            "created_at": TIMESTAMP,
            "updated_at": TIMESTAMP,
        },
        ("id", "name", "endpoint", "enabled", "timeout_seconds", "token_configured", "created_at", "updated_at"),
    )
    schemas["AgentDockNodeCreateRequest"] = obj(
        "新增 AgentDock 节点。",
        {
            "id": scalar("string", "稳定节点 ID。", pattern="^[a-z0-9][a-z0-9_-]{0,63}$"),
            "name": scalar("string", "节点显示名称。", minLength=1, maxLength=100),
            "endpoint": scalar("string", "AgentDock HTTP/HTTPS Origin。", format="uri"),
            "token": scalar("string", "该节点独立 Bearer Token。", minLength=1, maxLength=16384, writeOnly=True),
            "enabled": scalar("boolean", "节点是否启用。"),
            "timeout_seconds": scalar("integer", "请求超时秒数。", minimum=1, maximum=300),
        },
        ("id", "name", "endpoint", "token"),
    )
    schemas["AgentDockNodeUpdateRequest"] = obj(
        "更新 AgentDock 节点；省略 token 时保留现有凭据。",
        {
            "name": scalar("string", "节点显示名称。", minLength=1, maxLength=100),
            "endpoint": scalar("string", "AgentDock HTTP/HTTPS Origin。", format="uri"),
            "token": scalar("string", "替换该节点的 Bearer Token。", minLength=1, maxLength=16384, writeOnly=True),
            "enabled": scalar("boolean", "节点是否启用。"),
            "timeout_seconds": scalar("integer", "请求超时秒数。", minimum=1, maximum=300),
        },
    )
    schemas["AgentDockNodeListResponse"] = obj(
        "AgentDock 节点列表。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "nodes": array("AgentDock 节点。", ref("AgentDockNode")),
            "count": scalar("integer", "节点数量。", minimum=0),
        },
        ("ok", "nodes", "count"),
    )
    schemas["AgentDockNodeResponse"] = obj(
        "单个 AgentDock 节点响应。",
        {"ok": scalar("boolean", "请求是否成功。"), "node": ref("AgentDockNode")},
        ("ok", "node"),
    )
    return schemas

def response(schema: dict[str, Any], description: str = "成功。") -> dict[str, Any]:
    return {"description": description, "content": {"application/json": {"schema": schema}}}


def build_openapi(schemas: dict[str, Any]) -> dict[str, Any]:
    error = response({"oneOf": [ref("ErrorResponse"), ref("LegacyErrorEnvelope")]}, "错误。")
    generic = ref("JsonObject")

    def path_param(name: str, description: str, *, uuid: bool = True) -> dict[str, Any]:
        schema: dict[str, Any] = {"type": "string"}
        if uuid:
            schema["format"] = "uuid"
        return {"name": name, "in": "path", "required": True, "description": description, "schema": schema}

    parameters = {
        "SessionId": path_param("sessionID", "浏览器 Session ID。", uuid=False),
        "RecallPath": path_param("path", "URL 编码后的召回相对路径。", uuid=False),
        "RuntimeNodeId": path_param("nodeID", "Nexus 中登记的 AgentDock 节点 ID。", uuid=False),
        "RuntimeTaskId": path_param("taskID", "AgentDock Runtime task ID。", uuid=False),
        "RuntimeSkillSource": path_param("source", "AgentDock Runtime skill source。", uuid=False),
        "RuntimeSkillId": path_param("skillID", "AgentDock Runtime skill ID。", uuid=False),
        "RuntimeSkillFilePath": path_param("filePath", "AgentDock Runtime Skill 文件相对路径。", uuid=False),
        "RuntimeMCPName": path_param("name", "AgentDock 动态 MCP 服务名称。", uuid=False),
        "WorkflowTemplateId": path_param("templateID", "Nexus Workflow 模板 ID。", uuid=False),
        "WorkflowTemplateVersion": path_param("version", "Nexus Workflow 模板版本。", uuid=False),
        "WorkflowTemplateAction": path_param("action", "Nexus Workflow 模板动作。", uuid=False),
    }

    def body(schema: dict[str, Any] = generic) -> dict[str, Any]:
        return {"required": True, "content": {"application/json": {"schema": schema}}}

    def ok(schema: dict[str, Any] = generic, description: str = "成功。") -> dict[str, Any]:
        return response(schema, description)

    def operation(
        operation_id: str,
        summary: str,
        *,
        success: dict[str, Any] | None = None,
        request: dict[str, Any] | None = None,
        params: list[dict[str, Any]] | None = None,
        success_code: str = "200",
        additional_success: dict[str, dict[str, Any]] | None = None,
    ) -> dict[str, Any]:
        responses = {success_code: success or ok(), "400": error, "401": error, "403": error, "404": error, "409": error}
        if additional_success:
            responses.update(additional_success)
        value: dict[str, Any] = {
            "operationId": operation_id,
            "summary": summary,
            "responses": responses,
        }
        if request is not None:
            value["requestBody"] = request
        if params:
            value["parameters"] = params
        return value

    p = lambda name: {"$ref": f"#/components/parameters/{name}"}
    no_content = {"description": "已接受，无响应体。"}
    paths: dict[str, Any] = {
        "/health": {"get": operation("getHealth", "读取服务健康状态", success=ok(ref("HealthResponse")))},
        "/v1/system/status": {"get": operation("getSystemStatus", "读取 Nexus 与 SQLite 状态", success=ok(ref("SystemStatus")))},
        "/v1/backup/status": {"get": operation("getBackupStatus", "读取 AgentDock 与 Nexus 备份状态", success=ok(ref("BackupStatus")))},
        "/v1/auth/status": {"get": operation("getAuthStatus", "读取管理员初始化状态")},
        "/v1/auth/login": {"post": operation("login", "登录管理员会话", request=body())},
        "/v1/auth/session": {"get": operation("getCurrentSession", "读取当前浏览器会话")},
        "/v1/auth/logout": {"post": operation("logout", "退出当前浏览器会话", request=body())},
        "/v1/auth/credential": {"post": operation("updateCredential", "更新管理员凭据", request=body())},
        "/v1/auth/sessions": {"get": operation("listSessions", "列出管理员浏览器会话")},
        "/v1/auth/sessions/{sessionID}": {"delete": operation("revokeSession", "撤销指定浏览器会话", params=[p("SessionId")])},
        "/v1/auth/sessions/logout-others": {"post": operation("logoutOtherSessions", "撤销其他浏览器会话", request=body())},
        "/v1/recall": {
            "get": operation("listRecall", "列出召回条目"),
            "post": operation("writeRecall", "创建召回条目", request=body(ref("RecallEntry"))),
        },
        "/v1/recall/move": {"post": operation("moveRecall", "移动召回条目", request=body())},
        "/v1/recall/search": {"post": operation("searchRecall", "搜索召回内容", request=body())},
        "/v1/recall/pack": {"post": operation("packRecall", "打包召回条目", request=body())},
        "/v1/recall/notes/append": {"post": operation("appendRecallNote", "追加召回笔记", request=body())},
        "/v1/recall/{path}": {
            "get": operation("readRecall", "读取召回条目", params=[p("RecallPath")], success=ok(ref("RecallEntry"))),
            "patch": operation("patchRecall", "修改召回条目", params=[p("RecallPath")], request=body()),
            "delete": operation("deleteRecall", "删除召回条目", params=[p("RecallPath")]),
        },
        "/v1/private-notes/search": {
            "post": operation(
                "searchPrivateNotes",
                "只按标题、简介、标签、分类和路径检索私密笔记",
                request=body(ref("PrivateNoteSearchRequest")),
                success=ok(ref("PrivateNoteSearchResponse")),
            )
        },
        "/v1/private-notes/read": {
            "post": operation(
                "readPrivateNote",
                "显式读取一条私密笔记明文",
                request=body(ref("PrivateNoteReadRequest")),
                success=ok(ref("PrivateNoteReadResponse")),
            )
        },
        "/v1/private-notes/write": {
            "post": operation(
                "writePrivateNote",
                "原子写入私密笔记明文与 age 密文",
                request=body(ref("PrivateNoteWriteRequest")),
                success=ok(ref("PrivateNoteWriteResponse")),
            )
        },
        "/v1/private-notes/delete": {
            "post": operation(
                "deletePrivateNote",
                "同时删除私密笔记明文与 age 密文",
                request=body(ref("PrivateNoteDeleteRequest")),
                success=ok(ref("PrivateNoteDeleteResponse")),
            )
        },
        "/v1/private-notes/status": {
            "post": operation(
                "getPrivateNoteStatus",
                "读取私密笔记加密和 Git 忽略状态",
                request=body(ref("PrivateNoteStatusRequest")),
                success=ok(ref("PrivateNoteStatusResponse")),
            )
        },
        "/v1/private-notes/maintenance": {
            "post": operation(
                "maintainPrivateNotes",
                "初始化或重新生成私密笔记 age 密文",
                request=body(ref("PrivateNoteMaintenanceRequest")),
                success=ok(ref("PrivateNoteMaintenanceResponse")),
            )
        },
        "/v1/sync/status": {"get": operation("getSyncStatus", "读取召回 Git 同步状态")},
        "/v1/git/diff": {"get": operation("getGitDiff", "读取召回仓库变更")},
        "/v1/git/discard": {"post": operation("discardGitChanges", "丢弃召回仓库本地变更", request=body())},
        "/v1/git/log": {"get": operation("getGitLog", "读取召回仓库提交历史")},
        "/v1/git/commit": {"get": operation("getGitCommit", "读取召回仓库提交详情")},
        "/v1/sync/pull": {"post": operation("pullRecall", "从远端更新召回仓库", request=body())},
        "/v1/sync/push": {"post": operation("pushRecall", "保存召回仓库到远端", request=body())},
        "/v1/sync/now": {"post": operation("syncRecallNow", "立即双向同步召回仓库", request=body())},
        "/v1/runtime/nodes": {
            "get": operation("listAgentDockNodes", "列出 Nexus 管理的 AgentDock 节点", success=ok(ref("AgentDockNodeListResponse"))),
            "post": operation("createAgentDockNode", "新增 AgentDock 节点", request=body(ref("AgentDockNodeCreateRequest")), success=ok(ref("AgentDockNodeResponse")), success_code="201"),
        },
        "/v1/runtime/nodes/{nodeID}": {
            "get": operation("getAgentDockNode", "读取 AgentDock 节点", params=[p("RuntimeNodeId")], success=ok(ref("AgentDockNodeResponse"))),
            "patch": operation("updateAgentDockNode", "更新 AgentDock 节点", params=[p("RuntimeNodeId")], request=body(ref("AgentDockNodeUpdateRequest")), success=ok(ref("AgentDockNodeResponse"))),
            "delete": operation("deleteAgentDockNode", "删除 AgentDock 节点及加密凭据", params=[p("RuntimeNodeId")]),
        },
        "/v1/runtime/nodes/{nodeID}/probe": {"post": operation("probeAgentDockNode", "验证 AgentDock 节点连接、认证和 Runtime API", params=[p("RuntimeNodeId")])},
        "/v1/runtime/nodes/{nodeID}/overview": {"get": operation("getRuntimeOverview", "读取指定 AgentDock 节点的 Runtime 概览", params=[p("RuntimeNodeId")])},
        "/v1/runtime/nodes/{nodeID}/tasks": {"get": operation("listRuntimeTasks", "列出指定 AgentDock 节点的任务", params=[p("RuntimeNodeId")])},
        "/v1/runtime/nodes/{nodeID}/tasks/{taskID}": {
            "get": operation("getRuntimeTask", "读取指定 AgentDock 节点的任务详情", params=[p("RuntimeNodeId"), p("RuntimeTaskId")]),
            "delete": operation("deleteRuntimeTask", "删除指定 AgentDock 节点的任务", params=[p("RuntimeNodeId"), p("RuntimeTaskId")]),
        },
        "/v1/runtime/nodes/{nodeID}/skills": {"get": operation("listRuntimeSkills", "列出指定 AgentDock 节点的 Skill", params=[p("RuntimeNodeId")])},
        "/v1/runtime/nodes/{nodeID}/skills/{source}/{skillID}": {"get": operation("getRuntimeSkill", "读取指定 AgentDock 节点的 Skill 详情", params=[p("RuntimeNodeId"), p("RuntimeSkillSource"), p("RuntimeSkillId")])},
        "/v1/runtime/nodes/{nodeID}/skills/{source}/{skillID}/files/{filePath}": {"get": operation("getRuntimeSkillFile", "读取指定 AgentDock 节点的 Skill 文件", params=[p("RuntimeNodeId"), p("RuntimeSkillSource"), p("RuntimeSkillId"), p("RuntimeSkillFilePath")])},
        "/v1/runtime/nodes/{nodeID}/mcp": {
            "get": operation("listRuntimeMCPServers", "列出指定 AgentDock 节点的动态 MCP 服务", params=[p("RuntimeNodeId")]),
            "post": operation("manageRuntimeMCPServer", "管理指定 AgentDock 节点的动态 MCP 服务", params=[p("RuntimeNodeId")], request=body()),
        },
        "/v1/runtime/nodes/{nodeID}/mcp/{name}": {"get": operation("getRuntimeMCPServer", "读取指定 AgentDock 节点的动态 MCP 服务", params=[p("RuntimeNodeId"), p("RuntimeMCPName")])},
        "/v1/runtime/nodes/{nodeID}/mcp/{name}/environment": {"get": operation("getRuntimeMCPEnvironment", "读取指定 AgentDock 节点的 MCP 隔离环境元数据", params=[p("RuntimeNodeId"), p("RuntimeMCPName")])},
        "/v1/workflow-templates": {"get": operation("listWorkflowTemplates", "列出 Nexus Workflow 模板")},
        "/v1/workflow-templates/drafts": {"post": operation("saveWorkflowTemplateDraft", "保存 Nexus Workflow 模板草稿", request=body())},
        "/v1/workflow-templates/match": {"post": operation("matchWorkflowTemplates", "匹配 Nexus Workflow 模板", request=body())},
        "/v1/workflow-templates/reindex": {"post": operation("reindexWorkflowTemplates", "重建 Nexus Workflow 模板向量索引", request=body())},
        "/v1/workflow-templates/vector-index": {"get": operation("getWorkflowTemplateVectorIndex", "读取 Nexus Workflow 模板向量索引状态")},
        "/v1/workflow-templates/{templateID}/{version}": {"get": operation("getWorkflowTemplate", "读取 Nexus Workflow 模板详情", params=[p("WorkflowTemplateId"), p("WorkflowTemplateVersion")])},
        "/v1/workflow-templates/{templateID}/{version}/{action}": {"post": operation("manageWorkflowTemplate", "验证、发布或退役 Nexus Workflow 模板", params=[p("WorkflowTemplateId"), p("WorkflowTemplateVersion"), p("WorkflowTemplateAction")])},
    }

    return {
        "openapi": "3.1.0",
        "info": {
            "title": "NexusDock API",
            "version": "1.0.0",
            "description": "个人 NexusDock 控制台的当前 HTTP 契约，覆盖 Recall、备份、账号会话和 AgentDock Runtime 视图。",
        },
        "servers": [{"url": "/", "description": "当前 Nexus 实例。"}],
        "paths": paths,
        "components": {"parameters": parameters, "schemas": schemas},
    }

def rewrite_refs(value: Any) -> Any:
    if isinstance(value, dict):
        return {key: (item.replace("#/components/schemas/", "#/$defs/") if key == "$ref" else rewrite_refs(item)) for key, item in value.items()}
    if isinstance(value, list):
        return [rewrite_refs(item) for item in value]
    return value


def snake_to_camel(value: str) -> str:
    return "".join(part[:1].upper() + part[1:] for part in value.split("_"))


def go_type(schema: dict[str, Any], required: bool) -> str:
    if "$ref" in schema:
        base = schema["$ref"].rsplit("/", 1)[-1]
    elif "oneOf" in schema:
        non_null = [item for item in schema["oneOf"] if item.get("type") != "null"]
        if len(non_null) == 1 and "$ref" in non_null[0]:
            return "*" + non_null[0]["$ref"].rsplit("/", 1)[-1]
        return "json.RawMessage"
    else:
        kinds = schema.get("type")
        nullable = isinstance(kinds, list) and "null" in kinds
        kind = next((item for item in kinds if item != "null"), "object") if isinstance(kinds, list) else kinds
        if kind == "string":
            base = "string"
        elif kind == "integer":
            base = "int64"
        elif kind == "number":
            base = "float64"
        elif kind == "boolean":
            base = "bool"
        elif kind == "array":
            base = "[]" + go_type(schema.get("items", {}), True).lstrip("*")
        elif kind == "object" and schema.get("properties"):
            base = "json.RawMessage"
        else:
            base = "json.RawMessage"
        if nullable:
            return "*" + base
    if required:
        return base
    if base.startswith("[]") or base == "json.RawMessage":
        return base
    return "*" + base


def generate_go(schemas: dict[str, Any]) -> str:
    lines = [
        "// Code generated by scripts/generate-contracts.py; DO NOT EDIT.",
        "package nexuscontracts",
        "",
        'import "encoding/json"',
        "",
    ]
    for name, schema in schemas.items():
        lines.append(f"// {name} {schema.get('description', 'Nexus contract type.')}")
        if schema.get("type") != "object" or not schema.get("properties"):
            lines.append(f"type {name} json.RawMessage")
            lines.append("")
            continue
        lines.append(f"type {name} struct {{")
        required = set(schema.get("required", []))
        for prop_name, prop in schema["properties"].items():
            field = snake_to_camel(prop_name)
            description = prop.get("description", "契约字段。")
            lines.append(f"\t// {field} {description}")
            tag = prop_name if prop_name in required else prop_name + ",omitempty"
            lines.append(f"\t{field} {go_type(prop, prop_name in required)} `json:\"{tag}\"`")
        lines.append("}")
        lines.append("")
    return "\n".join(lines)


CLIENT_GO = r'''// Code generated by scripts/generate-contracts.py; DO NOT EDIT.
package nexuscontracts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client is the generated Nexus v1 HTTP client.
type Client struct {
	BaseURL    *url.URL
	HTTPClient *http.Client
	Token      string
}

// NewClient creates a Nexus client for baseURL.
func NewClient(baseURL string, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return nil, fmt.Errorf("parse Nexus base URL: %w", err)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{BaseURL: parsed, HTTPClient: httpClient}, nil
}

func (c *Client) doJSON(ctx context.Context, method, path, idempotencyKey string, requestBody, responseBody any) error {
	_, err := c.doJSONStatus(ctx, method, path, idempotencyKey, requestBody, responseBody)
	return err
}

func (c *Client) doJSONStatus(ctx context.Context, method, path, idempotencyKey string, requestBody, responseBody any) (int, error) {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return 0, fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	endpoint, err := c.BaseURL.Parse(strings.TrimLeft(path, "/"))
	if err != nil {
		return 0, fmt.Errorf("resolve endpoint: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiError ErrorResponse
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&apiError); err != nil {
			return resp.StatusCode, fmt.Errorf("Nexus returned HTTP %d", resp.StatusCode)
		}
		return resp.StatusCode, fmt.Errorf("Nexus %s: %s", apiError.Code, apiError.Message)
	}
	if responseBody == nil || resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(responseBody); err != nil {
		return resp.StatusCode, fmt.Errorf("decode response: %w", err)
	}
	return resp.StatusCode, nil
}

func (c *Client) GetBackupStatus(ctx context.Context) (BackupStatus, error) {
	var response BackupStatus
	err := c.doJSON(ctx, http.MethodGet, "/v1/backup/status", "", nil, &response)
	return response, err
}
'''


def write_json(path: pathlib.Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main() -> None:
    schemas = build_schemas()
    jsonschema_dir = CONTRACTS / "jsonschema"
    jsonschema_dir.mkdir(parents=True, exist_ok=True)
    for stale in jsonschema_dir.glob("*.json"):
        stale.unlink()
    events_dir = CONTRACTS / "events"
    if events_dir.exists():
        for stale in events_dir.glob("*.json"):
            stale.unlink()
        events_dir.rmdir()
    compatibility_dir = CONTRACTS / "compatibility"
    if compatibility_dir.exists():
        for stale in compatibility_dir.glob("*.json"):
            stale.unlink()
        compatibility_dir.rmdir()

    write_json(CONTRACTS / "openapi" / "nexus.yaml", build_openapi(schemas))
    for name, schema in schemas.items():
        standalone = rewrite_refs(
            {
                "$schema": "https://json-schema.org/draft/2020-12/schema",
                "$id": f"https://schemas.agentdock.dev/nexus/v1/{name}.json",
                "title": name,
                "$defs": schemas,
                **schema,
            }
        )
        write_json(jsonschema_dir / f"{name}.json", standalone)
    write_json(
        CONTRACTS / "error-codes.json",
        {"version": 1, "codes": [{"code": code, "description": "稳定公共错误码。"} for code in ERROR_CODES]},
    )
    GENERATED.mkdir(parents=True, exist_ok=True)
    (GENERATED / "types.gen.go").write_text(generate_go(schemas), encoding="utf-8")
    (GENERATED / "client.gen.go").write_text(CLIENT_GO, encoding="utf-8")
    subprocess.run(
        ["gofmt", "-w", str(GENERATED / "types.gen.go"), str(GENERATED / "client.gen.go")],
        check=True,
    )
    print(f"generated {len(schemas)} current Nexus DTO schemas and Go client")


if __name__ == "__main__":
    main()
