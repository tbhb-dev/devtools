# pylint: disable=duplicate-code
# The generator copies this helper verbatim into both harness packages.
# cspell:words surrogateescape alnum cntrl xdigit
# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""Inspect pinned Git changes and APM ownership; never execute repository settings."""

import json
import re
import shutil
import string
import subprocess
import sys
from pathlib import Path, PurePosixPath
from typing import NoReturn

LockRecord = dict[str, str | list[str] | None]

GIT = shutil.which("git")
if GIT is None:
    sys.exit("git is required")


def fail(message: str) -> NoReturn:
    """Reject unreadable policy without repeating private configuration values."""
    raise ValueError(message)


def git(*args: str) -> bytes:
    """Read exact objects without diff drivers or text conversion."""
    return subprocess.check_output([GIT, "-c", "core.quotePath=false", *args])  # noqa: S603 # Fixed Git operations with separate arguments.


def blob(ref: str, path: str) -> bytes:
    """Absent files are optional; other Git errors must propagate."""
    entries = git("ls-tree", ref, "--", path)
    if not entries:
        return b""
    if not entries.startswith(b"100644 ") and not entries.startswith(b"100755 "):
        fail("Policy input must be a regular Git blob")
    return git("cat-file", "blob", f"{ref}:{path}")


def regex(value: str, key: str = "policy") -> re.Pattern[str]:
    """Require explicit patterns, including ^$ for an intentionally empty set."""
    if not value:
        fail(f"Policy pattern {key} must not be empty")
    # Preserve the C-locale POSIX character classes used by existing ERE settings.
    classes = {
        "alnum": string.ascii_letters + string.digits,
        "alpha": string.ascii_letters,
        "blank": " \t",
        "cntrl": "".join(chr(code) for code in [*range(32), 127]),
        "digit": string.digits,
        "graph": string.ascii_letters + string.digits + string.punctuation,
        "lower": string.ascii_lowercase,
        "print": string.ascii_letters + string.digits + string.punctuation + " ",
        "punct": string.punctuation,
        "space": string.whitespace,
        "upper": string.ascii_uppercase,
        "xdigit": string.hexdigits,
    }
    for name, characters in classes.items():
        value = value.replace("[:" + name + ":]", re.escape(characters))
    return re.compile(value)


def deployment_path(value: object) -> str:
    """Only project-relative deployed paths may exempt a scan."""
    if not isinstance(value, str) or not value:
        fail("Invalid APM deployed path")
    path = PurePosixPath(value)
    if not path.parts or path.is_absolute() or ".." in path.parts:
        fail("Invalid APM deployed path")
    return value.removeprefix("./").rstrip("/")


def lock_scalar(value: str) -> str | None:
    """Read the scalar forms emitted in an APM lockfile."""
    value = value.strip()
    if not value:
        fail("Invalid APM lockfile scalar")
    if value in {"null", "~"}:
        return None
    if value.startswith('"'):
        parsed = json.loads(value)
        if not isinstance(parsed, str):
            fail("Invalid APM lockfile scalar")
        return parsed
    if value.startswith("'"):
        if not value[1:].endswith("'"):
            fail("Invalid APM lockfile scalar")
        return value[1:-1].replace("''", "'")
    return value


def lock_list(value: str) -> list[str]:
    """Read the simple flow sequences APM uses for deployed paths."""
    if value == "[]":
        return []
    if not value.startswith("[") or not value.endswith("]"):
        fail("Invalid APM lockfile list")
    items = value[1:-1].split(",")
    parsed = [lock_scalar(item) for item in items]
    if any(item is None for item in parsed):
        fail("Invalid APM lockfile list")
    return [item for item in parsed if item is not None]


def lock_records(text: str, name: str, *, required_section: bool) -> list[str]:
    """Split one top-level generated APM sequence into record bodies."""
    headers = list(re.finditer(rf"(?m)^{name}:(.*)$", text))
    if not headers:
        if required_section:
            fail("Invalid APM lockfile dependencies")
        return []
    if len(headers) != 1:
        fail("Duplicate APM lockfile section")
    header = headers[0]
    suffix = header.group(1).strip()
    if suffix == "[]":
        return []
    if suffix:
        fail("Invalid APM lockfile section")
    start = header.end() + 1
    following = re.search(r"(?m)^[A-Za-z_][\w-]*:", text[start:])
    end = start + following.start() if following else len(text)
    body = text[start:end].strip("\n")
    if not body:
        return []
    if not body.startswith("- "):
        fail("Invalid APM lockfile record")
    return body[2:].split("\n- ")


