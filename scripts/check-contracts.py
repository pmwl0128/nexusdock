#!/usr/bin/env python3
"""Validate Nexus contracts, generated output and v1 compatibility."""

from __future__ import annotations

import json
import pathlib
import subprocess
import sys
from typing import Any


ROOT = pathlib.Path(__file__).resolve().parents[1]
CONTRACTS = ROOT / "contracts"


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


def validate_openapi(errors: list[str]) -> dict[str, Any]:
    path = CONTRACTS / "openapi" / "nexus.yaml"
    document = load(path)
    if document.get("openapi") != "3.1.0":
        errors.append("OpenAPI version must be 3.1.0")
    schemas = document.get("components", {}).get("schemas", {})
    if not schemas:
        errors.append("OpenAPI components.schemas is empty")
    for reference in collect_refs(document):
        prefix = "#/components/schemas/"
        if reference.startswith(prefix) and reference[len(prefix):] not in schemas:
            errors.append(f"unresolved OpenAPI ref: {reference}")
    validate_descriptions({"schemas": schemas}, "components", errors)
    operation_ids: set[str] = set()
    for route, path_item in document.get("paths", {}).items():
        for method, operation in path_item.items():
            operation_id = operation.get("operationId")
            if not operation_id:
                errors.append(f"{method.upper()} {route}: missing operationId")
            elif operation_id in operation_ids:
                errors.append(f"duplicate operationId: {operation_id}")
            else:
                operation_ids.add(operation_id)
    return schemas


def validate_json_schemas(errors: list[str]) -> None:
    for path in sorted((CONTRACTS / "jsonschema").glob("*.json")):
        document = load(path)
        if document.get("$schema") != "https://json-schema.org/draft/2020-12/schema":
            errors.append(f"{path.name}: wrong or missing $schema")
        if not document.get("$id"):
            errors.append(f"{path.name}: missing $id")
        defs = document.get("$defs", {})
        for reference in collect_refs(document):
            prefix = "#/$defs/"
            if reference.startswith(prefix) and reference[len(prefix):] not in defs:
                errors.append(f"{path.name}: unresolved ref {reference}")


def validate_events(errors: list[str]) -> None:
    required = {
        "device.status.changed",
        "command.status.changed",
        "task.created",
        "task.updated",
        "run.started",
        "run.completed",
        "skill.release.published",
        "skill.installation.changed",
        "evolution.candidate.created",
        "memory.conflict.created",
    }
    actual = {path.stem for path in (CONTRACTS / "events").glob("*.json")}
    missing = sorted(required - actual)
    if missing:
        errors.append("missing event schemas: " + ", ".join(missing))
    for path in sorted((CONTRACTS / "events").glob("*.json")):
        document = load(path)
        props = document.get("properties", {})
        if props.get("type", {}).get("const") != path.stem:
            errors.append(f"{path.name}: event type const mismatch")
        if props.get("version", {}).get("const") != 1:
            errors.append(f"{path.name}: event version must be 1")


def validate_compatibility(current: dict[str, Any], errors: list[str]) -> None:
    baseline = load(CONTRACTS / "compatibility" / "v1-baseline.json")
    for schema_name, old_schema in baseline.get("schemas", {}).items():
        new_schema = current.get(schema_name)
        if new_schema is None:
            errors.append(f"breaking: removed schema {schema_name}")
            continue
        old_properties = old_schema.get("properties", {})
        new_properties = new_schema.get("properties", {})
        for property_name, old_property in old_properties.items():
            if property_name not in new_properties:
                errors.append(f"breaking: removed {schema_name}.{property_name}")
                continue
            new_property = new_properties[property_name]
            if old_property.get("type") != new_property.get("type"):
                errors.append(f"breaking: changed type of {schema_name}.{property_name}")
            if old_property.get("ref") != new_property.get("$ref"):
                errors.append(f"breaking: changed ref of {schema_name}.{property_name}")
            old_enum = old_property.get("enum")
            new_enum = new_property.get("enum")
            if old_enum and new_enum and not set(old_enum).issubset(new_enum):
                errors.append(f"breaking: removed enum value from {schema_name}.{property_name}")
        old_required = set(old_schema.get("required", []))
        new_required = set(new_schema.get("required", []))
        added_required = new_required - old_required
        if added_required:
            errors.append(f"breaking: added required fields to {schema_name}: {sorted(added_required)}")


def validate_generated_drift(errors: list[str]) -> None:
    tracked = [
        ROOT / "contracts" / "openapi" / "nexus.yaml",
        ROOT / "contracts" / "jsonschema",
        ROOT / "contracts" / "events",
        ROOT / "contracts" / "error-codes.json",
        ROOT / "generated" / "nexuscontracts",
    ]
    before = {path: path.read_bytes() for item in tracked for path in ([item] if item.is_file() else sorted(item.glob("*.json")) + sorted(item.glob("*.go")))}
    result = subprocess.run([sys.executable, str(ROOT / "scripts" / "generate-contracts.py")], cwd=ROOT, capture_output=True, text=True)
    if result.returncode != 0:
        errors.append("generator failed: " + result.stderr.strip())
        return
    after = {path: path.read_bytes() for item in tracked for path in ([item] if item.is_file() else sorted(item.glob("*.json")) + sorted(item.glob("*.go")))}
    if before != after:
        errors.append("generated contract output was stale; regenerate and commit it")


def main() -> int:
    errors: list[str] = []
    current = validate_openapi(errors)
    validate_json_schemas(errors)
    validate_events(errors)
    validate_compatibility(current, errors)
    validate_generated_drift(errors)
    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1
    print("contracts valid: OpenAPI, JSON Schema, events, compatibility and generated output")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

