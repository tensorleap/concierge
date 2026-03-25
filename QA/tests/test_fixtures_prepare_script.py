from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
import tempfile
import textwrap
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SOURCE_PREPARE_SCRIPT = REPO_ROOT / "scripts" / "fixtures_prepare.sh"
SOURCE_RESET_LIB = REPO_ROOT / "scripts" / "fixtures_reset_lib.sh"


class FixturesPrepareScriptTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        required_commands = ("bash", "git", "jq", "python3", "rg")
        missing = [command for command in required_commands if shutil.which(command) is None]
        if missing:
            raise unittest.SkipTest(f"missing required commands for fixtures_prepare.sh tests: {', '.join(missing)}")

    def test_targeted_prepare_only_materializes_selected_fixture(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            harness = FixturePrepareHarness(Path(tmpdir))

            completed = harness.run_prepare("--fixture", "alpha")

            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout={completed.stdout}\nstderr={completed.stderr}",
            )
            self.assertTrue((harness.fixtures_root / "alpha" / "post" / ".git").is_dir())
            self.assertTrue((harness.fixtures_root / "alpha" / "pre" / ".git").is_dir())
            self.assertFalse((harness.fixtures_root / "beta").exists())

    def test_default_prepare_materializes_every_fixture(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            harness = FixturePrepareHarness(Path(tmpdir))

            completed = harness.run_prepare()

            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout={completed.stdout}\nstderr={completed.stderr}",
            )
            for fixture_id in ("alpha", "beta"):
                self.assertTrue((harness.fixtures_root / fixture_id / "post" / ".git").is_dir(), fixture_id)
                self.assertTrue((harness.fixtures_root / fixture_id / "pre" / ".git").is_dir(), fixture_id)

    def test_targeted_prepare_does_not_touch_unselected_fixture_dirs(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            harness = FixturePrepareHarness(Path(tmpdir))

            first = harness.run_prepare("--fixture", "beta")
            self.assertEqual(first.returncode, 0, msg=f"stdout={first.stdout}\nstderr={first.stderr}")

            sentinel = harness.fixtures_root / "beta" / "keep.txt"
            sentinel.write_text("preserve me\n", encoding="utf-8")

            completed = harness.run_prepare("--fixture", "alpha")

            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout={completed.stdout}\nstderr={completed.stderr}",
            )
            self.assertTrue(sentinel.exists())
            self.assertEqual(sentinel.read_text(encoding="utf-8"), "preserve me\n")

    def test_targeted_prepare_matches_full_prepare_for_selected_fixture(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            upstream_root = tmp_path / "upstreams"
            upstreams = FixturePrepareHarness.create_upstreams(upstream_root)
            full_harness = FixturePrepareHarness(tmp_path / "full", upstreams=upstreams)
            targeted_harness = FixturePrepareHarness(tmp_path / "targeted", upstreams=upstreams)

            full = full_harness.run_prepare()
            self.assertEqual(full.returncode, 0, msg=f"stdout={full.stdout}\nstderr={full.stderr}")

            targeted = targeted_harness.run_prepare("--fixture", "alpha")
            self.assertEqual(targeted.returncode, 0, msg=f"stdout={targeted.stdout}\nstderr={targeted.stderr}")

            self.assertEqual(
                full_harness.capture_fixture_state("alpha"),
                targeted_harness.capture_fixture_state("alpha"),
            )

    def test_targeted_prepare_forwards_selector_to_bootstrap_script(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            harness = FixturePrepareHarness(Path(tmpdir))

            completed = harness.run_prepare("--fixture", "alpha", "--bootstrap-poetry")

            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout={completed.stdout}\nstderr={completed.stderr}",
            )
            self.assertEqual(
                harness.bootstrap_log.read_text(encoding="utf-8").strip(),
                "--variant all --fixture alpha",
            )


class FixturePrepareHarness:
    def __init__(self, root: Path, *, upstreams: dict[str, Path] | None = None) -> None:
        self.root = root
        self.repo_root = root / "repo"
        self.fixtures_root = self.repo_root / ".fixtures"
        self.bootstrap_log = root / "bootstrap.log"
        self.bin_dir = root / "bin"
        self.upstreams = upstreams or self.create_upstreams(root / "upstreams")
        self._write_repo_layout()
        self._write_fake_git()
        self._write_fake_poetry()

    @staticmethod
    def create_upstreams(base_dir: Path) -> dict[str, Path]:
        upstreams = {
            "alpha": base_dir / "alpha-source.git",
            "beta": base_dir / "beta-source.git",
        }
        for fixture_id, path in upstreams.items():
            create_fixture_source_repo(path, fixture_id)
        return upstreams

    def run_prepare(self, *args: str) -> subprocess.CompletedProcess[str]:
        env = os.environ.copy()
        env["PATH"] = f"{self.bin_dir}:{env['PATH']}"
        env["FIXTURE_BOOTSTRAP_LOG"] = str(self.bootstrap_log)
        return subprocess.run(
            ["bash", str(self.repo_root / "scripts" / "fixtures_prepare.sh"), *args],
            cwd=self.repo_root,
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )

    def capture_fixture_state(self, fixture_id: str) -> dict[str, object]:
        fixture_root = self.fixtures_root / fixture_id
        state: dict[str, object] = {}
        for variant in ("post", "pre"):
            variant_root = fixture_root / variant
            state[f"{variant}_head"] = git_output(variant_root, "rev-parse", "HEAD")
            state[f"{variant}_reset_script"] = (variant_root / ".fixture_reset.sh").read_text(encoding="utf-8")
            state[f"{variant}_tree"] = hash_repo_tree(variant_root)
        return state

    def _write_repo_layout(self) -> None:
        scripts_dir = self.repo_root / "scripts"
        fixtures_dir = self.repo_root / "fixtures"
        scripts_dir.mkdir(parents=True, exist_ok=True)
        fixtures_dir.mkdir(parents=True, exist_ok=True)

        shutil.copy2(SOURCE_PREPARE_SCRIPT, scripts_dir / "fixtures_prepare.sh")
        shutil.copy2(SOURCE_RESET_LIB, scripts_dir / "fixtures_reset_lib.sh")
        os.chmod(scripts_dir / "fixtures_prepare.sh", 0o755)
        os.chmod(scripts_dir / "fixtures_reset_lib.sh", 0o755)

        (scripts_dir / "fixtures_bootstrap_poetry.sh").write_text(
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            "printf '%s\\n' \"$*\" > \"${FIXTURE_BOOTSTRAP_LOG}\"\n",
            encoding="utf-8",
        )
        os.chmod(scripts_dir / "fixtures_bootstrap_poetry.sh", 0o755)

        (fixtures_dir / "manifest.json").write_text(
            textwrap.dedent(
                f"""\
                {{
                  "fixtures": [
                    {{
                      "id": "alpha",
                      "repo": "{self.upstreams['alpha'].resolve()}",
                      "post_ref": "{git_output(self.upstreams['alpha'], 'rev-parse', 'HEAD')}",
                      "strip_for_pre": [
                        "leap.yaml",
                        "leap_integration.py"
                      ]
                    }},
                    {{
                      "id": "beta",
                      "repo": "{self.upstreams['beta'].resolve()}",
                      "post_ref": "{git_output(self.upstreams['beta'], 'rev-parse', 'HEAD')}",
                      "strip_for_pre": [
                        "leap.yaml",
                        "leap_integration.py"
                      ]
                    }}
                  ]
                }}
                """
            ),
            encoding="utf-8",
        )

    def _write_fake_poetry(self) -> None:
        self.bin_dir.mkdir(parents=True, exist_ok=True)
        poetry_path = self.bin_dir / "poetry"
        poetry_path.write_text(
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            "exit 0\n",
            encoding="utf-8",
        )
        os.chmod(poetry_path, 0o755)

    def _write_fake_git(self) -> None:
        self.bin_dir.mkdir(parents=True, exist_ok=True)
        real_git = shutil.which("git")
        if real_git is None:
            raise unittest.SkipTest("git is required for fixtures_prepare.sh tests")

        git_path = self.bin_dir / "git"
        git_path.write_text(
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            "if [[ $# -gt 0 && \"$1\" == \"clone\" ]]; then\n"
            "  args=()\n"
            "  shift\n"
            "  for arg in \"$@\"; do\n"
            "    case \"$arg\" in\n"
            "      --quiet|--no-checkout|--filter=blob:none)\n"
            "        continue\n"
            "        ;;\n"
            "    esac\n"
            "    args+=(\"$arg\")\n"
            "  done\n"
            "  src=\"${args[0]}\"\n"
            "  dest=\"${args[1]}\"\n"
            "  mkdir -p \"$(dirname \"$dest\")\"\n"
            f"  {real_git} init --quiet \"$dest\"\n"
            f"  {real_git} -C \"$dest\" remote add origin \"$src\"\n"
            f"  {real_git} -C \"$dest\" fetch --quiet origin \"+refs/heads/*:refs/remotes/origin/*\"\n"
            "  exit 0\n"
            "fi\n"
            "args=()\n"
            "for arg in \"$@\"; do\n"
            "  if [[ \"$arg\" == \"--filter=blob:none\" ]]; then\n"
            "    continue\n"
            "  fi\n"
            "  args+=(\"$arg\")\n"
            "done\n"
            f"exec {real_git} \"${{args[@]}}\"\n",
            encoding="utf-8",
        )
        os.chmod(git_path, 0o755)


def create_fixture_source_repo(path: Path, fixture_id: str) -> None:
    worktree_path = path.parent / f"{fixture_id}-work"
    worktree_path.mkdir(parents=True, exist_ok=True)
    subprocess.run(["git", "-C", str(worktree_path), "init", "-b", "main"], capture_output=True, text=True, check=True)
    subprocess.run(["git", "-C", str(worktree_path), "config", "user.name", "Fixture Source"], capture_output=True, text=True, check=True)
    subprocess.run(
        ["git", "-C", str(worktree_path), "config", "user.email", "fixture-source@example.com"],
        capture_output=True,
        text=True,
        check=True,
    )

    (worktree_path / "pyproject.toml").write_text(
        textwrap.dedent(
            f"""\
            [tool.poetry]
            name = "{fixture_id}"
            version = "0.1.0"
            description = "fixture"
            authors = ["Fixture Source <fixture-source@example.com>"]

            [tool.poetry.dependencies]
            python = "^3.11"
            code-loader = "^1.0.165"
            """
        ),
        encoding="utf-8",
    )
    (worktree_path / "poetry.lock").write_text(
        textwrap.dedent(
            """\
            [[package]]
            name = "code-loader"
            version = "1.0.165"
            description = "fixture"
            optional = false
            python-versions = ">=3.11,<4.0"
            """
        ),
        encoding="utf-8",
    )
    (worktree_path / "leap.yaml").write_text("entryFile: leap_integration.py\n", encoding="utf-8")
    (worktree_path / "leap_integration.py").write_text(
        "from code_loader import decorators\n\n"
        "def build_fixture_name() -> str:\n"
        f"    return '{fixture_id}'\n",
        encoding="utf-8",
    )
    (worktree_path / "notes.txt").write_text(f"fixture {fixture_id}\n", encoding="utf-8")

    subprocess.run(["git", "-C", str(worktree_path), "add", "."], capture_output=True, text=True, check=True)
    subprocess.run(
        ["git", "-C", str(worktree_path), "commit", "--quiet", "-m", f"Seed {fixture_id} fixture"],
        capture_output=True,
        text=True,
        check=True,
    )
    subprocess.run(["git", "clone", "--bare", str(worktree_path), str(path)], capture_output=True, text=True, check=True)


def git_output(repo_dir: Path, *args: str) -> str:
    completed = subprocess.run(
        ["git", "-C", str(repo_dir), *args],
        capture_output=True,
        text=True,
        check=False,
    )
    if completed.returncode != 0:
        raise AssertionError(f"git {' '.join(args)} failed:\nstdout={completed.stdout}\nstderr={completed.stderr}")
    return completed.stdout.strip()


def hash_repo_tree(repo_dir: Path) -> dict[str, str]:
    result: dict[str, str] = {}
    for path in sorted(repo_dir.rglob("*")):
        if ".git" in path.parts or path.is_dir():
            continue
        rel_path = path.relative_to(repo_dir).as_posix()
        result[rel_path] = hashlib.sha256(path.read_bytes()).hexdigest()
    return result


if __name__ == "__main__":
    unittest.main()
