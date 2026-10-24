#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.14"
# dependencies = []
# ///
"""Retain literal private lists from tracked base mise configuration."""

import shlex
import subprocess
import sys
import tomllib
from pathlib import Path, PurePosixPath


def git(*args: str) -> bytes:
    return subprocess.check_output(["git", *args], stderr=subprocess.DEVNULL)


def file_settings(base: str, path: PurePosixPath, source: str) -> dict[str, str | None]:
    if not isinstance(source, str) or any(c in source for c in ("{{", "$", "~")):
        sys.exit("base environment file paths must be literal strings")
    relative = PurePosixPath(source)
    if relative.is_absolute() or ".." in relative.parts:
        sys.exit("base environment files must stay inside the repository")
    root = path.parent
    if root.name == "conf.d":
        root = root.parent
    if root.name == "mise" and root.parent.name == ".config":
        root = root.parent.parent
    file = root / relative
    if file.suffix in {".json", ".toml", ".yaml", ".yml"}:
        sys.exit("unsupported base environment file format")
    record = git("ls-tree", "-z", base, "--", f":(literal){file}").rstrip(b"\0")
    mode, kind, oid = record.split(b"\t", 1)[0].decode().split()
    if kind != "blob" or mode == "120000":
        sys.exit("invalid base environment file")
    settings = {}
    for line in git("cat-file", "blob", oid).decode().splitlines():
        # Shell and environment-file escape rules differ; refuse ambiguous input.
        if "\\" in line:
            sys.exit("unsupported base environment file escapes")
        fields = [] if line.lstrip().startswith("#") else shlex.split(line)
        if fields and fields[0] == "export":
            fields = fields[1:]
        if not fields:
            continue
        if len(fields) != 1 or "=" not in fields[0]:
            sys.exit("unsupported base environment file assignment")
        name, value = fields[0].split("=", 1)
        settings[name] = value
    return settings


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
        env = config.get("env", {})
        settings = list(env.items())
        sources = env.get("_", {}).get("file", [])
        sources = [sources] if isinstance(sources, str) else sources
        if not isinstance(sources, list):
            sys.exit("unsupported base environment file source")
        for source in sources:
            settings.extend(file_settings(base, path, source).items())
        for name, setting in settings:
            if not name.endswith("_PRIVATE_STRINGS"):
                continue
            value = setting["value"] if isinstance(setting, dict) else setting
            if not isinstance(value, str) or "{{" in value or "$" in value:
                sys.exit("base private lists must be literal strings")
            for index, raw_entry in enumerate(value.split(","), 1):
                entry = raw_entry.strip()
                if "\n" in entry or "\r" in entry:
                    sys.exit("invalid base private list entry")
                if entry and entry not in seen:
                    print(f"{name} entry {index}\t{entry}")
                    seen.add(entry)


if __name__ == "__main__":
    main()
