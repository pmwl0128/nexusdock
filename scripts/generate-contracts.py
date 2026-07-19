#!/usr/bin/env python3
"""Generate Nexus OpenAPI, JSON Schemas, event schemas and Go client DTOs.

The generator intentionally uses only the Python standard library so every
AgentDock development host can reproduce the checked-in output.
"""

from __future__ import annotations

import json
import pathlib
import re
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
    'ADMIN_NOT_INITIALIZED',
    'AGENTDOCK_NODE_CREDENTIALS_UNAVAILABLE',
    'AGENTDOCK_NODE_DISABLED',
    'AGENTDOCK_NODE_EXISTS',
    'AGENTDOCK_NODE_LIST_FAILED',
    'AGENTDOCK_NODE_NOT_FOUND',
    'AGENTDOCK_NODE_OPERATION_FAILED',
    'AGENTDOCK_NODE_STORE_UNAVAILABLE',
    'AGENTDOCK_RUNTIME_BAD_RESPONSE',
    'AGENTDOCK_RUNTIME_REQUEST_FAILED',
    'AGENTDOCK_RUNTIME_UNAVAILABLE',
    'AGENTDOCK_RUNTIME_UNREACHABLE',
    'APPEND_NOTE_FAILED',
    'AUTH_STATUS_FAILED',
    'CAPTURE_CARD_FAILED',
    'CONFIRMATION_REQUIRED',
    'CREDENTIAL_POLICY_FAILED',
    'CREDENTIAL_UPDATE_FAILED',
    'CREDENTIAL_UPDATE_REQUIRED',
    'CSRF_REJECTED',
    'CURRENT_CREDENTIAL_INVALID',
    'DELETE_FAILED',
    'EMBEDDING_DISABLED',
    'EMBEDDING_REINDEX_FAILED',
    'EMBEDDING_SEARCH_FAILED',
    'GIT_COMMIT_FAILED',
    'GIT_DIFF_FAILED',
    'GIT_DISCARD_FAILED',
    'GIT_LOG_FAILED',
    'HTTPS_REQUIRED',
    'INTERNAL_ERROR',
    'INVALID_AGENTDOCK_NODE',
    'INVALID_CREDENTIALS',
    'INVALID_JSON',
    'INVALID_MCP_ACTION',
    'INVALID_MCP_NAME',
    'INVALID_PATH',
    'INVALID_PRIVATE_NOTE_MAINTENANCE_ACTION',
    'INVALID_PRIVATE_NOTE_PATH',
    'INVALID_PRIVATE_NOTE_STATUS_ACTION',
    'INVALID_QUERY',
    'INVALID_SKILL_FILE',
    'INVALID_SKILL_ID',
    'INVALID_TASK_ID',
    'INVALID_WORKFLOW_TEMPLATE',
    'LIST_CARDS_FAILED',
    'LIST_FAILED',
    'LOGIN_FAILED',
    'LOGIN_RATE_LIMITED',
    'LOGOUT_FAILED',
    'MISSING_CONTENT',
    'MISSING_PATH',
    'MISSING_QUERY',
    'MOVE_FAILED',
    'ORIGIN_REJECTED',
    'PACK_FAILED',
    'PATCH_FAILED',
    'PRIVATE_NOTES_AGE_IDENTITY_INVALID',
    'PRIVATE_NOTES_AGE_RECIPIENT_INVALID',
    'PRIVATE_NOTES_AGE_RECIPIENT_MISSING',
    'PRIVATE_NOTES_ROOT_REQUIRED',
    'PRIVATE_NOTE_ENCRYPTED_MISSING',
    'PRIVATE_NOTE_EXISTS',
    'PRIVATE_NOTE_METADATA_INVALID',
    'PRIVATE_NOTE_METADATA_TOO_LARGE',
    'PRIVATE_NOTE_NOT_FOUND',
    'PRIVATE_NOTE_OPERATION_FAILED',
    'PRIVATE_NOTE_SYMLINK_REJECTED',
    'PRIVATE_NOTE_UNSAFE_FILE',
    'READ_FAILED',
    'REQUEST_TOO_LARGE',
    'SEARCH_CARDS_FAILED',
    'SEARCH_FAILED',
    'SESSION_LIST_FAILED',
    'SESSION_NOT_FOUND',
    'SESSION_REQUIRED',
    'SESSION_REVOKE_FAILED',
    'SYNC_FAILED',
    'SYNC_PULL_FAILED',
    'SYNC_PUSH_FAILED',
    'UNAUTHORIZED',
    'USE_LOGOUT',
    'WORKFLOW_ACTION_NOT_FOUND',
    'WORKFLOW_LIST_FAILED',
    'WORKFLOW_MATCH_FAILED',
    'WORKFLOW_PUBLISH_FAILED',
    'WORKFLOW_REGISTRY_FAILED',
    'WORKFLOW_REINDEX_FAILED',
    'WORKFLOW_RETIRE_FAILED',
    'WORKFLOW_RETIRE_OLD_FAILED',
    'WORKFLOW_SAVE_FAILED',
    'WORKFLOW_TEMPLATE_NOT_ACTIVE',
    'WORKFLOW_TEMPLATE_NOT_FOUND',
    'WORKFLOW_VECTOR_INDEX_INVALID',
    'WORKFLOW_VERSION_IMMUTABLE',
    'WRITE_CARD_FAILED',
    'WRITE_FAILED',
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
            "request_id": scalar("string", "请求关联 ID。"),
            "error": obj(
                "错误详情。",
                {
                    "code": scalar("string", "稳定错误码。"),
                    "message": scalar("string", "可读错误说明。"),
                },
                ("code", "message"),
            ),
        },
        ("ok", "request_id", "error"),
    )
    schemas["RuntimeErrorEnvelope"] = obj(
        "AgentDock Runtime 不可用或拒绝请求时的错误信封。",
        {
            "ok": scalar("boolean", "固定为 false。"),
            "available": scalar("boolean", "固定为 false，表示目标 Runtime 当前不可用。"),
            "source": scalar("string", "错误来源，固定为 agentdock-runtime-api。"),
            "request_id": scalar("string", "请求关联 ID。"),
            "error": obj(
                "稳定 Nexus 错误及可选的上游错误码。",
                {
                    "code": scalar("string", "Nexus 稳定错误码。"),
                    "message": scalar("string", "可读错误说明。"),
                    "upstream_code": scalar("string", "AgentDock Runtime 返回的原始错误码。"),
                },
                ("code", "message"),
            ),
        },
        ("ok", "available", "source", "request_id", "error"),
    )
    schemas["OperationOK"] = obj(
        "无附加数据的成功响应。",
        {"ok": scalar("boolean", "固定为 true。")},
        ("ok",),
    )
    schemas["AuthStatusResponse"] = obj(
        "管理员认证初始化状态。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "initialized": scalar("boolean", "管理员凭据是否已初始化。"),
        },
        ("ok", "initialized"),
    )
    schemas["AuthLoginRequest"] = obj(
        "管理员登录凭据。",
        {
            "username": scalar("string", "管理员用户名。", minLength=1),
            "password": scalar("string", "管理员密码。", minLength=1, maxLength=1024),
            "remember_me": scalar("boolean", "是否创建最长 30 天的记住登录会话。"),
        },
        ("username", "password"),
    )
    schemas["WebSession"] = obj(
        "已脱敏的管理员浏览器会话。",
        {
            "id": scalar("string", "会话 ID。"),
            "user_id": scalar("string", "管理员用户 ID。"),
            "username": scalar("string", "管理员用户名。"),
            "display_name": scalar("string", "管理员显示名称。"),
            "remember_me": scalar("boolean", "是否为记住登录会话。"),
            "ip_prefix": scalar("string", "脱敏后的客户端网络前缀。"),
            "user_agent_summary": scalar("string", "脱敏后的客户端摘要。"),
            "created_at": TIMESTAMP,
            "last_seen_at": TIMESTAMP,
            "idle_expires_at": TIMESTAMP,
            "absolute_expires_at": TIMESTAMP,
            "must_change_password": scalar("boolean", "是否必须先更新管理员密码。"),
            "csrf_token": scalar("string", "仅当前会话返回的 CSRF Token。"),
            "current": scalar("boolean", "是否为当前浏览器会话。"),
        },
        (
            "id", "user_id", "username", "display_name", "remember_me", "ip_prefix",
            "user_agent_summary", "created_at", "last_seen_at", "idle_expires_at",
            "absolute_expires_at", "must_change_password",
        ),
    )
    schemas["WebSessionResponse"] = obj(
        "当前或新创建的管理员浏览器会话。",
        {"ok": scalar("boolean", "请求是否成功。"), "session": ref("WebSession")},
        ("ok", "session"),
    )
    schemas["WebSessionListResponse"] = obj(
        "管理员活动浏览器会话列表。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "items": array("活动浏览器会话。", ref("WebSession")),
        },
        ("ok", "items"),
    )
    schemas["AuthCredentialUpdateRequest"] = obj(
        "管理员密码更新请求。",
        {
            "current": scalar("string", "当前密码。", minLength=1, maxLength=1024),
            "next": scalar("string", "符合策略的新密码。", minLength=12, maxLength=1024),
        },
        ("current", "next"),
    )
    schemas["AuthCredentialUpdateResponse"] = obj(
        "管理员密码更新结果。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "reauthenticate": scalar("boolean", "是否必须重新登录。"),
        },
        ("ok", "reauthenticate"),
    )
    schemas["WebSessionRevokeOthersResponse"] = obj(
        "撤销其他浏览器会话的结果。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "revoked": scalar("integer", "已撤销的会话数量。", minimum=0),
        },
        ("ok", "revoked"),
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
    schemas["RecallFileEntry"] = obj(
        "Recall 仓库中的文件或目录条目。",
        {
            "path": scalar("string", "Recall 相对路径。"),
            "name": scalar("string", "文件或目录名。"),
            "type": enum("条目类型。", ["file", "directory"]),
            "size_bytes": scalar("integer", "条目字节数。", minimum=0),
            "modified": TIMESTAMP,
        },
        ("path", "name", "type"),
    )
    schemas["RecallRecord"] = obj(
        "读取或写入后的完整 Recall 文本记录。",
        {
            "path": scalar("string", "Recall 相对路径。"),
            "content": scalar("string", "包含 Frontmatter 的完整文本。"),
            "body": scalar("string", "移除 Frontmatter 后的正文。"),
            "frontmatter": obj(
                "解析后的 Frontmatter 字符串字段。",
                {},
                additional=scalar("string", "Frontmatter 字段值。"),
            ),
            "size_bytes": scalar("integer", "文本字节数。", minimum=0),
        },
        ("path", "content", "body", "frontmatter", "size_bytes"),
    )
    schemas["RecallWriteRequest"] = obj(
        "创建或覆盖 Recall 文本记录。",
        {
            "path": scalar("string", "Recall 相对路径；PATCH 时由路径参数覆盖。", minLength=1),
            "content": scalar("string", "Markdown 或文本内容。"),
            "type": scalar("string", "可选的 Recall 类型。"),
            "scope": enum("Recall 作用域。", ["profile", "global", "project", "device", "agent", "ops", "notes", "inbox"]),
            "status": enum("Recall 状态。", ["inbox", "active", "verified", "stale", "archived", "rejected", "conflicted", "unverified", "deprecated"]),
            "project": scalar("string", "项目标识。"),
            "device": scalar("string", "设备标识。"),
            "agent": scalar("string", "Agent 标识。"),
            "skill": scalar("string", "Skill 标识。"),
            "source": scalar("string", "信息来源。"),
            "confidence": enum("可信度。", ["unknown", "low", "medium", "high"]),
            "verified_at": TIMESTAMP,
            "verification_run_id": scalar("string", "验证运行 ID。"),
            "source_device": scalar("string", "验证来源设备。"),
            "source_agent": scalar("string", "验证来源 Agent。"),
            "tags": array("Recall 标签。", scalar("string", "标签。")),
            "confirmed": scalar("boolean", "写入受保护目录时的确认标记。"),
            "overwrite": scalar("boolean", "是否覆盖已有文件。"),
        },
        ("content",),
    )
    schemas["RecallRecordResponse"] = obj(
        "Recall 读取或写入结果。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "recall": ref("RecallRecord"),
        },
        ("ok", "recall"),
    )
    schemas["RecallSearchResult"] = obj(
        "Recall 关键词搜索命中。",
        {
            "path": scalar("string", "Recall 相对路径。"),
            "title": scalar("string", "Markdown 标题。"),
            "snippet": scalar("string", "命中位置附近的文本片段。"),
            "frontmatter": obj(
                "命中文档 Frontmatter。",
                {},
                additional=scalar("string", "Frontmatter 字段值。"),
            ),
            "matched_terms": array("命中的查询词。", scalar("string", "查询词。")),
            "matched_fields": array("命中的文档字段。", scalar("string", "字段名。")),
        },
        ("path", "snippet", "frontmatter"),
    )
    schemas["RecallCardRequest"] = obj(
        "捕获或写入一张可复用 Recall 卡片。",
        {
            "title": scalar("string", "卡片标题。", minLength=1),
            "content": scalar("string", "卡片正文。"),
            "summary": scalar("string", "content 为空时使用的摘要正文。"),
            "type": enum("卡片类型。", ["preference", "runbook", "bug_pattern", "deploy_note", "project_trap", "architecture", "decision", "anti_pattern"]),
            "scope": enum("卡片作用域。", ["global", "project", "device"]),
            "project": scalar("string", "项目标识；为空时使用 global。"),
            "status": enum("卡片状态。", ["inbox", "active", "verified", "stale", "archived", "rejected", "conflicted", "unverified", "deprecated"]),
            "confidence": enum("卡片可信度。", ["unknown", "low", "medium", "high"]),
            "tags": array("卡片标签。", scalar("string", "标签。")),
            "source": scalar("string", "卡片信息来源。"),
            "evidence": scalar("string", "验证卡片内容的证据。"),
            "boundary": scalar("string", "卡片适用边界。"),
            "path": scalar("string", "可选的 recall/managed/cards/ 自定义路径。"),
            "confirmed": scalar("boolean", "真实写入时必须为 true。"),
            "overwrite": scalar("boolean", "是否覆盖同路径卡片。"),
            "allow_warnings": scalar("boolean", "是否在已审阅后接受规范警告。"),
            "max_results": scalar("integer", "捕获阶段相似项最大数量。", minimum=1, maximum=50),
        },
        ("title",),
    )
    schemas["RecallCard"] = obj(
        "规范化后的 Recall 卡片。",
        {
            "title": scalar("string", "卡片标题。"),
            "content": scalar("string", "卡片正文。"),
            "type": scalar("string", "卡片类型。"),
            "scope": scalar("string", "卡片作用域。"),
            "project": scalar("string", "项目标识。"),
            "status": scalar("string", "卡片状态。"),
            "confidence": scalar("string", "卡片可信度。"),
            "tags": array("规范化标签。", scalar("string", "标签。")),
            "source": scalar("string", "信息来源。"),
            "evidence": scalar("string", "验证证据。"),
            "boundary": scalar("string", "适用边界。"),
            "path": scalar("string", "最终 Recall 相对路径。"),
        },
        ("title", "content", "type", "scope", "project", "status", "confidence", "source", "path"),
    )
    schemas["RecallCardCaptureResponse"] = obj(
        "卡片写入前的规范化、风险提示和去重计划。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "card": ref("RecallCard"),
            "warnings": array("需人工审阅的规范警告。", scalar("string", "警告。")),
            "capture_plan": ref("JsonObject"),
            "similar_results": array("关键词相似的已有卡片。", ref("RecallSearchResult")),
            "similar_count": scalar("integer", "相似卡片数量。", minimum=0),
        },
        ("ok", "card", "capture_plan", "similar_count"),
    )
    schemas["RecallCardWriteResponse"] = obj(
        "卡片及其 Recall 文件写入结果。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "card": ref("RecallCard"),
            "warnings": array("已接受的规范警告。", scalar("string", "警告。")),
            "recall": ref("RecallRecord"),
            "index_policy": scalar("string", "卡片索引策略说明。"),
        },
        ("ok", "card", "recall", "index_policy"),
    )
    schemas["RecallCardListResponse"] = obj(
        "Recall 卡片文件列表。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "entries": array("卡片目录下的文件和目录。", ref("RecallFileEntry")),
            "count": scalar("integer", "条目数量。", minimum=0),
            "prefix": scalar("string", "固定为 recall/managed/cards。"),
        },
        ("ok", "entries", "count", "prefix"),
    )
    schemas["RecallCardSearchRequest"] = obj(
        "在 Recall 卡片目录中执行关键词搜索。",
        {
            "query": scalar("string", "关键词查询。", minLength=1),
            "max_results": scalar("integer", "最大结果数。", minimum=1, maximum=200),
        },
        ("query",),
    )
    schemas["RecallCardSearchResponse"] = obj(
        "Recall 卡片关键词搜索结果。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "query": scalar("string", "原始查询。"),
            "results": array("搜索命中。", ref("RecallSearchResult")),
            "count": scalar("integer", "结果数量。", minimum=0),
            "prefix": scalar("string", "固定为 recall/managed/cards。"),
        },
        ("ok", "query", "results", "count", "prefix"),
    )
    schemas["EmbeddingIndexSummary"] = obj(
        "Recall 向量索引摘要。",
        {
            "model": scalar("string", "索引使用的嵌入模型。"),
            "dimension": scalar("integer", "向量维度。", minimum=0),
            "count": scalar("integer", "索引文档数量。", minimum=0),
            "updated_at": TIMESTAMP,
        },
        ("model", "count", "updated_at"),
    )
    schemas["EmbeddingStatusResponse"] = obj(
        "Recall 嵌入服务及索引状态。",
        {
            "ok": scalar("boolean", "状态查询是否成功。"),
            "enabled": scalar("boolean", "嵌入服务是否启用。"),
            "configured": scalar("boolean", "嵌入端点是否配置。"),
            "model": scalar("string", "当前嵌入模型。"),
            "endpoint": scalar("string", "当前嵌入端点。"),
            "index_path": scalar("string", "本地索引文件路径。"),
            "index": ref("EmbeddingIndexSummary"),
            "reachable": scalar("boolean", "嵌入端点是否可达。"),
            "reason": scalar("string", "未启用原因。"),
            "error": scalar("string", "最近一次探测错误。"),
        },
        ("ok", "enabled", "configured"),
    )
    schemas["EmbeddingReindexRequest"] = obj(
        "重建 Recall 向量索引。",
        {
            "prefix": scalar("string", "要索引的 Recall 路径前缀。"),
            "max_entries": scalar("integer", "最多索引条目数。", minimum=1, maximum=2000),
        },
    )
    schemas["EmbeddingReindexResponse"] = obj(
        "Recall 向量索引重建结果。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "enabled": scalar("boolean", "嵌入服务是否启用。"),
            "model": scalar("string", "索引使用的嵌入模型。"),
            "endpoint": scalar("string", "嵌入端点。"),
            "index_path": scalar("string", "索引文件路径。"),
            "prefix": scalar("string", "已索引路径前缀。"),
            "count": scalar("integer", "索引文档数量。", minimum=0),
            "dimension": scalar("integer", "向量维度。", minimum=0),
            "updated_at": TIMESTAMP,
        },
        ("ok", "enabled", "model", "index_path", "prefix", "count", "updated_at"),
    )
    schemas["EmbeddingSearchRequest"] = obj(
        "使用 Recall 向量索引执行语义搜索。",
        {
            "query": scalar("string", "语义查询。", minLength=1),
            "prefix": scalar("string", "可选的 Recall 路径前缀。"),
            "max_results": scalar("integer", "最大结果数。", minimum=1, maximum=50),
        },
        ("query",),
    )
    schemas["EmbeddingSearchHit"] = obj(
        "Recall 向量搜索命中。",
        {
            "path": scalar("string", "Recall 相对路径。"),
            "title": scalar("string", "Markdown 标题。"),
            "score": scalar("number", "余弦相似度。", minimum=-1, maximum=1),
            "snippet": scalar("string", "文档文本片段。"),
            "frontmatter": obj(
                "命中文档 Frontmatter。",
                {},
                additional=scalar("string", "Frontmatter 字段值。"),
            ),
        },
        ("path", "score", "snippet"),
    )
    schemas["EmbeddingSearchResponse"] = obj(
        "Recall 向量搜索结果。",
        {
            "ok": scalar("boolean", "请求是否成功。"),
            "enabled": scalar("boolean", "嵌入服务是否启用。"),
            "model": scalar("string", "查询使用的嵌入模型。"),
            "query": scalar("string", "原始语义查询。"),
            "results": array("按相似度降序排列的命中。", ref("EmbeddingSearchHit")),
            "count": scalar("integer", "返回命中数量。", minimum=0),
            "index": ref("EmbeddingIndexSummary"),
        },
        ("ok", "enabled", "model", "query", "results", "count", "index"),
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
    error = response({"oneOf": [ref("ErrorResponse"), ref("LegacyErrorEnvelope"), ref("RuntimeErrorEnvelope")]}, "错误。")
    generic = ref("JsonObject")

    def path_param(name: str, description: str, *, uuid: bool = True) -> dict[str, Any]:
        schema: dict[str, Any] = {"type": "string"}
        if uuid:
            schema["format"] = "uuid"
        return {"name": name, "in": "path", "required": True, "description": description, "schema": schema}

    def query_param(
        name: str,
        description: str,
        kind: str = "string",
        *,
        required: bool = False,
        **schema_options: Any,
    ) -> dict[str, Any]:
        return {
            "name": name,
            "in": "query",
            "required": required,
            "description": description,
            "schema": {"type": kind, **schema_options},
        }

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
    q = query_param
    no_content = {"description": "已接受，无响应体。"}
    paths: dict[str, Any] = {
        "/health": {"get": operation("getHealth", "读取服务健康状态", success=ok(ref("HealthResponse")))},
        "/v1/system/status": {"get": operation("getSystemStatus", "读取 Nexus 与 SQLite 状态", success=ok(ref("SystemStatus")))},
        "/v1/backup/status": {"get": operation("getBackupStatus", "读取 AgentDock 与 Nexus 备份状态", success=ok(ref("BackupStatus")))},
        "/v1/auth/status": {
            "get": operation("getAuthStatus", "读取管理员初始化状态", success=ok(ref("AuthStatusResponse")))
        },
        "/v1/auth/login": {
            "post": operation(
                "login",
                "登录管理员会话",
                request=body(ref("AuthLoginRequest")),
                success=ok(ref("WebSessionResponse")),
            )
        },
        "/v1/auth/session": {
            "get": operation("getCurrentSession", "读取当前浏览器会话", success=ok(ref("WebSessionResponse")))
        },
        "/v1/auth/logout": {
            "post": operation("logout", "退出当前浏览器会话", success=ok(ref("OperationOK")))
        },
        "/v1/auth/credential": {
            "post": operation(
                "updateCredential",
                "更新管理员凭据",
                request=body(ref("AuthCredentialUpdateRequest")),
                success=ok(ref("AuthCredentialUpdateResponse")),
            )
        },
        "/v1/auth/sessions": {
            "get": operation("listSessions", "列出管理员浏览器会话", success=ok(ref("WebSessionListResponse")))
        },
        "/v1/auth/sessions/{sessionID}": {
            "delete": operation(
                "revokeSession",
                "撤销指定浏览器会话",
                params=[p("SessionId")],
                success=ok(ref("OperationOK")),
            )
        },
        "/v1/auth/sessions/logout-others": {
            "post": operation(
                "logoutOtherSessions",
                "撤销其他浏览器会话",
                success=ok(ref("WebSessionRevokeOthersResponse")),
            )
        },
        "/v1/recall": {
            "get": operation(
                "listRecall",
                "列出召回条目",
                params=[
                    q("prefix", "只列出该 Recall 相对路径前缀下的条目。"),
                    q("max_entries", "最大条目数；无效值使用服务默认值。", "integer", minimum=1),
                ],
            ),
            "post": operation(
                "writeRecall",
                "创建召回条目",
                request=body(ref("RecallWriteRequest")),
                success=ok(ref("RecallRecordResponse")),
            ),
        },
        "/v1/recall/move": {"post": operation("moveRecall", "移动召回条目", request=body())},
        "/v1/recall/search": {"post": operation("searchRecall", "搜索召回内容", request=body())},
        "/v1/recall/pack": {"post": operation("packRecall", "打包召回条目", request=body())},
        "/v1/recall/notes/append": {"post": operation("appendRecallNote", "追加召回笔记", request=body())},
        "/v1/recall/cards": {
            "get": operation(
                "listRecallCards",
                "列出 Recall 卡片",
                params=[q("max_entries", "最大卡片条目数；无效值使用服务默认值。", "integer", minimum=1)],
                success=ok(ref("RecallCardListResponse")),
            ),
            "post": operation(
                "writeRecallCard",
                "确认并写入 Recall 卡片",
                request=body(ref("RecallCardRequest")),
                success=ok(ref("RecallCardWriteResponse")),
            ),
        },
        "/v1/recall/cards/capture": {
            "post": operation(
                "captureRecallCard",
                "规范化 Recall 卡片并生成写入前审阅计划",
                request=body(ref("RecallCardRequest")),
                success=ok(ref("RecallCardCaptureResponse")),
            )
        },
        "/v1/recall/cards/search": {
            "post": operation(
                "searchRecallCards",
                "在 Recall 卡片目录执行关键词搜索",
                request=body(ref("RecallCardSearchRequest")),
                success=ok(ref("RecallCardSearchResponse")),
            )
        },
        "/v1/embeddings/status": {
            "get": operation("getEmbeddingStatus", "读取 Recall 嵌入服务和索引状态", success=ok(ref("EmbeddingStatusResponse")))
        },
        "/v1/embeddings/reindex": {
            "post": operation(
                "reindexEmbeddings",
                "重建 Recall 向量索引",
                request=body(ref("EmbeddingReindexRequest")),
                success=ok(ref("EmbeddingReindexResponse")),
            )
        },
        "/v1/embeddings/search": {
            "post": operation(
                "searchEmbeddings",
                "使用 Recall 向量索引执行语义搜索",
                request=body(ref("EmbeddingSearchRequest")),
                success=ok(ref("EmbeddingSearchResponse")),
            )
        },
        "/v1/recall/{path}": {
            "get": operation("readRecall", "读取召回条目", params=[p("RecallPath")], success=ok(ref("RecallRecordResponse"))),
            "patch": operation(
                "patchRecall",
                "修改召回条目",
                params=[p("RecallPath")],
                request=body(ref("RecallWriteRequest")),
                success=ok(ref("RecallRecordResponse")),
            ),
            "delete": operation(
                "deleteRecall",
                "删除召回条目",
                params=[p("RecallPath"), q("confirmed", "破坏性删除确认标记。", "boolean")],
            ),
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
        "/v1/git/log": {
            "get": operation(
                "getGitLog",
                "读取召回仓库提交历史",
                params=[q("limit", "最大提交数量；无效值使用服务默认值。", "integer", minimum=1)],
            )
        },
        "/v1/git/commit": {
            "get": operation(
                "getGitCommit",
                "读取召回仓库提交详情",
                params=[q("hash", "Git 提交哈希。", required=True, minLength=1)],
            )
        },
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
        "/v1/runtime/nodes/{nodeID}/tasks": {
            "get": operation(
                "listRuntimeTasks",
                "列出指定 AgentDock 节点的任务",
                params=[
                    p("RuntimeNodeId"),
                    q("status", "按任务状态过滤；all 表示不过滤。"),
                    q("q", "在任务 ID、标题、目标、状态和摘要中搜索。"),
                    q("limit", "最大任务数。", "integer", minimum=1, maximum=200),
                ],
            )
        },
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
        "/v1/workflow-templates": {
            "get": operation(
                "listWorkflowTemplates",
                "列出 Nexus Workflow 模板",
                params=[
                    q("status", "按模板状态过滤。", enum=["draft", "active", "retired"]),
                    q("q", "在模板摘要中搜索。"),
                    q("include_history", "未指定状态时是否返回全部历史版本。", "boolean"),
                    q("view", "history 等价于 include_history=true。", enum=["history"]),
                ],
            )
        },
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


def referenced_schema_names(value: Any) -> set[str]:
    names: set[str] = set()
    if isinstance(value, dict):
        reference = value.get("$ref")
        prefix = "#/components/schemas/"
        if isinstance(reference, str) and reference.startswith(prefix):
            names.add(reference[len(prefix):])
        for item in value.values():
            names.update(referenced_schema_names(item))
    elif isinstance(value, list):
        for item in value:
            names.update(referenced_schema_names(item))
    return names


def standalone_definitions(schema: dict[str, Any], schemas: dict[str, Any]) -> dict[str, Any]:
    """Include only definitions reachable from one standalone schema."""
    pending = list(referenced_schema_names(schema))
    selected: dict[str, Any] = {}
    while pending:
        name = pending.pop()
        if name in selected or name not in schemas:
            continue
        definition = schemas[name]
        selected[name] = rewrite_refs(definition)
        pending.extend(referenced_schema_names(definition) - selected.keys())
    return {name: selected[name] for name in sorted(selected)}


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


CLIENT_HEADER = r'''// Code generated by scripts/generate-contracts.py; DO NOT EDIT.
package nexuscontracts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	maxClientErrorBodyBytes    = 1 << 20
	maxClientResponseBodyBytes = 16 << 20
)

// Client is the generated Nexus v1 HTTP client.
type Client struct {
	BaseURL    *url.URL
	HTTPClient *http.Client
	Token      string
}

// APIError is a non-2xx Nexus response.
type APIError struct {
	StatusCode   int
	Code         string
	Message      string
	RequestID    string
	UpstreamCode string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("Nexus returned HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("Nexus %s: %s", e.Code, e.Message)
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

func (c *Client) doJSON(ctx context.Context, method, path string, requestBody, responseBody any) error {
	_, err := c.doJSONStatus(ctx, method, path, requestBody, responseBody)
	return err
}

func (c *Client) doJSONStatus(ctx context.Context, method, path string, requestBody, responseBody any) (int, error) {
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
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, err := readClientBody(resp.Body, maxClientErrorBodyBytes)
		if err != nil {
			return resp.StatusCode, fmt.Errorf("read Nexus error response: %w", err)
		}
		return resp.StatusCode, decodeAPIError(resp.StatusCode, data)
	}
	if responseBody == nil || resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil
	}
	data, err := readClientBody(resp.Body, maxClientResponseBodyBytes)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("read Nexus response: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return resp.StatusCode, errorsUnexpectedEmptyResponse(method, path)
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		return resp.StatusCode, fmt.Errorf("decode response: %w", err)
	}
	return resp.StatusCode, nil
}

func readClientBody(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response body exceeds %d bytes", limit)
	}
	return data, nil
}

func decodeAPIError(statusCode int, data []byte) error {
	var envelope struct {
		Code      string          `json:"code"`
		Message   string          `json:"message"`
		RequestID string          `json:"request_id"`
		Error     json.RawMessage `json:"error"`
	}
	upstreamCode := ""
	if err := json.Unmarshal(data, &envelope); err == nil && len(envelope.Error) > 0 {
		var detail struct {
			Code         string `json:"code"`
			Message      string `json:"message"`
			UpstreamCode string `json:"upstream_code"`
		}
		if err := json.Unmarshal(envelope.Error, &detail); err == nil {
			if envelope.Code == "" {
				envelope.Code = detail.Code
			}
			if envelope.Message == "" {
				envelope.Message = detail.Message
			}
			upstreamCode = detail.UpstreamCode
		} else {
			var legacyMessage string
			if json.Unmarshal(envelope.Error, &legacyMessage) == nil && envelope.Message == "" {
				envelope.Message = legacyMessage
			}
		}
	}
	if envelope.Message == "" {
		envelope.Message = http.StatusText(statusCode)
	}
	return &APIError{StatusCode: statusCode, Code: envelope.Code, Message: envelope.Message, RequestID: envelope.RequestID, UpstreamCode: upstreamCode}
}

func errorsUnexpectedEmptyResponse(method, path string) error {
	return fmt.Errorf("Nexus returned an empty response for %s %s", method, path)
}

func escapePathParameter(value string, preserveSlash bool) string {
	if !preserveSlash {
		return url.PathEscape(value)
	}
	parts := strings.Split(strings.Trim(value, "/"), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}
'''


def schema_go_client_type(schema: dict[str, Any] | None, *, request: bool) -> str | None:
    if not schema:
        return None
    reference = schema.get("$ref")
    if isinstance(reference, str):
        name = reference.rsplit("/", 1)[-1]
        if name == "JsonObject":
            return "any" if request else "json.RawMessage"
        return name
    return "any" if request else "json.RawMessage"


def operation_content_schema(container: dict[str, Any] | None) -> dict[str, Any] | None:
    if not container:
        return None
    content = container.get("content", {})
    media = content.get("application/json", {})
    schema = media.get("schema")
    return schema if isinstance(schema, dict) else None


def operation_success_schema(operation: dict[str, Any]) -> dict[str, Any] | None:
    for status, response_value in operation.get("responses", {}).items():
        if isinstance(status, str) and status.isdigit() and 200 <= int(status) < 300:
            return operation_content_schema(response_value)
    return None


def go_method_name(operation_id: str) -> str:
    return operation_id[:1].upper() + operation_id[1:]


def go_parameter_name(name: str) -> str:
    camel = snake_to_camel(name)
    return camel[:1].lower() + camel[1:]


def go_query_parameter_type(parameter: dict[str, Any]) -> str:
    schema_type = parameter.get("schema", {}).get("type", "string")
    go_type = {"boolean": "bool", "integer": "int64", "string": "string"}.get(schema_type)
    if go_type is None:
        raise ValueError(f"unsupported query parameter type: {schema_type}")
    return go_type if parameter.get("required") else "*" + go_type


def go_query_parameter_value(parameter: dict[str, Any], variable: str) -> str:
    schema_type = parameter.get("schema", {}).get("type", "string")
    value = variable if parameter.get("required") else "*" + variable
    if schema_type == "boolean":
        return f"strconv.FormatBool({value})"
    if schema_type == "integer":
        return f"strconv.FormatInt({value}, 10)"
    return value


def generate_client(document: dict[str, Any]) -> str:
    lines = [CLIENT_HEADER.rstrip(), ""]
    preserve_slash = {"path", "filePath"}
    for route, path_item in document["paths"].items():
        for method, operation in path_item.items():
            operation_id = operation["operationId"]
            method_name = go_method_name(operation_id)
            path_names = re.findall(r"\{([^}]+)\}", route)
            query_parameters = [
                parameter
                for parameter in operation.get("parameters", [])
                if parameter.get("in") == "query"
            ]
            request_schema = operation_content_schema(operation.get("requestBody"))
            request_type = schema_go_client_type(request_schema, request=True)
            response_schema = operation_success_schema(operation)
            response_type = schema_go_client_type(response_schema, request=False)
            parameters = ["ctx context.Context"]
            parameters.extend(f"{name} string" for name in path_names)
            parameters.extend(
                f"{go_parameter_name(parameter['name'])} {go_query_parameter_type(parameter)}"
                for parameter in query_parameters
            )
            if request_type:
                parameters.append(f"request {request_type}")
            lines.append(f"// {method_name} {operation['summary']}。")
            if response_type:
                lines.append(f"func (c *Client) {method_name}({', '.join(parameters)}) ({response_type}, error) {{")
                lines.append(f"\tvar response {response_type}")
            else:
                lines.append(f"func (c *Client) {method_name}({', '.join(parameters)}) error {{")
            lines.append(f"\tendpointPath := {json.dumps(route)}")
            for name in path_names:
                preserve = "true" if name in preserve_slash else "false"
                lines.append(f"\tendpointPath = strings.ReplaceAll(endpointPath, \"{{{name}}}\", escapePathParameter({name}, {preserve}))")
            if query_parameters:
                lines.append("\tqueryValues := url.Values{}")
                for parameter in query_parameters:
                    name = parameter["name"]
                    variable = go_parameter_name(name)
                    value = go_query_parameter_value(parameter, variable)
                    if parameter.get("required"):
                        lines.append(f"\tqueryValues.Set({json.dumps(name)}, {value})")
                    else:
                        lines.append(f"\tif {variable} != nil {{")
                        lines.append(f"\t\tqueryValues.Set({json.dumps(name)}, {value})")
                        lines.append("\t}")
                lines.append("\tif encodedQuery := queryValues.Encode(); encodedQuery != \"\" {")
                lines.append("\t\tendpointPath += \"?\" + encodedQuery")
                lines.append("\t}")
            request_value = "request" if request_type else "nil"
            response_value = "&response" if response_type else "nil"
            call = f"c.doJSON(ctx, http.Method{method.title()}, endpointPath, {request_value}, {response_value})"
            if response_type:
                lines.append(f"\terr := {call}")
                lines.append("\treturn response, err")
            else:
                lines.append(f"\treturn {call}")
            lines.append("}")
            lines.append("")
    return "\n".join(lines)


def write_json(path: pathlib.Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main() -> None:
    schemas = build_schemas()
    openapi = build_openapi(schemas)
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

    write_json(CONTRACTS / "openapi" / "nexus.yaml", openapi)
    for name, schema in schemas.items():
        standalone = {
            "$schema": "https://json-schema.org/draft/2020-12/schema",
            "$id": f"https://schemas.agentdock.dev/nexus/v1/{name}.json",
            "title": name,
            **rewrite_refs(schema),
        }
        definitions = standalone_definitions(schema, schemas)
        if definitions:
            standalone["$defs"] = definitions
        write_json(jsonschema_dir / f"{name}.json", standalone)
    write_json(
        CONTRACTS / "error-codes.json",
        {"version": 1, "codes": [{"code": code, "description": "稳定公共错误码。"} for code in ERROR_CODES]},
    )
    GENERATED.mkdir(parents=True, exist_ok=True)
    (GENERATED / "types.gen.go").write_text(generate_go(schemas), encoding="utf-8")
    (GENERATED / "client.gen.go").write_text(generate_client(openapi), encoding="utf-8")
    subprocess.run(
        ["gofmt", "-w", str(GENERATED / "types.gen.go"), str(GENERATED / "client.gen.go")],
        check=True,
    )
    print(f"generated {len(schemas)} current Nexus DTO schemas and Go client")


if __name__ == "__main__":
    main()
