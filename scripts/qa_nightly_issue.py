#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any

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


def load_summary(*, repo_root: Path, artifacts_root: Path | None, run_id: str) -> dict[str, Any] | None:
    resolved_artifacts_root = resolve_artifacts_root(repo_root, artifacts_root)
    path = summary_path_for(resolved_artifacts_root, run_id)
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


def sync_issue(
    *,
    repo_root: Path,
    artifacts_root: Path | None,
    run_id: str,
    issue_title: str,
    workflow_run_url: str,
    artifact_name: str,
) -> int:
    summary = load_summary(repo_root=repo_root, artifacts_root=artifacts_root, run_id=run_id)
    classification = classify_summary(summary)
    print(
        f"[qa-nightly] classification={classification.kind} "
        f"loop_state={classification.loop_state} stop_reason={classification.stop_reason}",
        flush=True,
    )

    existing_issue = find_open_issue_by_title(issue_title)

    if classification.kind == "product_regression":
        body = build_regression_issue_body(
            repo_root=repo_root,
            artifacts_root=artifacts_root,
            run_id=run_id,
            workflow_run_url=workflow_run_url,
            artifact_name=artifact_name,
        )
        if existing_issue is None:
            run_gh(
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
                input_text=body,
            )
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
        return 0

    if classification.kind == "pass":
        if existing_issue is None:
            print("[qa-nightly] no open rolling regression issue to close", flush=True)
            return 0

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
        return 0

    print("[qa-nightly] infra or harness failure; leaving rolling regression issue untouched", flush=True)
    return 0


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

    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    repo_root = Path(args.repo_root).resolve()
    artifacts_root = Path(args.artifacts_root).resolve() if args.artifacts_root else None

    if args.command == "sync-issue":
        return sync_issue(
            repo_root=repo_root,
            artifacts_root=artifacts_root,
            run_id=args.run_id,
            issue_title=args.issue_title,
            workflow_run_url=args.workflow_run_url,
            artifact_name=args.artifact_name,
        )

    raise AssertionError(f"unsupported command: {args.command}")


if __name__ == "__main__":
    raise SystemExit(main())
