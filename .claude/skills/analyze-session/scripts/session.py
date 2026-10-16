#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.14"
# ///
# pylint: disable=duplicate-code
# Both harness packages carry this file byte for byte, so the similarity
# checker sees each copy as a clone of the other.
"""session — query an agent transcript without reading it whole.

A transcript is JSONL, one record per line, and a working session runs to
several megabytes. Reading it into context to answer a question about it
costs more than the question is worth and often more than the context
holds. Every subcommand here answers from a streaming pass and prints
line numbers, so the follow-up read is bounded to what those numbers
point at.

Two transcript formats are understood, each behind its own parser, and
the parser follows the file rather than the harness running this script.
A session on either harness can therefore analyze a transcript from
either one. The format is detected from the first records unless
`--format claude` or `--format codex` names it.

Claude Code records carry `message.content` blocks of type text,
thinking, tool_use, and tool_result; user records carry either a string
`message.content` or a block list holding tool_result. A tool result
marks failure with `is_error`, but a shell command that exits nonzero
does not, so `errors` looks for both.

Codex rollouts carry `type`, `timestamp`, and `payload` on every record.
Response items hold messages, reasoning, custom tool calls, and outputs;
event messages hold task, item, and token events.

Subcommands print plain text. Nothing here writes to the transcript.

The script metadata above declares no dependencies because there are
none: everything here is standard library. It pins the interpreter
instead, matching the version this project targets. uv fetches that
version when the machine lacks it, so the skill states its own
requirement even in a repository with no Python project.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import re
import sys
from abc import ABC, abstractmethod
from collections import Counter, defaultdict
from pathlib import Path
from typing import TYPE_CHECKING, ClassVar, override

if TYPE_CHECKING:
    from collections.abc import Iterator, Mapping, Sequence

type Record = dict[str, object]
type Hit = tuple[Path, int, int | None, str]
type Cluster = tuple[str, list[int]]

# A tool result longer than this gets truncated in summary views. Full
# text stays available through `show`.
SUMMARY_WIDTH = 300

# Two tool calls farther apart than this many records belong to
# different episodes, so `loops` starts a new cluster.
CLUSTER_GAP = 40

# A run reporting at most this many findings is a trickle rather than a
# batch, which is the distinction `lint` reports on.
FEW_FINDINGS = 2

# Format detection reads this many records before giving up.
DETECT_RECORDS = 100

# Top-level record types that mark each format. Codex wraps everything in
# `payload`; Claude puts content under `message`.
CODEX_TYPES = {"session_meta", "response_item", "event_msg", "turn_context"}
CLAUDE_TYPES = {"user", "assistant", "system", "summary", "attachment"}

# Words a shell signature drops because they never distinguish an attempt.
NOISE_HEADS = ("cd", "echo", "printf", "set", "export")

# Task names carry an optional namespace, as in devtools:lint-toml, so the
# colon belongs in the class.
MISE_TASK = re.compile(r"\bmise\s+run\s+([a-z0-9:-]+)")


def die(msg: str) -> None:
    print(f"session: {msg}", file=sys.stderr)
    raise SystemExit(1)


def as_dict(value: object) -> Record:
    if isinstance(value, dict):
        mapping: Mapping[object, object] = value
        return {str(k): v for k, v in mapping.items()}
    return {}


def as_list(value: object) -> list[object]:
    if isinstance(value, list):
        seq: Sequence[object] = value
        return list(seq)
    return []


def as_str(value: object, default: str = "") -> str:
    return value if isinstance(value, str) else default


def records(path: str) -> Iterator[tuple[int, Record]]:
    """Yield (lineno, parsed) for each record, skipping unparseable lines.

    A truncated final line is normal in a transcript still being written,
    so a parse failure is reported once and does not stop the pass.
    """
    bad = 0
    with Path(path).open(encoding="utf-8", errors="replace") as fh:
        for lineno, raw in enumerate(fh, 1):
            line = raw.strip()
            if not line:
                continue
            try:
                loaded: object = json.loads(line)
            except ValueError:
                bad += 1
                continue
            if isinstance(loaded, dict):
                yield lineno, as_dict(loaded)
            else:
                bad += 1
    if bad:
        print(f"(skipped {bad} unparseable line(s))", file=sys.stderr)


def flat(text: str, width: int = SUMMARY_WIDTH) -> str:
    return " ".join(text.split())[:width]


def shell_heads(cmd: str, pattern: str, limit: int) -> str:
    """The distinct command words in a script, without the noise words."""
    heads: list[str] = re.findall(pattern, cmd)
    keep = [h for h in heads if h not in NOISE_HEADS]
    return ",".join(sorted(set(keep))[:limit])


def text_parts(kind: str, value: object) -> Iterator[tuple[str, str]]:
    if isinstance(value, str) and value:
        yield kind, value
    elif isinstance(value, list):
        for part in as_list(value):
            if isinstance(part, str):
                yield kind, part
            elif isinstance(part, dict):
                item = as_dict(part)
                text = as_str(item.get("text")) or as_str(item.get("content"))
                if text:
                    yield kind, text


class Parser(ABC):
    """What a transcript format has to answer.

    The commands never look inside a record themselves.
    """

    name: str
    # Where this format's transcripts live, and how to find them.
    root: str
    pattern: str

    @abstractmethod
    def blocks(self, rec: Record) -> Iterator[tuple[str, str]]:
        """Yield (kind, text) for every content block in a record.

        Kinds are TEXT, THINK, USE:<tool>, RES, STR, and EVENT. Tool
        inputs and results collapse to text so one regex can search
        across all of them.
        """

    @abstractmethod
    def tool_calls(self, rec: Record) -> Iterator[tuple[str, Record]]:
        """Yield (name, input) for each tool call in a record."""

    @abstractmethod
    def is_failure(self, rec: Record, text: str) -> bool:
        """Decide whether a tool result represents a failure."""

    @abstractmethod
    def signature(self, name: str, inp: Record) -> str:
        """Reduce a tool call to what makes two calls "the same attempt"."""

    @abstractmethod
    def is_sidechain(self, rec: Record) -> bool:
        """True for a record written by a delegated agent."""

    @abstractmethod
    def session_id(self, path: Path) -> str:
        """The session's identifier."""

    @abstractmethod
    def in_project(self, path: Path, project: str) -> bool:
        """Whether the transcript belongs to a project named by substring."""

    def classify(self, text: str) -> str:
        if "has been denied" in text or "Permission denied" in text:
            return "permission"
        if re.search(r"command not found|illegal option|invalid option", text):
            return "environment"
        if re.search(r"\[error\] ", text):
            return "lint"
        return "nonzero"

    def candidates(self) -> list[Path]:
        base = Path(self.root).expanduser()
        if not base.is_dir():
            return []
        return sorted(base.glob(self.pattern))


