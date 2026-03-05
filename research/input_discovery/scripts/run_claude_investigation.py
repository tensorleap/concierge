#!/usr/bin/env python3
"""Temporary research runner for Claude semantic investigation with full logging.

This is research-only code. It preserves all prompts and Claude activity logs for
human inspection.
"""

from __future__ import annotations

import argparse
import json
import shlex
import subprocess
from collections import Counter
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


def utc_now_iso() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat()


def read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def write_text(path: Path, value: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(value, encoding="utf-8")


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="Run Claude semantic investigation with logging")
    p.add_argument("--repo", required=True, help="Repository path Claude should investigate")
    p.add_argument("--lead-pack", required=True, help="lead_pack.json path")
    p.add_argument("--results-root", required=True, help="Root results directory")
    p.add_argument("--experiment-id", required=True, help="Experiment id")
    p.add_argument(
        "--system-prompt",
        default="research/input_discovery/prompts/pytorch_semantic_investigator.system.md",
        help="System prompt file",
    )
    p.add_argument(
        "--user-template",
        default="research/input_discovery/prompts/pytorch_semantic_investigator.user_template.md",
        help="User prompt template file",
    )
    p.add_argument(
        "--schema",
        default="research/input_discovery/schemas/agent_findings.schema.json",
        help="Findings schema file",
    )
    p.add_argument(
        "--tools",
        default="Read,Grep,Glob,LS",
        help="Claude tool whitelist for read-only investigation",
    )
    p.add_argument(
        "--model",
        default="claude-opus-4-6",
        help="Claude model id to use for the investigation run",
    )
    p.add_argument(
        "--max-summary-files",
        type=int,
        default=10,
        help="Top lead files to include in rendered summary",
    )
    p.add_argument(
        "--allow-auth-failure",
        action="store_true",
        help="Do not fail script when Claude auth fails; still write logs",
    )
    p.add_argument(
        "--allow-tool-errors",
        action="store_true",
        help="Allow run completion even when tool activity contains explicit errors",
    )
    p.add_argument(
        "--allow-missing-lead-pack-read",
        action="store_true",
        help="Allow run completion when lead_pack.json was not successfully read by Claude tool call",
    )
    return p.parse_args()


@dataclass
class ClaudeRunPaths:
    run_dir: Path
    system_prompt_copy: Path
    user_prompt_copy: Path
    rendered_summary: Path
    command_txt: Path
    metadata_json: Path
    stream_jsonl: Path
    stderr_log: Path
    final_json: Path
    final_text: Path
    activity_log_md: Path
    quality_json: Path


def prepare_paths(results_root: Path, experiment_id: str) -> ClaudeRunPaths:
    run_dir = results_root / experiment_id / "claude_run"
    return ClaudeRunPaths(
        run_dir=run_dir,
        system_prompt_copy=run_dir / "claude_system_prompt.md",
        user_prompt_copy=run_dir / "claude_user_prompt.md",
        rendered_summary=run_dir / "lead_summary_for_prompt.txt",
        command_txt=run_dir / "claude_command.txt",
        metadata_json=run_dir / "run_metadata.json",
        stream_jsonl=run_dir / "claude_stream.jsonl",
        stderr_log=run_dir / "claude_stderr.log",
        final_json=run_dir / "claude_final_findings.json",
        final_text=run_dir / "claude_final_text.txt",
        activity_log_md=run_dir / "claude_activity_log.md",
        quality_json=run_dir / "run_quality.json",
    )


