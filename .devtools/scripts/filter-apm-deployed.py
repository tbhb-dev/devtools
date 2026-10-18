#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.14"
# dependencies = ["pyyaml==6.0.3"]
# ///
"""Remove exact APM deployment paths from a NUL-separated Git file list."""

import os
import sys
from pathlib import Path

import yaml


def main() -> None:
    """Keep first-party files, including additions inside deployed directories."""
    lockfile = Path("apm.lock.yaml")
    if not lockfile.exists():
        return
    lock = yaml.safe_load(lockfile.read_text())
    deployed = {
        os.fsencode(path)
        for dependency in lock.get("dependencies", [])
        for path in dependency.get("deployed_files", [])
    }
    selected = Path(sys.argv[1])
    files = selected.read_bytes().split(b"\0")
    selected.write_bytes(
        b"".join(path + b"\0" for path in files if path and path not in deployed)
    )


if __name__ == "__main__":
    main()
