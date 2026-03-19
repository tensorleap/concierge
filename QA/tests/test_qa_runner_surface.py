from __future__ import annotations

import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]


class QARunnerSurfaceTest(unittest.TestCase):
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
        self.assertIn("onnx_exporter()", warmup)
        self.assertNotIn("onnxruntime==1.21.1", warmup)
        self.assertNotIn("yolo11n.onnx", warmup)


if __name__ == "__main__":
    unittest.main()