def build_lead_summary(lead_pack: dict[str, Any], max_files: int) -> str:
    lines: list[str] = []
    lines.append(f"Method: {lead_pack.get('method_version', '<unknown>')}")
    totals = lead_pack.get("totals", {})
    repo = lead_pack.get("repo", {})
    lines.append(f"Python files scanned: {repo.get('python_files_scanned', 0)}")
    lines.append(f"Files with hits: {totals.get('files_with_hits', 0)}")
    lines.append(f"Total signal hits: {totals.get('signal_hit_count', 0)}")
    lines.append("")
    lines.append("Top lead files:")
    for i, item in enumerate(lead_pack.get("files", [])[:max_files], start=1):
        lines.append(f"{i}. {item.get('path')} (score={item.get('score')})")
        for hit in item.get("signal_hits", [])[:4]:
            lines.append(
                f"   - {hit.get('signal_id')}: count={hit.get('count')}, contribution={hit.get('contribution')}"
            )
            for occ in hit.get("occurrences", [])[:2]:
                lines.append(f"     line {occ.get('line')}: {occ.get('snippet')}")
        lines.append("")
    return "\n".join(lines).rstrip() + "\n"


def render_user_prompt(template: str, *, repo_path: Path, experiment_id: str, lead_pack_path: Path, lead_summary: str) -> str:
    return (
        template.replace("{{REPO_PATH}}", str(repo_path))
        .replace("{{EXPERIMENT_ID}}", experiment_id)
        .replace("{{LEAD_PACK_PATH}}", str(lead_pack_path))
        .replace("{{LEAD_SUMMARY}}", lead_summary.rstrip())
    )


def extract_stream_events(stream_path: Path) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []
    if not stream_path.exists():
        return events
    for raw_line in stream_path.read_text(encoding="utf-8", errors="replace").splitlines():
        line = raw_line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
            events.append(event)
        except json.JSONDecodeError:
            events.append({"_raw": raw_line})
    return events


def to_rel(path_value: str, repo_path: Path) -> str:
    path = Path(path_value)
    try:
        return str(path.resolve().relative_to(repo_path.resolve()))
    except Exception:
        return str(path)


def one_line(text: str, limit: int = 220) -> str:
    collapsed = " ".join(text.split())
    if len(collapsed) <= limit:
        return collapsed
    return collapsed[: limit - 3] + "..."


def summarize_tool_input(tool_name: str, tool_input: Any, repo_path: Path) -> str:
    if not isinstance(tool_input, dict):
        return one_line(str(tool_input))

    if tool_name == "Read":
        path = tool_input.get("file_path")
        if isinstance(path, str):
            return f"file={to_rel(path, repo_path)}"
    elif tool_name == "Glob":
        pattern = tool_input.get("pattern")
        path = tool_input.get("path")
        if isinstance(pattern, str) and isinstance(path, str):
            return f"pattern={pattern} path={to_rel(path, repo_path)}"
    elif tool_name == "Grep":
        pattern = tool_input.get("pattern")
        path = tool_input.get("path")
        if isinstance(pattern, str) and isinstance(path, str):
            return f"pattern={pattern} path={to_rel(path, repo_path)}"
    elif tool_name == "LS":
        path = tool_input.get("path")
        if isinstance(path, str):
            return f"path={to_rel(path, repo_path)}"

    return one_line(json.dumps(tool_input, ensure_ascii=False))


def summarize_tool_result(tool_name: str, result_data: dict[str, Any], repo_path: Path) -> list[str]:
    lines: list[str] = []
    structured = result_data.get("structured")
    content_text = result_data.get("text")

    if isinstance(structured, dict):
        file_obj = structured.get("file")
        if isinstance(file_obj, dict):
            file_path = file_obj.get("filePath")
            start = file_obj.get("startLine")
            num = file_obj.get("numLines")
            total = file_obj.get("totalLines")
            if isinstance(file_path, str):
                if isinstance(start, int) and isinstance(num, int) and num > 0:
                    end = start + num - 1
                    if isinstance(total, int):
                        lines.append(f"read `{to_rel(file_path, repo_path)}` lines {start}-{end} of {total}")
                    else:
                        lines.append(f"read `{to_rel(file_path, repo_path)}` lines {start}-{end}")
                else:
                    lines.append(f"read `{to_rel(file_path, repo_path)}`")

        filenames = structured.get("filenames")
        if isinstance(filenames, list):
            num_files = structured.get("numFiles")
            truncated = structured.get("truncated")
            if isinstance(num_files, int):
                lines.append(f"returned {num_files} file path(s), truncated={bool(truncated)}")
            preview = [x for x in filenames[:6] if isinstance(x, str)]
            for item in preview:
                lines.append(f"- {to_rel(item, repo_path)}")
            if isinstance(num_files, int) and num_files > len(preview):
                lines.append(f"- ... ({num_files - len(preview)} more)")

        matches = structured.get("matches")
        if isinstance(matches, list):
            lines.append(f"returned {len(matches)} match(es)")

    if not lines and isinstance(content_text, str) and content_text.strip():
        lines.append(one_line(content_text))

    if not lines:
        lines.append("(no summarized result)")

    return lines


