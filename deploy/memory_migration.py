#!/usr/bin/env python3
from __future__ import annotations
import argparse
import hashlib
import json
from pathlib import Path
import shutil
from datetime import datetime, timezone


def files(root: Path):
    for path in sorted(root.rglob("*")):
        if path.is_file() and ".git" not in path.parts and path.name not in {".DS_Store", "MIGRATION_MANIFEST.json"}:
            yield path


def sha256(path: Path) -> str:
    value = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def snapshot(root: Path) -> dict[str, str]:
    return {path.relative_to(root).as_posix(): sha256(path) for path in files(root)}


def require_dir(path: Path, label: str) -> Path:
    path = path.expanduser().resolve()
    if not path.is_dir():
        raise SystemExit(f"{label} is not a directory: {path}")
    return path


def create_backup(source: Path, backup_root: Path) -> Path:
    source = require_dir(source, "source")
    backup_root = backup_root.expanduser().resolve()
    backup_root.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    destination = backup_root / f"memorydock-{stamp}"
    shutil.copytree(source, destination, symlinks=True, ignore=shutil.ignore_patterns(".git"))
    manifest = {"format": 1, "created_at": datetime.now(timezone.utc).isoformat(), "source": str(source), "files": snapshot(destination)}
    (destination / "MIGRATION_MANIFEST.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return destination


def verify_backup(backup_dir: Path) -> None:
    backup_dir = require_dir(backup_dir, "backup")
    manifest_path = backup_dir / "MIGRATION_MANIFEST.json"
    if not manifest_path.is_file():
        raise SystemExit(f"manifest missing: {manifest_path}")
    expected = dict(json.loads(manifest_path.read_text(encoding="utf-8")).get("files") or {})
    actual = snapshot(backup_dir)
    if expected != actual:
        missing = sorted(set(expected) - set(actual))
        extra = sorted(set(actual) - set(expected))
        changed = sorted(key for key in set(expected) & set(actual) if expected[key] != actual[key])
        raise SystemExit(f"verification failed: missing={missing} extra={extra} changed={changed}")


def restore_backup(backup_dir: Path, destination: Path, confirmed: bool, replace: bool) -> None:
    if not confirmed:
        raise SystemExit("restore requires --confirmed")
    backup_dir = require_dir(backup_dir, "backup")
    verify_backup(backup_dir)
    destination = destination.expanduser().resolve()
    if destination.exists() and any(destination.iterdir()):
        if not replace:
            raise SystemExit(f"destination is not empty: {destination}; pass --replace after stopping Nexus")
        shutil.rmtree(destination)
    destination.mkdir(parents=True, exist_ok=True)
    for source in files(backup_dir):
        relative = source.relative_to(backup_dir)
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)


def main() -> int:
    parser = argparse.ArgumentParser(description="MemoryDock migration backup, verification and rollback helper")
    sub = parser.add_subparsers(dest="command", required=True)
    backup_parser = sub.add_parser("backup")
    backup_parser.add_argument("source", type=Path)
    backup_parser.add_argument("backup_root", type=Path)
    verify_parser = sub.add_parser("verify")
    verify_parser.add_argument("backup", type=Path)
    restore_parser = sub.add_parser("restore")
    restore_parser.add_argument("backup", type=Path)
    restore_parser.add_argument("destination", type=Path)
    restore_parser.add_argument("--confirmed", action="store_true")
    restore_parser.add_argument("--replace", action="store_true")
    args = parser.parse_args()

    if args.command == "backup":
        print(create_backup(args.source, args.backup_root))
    elif args.command == "verify":
        verify_backup(args.backup)
        print("verification passed")
    else:
        restore_backup(args.backup, args.destination, args.confirmed, args.replace)
        print("restore completed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