def lock_record(text: str, keys: tuple[str, ...]) -> LockRecord:
    """Read selected scalar and path-list fields from one record."""
    text = "  " + text
    record: LockRecord = {}
    for key in keys:
        matches = list(re.finditer(rf"(?m)^  {key}:(?: (.*))?$", text))
        if len(matches) > 1:
            fail("Duplicate APM lockfile field")
        if not matches:
            continue
        value = (matches[0].group(1) or "").strip()
        if value.startswith("["):
            record[key] = lock_list(value)
        elif value:
            record[key] = lock_scalar(value)
        elif key == "deployed_files":
            tail = text[matches[0].end() :]
            items = re.match(r"(?m)((?:\n  - .*)*)", tail)
            values = re.findall(r"(?m)^  - (.*)$", items.group(1) if items else "")
            parsed = [lock_scalar(item) for item in values]
            if any(item is None for item in parsed):
                fail("Invalid APM lockfile list")
            record[key] = [item for item in parsed if item is not None]
        else:
            fail("Invalid APM lockfile scalar")
    return record


def parse_lock(data: bytes) -> tuple[list[LockRecord], list[LockRecord]]:
    """Parse only the generated lockfile records used for ownership."""
    text = data.decode("utf-8")
    dependencies = [
        lock_record(
            record,
            ("repo_url", "source", "local_path", "virtual_path", "deployed_files"),
        )
        for record in lock_records(text, "dependencies", required_section=True)
    ]
    deployments = [
        lock_record(record, ("kind", "value", "active_owner"))
        for record in lock_records(text, "deployments", required_section=False)
    ]
    return dependencies, deployments


def lock_text(record: LockRecord, key: str, default: str = "") -> str:
    """Read an optional string field with a typed default."""
    value = record.get(key, default)
    if not isinstance(value, str):
        fail("Invalid APM lockfile field")
    return value


def owned_remotely(base: str) -> dict[str, bool]:
    """Use base ownership and honor active owners where APM records them."""
    data = blob(base, "apm.lock.yaml")
    if not data:
        return {}
    dependencies, records = parse_lock(data)
    owned = {}
    identities = {}
    for dep in dependencies:
        repository = lock_text(dep, "repo_url")
        local_path = lock_text(dep, "local_path")
        local = bool(
            lock_text(dep, "source") == "local"
            or local_path
            or repository.startswith("_local/")
        )
        virtual_path = lock_text(dep, "virtual_path")
        identity = local_path or (
            repository + "/" + virtual_path if virtual_path else repository
        )
        identities[identity] = not local
        paths = dep.get("deployed_files", [])
        if not isinstance(paths, list):
            fail("Invalid APM deployed_files")
        for value in paths:
            path = deployment_path(value)
            # Without active ownership metadata, keep local overlaps in scope.
            owned[path] = owned.get(path, True) and not local
    for record in records:
        if lock_text(record, "kind") != "project-relative":
            continue
        owner = lock_text(record, "active_owner")
        if owner not in identities:
            fail("Unknown APM active owner")
        owned[deployment_path(lock_text(record, "value"))] = identities[owner]
    return owned


def remote_path(path: str, owned: dict[str, bool]) -> bool:
    """The most specific deployed owner wins; never exempt package sources."""
    matching = [root for root in owned if path == root or path.startswith(root + "/")]
    return bool(matching) and owned[max(matching, key=len)]


COMMENT = re.compile(r"^\s*(?:#|//|/\*|\*|<!--|;)")
EXCLUSION = re.compile(
    r"^\s*[-{,]?\s*[\"']?(?:exclude(?:s)?|extend-exclude|ignore|ignored|"
    r"ignore[-_]patterns|disable(?:d)?|skip|allow(?:ed)?|"
    r"allow[-_]list)[\"']?\s*[:=]",
    re.IGNORECASE,
)
REMOVED_GATE = re.compile(
    r"^\s*(?:-\s*id\s*:|(?:-\s*)?[\"']?mise run (?:lint|check|test)|"
    r"(?:lint|check|test)[\w-]*\s*=)"
)


def inline_comment(text: str) -> str:
    """Do not interpret a marker inside a quoted value as a suppression."""
    quote = ""
    escaped = False
    for index, char in enumerate(text):
        if escaped:
            escaped = False
        elif char == "\\":
            escaped = True
        elif quote:
            if char == quote:
                quote = ""
        elif char in "\"'`":
            quote = char
        elif (
            index
            and text[index - 1].isspace()
            and (char == "#" or text[index:].startswith(("//", "<!--")))
        ):
            return text[index:].lower()
    return ""