def collect_human_activity(events: list[dict[str, Any]]) -> tuple[list[dict[str, Any]], list[str], Counter]:
    event_counts: Counter = Counter()
    tool_calls: list[dict[str, Any]] = []
    tool_idx: dict[str, int] = {}
    assistant_texts: list[str] = []

    for event in events:
        event_type = event.get("type", "<unknown>")
        event_counts[event_type] += 1

        if event_type == "assistant":
            content = event.get("message", {}).get("content", [])
            if not isinstance(content, list):
                continue
            for block in content:
                if not isinstance(block, dict):
                    continue
                block_type = block.get("type")
                if block_type == "tool_use":
                    call = {
                        "id": block.get("id", ""),
                        "name": block.get("name", "<unknown>"),
                        "input": block.get("input", {}),
                        "result": None,
                    }
                    index = len(tool_calls)
                    tool_calls.append(call)
                    if isinstance(call["id"], str) and call["id"]:
                        tool_idx[call["id"]] = index
                elif block_type == "text":
                    text = block.get("text")
                    if isinstance(text, str) and text.strip():
                        assistant_texts.append(text.strip())

        elif event_type == "user":
            content = event.get("message", {}).get("content", [])
            if not isinstance(content, list):
                continue
            for item in content:
                if not isinstance(item, dict):
                    continue
                if item.get("type") != "tool_result":
                    continue
                tool_use_id = item.get("tool_use_id")
                if not isinstance(tool_use_id, str):
                    continue
                result_data = {
                    "structured": event.get("tool_use_result"),
                    "text": item.get("content"),
                }
                idx = tool_idx.get(tool_use_id)
                if idx is not None:
                    tool_calls[idx]["result"] = result_data

    return tool_calls, assistant_texts, event_counts


def path_equals(a: str | Path, b: str | Path) -> bool:
    try:
        return Path(a).resolve() == Path(b).resolve()
    except Exception:
        return str(a) == str(b)


def is_error_text(value: str) -> bool:
    lowered = value.lower()
    markers = [
        "<tool_use_error>",
        "requested permissions",
        "permission denied",
        "errored",
        "error:",
    ]
    return any(marker in lowered for marker in markers)


def evaluate_run_quality(
    *,
    tool_calls: list[dict[str, Any]],
    lead_pack_path: Path,
    result_event: dict[str, Any] | None,
) -> dict[str, Any]:
    tool_errors: list[str] = []
    permission_errors: list[str] = []
    lead_pack_read_attempted = False
    lead_pack_read_success = False

    for idx, call in enumerate(tool_calls, start=1):
        name = str(call.get("name", "<unknown>"))
        input_data = call.get("input")
        result_data = call.get("result")
        err_prefix = f"tool#{idx} {name}"

        result_text = ""
        structured: dict[str, Any] | None = None
        if isinstance(result_data, dict):
            text_data = result_data.get("text")
            result_text = text_data if isinstance(text_data, str) else ""
            s = result_data.get("structured")
            structured = s if isinstance(s, dict) else None

        if result_text and is_error_text(result_text):
            tool_errors.append(f"{err_prefix}: {one_line(result_text, 180)}")
            if "requested permissions" in result_text.lower() or "permission denied" in result_text.lower():
                permission_errors.append(f"{err_prefix}: {one_line(result_text, 180)}")

        if name != "Read":
            continue
        if not isinstance(input_data, dict):
            continue
        file_path = input_data.get("file_path")
        if not isinstance(file_path, str):
            continue
        if not path_equals(file_path, lead_pack_path):
            continue

        lead_pack_read_attempted = True
        if result_text and is_error_text(result_text):
            continue
        if not isinstance(structured, dict):
            continue
        file_obj = structured.get("file")
        if not isinstance(file_obj, dict):
            continue
        result_file = file_obj.get("filePath")
        if isinstance(result_file, str) and path_equals(result_file, lead_pack_path):
            lead_pack_read_success = True

    result_event_is_error = bool(result_event.get("is_error")) if isinstance(result_event, dict) else False

    return {
        "lead_pack_read_attempted": lead_pack_read_attempted,
        "lead_pack_read_success": lead_pack_read_success,
        "tool_errors": tool_errors,
        "permission_errors": permission_errors,
        "result_event_is_error": result_event_is_error,
    }


