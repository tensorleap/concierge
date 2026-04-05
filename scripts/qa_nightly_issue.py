#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from urllib import error, parse, request

REPO_ROOT = Path(__file__).resolve().parents[1]
if str(REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(REPO_ROOT))

from scripts.qa_issue_evidence import build_issue_evidence_markdown, load_json, resolve_artifacts_root, summary_path_for


DEFAULT_ISSUE_TITLE = "Nightly QA regression: ultralytics/pre"


@dataclass(frozen=True)
class NightlyClassification:
    kind: str
    loop_state: str
    stop_reason: str
    report_status: str


@dataclass(frozen=True)
class NightlySyncResult:
    classification_kind: str
    loop_state: str
    stop_reason: str
    report_status: str
    notification_action: str
    issue_number: int | None
    issue_title: str | None
    issue_url: str | None
    run_id: str
    workflow_run_url: str
    artifact_name: str
    summary_text: str

    def to_dict(self) -> dict[str, Any]:
        return {
            "classification_kind": self.classification_kind,
            "loop_state": self.loop_state,
            "stop_reason": self.stop_reason,
            "report_status": self.report_status,
            "notification_action": self.notification_action,
            "issue_number": self.issue_number,
            "issue_title": self.issue_title,
            "issue_url": self.issue_url,
            "run_id": self.run_id,
            "workflow_run_url": self.workflow_run_url,
            "artifact_name": self.artifact_name,
            "summary_text": self.summary_text,
        }

    @classmethod
    def from_dict(cls, payload: dict[str, Any]) -> "NightlySyncResult":
        issue_number = payload.get("issue_number")
        if issue_number in ("", None):
            issue_number = None
        return cls(
            classification_kind=str(payload.get("classification_kind", "")).strip(),
            loop_state=str(payload.get("loop_state", "")).strip(),
            stop_reason=str(payload.get("stop_reason", "")).strip(),
            report_status=str(payload.get("report_status", "")).strip(),
            notification_action=str(payload.get("notification_action", "")).strip(),
            issue_number=int(issue_number) if issue_number is not None else None,
            issue_title=str(payload.get("issue_title", "")).strip() or None,
            issue_url=str(payload.get("issue_url", "")).strip() or None,
            run_id=str(payload.get("run_id", "")).strip(),
            workflow_run_url=str(payload.get("workflow_run_url", "")).strip(),
            artifact_name=str(payload.get("artifact_name", "")).strip(),
            summary_text=str(payload.get("summary_text", "")).strip(),
        )


def classify_summary(summary: dict[str, Any] | None) -> NightlyClassification:
    if not isinstance(summary, dict):
        return NightlyClassification(
            kind="infra_failure",
            loop_state="unknown",
            stop_reason="missing_summary",
            report_status="missing",
        )

    loop_state = str(summary.get("loop_state", "")).strip() or "unknown"
    stop_reason = str(summary.get("stop_reason", "")).strip() or "unknown"
    report_status = str(summary.get("report_status", "")).strip() or "unknown"

    if loop_state == "STOP_FIX":
        kind = "product_regression"
    elif loop_state == "STOP_REPORT":
        kind = "pass"
    else:
        kind = "infra_failure"

    return NightlyClassification(
        kind=kind,
        loop_state=loop_state,
        stop_reason=stop_reason,
        report_status=report_status,
    )


def determine_notification_action(*, classification_kind: str, has_open_issue: bool) -> str:
    if classification_kind == "product_regression":
        return "regression_opened" if not has_open_issue else "regression_still_open"
    if classification_kind == "pass":
        return "recovered" if has_open_issue else "no_open_issue"
    return "infra_failure"