def inspect(base: str, head: str, config: dict[str, str]) -> dict[str, object]:
    """Return reasons needing an owner and the owner paths for reporting."""
    golden, markers, configs, owners = [
        regex(config.get(key, ""), key)
        for key in (
            "golden_paths",
            "loosening_markers",
            "gate_config_paths",
            "owner_paths",
        )
    ]
    # Validate cleanup policy before any irreversible write as well.
    regex(config.get("disposable_ignored", ""), "disposable_ignored")
    mergebase = git("merge-base", base, head).decode().strip()
    paths = [
        p.decode("utf-8", "surrogateescape")
        for p in git(
            "diff",
            "--no-ext-diff",
            "--no-textconv",
            "--no-renames",
            "--name-only",
            "-z",
            mergebase,
            head,
        ).split(b"\0")
        if p
    ]
    if not paths:
        fail("Pinned change has no paths")
    owned = owned_remotely(base)
    findings = []
    owner_paths = [path for path in paths if owners.search(path)]
    for path in paths:
        # Configuration changes cannot waive their own policy.
        if path == ".agents/agent-skills.conf":
            findings.append([path, "changes merge policy"])
        if remote_path(path, owned):
            continue
        if golden.search(path):
            findings.append([path, "changes a golden file"])
        diff = git(
            "diff",
            "--no-ext-diff",
            "--no-textconv",
            "--no-renames",
            "--text",
            "--unified=0",
            mergebase,
            head,
            "--",
            path,
        ).decode("utf-8", "surrogateescape")
        for line in diff.splitlines():
            if line.startswith(("+++", "---")) or not line.startswith(("+", "-")):
                continue
            text = line[1:]
            if COMMENT.match(text):
                continue
            if line.startswith("+") and (
                markers.search(inline_comment(text))
                or (configs.search(path) and EXCLUSION.search(text))
            ):
                findings.append([path, "adds a suppression or exclusion"])
            elif (
                line.startswith("-")
                and configs.search(path)
                and REMOVED_GATE.search(text)
            ):
                findings.append([path, "removes a hook or gate"])
    return {"findings": findings, "owner_paths": owner_paths}


def ref_matches(pattern: str, branch: str, default: str) -> bool:
    """Ruleset single stars cannot cross a slash; double stars can."""
    if pattern == "~ALL":
        return True
    if pattern == "~DEFAULT_BRANCH":
        return branch == default
    escaped = (
        re.escape(pattern)
        .replace(r"\*\*", ".*")
        .replace(r"\*", "[^/]*")
        .replace(r"\?", "[^/]")
    )
    return re.fullmatch(escaped, "refs/heads/" + branch) is not None


def string_list(value: object, label: str) -> list[str]:
    """Narrow one JSON list to strings."""
    if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
        fail(f"Invalid {label}")
    return value


def ruleset_rules(data: object, branch: str, default: str) -> list[object]:
    """Return active rules where a ruleset applies to the target branch."""
    if not isinstance(data, dict):
        fail("Invalid ruleset")
    if data.get("target") != "branch" or data.get("enforcement") != "active":
        return []
    conditions_value = data.get("conditions")
    if not isinstance(conditions_value, dict):
        fail("Invalid ruleset conditions")
    conditions: dict[str, object] = conditions_value
    refs_value = conditions.get("ref_name")
    if not isinstance(refs_value, dict):
        fail("Invalid ruleset ref names")
    refs: dict[str, object] = refs_value
    included = string_list(refs.get("include", []), "ruleset includes")
    excluded = string_list(refs.get("exclude", []), "ruleset excludes")
    rules = data.get("rules")
    if not isinstance(rules, list):
        fail("Invalid ruleset rules")
    if any(ref_matches(p, branch, default) for p in included) and not any(
        ref_matches(p, branch, default) for p in excluded
    ):
        return rules
    return []


def required_names(rules: list[object]) -> set[str]:
    """Validate required-check rules and collect their contexts."""
    names: set[str] = set()
    for rule in rules:
        if not isinstance(rule, dict):
            fail("Invalid required check rule")
        if rule.get("type") != "required_status_checks":
            continue
        parameters = rule.get("parameters")
        if not isinstance(parameters, dict):
            fail("Invalid required check parameters")
        checks = parameters.get("required_status_checks")
        if not isinstance(checks, list):
            fail("Invalid required checks")
        for check in checks:
            if not isinstance(check, dict) or not isinstance(check.get("context"), str):
                fail("Invalid required check")
            names.add(check["context"])
    return names


