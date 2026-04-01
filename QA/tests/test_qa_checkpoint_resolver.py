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

from scripts.qa_checkpoint_resolver import (
    compute_image_key,
    resolve_checkpoint,
    stage_runtime_prerequisites,
)


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
                    "pre",
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
                "Valid steps: pre, integration_script, preprocess, input_encoders, model_acquisition, integration_test, ground_truth_encoders",
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
                input="2\n4\n",
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
                input="5\n",
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

    def test_resolve_checkpoint_supports_explicit_pre_step_without_checkpoint(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            self._write_fixture_manifest(repo_root)
            self._write_checkpoint_manifest(
                repo_root,
                [
                    {
                        "fixture_id": "mnist",
                        "step": "preprocess",
                        "source_kind": "case",
                        "source_id": "mnist_preprocess_case",
                        "build_mode": "cold",
                        "expected_primary_step": "ensure.preprocess",
                    }
                ],
            )

            resolution = resolve_checkpoint(repo_root, fixture_id="mnist", step="pre")

            self.assertEqual(resolution["step"], "pre")
            self.assertEqual(resolution["source_kind"], "variant")
            self.assertEqual(resolution["source_id"], "pre")
            self.assertTrue(resolution["fallback"])
            self.assertEqual(
                resolution["repo_path"],
                str((repo_root / ".fixtures" / "mnist" / "pre").resolve()),
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
            self.assertTrue(resolution["requires_case_generation"])
            self.assertEqual(resolution["prepare_case_id"], "mnist_minimum_inputs")
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
            self.assertFalse(resolution["requires_case_generation"])
            self.assertIsNone(resolution["prepare_case_id"])
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

    def test_resolve_checkpoint_includes_declared_warmup_script(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            self._write_fixture_manifest(repo_root)
            self._write_checkpoint_manifest(
                repo_root,
                [
                    {
                        "fixture_id": "ultralytics",
                        "step": "input_encoders",
                        "source_kind": "case",
                        "source_id": "ultralytics_input_encoders",
                        "build_mode": "prewarmed",
                        "expected_primary_step": "ensure.input_encoders",
                        "warmup_script": "fixtures/checkpoints/warmup/ultralytics_input_encoders.sh",
                    }
                ],
            )

            resolution = resolve_checkpoint(repo_root, fixture_id="ultralytics", step="input_encoders")

            self.assertEqual(resolution["build_mode"], "prewarmed")
            self.assertEqual(
                resolution["warmup_script"],
                str((repo_root / "fixtures" / "checkpoints" / "warmup" / "ultralytics_input_encoders.sh").resolve()),
            )

    def test_resolve_checkpoint_includes_fixture_runtime_prerequisites(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            self._write_fixture_manifest(
                repo_root,
                fixtures=[
                    {
                        "id": "infineon_ts_v2",
                        "runtime_prerequisites": [
                            {
                                "id": "customer_parquet",
                                "kind": "local_file",
                                "required": True,
                                "mount_path": "/runtime-prerequisites/infineon_ts_v2/customer_parquet/customer.parquet",
                                "description": "Private parquet dataset required by the infineon fixture.",
                                "operator_guidance": "If Concierge asks where the dataset lives, answer with the mounted path.",
                                "local_resolution": {
                                    "env_vars": ["INFINEON_TS_V2_PARQUET"],
                                    "config_keys": ["infineon_ts_v2_parquet"],
                                },
                                "github_actions": {
                                    "fetch_kind": "git_repo_file",
                                    "repo": "https://github.com/example/private-assets.git",
                                    "ref": "deadbeef",
                                    "path": "infineon/customer.parquet",
                                    "auth_env_vars": ["QA_RUNTIME_PREREQ_GITHUB_TOKEN"],
                                },
                                "validation": {
                                    "extension": ".parquet",
                                    "filename_hint": "customer.parquet",
                                },
                            }
                        ],
                    }
                ],
            )
            self._write_checkpoint_manifest(repo_root, [])

            resolution = resolve_checkpoint(repo_root, fixture_id="infineon_ts_v2", step="pre")

            self.assertEqual(len(resolution["runtime_prerequisites"]), 1)
            prerequisite = resolution["runtime_prerequisites"][0]
            self.assertEqual(prerequisite["id"], "customer_parquet")
            self.assertEqual(prerequisite["kind"], "local_file")
            self.assertEqual(
                prerequisite["mount_path"],
                "/runtime-prerequisites/infineon_ts_v2/customer_parquet/customer.parquet",
            )
            self.assertEqual(prerequisite["local_resolution"]["env_vars"], ["INFINEON_TS_V2_PARQUET"])
            self.assertEqual(prerequisite["github_actions"]["fetch_kind"], "git_repo_file")

    def test_stage_runtime_prerequisites_prefers_env_var_and_stages_file(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            fixture_root = repo_root / "fixtures"
            fixture_root.mkdir(parents=True, exist_ok=True)
            dataset_path = repo_root / "trusted" / "customer.parquet"
            dataset_path.parent.mkdir(parents=True, exist_ok=True)
            dataset_path.write_text("fixture parquet bytes\n", encoding="utf-8")
            self._write_fixture_manifest(
                repo_root,
                fixtures=[
                    {
                        "id": "infineon_ts_v2",
                        "runtime_prerequisites": [
                            {
                                "id": "customer_parquet",
                                "kind": "local_file",
                                "required": True,
                                "mount_path": "/runtime-prerequisites/infineon_ts_v2/customer_parquet/customer.parquet",
                                "description": "Private parquet dataset required by the infineon fixture.",
                                "operator_guidance": "Use the mounted path if Concierge asks for the dataset.",
                                "local_resolution": {
                                    "env_vars": ["INFINEON_TS_V2_PARQUET"],
                                    "config_keys": ["infineon_ts_v2_parquet"],
                                },
                                "validation": {
                                    "extension": ".parquet",
                                    "filename_hint": "customer.parquet",
                                },
                            }
                        ],
                    }
                ],
            )
            stage_root = repo_root / "staged-runtime-prereqs"

            payload = stage_runtime_prerequisites(
                repo_root,
                fixture_id="infineon_ts_v2",
                stage_root=stage_root,
                backend="local",
                env={"INFINEON_TS_V2_PARQUET": str(dataset_path)},
            )

            staged_path = stage_root / "infineon_ts_v2" / "customer_parquet" / "customer.parquet"
            self.assertTrue(staged_path.is_file())
            self.assertEqual(staged_path.read_text(encoding="utf-8"), "fixture parquet bytes\n")
            self.assertEqual(payload["docker_mount_target"], "/runtime-prerequisites")
            self.assertEqual(payload["runtime_prerequisites"][0]["resolution_source"], "env:INFINEON_TS_V2_PARQUET")

    def test_stage_runtime_prerequisites_uses_gitignored_local_config_fallback(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            dataset_path = repo_root / "trusted" / "fallback.parquet"
            dataset_path.parent.mkdir(parents=True, exist_ok=True)
            dataset_path.write_text("fallback parquet bytes\n", encoding="utf-8")
            self._write_fixture_manifest(
                repo_root,
                fixtures=[
                    {
                        "id": "infineon_ts_v2",
                        "runtime_prerequisites": [
                            {
                                "id": "customer_parquet",
                                "kind": "local_file",
                                "required": True,
                                "mount_path": "/runtime-prerequisites/infineon_ts_v2/customer_parquet/fallback.parquet",
                                "description": "Private parquet dataset required by the infineon fixture.",
                                "operator_guidance": "Use the mounted path if Concierge asks for the dataset.",
                                "local_resolution": {
                                    "env_vars": ["INFINEON_TS_V2_PARQUET"],
                                    "config_keys": ["infineon_ts_v2_parquet"],
                                },
                                "validation": {
                                    "extension": ".parquet",
                                    "filename_hint": "fallback.parquet",
                                },
                            }
                        ],
                    }
                ],
            )
            (repo_root / "fixtures" / "runtime_prerequisites.local.json").write_text(
                json.dumps({"runtime_prerequisites": {"infineon_ts_v2_parquet": str(dataset_path)}}, indent=2) + "\n",
                encoding="utf-8",
            )

            payload = stage_runtime_prerequisites(
                repo_root,
                fixture_id="infineon_ts_v2",
                stage_root=repo_root / "staged-runtime-prereqs",
                backend="local",
                env={},
            )

            self.assertEqual(payload["runtime_prerequisites"][0]["resolution_source"], "config:infineon_ts_v2_parquet")

    def test_stage_runtime_prerequisites_supports_github_actions_git_file_fetch(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            asset_repo = repo_root / "private-assets"
            asset_repo.mkdir(parents=True, exist_ok=True)
            subprocess.run(["git", "init"], cwd=asset_repo, check=True, capture_output=True, text=True)
            subprocess.run(
                ["git", "config", "user.name", "QA Test"],
                cwd=asset_repo,
                check=True,
                capture_output=True,
                text=True,
            )
            subprocess.run(
                ["git", "config", "user.email", "qa@example.com"],
                cwd=asset_repo,
                check=True,
                capture_output=True,
                text=True,
            )
            private_asset_path = asset_repo / "infineon" / "customer.parquet"
            private_asset_path.parent.mkdir(parents=True, exist_ok=True)
            private_asset_path.write_text("private parquet bytes\n", encoding="utf-8")
            subprocess.run(["git", "add", "."], cwd=asset_repo, check=True, capture_output=True, text=True)
            subprocess.run(
                ["git", "commit", "-m", "Add private asset"],
                cwd=asset_repo,
                check=True,
                capture_output=True,
                text=True,
            )
            asset_ref = subprocess.run(
                ["git", "rev-parse", "HEAD"],
                cwd=asset_repo,
                capture_output=True,
                text=True,
                check=True,
            ).stdout.strip()
            self._write_fixture_manifest(
                repo_root,
                fixtures=[
                    {
                        "id": "infineon_ts_v2",
                        "runtime_prerequisites": [
                            {
                                "id": "customer_parquet",
                                "kind": "local_file",
                                "required": True,
                                "mount_path": "/runtime-prerequisites/infineon_ts_v2/customer_parquet/customer.parquet",
                                "description": "Private parquet dataset required by the infineon fixture.",
                                "operator_guidance": "Use the mounted path if Concierge asks for the dataset.",
                                "github_actions": {
                                    "fetch_kind": "git_repo_file",
                                    "repo": str(asset_repo),
                                    "ref": asset_ref,
                                    "path": "infineon/customer.parquet",
                                    "auth_env_vars": ["QA_RUNTIME_PREREQ_GITHUB_TOKEN"],
                                },
                                "validation": {
                                    "extension": ".parquet",
                                    "filename_hint": "customer.parquet",
                                },
                            }
                        ],
                    }
                ],
            )

            payload = stage_runtime_prerequisites(
                repo_root,
                fixture_id="infineon_ts_v2",
                stage_root=repo_root / "staged-runtime-prereqs",
                backend="github_actions",
                env={},
            )

            staged_path = repo_root / "staged-runtime-prereqs" / "infineon_ts_v2" / "customer_parquet" / "customer.parquet"
            self.assertTrue(staged_path.is_file())
            self.assertEqual(staged_path.read_text(encoding="utf-8"), "private parquet bytes\n")
            self.assertEqual(payload["runtime_prerequisites"][0]["resolution_source"], "github_actions:git_repo_file")

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
            "warmup_sha": "",
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
            "warmup_sha": "",
        }

        first = compute_image_key(**base_payload)
        second = compute_image_key(**{**base_payload, "sanitizer_sha": "sanitizer-b"})

    def test_compute_image_key_changes_when_warmup_script_changes(self) -> None:
        base_payload = {
            "fixture_id": "ultralytics",
            "checkpoint_key": "ultralytics:input_encoders",
            "requested_step": "input_encoders",
            "source_kind": "case",
            "source_id": "ultralytics_input_encoders",
            "fixture_ref": "abc123",
            "python_version": "3.10.16",
            "poetry_version": "2.2.1",
            "claude_version": "2.1.76",
            "concierge_sha": "deadbeef",
            "build_mode": "prewarmed",
            "dockerfile_sha": "docker",
            "runner_sha": "runner",
            "sanitizer_sha": "sanitizer-a",
            "resolver_sha": "resolver",
            "warmup_sha": "warmup-a",
        }

        first = compute_image_key(**base_payload)
        second = compute_image_key(**{**base_payload, "warmup_sha": "warmup-b"})

        self.assertNotEqual(first, second)

    def test_repo_checkpoint_manifest_has_unique_fixture_step_keys(self) -> None:
        manifest_path = REPO_ROOT / "fixtures" / "checkpoints" / "manifest.json"
        payload = json.loads(manifest_path.read_text(encoding="utf-8"))
        seen: set[tuple[str, str]] = set()
        for entry in payload["checkpoints"]:
            key = (entry["fixture_id"], entry["step"])
            self.assertNotIn(key, seen)
            seen.add(key)

    def test_repo_checkpoint_manifest_contains_ultralytics_input_encoders_checkpoint(self) -> None:
        manifest_path = REPO_ROOT / "fixtures" / "checkpoints" / "manifest.json"
        payload = json.loads(manifest_path.read_text(encoding="utf-8"))

        matching = [
            entry
            for entry in payload["checkpoints"]
            if entry["fixture_id"] == "ultralytics" and entry["step"] == "input_encoders"
        ]

        self.assertEqual(len(matching), 1, matching)
        entry = matching[0]
        self.assertEqual(entry["source_kind"], "case")
        self.assertEqual(entry["build_mode"], "prewarmed")
        self.assertEqual(entry["expected_primary_step"], "ensure.input_encoders")
        self.assertEqual(entry["warmup_script"], "fixtures/checkpoints/warmup/ultralytics_input_encoders.sh")

    def test_repo_checkpoint_manifest_contains_ultralytics_gt_encoders_checkpoint(self) -> None:
        manifest_path = REPO_ROOT / "fixtures" / "checkpoints" / "manifest.json"
        payload = json.loads(manifest_path.read_text(encoding="utf-8"))

        matching = [
            entry
            for entry in payload["checkpoints"]
            if entry["fixture_id"] == "ultralytics" and entry["step"] == "ground_truth_encoders"
        ]

        self.assertEqual(len(matching), 1, matching)
        entry = matching[0]
        self.assertEqual(entry["source_kind"], "case")
        self.assertEqual(entry["source_id"], "ultralytics_gt_encoders")
        self.assertEqual(entry["build_mode"], "prewarmed")
        self.assertEqual(entry["expected_primary_step"], "ensure.ground_truth_encoders")
        self.assertEqual(entry["warmup_script"], "fixtures/checkpoints/warmup/ultralytics_input_encoders.sh")

    def _write_fixture_manifest(
        self,
        repo_root: Path,
        *,
        fixtures: list[dict[str, object]] | None = None,
    ) -> None:
        fixtures_dir = repo_root / "fixtures"
        fixtures_dir.mkdir(parents=True, exist_ok=True)
        (fixtures_dir / "manifest.json").write_text(
            json.dumps({"fixtures": fixtures or [{"id": "mnist"}, {"id": "ultralytics"}]}, indent=2) + "\n",
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