class ClaudeParser(Parser):
    name: str = "claude"
    root: str = "~/.claude/projects"
    pattern: str = "*/*.jsonl"

    FAILURE: ClassVar[re.Pattern[str]] = re.compile(
        r"Exit code [1-9]|exit=[1-9]|command not found|"
        r"No such file or directory|hook error|has been denied|"
        r"illegal option|invalid option|Permission denied"
    )

    @staticmethod
    def _content(rec: Record) -> object:
        return as_dict(rec.get("message")).get("content")

    @override
    def blocks(self, rec: Record) -> Iterator[tuple[str, str]]:
        content = self._content(rec)
        if isinstance(content, str):
            yield "STR", content
            return
        for raw_blk in as_list(content):
            blk = as_dict(raw_blk)
            kind = as_str(blk.get("type"))
            if kind == "text":
                yield "TEXT", as_str(blk.get("text"))
            elif kind == "thinking":
                yield "THINK", as_str(blk.get("thinking"))
            elif kind == "tool_use":
                name = as_str(blk.get("name"), "?")
                yield f"USE:{name}", self._use_body(name, as_dict(blk.get("input")))
            elif kind == "tool_result":
                yield "RES", self._result_text(blk)

    @staticmethod
    def _use_body(name: str, inp: Record) -> str:
        if name == "Bash":
            return str(inp.get("command", ""))
        if name in ("Edit", "Write", "Read"):
            return str(inp.get("file_path", ""))
        return json.dumps(inp)

    @staticmethod
    def _result_text(blk: Record) -> str:
        content = blk.get("content")
        if isinstance(content, list):
            return " ".join(
                as_str(as_dict(part).get("text"))
                for part in as_list(content)
                if isinstance(part, dict)
            )
        return str(content)

    @override
    def tool_calls(self, rec: Record) -> Iterator[tuple[str, Record]]:
        for raw_blk in as_list(self._content(rec)):
            blk = as_dict(raw_blk)
            if blk.get("type") == "tool_use":
                yield as_str(blk.get("name"), "?"), as_dict(blk.get("input"))

    @override
    def is_failure(self, rec: Record, text: str) -> bool:
        """Decide whether a result failed, by flag or by its text.

        `is_error` alone undercounts badly. A Bash call that exits
        nonzero comes back as an ordinary result, and so does a linter
        reporting findings, so those get matched on their text.
        """
        for raw_blk in as_list(self._content(rec)):
            blk = as_dict(raw_blk)
            if blk.get("type") == "tool_result" and blk.get("is_error"):
                return True
        return bool(self.FAILURE.search(text))

    @override
    def classify(self, text: str) -> str:
        if "hook error" in text:
            return "hook-block"
        return super().classify(text)

    @override
    def signature(self, name: str, inp: Record) -> str:
        """Reduce a Bash call to the tasks and commands it invoked.

        The arguments are left out, so an edit-then-relint cycle
        collapses to one signature across its rounds.
        """
        if name != "Bash":
            return f"{name}:{inp.get('file_path', '')}"
        cmd = str(inp.get("command", ""))
        tasks: list[str] = MISE_TASK.findall(cmd)
        if tasks:
            return "mise run " + ",".join(sorted(set(tasks)))
        heads = shell_heads(cmd, r"(?:^|\n|&&|\|\||;)\s*([a-z][a-z0-9_-]*)", 3)
        return f"sh:{heads}"

    @override
    def is_sidechain(self, rec: Record) -> bool:
        return bool(rec.get("isSidechain"))

    @override
    def session_id(self, path: Path) -> str:
        return path.name.removesuffix(".jsonl")

    @override
    def in_project(self, path: Path, project: str) -> bool:
        return project in path.parent.name


