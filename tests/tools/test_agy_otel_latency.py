from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def test_task_rejects_slow_hook(tmp_path: Path) -> None:
    # Copy the committed task into a tool-free project so its build uses our stub.
    task = (ROOT / "mise.toml").read_text(encoding="utf-8")
    task = task.split("[tasks.agy-otel-latency]\n", 1)[1].split("\n[", 1)[0]
    task = task.replace(
        "tools/agy_otel_latency.py", str(ROOT / "tools/agy_otel_latency.py")
    )
    (tmp_path / "mise.toml").write_text(
        "[tasks.agy-otel-latency]\n" + task, encoding="utf-8"
    )
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    # The fixture has no uv pin for mise's shim to resolve under MISE_LOCKED.
    uv = subprocess.run(
        ["mise", "which", "uv"],
        cwd=ROOT,
        stdout=subprocess.PIPE,
        text=True,
        check=True,
    ).stdout.strip()
    (bin_dir / "uv").symlink_to(uv)
    compiler = bin_dir / "go"
    compiler.write_text(
        f"#!{sys.executable}\n"
        "import pathlib, sys\n"
        "assert sys.argv[1:3] == ['build', '-o']\n"
        "assert sys.argv[4:] == ['./cmd/agy-otel']\n"
        "binary = pathlib.Path(sys.argv[3])\n"
        f"binary.write_text('#!{sys.executable}\\n'"
        " + 'import time\\ntime.sleep(0.025)\\n')\n"
        "binary.chmod(0o755)\n",
        encoding="utf-8",
    )
    compiler.chmod(0o755)
    result = subprocess.run(
        ["mise", "run", "agy-otel-latency"],
        cwd=tmp_path,
        env={
            **os.environ,
            "PATH": str(bin_dir) + os.pathsep + os.environ["PATH"],
            "MISE_LOCKED": "1",
            "MISE_TRUSTED_CONFIG_PATHS": str(tmp_path),
        },
        capture_output=True,
        text=True,
        check=False,
        timeout=120,
    )
    sys.stdout.write(result.stdout)
    sys.stderr.write(result.stderr)
    assert result.returncode != 0, result.stdout + result.stderr
    assert "First cold call (non-gating):" in result.stdout
    assert (
        "20 warm-up calls, 500 hook subprocesses over hook/transcript fixtures:"
        in result.stdout
    )
    assert "p50 =" in result.stdout
    assert "p99 =" in result.stdout
    assert "max =" in result.stdout
    assert "Post-idle call (non-gating):" in result.stdout
    assert "FAIL: warm p99 exceeds 20 ms" in result.stdout