def extract_final_payload(events: list[dict[str, Any]]) -> tuple[dict[str, Any] | None, str]:
    # Preferred: stream event with `type == "result"` and JSON payload.
    for event in reversed(events):
        if event.get("type") == "result":
            result = event.get("result")
            if isinstance(result, dict):
                return result, json.dumps(result, indent=2)
            if isinstance(result, str):
                try:
                    maybe_json = json.loads(result)
                    if isinstance(maybe_json, dict):
                        return maybe_json, json.dumps(maybe_json, indent=2)
                except json.JSONDecodeError:
                    return None, result
    # Fallback: gather textual assistant chunks.
    parts: list[str] = []
    for event in events:
        if event.get("type") in {"assistant", "message"}:
            msg = event.get("message", {})
            content = msg.get("content")
            if isinstance(content, str):
                parts.append(content)
            elif isinstance(content, list):
                for item in content:
                    if isinstance(item, dict) and isinstance(item.get("text"), str):
                        parts.append(item["text"])
    text = "\n".join(p.strip() for p in parts if p.strip()).strip()
    if text:
        try:
            maybe_json = json.loads(text)
            if isinstance(maybe_json, dict):
                return maybe_json, json.dumps(maybe_json, indent=2)
        except json.JSONDecodeError:
            pass
    return None, text


def extract_result_event(events: list[dict[str, Any]]) -> dict[str, Any] | None:
    for event in reversed(events):
        if event.get("type") == "result":
            return event
    return None


def extract_resolved_model(events: list[dict[str, Any]]) -> str | None:
    for event in events:
        if event.get("type") == "system" and event.get("subtype") == "init":
            model = event.get("model")
            if isinstance(model, str) and model.strip():
                return model

    for event in events:
        message = event.get("message")
        if not isinstance(message, dict):
            continue
        model = message.get("model")
        if isinstance(model, str) and model.strip():
            return model

    return None