class CodexParser(Parser):
    name: str = "codex"
    root: str = "~/.codex/sessions"
    pattern: str = "**/*.jsonl"

    CALLS: ClassVar[set[str]] = {"custom_tool_call", "function_call"}
    OUTPUTS: ClassVar[set[str]] = {"custom_tool_call_output", "function_call_output"}
    FAILURE: ClassVar[re.Pattern[str]] = re.compile(
        r"Script failed with code [1-9]|Exit code [1-9]|"
        r"exit_code[\"': ]+[1-9]|exit=[1-9]|command not found|"
        r"No such file or directory|hook error|has been denied|"
        r"illegal option|invalid option|Permission denied"
    )

    @override
    def blocks(self, rec: Record) -> Iterator[tuple[str, str]]:
        payload = as_dict(rec.get("payload"))
        ptype = as_str(payload.get("type"))
        if ptype in self.CALLS:
            name = (
                as_str(payload.get("name")) or as_str(payload.get("tool_name")) or "?"
            )
            body = payload.get("input") or payload.get("arguments") or ""
            yield f"USE:{name}", body if isinstance(body, str) else json.dumps(body)
        elif ptype in self.OUTPUTS:
            yield "RES", self._result_text(payload)
        elif ptype == "reasoning":
            for field in ("summary", "content", "text"):
                yield from text_parts("THINK", payload.get(field))
        elif ptype in {"message", "agent_message"}:
            content = payload.get("content", payload.get("message", ""))
            yield from text_parts("TEXT", content)
        elif rec.get("type") == "event_msg":
            for field in ("message", "text", "output", "error"):
                yield from text_parts("EVENT", payload.get(field))

    @staticmethod
    def _result_text(blk: Record) -> str:
        content = blk.get("output", blk.get("content", ""))
        if isinstance(content, list):
            return " ".join(
                as_str(as_dict(part).get("text"))
                for part in as_list(content)
                if isinstance(part, dict)
            )
        return str(content)

    @override
    def tool_calls(self, rec: Record) -> Iterator[tuple[str, Record]]:
        payload = as_dict(rec.get("payload"))
        if as_str(payload.get("type")) not in self.CALLS:
            return
        name = as_str(payload.get("name")) or as_str(payload.get("tool_name")) or "?"
        raw = payload.get("input") or payload.get("arguments")
        if isinstance(raw, str):
            try:
                parsed: object = json.loads(raw)
            except ValueError:
                parsed = {"command": raw}
            raw = parsed
        yield name, as_dict(raw) if isinstance(raw, dict) else {"value": raw}

    @override
    def is_failure(self, rec: Record, text: str) -> bool:
        """Decide whether a result failed, by flag or by its text.

        Tool output is stored as content blocks and does not carry a
        stable top-level error flag. Match the structured wrapper text
        plus common command and permission failures.
        """
        payload = as_dict(rec.get("payload"))
        if payload.get("is_error") or payload.get("success") is False:
            return True
        return bool(self.FAILURE.search(text))

    @override
    def classify(self, text: str) -> str:
        if "Operation not permitted" in text:
            return "permission"
        return super().classify(text)

    @override
    def signature(self, name: str, inp: Record) -> str:
        """Reduce a call to the nested tools or tasks it invoked.

        Codex records the outer custom tool as `exec`; its input is the
        JavaScript orchestration source. Reducing that to nested tool
        methods and mise task names keeps unrelated exec calls from
        collapsing together.
        """
        cmd = str(inp.get("command", inp.get("source", "")))
        tasks: list[str] = MISE_TASK.findall(cmd)
        if tasks:
            return "mise run " + ",".join(sorted(set(tasks)))
        methods: list[str] = re.findall(r"\btools\.([A-Za-z0-9_]+)\s*\(", cmd)
        if methods:
            return self._nested_signature(cmd, methods)
        if name != "exec":
            return f"{name}:{inp.get('file_path', inp.get('path', ''))}"
        heads = shell_heads(cmd, r"(?:^|\n|&&|\|\||;)\s*([a-z][a-z0-9_-]*)", 3)
        return f"{name}:{heads}"

    @staticmethod
    def _nested_signature(cmd: str, methods: list[str]) -> str:
        if methods != ["exec_command"]:
            return "exec:" + ",".join(sorted(set(methods)))
        match = re.search(r"[\"']?cmd[\"']?\s*:\s*([\"'`])(.*?)\1", cmd, re.DOTALL)
        shell = match.group(2) if match else ""
        shell_tasks: list[str] = MISE_TASK.findall(shell)
        if shell_tasks:
            return "mise run " + ",".join(sorted(set(shell_tasks)))
        heads = shell_heads(shell, r"(?:^|\\n|&&|\|\||;|\|)\s*([a-z][a-z0-9_-]*)", 4)
        return f"exec_command:{heads}"

    @override
    def is_sidechain(self, rec: Record) -> bool:
        return rec.get("type") == "inter_agent_communication_metadata"

    @staticmethod
    def _meta(path: Path) -> Record:
        for _, rec in records(str(path)):
            if rec.get("type") == "session_meta":
                return as_dict(rec.get("payload"))
        return {}

    @override
    def session_id(self, path: Path) -> str:
        fallback = path.stem.removeprefix("rollout-").rsplit("-", 5)[-1]
        return as_str(self._meta(path).get("id")) or fallback

    @override
    def in_project(self, path: Path, project: str) -> bool:
        return project.lower() in as_str(self._meta(path).get("cwd")).lower()


