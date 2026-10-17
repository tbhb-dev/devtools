from __future__ import annotations

import runpy
from pathlib import Path
from typing import TYPE_CHECKING, cast

import pytest

if TYPE_CHECKING:
    from collections.abc import Callable, Iterator

SCRIPT = Path(__file__).resolve().parents[2] / ".devtools/scripts/check_curl_fail.py"
SUBJECT = runpy.run_path(str(SCRIPT))
findings = cast("Callable[[Path], Iterator[str]]", SUBJECT["findings"])


@pytest.mark.parametrize(
    "source",
    [
        'curl -s "$url" | jq -r .uid',
        'body=$(curl -sS -m 15 "$url")',
        'body=$(\n  curl -sS "$url" |\n    jq -r .status\n)',
        'fetch() { curl -s "$1"; }; body=$(fetch "$url")',
        'curl -s "$url" >"$out"; jq -r .status "$out"',
        'curl -s "$url" \\\n  --data "q=up" | jq .',
        "curl -H 'X-Mode: --fail ' url | jq .",
        'curl -s "$url/%{http_code}" | jq .',
        'code=$(curl -s -o /dev/null -w "%{http_code}" "$url")',
        "sum=$(curl -sSL url | cut -d' ' -f1)",
        "# it doesn't matter\ncat <<EOF\nit's text\nEOF\ncurl url | jq .",
        "# lint-curl: allow\nbody=$(curl url)",
        "# lint-curl: allow   \nbody=$(curl url)",
        "# lint-curl: allow intentional\n\nbody=$(curl url)",
        'curl "$flags" url',
        'code=$(curl -w "%{http_code}" url); value=$((code + 1))',
        "# lint-curl: allow first only\ncurl url; curl url",
        'code=$(curl -w "%{http_code}" url); test -n "$code"',
        'curl -w "%{http_code}" url | cat',
        'code=$(curl -w "%{http_code}" url); other=200; [[ "$other" == 200 ]]',
        'code=$(curl -w "%{http_code}" url); code=200; [[ "$code" == 200 ]]',
    ],
)
def test_rejects_http_errors_as_success(source: str, tmp_path: Path) -> None:
    script = tmp_path / "bad.sh"
    script.write_text(source)
    assert list(findings(script))


@pytest.mark.parametrize(
    "source",
    [
        'a=$(curl -f "$url")',
        'b=$(curl --fail "$url")',
        'c=$(curl --fail-with-body "$url")',
        'curl -fsSL "$url" | sh',
        "d=$(curl '--fail' url)",
        'e="$(curl -s "--fail" "$url")"',
        "curl -s url \\\n --fail | jq .",
        'x=$(bash tools/nas.sh in-network-curl "$id" | jq .)',
        "O11Y_CHECK_CURL=curl tools/receipt.sh",
        'remote "docker exec -i $id curl -s $url"',
        "cat <<EOF\ncurl url | jq .\nEOF",
        "# lint-curl: allow the body is an error page we assert on\nbody=$(curl url)",
        'code=$(curl -o /dev/null -w "%{http_code}" url); [[ "$code" == 200 ]]',
        (
            'code=$(curl --write-out "%{http_code}" url); '
            'if [ "${code}" -ne 200 ]; then exit 1; fi'
        ),
        '[[ $(curl -w "%{http_code}" url) == 200 ]]',
        'code=$(curl -w "%{http_code}" url); test "$code" -eq 200',
        '"$command" --fail url',
        "",
    ],
)
def test_accepts_safe_commands(source: str, tmp_path: Path) -> None:
    script = tmp_path / "good.sh"
    script.write_text(source)
    assert list(findings(script)) == []


def test_reports_each_curl_and_its_line(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    script = tmp_path / "mixed.sh"
    script.write_text(
        "#!/bin/bash\n# lint-curl: allow intentional error body\n"
        "body=$(curl url)\nagain=$(curl url)\ncurl --fail url\ncurl url | cat\n"
    )
    assert SUBJECT["main"]([str(script)]) == 1
    output = capsys.readouterr().out.splitlines()
    assert len(output) == 2
    assert output[0].startswith(f"{script}:4: curl without a fail option;")
    assert output[1].startswith(f"{script}:6: curl without a fail option;")


def test_clean_cli_and_empty_selection(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    script = tmp_path / "clean.sh"
    script.write_text("curl --fail url\n")
    assert SUBJECT["main"]([str(script)]) == 0
    assert SUBJECT["main"]([]) == 0
    assert capsys.readouterr().out == ""
