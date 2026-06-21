#!/usr/bin/env python3
"""Generate Nexus OpenAPI, JSON Schemas, event schemas and Go client DTOs.

The generator intentionally uses only the Python standard library so every
AgentDock development device can reproduce the checked-in output.
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
    "DEVICE_NOT_FOUND",
    "DEVICE_ALREADY_EXISTS",
    "DEVICE_NOT_APPROVED",
    "DEVICE_REVOKED",
    "DEVICE_TOKEN_INVALID",
    "ENROLLMENT_TOKEN_INVALID",
    "ENROLLMENT_TOKEN_EXPIRED",
    "ENROLLMENT_TOKEN_USED",
    "DEVICE_POLICY_DENIED",
    "COMMAND_NOT_FOUND",
    "COMMAND_TYPE_DENIED",
    "COMMAND_CANCELLED",
    "COMMAND_TERMINAL",
    "COMMAND_NOT_LEASEABLE",
    "LEASE_NOT_FOUND",
    "LEASE_MISMATCH",
    "INVALID_COMMAND_TRANSITION",
]

COMMAND_TYPES = [
    "health.check",
    "skill.install",
    "skill.run",
    "skill.rollback",
    "recall.sync",
    "service.inspect",
    "service.restart",
    "diagnostics.collect",
    "agentdock.reload",
    "env.manage",
    "artifact.pull",
    "artifact.fetch",
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
        "RecallDock 与浏览器接口使用的错误信封。",
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
            "service": scalar("string", "服务名称。"),
            "database": scalar("string", "SQLite 健康状态。"),
            "schema_version": scalar("integer", "数据库 Schema 版本。", minimum=0),
            "recall_root": scalar("string", "RecallDock 召回仓库路径。"),
                        "artifact_root": scalar("string", "Artifact 密文存储路径。"),
        },
        ("ok", "service", "database", "schema_version", "recall_root", "artifact_root"),
    )
    schemas["BackupHistory"] = obj(
        "一次备份执行的脱敏结果。",
        {
            "schema_version": scalar("integer", "状态文件版本。", minimum=0),
            "state": enum("备份状态。", ["never_run", "queued", "running", "success", "failed", "unknown", "disabled"]),
            "message": scalar("string", "状态说明。"),
            "started_at": TIMESTAMP,
            "completed_at": TIMESTAMP,
            "host": scalar("string", "执行设备。"),
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
            "device": scalar("string", "执行设备。"),
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
        ("id", "title", "provider", "device", "enabled", "schedule", "state", "next_run_at", "history"),
    )
    schemas["DeviceCapability"] = obj(
        "设备能力声明。",
        {
            "name": scalar("string", "稳定能力名。"),
            "version": scalar("string", "能力版本。"),
            "enabled": scalar("boolean", "当前是否启用。"),
            "metadata": obj("非敏感扩展元数据。", {}, additional=True),
        },
        ("name", "version", "enabled"),
    )
    schemas["DeviceEnrollmentRequest"] = obj(
        "设备注册请求。",
        {
            "enrollment_token": scalar("string", "一次性注册 token。", minLength=16),
            "name": scalar("string", "设备显示名称。"),
            "platform": enum("平台。", ["darwin", "linux"]),
            "arch": enum("架构。", ["arm64", "amd64"]),
            "agentdock_version": scalar("string", "AgentDock 版本。"),
            "public_key": scalar("string", "设备公钥。"),
            "labels": obj("设备标签。", {}, additional={"type": "string"}),
        },
        ("enrollment_token", "name", "platform", "arch", "agentdock_version", "public_key"),
    )
    schemas["DeviceEnrollmentResponse"] = obj(
        "设备注册结果。",
        {
            "device_id": ID,
            "device_token": scalar("string", "设备认证 token；仅返回一次。"),
            "token_expires_at": TIMESTAMP,
            "heartbeat_interval_seconds": scalar("integer", "心跳建议间隔。", minimum=10, maximum=300),
            "server_time": TIMESTAMP,
        },
        ("device_id", "device_token", "token_expires_at", "heartbeat_interval_seconds", "server_time"),
    )
    schemas["EnrollmentTokenCreateRequest"] = obj(
        "创建一次性设备注册 token。",
        {
            "created_by": scalar("string", "创建主体标识。"),
            "ttl_seconds": scalar("integer", "有效期秒数。", minimum=60, maximum=604800),
            "allowed_command_types": array("允许的结构化命令类型。", enum("命令类型。", COMMAND_TYPES)),
            "max_risk": enum("最大允许风险。", ["low", "medium", "high"]),
        },
        ("created_by", "ttl_seconds", "allowed_command_types", "max_risk"),
    )
    schemas["EnrollmentTokenCreateResponse"] = obj(
        "一次性设备注册 token。",
        {
            "token": scalar("string", "仅返回一次的明文注册 token。"),
            "expires_at": TIMESTAMP,
        },
        ("token", "expires_at"),
    )
    schemas["CommandLeaseAction"] = obj("命令租约动作请求。", {"lease_id": ID}, ("lease_id",))
    schemas["DeviceTokenRotationResponse"] = obj(
        "设备 token 轮换结果。",
        {
            "device_token": scalar("string", "新设备 token；仅返回一次。"),
            "token_expires_at": TIMESTAMP,
        },
        ("device_token", "token_expires_at"),
    )
    schemas["DeviceRevokeRequest"] = obj(
        "撤销设备请求。",
        {"reason": scalar("string", "撤销原因。", minLength=1, maxLength=1000)},
        ("reason",),
    )
    schemas["DeviceCommandCreateRequest"] = obj(
        "创建受控设备命令。",
        {
            "type": enum("命令类型。", COMMAND_TYPES),
            "payload": obj("命令结构化参数；不得包含明文 Secret。", {}, additional=True),
            "risk": enum("风险等级。", ["low", "medium", "high"]),
            "idempotency_key": scalar("string", "副作用幂等键。", minLength=8, maxLength=128),
            "priority": scalar("integer", "调度优先级。", minimum=-100, maximum=100),
            "max_attempts": scalar("integer", "最大尝试次数。", minimum=1, maximum=20),
            "not_before": TIMESTAMP,
            "expires_at": TIMESTAMP,
        },
        ("type", "payload", "risk", "idempotency_key", "priority", "max_attempts", "not_before", "expires_at"),
    )
    schemas["DeviceEnvActionRequest"] = obj(
        "创建设备 Env 管理动作；响应必须脱敏 value。",
        {
            "action": enum("Env 管理动作。", ["list", "inspect", "set", "delete", "verify", "migrate-from-agentdock-env"]),
            "skill": scalar(["string", "null"], "Runtime 上报的 Skill 名称。"),
            "name": scalar(["string", "null"], "Runtime 上报的环境变量名。"),
            "kind": enum("变量类型。", ["plain", "secret"]),
            "value": scalar(["string", "null"], "写入值；API 响应不得回显明文。"),
            "operation": scalar(["string", "null"], "verify 使用的 operation。"),
            "env_file": scalar(["string", "null"], "迁移来源 env 文件路径。"),
        },
        ("action",),
    )
    schemas["DeviceHeartbeat"] = obj(
        "设备心跳快照。",
        {
            "device_id": ID,
            "sent_at": TIMESTAMP,
            "uptime_seconds": scalar("integer", "进程运行秒数。", minimum=0),
            "agentdock_version": scalar("string", "AgentDock 版本。"),
            "metrics": obj("基础资源指标。", {}, additional=True),
            "capabilities": array("设备能力。", ref("DeviceCapability")),
            "skill_summary": obj("设备 Runtime 上报的 Skill 状态摘要。", {}, additional=True),
            "recall_sync_summary": obj("Recall 同步摘要。", {}, additional=True),
        },
        ("device_id", "sent_at", "uptime_seconds", "agentdock_version", "metrics", "capabilities"),
    )
    schemas["DeviceStatus"] = obj(
        "设备控制面状态。",
        {
            "device_id": ID,
            "status": enum("设备状态。", ["pending", "online", "degraded", "offline", "revoked"]),
            "last_seen_at": TIMESTAMP,
            "agentdock_version": scalar("string", "AgentDock 版本。"),
            "labels": obj("设备标签。", {}, additional={"type": "string"}),
            "capabilities": array("最近能力快照。", ref("DeviceCapability")),
            "version": VERSION,
        },
        ("device_id", "status", "last_seen_at", "agentdock_version", "labels", "capabilities", "version"),
    )
    schemas["DeviceCommand"] = obj(
        "受控设备命令；不允许任意 Shell。",
        {
            "id": ID,
            "device_id": ID,
            "type": enum("命令类型。", COMMAND_TYPES),
            "status": enum("命令状态。", ["queued", "leased", "running", "succeeded", "failed", "expired", "cancelled"]),
            "payload": obj("命令结构化参数。", {}, additional=True),
            "risk": enum("风险等级。", ["low", "medium", "high"]),
            "idempotency_key": scalar("string", "副作用幂等键。"),
            "created_at": TIMESTAMP,
            "expires_at": TIMESTAMP,
            "attempt": scalar("integer", "当前尝试次数。", minimum=0),
            "max_attempts": scalar("integer", "最大尝试次数。", minimum=1, maximum=20),
        },
        ("id", "device_id", "type", "status", "payload", "risk", "idempotency_key", "created_at", "expires_at", "attempt", "max_attempts"),
    )
    schemas["CommandLease"] = obj(
        "设备命令租约。",
        {
            "command": ref("DeviceCommand"),
            "lease_id": ID,
            "leased_at": TIMESTAMP,
            "expires_at": TIMESTAMP,
            "renew_after_seconds": scalar("integer", "建议续租间隔。", minimum=1),
        },
        ("command", "lease_id", "leased_at", "expires_at", "renew_after_seconds"),
    )
    schemas["CommandProgress"] = obj(
        "命令进度上报。",
        {
            "command_id": ID,
            "lease_id": ID,
            "status": enum("执行中状态。", ["leased", "running"]),
            "percent": scalar("integer", "完成百分比。", minimum=0, maximum=100),
            "message": scalar("string", "进度说明。"),
            "reported_at": TIMESTAMP,
        },
        ("command_id", "lease_id", "status", "reported_at"),
    )
    schemas["CommandResult"] = obj(
        "命令终态结果。",
        {
            "command_id": ID,
            "lease_id": ID,
            "status": enum("终态。", ["succeeded", "failed", "expired", "cancelled"]),
            "started_at": TIMESTAMP,
            "completed_at": TIMESTAMP,
            "output": obj("脱敏后的结构化结果。", {}, additional=True),
            "error": {"description": "失败详情；成功时为 null。", "oneOf": [ref("ErrorResponse"), {"type": "null"}]},
            "run_id": scalar(["string", "null"], "兼容节点可选上报的不透明执行关联 ID；Nexus 不创建对应资源。", format="uuid"),
        },
        ("command_id", "lease_id", "status", "started_at", "completed_at", "output"),
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
        "DeviceId": path_param("deviceId", "Device UUID。"),
        "CommandId": path_param("commandId", "Command UUID。"),
        "ArtifactId": path_param("artifactId", "Artifact UUID。"),
        "DeliveryId": path_param("deliveryId", "Delivery UUID。"),
        "FetchId": path_param("fetchId", "Artifact Fetch UUID。"),
        "SessionId": path_param("sessionID", "浏览器 Session ID。", uuid=False),
        "RecallPath": path_param("path", "URL 编码后的召回相对路径。", uuid=False),
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
    binary = {"description": "密文二进制内容。", "content": {"application/octet-stream": {"schema": {"type": "string", "format": "binary"}}}}
    multipart = {"required": True, "content": {"multipart/form-data": {"schema": generic}}}

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
        "/v1/devices": {"get": operation("listDevices", "列出已注册设备")},
        "/v1/devices/{deviceId}": {"get": operation("getDevice", "读取设备详情", params=[p("DeviceId")])},
        "/v1/devices/enroll": {"post": operation("enrollDevice", "注册设备", request=body(ref("DeviceEnrollmentRequest")), success=ok(ref("DeviceEnrollmentResponse")), success_code="201")},
        "/v1/devices/enrollment-tokens": {"post": operation("createEnrollmentToken", "创建一次性设备注册 token", request=body(ref("EnrollmentTokenCreateRequest")), success=ok(ref("EnrollmentTokenCreateResponse")), success_code="201")},
        "/v1/devices/{deviceId}/approve": {"post": operation("approveDevice", "批准设备", params=[p("DeviceId")], success=no_content, success_code="204")},
        "/v1/devices/{deviceId}/revoke": {"post": operation("revokeDevice", "撤销设备", params=[p("DeviceId")], request=body(ref("DeviceRevokeRequest")), success=no_content, success_code="204")},
        "/v1/devices/{deviceId}/policy": {"put": operation("updateDevicePolicy", "更新设备结构化命令策略", params=[p("DeviceId")], request=body())},
        "/v1/devices/{deviceId}/heartbeat": {"post": operation("reportDeviceHeartbeat", "上报设备心跳", params=[p("DeviceId")], request=body(ref("DeviceHeartbeat")), success=no_content, success_code="204")},
        "/v1/devices/{deviceId}/token/rotate": {"post": operation("rotateDeviceToken", "轮换设备 token", params=[p("DeviceId")], success=ok(ref("DeviceTokenRotationResponse")))},
        "/v1/devices/{deviceId}/env/actions": {"post": operation("createDeviceEnvAction", "创建基于 Runtime Registry 的 Env 管理动作", params=[p("DeviceId")], request=body(ref("DeviceEnvActionRequest")), success=ok(ref("DeviceCommand")), additional_success={"201": ok(ref("DeviceCommand"), "已创建。")})},
        "/v1/devices/{deviceId}/commands": {
            "get": operation("listDeviceCommands", "列出设备结构化命令", params=[p("DeviceId")]),
            "post": operation("createDeviceCommand", "创建设备结构化命令", params=[p("DeviceId")], request=body(ref("DeviceCommandCreateRequest")), success=ok(ref("DeviceCommand")), additional_success={"201": ok(ref("DeviceCommand"), "已创建。")}),
        },
        "/v1/devices/{deviceId}/commands/lease": {"post": operation("leaseDeviceCommand", "租用下一条设备命令", params=[p("DeviceId")], success=ok(ref("CommandLease")), additional_success={"204": no_content})},
        "/v1/commands/{commandId}": {"get": operation("getCommand", "读取设备命令详情", params=[p("CommandId")], success=ok(ref("DeviceCommand")))},
        "/v1/commands/{commandId}/start": {"post": operation("startCommand", "标记命令开始执行", params=[p("CommandId")], request=body(ref("CommandLeaseAction")), success=no_content, success_code="204")},
        "/v1/commands/{commandId}/renew": {"post": operation("renewCommandLease", "续租设备命令", params=[p("CommandId")], request=body(ref("CommandLeaseAction")), success=ok(ref("CommandLease")))},
        "/v1/commands/{commandId}/progress": {"post": operation("reportCommandProgress", "上报设备命令进度", params=[p("CommandId")], request=body(ref("CommandProgress")), success=no_content, success_code="204")},
        "/v1/commands/{commandId}/result": {"post": operation("completeCommand", "上报设备命令结果", params=[p("CommandId")], request=body(ref("CommandResult")), success=no_content, success_code="204")},
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
        "/v1/sync/status": {"get": operation("getSyncStatus", "读取召回 Git 同步状态")},
        "/v1/git/diff": {"get": operation("getGitDiff", "读取召回仓库变更")},
        "/v1/git/discard": {"post": operation("discardGitChanges", "丢弃召回仓库本地变更", request=body())},
        "/v1/git/log": {"get": operation("getGitLog", "读取召回仓库提交历史")},
        "/v1/git/commit": {"get": operation("getGitCommit", "读取召回仓库提交详情")},
        "/v1/sync/pull": {"post": operation("pullRecall", "从远端更新召回仓库", request=body())},
        "/v1/sync/push": {"post": operation("pushRecall", "保存召回仓库到远端", request=body())},
        "/v1/sync/now": {"post": operation("syncRecallNow", "立即双向同步召回仓库", request=body())},
        "/v1/artifacts": {"get": operation("listArtifacts", "列出最近加密文件发送记录")},
        "/v1/artifact-fetches": {"get": operation("listArtifactFetches", "列出最近反向文件接收记录")},
        "/v1/artifacts/uploads": {"post": operation("createArtifactUpload", "创建管理员文件上传", request=body(), success_code="201")},
        "/v1/artifacts/{artifactId}": {"get": operation("getArtifact", "读取文件发送详情", params=[p("ArtifactId")])},
        "/v1/artifacts/{artifactId}/dispatch": {"post": operation("dispatchArtifact", "派发加密文件", params=[p("ArtifactId")], request=body())},
        "/v1/artifacts/{artifactId}/content": {"post": operation("uploadArtifactContent", "上传加密文件内容", params=[p("ArtifactId")], request=multipart, success_code="201")},
        "/v1/devices/{deviceId}/artifacts/uploads": {"post": operation("createDeviceArtifactUpload", "创建设备文件上传", params=[p("DeviceId")], request=body(), success_code="201")},
        "/v1/devices/{deviceId}/artifact-deliveries/{deliveryId}/content": {"get": operation("downloadArtifactContent", "下载派发给设备的加密内容", params=[p("DeviceId"), p("DeliveryId")], success=binary)},
        "/v1/devices/{deviceId}/artifact-deliveries/{deliveryId}/result": {"post": operation("completeArtifactDelivery", "上报设备文件落盘结果", params=[p("DeviceId"), p("DeliveryId")], request=body())},
        "/v1/devices/{deviceId}/artifact-fetches": {"post": operation("createArtifactFetch", "创建反向文件接收请求", params=[p("DeviceId")], request=body(), success_code="201")},
        "/v1/devices/{deviceId}/artifact-fetches/{fetchId}": {"get": operation("getArtifactFetch", "读取反向文件接收详情", params=[p("DeviceId"), p("FetchId")])},
        "/v1/devices/{deviceId}/artifact-fetches/{fetchId}/content": {
            "get": operation("downloadArtifactFetch", "下载反向接收的加密内容", params=[p("DeviceId"), p("FetchId")], success=binary),
            "post": operation("uploadArtifactFetch", "上传反向接收的加密内容", params=[p("DeviceId"), p("FetchId")], request=multipart, success_code="201"),
        },
        "/v1/devices/{deviceId}/artifact-fetches/{fetchId}/mounted": {"post": operation("confirmArtifactFetchMounted", "确认反向接收文件已挂载", params=[p("DeviceId"), p("FetchId")], request=body())},
        "/v1/devices/{deviceId}/artifact-fetches/{fetchId}/result": {"post": operation("reportArtifactFetchResult", "上报反向接收文件结果", params=[p("DeviceId"), p("FetchId")], request=body())},
    }

    return {
        "openapi": "3.1.0",
        "info": {
            "title": "AgentDock Nexus API",
            "version": "1.0.0",
            "description": "个人多设备 AgentDock 控制台的真实生产 HTTP 契约，覆盖设备、召回、加密文件、备份状态和账号会话。",
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

func (c *Client) EnrollDevice(ctx context.Context, request DeviceEnrollmentRequest) (DeviceEnrollmentResponse, error) {
	var response DeviceEnrollmentResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/devices/enroll", "", request, &response)
	return response, err
}

func (c *Client) CreateEnrollmentToken(ctx context.Context, request EnrollmentTokenCreateRequest) (EnrollmentTokenCreateResponse, error) {
	var response EnrollmentTokenCreateResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/devices/enrollment-tokens", "", request, &response)
	return response, err
}

func (c *Client) ApproveDevice(ctx context.Context, deviceID string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/devices/"+url.PathEscape(deviceID)+"/approve", "", nil, nil)
}

func (c *Client) RevokeDevice(ctx context.Context, deviceID string, request DeviceRevokeRequest) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/devices/"+url.PathEscape(deviceID)+"/revoke", "", request, nil)
}

func (c *Client) CreateDeviceCommand(ctx context.Context, deviceID string, request DeviceCommandCreateRequest) (DeviceCommand, error) {
	var response DeviceCommand
	err := c.doJSON(ctx, http.MethodPost, "/v1/devices/"+url.PathEscape(deviceID)+"/commands", request.IdempotencyKey, request, &response)
	return response, err
}

func (c *Client) ReportDeviceHeartbeat(ctx context.Context, deviceID string, request DeviceHeartbeat) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/devices/"+url.PathEscape(deviceID)+"/heartbeat", "", request, nil)
}

func (c *Client) RotateDeviceToken(ctx context.Context, deviceID string) (DeviceTokenRotationResponse, error) {
	var response DeviceTokenRotationResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/devices/"+url.PathEscape(deviceID)+"/token/rotate", "", nil, &response)
	return response, err
}

func (c *Client) LeaseDeviceCommand(ctx context.Context, deviceID string) (*CommandLease, error) {
	var response CommandLease
	status, err := c.doJSONStatus(ctx, http.MethodPost, "/v1/devices/"+url.PathEscape(deviceID)+"/commands/lease", "", nil, &response)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNoContent {
		return nil, nil
	}
	return &response, nil
}

func (c *Client) StartCommand(ctx context.Context, commandID string, request CommandLeaseAction) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/commands/"+url.PathEscape(commandID)+"/start", "", request, nil)
}

func (c *Client) RenewCommandLease(ctx context.Context, commandID string, request CommandLeaseAction) (CommandLease, error) {
	var response CommandLease
	err := c.doJSON(ctx, http.MethodPost, "/v1/commands/"+url.PathEscape(commandID)+"/renew", "", request, &response)
	return response, err
}

func (c *Client) ReportCommandProgress(ctx context.Context, commandID string, request CommandProgress) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/commands/"+url.PathEscape(commandID)+"/progress", "", request, nil)
}

func (c *Client) CompleteCommand(ctx context.Context, commandID string, request CommandResult) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/commands/"+url.PathEscape(commandID)+"/result", "", request, nil)
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
