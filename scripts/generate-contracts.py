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
        "SessionId": path_param("sessionID", "浏览器 Session ID。", uuid=False),
        "RecallPath": path_param("path", "URL 编码后的召回相对路径。", uuid=False),
        "RuntimeTaskId": path_param("taskID", "AgentDock Runtime task ID。", uuid=False),
        "RuntimeSkillSource": path_param("source", "AgentDock Runtime skill source。", uuid=False),
        "RuntimeSkillId": path_param("skillID", "AgentDock Runtime skill ID。", uuid=False),
        "RuntimeWorkflowLocation": path_param("location", "Runtime workflow template location。", uuid=False),
        "RuntimeWorkflowFileName": path_param("fileName", "Runtime workflow template file name。", uuid=False),
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
        "/v1/sync/status": {"get": operation("getSyncStatus", "读取召回 Git 同步状态")},
        "/v1/git/diff": {"get": operation("getGitDiff", "读取召回仓库变更")},
        "/v1/git/discard": {"post": operation("discardGitChanges", "丢弃召回仓库本地变更", request=body())},
        "/v1/git/log": {"get": operation("getGitLog", "读取召回仓库提交历史")},
        "/v1/git/commit": {"get": operation("getGitCommit", "读取召回仓库提交详情")},
        "/v1/sync/pull": {"post": operation("pullRecall", "从远端更新召回仓库", request=body())},
        "/v1/sync/push": {"post": operation("pushRecall", "保存召回仓库到远端", request=body())},
        "/v1/sync/now": {"post": operation("syncRecallNow", "立即双向同步召回仓库", request=body())},
        "/v1/runtime/overview": {"get": operation("getRuntimeOverview", "读取 AgentDock Runtime 可用性和概览")},
        "/v1/runtime/tasks": {"get": operation("listRuntimeTasks", "通过 AgentDock Runtime API 列出任务视图")},
        "/v1/runtime/tasks/{taskID}": {"get": operation("getRuntimeTask", "通过 AgentDock Runtime API 读取任务详情", params=[p("RuntimeTaskId")])},
        "/v1/runtime/skills": {"get": operation("listRuntimeSkills", "通过 AgentDock Runtime API 列出 Skill 视图")},
        "/v1/runtime/skills/{source}/{skillID}": {"get": operation("getRuntimeSkill", "通过 AgentDock Runtime API 读取 Skill 详情", params=[p("RuntimeSkillSource"), p("RuntimeSkillId")])},
        "/v1/runtime/workflow-templates": {"get": operation("listRuntimeWorkflowTemplates", "通过 AgentDock Runtime API 列出 Workflow 模板视图")},
        "/v1/runtime/workflow-templates/{location}/{fileName}": {"get": operation("getRuntimeWorkflowTemplate", "通过 AgentDock Runtime API 读取 Workflow 模板详情", params=[p("RuntimeWorkflowLocation"), p("RuntimeWorkflowFileName")])},
        "/v1/runtime/capabilities": {"get": operation("listRuntimeCapabilities", "读取 AgentDock Runtime 能力视图")},
    }

    return {
        "openapi": "3.1.0",
        "info": {
            "title": "NexusDock API",
            "version": "1.0.0",
            "description": "个人 NexusDock 控制台的当前 HTTP 契约，覆盖设备、Recall、加密文件、备份、账号会话和 AgentDock Runtime 视图。",
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