def build_regression_issue_body(
    *,
    repo_root: Path,
    artifacts_root: Path | None,
    run_id: str,
    workflow_run_url: str,
    artifact_name: str,
) -> str:
    evidence = build_issue_evidence_markdown(
        repo_root=repo_root,
        artifacts_root=artifacts_root,
        run_id=run_id,
        expected_behavior="The nightly ultralytics/pre QA should complete without stopping on a Concierge product regression.",
    ).strip()

    lines = [
        "## Summary",
        "Nightly QA regression for `ultralytics/pre` is currently open.",
        "",
        "## Latest Failing Run",
        f"- Run ID: `{run_id}`",
        f"- Workflow run: {workflow_run_url}",
        f"- Artifact: `{artifact_name}`",
        "",
        "## Automation",
        "This issue is managed by `.github/workflows/nightly-ultralytics-qa.yml`.",
        "It stays open while nightly product regressions continue and closes automatically after a passing nightly.",
        "",
        evidence,
        "",
    ]
    return "\n".join(lines)


def build_recovery_comment(*, run_id: str, workflow_run_url: str, artifact_name: str) -> str:
    return "\n".join(
        [
            f"Nightly ultralytics/pre QA recovered on run `{run_id}`.",
            "",
            f"- Workflow run: {workflow_run_url}",
            f"- Artifact: `{artifact_name}`",
            "",
            "Closing the rolling nightly regression issue.",
        ]
    )


def build_slack_payload(result: NightlySyncResult) -> dict[str, str] | None:
    if result.notification_action not in {"regression_opened", "recovered"}:
        return None

    issue_label = result.issue_title or DEFAULT_ISSUE_TITLE
    issue_reference = issue_label
    if result.issue_url:
        issue_reference = f"<{result.issue_url}|{issue_label}>"

    issue_heading = "Rolling issue" if result.notification_action == "regression_opened" else "Closed issue"
    summary_heading = (
        "Nightly ultralytics/pre QA regression detected."
        if result.notification_action == "regression_opened"
        else "Nightly ultralytics/pre QA recovered."
    )
    text = "\n".join(
        [
            summary_heading,
            f"- Outcome: {result.summary_text}",
            f"- Run: <{result.workflow_run_url}|{result.run_id}>",
            f"- {issue_heading}: {issue_reference}",
        ]
    )
    return {"text": text}


def validate_webhook_url(webhook_url: str, *, allow_insecure_webhook_url: bool) -> str:
    cleaned = webhook_url.strip()
    if not cleaned:
        raise ValueError("webhook URL is required")

    parsed = parse.urlparse(cleaned)
    if parsed.scheme not in {"https", "http"} or not parsed.netloc:
        raise ValueError(f"invalid webhook URL: {cleaned}")

    if parsed.scheme != "https" and not allow_insecure_webhook_url:
        raise ValueError(
            "refusing non-HTTPS webhook URL without --allow-insecure-webhook-url"
        )

    return cleaned


def post_slack_payload(
    *,
    webhook_url: str,
    payload: dict[str, str],
) -> None:
    body = json.dumps(payload).encode("utf-8")
    req = request.Request(
        webhook_url,
        data=body,
        method="POST",
        headers={"Content-Type": "application/json"},
    )
    try:
        with request.urlopen(req, timeout=30) as response:
            response.read()
    except error.URLError as exc:
        raise RuntimeError(f"Slack webhook POST failed: {exc}") from exc


def load_summary(*, repo_root: Path, artifacts_root: Path | None, run_id: str) -> dict[str, Any] | None:
    resolved_artifacts_root = resolve_artifacts_root(repo_root, artifacts_root)
    path = summary_path_for(resolved_artifacts_root, run_id)
    if not path.is_file():
        return None
    return load_json(path)


def load_report(*, repo_root: Path, artifacts_root: Path | None, run_id: str, summary: dict[str, Any] | None) -> dict[str, Any] | None:
    candidate = None
    if isinstance(summary, dict):
        candidate = str(summary.get("paths", {}).get("report_json", "")).strip()
    if candidate:
        path = Path(candidate)
        if path.is_file():
            return load_json(path)

    resolved_artifacts_root = resolve_artifacts_root(repo_root, artifacts_root)
    path = resolved_artifacts_root / "reports" / f"{run_id}.json"
    if not path.is_file():
        return None
    return load_json(path)


