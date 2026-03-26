#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parents[1]
FAILURE_MARKERS = (
    "error",
    "exception",
    "failed",
    "failure",
    "traceback",
    "missing",
    "blocked",
    "no module",
    "stop_fix",
    "stop_deadend",
)
QA_CONTEXT_KEYS = (
    "fixture_id",
    "guide_step",
    "ref_under_test",
    "checkpoint_key",
    "source_kind",
    "source_id",
)


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def clean_text(value: Any) -> str:
    return str(value or "").strip()


def clean_items(values: Any) -> list[str]:
    if not isinstance(values, list):
        return []
    return [str(value).strip() for value in values if str(value).strip()]


def resolve_artifacts_root(repo_root: Path, artifacts_root: Path | None) -> Path:
    root = artifacts_root if artifacts_root is not None else repo_root / "QA"
    return Path(root).resolve()


def summary_path_for(artifacts_root: Path, run_id: str) -> Path:
    return artifacts_root / "runs" / run_id / "summary.json"


def report_json_path_for(artifacts_root: Path, run_id: str) -> Path:
    return artifacts_root / "reports" / f"{run_id}.json"


def report_markdown_path_for(artifacts_root: Path, run_id: str) -> Path:
    return artifacts_root / "reports" / f"{run_id}.md"


def transcript_path_for(artifacts_root: Path, run_id: str) -> Path:
    return artifacts_root / "transcripts" / f"{run_id}.full.txt"


def path_from_summary(summary: dict[str, Any], key: str, fallback: Path) -> Path:
    raw_value = summary.get("paths", {}).get(key, "")
    candidate = Path(raw_value).expanduser() if clean_text(raw_value) else fallback
    return candidate.resolve()


def normalize_qa_context(summary: dict[str, Any]) -> dict[str, str]:
    context = {key: "" for key in QA_CONTEXT_KEYS}
    raw_context = summary.get("qa_context", {})
    if not isinstance(raw_context, dict):
        return context
    for key in QA_CONTEXT_KEYS:
        context[key] = clean_text(raw_context.get(key, ""))
    return context


def default_expected_behavior(summary: dict[str, Any], context: dict[str, str]) -> str:
    fixture_id = context.get("fixture_id") or "the selected fixture"
    guide_step = context.get("guide_step") or "the selected guide step"
    _ = summary
    return f"Concierge should complete the `{guide_step}` QA flow for `{fixture_id}` without stopping on an integration defect."


def default_actual_behavior(summary: dict[str, Any], report: dict[str, Any]) -> str:
    details: list[str] = []
    overall_outcome = clean_text(report.get("overall_outcome", ""))
    if overall_outcome:
        details.append(overall_outcome)
    else:
        details.append(
            f"QA loop ended in `{clean_text(summary.get('loop_state', 'unknown'))}` with stop reason `{clean_text(summary.get('stop_reason', 'unknown'))}`."
        )
    for item in clean_items(report.get("product_issues", []))[:1]:
        details.append(item)
    for item in clean_items(report.get("agent_interaction_issues", []))[:1]:
        details.append(item)
    return " ".join(details).strip()


def extract_transcript_excerpts(
    transcript_text: str,
    *,
    stop_reason: str,
    loop_state: str,
    max_excerpts: int = 3,
) -> list[str]:
    lines = [line.rstrip() for line in transcript_text.splitlines()]
    if not lines:
        return []

    search_terms = {
        clean_text(stop_reason).lower().replace("_", " "),
        clean_text(stop_reason).lower(),
        clean_text(loop_state).lower().replace("_", " "),
        clean_text(loop_state).lower(),
    }
    search_terms = {term for term in search_terms if term}

    hits: list[int] = []
    for index, line in enumerate(lines):
        lowered = line.lower()
        if any(term in lowered for term in search_terms) or any(marker in lowered for marker in FAILURE_MARKERS):
            hits.append(index)

    if not hits:
        hits.append(max(len(lines) - 1, 0))

    windows: list[list[int]] = []
    for index in hits:
        start = max(0, index - 2)
        end = min(len(lines), index + 3)
        if windows and start <= windows[-1][1]:
            windows[-1][1] = max(windows[-1][1], end)
        else:
            windows.append([start, end])

    excerpts: list[str] = []
    for start, end in windows[-max_excerpts:]:
        excerpt_lines = lines[start:end]
        if start > 0:
            excerpt_lines.insert(0, "...")
        if end < len(lines):
            excerpt_lines.append("...")
        excerpts.append("\n".join(excerpt_lines).strip())
    return excerpts


def summarize_exported_artifacts(run_dir: Path) -> list[str]:
    summaries: list[str] = []
    concierge_root = run_dir / "docker" / "export" / "workspace" / ".concierge"
    if concierge_root.exists():
        file_count = sum(1 for path in concierge_root.rglob("*") if path.is_file())
        summaries.append(f"workspace/.concierge: {file_count} files")

    docker_dir = run_dir / "docker"
    diff_count = len(list(docker_dir.glob("turn-*.diff.txt")))
    if diff_count:
        summaries.append(f"Docker diff snapshots: {diff_count} files")

    inspect_count = len(list(docker_dir.glob("turn-*.inspect.json")))
    if inspect_count:
        summaries.append(f"Docker inspect snapshots: {inspect_count} files")

    if not summaries:
        summaries.append("None recorded.")
    return summaries


