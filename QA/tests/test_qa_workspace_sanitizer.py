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
            git_global_config = tmp_path / "gitconfig"
            git_template_dir = tmp_path / "git-template"

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

            git_global_config.write_text(
                "[remote \"origin\"]\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n",
                encoding="utf-8",
            )
            git_template_dir.mkdir()
            (git_template_dir / "config").write_text(
                (
                    "[remote \"origin\"]\n"
                    "\turl = https://github.com/example/template.git\n"
                    "\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
                ),
                encoding="utf-8",
            )
            git_env = os.environ.copy()
            git_env["GIT_CONFIG_GLOBAL"] = str(git_global_config)
            git_env["GIT_TEMPLATE_DIR"] = str(git_template_dir)

            completed = subprocess.run(
                ["bash", str(SANITIZER), str(source), str(output)],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                check=False,
                env=git_env,
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

            inherited_remotes = self._git(output, "remote", env=git_env).strip()
            self.assertEqual(inherited_remotes, "origin")

            local_remote_config = subprocess.run(
                ["git", "-C", str(output), "config", "--local", "--get-regexp", r"^remote\."],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                check=False,
                env=git_env,
            )
            self.assertEqual(
                local_remote_config.returncode,
                1,
                msg=(
                    "expected sanitized repo to have no repo-local remote config, got "
                    f"stdout={local_remote_config.stdout!r} stderr={local_remote_config.stderr!r}"
                ),
            )

            leak_show = subprocess.run(
                ["git", "-C", str(output), "show", "HEAD~1:leak.txt"],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertNotEqual(leak_show.returncode, 0)

    def test_sanitizer_ignores_checkpoint_warmup_helper(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            source = tmp_path / "source"
            output = tmp_path / "output"

            source.mkdir()
            self._git(source, "init")
            self._git(source, "config", "user.name", "Fixture Source")
            self._git(source, "config", "user.email", "fixture-source@example.com")

            (source / "tracked.txt").write_text("tracked\n", encoding="utf-8")
            self._git(source, "add", "tracked.txt")
            self._git(source, "commit", "-m", "seed sanitized repo")

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

            (output / ".checkpoint_warmup.sh").write_text("#!/usr/bin/env bash\necho warm\n", encoding="utf-8")
            status = self._git(output, "status", "--porcelain")
            self.assertEqual(status.strip(), "", msg=f"expected helper to be ignored, got git status {status!r}")

    def _git(self, repo: Path, *args: str, env: dict[str, str] | None = None) -> str:
        completed = subprocess.run(
            ["git", "-C", str(repo), *args],
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
            check=False,
            env=env,
        )
        if completed.returncode != 0:
            self.fail(f"git {' '.join(args)} failed:\nstdout={completed.stdout}\nstderr={completed.stderr}")
        return completed.stdout


if __name__ == "__main__":
    unittest.main()
