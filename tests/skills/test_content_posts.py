from __future__ import annotations

import re
from pathlib import Path

import pytest
import tree_sitter_bash
from tree_sitter import Language, Node, Parser

SOURCE = Path(__file__).resolve().parents[2] / "packages/agents/source"


def posts_content(words: list[str]) -> bool:
    while words and (words[0] in {"command", "env"} or "=" in words[0]):
        words = words[1:]
    if len(words) < 3 or Path(words[0]).name not in {"gh", "dt_gh"}:
        return False
    family, operation = words[1:3]
    if family == "pr" and operation == "edit":
        return True
    if family in {"issue", "pr", "release"}:
        if operation in {"create", "comment", "review"}:
            return True
        text_flags = {
            "-b",
            "-F",
            "--body",
            "--body-file",
            "--title",
            "--notes",
            "--notes-file",
            "$@",
        }
        return operation == "edit" and any(
            word.split("=", 1)[0] in text_flags or word.startswith(("-b", "-F"))
            for word in words[3:]
        )
    if family != "api":
        return False
    endpoint = any(re.search(r"/(comments|reviews)(/|$)", word) for word in words[2:])
    methods = {"POST", "PATCH", "PUT", "DELETE"}
    write = any(
        word in methods
        or word.removeprefix("--method=") in methods
        or word.removeprefix("-X") in methods
        for word in words[2:]
    )
    payload = any(
        word.split("=", 1)[0] in {"-f", "-F", "--field", "--raw-field", "--input"}
        for word in words[2:]
    )
    return endpoint and (write or payload)


def violations(source: str) -> list[int]:
    parser = Parser(Language(tree_sitter_bash.language()))
    root = parser.parse(source.encode()).root_node
    findings = []
    pending: list[Node] = [root]
    while pending:
        node = pending.pop()
        pending.extend(node.named_children)
        if node.type != "command":
            continue
        name = node.child_by_field_name("name")
        if name is None:
            continue
        arguments = [name, *node.children_by_field_name("argument")]
        words = [(argument.text or b"").decode().strip("\"'") for argument in arguments]
        if posts_content(words):
            findings.append(node.start_point.row + 1)
    return findings


@pytest.mark.parametrize(
    "command",
    [
        "gh issue create --title title --body-file body",
        "gh issue comment 1 -b text",
        "gh issue edit 1 --body=text",
        "gh pr edit 1 -b" + "text",
        "gh issue edit 1 -F" + "body",
        'gh pr create "$@"',
        "dt_gh pr edit 1 --body-file body",
        "dt_gh pr edit 1 --base main",
        'dt_gh pr edit 1 "$flag" "$item"',
        "gh pr comment 1 -F body",
        "command gh pr review 1 --body-file body --approve",
        "result=$(gh pr review 1 \\\n --body-file body --approve)",
        "gh api repos/o/r/issues/comments/1 --method PATCH --input -",
        "gh api --method PUT repos/o/r/pulls/1/reviews/2 -f body=text",
        "gh api --method=PATCH repos/o/r/issues/comments/1 --input=-",
        "gh api -X" + "PATCH repos/o/r/pulls/comments/1 --input -",
        "env GH_TOKEN=value gh api repos/o/r/pulls/comments/1 -F body=@file",
        "gh release edit v1 --notes-file notes",
    ],
)
def test_detects_content_posts(command: str) -> None:
    assert violations(command)


@pytest.mark.parametrize(
    "command",
    [
        "gh-post pr create --title title --body-file body",
        'dt_gh_post pr create "$@"',
        "gh pr view 1 --json body",
        "gh-post pr edit 1 --base main",
        "gh api repos/o/r/issues/comments",
        "# gh pr comment 1 --body-file body",
        'printf "%s" "gh pr review 1 --body-file body"',
    ],
)
def test_permits_reads_metadata_and_wrapper(command: str) -> None:
    assert not violations(command)


def test_skill_sources_never_post_content_directly() -> None:
    scripts = sorted(SOURCE.rglob("*.sh"))
    assert scripts
    findings = [
        f"{path.relative_to(SOURCE)}:{line}"
        for path in scripts
        for line in violations(path.read_text())
    ]
    assert not findings, "Use gh-post for content writes:\n" + "\n".join(findings)