PARSERS: dict[str, Parser] = {p.name: p for p in (ClaudeParser(), CodexParser())}


def detect_format(path: str) -> str:
    """Name the format of a transcript from its first records.

    A file that matches neither is refused rather than guessed at,
    because the wrong parser reads every record as empty and reports a
    clean session.
    """
    for seen, (_, rec) in enumerate(records(path)):
        if seen >= DETECT_RECORDS:
            break
        rec_type = as_str(rec.get("type"))
        if isinstance(rec.get("payload"), dict) and rec_type in CODEX_TYPES:
            return "codex"
        if "message" in rec or rec_type in CLAUDE_TYPES:
            return "claude"
    die(f"cannot tell which harness wrote {path}; pass --format claude|codex")
    return ""  # pragma: no cover - die exits


def parser_for(args: argparse.Namespace) -> Parser:
    """The parser for the transcript a subcommand was pointed at."""
    fmt = as_str(getattr(args, "format", "auto"), "auto")
    path = as_str(getattr(args, "path", ""))
    chosen = fmt if fmt != "auto" else detect_format(path)
    return PARSERS[chosen]


# --- locate ---------------------------------------------------------------


def locate_hits(parser: Parser, args: argparse.Namespace) -> Iterator[Hit]:
    """Yield (path, size, match count, format) for each candidate kept."""
    project = as_str(getattr(args, "project", ""))
    contains = as_str(getattr(args, "contains", ""))
    for path in parser.candidates():
        if project and not parser.in_project(path, project):
            continue
        found: int | None = None
        if contains:
            with path.open(encoding="utf-8", errors="replace") as fh:
                found = sum(1 for line in fh if contains in line)
            if not found:
                continue
        yield path, path.stat().st_size, found, parser.name


