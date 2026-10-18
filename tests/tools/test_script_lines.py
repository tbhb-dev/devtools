from __future__ import annotations

import runpy
import subprocess
from pathlib import Path

import pytest

SCRIPT = Path(__file__).resolve().parents[2] / "tools/script_lines.py"
SUBJECT = runpy.run_path(str(SCRIPT))


@pytest.mark.parametrize(
    ("source", "expected"),
    [
        ("", 0),
        (" \n\t\n# comment\n  # indented\n#!/bin/bash\n", 0),
        ('echo "# literal" # comment\nvalue=1\n', 2),
        ("if true; then\n  :\nfi", 3),
        ("echo a\\\n  b\r\n", 2),
        ("cat <<'EOF'\n# data\n\ntext\nEOF\n", 3),
    ],
)
def test_physical_line_rule(source: str, expected: int) -> None:
    assert SUBJECT["count_lines"](source) == expected


def test_cli_counts_each_path(tmp_path: Path) -> None:
    first = tmp_path / "a file.sh"
    second = tmp_path / "empty.sh"
    first.write_text("# comment\necho yes\n", encoding="utf-8")
    second.write_text("", encoding="utf-8")
    result = subprocess.run(
        ["uv", "run", "python", str(SCRIPT), str(first), str(second)],
        capture_output=True,
        text=True,
        check=True,
    )
    assert result.stdout == f"1\t{first}\n0\t{second}\n"


def test_cli_requires_paths() -> None:
    result = subprocess.run(
        ["uv", "run", "python", str(SCRIPT)],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 2
    assert "paths" in result.stderr


def test_main_reads_utf8_path(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    path = tmp_path / "sample.sh"
    path.write_text("echo café\n", encoding="utf-8")
    monkeypatch.setattr("sys.argv", [str(SCRIPT), str(path)])
    SUBJECT["main"]()
    assert capsys.readouterr().out == f"1\t{path}\n"
