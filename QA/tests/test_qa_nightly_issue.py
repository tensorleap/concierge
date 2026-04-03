from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from scripts.qa_nightly_issue import build_recovery_comment, build_regression_issue_body, classify_summary


class QANightlyIssueTest(unittest.TestCase):
    def test_classify_summary_treats_stop_report_as_pass(self) -> None:
        classification = classify_summary(
            {
                "loop_state": "STOP_REPORT",
                "stop_reason": "supervisor_stop_report",
                "report_status": "ready",
            }
        )

        self.assertEqual(classification.kind, "pass")
        self.assertEqual(classification.loop_state, "STOP_REPORT")
        self.assertEqual(classification.stop_reason, "supervisor_stop_report")

    def test_classify_summary_treats_stop_fix_as_product_regression(self) -> None:
        classification = classify_summary(
            {
                "loop_state": "STOP_FIX",
                "stop_reason": "integration_review_failed",
                "report_status": "ready",
            }
        )

        self.assertEqual(classification.kind, "product_regression")
        self.assertEqual(classification.loop_state, "STOP_FIX")
        self.assertEqual(classification.stop_reason, "integration_review_failed")

    def test_classify_summary_treats_deadend_or_missing_summary_as_infra_failure(self) -> None:
        deadend = classify_summary(
            {
                "loop_state": "STOP_DEADEND",
                "stop_reason": "supervisor_stop_deadend",
                "report_status": "fallback",
            }
        )
        missing = classify_summary(None)

        self.assertEqual(deadend.kind, "infra_failure")
        self.assertEqual(missing.kind, "infra_failure")
        self.assertEqual(missing.stop_reason, "missing_summary")

    def test_build_regression_issue_body_includes_latest_run_details_and_inline_evidence(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            qa_root = repo_root / "QA"
            run_id = "nightly-ultralytics-pre-20260403"
            self._write_run_artifacts(qa_root, run_id)

            markdown = build_regression_issue_body(
                repo_root=repo_root,
                artifacts_root=qa_root,
                run_id=run_id,
                workflow_run_url="https://github.com/tensorleap/concierge/actions/runs/123456789",
                artifact_name=f"qa-loop-{run_id}",
            )

            self.assertIn("Nightly QA regression for `ultralytics/pre`", markdown)
            self.assertIn("Workflow run: https://github.com/tensorleap/concierge/actions/runs/123456789", markdown)
            self.assertIn(f"Artifact: `qa-loop-{run_id}`", markdown)
            self.assertIn("## Inline Evidence Bundle", markdown)
            self.assertIn(f"Run ID: `{run_id}`", markdown)

    def test_build_recovery_comment_links_back_to_passing_run(self) -> None:
        comment = build_recovery_comment(
            run_id="nightly-ultralytics-pre-20260404",
            workflow_run_url="https://github.com/tensorleap/concierge/actions/runs/123456790",
            artifact_name="qa-loop-nightly-ultralytics-pre-20260404",
        )

        self.assertIn("Nightly ultralytics/pre QA recovered", comment)
        self.assertIn("nightly-ultralytics-pre-20260404", comment)
        self.assertIn("https://github.com/tensorleap/concierge/actions/runs/123456790", comment)

    def _write_run_artifacts(self, qa_root: Path, run_id: str) -> None:
        run_dir = qa_root / "runs" / run_id
        report_dir = qa_root / "reports"
        transcript_dir = qa_root / "transcripts"
        export_root = run_dir / "docker" / "export" / "workspace" / ".concierge"
        export_root.mkdir(parents=True, exist_ok=True)
        report_dir.mkdir(parents=True, exist_ok=True)
        transcript_dir.mkdir(parents=True, exist_ok=True)

        summary = {
            "run_id": run_id,
            "loop_state": "STOP_FIX",
            "stop_reason": "integration_review_failed",
            "qa_context": {
                "fixture_id": "ultralytics",
                "guide_step": "pre",
                "ref_under_test": "main@a295aa11ff65",
                "checkpoint_key": "ultralytics:pre",
                "source_kind": "variant",
                "source_id": "pre",
            },
            "paths": {
                "summary_json": str((run_dir / "summary.json").resolve()),
                "report_json": str((report_dir / f"{run_id}.json").resolve()),
                "report_markdown": str((report_dir / f"{run_id}.md").resolve()),
                "full_transcript": str((transcript_dir / f"{run_id}.full.txt").resolve()),
            },
        }
        (run_dir / "summary.json").parent.mkdir(parents=True, exist_ok=True)
        (run_dir / "summary.json").write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")

        report = {
            "title": "Synthetic nightly regression",
            "overall_outcome": "Concierge reached a terminal failure during the nightly ultralytics/pre QA run.",
            "loop_state": "STOP_FIX",
            "integration_progress": "The nightly run progressed through scaffold checks but failed the final review gate.",
            "ux_clarity": [],
            "product_issues": ["The generated integration did not match the known-good ultralytics fixture."],
            "agent_interaction_issues": [],
            "suggestions": ["Inspect the exported workspace and fixture diff."],
            "notable_moments": ["The run finished with `STOP_FIX` after integration review."],
        }
        (report_dir / f"{run_id}.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
        (report_dir / f"{run_id}.md").write_text("# Synthetic nightly regression\n", encoding="utf-8")

        transcript = "\n".join(
            [
                f"[qa-loop] run id: {run_id}",
                "Starting Concierge",
                "Checking repository layout",
                "Integration review verdict: generated workspace is not functionally equivalent",
                "[qa-loop] turn 5: STOP_FIX integration_review_failed",
            ]
        )
        (transcript_dir / f"{run_id}.full.txt").write_text(transcript + "\n", encoding="utf-8")

        (export_root / "reports").mkdir(parents=True, exist_ok=True)
        (export_root / "reports" / "snapshot.json").write_text("{}\n", encoding="utf-8")
