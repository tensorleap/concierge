from __future__ import annotations

import json
import os
import shutil
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]


class FixturesMutateCasesScriptTest(unittest.TestCase):
    def test_targeted_mode_generates_only_selected_case_repo(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = self._create_test_repo(Path(tmpdir))

            completed = subprocess.run(
                ["bash", str(repo_root / "scripts" / "fixtures_mutate_cases.sh"), "--case", "mnist_case"],
                cwd=repo_root,
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertTrue((repo_root / ".fixtures" / "cases" / "mnist_case" / ".git").is_dir())
            self.assertTrue((repo_root / ".fixtures" / "cases" / "mnist_case" / ".fixture_reset.sh").is_file())
            self.assertFalse((repo_root / ".fixtures" / "cases" / "ultralytics_case").exists())

    def test_default_mode_generates_all_case_repos(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = self._create_test_repo(Path(tmpdir))

            completed = subprocess.run(
                ["bash", str(repo_root / "scripts" / "fixtures_mutate_cases.sh")],
                cwd=repo_root,
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertTrue((repo_root / ".fixtures" / "cases" / "mnist_case" / ".git").is_dir())
            self.assertTrue((repo_root / ".fixtures" / "cases" / "ultralytics_case" / ".git").is_dir())

    def _create_test_repo(self, repo_root: Path) -> Path:
        (repo_root / "scripts").mkdir(parents=True, exist_ok=True)
        (repo_root / "fixtures" / "cases" / "patches").mkdir(parents=True, exist_ok=True)
        (repo_root / ".fixtures").mkdir(parents=True, exist_ok=True)

        self._copy_script("scripts/fixtures_mutate_cases.sh", repo_root)
        self._copy_script("scripts/fixtures_reset_lib.sh", repo_root)
        self._write_bootstrap_stub(repo_root)
        (repo_root / "fixtures" / "cases" / "schema.json").write_text("{}\n", encoding="utf-8")

        cases = [
            self._create_fixture_case(
                repo_root=repo_root,
                fixture_id="mnist",
                case_id="mnist_case",
                tracked_filename="README.md",
                base_content="# Fixture\n",
                mutated_content="# Fixture\n\nmnist case\n",
            ),
            self._create_fixture_case(
                repo_root=repo_root,
                fixture_id="ultralytics",
                case_id="ultralytics_case",
                tracked_filename="README.md",
                base_content="# Fixture\n",
                mutated_content="# Fixture\n\nultralytics case\n",
            ),
        ]

        (repo_root / "fixtures" / "cases" / "manifest.json").write_text(
            json.dumps({"cases": cases}, indent=2) + "\n",
            encoding="utf-8",
        )
        return repo_root

    def _copy_script(self, relative_path: str, repo_root: Path) -> None:
        source_path = REPO_ROOT / relative_path
        target_path = repo_root / relative_path
        target_path.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source_path, target_path)
        target_path.chmod(target_path.stat().st_mode | stat.S_IXUSR)

    def _write_bootstrap_stub(self, repo_root: Path) -> None:
        target_path = repo_root / "scripts" / "fixtures_bootstrap_poetry.sh"
        target_path.write_text(
            "#!/usr/bin/env bash\nset -euo pipefail\nexit 0\n",
            encoding="utf-8",
        )
        target_path.chmod(target_path.stat().st_mode | stat.S_IXUSR)

    def _create_fixture_case(
        self,
        *,
        repo_root: Path,
        fixture_id: str,
        case_id: str,
        tracked_filename: str,
        base_content: str,
        mutated_content: str,
    ) -> dict[str, object]:
        source_dir = repo_root / ".fixtures" / fixture_id / "post"
        source_dir.mkdir(parents=True, exist_ok=True)
        tracked_path = source_dir / tracked_filename

        subprocess.run(["git", "init", "-b", "main"], cwd=source_dir, check=True, capture_output=True, text=True)
        subprocess.run(["git", "config", "user.name", "Fixture Bot"], cwd=source_dir, check=True, capture_output=True, text=True)
        subprocess.run(
            ["git", "config", "user.email", "fixture-bot@example.com"],
            cwd=source_dir,
            check=True,
            capture_output=True,
            text=True,
        )
        tracked_path.write_text(base_content, encoding="utf-8")
        subprocess.run(["git", "add", tracked_filename], cwd=source_dir, check=True, capture_output=True, text=True)
        subprocess.run(["git", "commit", "-m", "base fixture"], cwd=source_dir, check=True, capture_output=True, text=True)

        tracked_path.write_text(mutated_content, encoding="utf-8")
        patch_relpath = f"fixtures/cases/patches/{case_id}.patch"
        patch_path = repo_root / patch_relpath
        diff = subprocess.run(
            ["git", "diff", "--binary", "HEAD"],
            cwd=source_dir,
            capture_output=True,
            text=True,
            check=True,
        ).stdout
        patch_path.write_text(diff, encoding="utf-8")
        subprocess.run(["git", "checkout", "--", tracked_filename], cwd=source_dir, check=True, capture_output=True, text=True)

        return {
            "id": case_id,
            "source_fixture_id": fixture_id,
            "source_variant": "post",
            "family": "test_case",
            "patch": patch_relpath,
            "expected_primary_step": "ensure.preprocess_contract",
            "expected_issue_codes": ["test_issue"],
        }


if __name__ == "__main__":
    unittest.main()
