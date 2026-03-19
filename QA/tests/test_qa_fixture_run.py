from __future__ import annotations

import os
import subprocess
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]


class QAFixtureRunScriptTest(unittest.TestCase):
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
