from __future__ import annotations

import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SANITIZER = REPO_ROOT / "scripts" / "qa_sanitize_workspace.sh"


class QAWorkspaceSanitizerTest(unittest.TestCase):
    def test_sanitizer_rewrites_git_state_without_leaking_history(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            source = tmp_path / "source"
            output = tmp_path / "output"

            source.mkdir()
            self._git(source, "init")
            self._git(source, "config", "user.name", "Fixture Source")
            self._git(source, "config", "user.email", "fixture-source@example.com")

            (source / "tracked.txt").write_text("tracked\n", encoding="utf-8")
            (source / "leak.txt").write_text("secret integration\n", encoding="utf-8")
            self._git(source, "add", "tracked.txt", "leak.txt")
            self._git(source, "commit", "-m", "seed integrated history")

            (source / "tool.sh").write_text("#!/usr/bin/env bash\necho ok\n", encoding="utf-8")
            os.chmod(source / "tool.sh", os.stat(source / "tool.sh").st_mode | stat.S_IXUSR)
            (source / "safe.txt").write_text("current fixture state\n", encoding="utf-8")
            (source / "leak.txt").unlink()
            self._git(source, "add", "-A")
            self._git(source, "commit", "-m", "current fixture state")

            self._git(source, "remote", "add", "origin", "https://github.com/example/upstream.git")
            (source / ".fixture_reset.sh").write_text("#!/usr/bin/env bash\necho reset\n", encoding="utf-8")
            os.chmod(source / ".fixture_reset.sh", os.stat(source / ".fixture_reset.sh").st_mode | stat.S_IXUSR)

            completed = subprocess.run(
                ["bash", str(SANITIZER), str(source), str(output)],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout={completed.stdout}\nstderr={completed.stderr}",
            )
            self.assertTrue((output / ".git").is_dir())
            self.assertFalse((output / ".fixture_reset.sh").exists())
            self.assertEqual((output / "safe.txt").read_text(encoding="utf-8"), "current fixture state\n")
            self.assertTrue(os.access(output / "tool.sh", os.X_OK))

            head_ref = self._git(output, "rev-parse", "--abbrev-ref", "HEAD").strip()
            self.assertEqual(head_ref, "HEAD")

            commit_count = self._git(output, "rev-list", "--count", "HEAD").strip()
            self.assertEqual(commit_count, "1")

            remotes = self._git(output, "remote").strip()
            self.assertEqual(remotes, "")

            leak_show = subprocess.run(
                ["git", "-C", str(output), "show", "HEAD~1:leak.txt"],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertNotEqual(leak_show.returncode, 0)

    def _git(self, repo: Path, *args: str) -> str:
        completed = subprocess.run(
            ["git", "-C", str(repo), *args],
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
            check=False,
        )
        if completed.returncode != 0:
            self.fail(f"git {' '.join(args)} failed:\nstdout={completed.stdout}\nstderr={completed.stderr}")
        return completed.stdout


if __name__ == "__main__":
    unittest.main()
