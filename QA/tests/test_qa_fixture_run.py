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

    def test_qa_python_runtime_imports_without_stdlib_tomllib(self) -> None:
        script = f"""
import builtins
import importlib.util
import tempfile
import types
from pathlib import Path

real_import = builtins.__import__

def fake_import(name, globals=None, locals=None, fromlist=(), level=0):
    if name == "tomllib":
        raise ModuleNotFoundError("No module named 'tomllib'")
    if name == "tomli":
        module = types.ModuleType("tomli")

        def loads(text):
            return {{"tool": {{"poetry": {{"dependencies": {{"python": ">=3.9,<3.11"}}}}}}}}

        module.loads = loads
        return module
    return real_import(name, globals, locals, fromlist, level)

builtins.__import__ = fake_import
module_path = Path({str(REPO_ROOT / "scripts" / "qa_python_runtime.py")!r})
spec = importlib.util.spec_from_file_location("qa_python_runtime_under_test", module_path)
module = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(module)

with tempfile.TemporaryDirectory() as tmpdir:
    pyproject = Path(tmpdir) / "pyproject.toml"
    pyproject.write_text(
        "[tool.poetry]\\n"
        "name = \\"fixture\\"\\n"
        "version = \\"0.1.0\\"\\n\\n"
        "[tool.poetry.dependencies]\\n"
        "python = \\">=3.9,<3.11\\"\\n",
        encoding="utf-8",
    )
    print(module.resolve_python_version(pyproject))
"""
        completed = subprocess.run(
            [sys.executable, "-c", script],
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
            check=False,
        )

        self.assertEqual(completed.returncode, 0, completed.stderr)
        self.assertEqual(completed.stdout.strip(), "3.10.16")

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
