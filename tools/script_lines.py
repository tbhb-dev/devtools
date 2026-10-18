#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.14"
# dependencies = []
# ///
"""Count physical shell source lines, excluding blanks and full-line comments.

Inline comments do not change the count of their code line. This is a physical
line metric, not a shell parser: here-document data and embedded languages use
exactly the same rule. No expansion or execution of the input takes place.
"""

from __future__ import annotations

import argparse
from pathlib import Path


def count_lines(source: str) -> int:
    """Return the number of code lines under the physical-line rule."""
    return sum(
        bool(line.strip()) and not line.lstrip().startswith("#")
        for line in source.splitlines()
    )


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("paths", nargs="+", type=Path)
    args = parser.parse_args()
    paths: list[Path] = args.paths
    for path in paths:
        print(f"{count_lines(path.read_text(encoding='utf-8'))}\t{path}")


if __name__ == "__main__":
    main()
