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


if __name__ == "__main__":
    unittest.main()
