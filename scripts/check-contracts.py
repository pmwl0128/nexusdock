#!/usr/bin/env python3
"""Validate the current NexusDock product contract and generated output."""

from __future__ import annotations

import json
import pathlib
import subprocess
import sys
from typing import Any

ROOT = pathlib.Path(__file__).resolve().parents[1]
CONTRACTS = ROOT / "contracts"

REQUIRED_PATHS = {
    "/v1/backup/status",
    "/v1/recall",
    "/v1/runtime/nodes",
    "/v1/runtime/nodes/{nodeID}/tasks",
    "/v1/runtime/nodes/{nodeID}/skills",
    "/v1/runtime/nodes/{nodeID}/mcp",
    "/v1/workflow-templates",
}
FORBIDDEN_PATH_PREFIXES = (
    "/v1/" + "arti" + "facts",
    "/v1/" + "arti" + "fact-fetches",
    "/v1/devices/{deviceId}/" + "arti" + "facts",
    "/v1/devices/{deviceId}/" + "arti" + "fact",
    "/v1/tasks",
    "/v1/runs",
    "/v1/skills",
    "/v1/skill-runs",
    "/v1/ops",
    "/v1/runtime/tasks",
    "/v1/runtime/skills",
    "/v1/runtime/mcp",
    "/v1/runtime/overview",
    "/v1/runtime/workflow-templates",
    "/v1/runtime/capabilities",
    "/v1/events",
    "/v1/schedules",
)
FORBIDDEN_FIELDS = {"recall_root"}
FORBIDDEN_SCHEMAS = {
    "DeviceCapability",
    "DeviceEnrollmentRequest",
    "DeviceEnrollmentResponse",
    "EnrollmentTokenCreateRequest",
    "EnrollmentTokenCreateResponse",
    "CommandLeaseAction",
    "DeviceTokenRotationResponse",
    "DeviceRevokeRequest",
    "DeviceCommandCreateRequest",
    "DeviceEnvActionRequest",
    "DeviceHeartbeat",
    "DeviceStatus",
    "DeviceCommand",
    "CommandLease",
    "CommandProgress",
    "CommandResult",
}


def load(path: pathlib.Path) -> Any:
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


def collect_refs(value: Any) -> list[str]:
    refs: list[str] = []
    if isinstance(value, dict):
        for key, item in value.items():
            if key == "$ref" and isinstance(item, str):
                refs.append(item)
            refs.extend(collect_refs(item))
    elif isinstance(value, list):
        for item in value:
            refs.extend(collect_refs(item))
    return refs


def validate_descriptions(value: Any, path: str, errors: list[str]) -> None:
    if isinstance(value, dict):
        if "properties" in value:
            for name, prop in value["properties"].items():
                if not isinstance(prop, dict) or not prop.get("description") and "$ref" not in prop:
                    errors.append(f"{path}.properties.{name}: missing description")
        for key, item in value.items():
            validate_descriptions(item, f"{path}.{key}", errors)
    elif isinstance(value, list):
        for index, item in enumerate(value):
            validate_descriptions(item, f"{path}[{index}]", errors)


def validate_openapi(errors: list[str]) -> None:
    document = load(CONTRACTS / "openapi" / "nexus.yaml")
    if document.get("openapi") != "3.1.0":
        errors.append("OpenAPI version must be 3.1.0")
    schemas = document.get("components", {}).get("schemas", {})
    paths = document.get("paths", {})
    if not schemas:
        errors.append("OpenAPI components.schemas is empty")
    missing_paths = sorted(REQUIRED_PATHS - set(paths))
    if missing_paths:
        errors.append("missing current product paths: " + ", ".join(missing_paths))
    for route in paths:
        if route.startswith(FORBIDDEN_PATH_PREFIXES):
            errors.append(f"retired product path is still public: {route}")
    retired_schemas = sorted(FORBIDDEN_SCHEMAS & set(schemas))
    if retired_schemas:
        errors.append("retired product schemas are still public: " + ", ".join(retired_schemas))
    for schema_name, schema in schemas.items():
        properties = schema.get("properties", {}) if isinstance(schema, dict) else {}
        retired_fields = sorted(FORBIDDEN_FIELDS & set(properties))
        if retired_fields:
            errors.append(f"{schema_name}: retired fields still public: " + ", ".join(retired_fields))
    for reference in collect_refs(document):
        prefix = "#/components/schemas/"
        if reference.startswith(prefix) and reference[len(prefix):] not in schemas:
            errors.append(f"unresolved OpenAPI ref: {reference}")
    validate_descriptions({"schemas": schemas}, "components", errors)
    operation_ids: set[str] = set()
    for route, path_item in paths.items():
        for method, operation in path_item.items():
            operation_id = operation.get("operationId")
            if not operation_id:
                errors.append(f"{method.upper()} {route}: missing operationId")
            elif operation_id in operation_ids:
                errors.append(f"duplicate operationId: {operation_id}")
            else:
                operation_ids.add(operation_id)


