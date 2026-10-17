"""Keep diagnosis commands aligned with the shipped task contracts."""

import re
import subprocess
import tomllib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[4]
SCRIPT = ROOT / "packages/agents/source/skills/fix-pr/shared/scripts/diagnose.sh"
SOURCE = SCRIPT.read_text()
REPRODUCER = SOURCE.split("reproducer() {\n", 1)[1].split("\n}\n", 1)[0]
SHARED_TASKS: set[str] = set()
for task_file in (ROOT / ".devtools/tasks").glob("*.toml"):
    SHARED_TASKS.update(tomllib.loads(task_file.read_text()))
REPO_TASKS = tomllib.loads((ROOT / "mise.toml").read_text())["tasks"]


def test_every_mapping_names_defined_tasks_and_prefers_shared_names() -> None:
    """Exercise every case pattern, including both commands in shell checks."""
    for line in REPRODUCER.splitlines():
        if ";;" not in line:
            continue
        patterns = line.split(")", 1)[0].strip().split(" | ")
        for pattern in patterns:
            job = pattern.replace("*", "") or "unknown job"
            result = subprocess.run(
                [
                    "bash",
                    "-c",
                    "reproducer() {\n" + REPRODUCER + '\n}\nreproducer "$1"',
                    "bash",
                    job,
                ],
                capture_output=True,
                text=True,
                check=True,
            )
            for match in re.finditer(r"mise run ([\w:-]+)", result.stdout):
                task: str = match.group(1)
                assert task in SHARED_TASKS or task in REPO_TASKS, (job, task)
                assert "devtools:" + task not in SHARED_TASKS, (job, task)
