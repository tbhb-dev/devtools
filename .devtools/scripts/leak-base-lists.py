#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.14"
# dependencies = []
# ///
"""Let mise evaluate private lists from an exported base tree."""

import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path


def main() -> None:
    base, needles = sys.argv[1:]
    private = Path(needles).parent
    seen = {line.split("\t", 1)[1] for line in Path(needles).read_text().splitlines()}
    with tempfile.TemporaryDirectory(dir=private) as directory:
        root = Path(directory)
        tree = root / "tree"
        tree.mkdir()
        archive = root / "base.tar"
        with archive.open("wb") as output:
            subprocess.run(
                ["git", "archive", "--format=tar", base], stdout=output, check=True
            )
        subprocess.run(["tar", "-xf", str(archive), "-C", str(tree)], check=True)
        env = dict(os.environ)
        env.update(
            TMPDIR=str(root),
            MISE_CACHE_DIR=str(root / "cache"),
            MISE_STATE_DIR=str(root / "state"),
            MISE_TRUSTED_CONFIG_PATHS=str(tree),
        )
        result = subprocess.run(
            ["mise", "env", "--json"],
            cwd=tree,
            env=env,
            capture_output=True,
            check=True,
        )
        for name, value in json.loads(result.stdout).items():
            if not name.endswith("_PRIVATE_STRINGS") or value is None:
                continue
            for index, raw_entry in enumerate(value.split(","), 1):
                entry = raw_entry.strip()
                if "\n" in entry or "\r" in entry:
                    sys.exit("invalid base private list entry")
                if entry and entry not in seen:
                    print(f"{name} entry {index}\t{entry}")
                    seen.add(entry)


if __name__ == "__main__":
    main()
