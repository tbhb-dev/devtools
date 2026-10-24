#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.14"
# dependencies = []
# ///
"""Retain literal private lists from tracked base mise configuration."""

import subprocess
import sys
import tomllib
from pathlib import Path, PurePosixPath


def git(*args: str) -> bytes:
    return subprocess.check_output(["git", *args], stderr=subprocess.DEVNULL)


def main() -> None:
    base, needles = sys.argv[1:]
    seen = {line.split("\t", 1)[1] for line in Path(needles).read_text().splitlines()}
    for record in git("ls-tree", "-rz", base).split(b"\0"):
        if not record:
            continue
        meta, raw_path = record.split(b"\t", 1)
        path = PurePosixPath(raw_path.decode(errors="replace"))
        if not (
            path.match("mise*.toml")
            or path.match(".mise*.toml")
            or path.match(".config/mise/config.toml")
            or path.match(".config/mise/conf.d/*.toml")
        ):
            continue
        mode, kind, oid = meta.decode().split()
        if kind != "blob" or mode == "120000":
            sys.exit("invalid base mise configuration")
        config = tomllib.loads(git("cat-file", "blob", oid).decode())
        for name, setting in config.get("env", {}).items():
            if not name.endswith("_PRIVATE_STRINGS"):
                continue
            value = setting["value"] if isinstance(setting, dict) else setting
            if not isinstance(value, str) or "{{" in value:
                sys.exit("base private lists must be literal strings")
            for index, raw_entry in enumerate(value.split(","), 1):
                entry = raw_entry.strip()
                if "\n" in entry or "\r" in entry:
                    sys.exit("invalid base private list entry")
                if entry and entry not in seen:
                    print(f"{name} base entry {index}\t{entry}")
                    seen.add(entry)


if __name__ == "__main__":
    main()
