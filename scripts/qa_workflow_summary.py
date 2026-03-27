#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[1]
if str(REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(REPO_ROOT))

from scripts.qa_issue_evidence import (
    clean_items,
    clean_text,
    load_json,
    normalize_qa_context,
    path_from_summary,
    report_json_path_for,
    resolve_artifacts_root,
    summarize_exported_artifacts,
    summary_path_for,
)


def turns_path_for(artifacts_root: Path, run_id: str) -> Path:
    return artifacts_root / "runs" / run_id / "turns.jsonl"


def load_turn_summaries(path: Path) -> list[tuple[int, str]]:
    if not path.is_file():
        return []

    summaries: list[tuple[int, str]] = []
    last_summary = ""
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        raw_line = raw_line.strip()
        if not raw_line:
            continue
        payload = json.loads(raw_line)
        iteration = int(payload.get("iteration", 0))
        directive = payload.get("directive", {})
        summary = clean_text(directive.get("summary", "")) if isinstance(directive, dict) else ""
        if not summary or summary == last_summary:
            continue
        summaries.append((iteration, summary))
        last_summary = summary
    return summaries


def evidence_events_paths(run_root: Path) -> list[Path]:
    candidates = [
        run_root / "docker" / "export" / "workspace" / ".concierge" / "evidence",
        run_root / "docker" / "workspace.concierge" / "evidence",
    ]
    event_paths: list[Path] = []
    for evidence_root in candidates:
        if not evidence_root.is_dir():
            continue
        event_paths.extend(sorted(evidence_root.glob("*/events.jsonl")))
    return event_paths


def load_observed_step_trajectory(run_root: Path) -> list[str]:
    observed_steps: list[str] = []
    for events_path in evidence_events_paths(run_root):
        for raw_line in events_path.read_text(encoding="utf-8").splitlines():
            raw_line = raw_line.strip()
            if not raw_line:
                continue
            payload = json.loads(raw_line)
            if clean_text(payload.get("kind", "")) != "step_selected":
                continue
            step_id = clean_text(payload.get("stepId", ""))
            if not step_id:
                continue
            if observed_steps and observed_steps[-1] == step_id:
                continue
            observed_steps.append(step_id)
    return observed_steps


def describe_trajectory_regression(observed_steps: list[str]) -> str:
    early_steps = {
        "ensure.leap_yaml",
        "ensure.integration_script",
        "ensure.integration_test_contract",
        "ensure.preprocess_contract",
    }
    downstream_steps = {
        "ensure.input_encoders",
        "ensure.ground_truth_encoders",
        "ensure.model_acquisition",
        "ensure.model_contract",
        "ensure.integration_test_wiring",
    }

    latest_downstream = ""
    for step_id in observed_steps:
        if step_id in downstream_steps:
            latest_downstream = step_id
            continue
        if latest_downstream and step_id in early_steps:
            return f"Early-step fallback detected: {step_id} after downstream progress at {latest_downstream}."
    if observed_steps:
        return "No early-step fallback detected after downstream progress."
    return "No canonical step trajectory was exported for this run."


def render_bullets(items: list[str]) -> list[str]:
    return [f"- {item}" for item in items] if items else ["- None recorded."]


def build_workflow_summary_markdown(
    *,
    repo_root: Path,
    run_id: str,
    artifacts_root: Path | None = None,
    ref_under_test: str = "",
    artifact_name: str = "",
) -> str:
    repo_root = Path(repo_root).resolve()
    artifacts_root = resolve_artifacts_root(repo_root, artifacts_root)
    summary_path = summary_path_for(artifacts_root, run_id)

    if not summary_path.is_file():
        lines = [
            "## QA Loop",
            "",
            f"- Ref: `{clean_text(ref_under_test) or 'unknown'}`",
            f"- Run ID: `{run_id}`",
            f"- Artifact: `{clean_text(artifact_name) or f'qa-loop-{run_id}'}`",
            "- Summary JSON: missing",
            "",
        ]
        return "\n".join(lines)

    summary = load_json(summary_path)
    qa_context = normalize_qa_context(summary)
    report_json_path = path_from_summary(summary, "report_json", report_json_path_for(artifacts_root, run_id))
    turns_json_path = path_from_summary(summary, "turns_jsonl", turns_path_for(artifacts_root, run_id))
    report = load_json(report_json_path) if report_json_path.is_file() else {}
    turn_summaries = load_turn_summaries(turns_json_path)
    observed_trajectory = load_observed_step_trajectory(summary_path.parent)
    trajectory_assessment = describe_trajectory_regression(observed_trajectory)
    final_observed_step = turn_summaries[-1][1] if turn_summaries else clean_text(report.get("integration_progress", ""))
    key_issues = clean_items(report.get("product_issues", []))[:3] + clean_items(report.get("agent_interaction_issues", []))[:2]

    lines = [
        "## QA Loop",
        "",
        f"- Ref: `{clean_text(ref_under_test) or qa_context.get('ref_under_test') or 'unknown'}`",
        f"- Fixture: `{qa_context.get('fixture_id') or 'unknown'}`",
        f"- Step: `{qa_context.get('guide_step') or 'unknown'}`",
        f"- Resolved checkpoint: `{qa_context.get('checkpoint_key') or 'unresolved'}`",
        f"- Source: `{qa_context.get('source_kind') or 'unknown'}:{qa_context.get('source_id') or 'unknown'}`",
        f"- Run ID: `{run_id}`",
        f"- Loop state: `{clean_text(summary.get('loop_state', 'unknown'))}`",
        f"- Stop reason: `{clean_text(summary.get('stop_reason', 'unknown'))}`",
        f"- Artifact: `{clean_text(artifact_name) or f'qa-loop-{run_id}'}`",
        "",
        "### Outcome",
        clean_text(report.get("overall_outcome", "")) or "No synthesized QA outcome was recorded.",
        "",
        "### Integration Progress",
        clean_text(report.get("integration_progress", "")) or "No integration progress summary was provided.",
        "",
        "### Timeline",
        *render_bullets([f"Turn {iteration}: {summary_text}" for iteration, summary_text in turn_summaries]),
        "",
        "### Observed Step Trajectory",
        " -> ".join(observed_trajectory) if observed_trajectory else "No canonical step trajectory was exported for this run.",
        "",
        "### Trajectory Assessment",
        trajectory_assessment,
        "",
        "### Final Observed Step",
        final_observed_step or "No final step summary was recorded.",
        "",
        "### Key Issues",
        *render_bullets(key_issues),
        "",
        "### Exported Evidence",
        *render_bullets(summarize_exported_artifacts(summary_path.parent)),
        "",
    ]
    return "\n".join(lines).rstrip() + "\n"


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Render a QA workflow markdown summary for GITHUB_STEP_SUMMARY.")
    parser.add_argument("--repo-root", default=str(REPO_ROOT))
    parser.add_argument("--artifacts-root")
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--ref", default="")
    parser.add_argument("--artifact-name", default="")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    artifacts_root = Path(args.artifacts_root).resolve() if args.artifacts_root else None
    markdown = build_workflow_summary_markdown(
        repo_root=Path(args.repo_root),
        artifacts_root=artifacts_root,
        run_id=args.run_id,
        ref_under_test=args.ref,
        artifact_name=args.artifact_name,
    )
    sys.stdout.write(markdown)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
