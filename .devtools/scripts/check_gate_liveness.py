#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.14"
# dependencies = []
# ///
"""Prove that the published tree-walking gates reach nested defects."""

import json
import os
import shutil
import subprocess
import tempfile
from pathlib import Path

# Keep task names exact: task presence, rather than aggregate membership,
# determines which consumer gates participate.
DEFECTS = {
    "lint-prose": ("md", "# Probe\n\nThis is very unique.\n"),
    "devtools:lint-markdown": ("md", "# Probe\n\n# Probe\n"),
    "lint-markdown-wrap": (
        "md",
        "# Probe\n\nThis paragraph starts here\nand continues here.\n",
    ),
    "lint-shell": ("sh", '#!/bin/sh\nprintf "%s\\n" $gate_liveness_unset\n'),
    "lint-shell-fmt": ("sh", "#!/bin/sh\nif true;then echo yes;fi\n"),
    "devtools:lint-curl": ("sh", "#!/bin/sh\ncurl https://example.com\n"),
}


def command(
    args: list[str], cwd: Path, env: dict[str, str]
) -> subprocess.CompletedProcess[str]:
    """Capture diagnostics so only the gate under examination is reported."""
    return subprocess.run(
        args, cwd=cwd, env=env, text=True, capture_output=True, check=False
    )


def snapshot(source: Path, target: Path) -> list[str]:
    """Copy Git-visible content plus installed Vale styles, without Git state."""
    paths = (
        subprocess.check_output(
            ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
            cwd=source,
        )
        .decode()
        .split("\0")
    )
    copied = []
    for name in dict.fromkeys(filter(None, paths)):
        src = source / name
        if not src.is_file() and not src.is_symlink():
            continue
        dst = target / name
        dst.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(src, dst, follow_symlinks=True)
        copied.append(name)
    if (source / ".vale").is_dir():
        shutil.copytree(source / ".vale", target / ".vale", dirs_exist_ok=True)
    return copied


def probe(source: Path, gate: str, suffix: str, defect: str) -> bool:
    """Use a fresh tree and caches for each gate, including its baseline."""
    with tempfile.TemporaryDirectory(prefix="gate-liveness-") as temporary:
        base = Path(temporary)
        tree = base / "tree"
        tree.mkdir()
        env = os.environ.copy()
        # These gates use mise, uv, Go and repository-local lint caches.
        # Never send planted content to their usual shared cache directories.
        for variable, directory in {
            "XDG_CACHE_HOME": "cache",
            "UV_CACHE_DIR": "uv",
            "GOCACHE": "go",
            "MISE_CACHE_DIR": "mise",
            "TMPDIR": "tmp",
        }.items():
            location = base / directory
            location.mkdir()
            env[variable] = str(location)
        env["MISE_TRUSTED_CONFIG_PATHS"] = str(tree)
        env["DEVTOOLS_DIR"] = str(tree / ".devtools")
        paths = snapshot(source, tree)
        subprocess.run(["git", "init", "--quiet", str(tree)], check=True, env=env)
        for offset in range(0, len(paths), 100):
            subprocess.run(
                ["git", "add", "--force", "--", *paths[offset : offset + 100]],
                cwd=tree,
                env=env,
                check=True,
                capture_output=True,
            )
        nested = f"gate-liveness/nested/probe.{suffix}"
        baseline = command(["mise", "run", gate], tree, env)
        if baseline.returncode:
            print(f"FAIL {gate}: baseline failed before planting {nested}")
            print(baseline.stdout + baseline.stderr)
            return False
        (tree / nested).parent.mkdir(parents=True)
        (tree / nested).write_text(defect)
        subprocess.run(
            ["git", "add", "--force", "--", str(nested)], cwd=tree, env=env, check=True
        )
        result = command(["mise", "run", gate], tree, env)
        output = result.stdout + result.stderr
        if result.returncode and nested in output.replace("\\", "/"):
            print(f"PASS {gate}: rejected {nested}")
            return True
        print(f"FAIL {gate}: did not reject {nested}")
        print(output)
        return False


def main() -> int:
    """Discover tasks and exercise exactly the supported gates."""
    source = Path(
        subprocess.check_output(
            ["git", "rev-parse", "--show-toplevel"], text=True
        ).strip()
    )
    tasks = json.loads(
        subprocess.check_output(["mise", "tasks", "--json"], cwd=source, text=True)
    )
    names = {task["name"] for task in tasks}
    success = True
    for gate, (suffix, defect) in DEFECTS.items():
        if gate not in names:
            print(f"SKIP {gate}: task not defined")
        elif not probe(source, gate, suffix, defect):
            success = False
    return 0 if success else 1


if __name__ == "__main__":
    raise SystemExit(main())
