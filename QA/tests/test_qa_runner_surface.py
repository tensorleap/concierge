from __future__ import annotations

import json
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]


class QARunnerSurfaceTest(unittest.TestCase):
    def test_qa_workflow_supports_manual_dispatch_inputs(self) -> None:
        workflow = (REPO_ROOT / ".github" / "workflows" / "qa-loop.yml").read_text(encoding="utf-8")

        self.assertIn("workflow_dispatch:", workflow)
        self.assertIn("ref:", workflow)
        self.assertIn("fixture:", workflow)
        self.assertIn("step:", workflow)
        self.assertIn("run_id:", workflow)
        self.assertIn("issue_number:", workflow)
        self.assertIn("pr_number:", workflow)

    def test_qa_workflow_uses_choice_inputs_for_fixture_and_step(self) -> None:
        workflow = (REPO_ROOT / ".github" / "workflows" / "qa-loop.yml").read_text(encoding="utf-8")
        fixture_ids = [
            str(entry["id"]).strip()
            for entry in json.loads((REPO_ROOT / "fixtures" / "manifest.json").read_text(encoding="utf-8"))["fixtures"]
        ]

        self.assertIn("fixture:", workflow)
        self.assertIn("type: choice", workflow)
        self.assertIn("options:", workflow)
        for fixture_id in fixture_ids:
            self.assertIn(f"          - {fixture_id}", workflow)

        for step in (
            "pre",
            "integration_script",
            "preprocess",
            "input_encoders",
            "model_acquisition",
            "integration_test",
            "ground_truth_encoders",
        ):
            self.assertIn(f"          - {step}", workflow)

    def test_qa_workflow_uploads_artifacts_and_supports_linked_comments(self) -> None:
        workflow = (REPO_ROOT / ".github" / "workflows" / "qa-loop.yml").read_text(encoding="utf-8")

        self.assertIn("actions/upload-artifact@v4", workflow)
        self.assertIn("GITHUB_STEP_SUMMARY", workflow)
        self.assertIn("Artifact:", workflow)
        self.assertIn("issues: write", workflow)
        self.assertIn("pull-requests: write", workflow)
        self.assertIn("gh issue comment", workflow)
        self.assertIn("gh pr comment", workflow)

    def test_qa_workflow_uses_explicit_targeted_setup_steps(self) -> None:
        workflow = (REPO_ROOT / ".github" / "workflows" / "qa-loop.yml").read_text(encoding="utf-8")

        self.assertIn("- name: Prepare selected fixture", workflow)
        self.assertIn('bash scripts/fixtures_prepare.sh --fixture "${QA_FIXTURE}"', workflow)
        self.assertIn("- name: Resolve selected QA source", workflow)
        self.assertIn("qa_checkpoint_resolver.py", workflow)
        self.assertIn("resolve", workflow)
        self.assertIn("QA_PREPARE_CASE_ID", workflow)
        self.assertIn("- name: Generate selected case repo", workflow)
        self.assertIn('bash scripts/fixtures_mutate_cases.sh --case "${QA_PREPARE_CASE_ID}"', workflow)
        self.assertIn("--require-explicit-setup", workflow)

    def test_qa_workflow_generates_case_repos_only_when_resolution_requires_it(self) -> None:
        workflow = (REPO_ROOT / ".github" / "workflows" / "qa-loop.yml").read_text(encoding="utf-8")

        self.assertIn("if: ${{ env.QA_PREPARE_CASE_ID != '' }}", workflow)

    def test_makefile_does_not_force_default_repo(self) -> None:
        makefile = (REPO_ROOT / "Makefile").read_text(encoding="utf-8")

        self.assertIn("REPO ?=", makefile)
        self.assertNotIn("REPO ?= ultralytics", makefile)

    def test_makefile_exposes_step_without_forcing_default_image_override(self) -> None:
        makefile = (REPO_ROOT / "Makefile").read_text(encoding="utf-8")

        self.assertIn("QA_STEP ?=", makefile)
        self.assertNotIn("QA_IMAGE_MODE ?= cold", makefile)

    def test_runner_script_uses_checkpoint_resolver_and_step_selection(self) -> None:
        script = (REPO_ROOT / "scripts" / "qa_fixture_run.sh").read_text(encoding="utf-8")

        self.assertIn("--step", script)
        self.assertIn("qa_checkpoint_resolver.py", script)
        self.assertIn("selected_repo_dir", script)
        self.assertIn("selected_build_mode", script)
        self.assertNotIn('cp -a "${pre_dir}/." "${context_dir}/workspace/"', script)
        self.assertIn("qa_sanitize_workspace.sh", script)
        self.assertNotIn('cp -a "${selected_repo_dir}/." "${context_dir}/workspace/"', script)

    def test_runner_script_tracks_selected_warmup_script(self) -> None:
        script = (REPO_ROOT / "scripts" / "qa_fixture_run.sh").read_text(encoding="utf-8")

        self.assertIn("selected_warmup_script", script)
        self.assertIn("warmup_sha", script)
        self.assertIn(".checkpoint_warmup.sh", script)

    def test_runner_script_uses_targeted_case_generation_for_case_sources(self) -> None:
        script = (REPO_ROOT / "scripts" / "qa_fixture_run.sh").read_text(encoding="utf-8")

        self.assertIn("selected_prepare_case_id", script)
        self.assertIn('fixtures_mutate_cases.sh" --case "${selected_prepare_case_id}"', script)

    def test_runner_script_supports_strict_explicit_setup_mode(self) -> None:
        script = (REPO_ROOT / "scripts" / "qa_fixture_run.sh").read_text(encoding="utf-8")

        self.assertIn("--require-explicit-setup", script)
        self.assertIn("require_explicit_setup", script)

    def test_case_generation_script_does_not_fetch_from_prepared_fixture_repo(self) -> None:
        script = (REPO_ROOT / "scripts" / "fixtures_mutate_cases.sh").read_text(encoding="utf-8")

        self.assertNotIn('git -C "${case_dir}" fetch --quiet origin "${source_ref}"', script)
        self.assertIn('cp -R "${source_dir}" "${case_dir}"', script)

    def test_dockerfile_runs_checkpoint_warmup_for_prewarmed_images(self) -> None:
        dockerfile = (REPO_ROOT / "QA" / "docker" / "fixture.Dockerfile").read_text(encoding="utf-8")

        self.assertIn(".checkpoint_warmup.sh", dockerfile)
        self.assertIn("fixture-prewarmed", dockerfile)

    def test_ultralytics_warmup_validates_root_entrypoints(self) -> None:
        warmup = (
            REPO_ROOT / "fixtures" / "checkpoints" / "warmup" / "ultralytics_input_encoders.sh"
        ).read_text(encoding="utf-8")

        self.assertIn("li.preprocess()", warmup)
        self.assertIn("li.load_model()", warmup)

    def test_ultralytics_warmup_exports_repo_native_onnx_without_runtime_bump(self) -> None:
        warmup = (
            REPO_ROOT / "fixtures" / "checkpoints" / "warmup" / "ultralytics_input_encoders.sh"
        ).read_text(encoding="utf-8")

        self.assertIn("onnxslim==0.1.89", warmup)
        self.assertIn("torch.backends.mkldnn.enabled = False", warmup)
        self.assertIn("onnx_exporter()", warmup)
        self.assertNotIn("onnxruntime==1.21.1", warmup)
        self.assertNotIn("yolo11n.onnx", warmup)


if __name__ == "__main__":
    unittest.main()
