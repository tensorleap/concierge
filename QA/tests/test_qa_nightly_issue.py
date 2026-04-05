from __future__ import annotations

import http.server
import json
import subprocess
import sys
import tempfile
import threading
import unittest
from pathlib import Path

from scripts.qa_nightly_issue import (
    NightlySyncResult,
    build_recovery_comment,
    build_regression_issue_body,
    build_slack_payload,
    classify_summary,
    determine_notification_action,
)


REPO_ROOT = Path(__file__).resolve().parents[2]


class QANightlyIssueTest(unittest.TestCase):
    def test_determine_notification_action_distinguishes_transition_states(self) -> None:
        self.assertEqual(
            determine_notification_action(classification_kind="product_regression", has_open_issue=False),
            "regression_opened",
        )
        self.assertEqual(
            determine_notification_action(classification_kind="product_regression", has_open_issue=True),
            "regression_still_open",
        )
        self.assertEqual(
            determine_notification_action(classification_kind="pass", has_open_issue=True),
            "recovered",
        )
        self.assertEqual(
            determine_notification_action(classification_kind="pass", has_open_issue=False),
            "no_open_issue",
        )
        self.assertEqual(
            determine_notification_action(classification_kind="infra_failure", has_open_issue=True),
            "infra_failure",
        )

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

    def test_build_slack_payload_for_regression_transition_includes_run_and_issue_links(self) -> None:
        payload = build_slack_payload(
            NightlySyncResult(
                classification_kind="product_regression",
                loop_state="STOP_FIX",
                stop_reason="integration_review_failed",
                report_status="ready",
                notification_action="regression_opened",
                issue_number=184,
                issue_title="Nightly QA regression: ultralytics/pre",
                issue_url="https://github.com/tensorleap/concierge/issues/184",
                run_id="nightly-ultralytics-pre-20260403-23950721920",
                workflow_run_url="https://github.com/tensorleap/concierge/actions/runs/23950721920",
                artifact_name="qa-loop-nightly-ultralytics-pre-20260403-23950721920",
                summary_text="Nightly ultralytics/pre QA stopped on STOP_FIX (integration_review_failed).",
            )
        )

        self.assertIsNotNone(payload)
        assert payload is not None
        self.assertIn("Nightly ultralytics/pre QA regression detected.", payload["text"])
        self.assertIn("<https://github.com/tensorleap/concierge/actions/runs/23950721920|nightly-ultralytics-pre-20260403-23950721920>", payload["text"])
        self.assertIn("<https://github.com/tensorleap/concierge/issues/184|Nightly QA regression: ultralytics/pre>", payload["text"])
        self.assertIn("STOP_FIX", payload["text"])

    def test_build_slack_payload_returns_none_for_non_transition_actions(self) -> None:
        payload = build_slack_payload(
            NightlySyncResult(
                classification_kind="product_regression",
                loop_state="STOP_FIX",
                stop_reason="integration_review_failed",
                report_status="ready",
                notification_action="regression_still_open",
                issue_number=184,
                issue_title="Nightly QA regression: ultralytics/pre",
                issue_url="https://github.com/tensorleap/concierge/issues/184",
                run_id="nightly-ultralytics-pre-20260403-23950721920",
                workflow_run_url="https://github.com/tensorleap/concierge/actions/runs/23950721920",
                artifact_name="qa-loop-nightly-ultralytics-pre-20260403-23950721920",
                summary_text="Nightly ultralytics/pre QA stopped on STOP_FIX (integration_review_failed).",
            )
        )

        self.assertIsNone(payload)

    def test_build_slack_payload_cli_accepts_result_json_without_sync_only_args(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            result_json = Path(tmpdir) / "nightly-sync-result.json"
            result_json.write_text(
                json.dumps(self._slack_result().to_dict(), indent=2) + "\n",
                encoding="utf-8",
            )

            completed = subprocess.run(
                [
                    sys.executable,
                    str(REPO_ROOT / "scripts" / "qa_nightly_issue.py"),
                    "build-slack-payload",
                    "--result-json",
                    str(result_json),
                ],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            payload = json.loads(completed.stdout)
            self.assertIn("Nightly ultralytics/pre QA regression detected.", payload["text"])
            self.assertIn(
                "<https://github.com/tensorleap/concierge/issues/190|Nightly QA regression: ultralytics/pre>",
                payload["text"],
            )

    def test_send_slack_cli_posts_payload_to_local_test_webhook(self) -> None:
        server = _SlackCaptureServer()
        server.start()
        self.addCleanup(server.stop)

        with tempfile.TemporaryDirectory() as tmpdir:
            result_json = Path(tmpdir) / "nightly-sync-result.json"
            result_json.write_text(
                json.dumps(self._slack_result().to_dict(), indent=2) + "\n",
                encoding="utf-8",
            )

            completed = subprocess.run(
                [
                    sys.executable,
                    str(REPO_ROOT / "scripts" / "qa_nightly_issue.py"),
                    "send-slack",
                    "--result-json",
                    str(result_json),
                    "--webhook-url",
                    server.url,
                    "--allow-insecure-webhook-url",
                ],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            request = server.wait_for_request()
            self.assertEqual(request["path"], "/")
            self.assertEqual(request["headers"]["Content-Type"], "application/json")
            payload = json.loads(request["body"])
            self.assertIn("Nightly ultralytics/pre QA regression detected.", payload["text"])
            self.assertIn("nightly-ultralytics-pre-20260405-23994060462", payload["text"])

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

    def _slack_result(self) -> NightlySyncResult:
        return NightlySyncResult(
            classification_kind="product_regression",
            loop_state="STOP_FIX",
            stop_reason="integration_review_failed",
            report_status="ready",
            notification_action="regression_opened",
            issue_number=190,
            issue_title="Nightly QA regression: ultralytics/pre",
            issue_url="https://github.com/tensorleap/concierge/issues/190",
            run_id="nightly-ultralytics-pre-20260405-23994060462",
            workflow_run_url="https://github.com/tensorleap/concierge/actions/runs/23994060462",
            artifact_name="qa-loop-nightly-ultralytics-pre-20260405-23994060462",
            summary_text="Nightly ultralytics/pre QA stopped on STOP_FIX (integration_review_failed).",
        )


class _SlackCaptureServer:
    def __init__(self) -> None:
        self._requests: list[dict[str, object]] = []
        self._event = threading.Event()
        handler = self._build_handler()
        self._server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
        self._thread = threading.Thread(target=self._server.serve_forever, daemon=True)

    @property
    def url(self) -> str:
        host, port = self._server.server_address
        return f"http://{host}:{port}/"

    def start(self) -> None:
        self._thread.start()

    def stop(self) -> None:
        self._server.shutdown()
        self._server.server_close()
        self._thread.join(timeout=5)

    def wait_for_request(self) -> dict[str, object]:
        if not self._event.wait(timeout=5):
            raise AssertionError("timed out waiting for Slack webhook request")
        return self._requests[-1]

    def _build_handler(self) -> type[http.server.BaseHTTPRequestHandler]:
        outer = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_POST(self) -> None:  # noqa: N802
                content_length = int(self.headers.get("Content-Length", "0"))
                body = self.rfile.read(content_length).decode("utf-8")
                outer._requests.append(
                    {
                        "path": self.path,
                        "headers": dict(self.headers.items()),
                        "body": body,
                    }
                )
                outer._event.set()
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b"ok")

            def log_message(self, format: str, *args: object) -> None:
                return

        return Handler
