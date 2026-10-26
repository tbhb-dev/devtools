#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.14"
# dependencies = []
# ///
"""Resolve private lists without evaluating repository configuration."""

import os
import re
import subprocess
import sys
import tomllib
from pathlib import Path
from typing import NoReturn


def read_command(*args: str) -> str:
    """Capture private output and discard diagnostics that may contain values."""
    return subprocess.run(
        args, check=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL
    ).stdout.decode()


def machine_file(key: str, main: Path) -> str:
    """Preserve the shared lookup's fallback for the optional agentrepo binary."""
    override = os.environ.get("DEVTOOLS_" + key.upper())
    if override is not None:
        return override
    directory = Path(os.environ.get("XDG_CONFIG_HOME") or Path.home() / ".config")
    for name in (main.name + ".conf", "devtools.conf"):
        path = directory / "devtools" / name
        if not path.is_file():
            continue
        for line in path.read_text().splitlines():
            if line.lstrip().startswith("#"):
                continue
            setting, separator, value = line.partition("=")
            if separator and setting.strip() == key:
                return value.strip()
    return ""


def machine(key: str, main: Path) -> str:
    """Use the shared machine reader when supported, otherwise its file lookup."""
    try:
        read_command("agentrepo", "config", "--help")
    except (FileNotFoundError, subprocess.CalledProcessError) as _error:
        return machine_file(key, main)
    return read_command("agentrepo", "config", "machine", key).strip()


def fail(source: str) -> NoReturn:
    """Name the failing source without repeating diagnostics or private values."""
    print(
        f"check-leaks: could not load private strings from {source}; "
        "the result is incomplete",
        file=sys.stderr,
    )
    sys.exit(2)


def resolve() -> tuple[dict[str, str], str | None]:
    """Load the first configured source, refusing incomplete configured lists."""
    exported = {
        key: value
        for key, value in os.environ.items()
        if key.endswith("_PRIVATE_STRINGS")
    }
    if any(part.strip() for value in exported.values() for part in value.split(",")):
        return {}, "environment"
    try:
        common = read_command(
            "git", "rev-parse", "--path-format=absolute", "--git-common-dir"
        ).strip()
    except subprocess.CalledProcessError:
        return {}, None

    main = Path(common).parent
    source = "machine configuration"
    try:
        reference = machine("private_strings_ref", main)
        if reference:
            source = "machine reference (private_strings_ref)"
            if exported:
                fail(source)
            token_command = machine("private_strings_token_command", main)
            token = (
                read_command(token_command).strip()
                if token_command
                else os.environ.get("OP_SERVICE_ACCOUNT_TOKEN", "")
            )
            if not token:
                fail(source)
            env = dict(os.environ, OP_SERVICE_ACCOUNT_TOKEN=token)
            value = (
                subprocess.run(
                    ["op", "read", reference],
                    check=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.DEVNULL,
                    env=env,
                )
                .stdout.decode()
                .strip()
            )
            if not any(part.strip() for part in value.split(",")):
                fail(source)
            return {"DEVTOOLS_PRIVATE_STRINGS": value}, source

        source = "main clone file (mise.local.toml)"
        path = main / "mise.local.toml"
        if not os.path.lexists(path):
            return {}, None
        with path.open("rb") as stream:
            config = tomllib.load(stream)
        values = {
            key: value
            for key, value in config.get("env", {}).items()
            if key.endswith("_PRIVATE_STRINGS")
        }
        if any(
            not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key)
            or not isinstance(value, str)
            for key, value in values.items()
        ):
            fail(source)
    except (OSError, ValueError, subprocess.SubprocessError, AttributeError) as _error:
        fail(source)
    else:
        return values, source


if __name__ == "__main__":
    lists, origin = resolve()
    if any(
        "\n" in value or "\r" in value or "\0" in value or "\t" in value
        for value in lists.values()
    ):
        print(
            f"check-leaks: invalid private strings from {origin}; "
            "the result is incomplete",
            file=sys.stderr,
        )
        sys.exit(2)
    for name, literal in lists.items():
        sys.stdout.buffer.write(name.encode() + b"\0" + literal.encode() + b"\0")
    if origin:
        print(f"check-leaks: private strings source: {origin}", file=sys.stderr)
