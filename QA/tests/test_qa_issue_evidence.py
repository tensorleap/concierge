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

from scripts.qa_issue_evidence import build_issue_evidence_markdown


class QAIssueEvidenceTest(unittest.TestCase):
    def test_build_issue_evidence_markdown_renders_inline_bundle(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            qa_root = repo_root / "QA"
            run_id = "mnist-preprocess-123"
            self._write_run_artifacts(qa_root, run_id)

            markdown = build_issue_evidence_markdown(repo_root=repo_root, artifacts_root=qa_root, run_id=run_id)

            self.assertIn(f"Run ID: `{run_id}`", markdown)
            self.assertIn("Fixture: `mnist`", markdown)
            self.assertIn("Guide step: `preprocess`", markdown)
            self.assertIn("Ref under test: `feature/issue-90@abc1234`", markdown)
            self.assertIn("Loop state: `STOP_FIX`", markdown)
            self.assertIn("Stop reason: `missing_entrypoint`", markdown)
            self.assertIn("Concierge should complete the `preprocess` QA flow for `mnist`", markdown)
            self.assertIn("Concierge stopped before it could finish the preprocess stage.", markdown)
            self.assertIn("ModuleNotFoundError: No module named 'leap_integration'", markdown)
            self.assertIn("workspace/.concierge: 3 files", markdown)
            self.assertIn(str((qa_root / "runs" / run_id / "summary.json").resolve()), markdown)

    def test_cli_prints_issue_ready_markdown(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            qa_root = repo_root / "QA"
            run_id = "mnist-preprocess-123"
            self._write_run_artifacts(qa_root, run_id)

            completed = subprocess.run(
                [
                    sys.executable,
                    str(REPO_ROOT / "scripts" / "qa_issue_evidence.py"),
                    "--repo-root",
                    str(repo_root),
                    "--artifacts-root",
                    str(qa_root),
                    "--run-id",
                    run_id,
                ],
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertIn("## Inline Evidence Bundle", completed.stdout)
            self.assertIn("### Transcript Excerpt 1", completed.stdout)
            self.assertIn("### Local Artifacts", completed.stdout)

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
            "stop_reason": "missing_entrypoint",
            "qa_context": {
                "fixture_id": "mnist",
                "guide_step": "preprocess",
                "ref_under_test": "feature/issue-90@abc1234",
                "checkpoint_key": "mnist:preprocess",
                "source_kind": "case",
                "source_id": "mnist_missing_entrypoint",
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
            "title": "Synthetic QA issue",
            "overall_outcome": "Concierge stopped before it could finish the preprocess stage.",
            "loop_state": "STOP_FIX",
            "integration_progress": "The run created some `.concierge` artifacts but never generated `leap_integration.py`.",
            "ux_clarity": ["The initial instruction was clear."],
            "product_issues": ["Concierge never wrote the expected root-level entrypoint."],
            "agent_interaction_issues": ["The agent kept circling on Poetry diagnostics."],
            "suggestions": ["Surface the missing entrypoint requirement earlier."],
            "notable_moments": ["The run failed immediately after validating `leap.yaml`."],
        }
        (report_dir / f"{run_id}.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
        (report_dir / f"{run_id}.md").write_text("# Synthetic QA issue\n", encoding="utf-8")

        transcript = "\n".join(
            [
                "[qa-loop] run id: mnist-preprocess-123",
                "Starting Concierge",
                "Checking repository layout",
                "ERROR expected leap_integration.py at repository root",
                "Traceback (most recent call last):",
                "ModuleNotFoundError: No module named 'leap_integration'",
                "[qa-loop] turn 2: STOP_FIX missing entrypoint",
            ]
        )
        (transcript_dir / f"{run_id}.full.txt").write_text(transcript + "\n", encoding="utf-8")

        (export_root / "reports").mkdir(parents=True, exist_ok=True)
        (export_root / "evidence").mkdir(parents=True, exist_ok=True)
        (export_root / "reports" / "snapshot.json").write_text("{}\n", encoding="utf-8")
        (export_root / "reports" / "validation.json").write_text("{}\n", encoding="utf-8")
        (export_root / "evidence" / "note.txt").write_text("artifact\n", encoding="utf-8")
