#!/usr/bin/env python3
"""检查 NexusDock 仓库边界和不应回归的历史配置。"""

from __future__ import annotations

import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]

FORBIDDEN_WORKFLOW_ROOTS = (".agents", ".codex", ".trellis")
FORBIDDEN_TRACKED_PREFIXES = (
    "bin/",
    "nexus-data/",
    "recall/",
    "web/node_modules/",
    "web/tsconfig.tsbuildinfo",
)
FORBIDDEN_TRACKED_PARTS = ("/__pycache__/",)
LEGACY_AUTH_TOKENS = (
    "AGENTDOCK_NEXUS_",
    "NEXUS_USERNAME",
    "NEXUS_PASSWORD",
    "NEXUS_PASSWORD_HASH",
    "NEXUS_ACCESS_FILE",
    "pbkdf2-sha256",
    "EnsureLegacyAdmin",
    "StoreDir",
)
LEGACY_AUTH_PATHS = (
    ROOT / ".env.example",
    ROOT / "docker-compose.yml",
    ROOT / "README.md",
    ROOT / "deploy",
    ROOT / "internal" / "auth",
    ROOT / "internal" / "config",
    ROOT / "internal" / "core",
    ROOT / "internal" / "nexusapp",
)


def git_paths(*args: str) -> set[str]:
    result = subprocess.run(
        ["git", *args],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    return {line for line in result.stdout.splitlines() if line}


def repository_files() -> set[str]:
    tracked = git_paths("ls-files", "--cached")
    staged_deletions = git_paths(
        "diff", "--cached", "--name-only", "--diff-filter=D"
    )
    untracked = git_paths("ls-files", "--others", "--exclude-standard")
    return (tracked - staged_deletions) | untracked


def source_files(path: pathlib.Path):
    if path.is_file():
        yield path
        return
    if not path.exists():
        return
    for candidate in path.rglob("*"):
        if not candidate.is_file() or candidate.name.endswith("_test.go"):
            continue
        yield candidate


def main() -> int:
    errors: list[str] = []
    files = repository_files()

    for root_name in FORBIDDEN_WORKFLOW_ROOTS:
        if any(path == root_name or path.startswith(f"{root_name}/") for path in files):
            errors.append(f"Agent 本地工作流目录不应进入项目仓库: {root_name}")

    for path in files:
        if path.endswith(".pyc") or any(path.startswith(prefix) for prefix in FORBIDDEN_TRACKED_PREFIXES):
            errors.append(f"构建或缓存产物不应被 Git 跟踪: {path}")
        if any(part in f"/{path}" for part in FORBIDDEN_TRACKED_PARTS):
            errors.append(f"Python 缓存不应被 Git 跟踪: {path}")

    agents = (ROOT / "AGENTS.md").read_text(encoding="utf-8")
    if "Trellis" in agents or ".trellis" in agents:
        errors.append("AGENTS.md 仍依赖已退出的 Trellis 工作流")

    for base in LEGACY_AUTH_PATHS:
        for path in source_files(base):
            try:
                text = path.read_text(encoding="utf-8")
            except UnicodeDecodeError:
                continue
            for token in LEGACY_AUTH_TOKENS:
                if token in text:
                    errors.append(f"旧管理员配置重新进入当前代码: {path.relative_to(ROOT)}: {token}")

    if errors:
        for error in sorted(set(errors)):
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print("repository valid: project boundary, tracked files and current authentication model")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
