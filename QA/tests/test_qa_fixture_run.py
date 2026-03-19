from __future__ import annotations

import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
if str(REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(REPO_ROOT))

from scripts.qa_python_runtime import resolve_python_version


class QAFixtureRunScriptTest(unittest.TestCase):
    def test_resolve_python_version_picks_highest_curated_match(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            pyproject = Path(tmpdir) / "pyproject.toml"
            pyproject.write_text(
                """
[tool.poetry]
name = "fixture"
version = "0.1.0"

[tool.poetry.dependencies]
python = ">=3.9,<3.11"
""".strip()
                + "\n",
                encoding="utf-8",
            )

            self.assertEqual(resolve_python_version(pyproject), "3.10.16")

    def test_runner_requires_repo_and_step_in_non_interactive_shell(self) -> None:
        env = os.environ.copy()
        env["REPO"] = ""
        env["QA_STEP"] = ""
        env.pop("ANTHROPIC_API_KEY", None)

        completed = subprocess.run(
            ["bash", str(REPO_ROOT / "scripts" / "qa_fixture_run.sh")],
            cwd=REPO_ROOT,
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )

        self.assertEqual(completed.returncode, 1)
        self.assertIn("missing required QA selectors for non-interactive run", completed.stderr)
        self.assertIn("Valid fixtures:", completed.stderr)
        self.assertIn("Valid steps:", completed.stderr)


if __name__ == "__main__":
    unittest.main()