def validate_json_schemas(errors: list[str]) -> None:
    schema_dir = CONTRACTS / "jsonschema"
    actual = {path.stem for path in schema_dir.glob("*.json")}
    retired = sorted(FORBIDDEN_SCHEMAS & actual)
    if retired:
        errors.append("retired standalone schemas remain: " + ", ".join(retired))
    for path in sorted(schema_dir.glob("*.json")):
        document = load(path)
        if document.get("$schema") != "https://json-schema.org/draft/2020-12/schema":
            errors.append(f"{path.name}: wrong or missing $schema")
        if not document.get("$id"):
            errors.append(f"{path.name}: missing $id")
        defs = document.get("$defs", {})
        properties = document.get("properties", {}) if isinstance(document, dict) else {}
        retired_fields = sorted(FORBIDDEN_FIELDS & set(properties))
        if retired_fields:
            errors.append(f"{path.name}: retired fields still public: " + ", ".join(retired_fields))
        for def_name, definition in defs.items():
            properties = definition.get("properties", {}) if isinstance(definition, dict) else {}
            retired_fields = sorted(FORBIDDEN_FIELDS & set(properties))
            if retired_fields:
                errors.append(f"{path.name}#$defs/{def_name}: retired fields still public: " + ", ".join(retired_fields))
        for reference in collect_refs(document):
            prefix = "#/$defs/"
            if reference.startswith(prefix) and reference[len(prefix):] not in defs:
                errors.append(f"{path.name}: unresolved ref {reference}")


def validate_retired_contract_dirs(errors: list[str]) -> None:
    for name in ("events", "compatibility"):
        directory = CONTRACTS / name
        if directory.exists() and any(directory.iterdir()):
            errors.append(f"retired active contract directory remains: contracts/{name}")


def validate_generated_boundary(errors: list[str]) -> None:
    generated = ROOT / "generated" / "nexuscontracts"
    text = "\n".join(path.read_text(encoding="utf-8") for path in generated.glob("*.go"))
    forbidden_tokens = [
        "type Task ", "type Run ", "type Evolution", "type SkillRelease ",
        "type NexusTask", "type NexusSkillRegistry", "type NexusWorkflowLifecycle",
        "RunSkill(", "GetTask(", "GetRun(", "ListSchedules(", "GetSchedule(",
        "EnrollDevice(", "CreateEnrollmentToken(", "CreateDeviceCommand(", "ReportDeviceHeartbeat(",
        "LeaseDeviceCommand(", "StartCommand(", "RenewCommandLease(", "CompleteCommand(",
        "type Device", "type CommandLease", "type CommandProgress", "type CommandResult",
        "RecallRoot", "recall_root",
    ]
    for token in forbidden_tokens:
        if token in text:
            errors.append(f"generated client still exposes retired concept: {token.strip()}")


def snapshot_generated() -> dict[str, bytes]:
    roots = [
        CONTRACTS / "openapi" / "nexus.yaml",
        CONTRACTS / "jsonschema",
        CONTRACTS / "error-codes.json",
        ROOT / "generated" / "nexuscontracts",
    ]
    result: dict[str, bytes] = {}
    for root in roots:
        if root.is_file():
            result[str(root.relative_to(ROOT))] = root.read_bytes()
        elif root.exists():
            for path in sorted(item for item in root.rglob("*") if item.is_file()):
                result[str(path.relative_to(ROOT))] = path.read_bytes()
    return result


def validate_generated_drift(errors: list[str]) -> None:
    before = snapshot_generated()
    result = subprocess.run(
        [sys.executable, str(ROOT / "scripts" / "generate-contracts.py")],
        cwd=ROOT,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        errors.append("generator failed: " + result.stderr.strip())
        return
    after = snapshot_generated()
    if before != after:
        errors.append("generated contract output was stale; regenerate and commit it")


def main() -> int:
    errors: list[str] = []
    validate_openapi(errors)
    validate_json_schemas(errors)
    validate_retired_contract_dirs(errors)
    validate_generated_boundary(errors)
    validate_generated_drift(errors)
    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1
    print("contracts valid: current Nexus paths, schemas, product boundary and generated output")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
