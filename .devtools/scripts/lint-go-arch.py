#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.14"
# dependencies = ["pyyaml==6.0.3"]
# ///
"""Keep ignored and consumer-generated Go files out of architecture checks."""

import os
import re
import subprocess
import tempfile
from pathlib import Path

import yaml


def main() -> int:
    """Extend the architecture config without modifying the consumer's file."""
    exclusions = []
    for entry in os.environ.get("DEVTOOLS_GENERATED_PATHS", "").splitlines():
        if entry:
            path = entry.rstrip("/")
            exclusions.extend([f":(exclude){path}", f":(exclude){path}/**"])
    selected = subprocess.check_output(
        [
            "git",
            "ls-files",
            "-z",
            "--cached",
            "--others",
            "--exclude-standard",
            "--",
            "*.go",
            *exclusions,
        ],
    ).split(b"\0")
    owned = {os.fsdecode(path) for path in selected if path}
    config = yaml.safe_load(Path(".go-arch-lint.yml").read_text())
    excluded = config.setdefault("excludeFiles", [])
    for path in Path().rglob("*.go"):
        if path.as_posix() not in owned:
            excluded.append(re.escape(str(path.resolve())) + "$")
    with tempfile.TemporaryDirectory(prefix="devtools-arch-") as directory:
        arch_file = Path(directory) / "arch.yml"
        arch_file.write_text(yaml.safe_dump(config, sort_keys=False))
        # Go may preserve a symlinked PWD; use the same root as the exclusions.
        return subprocess.call(
            [
                "go-arch-lint",
                "check",
                "--project-path",
                str(Path.cwd()),
                "--arch-file",
                str(arch_file),
            ],
        )


if __name__ == "__main__":
    raise SystemExit(main())
