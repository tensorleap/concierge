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

from scripts.qa_workflow_summary import build_workflow_summary_markdown


class QAWorkflowSummaryTest(unittest.TestCase):
    def test_build_workflow_summary_markdown_renders_run_page_summary(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            qa_root = repo_root / "QA"
            run_id = "mnist-preprocess-123"
            self._write_run_artifacts(qa_root, run_id)

            markdown = build_workflow_summary_markdown(
                repo_root=repo_root,
                artifacts_root=qa_root,
                run_id=run_id,
                ref_under_test="feature/qa-summary@abc1234",
                artifact_name=f"qa-loop-{run_id}",
            )

            self.assertIn("## QA Loop", markdown)
            self.assertIn("Ref: `feature/qa-summary@abc1234`", markdown)
            self.assertIn("Fixture: `mnist`", markdown)
            self.assertIn("Step: `preprocess`", markdown)
            self.assertIn("Loop state: `STOP_FIX`", markdown)
            self.assertIn("Stop reason: `missing_preprocess`", markdown)
            self.assertIn("Concierge repaired `leap.yaml` but stopped at preprocess validation.", markdown)
            self.assertIn("The run reached canonical layout validation and then stalled on preprocess.", markdown)
            self.assertIn("Checked repository layout.", markdown)
            self.assertIn("Repaired the root-level `leap.yaml` entrypoint.", markdown)
            self.assertIn("Validation failed at preprocess after the rerun.", markdown)
            self.assertIn("No canonical step trajectory was exported for this run.", markdown)
            self.assertIn("Final Observed Step", markdown)
            self.assertIn("workspace/leap.yaml", markdown)
            self.assertIn("workspace/leap_integration.py", markdown)

    def test_cli_prints_workflow_ready_markdown(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            qa_root = repo_root / "QA"
            run_id = "mnist-preprocess-123"
            self._write_run_artifacts(qa_root, run_id)

            completed = subprocess.run(
                [
                    sys.executable,
                    str(REPO_ROOT / "scripts" / "qa_workflow_summary.py"),
                    "--repo-root",
                    str(repo_root),
                    "--artifacts-root",
                    str(qa_root),
                    "--run-id",
                    run_id,
                    "--ref",
                    "feature/qa-summary@abc1234",
                    "--artifact-name",
                    f"qa-loop-{run_id}",
                ],
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertIn("## QA Loop", completed.stdout)
            self.assertIn("### Timeline", completed.stdout)
            self.assertIn("### Exported Evidence", completed.stdout)

    def test_build_workflow_summary_markdown_renders_ultralytics_pre_rerun_trajectory(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            qa_root = repo_root / "QA"
            run_id = "ultralytics-pre-trajectory-123"
            self._write_run_artifacts(
                qa_root,
                run_id,
                qa_context={
                    "fixture_id": "ultralytics",
                    "guide_step": "pre",
                    "ref_under_test": "feature/issue-145@abc1234",
                    "checkpoint_key": "ultralytics:pre",
                    "source_kind": "variant",
                    "source_id": "pre",
                },
                observed_steps=[
                    "ensure.python_runtime",
                    "ensure.leap_yaml",
                    "ensure.integration_test_contract",
                    "ensure.preprocess_contract",
                    "ensure.input_encoders",
                    "ensure.ground_truth_encoders",
                    "ensure.model_acquisition",
                ],
                turns=[
                    {
                        "iteration": 1,
                        "directive": {
                            "summary": "Accepted the leap.yaml scaffold review.",
                            "action": "SEND_INPUT",
                            "loop_state": "CONTINUE",
                        },
                    },
                    {
                        "iteration": 2,
                        "directive": {
                            "summary": "Reran `concierge run` after the review-only stop.",
                            "action": "RUN_COMMAND",
                            "loop_state": "CONTINUE",
                        },
                    },
                    {
                        "iteration": 3,
                        "directive": {
                            "summary": "Validation advanced to ground-truth encoders after the rerun.",
                            "action": "WAIT",
                            "loop_state": "STOP_REPORT",
                        },
                    },
                ],
                report_overrides={
                    "overall_outcome": "The rerun advanced through downstream ultralytics wiring without snapping back to early setup steps.",
                    "integration_progress": "The rerun progressed from the integration scaffold into encoder and model work.",
                },
            )

            markdown = build_workflow_summary_markdown(
                repo_root=repo_root,
                artifacts_root=qa_root,
                run_id=run_id,
                ref_under_test="feature/issue-145@abc1234",
                artifact_name=f"qa-loop-{run_id}",
            )

            self.assertIn("### Observed Step Trajectory", markdown)
            self.assertIn(
                "ensure.python_runtime -> ensure.leap_yaml -> ensure.integration_test_contract -> ensure.preprocess_contract -> ensure.input_encoders -> ensure.ground_truth_encoders -> ensure.model_acquisition",
                markdown,
            )
            self.assertIn("Reran `concierge run` after the review-only stop.", markdown)
            self.assertIn("No early-step fallback detected after downstream progress.", markdown)

    def test_build_workflow_summary_markdown_flags_ultralytics_pre_fallback_after_rerun(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            qa_root = repo_root / "QA"
            run_id = "ultralytics-pre-fallback-123"
            self._write_run_artifacts(
                qa_root,
                run_id,
                qa_context={
                    "fixture_id": "ultralytics",
                    "guide_step": "pre",
                    "ref_under_test": "feature/issue-145@def5678",
                    "checkpoint_key": "ultralytics:pre",
                    "source_kind": "variant",
                    "source_id": "pre",
                },
                observed_steps=[
                    "ensure.python_runtime",
                    "ensure.leap_yaml",
                    "ensure.integration_test_contract",
                    "ensure.preprocess_contract",
                    "ensure.input_encoders",
                    "ensure.ground_truth_encoders",
                    "ensure.preprocess_contract",
                ],
                turns=[
                    {
                        "iteration": 1,
                        "directive": {
                            "summary": "Accepted the leap.yaml scaffold review.",
                            "action": "SEND_INPUT",
                            "loop_state": "CONTINUE",
                        },
                    },
                    {
                        "iteration": 2,
                        "directive": {
                            "summary": "Reran `concierge run` after the review-only stop.",
                            "action": "RUN_COMMAND",
                            "loop_state": "CONTINUE",
                        },
                    },
                    {
                        "iteration": 3,
                        "directive": {
                            "summary": "Validation snapped back to dataset preprocessing after downstream encoder work.",
                            "action": "WAIT",
                            "loop_state": "STOP_FIX",
                        },
                    },
                ],
                report_overrides={
                    "overall_outcome": "The rerun regressed back to an early preprocessing blocker after downstream progress.",
                    "integration_progress": "The flow reached downstream encoder work and then resurfaced preprocess again.",
                    "product_issues": ["The rerun snapped back to `ensure.preprocess_contract` after downstream progress."],
                },
            )

            markdown = build_workflow_summary_markdown(
                repo_root=repo_root,
                artifacts_root=qa_root,
                run_id=run_id,
                ref_under_test="feature/issue-145@def5678",
                artifact_name=f"qa-loop-{run_id}",
            )

            self.assertIn(
                "Early-step fallback detected: ensure.preprocess_contract after downstream progress at ensure.ground_truth_encoders.",
                markdown,
            )
            self.assertIn(
                "ensure.python_runtime -> ensure.leap_yaml -> ensure.integration_test_contract -> ensure.preprocess_contract -> ensure.input_encoders -> ensure.ground_truth_encoders -> ensure.preprocess_contract",
                markdown,
            )

    def _write_run_artifacts(
        self,
        qa_root: Path,
        run_id: str,
        *,
        qa_context: dict[str, str] | None = None,
        observed_steps: list[str] | None = None,
        turns: list[dict[str, object]] | None = None,
        report_overrides: dict[str, object] | None = None,
    ) -> None:
        run_dir = qa_root / "runs" / run_id
        report_dir = qa_root / "reports"
        transcript_dir = qa_root / "transcripts"
        export_root = run_dir / "docker" / "export" / "workspace"
        report_dir.mkdir(parents=True, exist_ok=True)
        transcript_dir.mkdir(parents=True, exist_ok=True)
        export_root.mkdir(parents=True, exist_ok=True)

        summary = {
            "run_id": run_id,
            "loop_state": "STOP_FIX",
            "stop_reason": "missing_preprocess",
            "qa_context": qa_context
            or {
                "fixture_id": "mnist",
                "guide_step": "preprocess",
                "ref_under_test": "feature/issue-90@abc1234",
                "checkpoint_key": "mnist:preprocess",
                "source_kind": "case",
                "source_id": "mnist_missing_preprocess",
            },
            "paths": {
                "summary_json": str((run_dir / "summary.json").resolve()),
                "report_json": str((report_dir / f"{run_id}.json").resolve()),
                "report_markdown": str((report_dir / f"{run_id}.md").resolve()),
                "full_transcript": str((transcript_dir / f"{run_id}.full.txt").resolve()),
                "turns_jsonl": str((run_dir / "turns.jsonl").resolve()),
            },
        }
        (run_dir / "summary.json").parent.mkdir(parents=True, exist_ok=True)
        (run_dir / "summary.json").write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")

        report = {
            "title": "Synthetic QA summary",
            "overall_outcome": "Concierge repaired `leap.yaml` but stopped at preprocess validation.",
            "loop_state": "STOP_FIX",
            "integration_progress": "The run reached canonical layout validation and then stalled on preprocess.",
            "ux_clarity": ["The rerun instruction was clear."],
            "product_issues": ["The generated integration still lacked `@tensorleap_preprocess`."],
            "agent_interaction_issues": ["The QA agent had to infer the rerun boundary from the terminal transcript."],
            "suggestions": ["Summarize the repaired step and blocked step directly in the run page."],
            "notable_moments": ["Concierge reran after accepting the repaired `leap.yaml` diff."],
        }
        if report_overrides:
            report.update(report_overrides)
        (report_dir / f"{run_id}.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
        (report_dir / f"{run_id}.md").write_text("# Synthetic QA summary\n", encoding="utf-8")

        turns = turns or [
            {
                "iteration": 1,
                "directive": {
                    "summary": "Checked repository layout.",
                    "action": "WAIT",
                    "loop_state": "CONTINUE",
                },
            },
            {
                "iteration": 2,
                "directive": {
                    "summary": "Repaired the root-level `leap.yaml` entrypoint.",
                    "action": "SEND_INPUT",
                    "loop_state": "CONTINUE",
                },
            },
            {
                "iteration": 3,
                "directive": {
                    "summary": "Validation failed at preprocess after the rerun.",
                    "action": "WAIT",
                    "loop_state": "STOP_FIX",
                },
            },
        ]
        turns_path = run_dir / "turns.jsonl"
        turns_path.parent.mkdir(parents=True, exist_ok=True)
        turns_path.write_text(
            "".join(json.dumps(item) + "\n" for item in turns),
            encoding="utf-8",
        )

        (transcript_dir / f"{run_id}.full.txt").write_text("Synthetic transcript\n", encoding="utf-8")
        (export_root / "leap.yaml").write_text("entryFile: leap_integration.py\n", encoding="utf-8")
        (export_root / "leap_integration.py").write_text("print('integration')\n", encoding="utf-8")
        if observed_steps:
            events_path = export_root / ".concierge" / "evidence" / "snapshot-123" / "events.jsonl"
            events_path.parent.mkdir(parents=True, exist_ok=True)
            events_path.write_text(
                "".join(
                    json.dumps(
                        {
                            "kind": "step_selected",
                            "iteration": index,
                            "snapshotId": "snapshot-123",
                            "stepId": step_id,
                            "message": f"Working on: {step_id}",
                        }
                    )
                    + "\n"
                    for index, step_id in enumerate(observed_steps, start=1)
                ),
                encoding="utf-8",
            )


if __name__ == "__main__":
    unittest.main()