def render_bullets(items: list[str]) -> list[str]:
    return [f"- {item}" for item in items] if items else ["- None recorded."]


def build_issue_evidence_markdown(
    *,
    repo_root: Path,
    run_id: str,
    artifacts_root: Path | None = None,
    expected_behavior: str | None = None,
    actual_behavior: str | None = None,
) -> str:
    repo_root = Path(repo_root).resolve()
    artifacts_root = resolve_artifacts_root(repo_root, artifacts_root)
    summary_path = summary_path_for(artifacts_root, run_id)
    if not summary_path.is_file():
        raise FileNotFoundError(f"QA summary not found for run {run_id!r}: {summary_path}")

    summary = load_json(summary_path)
    qa_context = normalize_qa_context(summary)
    run_dir = summary_path.parent
    report_json_path = path_from_summary(summary, "report_json", report_json_path_for(artifacts_root, run_id))
    report_markdown_path = path_from_summary(summary, "report_markdown", report_markdown_path_for(artifacts_root, run_id))
    transcript_path = path_from_summary(summary, "full_transcript", transcript_path_for(artifacts_root, run_id))

    report = load_json(report_json_path) if report_json_path.is_file() else {}
    transcript_text = transcript_path.read_text(encoding="utf-8") if transcript_path.is_file() else ""

    expected = clean_text(expected_behavior) or default_expected_behavior(summary, qa_context)
    actual = clean_text(actual_behavior) or default_actual_behavior(summary, report)
    integration_progress = clean_text(report.get("integration_progress", "")) or "No integration progress summary was provided."
    product_issues = clean_items(report.get("product_issues", []))
    agent_interaction_issues = clean_items(report.get("agent_interaction_issues", []))
    notable_moments = clean_items(report.get("notable_moments", []))
    transcript_excerpts = extract_transcript_excerpts(
        transcript_text,
        stop_reason=clean_text(summary.get("stop_reason", "")),
        loop_state=clean_text(summary.get("loop_state", "")),
    )
    artifact_summary = summarize_exported_artifacts(run_dir)

    lines = [
        "## Inline Evidence Bundle",
        "",
        f"- Run ID: `{run_id}`",
        f"- Fixture: `{qa_context.get('fixture_id') or 'unknown'}`",
        f"- Guide step: `{qa_context.get('guide_step') or 'unknown'}`",
        f"- Ref under test: `{qa_context.get('ref_under_test') or 'unknown'}`",
        f"- Loop state: `{clean_text(summary.get('loop_state', 'unknown')) or 'unknown'}`",
        f"- Stop reason: `{clean_text(summary.get('stop_reason', 'unknown')) or 'unknown'}`",
    ]
    if qa_context.get("checkpoint_key"):
        lines.append(f"- Checkpoint: `{qa_context['checkpoint_key']}`")
    if qa_context.get("source_kind") or qa_context.get("source_id"):
        lines.append(
            f"- Resolved QA source: `{qa_context.get('source_kind') or 'unknown'}:{qa_context.get('source_id') or 'unknown'}`"
        )

    lines.extend(
        [
            "",
            "### Expected Behavior",
            expected,
            "",
            "### Actual Behavior",
            actual,
            "",
            "### Integration Progress",
            integration_progress,
            "",
            "### Product Issues",
            *render_bullets(product_issues),
            "",
            "### Agent Interaction Issues",
            *render_bullets(agent_interaction_issues),
            "",
            "### Notable Moments",
            *render_bullets(notable_moments),
            "",
        ]
    )

    if transcript_excerpts:
        for index, excerpt in enumerate(transcript_excerpts, start=1):
            lines.extend(
                [
                    f"### Transcript Excerpt {index}",
                    "```text",
                    excerpt,
                    "```",
                    "",
                ]
            )

    lines.extend(
        [
            "### Artifact Summary",
            *render_bullets(artifact_summary),
            "",
            "### Local Artifacts",
            f"- Summary JSON: `{summary_path.resolve()}`",
            f"- Report JSON: `{report_json_path.resolve()}`",
            f"- Report Markdown: `{report_markdown_path.resolve()}`",
            f"- Full transcript: `{transcript_path.resolve()}`",
            "",
        ]
    )
    return "\n".join(lines).rstrip() + "\n"


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Render a saved QA run into issue-ready markdown with inline evidence.",
    )
    parser.add_argument("--repo-root", default=str(REPO_ROOT))
    parser.add_argument("--artifacts-root", default=None)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--expected-behavior", default=None)
    parser.add_argument("--actual-behavior", default=None)
    parser.add_argument("--output", default=None)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    repo_root = Path(args.repo_root).resolve()
    artifacts_root = Path(args.artifacts_root).resolve() if args.artifacts_root else None
    markdown = build_issue_evidence_markdown(
        repo_root=repo_root,
        artifacts_root=artifacts_root,
        run_id=args.run_id,
        expected_behavior=args.expected_behavior,
        actual_behavior=args.actual_behavior,
    )
    if args.output:
        Path(args.output).write_text(markdown, encoding="utf-8")
    else:
        sys.stdout.write(markdown)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