def find_open_issue_by_title(title: str) -> dict[str, Any] | None:
    completed = run_gh(
        [
            "issue",
            "list",
            "--state",
            "open",
            "--limit",
            "200",
            "--json",
            "number,title,url",
        ],
        capture_output=True,
    )
    issues = json.loads(completed.stdout)
    for issue in issues:
        if str(issue.get("title", "")).strip() == title:
            return issue
    return None


def issue_reference_from_url(*, url: str | None, title: str) -> dict[str, Any] | None:
    cleaned_url = (url or "").strip()
    if not cleaned_url:
        return None

    issue_number: int | None = None
    try:
        issue_number = int(cleaned_url.rstrip("/").rsplit("/", 1)[-1])
    except ValueError:
        issue_number = None

    return {
        "number": issue_number,
        "title": title,
        "url": cleaned_url,
    }


def build_summary_text(
    *,
    classification: NightlyClassification,
    notification_action: str,
    report: dict[str, Any] | None,
) -> str:
    if notification_action == "recovered":
        return "Nightly ultralytics/pre QA passed and closed the rolling regression issue."
    if notification_action == "no_open_issue":
        return "Nightly ultralytics/pre QA passed with no open rolling regression issue."

    if isinstance(report, dict):
        overall_outcome = str(report.get("overall_outcome", "")).strip()
        if overall_outcome:
            return overall_outcome

    return (
        "Nightly ultralytics/pre QA "
        f"stopped on {classification.loop_state} ({classification.stop_reason})."
    )


def write_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")


def sync_issue(
    *,
    repo_root: Path,
    artifacts_root: Path | None,
    run_id: str,
    issue_title: str,
    workflow_run_url: str,
    artifact_name: str,
) -> NightlySyncResult:
    summary = load_summary(repo_root=repo_root, artifacts_root=artifacts_root, run_id=run_id)
    classification = classify_summary(summary)
    existing_issue = find_open_issue_by_title(issue_title)
    notification_action = determine_notification_action(
        classification_kind=classification.kind,
        has_open_issue=existing_issue is not None,
    )
    issue_reference = existing_issue

    if classification.kind == "product_regression":
        body = build_regression_issue_body(
            repo_root=repo_root,
            artifacts_root=artifacts_root,
            run_id=run_id,
            workflow_run_url=workflow_run_url,
            artifact_name=artifact_name,
        )
        if existing_issue is None:
            created = run_gh(
                [
                    "issue",
                    "create",
                    "--title",
                    issue_title,
                    "--label",
                    "bug",
                    "--label",
                    "testing",
                    "--body-file",
                    "-",
                ],
                capture_output=True,
                input_text=body,
            )
            issue_reference = issue_reference_from_url(url=created.stdout.strip(), title=issue_title)
            if issue_reference is None:
                issue_reference = find_open_issue_by_title(issue_title)
            print(f"[qa-nightly] created rolling regression issue: {issue_title}", flush=True)
        else:
            run_gh(
                [
                    "issue",
                    "edit",
                    str(existing_issue["number"]),
                    "--add-label",
                    "bug",
                    "--add-label",
                    "testing",
                    "--body-file",
                    "-",
                ],
                input_text=body,
            )
            print(f"[qa-nightly] updated rolling regression issue #{existing_issue['number']}", flush=True)

    elif classification.kind == "pass":
        if existing_issue is None:
            print("[qa-nightly] no open rolling regression issue to close", flush=True)
        else:
            comment = build_recovery_comment(
                run_id=run_id,
                workflow_run_url=workflow_run_url,
                artifact_name=artifact_name,
            )
            run_gh(
                [
                    "issue",
                    "comment",
                    str(existing_issue["number"]),
                    "--body-file",
                    "-",
                ],
                input_text=comment,
            )
            run_gh(["issue", "close", str(existing_issue["number"])])
            print(f"[qa-nightly] closed rolling regression issue #{existing_issue['number']}", flush=True)
    else:
        print("[qa-nightly] infra or harness failure; leaving rolling regression issue untouched", flush=True)

    report = load_report(repo_root=repo_root, artifacts_root=artifacts_root, run_id=run_id, summary=summary)
    result = NightlySyncResult(
        classification_kind=classification.kind,
        loop_state=classification.loop_state,
        stop_reason=classification.stop_reason,
        report_status=classification.report_status,
        notification_action=notification_action,
        issue_number=int(issue_reference["number"]) if issue_reference and issue_reference.get("number") is not None else None,
        issue_title=(str(issue_reference.get("title", "")).strip() or None) if issue_reference else None,
        issue_url=(str(issue_reference.get("url", "")).strip() or None) if issue_reference else None,
        run_id=run_id,
        workflow_run_url=workflow_run_url,
        artifact_name=artifact_name,
        summary_text=build_summary_text(
            classification=classification,
            notification_action=notification_action,
            report=report,
        ),
    )
    print(
        f"[qa-nightly] classification={classification.kind} "
        f"loop_state={classification.loop_state} stop_reason={classification.stop_reason} "
        f"notification_action={notification_action}",
        flush=True,
    )
    return result