def hit_mtime(hit: Hit) -> float:
    return hit[0].stat().st_mtime


def cmd_locate(args: argparse.Namespace) -> None:
    """Find transcripts, optionally filtering by a content fingerprint.

    Claude Code stores transcripts per project directory; Codex stores
    them in date directories. Searching by content or by project
    metadata avoids guessing either path. Without `--format`, both
    stores are searched.
    """
    fmt = as_str(getattr(args, "format", "auto"), "auto")
    parsers = PARSERS.values() if fmt == "auto" else [PARSERS[fmt]]
    hits = [hit for parser in parsers for hit in locate_hits(parser, args)]

    if not hits:
        print("no transcript matched")
        if as_str(getattr(args, "contains", "")):
            print("(a zero-hit search proves nothing until you confirm the")
            print(" fingerprint appears in some transcript at all)")
        return

    hits.sort(key=hit_mtime, reverse=True)
    for path, size, found, hit_fmt in hits:
        mark = f"  matches={found}" if found is not None else ""
        print(f"{size / 1e6:7.1f} MB  {path}{mark}  format={hit_fmt}")


# --- index ----------------------------------------------------------------


def cmd_index(args: argparse.Namespace) -> None:
    """Summarize the whole transcript in one pass.

    This is the first command to run. It answers how big the session
    was, which tools carried it, and where the failures cluster, without
    putting any transcript prose into context.
    """
    parser = parser_for(args)
    path = as_str(getattr(args, "path", ""))
    kinds: Counter[str] = Counter()
    tools: Counter[str] = Counter()
    errors: list[int] = []
    sidechain = 0
    total = 0

    for lineno, rec in records(path):
        total += 1
        kinds[as_str(rec.get("type"), "?")] += 1
        if parser.is_sidechain(rec):
            sidechain += 1
        for name, _ in parser.tool_calls(rec):
            tools[name] += 1
        for kind, text in parser.blocks(rec):
            if kind == "RES" and parser.is_failure(rec, text):
                errors.append(lineno)

    print(f"format:         {parser.name}")
    print(f"records:        {total}")
    print(f"sidechain:      {sidechain}")
    print(f"record types:   {dict(kinds.most_common())}")
    print()
    print("tool calls:")
    for name, count in tools.most_common():
        print(f"  {count:5d}  {name}")
    print()
    print(f"failing results: {len(errors)}")
    if errors:
        print(f"  lines: {errors[:60]}")
    print()
    print("next: `errors` for what failed, `loops` for what repeated")


# --- errors ---------------------------------------------------------------


def cmd_errors(args: argparse.Namespace) -> None:
    """List every failing result with its line number and first line."""
    parser = parser_for(args)
    path = as_str(getattr(args, "path", ""))
    width: int = getattr(args, "width", SUMMARY_WIDTH)
    shown = 0
    for lineno, rec in records(path):
        for kind, text in parser.blocks(rec):
            if kind != "RES" or not parser.is_failure(rec, text):
                continue
            label = parser.classify(text)
            print(f"L{lineno:<6} [{label}] {flat(text, width)}")
            shown += 1
    print(f"\n{shown} failing result(s)")


# --- grep -----------------------------------------------------------------


def grep_record(
    parser: Parser, args: argparse.Namespace, pattern: re.Pattern[str], rec: Record
) -> Iterator[tuple[str, str]]:
    """Yield (kind, excerpt) for each match in one record."""
    wanted_kind = as_str(getattr(args, "kind", ""))
    context: int = getattr(args, "context", 250)
    first = bool(getattr(args, "first", False))
    for kind, text in parser.blocks(rec):
        if wanted_kind and not kind.startswith(wanted_kind):
            continue
        for match in pattern.finditer(text):
            start = max(0, match.start() - context)
            end = match.end() + context
            yield kind, flat(text[start:end], context * 2 + 200)
            if first:
                break


