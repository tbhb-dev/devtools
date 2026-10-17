#!/usr/bin/env python3
"""Run APM's CI audit without crossing registered Git worktrees."""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path
from typing import TypedDict, cast


class AuditCheck(TypedDict):
    """One check from APM's JSON audit report."""

    name: str
    passed: bool
    message: str
    details: list[str]


class AuditReport(TypedDict):
    """The part of APM's JSON audit report this wrapper consumes."""

    checks: list[AuditCheck]


def registered_nested_worktrees(project_root: Path) -> tuple[Path, ...]:
    """Return registered worktrees contained by *project_root*, excluding it."""
    proc = subprocess.run(
        ["git", "worktree", "list", "--porcelain"],
        cwd=project_root,
        check=True,
        capture_output=True,
        text=True,
    )
    roots: list[Path] = []
    resolved_root = project_root.resolve()
    for line in proc.stdout.splitlines():
        if not line.startswith("worktree "):
            continue
        candidate = Path(line.removeprefix("worktree ")).resolve()
        if candidate == resolved_root:
            continue
        try:
            candidate.relative_to(resolved_root)
        except ValueError:
            continue
        roots.append(candidate)
    return tuple(roots)


def suppressible_worktree_findings(
    report: AuditReport, project_root: Path, worktrees: tuple[Path, ...]
) -> tuple[bool, tuple[str, ...]]:
    """Return whether the report fails only on Unicode inside nested worktrees."""
    failed = [
        check for check in report.get("checks", []) if not check.get("passed", False)
    ]
    if not failed:
        return True, ()
    if len(failed) != 1 or failed[0].get("name") != "content-integrity":
        return False, ()

    details = failed[0].get("details", [])
    if not details:
        return False, ()

    ignored: list[str] = []
    for detail in details:
        if not detail.startswith("unicode: "):
            return False, ()
        candidate = (project_root / detail.removeprefix("unicode: ")).resolve()
        if not any(candidate.is_relative_to(worktree) for worktree in worktrees):
            return False, ()
        ignored.append(detail)
    return True, tuple(ignored)


def render_report(report: AuditReport, ignored: tuple[str, ...]) -> None:
    """Print a compact audit result, marking excluded worktree findings."""
    for check in report.get("checks", []):
        name = check.get("name", "unknown")
        message = check.get("message", "")
        if name == "content-integrity" and ignored:
            print(
                f"[!] {name}: excluded {len(ignored)} nested-worktree "
                "Unicode finding(s)"
            )
            for detail in ignored:
                print(f"    - {detail}")
            continue
        symbol = "[+]" if check.get("passed", False) else "[x]"
        print(f"{symbol} {name}: {message}")
        if not check.get("passed", False):
            for detail in check.get("details", []):
                print(f"    - {detail}")


def main() -> int:
    """Run APM, preserve its checks, and exclude registered worktree findings."""
    project_root = Path.cwd().resolve()
    proc = subprocess.run(
        ["apm", "audit", "--ci", "--no-policy", "--format", "json"],
        cwd=project_root,
        check=False,
        capture_output=True,
        text=True,
    )
    sys.stderr.write(proc.stderr)
    try:
        report = cast("AuditReport", json.loads(proc.stdout))
    except json.JSONDecodeError:
        sys.stdout.write(proc.stdout)
        return proc.returncode or 1

    try:
        worktrees = registered_nested_worktrees(project_root)
    except subprocess.CalledProcessError as exc:
        print(
            f"[x] Could not enumerate registered Git worktrees: {exc}", file=sys.stderr
        )
        return 1

    passed, ignored = suppressible_worktree_findings(report, project_root, worktrees)
    render_report(report, ignored)
    clean = proc.returncode == 0 and passed and not ignored
    excluded_only = proc.returncode == 1 and passed and bool(ignored)
    if clean or excluded_only:
        print(
            f"[*] APM audit passed with {len(ignored)} nested-worktree "
            "finding(s) excluded"
        )
        return 0
    return proc.returncode or 1


if __name__ == "__main__":
    raise SystemExit(main())