def required(base: str, branch: str, default: str, directory: str, live: str) -> None:
    """Union live applied rules and active ruleset files at the pinned base."""
    if (
        not directory
        or PurePosixPath(directory).is_absolute()
        or ".." in PurePosixPath(directory).parts
    ):
        fail("ruleset_dir must be inside the repository")
    parsed_rules: object = json.loads(live)
    if not isinstance(parsed_rules, list):
        fail("Invalid applied rules")
    rules: list[object] = parsed_rules
    for name in git("ls-tree", "-r", "--name-only", "-z", base, "--", directory).split(
        b"\0"
    ):
        if not name.endswith(b".json"):
            continue
        rules.extend(
            ruleset_rules(json.loads(blob(base, name.decode())), branch, default)
        )
    names = required_names(rules)
    if not names or any(not name for name in names):
        fail("No required checks are known for the base")
    print("\n".join(sorted(names)))


def valuable_ignored(worktree: Path, pattern: re.Pattern[str]) -> bool:
    """Keep ignored configuration unless the repository marks it disposable."""
    ignored = git(
        "-C",
        str(worktree),
        "ls-files",
        "--others",
        "--ignored",
        "--exclude-standard",
        "-z",
    )
    return any(
        not pattern.search(path.decode("utf-8", "surrogateescape"))
        for path in ignored.split(b"\0")
        if path
    )


def cleanup(base: str, branch: str, expected: str, merge: str, disposable: str) -> None:
    """Keep current, dirty, non-agent, and ignored-file worktrees intact."""
    pattern = regex(disposable)
    rows = (
        git("worktree", "list", "--porcelain", "-z")
        .decode("utf-8", "surrogateescape")
        .split("\0\0")
    )
    worktrees = [
        dict(field.split(" ", 1) for field in row.split("\0") if " " in field)
        for row in rows
        if row
    ]
    main = Path(worktrees[0]["worktree"]).resolve()
    current = Path.cwd().resolve()

    def run(*args: str) -> bool:
        result = subprocess.run([GIT, *args], check=False)  # noqa: S603 # Fixed cleanup operations; no shell.
        return result.returncode == 0

    main_branch = subprocess.run(  # noqa: S603 # Fixed Git branch lookup.
        [GIT, "-C", str(main), "symbolic-ref", "--short", "HEAD"],
        capture_output=True,
        check=False,
    )
    if (
        main_branch.returncode == 0
        and main_branch.stdout.decode().strip() == base
        and not git("-C", str(main), "status", "--porcelain", "--untracked-files=no")
    ):
        if valuable_ignored(main, pattern):
            print("Main checkout holds valuable ignored files; retained")
        else:
            run("-C", str(main), "merge", "--ff-only", merge)
    ref = "refs/heads/" + branch
    tip = subprocess.run(  # noqa: S603 # Fixed Git ref lookup.
        [GIT, "rev-parse", "--verify", ref], capture_output=True, check=False
    )
    if tip.returncode or tip.stdout.decode().strip() != expected:
        print("Local branch absent or moved; retained")
        return
    for row in worktrees:
        if row.get("branch") != ref:
            continue
        worktree = Path(row["worktree"]).resolve()
        allowed = main / ".claude" / "worktrees"
        if (
            worktree == current
            or worktree in current.parents
            or allowed not in worktree.parents
        ):
            print("Current or non-agent worktree retained")
            return
        if git("-C", str(worktree), "status", "--porcelain", "--untracked-files=all"):
            print("Dirty worktree retained")
            return
        if valuable_ignored(worktree, pattern):
            print("Worktree with valuable ignored files retained")
            return
        if not run("worktree", "remove", str(worktree)):
            print("Worktree removal refused; branch retained")
            return
    # Keep native checked-out-branch and unmerged-work protections.
    if not run("branch", "-d", "--", branch):
        print("Local branch retained by Git cleanup protections")


def main() -> None:
    """Expose file-oriented interfaces for the shared shell entry points."""
    mode, *args = sys.argv[1:]
    match mode, args:
        case "inspect", [base, head]:
            print(json.dumps(inspect(base, head, json.loads(sys.stdin.read()))))
        case "required", [base, branch, default, directory]:
            required(base, branch, default, directory, sys.stdin.read())
        case "cleanup", [base, branch, expected, merge, disposable]:
            cleanup(base, branch, expected, merge, disposable)
        case _:
            fail("Unknown policy operation or arguments")


if __name__ == "__main__":
    try:
        main()
    except (
        ValueError,
        re.error,
        KeyError,
        TypeError,
        UnicodeDecodeError,
        OSError,
        subprocess.CalledProcessError,
    ) as error:
        print(f"merge policy unreadable: {error}", file=sys.stderr)
        sys.exit(1)
