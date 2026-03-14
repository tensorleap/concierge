#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import re
import shlex
import subprocess
import sys
import textwrap
import time
import uuid
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any, Iterable

from pty_driver import PTYDriver


DEFAULT_MAX_ITERATIONS = 30
DEFAULT_MAX_IDLE_TURNS = 5
DEFAULT_MAX_RUNTIME_SECONDS = 60 * 60
DEFAULT_READ_QUIET_SECONDS = 0.35
DEFAULT_READ_TIMEOUT_SECONDS = 2.0
DEFAULT_SETTLE_TIMEOUT_SECONDS = 20.0
DEFAULT_TRANSCRIPT_TAIL_CHARS = 16000
DEFAULT_LATEST_OUTPUT_CHARS = 6000
DEFAULT_CODEX_TIMEOUT_SECONDS = 300.0
ANSI_ESCAPE_RE = re.compile(r"\x1B[@-_][0-?]*[ -/]*[@-~]")
PROMPT_LINE_RE = re.compile(
    r"(\[[^\]]+\]\s*:?\s*$|(?:continue|apply|type|enter|select|choose|confirm|approve|input).*[?:]\s*$|you\s*>\s*.*$)",
    re.IGNORECASE,
)
QA_DIR = Path(__file__).resolve().parent
REPO_ROOT = QA_DIR.parent
PROMPTS_DIR = QA_DIR / "prompts"
EXTERNAL_COMMAND_TIMEOUT_FLOOR_SECONDS = 1.0


@dataclass
class LoopDirective:
    action: str
    input_text: str
    loop_state: str
    summary: str
    issues: list[str]
    next_focus: str

    @classmethod
    def from_dict(cls, payload: dict[str, Any]) -> "LoopDirective":
        return cls(
            action=str(payload.get("action", "")).strip(),
            input_text=str(payload.get("input_text", "")),
            loop_state=str(payload.get("loop_state", "")).strip(),
            summary=str(payload.get("summary", "")).strip(),
            issues=[str(item).strip() for item in payload.get("issues", []) if str(item).strip()],
            next_focus=str(payload.get("next_focus", "")).strip(),
        )


@dataclass
class QAReport:
    title: str
    overall_outcome: str
    loop_state: str
    integration_progress: str
    ux_clarity: list[str]
    product_issues: list[str]
    agent_interaction_issues: list[str]
    suggestions: list[str]
    notable_moments: list[str]


@dataclass
class LoopConfig:
    artifacts_root: Path
    command: list[str]
    command_cwd: Path
    codex_command: str
    codex_model: str | None
    codex_timeout_seconds: float
    max_iterations: int
    max_idle_turns: int
    max_runtime_seconds: int
    read_quiet_seconds: float
    read_timeout_seconds: float
    settle_timeout_seconds: float
    transcript_tail_chars: int
    latest_output_chars: int
    fixture_post_path: Path | None


@dataclass
class RunPaths:
    run_id: str
    run_dir: Path
    codex_dir: Path
    summary_json: Path
    turns_jsonl: Path
    terminal_raw: Path
    terminal_clean: Path
    interaction_log: Path
    report_json: Path
    report_markdown: Path


class CodexInvocationError(RuntimeError):
    pass