def main() -> int:
    args = parse_args()

    repo_path = Path(args.repo).resolve()
    lead_pack_path = Path(args.lead_pack).resolve()
    results_root = Path(args.results_root).resolve()
    system_prompt_path = Path(args.system_prompt).resolve()
    user_template_path = Path(args.user_template).resolve()
    schema_path = Path(args.schema).resolve()

    if not repo_path.exists():
        raise SystemExit(f"repo path does not exist: {repo_path}")
    if not lead_pack_path.exists():
        raise SystemExit(f"lead pack does not exist: {lead_pack_path}")
    for path in (system_prompt_path, user_template_path, schema_path):
        if not path.exists():
            raise SystemExit(f"required file missing: {path}")

    lead_pack = json.loads(read_text(lead_pack_path))
    system_prompt_text = read_text(system_prompt_path)
    user_template_text = read_text(user_template_path)
    schema_text = read_text(schema_path)

    lead_summary = build_lead_summary(lead_pack, max_files=args.max_summary_files)
    user_prompt_text = render_user_prompt(
        user_template_text,
        repo_path=repo_path,
        experiment_id=args.experiment_id,
        lead_pack_path=lead_pack_path,
        lead_summary=lead_summary,
    )

    paths = prepare_paths(results_root=results_root, experiment_id=args.experiment_id)
    paths.run_dir.mkdir(parents=True, exist_ok=True)
    started_at = utc_now_iso()

    write_text(paths.system_prompt_copy, system_prompt_text)
    write_text(paths.user_prompt_copy, user_prompt_text)
    write_text(paths.rendered_summary, lead_summary)

    command = [
        "claude",
        "-p",
        "--model",
        args.model,
        "--verbose",
        "--output-format",
        "stream-json",
        "--include-partial-messages",
        "--add-dir",
        str(lead_pack_path.parent),
        "--system-prompt",
        system_prompt_text,
        "--json-schema",
        schema_text,
        "--tools",
        args.tools,
    ]

    write_text(paths.command_txt, " ".join(shlex.quote(c) for c in command) + "\n")
    write_json(
        paths.metadata_json,
        {
            "started_at": started_at,
            "experiment_id": args.experiment_id,
            "repo": str(repo_path),
            "lead_pack": str(lead_pack_path),
            "system_prompt": str(system_prompt_path),
            "user_template": str(user_template_path),
            "schema": str(schema_path),
            "tools": args.tools,
            "model": args.model,
            "allow_tool_errors": args.allow_tool_errors,
            "allow_missing_lead_pack_read": args.allow_missing_lead_pack_read,
            "allow_auth_failure": args.allow_auth_failure,
        },
    )

    proc = subprocess.run(
        command,
        input=user_prompt_text,
        text=True,
        capture_output=True,
        cwd=str(repo_path),
    )

    write_text(paths.stream_jsonl, proc.stdout or "")
    write_text(paths.stderr_log, proc.stderr or "")

    events = extract_stream_events(paths.stream_jsonl)
    tool_calls, assistant_texts, event_counts = collect_human_activity(events)
    result_event = extract_result_event(events)
    resolved_model = extract_resolved_model(events)
    quality = evaluate_run_quality(
        tool_calls=tool_calls,
        lead_pack_path=lead_pack_path,
        result_event=result_event,
    )
    final_json, final_text = extract_final_payload(events)

    if final_json is not None:
        write_json(paths.final_json, final_json)
    write_text(paths.final_text, final_text + ("\n" if final_text and not final_text.endswith("\n") else ""))

    log_lines: list[str] = []
    log_lines.append("# Claude Activity Log")
    log_lines.append("")
    log_lines.append(f"- Timestamp: {utc_now_iso()}")
    log_lines.append(f"- Experiment: `{args.experiment_id}`")
    log_lines.append(f"- Repo: `{repo_path}`")
    log_lines.append(f"- Requested model: `{args.model}`")
    log_lines.append(f"- Resolved model: `{resolved_model or '<unknown>'}`")
    log_lines.append(f"- Exit code: `{proc.returncode}`")
    log_lines.append("")
    log_lines.append("## Inputs")
    log_lines.append(f"- System prompt copy: `{paths.system_prompt_copy}`")
    log_lines.append(f"- User prompt copy: `{paths.user_prompt_copy}`")
    log_lines.append(f"- Lead summary copy: `{paths.rendered_summary}`")
    log_lines.append(f"- Lead pack: `{lead_pack_path}`")
    log_lines.append("")
    log_lines.append("## Command")
    log_lines.append("```bash")
    log_lines.append(read_text(paths.command_txt).rstrip())
    log_lines.append("```")
    log_lines.append("")
    log_lines.append("## Event Counts")
    if event_counts:
        for key in sorted(event_counts):
            log_lines.append(f"- `{key}`: {event_counts[key]}")
    else:
        log_lines.append("(no events emitted)")
    log_lines.append("")
    log_lines.append("## Tool Activity")
    if tool_calls:
        for idx, call in enumerate(tool_calls, start=1):
            call_name = call.get("name", "<unknown>")
            call_input = summarize_tool_input(call_name, call.get("input"), repo_path)
            log_lines.append(f"{idx}. `{call_name}` ({call_input})")
            result_data = call.get("result")
            if isinstance(result_data, dict):
                for entry in summarize_tool_result(call_name, result_data, repo_path):
                    log_lines.append(f"   {entry}")
            else:
                log_lines.append("   (no result captured)")
    else:
        log_lines.append("(no tool activity)")
    log_lines.append("")
    log_lines.append("## Assistant Narrative (Non-Thinking)")
    if assistant_texts:
        for idx, text in enumerate(assistant_texts, start=1):
            log_lines.append(f"### Message {idx}")
            log_lines.append("")
            log_lines.append(text)
            log_lines.append("")
    else:
        log_lines.append("(no non-thinking assistant messages captured)")
    log_lines.append("")
    log_lines.append("## STDERR")
    stderr_text = read_text(paths.stderr_log).strip()
    if stderr_text:
        log_lines.append("```text")
        log_lines.append(stderr_text)
        log_lines.append("```")
    else:
        log_lines.append("(empty)")
    log_lines.append("")
    log_lines.append("## Result Event")
    if isinstance(result_event, dict):
        subtype = result_event.get("subtype", "<unknown>")
        is_error = result_event.get("is_error")
        num_turns = result_event.get("num_turns")
        duration_ms = result_event.get("duration_ms")
        log_lines.append(
            f"- subtype: `{subtype}` | is_error: `{is_error}` | turns: `{num_turns}` | duration_ms: `{duration_ms}`"
        )
    else:
        log_lines.append("(none)")
    log_lines.append("")
    log_lines.append("## Run Quality Gates")
    log_lines.append(f"- lead_pack_read_attempted: `{quality['lead_pack_read_attempted']}`")
    log_lines.append(f"- lead_pack_read_success: `{quality['lead_pack_read_success']}`")
    log_lines.append(f"- tool_error_count: `{len(quality['tool_errors'])}`")
    log_lines.append(f"- permission_error_count: `{len(quality['permission_errors'])}`")
    log_lines.append(f"- result_event_is_error: `{quality['result_event_is_error']}`")
    if quality["tool_errors"]:
        log_lines.append("- tool_errors:")
        for item in quality["tool_errors"]:
            log_lines.append(f"  - {item}")
    log_lines.append("")
    log_lines.append("## Final Payload")
    if final_json is not None:
        log_lines.append("```json")
        log_lines.append(json.dumps(final_json, indent=2))
        log_lines.append("```")
    elif final_text.strip():
        log_lines.append("```text")
        log_lines.append(final_text.strip())
        log_lines.append("```")
    else:
        log_lines.append("(empty)")
    log_lines.append("")
    log_lines.append("## Raw Stream")
    log_lines.append(f"- `{paths.stream_jsonl}`")

    write_text(paths.activity_log_md, "\n".join(log_lines).rstrip() + "\n")
    write_json(paths.quality_json, quality)
    write_json(
        paths.metadata_json,
        {
            "started_at": started_at,
            "ended_at": utc_now_iso(),
            "experiment_id": args.experiment_id,
            "repo": str(repo_path),
            "lead_pack": str(lead_pack_path),
            "system_prompt": str(system_prompt_path),
            "user_template": str(user_template_path),
            "schema": str(schema_path),
            "tools": args.tools,
            "model": args.model,
            "resolved_model": resolved_model,
            "allow_tool_errors": args.allow_tool_errors,
            "allow_missing_lead_pack_read": args.allow_missing_lead_pack_read,
            "allow_auth_failure": args.allow_auth_failure,
        },
    )

    print(f"experiment_id={args.experiment_id}")
    print(f"run_dir={paths.run_dir}")
    print(f"exit_code={proc.returncode}")
    print(f"activity_log={paths.activity_log_md}")
    print(f"quality={paths.quality_json}")

    if proc.returncode != 0 and not args.allow_auth_failure:
        return proc.returncode
    if quality["result_event_is_error"] and not args.allow_auth_failure:
        return 2
    if quality["tool_errors"] and not args.allow_tool_errors:
        return 3
    if not quality["lead_pack_read_success"] and not args.allow_missing_lead_pack_read:
        return 4
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
