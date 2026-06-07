#!/usr/bin/env python3
"""Generate Nexus OpenAPI, JSON Schemas, event schemas and Go client DTOs.

The generator intentionally uses only the Python standard library so every
AgentDock development device can reproduce the checked-in output.
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
    "SCHEDULE_NOT_FOUND",
]

COMMAND_TYPES = [
    "health.check",
    "skill.install",
    "skill.run",
    "skill.rollback",
    "memory.sync",
    "service.inspect",
    "service.restart",
    "diagnostics.collect",
    "agentdock.reload",
]

EVOLUTION_TRIGGERS = [
    "user_correction",
    "agent_recovery_success",
    "false_success",
    "false_failure",
    "repeated_failure",
    "repeated_manual_step",
    "missing_validation",
    "environment_drift",
    "performance_regression",
    "security_violation",
    "upstream_update",
]

EVOLUTION_STATES = [
    "observed",
    "watching",
    "candidate",
    "proposal_draft",
    "review_ready",
    "approved",
    "testing",
    "canary",
    "released",
    "rejected",
    "deferred",
    "rolled_back",
]


def build_schemas() -> dict[str, dict[str, Any]]:
    schemas: dict[str, dict[str, Any]] = {}
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
            "code": enum("稳定错误码。", ERROR_CODES),
            "message": scalar("string", "面向调用方的错误说明。"),
            "request_id": scalar("string", "请求关联 ID。"),
            "details": array("可选字段级错误。", ref("ErrorDetail")),
        },
        ("code", "message", "request_id"),
    )
    schemas["Pagination"] = obj(
        "游标分页信息。",
        {
            "next_cursor": scalar(["string", "null"], "下一页游标；无下一页时为 null。"),
            "limit": scalar("integer", "本页上限。", minimum=1, maximum=200),
            "total": scalar(["integer", "null"], "可选总数。", minimum=0),
        },
        ("limit",),
    )
    schemas["Actor"] = obj(
        "执行动作的主体。",
        {
            "type": enum("主体类型。", ["user", "agent", "device", "system"]),
            "id": ID,
            "display_name": scalar("string", "显示名称。"),
        },
        ("type", "id"),
    )
    schemas["AuditReference"] = obj(
        "审计事件引用。",
        {
            "audit_id": ID,
            "request_id": scalar("string", "请求关联 ID。"),
            "run_id": scalar(["string", "null"], "关联 Run ID。", format="uuid"),
        },
        ("audit_id", "request_id"),
    )
    schemas["IdempotencyKey"] = obj(
        "幂等键及其作用域。",
        {
            "key": scalar("string", "调用方生成的幂等键。", minLength=8, maxLength=128),
            "scope": scalar("string", "幂等操作作用域。", minLength=1, maxLength=128),
        },
        ("key", "scope"),
    )
    schemas["ObjectReference"] = obj(
        "跨模块对象引用。",
        {
            "type": enum(
                "对象类型。",
                ["device", "memory", "skill", "run", "proposal", "project", "task", "command", "schedule"],
            ),
            "id": scalar("string", "对象 ID。"),
            "label": scalar("string", "可读标签。"),
        },
        ("type", "id"),
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
            "public_key": scalar("string", "设备公钥，PEM 或 JWK。"),
            "labels": obj("设备标签。", {}, additional={"type": "string"}),
        },
        ("enrollment_token", "name", "platform", "arch", "agentdock_version", "public_key"),
    )
    schemas["DeviceEnrollmentResponse"] = obj(
        "设备注册结果。",
        {
            "device_id": ID,
            "device_token": scalar("string", "后续设备认证 token；仅返回一次。"),
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
            "allowed_command_types": array("允许的命令类型。", enum("命令类型。", COMMAND_TYPES)),
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
    schemas["CommandLeaseAction"] = obj(
        "命令租约动作请求。",
        {"lease_id": ID},
        ("lease_id",),
    )
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
    schemas["DeviceHeartbeat"] = obj(
        "设备心跳快照。",
        {
            "device_id": ID,
            "sent_at": TIMESTAMP,
            "uptime_seconds": scalar("integer", "进程运行秒数。", minimum=0),
            "agentdock_version": scalar("string", "AgentDock 版本。"),
            "metrics": obj(
                "基础资源指标。",
                {
                    "cpu_percent": scalar("number", "CPU 使用率。", minimum=0, maximum=100),
                    "memory_percent": scalar("number", "内存使用率。", minimum=0, maximum=100),
                    "disk_percent": scalar("number", "数据盘使用率。", minimum=0, maximum=100),
                },
                ("cpu_percent", "memory_percent", "disk_percent"),
            ),
            "capabilities": array("设备能力。", ref("DeviceCapability")),
            "skill_summary": obj("Skill 安装摘要。", {}, additional=True),
            "memory_sync_summary": obj("Memory 同步摘要。", {}, additional=True),
        },
        ("device_id", "sent_at", "uptime_seconds", "agentdock_version", "metrics", "capabilities"),
    )
    schemas["DeviceStatus"] = obj(
        "设备控制面状态。",
        {
            "device_id": ID,
            "status": enum("设备状态；90 秒无心跳 degraded，180 秒 offline。", ["pending", "online", "degraded", "offline", "revoked"]),
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
            "payload": obj("命令结构化参数；不得包含明文 Secret。", {}, additional=True),
            "risk": enum("风险等级。", ["low", "medium", "high"]),
            "idempotency_key": scalar("string", "副作用幂等键。", minLength=8, maxLength=128),
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
            "run_id": scalar(["string", "null"], "关联 Run ID。", format="uuid"),
        },
        ("command_id", "lease_id", "status", "started_at", "completed_at", "output"),
    )

    schemas["SkillOperation"] = obj(
        "Skill 可执行 operation。",
        {
            "name": scalar("string", "稳定 operation 名。", pattern="^[a-z][a-z0-9._-]*$"),
            "description": scalar("string", "operation 说明。"),
            "input_schema": obj("JSON Schema 输入定义。", {}, additional=True),
            "output_schema": obj("JSON Schema 输出定义。", {}, additional=True),
            "timeout_seconds": scalar("integer", "默认超时。", minimum=1, maximum=86400),
            "permissions": array("声明权限。", scalar("string", "权限名。")),
        },
        ("name", "description", "input_schema", "output_schema", "timeout_seconds", "permissions"),
    )
    schemas["SkillSummary"] = obj(
        "Skill 列表摘要。",
        {
            "id": ID,
            "name": scalar("string", "稳定 Skill 名。"),
            "display_name": scalar("string", "显示名称。"),
            "description": scalar("string", "简介。"),
            "latest_version": scalar("string", "最新发布版本。"),
            "trust": enum("信任状态。", ["unknown", "reviewed", "trusted", "blocked"]),
            "maturity": enum("成熟度。", ["experimental", "development", "canary", "stable", "deprecated"]),
            "updated_at": TIMESTAMP,
        },
        ("id", "name", "display_name", "description", "latest_version", "trust", "maturity", "updated_at"),
    )
    schemas["SkillRelease"] = obj(
        "不可变 Skill 发布。",
        {
            "id": ID,
            "skill_id": ID,
            "version": scalar("string", "语义版本。", pattern=r"^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$"),
            "channel": enum("发布通道。", ["development", "canary", "stable", "pinned"]),
            "digest": scalar("string", "sha256 摘要。", pattern="^sha256:[0-9a-f]{64}$"),
            "manifest_url": scalar("string", "manifest 下载地址。", format="uri"),
            "package_url": scalar("string", "包下载地址。", format="uri"),
            "published_at": TIMESTAMP,
            "published_by": ref("Actor"),
        },
        ("id", "skill_id", "version", "channel", "digest", "manifest_url", "package_url", "published_at", "published_by"),
    )
    schemas["SkillInstallation"] = obj(
        "设备上的 Skill 安装状态。",
        {
            "id": ID,
            "skill_id": ID,
            "device_id": ID,
            "release_id": ID,
            "version": scalar("string", "安装版本。"),
            "channel": enum("安装通道。", ["development", "canary", "stable", "pinned"]),
            "status": enum("安装状态。", ["pending", "installing", "active", "failed", "rolling_back", "rolled_back", "removed"]),
            "installed_at": TIMESTAMP,
            "verified_at": scalar(["string", "null"], "最近验证时间。", format="date-time"),
            "last_error": {"description": "最近错误。", "oneOf": [ref("ErrorResponse"), {"type": "null"}]},
        },
        ("id", "skill_id", "device_id", "release_id", "version", "channel", "status", "installed_at"),
    )
    schemas["SkillDetail"] = obj(
        "Skill 完整详情。",
        {
            "summary": ref("SkillSummary"),
            "operations": array("可执行 operations。", ref("SkillOperation")),
            "releases": array("发布列表。", ref("SkillRelease")),
            "compatibility": obj("平台和 AgentDock 兼容性。", {}, additional=True),
            "provenance": obj("来源与许可证信息。", {}, additional=True),
            "installed_devices": array("安装记录。", ref("SkillInstallation")),
        },
        ("summary", "operations", "releases", "compatibility", "provenance", "installed_devices"),
    )
    schemas["SkillRunRequest"] = obj(
        "执行 Skill operation 请求。",
        {
            "skill_id": ID,
            "operation": scalar("string", "operation 名。"),
            "input": obj("按 operation input_schema 校验的输入。", {}, additional=True),
            "device_id": ID,
            "release_id": scalar(["string", "null"], "指定 release；null 使用 active。", format="uuid"),
            "timeout_seconds": scalar(["integer", "null"], "调用级超时覆盖。", minimum=1, maximum=86400),
            "idempotency_key": scalar("string", "运行幂等键。", minLength=8, maxLength=128),
        },
        ("skill_id", "operation", "input", "device_id", "idempotency_key"),
    )

    schemas["VerificationResult"] = obj(
        "运行验证结果。",
        {
            "status": enum("验证状态。", ["passed", "failed", "skipped"]),
            "summary": scalar("string", "验证摘要。"),
            "checks": array(
                "验证检查项。",
                obj(
                    "验证检查项。",
                    {
                        "name": scalar("string", "检查名。"),
                        "passed": scalar("boolean", "是否通过。"),
                        "evidence_id": scalar(["string", "null"], "证据 ID。", format="uuid"),
                        "message": scalar("string", "检查说明。"),
                    },
                    ("name", "passed", "message"),
                ),
            ),
            "verified_at": TIMESTAMP,
        },
        ("status", "summary", "checks", "verified_at"),
    )
    schemas["RunEvidence"] = obj(
        "Run 证据。",
        {
            "id": ID,
            "run_id": ID,
            "type": enum("证据类型。", ["log", "command", "http", "screenshot", "artifact", "diff", "metric", "user_confirmation"]),
            "uri": scalar(["string", "null"], "证据 URI。", format="uri"),
            "digest": scalar(["string", "null"], "内容摘要。"),
            "summary": scalar("string", "脱敏摘要。"),
            "created_at": TIMESTAMP,
        },
        ("id", "run_id", "type", "summary", "created_at"),
    )
    schemas["RunStep"] = obj(
        "Run 步骤。",
        {
            "id": ID,
            "run_id": ID,
            "sequence": scalar("integer", "从 1 开始的顺序号。", minimum=1),
            "name": scalar("string", "步骤名。"),
            "status": enum("步骤状态。", ["pending", "running", "succeeded", "failed", "skipped"]),
            "started_at": scalar(["string", "null"], "开始时间。", format="date-time"),
            "completed_at": scalar(["string", "null"], "结束时间。", format="date-time"),
            "summary": scalar("string", "步骤摘要。"),
            "error": {"description": "步骤错误。", "oneOf": [ref("ErrorResponse"), {"type": "null"}]},
        },
        ("id", "run_id", "sequence", "name", "status", "summary"),
    )
    schemas["Run"] = obj(
        "统一运行注册记录。",
        {
            "id": ID,
            "type": enum("Run 类型。", ["skill", "command", "memory", "task", "evolution", "migration", "system"]),
            "status": enum("Run 状态。", ["pending", "running", "succeeded", "failed", "cancelled", "timed_out"]),
            "actor": ref("Actor"),
            "device_id": scalar(["string", "null"], "设备 ID。", format="uuid"),
            "task_id": scalar(["string", "null"], "Task ID。", format="uuid"),
            "skill_id": scalar(["string", "null"], "Skill ID。", format="uuid"),
            "started_at": scalar(["string", "null"], "开始时间。", format="date-time"),
            "completed_at": scalar(["string", "null"], "结束时间。", format="date-time"),
            "summary": scalar("string", "运行摘要。"),
            "steps": array("步骤。", ref("RunStep")),
            "evidence": array("证据。", ref("RunEvidence")),
            "verification": {"description": "最终验证。", "oneOf": [ref("VerificationResult"), {"type": "null"}]},
            "version": VERSION,
        },
        ("id", "type", "status", "actor", "summary", "steps", "evidence", "version"),
    )
    schemas["SkillRunResult"] = obj(
        "Skill operation 结果。",
        {
            "run_id": ID,
            "skill_id": ID,
            "operation": scalar("string", "operation 名。"),
            "status": enum("运行终态。", ["succeeded", "failed", "cancelled", "timed_out"]),
            "output": obj("按 output_schema 校验并脱敏的输出。", {}, additional=True),
            "error": {"description": "运行错误。", "oneOf": [ref("ErrorResponse"), {"type": "null"}]},
            "started_at": TIMESTAMP,
            "completed_at": TIMESTAMP,
            "verification": {"description": "结果验证。", "oneOf": [ref("VerificationResult"), {"type": "null"}]},
        },
        ("run_id", "skill_id", "operation", "status", "output", "started_at", "completed_at"),
    )

    schedule_states = ["never_run", "queued", "running", "success", "failed", "unknown", "disabled"]
    schemas["ScheduleHistory"] = obj(
        "计划任务执行历史；只包含脱敏后的归档和状态证据。",
        {
            "schema_version": scalar("integer", "状态文件版本。", minimum=1),
            "state": enum("执行状态。", schedule_states),
            "message": scalar("string", "脱敏后的状态说明。"),
            "started_at": scalar(["string", "null"], "开始时间。", format="date-time"),
            "completed_at": scalar(["string", "null"], "结束时间。", format="date-time"),
            "host": scalar("string", "执行主机显示名；不得包含凭据。"),
            "archive": scalar("string", "归档文件名。"),
            "archive_size": scalar("integer", "归档字节数。", minimum=0),
            "sha256": scalar("string", "归档 SHA256。"),
            "remote_path": scalar("string", "脱敏后的远端路径。"),
        },
        ("state",),
    )
    schemas["ScheduleItem"] = obj(
        "计划任务状态摘要。",
        {
            "id": scalar("string", "稳定计划任务 ID。", pattern="^[a-z][a-z0-9._-]*$"),
            "title": scalar("string", "显示标题。"),
            "description": scalar("string", "任务说明。"),
            "provider": scalar("string", "调度提供方。"),
            "device": scalar("string", "执行设备显示名。"),
            "enabled": scalar("boolean", "是否启用。"),
            "schedule": scalar("string", "可读执行计划。"),
            "schedule_type": enum("计划类型。", ["calendar", "interval", "manual"]),
            "state": enum("最近状态。", schedule_states),
            "last_started_at": scalar(["string", "null"], "最近开始时间。", format="date-time"),
            "last_completed_at": scalar(["string", "null"], "最近完成时间。", format="date-time"),
            "next_run_at": TIMESTAMP,
            "message": scalar("string", "脱敏后的状态说明。"),
            "archive": scalar("string", "最近归档文件名。"),
            "archive_size": scalar("integer", "最近归档字节数。", minimum=0),
            "sha256": scalar("string", "最近归档 SHA256。"),
            "remote_path": scalar("string", "脱敏后的远端路径。"),
            "history": array("最近执行历史。", ref("ScheduleHistory")),
        },
        ("id", "title", "provider", "device", "enabled", "schedule", "schedule_type", "state", "next_run_at", "history"),
    )
    schemas["ScheduleListResponse"] = obj(
        "计划任务列表响应。",
        {"items": array("计划任务。", ref("ScheduleItem"))},
        ("items",),
    )

    schemas["MemoryEntry"] = obj(
        "长期记忆条目。",
        {
            "id": ID,
            "path": scalar("string", "Memory Repository 相对路径。"),
            "title": scalar("string", "标题。"),
            "scope": enum("作用域。", ["profile", "global", "project", "device", "agent", "ops", "inbox"]),
            "status": enum("记忆状态。", ["active", "stale", "conflicted", "unverified", "deprecated"]),
            "content": scalar("string", "Markdown 内容。"),
            "verified_at": scalar(["string", "null"], "最近验证时间。", format="date-time"),
            "verification_run_id": scalar(["string", "null"], "验证 Run ID。", format="uuid"),
            "source_device": scalar(["string", "null"], "来源设备 ID。", format="uuid"),
            "source_agent": scalar(["string", "null"], "来源 Agent ID。", format="uuid"),
            "confidence": scalar("number", "置信度。", minimum=0, maximum=1),
            "updated_at": TIMESTAMP,
            "version": VERSION,
        },
        ("id", "path", "title", "scope", "status", "content", "confidence", "updated_at", "version"),
    )
    schemas["MemoryConflict"] = obj(
        "记忆与事实冲突。",
        {
            "id": ID,
            "memory_id": ID,
            "status": enum("冲突状态。", ["open", "resolved", "dismissed"]),
            "source_type": enum("冲突来源。", ["device_snapshot", "skill_run", "user_edit", "git_merge", "agent_repair"]),
            "observed_value": {"description": "设备或运行观察到的结构化值。"},
            "memory_value": {"description": "Memory 当前结构化值。"},
            "summary": scalar("string", "冲突摘要。"),
            "detected_at": TIMESTAMP,
            "resolved_at": scalar(["string", "null"], "解决时间。", format="date-time"),
            "resolution_run_id": scalar(["string", "null"], "解决 Run ID。", format="uuid"),
        },
        ("id", "memory_id", "status", "source_type", "observed_value", "memory_value", "summary", "detected_at"),
    )
    schemas["MemoryContextPack"] = obj(
        "任务所需记忆上下文。",
        {
            "entries": array("选中的记忆。", ref("MemoryEntry")),
            "conflicts": array("相关冲突。", ref("MemoryConflict")),
            "truncated": scalar("boolean", "是否因 max_bytes 截断。"),
            "total_bytes": scalar("integer", "实际包字节数。", minimum=0),
            "generated_at": TIMESTAMP,
        },
        ("entries", "conflicts", "truncated", "total_bytes", "generated_at"),
    )

    schemas["Observation"] = obj(
        "Skill 运行观察事件。",
        {
            "id": ID,
            "skill_id": ID,
            "run_id": ID,
            "device_id": scalar(["string", "null"], "设备 ID。", format="uuid"),
            "trigger": enum("进化触发类型。", EVOLUTION_TRIGGERS),
            "signature": scalar("string", "归一化错误或行为签名。"),
            "summary": scalar("string", "观察摘要。"),
            "evidence_ids": array("证据 ID。", ID),
            "private_scope": scalar("boolean", "是否包含仅设备可用信息。"),
            "observed_at": TIMESTAMP,
        },
        ("id", "skill_id", "run_id", "trigger", "signature", "summary", "evidence_ids", "private_scope", "observed_at"),
    )
    schemas["EvolutionCandidate"] = obj(
        "聚合后的进化候选。",
        {
            "id": ID,
            "skill_id": ID,
            "status": enum("候选状态。", EVOLUTION_STATES),
            "signature": scalar("string", "聚合签名。"),
            "trigger": enum("主要触发类型。", EVOLUTION_TRIGGERS),
            "observation_ids": array("Observation ID。", ID),
            "score": scalar("number", "固定规则计算的可解释分数。", minimum=0, maximum=100),
            "confidence": scalar("number", "跨运行置信度。", minimum=0, maximum=1),
            "reasoning": array("评分依据。", scalar("string", "可解释规则结果。")),
            "created_at": TIMESTAMP,
            "updated_at": TIMESTAMP,
        },
        ("id", "skill_id", "status", "signature", "trigger", "observation_ids", "score", "confidence", "reasoning", "created_at", "updated_at"),
    )
    schemas["EvolutionProposal"] = obj(
        "待审查的 Skill 进化提案。",
        {
            "id": ID,
            "candidate_id": ID,
            "skill_id": ID,
            "status": enum("提案状态。", EVOLUTION_STATES),
            "problem": scalar("string", "问题定义。"),
            "evidence": array("证据引用。", ref("ObjectReference")),
            "scope": enum("提案作用域。", ["global", "project", "device"]),
            "suggested_files": array("建议修改的 Skill 相对路径。", scalar("string", "相对路径。")),
            "risk": enum("风险等级。", ["low", "medium", "high", "critical"]),
            "tests": array("必须执行的测试。", scalar("string", "测试说明。")),
            "expected_benefit": scalar("string", "预期收益。"),
            "created_at": TIMESTAMP,
            "updated_at": TIMESTAMP,
        },
        ("id", "candidate_id", "skill_id", "status", "problem", "evidence", "scope", "suggested_files", "risk", "tests", "expected_benefit", "created_at", "updated_at"),
    )

    schemas["TaskLink"] = obj(
        "Task 与领域对象的 Link。",
        {
            "type": enum("Link 类型。", ["device", "memory", "skill", "run", "proposal", "project"]),
            "object_id": scalar("string", "对象 ID。"),
            "relation": scalar("string", "关系说明。"),
        },
        ("type", "object_id", "relation"),
    )
    schemas["TaskCompletion"] = obj(
        "Task 完成结果。",
        {
            "summary": scalar("string", "完成摘要。"),
            "verification_summary": scalar("string", "必须提供的验证摘要。", minLength=1),
            "run_id": scalar(["string", "null"], "完成 Run ID。", format="uuid"),
            "evidence_ids": array("完成证据 ID。", ID),
            "completed_at": TIMESTAMP,
        },
        ("summary", "verification_summary", "evidence_ids", "completed_at"),
    )
    schemas["Task"] = obj(
        "Agent Inbox 任务。",
        {
            "id": ID,
            "type": enum("任务类型。", ["needs_agent", "needs_user", "automatic", "scheduled", "review"]),
            "status": enum("任务状态。", ["inbox", "ready", "in_progress", "blocked", "awaiting_user", "awaiting_agent", "completed", "cancelled", "failed"]),
            "title": scalar("string", "标题。"),
            "description": scalar("string", "任务说明。"),
            "category": scalar("string", "稳定分类。"),
            "source_type": scalar("string", "创建来源类型。"),
            "source_id": scalar("string", "创建来源 ID。"),
            "object_id": scalar("string", "主要对象 ID。"),
            "priority": enum("优先级。", ["low", "normal", "high", "critical"]),
            "links": array("领域对象链接。", ref("TaskLink")),
            "assigned_actor": {"description": "当前负责人。", "oneOf": [ref("Actor"), {"type": "null"}]},
            "completion_criteria": array("完成标准。", scalar("string", "标准。")),
            "risk_constraints": array("风险约束。", scalar("string", "约束。")),
            "completion": {"description": "完成结果。", "oneOf": [ref("TaskCompletion"), {"type": "null"}]},
            "created_at": TIMESTAMP,
            "updated_at": TIMESTAMP,
            "version": VERSION,
        },
        ("id", "type", "status", "title", "description", "category", "source_type", "source_id", "object_id", "priority", "links", "completion_criteria", "risk_constraints", "created_at", "updated_at", "version"),
    )
    schemas["TaskContextPack"] = obj(
        "完成 Task 的聚合上下文。",
        {
            "task": ref("Task"),
            "memory": ref("MemoryContextPack"),
            "device": {"description": "设备快照。", "oneOf": [ref("DeviceStatus"), {"type": "null"}]},
            "skill": {"description": "Skill 详情。", "oneOf": [ref("SkillDetail"), {"type": "null"}]},
            "recent_runs": array("最近相关 Runs。", ref("Run")),
            "evidence": array("相关证据。", ref("RunEvidence")),
            "generated_at": TIMESTAMP,
            "truncated": scalar("boolean", "是否截断。"),
        },
        ("task", "memory", "recent_runs", "evidence", "generated_at", "truncated"),
    )
    return schemas


def response(schema: dict[str, Any], description: str = "成功。") -> dict[str, Any]:
    return {"description": description, "content": {"application/json": {"schema": schema}}}


def build_openapi(schemas: dict[str, Any]) -> dict[str, Any]:
    error = response(ref("ErrorResponse"), "错误。")
    path_param = lambda name, description: {
        "name": name,
        "in": "path",
        "required": True,
        "description": description,
        "schema": {"type": "string", "format": "uuid"},
    }
    parameters = {
        "DeviceId": path_param("deviceId", "Device UUID。"),
        "CommandId": path_param("commandId", "Command UUID。"),
        "SkillId": path_param("skillId", "Skill UUID。"),
        "TaskId": path_param("taskId", "Task UUID。"),
        "RunId": path_param("runId", "Run UUID。"),
        "ScheduleId": {
            "name": "scheduleId",
            "in": "path",
            "required": True,
            "description": "Schedule stable ID。",
            "schema": {"type": "string", "pattern": "^[a-z][a-z0-9._-]*$"},
        },
        "IdempotencyKey": {
            "name": "Idempotency-Key",
            "in": "header",
            "required": True,
            "description": "写操作幂等键。",
            "schema": {"type": "string", "minLength": 8, "maxLength": 128},
        },
    }
    body = lambda schema: {"required": True, "content": {"application/json": {"schema": schema}}}
    paths: dict[str, Any] = {
        "/health": {"get": {"operationId": "getHealth", "summary": "存活检查", "responses": {"200": response(obj("健康状态。", {"ok": scalar("boolean", "是否健康。"), "service": scalar("string", "服务名。")}, ("ok", "service")))}}},
        "/ready": {"get": {"operationId": "getReadiness", "summary": "就绪检查", "responses": {"200": response(obj("就绪状态。", {"ready": scalar("boolean", "是否就绪。")}, ("ready",))), "503": error}}},
        "/v1/devices/enroll": {"post": {"operationId": "enrollDevice", "summary": "注册设备", "requestBody": body(ref("DeviceEnrollmentRequest")), "responses": {"201": response(ref("DeviceEnrollmentResponse")), "400": error, "401": error, "409": error}}},
        "/v1/devices/enrollment-tokens": {"post": {"operationId": "createEnrollmentToken", "summary": "创建一次性注册 token", "requestBody": body(ref("EnrollmentTokenCreateRequest")), "responses": {"201": response(ref("EnrollmentTokenCreateResponse")), "400": error, "401": error, "403": error}}},
        "/v1/devices/{deviceId}/heartbeat": {"post": {"operationId": "reportDeviceHeartbeat", "summary": "上报设备心跳", "parameters": [{"$ref": "#/components/parameters/DeviceId"}], "requestBody": body(ref("DeviceHeartbeat")), "responses": {"204": {"description": "已接受。"}, "401": error, "409": error}}},
        "/v1/devices/{deviceId}/approve": {"post": {"operationId": "approveDevice", "summary": "批准设备", "parameters": [{"$ref": "#/components/parameters/DeviceId"}], "responses": {"204": {"description": "已批准。"}, "401": error, "403": error, "404": error, "409": error}}},
        "/v1/devices/{deviceId}/revoke": {"post": {"operationId": "revokeDevice", "summary": "撤销设备", "parameters": [{"$ref": "#/components/parameters/DeviceId"}], "requestBody": body(ref("DeviceRevokeRequest")), "responses": {"204": {"description": "已撤销。"}, "401": error, "403": error, "404": error, "409": error}}},
        "/v1/devices/{deviceId}/token/rotate": {"post": {"operationId": "rotateDeviceToken", "summary": "轮换设备 token", "parameters": [{"$ref": "#/components/parameters/DeviceId"}], "responses": {"200": response(ref("DeviceTokenRotationResponse")), "401": error, "409": error}}},
        "/v1/devices/{deviceId}/commands": {"post": {"operationId": "createDeviceCommand", "summary": "创建设备命令", "parameters": [{"$ref": "#/components/parameters/DeviceId"}], "requestBody": body(ref("DeviceCommandCreateRequest")), "responses": {"201": response(ref("DeviceCommand")), "200": response(ref("DeviceCommand"), "幂等命中。"), "400": error, "401": error, "403": error, "409": error}}},
        "/v1/devices/{deviceId}/commands/lease": {"post": {"operationId": "leaseDeviceCommand", "summary": "租用下一条设备命令", "parameters": [{"$ref": "#/components/parameters/DeviceId"}], "responses": {"200": response(ref("CommandLease")), "204": {"description": "当前无命令。"}, "401": error, "409": error}}},
        "/v1/commands/{commandId}/start": {"post": {"operationId": "startCommand", "summary": "标记命令开始执行", "parameters": [{"$ref": "#/components/parameters/CommandId"}], "requestBody": body(ref("CommandLeaseAction")), "responses": {"204": {"description": "已开始。"}, "401": error, "409": error}}},
        "/v1/commands/{commandId}/renew": {"post": {"operationId": "renewCommandLease", "summary": "续租命令", "parameters": [{"$ref": "#/components/parameters/CommandId"}], "requestBody": body(ref("CommandLeaseAction")), "responses": {"200": response(ref("CommandLease")), "401": error, "409": error}}},
        "/v1/commands/{commandId}/progress": {"post": {"operationId": "reportCommandProgress", "summary": "上报命令进度", "parameters": [{"$ref": "#/components/parameters/CommandId"}], "requestBody": body(ref("CommandProgress")), "responses": {"204": {"description": "已接受。"}, "401": error, "409": error}}},
        "/v1/commands/{commandId}/result": {"post": {"operationId": "completeCommand", "summary": "上报命令结果", "parameters": [{"$ref": "#/components/parameters/CommandId"}], "requestBody": body(ref("CommandResult")), "responses": {"204": {"description": "已接受。"}, "401": error, "409": error}}},
        "/v1/skills": {"get": {"operationId": "listSkills", "summary": "列出 Skills", "responses": {"200": response(obj("Skill 分页列表。", {"items": array("Skills。", ref("SkillSummary")), "pagination": ref("Pagination")}, ("items", "pagination"))), "401": error}}},
        "/v1/skills/{skillId}": {"get": {"operationId": "getSkill", "summary": "读取 Skill 详情", "parameters": [{"$ref": "#/components/parameters/SkillId"}], "responses": {"200": response(ref("SkillDetail")), "404": error}}},
        "/v1/skill-runs": {"post": {"operationId": "runSkill", "summary": "请求运行 Skill", "parameters": [{"$ref": "#/components/parameters/IdempotencyKey"}], "requestBody": body(ref("SkillRunRequest")), "responses": {"202": response(ref("Run")), "400": error, "409": error}}},
        "/v1/tasks": {"get": {"operationId": "listTasks", "summary": "列出 Tasks", "responses": {"200": response(obj("Task 分页列表。", {"items": array("Tasks。", ref("Task")), "pagination": ref("Pagination")}, ("items", "pagination"))), "401": error}}},
        "/v1/tasks/{taskId}": {"get": {"operationId": "getTask", "summary": "读取 Task", "parameters": [{"$ref": "#/components/parameters/TaskId"}], "responses": {"200": response(ref("Task")), "404": error}}},
        "/v1/tasks/{taskId}/context": {"get": {"operationId": "getTaskContext", "summary": "读取 Task Context Pack", "parameters": [{"$ref": "#/components/parameters/TaskId"}], "responses": {"200": response(ref("TaskContextPack")), "404": error}}},
        "/v1/tasks/{taskId}/complete": {"post": {"operationId": "completeTask", "summary": "完成 Task", "parameters": [{"$ref": "#/components/parameters/TaskId"}, {"$ref": "#/components/parameters/IdempotencyKey"}], "requestBody": body(ref("TaskCompletion")), "responses": {"200": response(ref("Task")), "409": error}}},
        "/v1/runs/{runId}": {"get": {"operationId": "getRun", "summary": "读取 Run", "parameters": [{"$ref": "#/components/parameters/RunId"}], "responses": {"200": response(ref("Run")), "404": error}}},
        "/v1/schedules": {"get": {"operationId": "listSchedules", "summary": "列出计划任务", "responses": {"200": response(ref("ScheduleListResponse")), "401": error}}},
        "/v1/schedules/{scheduleId}": {"get": {"operationId": "getSchedule", "summary": "读取计划任务", "parameters": [{"$ref": "#/components/parameters/ScheduleId"}], "responses": {"200": response(ref("ScheduleItem")), "401": error, "404": error}}},
        "/v1/memory/context": {"post": {"operationId": "buildMemoryContext", "summary": "构建 Memory Context Pack", "requestBody": body(obj("Memory Context 请求。", {"task_id": scalar(["string", "null"], "Task ID。", format="uuid"), "project": scalar(["string", "null"], "项目键。"), "device_id": scalar(["string", "null"], "设备 ID。", format="uuid"), "skill_id": scalar(["string", "null"], "Skill ID。", format="uuid"), "max_bytes": scalar("integer", "最大字节数。", minimum=1024, maximum=1000000)}, ("max_bytes",))), "responses": {"200": response(ref("MemoryContextPack")), "400": error}}},
        "/v1/events": {"get": {"operationId": "streamEvents", "summary": "SSE 事件流", "parameters": [{"name": "Last-Event-ID", "in": "header", "required": False, "description": "断点续传事件 ID。", "schema": {"type": "string"}}], "responses": {"200": {"description": "text/event-stream 事件流。", "content": {"text/event-stream": {"schema": {"type": "string"}}}}, "401": error}}},
    }
    return {
        "openapi": "3.1.0",
        "info": {
            "title": "AgentDock Nexus API",
            "version": "1.0.0",
            "description": "AgentDock Nexus 公共 REST/SSE 契约。",
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


def skill_manifest_schema() -> dict[str, Any]:
    operation = obj(
        "Skill operation 声明。",
        {
            "name": scalar("string", "Operation 名。", pattern="^[a-z][a-z0-9._-]*$"),
            "description": scalar("string", "Operation 说明。", minLength=1),
            "inputSchema": obj("输入 JSON Schema。", {}, additional=True),
            "outputSchema": obj("输出 JSON Schema。", {}, additional=True),
            "timeoutSeconds": scalar("integer", "超时秒数。", minimum=1, maximum=86400),
        },
        ("name", "description", "inputSchema", "outputSchema", "timeoutSeconds"),
    )
    return {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "$id": "https://schemas.agentdock.dev/skill/v1/agentdock-skill-v1.json",
        "title": "AgentDock Skill Manifest V1",
        **obj(
            "AgentDock Skill 包清单。",
            {
                "apiVersion": {"const": "agentdock.dev/v1", "description": "Skill manifest API 版本。"},
                "kind": {"const": "Skill", "description": "固定资源类型。"},
                "metadata": obj(
                    "Skill 元数据。",
                    {
                        "name": scalar("string", "稳定 Skill 名。", pattern="^[a-z][a-z0-9-]{1,62}$"),
                        "version": scalar("string", "语义版本。", pattern=r"^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$"),
                        "displayName": scalar("string", "显示名称。", minLength=1, maxLength=120),
                        "description": scalar("string", "Skill 说明。", minLength=1, maxLength=2000),
                        "license": scalar("string", "SPDX 或许可证说明。"),
                        "homepage": scalar("string", "主页。", format="uri"),
                    },
                    ("name", "version", "displayName", "description"),
                ),
                "spec": obj(
                    "Skill 运行声明。",
                    {
                        "entrypoint": scalar("string", "包内相对入口路径。", pattern=r"^(?!/)(?!.*(?:^|/)\.\.(?:/|$)).+$"),
                        "operations": scalar("array", "Operations。", minItems=1, items=operation),
                        "compatibility": obj(
                            "兼容性。",
                            {
                                "platforms": scalar("array", "支持平台。", minItems=1, uniqueItems=True, items={"type": "string", "enum": ["darwin", "linux"]}),
                                "architectures": scalar("array", "支持架构。", minItems=1, uniqueItems=True, items={"type": "string", "enum": ["arm64", "amd64"]}),
                                "agentdock": scalar("string", "兼容的 AgentDock 版本约束。"),
                            },
                            ("platforms", "architectures", "agentdock"),
                        ),
                        "permissions": obj(
                            "显式权限声明。",
                            {
                                "filesystem": array("声明文件访问。", scalar("string", "路径或访问模式。")),
                                "network": array("声明网络目标。", scalar("string", "网络目标。")),
                                "secrets": array("Secret 逻辑名。", scalar("string", "Secret 名。")),
                                "commands": array("允许子进程命令。", scalar("string", "命令名。")),
                            },
                            ("filesystem", "network", "secrets", "commands"),
                        ),
                        "bindings": array("所需设备 Binding 名。", scalar("string", "Binding 名。")),
                        "verification": array("发布和回退后的验证项。", scalar("string", "验证项。")),
                    },
                    ("entrypoint", "operations", "compatibility", "permissions"),
                ),
            },
            ("apiVersion", "kind", "metadata", "spec"),
        ),
    }


EVENTS = {
    "device.status.changed": ("DeviceStatus", "设备状态变化。"),
    "command.status.changed": ("DeviceCommand", "命令状态变化。"),
    "task.created": ("Task", "Task 创建。"),
    "task.updated": ("Task", "Task 更新。"),
    "run.started": ("Run", "Run 开始。"),
    "run.completed": ("Run", "Run 完成。"),
    "skill.release.published": ("SkillRelease", "Skill Release 发布。"),
    "skill.installation.changed": ("SkillInstallation", "Skill 安装状态变化。"),
    "evolution.candidate.created": ("EvolutionCandidate", "进化候选创建。"),
    "memory.conflict.created": ("MemoryConflict", "Memory 冲突创建。"),
}


def event_schema(event_type: str, payload: str, description: str, schemas: dict[str, Any]) -> dict[str, Any]:
    return rewrite_refs(
        {
            "$schema": "https://json-schema.org/draft/2020-12/schema",
            "$id": f"https://schemas.agentdock.dev/nexus/events/v1/{event_type}.json",
            "title": event_type,
            **obj(
                description,
                {
                    "id": ID,
                    "type": {"const": event_type, "description": "冻结事件类型。"},
                    "version": {"const": 1, "description": "事件 Schema 版本。"},
                    "occurred_at": TIMESTAMP,
                    "producer": scalar("string", "事件生产者。"),
                    "subject": schemas["ObjectReference"],
                    "correlation_id": scalar(["string", "null"], "跨流程关联 ID。"),
                    "causation_id": scalar(["string", "null"], "直接原因事件 ID。"),
                    "data": {"$ref": f"#/components/schemas/{payload}"},
                },
                ("id", "type", "version", "occurred_at", "producer", "subject", "data"),
            ),
            "$defs": schemas,
        }
    )


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

func (c *Client) RunSkill(ctx context.Context, idempotencyKey string, request SkillRunRequest) (Run, error) {
	var response Run
	err := c.doJSON(ctx, http.MethodPost, "/v1/skill-runs", idempotencyKey, request, &response)
	return response, err
}

func (c *Client) GetTask(ctx context.Context, taskID string) (Task, error) {
	var response Task
	err := c.doJSON(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(taskID), "", nil, &response)
	return response, err
}

func (c *Client) GetTaskContext(ctx context.Context, taskID string) (TaskContextPack, error) {
	var response TaskContextPack
	err := c.doJSON(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(taskID)+"/context", "", nil, &response)
	return response, err
}

func (c *Client) GetRun(ctx context.Context, runID string) (Run, error) {
	var response Run
	err := c.doJSON(ctx, http.MethodGet, "/v1/runs/"+url.PathEscape(runID), "", nil, &response)
	return response, err
}

func (c *Client) ListSchedules(ctx context.Context) (ScheduleListResponse, error) {
	var response ScheduleListResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/schedules", "", nil, &response)
	return response, err
}

func (c *Client) GetSchedule(ctx context.Context, scheduleID string) (ScheduleItem, error) {
	var response ScheduleItem
	err := c.doJSON(ctx, http.MethodGet, "/v1/schedules/"+url.PathEscape(scheduleID), "", nil, &response)
	return response, err
}
'''


def compatibility_signature(schemas: dict[str, Any]) -> dict[str, Any]:
    result: dict[str, Any] = {"version": 1, "schemas": {}}
    for name, schema in sorted(schemas.items()):
        properties: dict[str, Any] = {}
        for prop_name, prop in sorted(schema.get("properties", {}).items()):
            properties[prop_name] = {
                "type": prop.get("type"),
                "ref": prop.get("$ref"),
                "enum": prop.get("enum"),
            }
        result["schemas"][name] = {
            "required": sorted(schema.get("required", [])),
            "properties": properties,
        }
    return result


def write_json(path: pathlib.Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main() -> None:
    schemas = build_schemas()
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
        write_json(CONTRACTS / "jsonschema" / f"{name}.json", standalone)
    write_json(CONTRACTS / "jsonschema" / "agentdock-skill-v1.json", skill_manifest_schema())
    for event_type, (payload, description) in EVENTS.items():
        write_json(CONTRACTS / "events" / f"{event_type}.json", event_schema(event_type, payload, description, schemas))
    write_json(
        CONTRACTS / "error-codes.json",
        {"version": 1, "codes": [{"code": code, "description": "稳定公共错误码。"} for code in ERROR_CODES]},
    )
    baseline = CONTRACTS / "compatibility" / "v1-baseline.json"
    if not baseline.exists():
        write_json(baseline, compatibility_signature(schemas))
    GENERATED.mkdir(parents=True, exist_ok=True)
    (GENERATED / "types.gen.go").write_text(generate_go(schemas), encoding="utf-8")
    (GENERATED / "client.gen.go").write_text(CLIENT_GO, encoding="utf-8")
    subprocess.run(
        ["gofmt", "-w", str(GENERATED / "types.gen.go"), str(GENERATED / "client.gen.go")],
        check=True,
    )
    print(f"generated {len(schemas)} DTO schemas, {len(EVENTS)} event schemas and Go client")


if __name__ == "__main__":
    main()