class CodexClient:
    def __init__(
        self,
        *,
        workspace_root: Path,
        artifacts_root: Path,
        command: str = "codex",
        model: str | None = None,
        timeout_seconds: float = DEFAULT_CODEX_TIMEOUT_SECONDS,
    ) -> None:
        self.workspace_root = workspace_root
        self.artifacts_root = artifacts_root
        self.command = command
        self.model = model
        self.timeout_seconds = timeout_seconds

    def run_structured(
        self,
        *,
        prompt: str,
        schema_path: Path,
        output_path: Path,
        event_log_path: Path,
        stderr_log_path: Path,
        add_dirs: Iterable[Path] = (),
    ) -> dict[str, Any]:
        cmd = [
            *shlex.split(self.command),
            "exec",
            "--json",
            "--ephemeral",
            "--skip-git-repo-check",
            "--sandbox",
            "read-only",
            "--color",
            "never",
            "--output-schema",
            str(schema_path),
            "-o",
            str(output_path),
            "-C",
            str(self.workspace_root),
        ]
        if self.model:
            cmd.extend(["--model", self.model])
        for path in unique_paths([self.artifacts_root, *add_dirs]):
            cmd.extend(["--add-dir", str(path)])
        cmd.append("-")

        try:
            completed = subprocess.run(
                cmd,
                input=prompt,
                text=True,
                capture_output=True,
                cwd=str(self.workspace_root),
                check=False,
                timeout=self.timeout_seconds,
            )
            stdout = completed.stdout
            stderr = completed.stderr
            returncode = completed.returncode
        except subprocess.TimeoutExpired as exc:
            stdout = exc.stdout or ""
            stderr = exc.stderr or ""
            returncode = None
        stdout = coerce_text(stdout)
        stderr = coerce_text(stderr)
        event_log_path.parent.mkdir(parents=True, exist_ok=True)
        event_log_path.write_text(stdout, encoding="utf-8")
        stderr_log_path.write_text(stderr, encoding="utf-8")

        if returncode is None:
            raise CodexInvocationError(
                f"codex exec timed out after {self.timeout_seconds:.1f}s; see {stderr_log_path}"
            )
        if returncode != 0:
            raise CodexInvocationError(
                f"codex exec failed with exit code {returncode}; see {stderr_log_path}"
            )
        try:
            return json.loads(output_path.read_text(encoding="utf-8"))
        except FileNotFoundError as exc:
            raise CodexInvocationError(f"codex did not write {output_path}") from exc
        except json.JSONDecodeError as exc:
            raise CodexInvocationError(f"codex wrote invalid JSON to {output_path}: {exc}") from exc


