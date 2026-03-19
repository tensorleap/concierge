from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
if str(REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(REPO_ROOT))

from scripts.qa_checkpoint_resolver import compute_image_key, resolve_checkpoint


class QACheckpointResolverTest(unittest.TestCase):
    def test_list_fixtures_cli_preserves_manifest_order(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            fixtures_dir = repo_root / "fixtures"
            fixtures_dir.mkdir(parents=True, exist_ok=True)
            (fixtures_dir / "manifest.json").write_text(
                json.dumps(
                    {
                        "fixtures": [
                            {"id": "ultralytics"},
                            {"id": "mnist"},
                            {"id": "webinar"},
                        ]
                    },
                    indent=2,
                )
                + "\n",
                encoding="utf-8",
            )
            (fixtures_dir / "checkpoints").mkdir(parents=True, exist_ok=True)
            (fixtures_dir / "checkpoints" / "manifest.json").write_text('{"checkpoints":[]}\n', encoding="utf-8")

            completed = subprocess.run(
                [
                    sys.executable,
                    str(REPO_ROOT / "scripts" / "qa_checkpoint_resolver.py"),
                    "list-fixtures",
                    "--repo-root",
                    str(repo_root),
                ],
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertEqual(json.loads(completed.stdout), ["ultralytics", "mnist", "webinar"])

    def test_list_steps_cli_returns_guide_native_order(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            self._write_fixture_manifest(repo_root)
            self._write_checkpoint_manifest(
                repo_root,
                [
                    {
                        "fixture_id": "mnist",
                        "step": "input_encoders",
                        "source_kind": "case",
                        "source_id": "mnist_minimum_inputs",
                        "build_mode": "cold",
                        "expected_primary_step": "ensure.input_encoders",
                    }
                ],
            )

            completed = subprocess.run(
                [
                    sys.executable,
                    str(REPO_ROOT / "scripts" / "qa_checkpoint_resolver.py"),
                    "list-steps",
                    "--repo-root",
                    str(repo_root),
                    "--fixture-id",
                    "mnist",
                ],
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertEqual(
                json.loads(completed.stdout),
                [
                    "integration_script",
                    "preprocess",
                    "input_encoders",
                    "model_acquisition",
                    "integration_test",
                    "ground_truth_encoders",
                ],
            )

    def test_select_runner_cli_rejects_missing_selectors_without_interactive_mode(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            fixtures_dir = repo_root / "fixtures"
            fixtures_dir.mkdir(parents=True, exist_ok=True)
            (fixtures_dir / "manifest.json").write_text(
                json.dumps({"fixtures": [{"id": "ultralytics"}, {"id": "mnist"}]}, indent=2) + "\n",
                encoding="utf-8",
            )
            (fixtures_dir / "checkpoints").mkdir(parents=True, exist_ok=True)
            (fixtures_dir / "checkpoints" / "manifest.json").write_text('{"checkpoints":[]}\n', encoding="utf-8")

            completed = subprocess.run(
                [
                    sys.executable,
                    str(REPO_ROOT / "scripts" / "qa_checkpoint_resolver.py"),
                    "select-runner",
                    "--repo-root",
                    str(repo_root),
                ],
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(completed.returncode, 1)
            self.assertIn("missing required QA selectors for non-interactive run", completed.stderr)
            self.assertIn("Valid fixtures: ultralytics, mnist", completed.stderr)
            self.assertIn(
                "Valid steps: integration_script, preprocess, input_encoders, model_acquisition, integration_test, ground_truth_encoders",
                completed.stderr,
            )

    def test_select_runner_cli_prompts_for_missing_fixture_and_step(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            fixtures_dir = repo_root / "fixtures"
            fixtures_dir.mkdir(parents=True, exist_ok=True)
            (fixtures_dir / "manifest.json").write_text(
                json.dumps({"fixtures": [{"id": "ultralytics"}, {"id": "mnist"}]}, indent=2) + "\n",
                encoding="utf-8",
            )
            (fixtures_dir / "checkpoints").mkdir(parents=True, exist_ok=True)
            (fixtures_dir / "checkpoints" / "manifest.json").write_text('{"checkpoints":[]}\n', encoding="utf-8")

            completed = subprocess.run(
                [
                    sys.executable,
                    str(REPO_ROOT / "scripts" / "qa_checkpoint_resolver.py"),
                    "select-runner",
                    "--repo-root",
                    str(repo_root),
                    "--interactive",
                ],
                input="2\n3\n",
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertIn("Choose a fixture:", completed.stderr)
            self.assertIn("Choose a starting step:", completed.stderr)
            self.assertEqual(
                json.loads(completed.stdout),
                {"fixture_id": "mnist", "step": "input_encoders"},
            )

    def test_select_runner_cli_prompts_only_for_step_when_fixture_is_preselected(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            fixtures_dir = repo_root / "fixtures"
            fixtures_dir.mkdir(parents=True, exist_ok=True)
            (fixtures_dir / "manifest.json").write_text(
                json.dumps({"fixtures": [{"id": "ultralytics"}, {"id": "mnist"}]}, indent=2) + "\n",
                encoding="utf-8",
            )
            (fixtures_dir / "checkpoints").mkdir(parents=True, exist_ok=True)
            (fixtures_dir / "checkpoints" / "manifest.json").write_text('{"checkpoints":[]}\n', encoding="utf-8")

            completed = subprocess.run(
                [
                    sys.executable,
                    str(REPO_ROOT / "scripts" / "qa_checkpoint_resolver.py"),
                    "select-runner",
                    "--repo-root",
                    str(repo_root),
                    "--fixture-id",
                    "mnist",
                    "--interactive",
                ],
                input="4\n",
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertNotIn("Choose a fixture:", completed.stderr)
            self.assertIn("Choose a starting step:", completed.stderr)
            self.assertEqual(
                json.loads(completed.stdout),
                {"fixture_id": "mnist", "step": "model_acquisition"},
            )

    def test_resolve_checkpoint_uses_matching_case_entry(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            self._write_fixture_manifest(repo_root)
            self._write_checkpoint_manifest(
                repo_root,
                [
                    {
                        "fixture_id": "mnist",
                        "step": "input_encoders",
                        "source_kind": "case",
                        "source_id": "mnist_minimum_inputs",
                        "build_mode": "cold",
                        "expected_primary_step": "ensure.input_encoders",
                    }
                ],
            )

            resolution = resolve_checkpoint(repo_root, fixture_id="mnist", step="input_encoders")

            self.assertEqual(resolution["fixture_id"], "mnist")
            self.assertEqual(resolution["step"], "input_encoders")
            self.assertEqual(resolution["source_kind"], "case")
            self.assertEqual(resolution["source_id"], "mnist_minimum_inputs")
            self.assertEqual(resolution["build_mode"], "cold")
            self.assertEqual(resolution["expected_primary_step"], "ensure.input_encoders")
            self.assertFalse(resolution["fallback"])
            self.assertEqual(
                resolution["repo_path"],
                str((repo_root / ".fixtures" / "cases" / "mnist_minimum_inputs").resolve()),
            )

    def test_resolve_checkpoint_falls_back_to_pre_variant(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            self._write_fixture_manifest(repo_root)
            self._write_checkpoint_manifest(repo_root, [])

            resolution = resolve_checkpoint(repo_root, fixture_id="mnist", step="preprocess")

            self.assertEqual(resolution["source_kind"], "variant")
            self.assertEqual(resolution["source_id"], "pre")
            self.assertEqual(resolution["build_mode"], "cold")
            self.assertTrue(resolution["fallback"])
            self.assertIsNone(resolution["expected_primary_step"])
            self.assertEqual(
                resolution["repo_path"],
                str((repo_root / ".fixtures" / "mnist" / "pre").resolve()),
            )

    def test_resolve_checkpoint_allows_explicit_build_mode_override(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            self._write_fixture_manifest(repo_root)
            self._write_checkpoint_manifest(
                repo_root,
                [
                    {
                        "fixture_id": "mnist",
                        "step": "input_encoders",
                        "source_kind": "case",
                        "source_id": "mnist_minimum_inputs",
                        "build_mode": "prewarmed",
                        "expected_primary_step": "ensure.input_encoders",
                    }
                ],
            )

            resolution = resolve_checkpoint(
                repo_root,
                fixture_id="mnist",
                step="input_encoders",
                build_mode_override="cold",
            )

            self.assertEqual(resolution["build_mode"], "cold")

    def test_compute_image_key_changes_when_requested_step_changes(self) -> None:
        base_payload = {
            "fixture_id": "mnist",
            "checkpoint_key": "mnist:input_encoders",
            "requested_step": "input_encoders",
            "source_kind": "case",
            "source_id": "mnist_minimum_inputs",
            "fixture_ref": "abc123",
            "python_version": "3.11.11",
            "poetry_version": "2.2.1",
            "claude_version": "2.1.76",
            "concierge_sha": "deadbeef",
            "build_mode": "cold",
            "dockerfile_sha": "docker",
            "runner_sha": "runner",
            "sanitizer_sha": "sanitizer-a",
            "resolver_sha": "resolver",
        }

        first = compute_image_key(**base_payload)
        second = compute_image_key(**{**base_payload, "checkpoint_key": "mnist:gt_encoders", "requested_step": "gt_encoders"})

        self.assertNotEqual(first, second)

    def test_compute_image_key_changes_when_sanitizer_changes(self) -> None:
        base_payload = {
            "fixture_id": "mnist",
            "checkpoint_key": "mnist:input_encoders",
            "requested_step": "input_encoders",
            "source_kind": "case",
            "source_id": "mnist_minimum_inputs",
            "fixture_ref": "abc123",
            "python_version": "3.11.11",
            "poetry_version": "2.2.1",
            "claude_version": "2.1.76",
            "concierge_sha": "deadbeef",
            "build_mode": "cold",
            "dockerfile_sha": "docker",
            "runner_sha": "runner",
            "sanitizer_sha": "sanitizer-a",
            "resolver_sha": "resolver",
        }

        first = compute_image_key(**base_payload)
        second = compute_image_key(**{**base_payload, "sanitizer_sha": "sanitizer-b"})

        self.assertNotEqual(first, second)

    def test_repo_checkpoint_manifest_has_unique_fixture_step_keys(self) -> None:
        manifest_path = REPO_ROOT / "fixtures" / "checkpoints" / "manifest.json"
        payload = json.loads(manifest_path.read_text(encoding="utf-8"))
        seen: set[tuple[str, str]] = set()
        for entry in payload["checkpoints"]:
            key = (entry["fixture_id"], entry["step"])
            self.assertNotIn(key, seen)
            seen.add(key)

    def _write_fixture_manifest(self, repo_root: Path) -> None:
        fixtures_dir = repo_root / "fixtures"
        fixtures_dir.mkdir(parents=True, exist_ok=True)
        (fixtures_dir / "manifest.json").write_text(
            json.dumps({"fixtures": [{"id": "mnist"}]}, indent=2) + "\n",
            encoding="utf-8",
        )

    def _write_checkpoint_manifest(self, repo_root: Path, checkpoints: list[dict[str, object]]) -> None:
        checkpoints_dir = repo_root / "fixtures" / "checkpoints"
        checkpoints_dir.mkdir(parents=True, exist_ok=True)
        (checkpoints_dir / "manifest.json").write_text(
            json.dumps({"checkpoints": checkpoints}, indent=2) + "\n",
            encoding="utf-8",
        )


if __name__ == "__main__":
    unittest.main()
