from __future__ import annotations

import re
import subprocess
from collections import Counter
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[2]
AREAS = (
    "packages/agents/source",
    ".devtools",
    "tools",
    ".claude/skills",
    "scripts",
    "tests",
)


def maintained(root: Path) -> set[str]:
    paths = subprocess.check_output(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "--", *AREAS],
        cwd=root,
        text=True,
    ).splitlines()
    result = set()
    for name in paths:
        path = Path(name)
        if "fixtures" in path.parts or (
            path.name.startswith("test_")
            and path.suffix == ".py"
            and name != "tests/skills/test_content_posts.py"
        ):
            continue
        if (
            name.startswith(".claude/skills/")
            and (root / "packages/agents/source/skills" / path.parts[2]).is_dir()
        ):
            continue
        if path.suffix in {".sh", ".bash", ".py"} or name == "tools/mise-bootstrap":
            result.add(name)
    return result


def classification(source: str) -> str:
    """A thin shim only sets execution context and forwards all arguments."""
    lines = [line.strip() for line in source.splitlines()]
    code = [line for line in lines if line and not line.startswith("#")]
    preparation = re.compile(
        r"set -[a-zA-Z ]+|export [A-Z_]+=\S+|"
        r'here=\$\(cd "\$\(dirname "\$\{BASH_SOURCE\[0\]\}"\)" && pwd\)'
    )
    if (
        code
        and re.fullmatch(r'exec .+ "\$@"', code[-1])
        and all(preparation.fullmatch(line) for line in code[:-1])
    ):
        return "thin"
    return "logic"


def inventory_errors(root: Path, document: str) -> list[str]:
    rows = [
        [cell.strip() for cell in line.strip().strip("|").split("|")]
        for line in document.splitlines()
        if line.startswith("| `")
    ]
    counts = Counter(row[0].strip("`") for row in rows)
    expected = maintained(root)
    errors = [f"{path}: missing row" for path in sorted(expected - counts.keys())]
    for row in rows:
        path = row[0].strip("`")
        if len(row) != 7:
            errors.append(f"{path}: expected seven columns")
            continue
        _, owner, count, kind, go, coverage, reason = row
        if counts[path] != 1:
            errors.append(f"{path}: duplicate row")
        if path not in expected:
            errors.append(f"{path}: not a maintained file")
        if not re.fullmatch(r"#[1-9][0-9]*", owner):
            errors.append(f"{path}: expected exactly one owning issue")
        subject = root / path
        if not subject.is_file():
            errors.append(f"{path}: missing file")
        else:
            if count != str(subject.read_bytes().count(b"\n")):
                errors.append(f"{path}: newline count differs")
            actual = (
                "logic" if path.endswith(".py") else classification(subject.read_text())
            )
            if kind != actual:
                errors.append(f"{path}: expected {actual}, got {kind}")
        if kind not in {"thin", "logic"}:
            errors.append(f"{path}: invalid classification")
        errors.extend(reference_errors(root, path, go, coverage))
        if not reason or (
            (go == "none" or coverage == "none") and "none:" not in reason
        ):
            errors.append(f"{path}: justify none in the row")
    return errors


def reference_errors(root: Path, path: str, go: str, coverage: str) -> list[str]:
    errors = []
    errors.extend(
        f"{path}: missing Go counterpart {counterpart}"
        for counterpart in re.findall(r"`([^`]+)`", go)
        if not counterpart.endswith(".go") or not (root / counterpart).is_file()
    )
    if go != "none" and not re.search(r"`[^`]+\.go`", go):
        errors.append(f"{path}: expected Go path or none")
    if path.endswith(".py") and coverage != "none":
        suites = re.findall(r"`([^`]+)`", coverage)
        if not suites or any(
            not Path(suite).name.startswith("test_")
            or not suite.endswith(".py")
            or not (root / suite).is_file()
            for suite in suites
        ):
            errors.append(f"{path}: missing pytest file")
    return errors


def test_inventory_matches_tree() -> None:
    document = (ROOT / "docs/script-inventory.md").read_text()
    errors = inventory_errors(ROOT, document)
    assert not errors, "\n".join(errors)


@pytest.fixture
def inventory_tree(tmp_path: Path) -> Path:
    subprocess.run(["git", "init", "-q", str(tmp_path)], check=True)
    (tmp_path / "tools").mkdir()
    (tmp_path / "tools/sample.sh").write_text('#!/bin/bash\nexec tool "$@"\n')
    return tmp_path


def row(**changes: str) -> str:
    values = {
        "path": "`tools/sample.sh`",
        "owner": "#588",
        "count": "2",
        "kind": "thin",
        "go": "none",
        "coverage": "n/a",
        "reason": "none: shell forwarding has no Go implementation.",
    }
    values.update(changes)
    return "| " + " | ".join(values.values()) + " |"


@pytest.mark.parametrize(
    ("document", "message"),
    [
        ("", "missing row"),
        (row(owner=""), "exactly one owning issue"),
        (row(owner="#588 #713"), "exactly one owning issue"),
        (row() + "\n" + row(), "duplicate row"),
        (row(path="`tools/missing.sh`"), "tools/missing.sh: not a maintained file"),
        (row(count="3"), "newline count differs"),
        (row(kind="logic"), "expected thin"),
        (row(kind="other"), "invalid classification"),
        (row(go="`internal/missing.go`"), "missing Go counterpart"),
        (row(go="agentrepo"), "expected Go path or none"),
        (row(reason=""), "justify none"),
    ],
)
def test_invalid_inventory_names_path(
    inventory_tree: Path, document: str, message: str
) -> None:
    errors = inventory_errors(inventory_tree, document)
    assert any(message in error for error in errors)
    assert all("tools/" in error for error in errors)


def test_valid_inventory_and_unterminated_line(inventory_tree: Path) -> None:
    assert inventory_errors(inventory_tree, row()) == []
    path = inventory_tree / "tools/sample.sh"
    path.write_text('#!/bin/bash\nexec tool "$@"')
    count = subprocess.check_output(["wc", "-l", str(path)], text=True).split()[0]
    assert count == "1"
    assert inventory_errors(inventory_tree, row(count=count)) == []


def test_logic_label_and_missing_pytest(inventory_tree: Path) -> None:
    (inventory_tree / "tools/sample.sh").write_text("echo logic\n")
    assert any(
        "tools/sample.sh: expected logic" in error
        for error in inventory_errors(inventory_tree, row(count="1"))
    )
    (inventory_tree / "tools/sample.py").write_text("print('helper')\n")
    document = (
        row(count="1", kind="logic")
        + "\n"
        + row(
            path="`tools/sample.py`",
            count="1",
            kind="logic",
            coverage="`tests/test_missing.py`",
        )
    )
    assert inventory_errors(inventory_tree, document) == [
        "tools/sample.py: missing pytest file"
    ]


def test_tracked_missing_file_names_path(inventory_tree: Path) -> None:
    subprocess.run(
        ["git", "add", "--", "tools/sample.sh"], cwd=inventory_tree, check=True
    )
    (inventory_tree / "tools/sample.sh").unlink()
    assert inventory_errors(inventory_tree, row()) == ["tools/sample.sh: missing file"]