def cmd_grep(args: argparse.Namespace) -> None:
    """Show regex matches with surrounding context and line numbers.

    Searching the raw JSONL with the shell finds the record but hands
    back an escaped one-line blob. This searches the decoded blocks
    instead, so the context that prints is the text as the model saw it.
    """
    parser = parser_for(args)
    path = as_str(getattr(args, "path", ""))
    raw_pat = as_str(getattr(args, "pattern", ""))
    ignore_case = bool(getattr(args, "ignore_case", False))
    pattern = re.compile(raw_pat, re.IGNORECASE if ignore_case else 0)
    lo, hi = getattr(args, "range", (0, 10**9))
    count = 0
    for lineno, rec in records(path):
        if lineno < lo or lineno > hi:
            continue
        for kind, excerpt in grep_record(parser, args, pattern, rec):
            print(f"--- L{lineno} [{kind}] ---")
            print(excerpt)
            count += 1
    print(f"\n{count} match(es)")


# --- show -----------------------------------------------------------------


def cmd_show(args: argparse.Namespace) -> None:
    """Print whole records, decoded, for a set of line numbers."""
    parser = parser_for(args)
    path = as_str(getattr(args, "path", ""))
    lines_arg = as_str(getattr(args, "lines", ""))
    width: int = getattr(args, "width", 2000)
    wanted: set[int] = set()
    for raw in lines_arg.split(","):
        part = raw.strip()
        if "-" in part:
            first, last = part.split("-", 1)
            wanted.update(range(int(first), int(last) + 1))
        elif part:
            wanted.add(int(part))

    for lineno, rec in records(path):
        if lineno not in wanted:
            continue
        print(f"\n===== L{lineno} [{rec.get('type')}] =====")
        for kind, text in parser.blocks(rec):
            print(f"[{kind}]")
            print(text[:width])


# --- loops ----------------------------------------------------------------


def cluster_rounds(cluster: Cluster) -> int:
    return len(cluster[1])


def cmd_loops(args: argparse.Namespace) -> None:
    """Cluster repeated equivalent tool calls into retry loops.

    A loop is the unit of waste worth reporting: not one failure, but
    the same attempt made repeatedly with edits in between. Clusters are
    ranked by round count, which approximates the round trips wasted.
    """
    parser = parser_for(args)
    path = as_str(getattr(args, "path", ""))
    min_rounds: int = getattr(args, "min_rounds", 3)
    calls: list[tuple[int, str]] = []
    for lineno, rec in records(path):
        for name, inp in parser.tool_calls(rec):
            calls.append((lineno, parser.signature(name, inp)))

    groups: defaultdict[str, list[int]] = defaultdict(list)
    for lineno, sig in calls:
        groups[sig].append(lineno)

    clusters: list[Cluster] = []
    for sig, linenos in groups.items():
        run = [linenos[0]]
        for lineno in linenos[1:]:
            if lineno - run[-1] <= CLUSTER_GAP:
                run.append(lineno)
            else:
                clusters.append((sig, run))
                run = [lineno]
        clusters.append((sig, run))

    clusters = [c for c in clusters if len(c[1]) >= min_rounds]
    clusters.sort(key=cluster_rounds, reverse=True)

    total = 0
    for sig, run in clusters:
        span = f"L{run[0]}-{run[-1]}"
        print(f"{len(run):4d} rounds  {span:<16} {sig}")
        total += len(run) - 1
    print(f"\n{len(clusters)} loop(s), ~{total} repeat round trip(s) beyond first try")


# --- lint -----------------------------------------------------------------


def cmd_lint(args: argparse.Namespace) -> None:
    """Profile linter findings: which rules fired, and how they arrived.

    The distribution matters more than the total. Many failing runs each
    reporting one or two findings needs a different fix than a few runs
    reporting many at once.
    """
    parser = parser_for(args)
    path = as_str(getattr(args, "path", ""))
    top: int = getattr(args, "top", 25)
    rules: Counter[str] = Counter()
    per_run: list[int] = []
    for _, rec in records(path):
        for kind, text in parser.blocks(rec):
            found: list[str] = re.findall(r"\[error\] ([A-Za-z0-9_.-]+) match=", text)
            if kind == "RES" and (found or "[error]" in text):
                rules.update(found)
                per_run.append(len(found))

    failing = [n for n in per_run if n]
    print(f"runs reporting findings: {len(failing)}")
    print(f"total findings:          {sum(failing)}")
    if failing:
        few = sum(1 for n in failing if n <= FEW_FINDINGS)
        print(f"runs reporting 1-2:      {few} ({100 * few // len(failing)}%)")
        print(f"histogram:               {dict(sorted(Counter(failing).items()))}")
    print()
    print("rules by frequency:")
    for rule, count in rules.most_common(top):
        print(f"  {count:5d}  {rule}")


