#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.14"
# dependencies = []
# ///
"""Measure hook subprocess latency on an agent host."""

from __future__ import annotations

import json
import os
import subprocess
import tempfile
import time
from pathlib import Path

WARM_P99_LIMIT_MS = 20


def main() -> int:
    root = Path(__file__).resolve().parents[1]
    events = [
        "SessionStart",
        "PreToolUse",
        "PostToolUse",
        "PreInvocation",
        "PostInvocation",
        "Stop",
    ]
    inputs = [
        (root / "cmd/agy-otel/testdata" / f"{event}.json").read_bytes()
        for event in events
    ]
    with tempfile.TemporaryDirectory(prefix="agy-otel-latency-") as directory:
        for size in (10 * 1024, 1024 * 1024):
            transcript = Path(directory) / f"transcript-{size}.jsonl"
            response = (
                json.dumps(
                    {
                        "type": "PLANNER_RESPONSE",
                        "input_tokens": 12,
                        "output_tokens": 23,
                        "cache_read_tokens": 7,
                    }
                )
                + "\n"
            )
            transcript.write_text(" " * (size - len(response) - 1) + "\n" + response)
            events.append("PostInvocation")
            inputs.append(json.dumps({"transcriptPath": str(transcript)}).encode())
        binary = Path(directory) / "agy-otel"
        subprocess.run(
            ["go", "build", "-o", str(binary), "./cmd/agy-otel"],
            cwd=root,
            check=True,
        )
        env = {**os.environ, "XDG_STATE_HOME": str(Path(directory) / "state")}

        def measure(index: int) -> float:
            index %= len(events)
            start = time.perf_counter_ns()
            result = subprocess.run(
                [str(binary), "hook", events[index]],
                input=inputs[index],
                capture_output=True,
                env=env,
                check=True,
            )
            elapsed_ms = (time.perf_counter_ns() - start) / 1_000_000
            if result.stdout or result.stderr:
                message = "hook produced output"
                raise RuntimeError(message)
            return elapsed_ms

        print(f"First cold call (non-gating): {measure(0):.3f} ms", flush=True)
        for index in range(20):
            measure(index)
        durations = sorted(measure(index) for index in range(500))
        p99 = durations[494]
        print(
            "Warm: 20 warm-up calls, 500 hook subprocesses "
            "over hook/transcript fixtures: "
            f"p50 = {durations[249]:.3f} ms, p99 = {p99:.3f} ms, "
            f"max = {durations[-1]:.3f} ms",
            flush=True,
        )
        print("Pausing 60 s before post-idle sample...", flush=True)
        time.sleep(60)
        print(f"Post-idle call (non-gating): {measure(0):.3f} ms", flush=True)
        if p99 > WARM_P99_LIMIT_MS:
            print("FAIL: warm p99 exceeds 20 ms", flush=True)
            return 1
        print("PASS: warm p99 <= 20 ms", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