def run_gh(args: list[str], *, capture_output: bool = False, input_text: str | None = None) -> subprocess.CompletedProcess[str]:
    completed = subprocess.run(
        ["gh", *args],
        input=input_text,
        text=True,
        capture_output=capture_output,
        check=False,
    )
    if completed.returncode != 0:
        if capture_output:
            sys.stderr.write(completed.stderr)
        raise RuntimeError(f"gh {' '.join(args)} failed with exit code {completed.returncode}")
    return completed


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Manage the rolling nightly QA regression issue for ultralytics/pre.")
    subparsers = parser.add_subparsers(dest="command", required=True)

    sync_parser = subparsers.add_parser("sync-issue")
    sync_parser.add_argument("--repo-root", default=str(REPO_ROOT))
    sync_parser.add_argument("--artifacts-root", default=None)
    sync_parser.add_argument("--run-id", required=True)
    sync_parser.add_argument("--issue-title", default=DEFAULT_ISSUE_TITLE)
    sync_parser.add_argument("--workflow-run-url", required=True)
    sync_parser.add_argument("--artifact-name", required=True)
    sync_parser.add_argument("--result-json", default=None)

    slack_parser = subparsers.add_parser("build-slack-payload")
    slack_parser.add_argument("--result-json", required=True)

    send_parser = subparsers.add_parser("send-slack")
    send_parser.add_argument("--result-json", required=True)
    send_parser.add_argument("--webhook-url", required=True)
    send_parser.add_argument("--allow-insecure-webhook-url", action="store_true")

    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)

    if args.command == "sync-issue":
        repo_root = Path(args.repo_root).resolve()
        artifacts_root = Path(args.artifacts_root).resolve() if args.artifacts_root else None
        result = sync_issue(
            repo_root=repo_root,
            artifacts_root=artifacts_root,
            run_id=args.run_id,
            issue_title=args.issue_title,
            workflow_run_url=args.workflow_run_url,
            artifact_name=args.artifact_name,
        )
        if args.result_json:
            write_json(Path(args.result_json).resolve(), result.to_dict())
        return 0

    if args.command == "build-slack-payload":
        result = NightlySyncResult.from_dict(load_json(Path(args.result_json).resolve()))
        payload = build_slack_payload(result)
        if payload is None:
            raise SystemExit(
                f"no Slack payload for notification action: {result.notification_action}"
            )
        json.dump(payload, sys.stdout)
        sys.stdout.write("\n")
        return 0

    if args.command == "send-slack":
        result = NightlySyncResult.from_dict(load_json(Path(args.result_json).resolve()))
        payload = build_slack_payload(result)
        if payload is None:
            raise SystemExit(
                f"no Slack payload for notification action: {result.notification_action}"
            )
        webhook_url = validate_webhook_url(
            args.webhook_url,
            allow_insecure_webhook_url=bool(args.allow_insecure_webhook_url),
        )
        post_slack_payload(webhook_url=webhook_url, payload=payload)
        print(
            f"[qa-nightly] sent Slack notification for {result.notification_action}",
            flush=True,
        )
        return 0

    raise AssertionError(f"unsupported command: {args.command}")


if __name__ == "__main__":
    raise SystemExit(main())