# --- meta ----------------------------------------------------------------


def cmd_meta(args: argparse.Namespace) -> None:
    """Print shell-consumable session metadata for new-retro.sh.

    The date comes from the session's own last stamped record rather than
    from whenever the scaffolding runs, so re-scaffolding an old
    transcript uses the directory it already had.
    """
    parser = parser_for(args)
    raw_path = as_str(getattr(args, "path", ""))
    path = Path(raw_path)
    stamp = ""
    total = 0
    for _, rec in records(raw_path):
        total += 1
        ts = as_str(rec.get("timestamp"))
        if ts:
            stamp = ts

    when = None
    if stamp:
        try:
            when = dt.datetime.fromisoformat(stamp).astimezone()
        except ValueError:
            when = None
    if when is None:
        when = dt.datetime.fromtimestamp(path.stat().st_mtime, tz=dt.UTC).astimezone()

    print(f"FORMAT={parser.name}")
    print(f"SESSION_ID={parser.session_id(path)}")
    print(f"SESSION_DATE={when.strftime('%Y%m%d')}")
    print(f"RECORDS={total}")
    print(f"SIZE_MB={path.stat().st_size / 1e6:.1f}")


# --- main -----------------------------------------------------------------


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="session", description="Query a Claude Code or Codex transcript."
    )
    # Every subcommand takes --format, so it can come after the verb
    # where the examples in the skill put it.
    fmt = argparse.ArgumentParser(add_help=False)
    fmt.add_argument(
        "--format",
        choices=("auto", *PARSERS),
        default="auto",
        help="transcript format; auto detects it from the first records",
    )
    sub = parser.add_subparsers(dest="cmd", required=True)

    loc = sub.add_parser(
        "locate", parents=[fmt], help="find transcripts by fingerprint"
    )
    loc.add_argument("--contains", help="string that must appear in the file")
    loc.add_argument(
        "--project",
        help="substring of the Claude project directory or the Codex session cwd",
    )
    loc.set_defaults(func=cmd_locate)

    for name, func, helptext in (
        ("index", cmd_index, "one-pass summary: size, tools, failures"),
        ("errors", cmd_errors, "every failing result with line numbers"),
        ("loops", cmd_loops, "clusters of repeated equivalent calls"),
        ("lint", cmd_lint, "linter rule frequency and arrival pattern"),
        ("meta", cmd_meta, "session id, date, size (for new-retro.sh)"),
    ):
        spec = sub.add_parser(name, parents=[fmt], help=helptext)
        spec.add_argument("path")
        spec.set_defaults(func=func)

    sub.choices["errors"].add_argument("--width", type=int, default=SUMMARY_WIDTH)
    sub.choices["loops"].add_argument("--min-rounds", type=int, default=3)
    sub.choices["lint"].add_argument("--top", type=int, default=25)

    grep = sub.add_parser(
        "grep", parents=[fmt], help="regex over decoded content blocks"
    )
    grep.add_argument("path")
    grep.add_argument("pattern")
    grep.add_argument("--context", type=int, default=250)
    grep.add_argument(
        "--kind", help="restrict to TEXT, THINK, USE:<tool>, RES, or EVENT"
    )
    grep.add_argument("--ignore-case", action="store_true")
    grep.add_argument("--first", action="store_true", help="one match per block")
    grep.add_argument(
        "--range",
        type=int,
        nargs=2,
        default=(0, 10**9),
        metavar=("LO", "HI"),
        help="restrict to a line range",
    )
    grep.set_defaults(func=cmd_grep)

    show = sub.add_parser(
        "show", parents=[fmt], help="print whole records by line number"
    )
    show.add_argument("path")
    show.add_argument("lines", help="comma-separated, ranges allowed: 12,40-45")
    show.add_argument("--width", type=int, default=2000)
    show.set_defaults(func=cmd_show)
    return parser


def main() -> None:
    args = build_parser().parse_args()
    path = as_str(getattr(args, "path", ""))
    if path and not Path(path).is_file():
        die(f"no such transcript: {path}")
    func = getattr(args, "func", None)
    if callable(func):
        func(args)


if __name__ == "__main__":
    main()