class SupervisorLoop:
    def __init__(
        self,
        *,
        config: LoopConfig,
        codex_client: CodexClient,
        driver: PTYDriver | None = None,
        role_prompt: str,
        nudge_prompt: str,
    ) -> None:
        self.config = config
        self.codex_client = codex_client
        self.driver = driver or PTYDriver()
        self.role_prompt = role_prompt.strip()
        self.nudge_prompt = nudge_prompt.strip()

    def run(self, *, run_id: str | None = None) -> dict[str, Any]:
        run_id = run_id or default_run_id()
        paths = prepare_run_paths(self.config.artifacts_root, run_id)
        started_at = time.time()
        started_monotonic = time.monotonic()
        self.driver.start(self.config.command, cwd=self.config.command_cwd, env=os.environ.copy())

        transcript_raw = ""
        transcript_clean = ""
        turns: list[dict[str, Any]] = []
        ensure_run_artifact_files(paths)
        latest_output = visible_terminal_output(self._read_until_actionable(patience=True))
        transcript_raw, transcript_clean = append_terminal_output(paths, transcript_raw, transcript_clean, latest_output)

        loop_state = "CONTINUE"
        stop_reason = ""
        idle_turns = 0
        blind_first_active = True

        try:
            for iteration in range(1, self.config.max_iterations + 1):
                buffered_output = self.driver.drain(max_bytes=262144)
                if buffered_output:
                    latest_output = visible_terminal_output(
                        self._read_until_actionable(initial_output=buffered_output, patience=True)
                    )
                    transcript_raw, transcript_clean = append_terminal_output(
                        paths,
                        transcript_raw,
                        transcript_clean,
                        latest_output,
                    )
                else:
                    latest_output = ""

                elapsed = int(time.monotonic() - started_monotonic)
                released_blind_first = not blind_first_active
                stalled = idle_turns >= 2
                if blind_first_active and stalled and self.config.fixture_post_path is not None and transcript_clean.strip():
                    blind_first_active = False
                    released_blind_first = True
                last_action = ""
                if turns:
                    last_action = str(turns[-1].get("directive", {}).get("action", "")).strip()
                if (
                    not latest_output
                    and self.driver.is_running()
                    and last_action == "WAIT"
                    and not terminal_requires_input(transcript_clean)
                ):
                    latest_output = visible_terminal_output(self._read_until_actionable(patience=True))
                    transcript_raw, transcript_clean = append_terminal_output(
                        paths,
                        transcript_raw,
                        transcript_clean,
                        latest_output,
                    )
                    if latest_output:
                        idle_turns = 0
                    else:
                        idle_turns += 1
                    if int(time.monotonic() - started_monotonic) >= self.config.max_runtime_seconds:
                        loop_state = "STOP_DEADEND"
                        stop_reason = "runtime_limit"
                        break
                    if idle_turns >= self.config.max_idle_turns:
                        loop_state = "STOP_DEADEND"
                        stop_reason = "idle_limit"
                        break
                    continue

                try:
                    directive = self._request_control(
                        iteration=iteration,
                        paths=paths,
                        transcript_clean=transcript_clean,
                        latest_output=latest_output,
                        idle_turns=idle_turns,
                        elapsed_seconds=elapsed,
                        turns=turns,
                        blind_first_active=blind_first_active,
                        released_blind_first=released_blind_first,
                    )
                except CodexInvocationError as exc:
                    prompt_visible = terminal_requires_input(latest_output)
                    if last_action != "SEND_INPUT":
                        prompt_visible = prompt_visible or terminal_requires_input(transcript_clean)
                    if self.driver.is_running() and not prompt_visible:
                        append_jsonl(
                            paths.interaction_log,
                            {
                                "time": utc_now(),
                                "iteration": iteration,
                                "kind": "control_timeout_autowait",
                                "reason": str(exc),
                            },
                        )
                        latest_output = visible_terminal_output(self._read_until_actionable(patience=True))
                        transcript_raw, transcript_clean = append_terminal_output(
                            paths,
                            transcript_raw,
                            transcript_clean,
                            latest_output,
                        )
                        if latest_output:
                            idle_turns = 0
                        else:
                            idle_turns += 1
                        if int(time.monotonic() - started_monotonic) >= self.config.max_runtime_seconds:
                            loop_state = "STOP_DEADEND"
                            stop_reason = "runtime_limit"
                            break
                        if idle_turns >= self.config.max_idle_turns:
                            loop_state = "STOP_DEADEND"
                            stop_reason = "idle_limit"
                            break
                        continue
                    append_jsonl(
                        paths.interaction_log,
                        {
                            "time": utc_now(),
                            "iteration": iteration,
                            "kind": "control_error",
                            "reason": str(exc),
                        },
                    )
                    loop_state = "STOP_DEADEND"
                    stop_reason = "codex_control_error"
                    break
                turn_record = {
                    "iteration": iteration,
                    "time": utc_now(),
                    "directive": asdict(directive),
                    "terminal_running": self.driver.is_running(),
                    "terminal_exit_code": self.driver.returncode,
                    "latest_output_chars": len(latest_output),
                    "idle_turns_before": idle_turns,
                }
                append_jsonl(paths.turns_jsonl, turn_record)
                turns.append(turn_record)
                print(
                    f"[qa-loop] turn {iteration}: {directive.action} {directive.loop_state} "
                    f"{directive.summary or directive.next_focus}",
                    flush=True,
                )

                if directive.action == "SEND_INPUT" and directive.input_text:
                    if not self.driver.is_running():
                        append_jsonl(
                            paths.interaction_log,
                            {
                                "time": utc_now(),
                                "iteration": iteration,
                                "kind": "input_skipped",
                                "text": directive.input_text,
                                "reason": "target process already exited",
                            },
                        )
                        loop_state = "STOP_REPORT" if self.driver.returncode is not None else "STOP_DEADEND"
                        stop_reason = "process_exit_before_input"
                        break
                    try:
                        self.driver.send(directive.input_text, append_newline=True)
                    except OSError as exc:
                        append_jsonl(
                            paths.interaction_log,
                            {
                                "time": utc_now(),
                                "iteration": iteration,
                                "kind": "input_failed",
                                "text": directive.input_text,
                                "reason": str(exc),
                            },
                        )
                        loop_state = "STOP_REPORT" if self.driver.returncode is not None else "STOP_DEADEND"
                        stop_reason = "pty_send_failed"
                        break
                    append_jsonl(
                        paths.interaction_log,
                        {
                            "time": utc_now(),
                            "iteration": iteration,
                            "kind": "input",
                            "text": directive.input_text,
                        },
                    )
                    idle_turns = 0
                elif directive.action == "RUN_COMMAND" and directive.input_text:
                    command_result = self._run_external_command(
                        command_text=directive.input_text,
                        iteration=iteration,
                        paths=paths,
                        remaining_runtime_seconds=max(
                            self.config.max_runtime_seconds - (time.monotonic() - started_monotonic),
                            EXTERNAL_COMMAND_TIMEOUT_FLOOR_SECONDS,
                        ),
                    )
                    transcript_raw, transcript_clean = append_terminal_output(
                        paths,
                        transcript_raw,
                        transcript_clean,
                        command_result["transcript"],
                    )
                    idle_turns = 0
                    if not self.driver.is_running() and command_result["returncode"] == 0:
                        self.driver.stop()
                        append_jsonl(
                            paths.interaction_log,
                            {
                                "time": utc_now(),
                                "iteration": iteration,
                                "kind": "process_restart",
                                "command": self.config.command,
                            },
                        )
                        self.driver.start(self.config.command, cwd=self.config.command_cwd, env=os.environ.copy())
                    latest_output = visible_terminal_output(self._read_until_actionable(patience=True))
                    transcript_raw, transcript_clean = append_terminal_output(paths, transcript_raw, transcript_clean, latest_output)
                elif latest_output:
                    idle_turns = 0
                else:
                    idle_turns += 1

                latest_output = visible_terminal_output(
                    self._read_until_actionable(patience=directive.action == "SEND_INPUT")
                )
                transcript_raw, transcript_clean = append_terminal_output(paths, transcript_raw, transcript_clean, latest_output)

                loop_state = directive.loop_state
                if loop_state != "CONTINUE":
                    stop_reason = "codex_stop"
                    break
                if int(time.monotonic() - started_monotonic) >= self.config.max_runtime_seconds:
                    loop_state = "STOP_DEADEND"
                    stop_reason = "runtime_limit"
                    break
                if idle_turns >= self.config.max_idle_turns:
                    loop_state = "STOP_DEADEND"
                    stop_reason = "idle_limit"
                    break
                if not self.driver.is_running() and not latest_output:
                    loop_state = "STOP_REPORT"
                    stop_reason = "process_exit"
                    break
            else:
                loop_state = "STOP_DEADEND"
                stop_reason = "iteration_limit"
        finally:
            trailing_output = self.driver.read_until_quiet(
                quiet_period=self.config.read_quiet_seconds,
                hard_timeout=0.5,
            )
            if trailing_output:
                transcript_raw, transcript_clean = append_terminal_output(
                    paths,
                    transcript_raw,
                    transcript_clean,
                    trailing_output,
                )
            exit_code = self.driver.stop()
            trailing_output = self.driver.drain(max_bytes=262144)
            if trailing_output:
                transcript_raw, transcript_clean = append_terminal_output(
                    paths,
                    transcript_raw,
                    transcript_clean,
                    trailing_output,
                )

        if not stop_reason:
            stop_reason = "codex_stop"

        summary = {
            "run_id": run_id,
            "started_at": utc_from_timestamp(started_at),
            "finished_at": utc_now(),
            "command": self.config.command,
            "command_cwd": str(self.config.command_cwd),
            "artifacts_root": str(self.config.artifacts_root),
            "loop_state": loop_state,
            "stop_reason": stop_reason,
            "iterations_completed": len(turns),
            "idle_turns": idle_turns,
            "blind_first_released": not blind_first_active,
            "fixture_post_path": str(self.config.fixture_post_path) if self.config.fixture_post_path else "",
            "terminal_exit_code": exit_code,
            "paths": {
                "summary_json": str(paths.summary_json),
                "turns_jsonl": str(paths.turns_jsonl),
                "terminal_raw": str(paths.terminal_raw),
                "terminal_clean": str(paths.terminal_clean),
                "interaction_log": str(paths.interaction_log),
                "report_json": str(paths.report_json),
                "report_markdown": str(paths.report_markdown),
            },
        }
        paths.summary_json.write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")

        try:
            report = self._request_report(paths=paths, summary=summary, loop_state=loop_state)
        except CodexInvocationError as exc:
            append_jsonl(
                paths.interaction_log,
                {
                    "time": utc_now(),
                    "iteration": len(turns),
                    "kind": "report_fallback",
                    "reason": str(exc),
                },
            )
            report = fallback_report(summary=summary, turns=turns, error=str(exc))
        paths.report_json.write_text(json.dumps(asdict(report), indent=2) + "\n", encoding="utf-8")
        paths.report_markdown.write_text(render_markdown_report(run_id, summary, report), encoding="utf-8")

        return {
            "loop_state": loop_state,
            "stop_reason": stop_reason,
            "summary_path": str(paths.summary_json),
            "report_json_path": str(paths.report_json),
            "report_markdown_path": str(paths.report_markdown),
            "terminal_clean_path": str(paths.terminal_clean),
            "turns_path": str(paths.turns_jsonl),
        }

    def _read_until_actionable(self, *, initial_output: str = "", patience: bool = False) -> str:
        chunks: list[str] = [initial_output] if initial_output else []
        deadline = time.monotonic() + (
            self.config.settle_timeout_seconds if patience else self.config.read_timeout_seconds
        )
        while time.monotonic() < deadline:
            if terminal_requires_input("".join(chunks)):
                break
            if not self.driver.is_running() and chunks:
                break
            remaining = max(deadline - time.monotonic(), 0.0)
            if remaining <= 0:
                break
            chunk = self.driver.read_until_quiet(
                quiet_period=self.config.read_quiet_seconds,
                hard_timeout=min(self.config.read_timeout_seconds, remaining),
            )
            if chunk:
                chunks.append(chunk)
                continue
            if not patience or not self.driver.is_running():
                break
        return "".join(chunks)

    def _request_control(
        self,
        *,
        iteration: int,
        paths: RunPaths,
        transcript_clean: str,
        latest_output: str,
        idle_turns: int,
        elapsed_seconds: int,
        turns: list[dict[str, Any]],
        blind_first_active: bool,
        released_blind_first: bool,
    ) -> LoopDirective:
        context = {
            "iteration": iteration,
            "max_iterations": self.config.max_iterations,
            "idle_turns": idle_turns,
            "max_idle_turns": self.config.max_idle_turns,
            "elapsed_seconds": elapsed_seconds,
            "max_runtime_seconds": self.config.max_runtime_seconds,
            "terminal_running": self.driver.is_running(),
            "terminal_exit_code": self.driver.returncode,
            "command": self.config.command,
            "command_cwd": str(self.config.command_cwd),
            "blind_first_active": blind_first_active,
            "post_fixture_path_available": bool(self.config.fixture_post_path and not blind_first_active),
            "post_fixture_path": str(self.config.fixture_post_path) if self.config.fixture_post_path and not blind_first_active else "",
            "recent_turn_summaries": [
                {
                    "iteration": item["iteration"],
                    "action": item["directive"]["action"],
                    "loop_state": item["directive"]["loop_state"],
                    "summary": item["directive"]["summary"],
                }
                for item in turns[-5:]
            ],
            "latest_output": tail_text(normalize_terminal_text(latest_output), self.config.latest_output_chars),
            "transcript_tail": tail_text(transcript_clean, self.config.transcript_tail_chars),
        }
        prompt = self.role_prompt + "\n\n"
        if released_blind_first and self.config.fixture_post_path:
            prompt += self.nudge_prompt + "\n\n"
        prompt += "Return only JSON matching the provided schema.\n\n"
        prompt += "Session context:\n```json\n"
        prompt += json.dumps(context, indent=2)
        prompt += "\n```\n"

        turn_base = paths.codex_dir / f"turn-{iteration:03d}"
        payload = self.codex_client.run_structured(
            prompt=prompt,
            schema_path=PROMPTS_DIR / "control_schema.json",
            output_path=turn_base.with_suffix(".output.json"),
            event_log_path=turn_base.with_suffix(".jsonl"),
            stderr_log_path=turn_base.with_suffix(".stderr.log"),
            add_dirs=codex_add_dirs(self.config.fixture_post_path if not blind_first_active else None),
        )
        return LoopDirective.from_dict(payload)

    def _run_external_command(
        self,
        *,
        command_text: str,
        iteration: int,
        paths: RunPaths,
        remaining_runtime_seconds: float,
    ) -> dict[str, Any]:
        append_jsonl(
            paths.interaction_log,
            {
                "time": utc_now(),
                "iteration": iteration,
                "kind": "command",
                "text": command_text,
                "cwd": str(self.config.command_cwd),
            },
        )
        try:
            completed = subprocess.run(
                ["bash", "-lc", command_text],
                cwd=str(self.config.command_cwd),
                env=os.environ.copy(),
                text=True,
                capture_output=True,
                check=False,
                timeout=max(remaining_runtime_seconds, EXTERNAL_COMMAND_TIMEOUT_FLOOR_SECONDS),
            )
            returncode = completed.returncode
            stdout = coerce_text(completed.stdout)
            stderr = coerce_text(completed.stderr)
        except subprocess.TimeoutExpired as exc:
            returncode = 124
            stdout = coerce_text(exc.stdout or "")
            stderr = coerce_text(exc.stderr or "")

        transcript_lines = [
            f"[qa-loop] external command in {self.config.command_cwd}:",
            f"$ {command_text}",
        ]
        if stdout.strip():
            transcript_lines.extend(["[stdout]", stdout.rstrip()])
        if stderr.strip():
            transcript_lines.extend(["[stderr]", stderr.rstrip()])
        transcript_lines.append(f"[qa-loop] external command exit code: {returncode}")
        transcript = "\n".join(transcript_lines) + "\n"

        append_jsonl(
            paths.interaction_log,
            {
                "time": utc_now(),
                "iteration": iteration,
                "kind": "command_result",
                "text": command_text,
                "cwd": str(self.config.command_cwd),
                "returncode": returncode,
            },
        )

        return {"returncode": returncode, "stdout": stdout, "stderr": stderr, "transcript": transcript}

    def _request_report(self, *, paths: RunPaths, summary: dict[str, Any], loop_state: str) -> QAReport:
        prompt = textwrap.dedent(
            f"""
            You are writing the final qualitative QA report for a Concierge terminal session.
            Use the saved artifacts instead of asking the user questions.

            Review these files if needed:
            - Summary JSON: {paths.summary_json}
            - Turn log: {paths.turns_jsonl}
            - Clean terminal transcript: {paths.terminal_clean}
            - Raw terminal transcript: {paths.terminal_raw}
            - Interaction log: {paths.interaction_log}

            Focus on integration progress, UX clarity, product issues, agent interaction issues, and concrete suggestions.
            Return only JSON matching the provided schema.

            Run summary:
            ```json
            {json.dumps(summary, indent=2)}
            ```

            Final loop state: {loop_state}
            """
        ).strip()
        report_base = paths.codex_dir / "final-report"
        payload = self.codex_client.run_structured(
            prompt=prompt,
            schema_path=PROMPTS_DIR / "report_schema.json",
            output_path=report_base.with_suffix(".output.json"),
            event_log_path=report_base.with_suffix(".jsonl"),
            stderr_log_path=report_base.with_suffix(".stderr.log"),
            add_dirs=codex_add_dirs(self.config.fixture_post_path if summary.get("blind_first_released") else None),
        )
        return QAReport(
            title=str(payload.get("title", "")).strip(),
            overall_outcome=str(payload.get("overall_outcome", "")).strip(),
            loop_state=str(payload.get("loop_state", "")).strip(),
            integration_progress=str(payload.get("integration_progress", "")).strip(),
            ux_clarity=clean_string_list(payload.get("ux_clarity", [])),
            product_issues=clean_string_list(payload.get("product_issues", [])),
            agent_interaction_issues=clean_string_list(payload.get("agent_interaction_issues", [])),
            suggestions=clean_string_list(payload.get("suggestions", [])),
            notable_moments=clean_string_list(payload.get("notable_moments", [])),
        )


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run Concierge inside a PTY and let Codex drive it like a QA engineer.",
    )
    parser.add_argument(
        "--artifacts-root",
        default=str(QA_DIR),
        help="Directory where runs/, transcripts/, and reports/ will be written. Defaults to QA/.",
    )
    parser.add_argument(
        "--command-cwd",
        default=os.getcwd(),
        help="Working directory for the Concierge command under test.",
    )
    parser.add_argument("--codex-command", default=os.environ.get("CODEX_BIN", "codex"))
    parser.add_argument("--model", default=None)
    parser.add_argument("--codex-timeout-seconds", type=float, default=DEFAULT_CODEX_TIMEOUT_SECONDS)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--max-iterations", type=int, default=DEFAULT_MAX_ITERATIONS)
    parser.add_argument("--max-idle-turns", type=int, default=DEFAULT_MAX_IDLE_TURNS)
    parser.add_argument("--max-runtime-seconds", type=int, default=DEFAULT_MAX_RUNTIME_SECONDS)
    parser.add_argument("--read-quiet-seconds", type=float, default=DEFAULT_READ_QUIET_SECONDS)
    parser.add_argument("--read-timeout-seconds", type=float, default=DEFAULT_READ_TIMEOUT_SECONDS)
    parser.add_argument("--settle-timeout-seconds", type=float, default=DEFAULT_SETTLE_TIMEOUT_SECONDS)
    parser.add_argument("--transcript-tail-chars", type=int, default=DEFAULT_TRANSCRIPT_TAIL_CHARS)
    parser.add_argument("--latest-output-chars", type=int, default=DEFAULT_LATEST_OUTPUT_CHARS)
    parser.add_argument("--fixture-post-path", default=None)
    parser.add_argument(
        "command",
        nargs=argparse.REMAINDER,
        help="Command to run in the PTY. Prefix with -- to stop qa_loop option parsing.",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    artifacts_root = Path(args.artifacts_root).resolve()
    command_cwd = Path(args.command_cwd).resolve()
    fixture_post_path = Path(args.fixture_post_path).resolve() if args.fixture_post_path else None
    command = list(args.command)
    if command and command[0] == "--":
        command = command[1:]
    if not command:
        command = default_concierge_command(artifacts_root)

    config = LoopConfig(
        artifacts_root=artifacts_root,
        command=command,
        command_cwd=command_cwd,
        codex_command=args.codex_command,
        codex_model=args.model,
        codex_timeout_seconds=args.codex_timeout_seconds,
        max_iterations=args.max_iterations,
        max_idle_turns=args.max_idle_turns,
        max_runtime_seconds=args.max_runtime_seconds,
        read_quiet_seconds=args.read_quiet_seconds,
        read_timeout_seconds=args.read_timeout_seconds,
        settle_timeout_seconds=args.settle_timeout_seconds,
        transcript_tail_chars=args.transcript_tail_chars,
        latest_output_chars=args.latest_output_chars,
        fixture_post_path=fixture_post_path,
    )
    role_prompt = (PROMPTS_DIR / "role_prompt.md").read_text(encoding="utf-8")
    nudge_prompt = (PROMPTS_DIR / "nudge_prompt.md").read_text(encoding="utf-8")
    codex_workspace = artifacts_root
    client = CodexClient(
        workspace_root=codex_workspace,
        artifacts_root=artifacts_root,
        command=config.codex_command,
        model=config.codex_model,
        timeout_seconds=config.codex_timeout_seconds,
    )
    loop = SupervisorLoop(
        config=config,
        codex_client=client,
        role_prompt=role_prompt,
        nudge_prompt=nudge_prompt,
    )
    result = loop.run(run_id=args.run_id)
    print(f"[qa-loop] report: {result['report_markdown_path']}", flush=True)
    return exit_code_for_loop_state(result["loop_state"])


def default_run_id() -> str:
    return f"{time.strftime('%Y%m%dT%H%M%SZ', time.gmtime())}-{uuid.uuid4().hex[:8]}"


def prepare_run_paths(artifacts_root: Path, run_id: str) -> RunPaths:
    run_dir = artifacts_root / "runs" / run_id
    codex_dir = run_dir / "codex"
    transcripts_dir = artifacts_root / "transcripts"
    reports_dir = artifacts_root / "reports"
    for directory in (run_dir, codex_dir, transcripts_dir, reports_dir):
        directory.mkdir(parents=True, exist_ok=True)
    return RunPaths(
        run_id=run_id,
        run_dir=run_dir,
        codex_dir=codex_dir,
        summary_json=run_dir / "summary.json",
        turns_jsonl=run_dir / "turns.jsonl",
        terminal_raw=transcripts_dir / f"{run_id}.terminal.raw.txt",
        terminal_clean=transcripts_dir / f"{run_id}.terminal.txt",
        interaction_log=transcripts_dir / f"{run_id}.interaction.jsonl",
        report_json=reports_dir / f"{run_id}.json",
        report_markdown=reports_dir / f"{run_id}.md",
    )


def ensure_run_artifact_files(paths: RunPaths) -> None:
    for path in (paths.terminal_raw, paths.terminal_clean, paths.interaction_log):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.touch(exist_ok=True)


def append_terminal_output(paths: RunPaths, raw_text: str, clean_text: str, latest_output: str) -> tuple[str, str]:
    if not latest_output:
        return raw_text, clean_text
    raw_text += latest_output
    clean_chunk = normalize_terminal_text(latest_output)
    clean_text += clean_chunk
    paths.terminal_raw.parent.mkdir(parents=True, exist_ok=True)
    with paths.terminal_raw.open("a", encoding="utf-8") as handle:
        handle.write(latest_output)
    with paths.terminal_clean.open("a", encoding="utf-8") as handle:
        handle.write(clean_chunk)
    return raw_text, clean_text


def normalize_terminal_text(text: str) -> str:
    without_ansi = ANSI_ESCAPE_RE.sub("", text)
    return without_ansi.replace("\r\n", "\n").replace("\r", "\n")


def tail_text(text: str, limit: int) -> str:
    text = text.strip()
    if len(text) <= limit:
        return text
    clipped = len(text) - limit
    return f"[... clipped {clipped} chars ...]\n{text[-limit:]}"


def append_jsonl(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(payload) + "\n")


def unique_paths(paths: Iterable[Path]) -> list[Path]:
    seen: set[str] = set()
    ordered: list[Path] = []
    for path in paths:
        resolved = str(path.resolve())
        if resolved in seen:
            continue
        seen.add(resolved)
        ordered.append(Path(resolved))
    return ordered


def codex_add_dirs(fixture_post_path: Path | None) -> list[Path]:
    add_dirs: list[Path] = []
    if fixture_post_path is not None:
        add_dirs.append(fixture_post_path if fixture_post_path.is_dir() else fixture_post_path.parent)
    return add_dirs


def clean_string_list(values: Iterable[Any]) -> list[str]:
    cleaned: list[str] = []
    for value in values:
        item = str(value).strip()
        if item:
            cleaned.append(item)
    return cleaned


def coerce_text(value: str | bytes | None) -> str:
    if value is None:
        return ""
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    return value


def terminal_requires_input(text: str) -> bool:
    normalized = normalize_terminal_text(text)
    for line in reversed(normalized.splitlines()):
        candidate = line.strip()
        if not candidate:
            continue
        if PROMPT_LINE_RE.search(candidate):
            return True
        return candidate.endswith(":") or candidate.endswith("?")
    return False


def visible_terminal_output(text: str) -> str:
    return text if normalize_terminal_text(text).strip() else ""


def fallback_report(*, summary: dict[str, Any], turns: list[dict[str, Any]], error: str) -> QAReport:
    last_summary = ""
    notable_moments: list[str] = []
    product_issues: list[str] = []
    for turn in turns:
        directive = turn.get("directive", {})
        summary_text = str(directive.get("summary", "")).strip()
        if summary_text:
            last_summary = summary_text
        notable_moments.extend(clean_string_list(directive.get("issues", [])))
    if last_summary:
        notable_moments.append(f"Last control summary: {last_summary}")
    if summary.get("stop_reason"):
        product_issues.append(f"Run stopped with `{summary['stop_reason']}` while the QA loop was still active.")
    return QAReport(
        title="QA Loop Report (fallback)",
        overall_outcome=f"Fallback report generated after QA-loop report synthesis failed: {error}",
        loop_state=str(summary.get("loop_state", "")).strip(),
        integration_progress=last_summary or "The run ended before a synthesized QA summary was available.",
        ux_clarity=[],
        product_issues=product_issues,
        agent_interaction_issues=[f"Automatic report generation failed: {error}"],
        suggestions=["Inspect the saved transcript and Codex stderr log for the interrupted report step."],
        notable_moments=notable_moments,
    )


def render_markdown_report(run_id: str, summary: dict[str, Any], report: QAReport) -> str:
    lines = [
        f"# {report.title or 'QA Loop Report'}",
        "",
        f"- Run ID: `{run_id}`",
        f"- Outcome: {report.overall_outcome}",
        f"- Loop state: `{summary['loop_state']}`",
        f"- Stop reason: `{summary['stop_reason']}`",
        f"- Command cwd: `{summary['command_cwd']}`",
        "",
        "## Integration Progress",
        report.integration_progress or "No integration progress summary was provided.",
        "",
        "## UX Clarity",
        *render_bullets(report.ux_clarity),
        "",
        "## Product Issues",
        *render_bullets(report.product_issues),
        "",
        "## Agent Interaction Issues",
        *render_bullets(report.agent_interaction_issues),
        "",
        "## Suggestions",
        *render_bullets(report.suggestions),
        "",
        "## Notable Moments",
        *render_bullets(report.notable_moments),
        "",
    ]
    return "\n".join(lines).rstrip() + "\n"


def render_bullets(items: list[str]) -> list[str]:
    if not items:
        return ["- None recorded."]
    return [f"- {item}" for item in items]


def default_concierge_command(artifacts_root: Path) -> list[str]:
    _ = artifacts_root
    binary = REPO_ROOT / "bin" / "concierge"
    if binary.is_file() and os.access(binary, os.X_OK):
        return [str(binary), "run"]
    return ["go", "run", str(REPO_ROOT / "cmd" / "concierge"), "run"]


def exit_code_for_loop_state(loop_state: str) -> int:
    return {
        "STOP_REPORT": 0,
        "STOP_FIX": 2,
        "STOP_DEADEND": 3,
    }.get(loop_state, 1)


def utc_now() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def utc_from_timestamp(value: float) -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(value))


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        raise SystemExit(130)
