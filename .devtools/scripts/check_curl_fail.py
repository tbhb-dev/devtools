#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.14"
# dependencies = ["tree-sitter==0.25.2", "tree-sitter-bash==0.25.1"]
# ///
"""Reject curl commands that can deliver HTTP error bodies as successful output.

Parse shell syntax rather than searching strings: quoted remote commands and
heredoc bodies are data. Commands hidden behind eval, variables, or wrappers
remain a review responsibility. Explicit paths are supplied by the shared
source-file selector, keeping exclusions identical to the shellcheck gate.
"""

import argparse
import re
import shlex
from itertools import pairwise
from pathlib import Path
from typing import TYPE_CHECKING

import tree_sitter_bash
from tree_sitter import Language, Node, Parser

if TYPE_CHECKING:
    from collections.abc import Iterator

ALLOW = re.compile(r"\s*#\s*lint-curl: allow\s+\S.*")
FAIL = re.compile(r"--fail(?:-with-body)?|-[a-zA-Z]*f[a-zA-Z]*")


def walk(node: Node) -> Iterator[Node]:
    yield node
    for child in node.named_children:
        yield from walk(child)


def literal(node: Node) -> str:
    # Only literal arguments can establish an option; expansions are unknown.
    if any(child.type.endswith("expansion") for child in walk(node)):
        return ""
    return "".join(shlex.split(node.text.decode() if node.text else ""))


def comparison(node: Node) -> bool:
    if node.type == "binary_expression":
        operator = node.child_by_field_name("operator")
        return operator is not None and operator.text in {
            b"=",
            b"==",
            b"!=",
            b"-eq",
            b"-ne",
            b"-lt",
            b"-le",
            b"-gt",
            b"-ge",
            b"=~",
        }
    if node.type == "command":
        name = node.child_by_field_name("name")
        return (
            name is not None
            and literal(name) == "test"
            and any(
                literal(argument)
                in {"=", "==", "!=", "-eq", "-ne", "-lt", "-le", "-gt", "-ge"}
                for argument in node.children_by_field_name("argument")
            )
        )
    return False


def compared(command: Node, root: Node) -> bool:
    """Recognize a status substitution tested inline or assigned then tested."""
    parents = []
    parent = command.parent
    while parent is not None:
        parents.append(parent)
        parent = parent.parent
    if any(comparison(node) for node in parents):
        return True
    assignments = [node for node in parents if node.type == "variable_assignment"]
    if not assignments:
        return False
    assignment = assignments[0]
    return tested_after(assignment, root)


def tested_after(assignment: Node, root: Node) -> bool:
    name = assignment.children_by_field_name("name")[0]
    variable = name.text
    later = (node for node in walk(root) if node.start_byte > assignment.end_byte)
    for node in later:
        if node.type == "variable_assignment":
            target = node.child_by_field_name("name")
            if target is not None and target.text == variable:
                return False
        if comparison(node) and any(
            child.type == "variable_name" and child.text == variable
            for child in walk(node)
        ):
            return True
    return False


def safe(command: Node, root: Node) -> bool:
    arguments = [literal(node) for node in command.children_by_field_name("argument")]
    if any(FAIL.fullmatch(argument) for argument in arguments):
        return True
    status = any(
        option in {"-w", "--write-out"} and value == "%{http_code}"
        for option, value in pairwise(arguments)
    )
    return status and compared(command, root)


def findings(path: Path) -> Iterator[str]:
    source = path.read_bytes()
    root = Parser(Language(tree_sitter_bash.language())).parse(source).root_node
    lines = source.decode().splitlines()
    allowed_line = -1
    for command in walk(root):
        if command.type != "command":
            continue
        name = command.child_by_field_name("name")
        if name is None or literal(name) != "curl":
            continue
        line = command.start_point.row
        if line > 0 and line != allowed_line and ALLOW.fullmatch(lines[line - 1]):
            allowed_line = line
            continue
        if not safe(command, root):
            yield (
                f"{path}:{line + 1}: curl without a fail option; add -f, "
                "or put # lint-curl: allow <reason> on the line before"
            )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("paths", type=Path, nargs="*")
    args = parser.parse_args(argv)
    failed = False
    for path in args.paths:
        for finding in findings(path):
            print(finding)
            failed = True
    return int(failed)


if __name__ == "__main__":
    raise SystemExit(main())
