#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import re
import shlex
import subprocess
import sys
import threading
import textwrap
import time
import uuid
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any, Callable, Iterable, TextIO

from pty_driver import PTYDriver


DEFAULT_MAX_ITERATIONS = 50
DEFAULT_MAX_IDLE_TURNS = 5
DEFAULT_MAX_RUNTIME_SECONDS = 60 * 60
DEFAULT_READ_QUIET_SECONDS = 0.35
DEFAULT_READ_TIMEOUT_SECONDS = 2.0
DEFAULT_SETTLE_TIMEOUT_SECONDS = 20.0
DEFAULT_TRANSCRIPT_TAIL_CHARS = 16000
DEFAULT_LATEST_OUTPUT_CHARS = 6000
DEFAULT_CODEX_TIMEOUT_SECONDS = 300.0
DEFAULT_REPORT_TIMEOUT_SECONDS = 120.0
ANSI_ESCAPE_RE = re.compile(r"\x1B[@-_][0-?]*[ -/]*[@-~]")
JSON_CODE_FENCE_RE = re.compile(r"```(?:json)?\s*(.*?)\s*```", re.IGNORECASE | re.DOTALL)
PROMPT_LINE_RE = re.compile(
    r"(\[[^\]]+\]\s*:?\s*$|(?:continue|apply|type|enter|select|choose|confirm|approve|input).*[?:]\s*$|you\s*>\s*.*$)",
    re.IGNORECASE,
)
QA_DIR = Path(__file__).resolve().parent
REPO_ROOT = QA_DIR.parent
PROMPTS_DIR = QA_DIR / "prompts"
EXTERNAL_COMMAND_TIMEOUT_FLOOR_SECONDS = 1.0
EXPORTED_WORKSPACE_PATHS = (
    ".concierge",
    "leap.yaml",
    "leap_integration.py",
    "leap_binder.py",
    "leap_custom_test.py",
)
EXPORTED_REVIEW_PATHS = (
    "leap.yaml",
    "leap_integration.py",
    "leap_binder.py",
    "leap_custom_test.py",
)


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
        summary = str(payload.get("summary", "")).strip()
        if not summary:
            summary = str(payload.get("observation", "")).strip()
        return cls(
            action=str(payload.get("action", "")).strip(),
            input_text=str(payload.get("input_text", "")),
            loop_state=str(payload.get("loop_state", "")).strip(),
            summary=summary,
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
class IntegrationReview:
    status: str
    verdict: str
    functional_equivalence: str
    quality_assessment: str
    issues: list[str]
    confidence: str


@dataclass
class LoopConfig:
    artifacts_root: Path
    docker_bin: str
    host_cwd: Path
    container_name: str
    container_image: str | None
    command: list[str]
    command_cwd: str
    claude_command: str
    claude_model: str | None
    claude_timeout_seconds: float
    review_command: str
    review_model: str | None
    review_timeout_seconds: float
    max_iterations: int
    max_idle_turns: int
    max_runtime_seconds: int
    read_quiet_seconds: float
    read_timeout_seconds: float
    settle_timeout_seconds: float
    transcript_tail_chars: int
    latest_output_chars: int
    fixture_post_path: Path | None
    docker_snapshots_enabled: bool
    fixture_id: str
    guide_step: str
    ref_under_test: str
    checkpoint_key: str
    source_kind: str
    source_id: str
    runtime_prerequisites: list[dict[str, Any]] | tuple[dict[str, Any], ...] = ()


@dataclass
class RunPaths:
    run_id: str
    run_dir: Path
    claude_dir: Path
    docker_dir: Path
    summary_json: Path
    turns_jsonl: Path
    terminal_raw: Path
    terminal_clean: Path
    interaction_log: Path
    full_transcript: Path
    report_json: Path
    report_markdown: Path


class CodexInvocationError(RuntimeError):
    pass


class ClaudeInvocationError(RuntimeError):
    pass


class DockerInvocationError(RuntimeError):
    pass


class LiveIO:
    def __init__(self, *, transcript_path: Path) -> None:
        self.transcript_path = transcript_path
        self._lock = threading.Lock()

    def stdout(self, text: str) -> None:
        self._write(text, stream=sys.stdout)

    def stderr(self, text: str) -> None:
        self._write(text, stream=sys.stderr)

    def transcript_only(self, text: str) -> None:
        self._write(text, stream=None)

    def _write(self, text: str, *, stream: TextIO | None) -> None:
        if not text:
            return
        with self._lock:
            if stream is not None:
                stream.write(text)
                stream.flush()
            self.transcript_path.parent.mkdir(parents=True, exist_ok=True)
            with self.transcript_path.open("a", encoding="utf-8") as handle:
                handle.write(text)


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
        timeout_seconds: float | None = None,
        live_io: LiveIO | None = None,
        session_label: str = "codex",
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
        cmd.extend(["-", prompt])

        if live_io is not None:
            live_io.stdout(f"[qa-loop] starting {session_label}: {shlex.join(cmd)}\n")
            live_io.transcript_only(
                f"[qa-loop] --- {session_label} stdin begin ---\n"
                f"{prompt.rstrip()}\n"
                f"[qa-loop] --- {session_label} stdin end ---\n"
            )

        effective_timeout = self.timeout_seconds if timeout_seconds is None else timeout_seconds
        try:
            completed = run_streaming_subprocess(
                cmd=cmd,
                cwd=self.workspace_root,
                env=None,
                input_text=prompt,
                timeout_seconds=effective_timeout,
                live_io=live_io,
                stdout_prefix=f"[qa-loop][{session_label} stdout] ",
                stderr_prefix=f"[qa-loop][{session_label} stderr] ",
                stdout_formatter=lambda line, session_label=session_label: format_compat_stream_event(session_label, line),
                stderr_formatter=lambda line, session_label=session_label: format_codex_stderr_event(session_label, line),
            )
            stdout = completed["stdout"]
            stderr = completed["stderr"]
            returncode = completed["returncode"]
        except subprocess.TimeoutExpired:
            raise CodexInvocationError(
                f"codex exec timed out after {effective_timeout:.1f}s; see {stderr_log_path}"
            )
        event_log_path.parent.mkdir(parents=True, exist_ok=True)
        event_log_path.write_text(stdout, encoding="utf-8")
        stderr_log_path.write_text(stderr, encoding="utf-8")

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


class ClaudeClient:
    def __init__(
        self,
        *,
        workspace_root: Path,
        artifacts_root: Path,
        command: str = "claude",
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
        allowed_tools: str | None = "Read,Grep,Glob,LS",
        timeout_seconds: float | None = None,
        live_io: LiveIO | None = None,
        session_label: str = "claude",
    ) -> dict[str, Any]:
        if not command_looks_like_claude_cli(self.command):
            return self._run_compat_structured(
                prompt=prompt,
                schema_path=schema_path,
                output_path=output_path,
                event_log_path=event_log_path,
                stderr_log_path=stderr_log_path,
                add_dirs=add_dirs,
                timeout_seconds=timeout_seconds,
                live_io=live_io,
                session_label=session_label,
            )

        schema_text = schema_path.read_text(encoding="utf-8").strip()
        cmd = [
            *shlex.split(self.command),
            "--print",
            "--output-format",
            "json",
            "--json-schema",
            schema_text,
            "--permission-mode",
            "bypassPermissions",
            "--no-session-persistence",
        ]
        if allowed_tools:
            cmd.extend(["--allowedTools", allowed_tools])
        if self.model:
            cmd.extend(["--model", self.model])
        for path in unique_paths([self.artifacts_root, *add_dirs]):
            cmd.extend(["--add-dir", str(path)])

        if live_io is not None:
            live_io.stdout(f"[qa-loop] starting {session_label}: {shlex.join(cmd)}\n")
            live_io.transcript_only(
                f"[qa-loop] --- {session_label} prompt begin ---\n"
                f"{prompt.rstrip()}\n"
                f"[qa-loop] --- {session_label} prompt end ---\n"
            )

        effective_timeout = self.timeout_seconds if timeout_seconds is None else timeout_seconds
        try:
            completed = run_streaming_subprocess(
                cmd=cmd,
                cwd=self.workspace_root,
                env=None,
                timeout_seconds=effective_timeout,
                input_text=prompt,
                live_io=live_io,
                stdout_prefix=f"[qa-loop][{session_label} stdout] ",
                stderr_prefix=f"[qa-loop][{session_label} stderr] ",
                stdout_formatter=lambda line, session_label=session_label: format_claude_stream_event(session_label, line),
                stderr_formatter=lambda line, session_label=session_label: format_claude_stderr_event(session_label, line),
            )
            stdout = completed["stdout"]
            stderr = completed["stderr"]
            returncode = completed["returncode"]
        except subprocess.TimeoutExpired as exc:
            persist_stream_artifacts(
                event_log_path=event_log_path,
                stderr_log_path=stderr_log_path,
                stdout=exc.output,
                stderr=exc.stderr,
            )
            raise ClaudeInvocationError(
                f"claude --print timed out after {effective_timeout:.1f}s; see {stderr_log_path}"
            ) from exc
        persist_stream_artifacts(
            event_log_path=event_log_path,
            stderr_log_path=stderr_log_path,
            stdout=stdout,
            stderr=stderr,
        )

        if returncode != 0:
            raise ClaudeInvocationError(
                f"claude --print failed with exit code {returncode}; see {stderr_log_path}"
            )

        try:
            payload = extract_claude_structured_output(stdout)
        except ValueError as exc:
            raise ClaudeInvocationError(str(exc)) from exc

        output_path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
        return payload

    def _run_compat_structured(
        self,
        *,
        prompt: str,
        schema_path: Path,
        output_path: Path,
        event_log_path: Path,
        stderr_log_path: Path,
        add_dirs: Iterable[Path] = (),
        timeout_seconds: float | None = None,
        live_io: LiveIO | None = None,
        session_label: str = "claude",
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

        if live_io is not None:
            live_io.stdout(f"[qa-loop] starting {session_label}: {shlex.join(cmd)}\n")
            live_io.transcript_only(
                f"[qa-loop] --- {session_label} prompt begin ---\n"
                f"{prompt.rstrip()}\n"
                f"[qa-loop] --- {session_label} prompt end ---\n"
            )

        effective_timeout = self.timeout_seconds if timeout_seconds is None else timeout_seconds
        try:
            completed = run_streaming_subprocess(
                cmd=cmd,
                cwd=self.workspace_root,
                env=None,
                input_text=prompt,
                timeout_seconds=effective_timeout,
                live_io=live_io,
                stdout_prefix=f"[qa-loop][{session_label} stdout] ",
                stderr_prefix=f"[qa-loop][{session_label} stderr] ",
                stdout_formatter=lambda line, session_label=session_label: format_compat_stream_event(session_label, line),
                stderr_formatter=lambda line, session_label=session_label: format_codex_stderr_event(session_label, line),
            )
            stdout = completed["stdout"]
            stderr = completed["stderr"]
            returncode = completed["returncode"]
        except subprocess.TimeoutExpired as exc:
            persist_stream_artifacts(
                event_log_path=event_log_path,
                stderr_log_path=stderr_log_path,
                stdout=exc.output,
                stderr=exc.stderr,
            )
            raise ClaudeInvocationError(
                f"claude compat exec timed out after {effective_timeout:.1f}s; see {stderr_log_path}"
            ) from exc
        persist_stream_artifacts(
            event_log_path=event_log_path,
            stderr_log_path=stderr_log_path,
            stdout=stdout,
            stderr=stderr,
        )

        if returncode != 0:
            raise ClaudeInvocationError(
                f"claude compat exec failed with exit code {returncode}; see {stderr_log_path}"
            )
        try:
            return json.loads(output_path.read_text(encoding="utf-8"))
        except FileNotFoundError as exc:
            try:
                payload = extract_claude_structured_output(stdout)
            except ValueError:
                raise ClaudeInvocationError(f"compat command did not write {output_path}") from exc
            output_path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
            return payload
        except json.JSONDecodeError as exc:
            raise ClaudeInvocationError(f"compat command wrote invalid JSON to {output_path}: {exc}") from exc


class SupervisorLoop:
    def __init__(
        self,
        *,
        config: LoopConfig,
        claude_client: ClaudeClient,
        codex_client: CodexClient | None = None,
        driver: PTYDriver | None = None,
        role_prompt: str,
        nudge_prompt: str,
    ) -> None:
        self.config = config
        self.claude_client = claude_client
        self.codex_client = codex_client
        self.driver = driver or PTYDriver()
        self.role_prompt = role_prompt.strip()
        self.nudge_prompt = nudge_prompt.strip()

    def run(self, *, run_id: str | None = None) -> dict[str, Any]:
        run_id = run_id or default_run_id()
        paths = prepare_run_paths(self.config.artifacts_root, run_id)
        live_io = LiveIO(transcript_path=paths.full_transcript)
        started_at = time.time()
        started_monotonic = time.monotonic()
        docker_snapshots: list[dict[str, Any]] = []
        live_io.stdout(
            f"[qa-loop] target container: {self.config.container_name} "
            f"({self.config.command_cwd})\n"
        )
        self.driver.start(self._target_command(), cwd=self.config.host_cwd, env=os.environ.copy())

        transcript_raw = ""
        transcript_clean = ""
        turns: list[dict[str, Any]] = []
        ensure_run_artifact_files(paths)
        live_io.stdout(f"[qa-loop] run id: {run_id}\n")
        latest_output = visible_terminal_output(self._read_until_actionable(patience=True))
        transcript_raw, transcript_clean = append_terminal_output(
            paths,
            transcript_raw,
            transcript_clean,
            latest_output,
            live_io=live_io,
        )

        loop_state = "CONTINUE"
        stop_reason = ""
        idle_turns = 0
        control_turns = 0
        blind_first_active = True
        target_stopped_by_supervisor = False

        try:
            while True:
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
                        live_io=live_io,
                    )
                else:
                    latest_output = ""

                elapsed = int(time.monotonic() - started_monotonic)
                pending_iteration = control_turns + 1
                released_blind_first = not blind_first_active
                stalled = idle_turns >= blind_first_release_threshold(self.config.max_idle_turns)
                if (
                    blind_first_active
                    and stalled
                    and self.config.fixture_post_path is not None
                    and transcript_clean.strip()
                    and not terminal_requires_input(transcript_clean)
                ):
                    blind_first_active = False
                    released_blind_first = True
                    append_jsonl(
                        paths.interaction_log,
                        {
                            "time": utc_now(),
                            "iteration": pending_iteration,
                            "kind": "blind_first_released",
                            "idle_turns": idle_turns,
                        },
                    )
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
                        live_io=live_io,
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

                if control_turns >= self.config.max_iterations:
                    loop_state = "STOP_DEADEND"
                    stop_reason = "iteration_limit"
                    break
                control_turns += 1
                iteration = control_turns

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
                        live_io=live_io,
                    )
                except ClaudeInvocationError as exc:
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
                            live_io=live_io,
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
                    stop_reason = "claude_control_error"
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
                live_io.stdout(
                    f"[qa-loop] turn {iteration}: {directive.action} {directive.loop_state} "
                    f"{directive.summary or directive.next_focus}\n"
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
                        live_io.stdout(f"[qa-loop] input -> {directive.input_text}\n")
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
                    idle_turns = 0
                    if not self.driver.is_running() and self._command_matches_target(directive.input_text):
                        self.driver.stop()
                        append_jsonl(
                            paths.interaction_log,
                            {
                                "time": utc_now(),
                                "iteration": iteration,
                                "kind": "process_restart",
                                "command": directive.input_text,
                            },
                        )
                        live_io.stdout(
                            f"[qa-loop] restarting target command in {self.config.command_cwd}: "
                            f"{directive.input_text}\n"
                        )
                        self.driver.start(
                            self._docker_exec_command(["bash", "-lc", directive.input_text], tty=True),
                            cwd=self.config.host_cwd,
                            env=os.environ.copy(),
                        )
                    else:
                        command_result = self._run_external_command(
                            command_text=directive.input_text,
                            iteration=iteration,
                            paths=paths,
                            remaining_runtime_seconds=max(
                                self.config.max_runtime_seconds - (time.monotonic() - started_monotonic),
                                EXTERNAL_COMMAND_TIMEOUT_FLOOR_SECONDS,
                            ),
                            live_io=live_io,
                        )
                        transcript_raw, transcript_clean = append_terminal_output(
                            paths,
                            transcript_raw,
                            transcript_clean,
                            command_result["transcript"],
                            live_io=live_io,
                            echo_live=False,
                        )
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
                            live_io.stdout(
                                f"[qa-loop] restarting target command in {self.config.command_cwd}: "
                                f"{shlex.join(self.config.command)}\n"
                            )
                            self.driver.start(self._target_command(), cwd=self.config.host_cwd, env=os.environ.copy())
                    latest_output = visible_terminal_output(self._read_until_actionable(patience=True))
                    transcript_raw, transcript_clean = append_terminal_output(
                        paths,
                        transcript_raw,
                        transcript_clean,
                        latest_output,
                        live_io=live_io,
                    )
                elif latest_output:
                    idle_turns = 0
                else:
                    idle_turns += 1

                latest_output = visible_terminal_output(
                    self._read_until_actionable(patience=directive.action == "SEND_INPUT")
                )
                transcript_raw, transcript_clean = append_terminal_output(
                    paths,
                    transcript_raw,
                    transcript_clean,
                    latest_output,
                    live_io=live_io,
                )

                if self.config.docker_snapshots_enabled:
                    try:
                        snapshot = self._capture_container_snapshot(
                            iteration=iteration,
                            paths=paths,
                            live_io=live_io,
                        )
                        docker_snapshots.append(snapshot)
                    except DockerInvocationError as exc:
                        append_jsonl(
                            paths.interaction_log,
                            {
                                "time": utc_now(),
                                "iteration": iteration,
                                "kind": "docker_snapshot_error",
                                "reason": str(exc),
                            },
                        )
                        loop_state = "STOP_DEADEND"
                        stop_reason = "docker_snapshot_error"
                        break

                loop_state = directive.loop_state
                if loop_state != "CONTINUE":
                    stop_reason = supervisor_stop_reason(loop_state)
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
                    live_io=live_io,
                )
            target_stopped_by_supervisor = self.driver.is_running()
            exit_code = self.driver.stop()
            trailing_output = self.driver.drain(max_bytes=262144)
            if trailing_output:
                transcript_raw, transcript_clean = append_terminal_output(
                    paths,
                    transcript_raw,
                    transcript_clean,
                    trailing_output,
                    live_io=live_io,
                )

        if not stop_reason:
            stop_reason = supervisor_stop_reason(loop_state)

        summary = {
            "run_id": run_id,
            "started_at": utc_from_timestamp(started_at),
            "finished_at": utc_now(),
            "command": self.config.command,
            "command_cwd": self.config.command_cwd,
            "artifacts_root": str(self.config.artifacts_root),
            "loop_state": loop_state,
            "stop_reason": stop_reason,
            "iterations_completed": len(turns),
            "idle_turns": idle_turns,
            "blind_first_released": not blind_first_active,
            "fixture_post_path": str(self.config.fixture_post_path) if self.config.fixture_post_path else "",
            "terminal_exit_code": None if target_stopped_by_supervisor else exit_code,
            "terminal_stopped_by_supervisor": target_stopped_by_supervisor,
            "report_status": "pending",
            "qa_context": {
                "fixture_id": self.config.fixture_id,
                "guide_step": self.config.guide_step,
                "ref_under_test": self.config.ref_under_test,
                "checkpoint_key": self.config.checkpoint_key,
                "source_kind": self.config.source_kind,
                "source_id": self.config.source_id,
                "runtime_prerequisites": list(self.config.runtime_prerequisites),
            },
            "docker": {
                "docker_bin": self.config.docker_bin,
                "container_name": self.config.container_name,
                "container_image": self.config.container_image or "",
                "container_workdir": self.config.command_cwd,
                "snapshots_enabled": self.config.docker_snapshots_enabled,
                "snapshots": docker_snapshots,
            },
            "paths": {
                "summary_json": str(paths.summary_json),
                "turns_jsonl": str(paths.turns_jsonl),
                "docker_dir": str(paths.docker_dir),
                "terminal_raw": str(paths.terminal_raw),
                "terminal_clean": str(paths.terminal_clean),
                "interaction_log": str(paths.interaction_log),
                "full_transcript": str(paths.full_transcript),
                "report_json": str(paths.report_json),
                "report_markdown": str(paths.report_markdown),
            },
        }
        exported_artifacts = self._export_container_artifacts(paths=paths)
        if exported_artifacts:
            summary["docker"]["exported_artifacts"] = exported_artifacts
        if loop_state == "STOP_REPORT" and self.config.fixture_post_path is not None:
            candidate_workspace = exported_workspace_review_root(paths)
            if candidate_workspace is not None:
                try:
                    review = self._request_integration_review(paths=paths, summary=summary, live_io=live_io)
                except CodexInvocationError as exc:
                    review = integration_review_error(
                        f"Codex integration review failed: {exc}",
                        issues=["The exported integration could not be reviewed against the post fixture."],
                    )
                summary["integration_review"] = asdict(review)
                append_jsonl(
                    paths.interaction_log,
                    {
                        "time": utc_now(),
                        "iteration": len(turns),
                        "kind": "integration_review",
                        "status": review.status,
                        "confidence": review.confidence,
                        "verdict": review.verdict,
                    },
                )
                if review.status == "fail":
                    loop_state = "STOP_FIX"
                    stop_reason = "integration_review_failed"
                elif review.status == "error":
                    loop_state = "STOP_FIX"
                    stop_reason = "integration_review_error"
        summary["loop_state"] = loop_state
        summary["stop_reason"] = stop_reason
        paths.summary_json.write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
        write_report_artifacts(paths=paths, run_id=run_id, summary=summary, report=provisional_report(summary=summary, turns=turns))

        try:
            report = self._request_report(paths=paths, summary=summary, loop_state=loop_state, live_io=live_io)
            summary["report_status"] = "ready"
        except ClaudeInvocationError as exc:
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
            summary["report_status"] = "fallback"
        summary["report_generated_at"] = utc_now()
        write_report_artifacts(paths=paths, run_id=run_id, summary=summary, report=report)
        paths.summary_json.write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")

        return {
            "loop_state": loop_state,
            "stop_reason": stop_reason,
            "summary_path": str(paths.summary_json),
            "report_json_path": str(paths.report_json),
            "report_markdown_path": str(paths.report_markdown),
            "terminal_clean_path": str(paths.terminal_clean),
            "full_transcript_path": str(paths.full_transcript),
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
        live_io: LiveIO,
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
            "command_cwd": self.config.command_cwd,
            "container_name": self.config.container_name,
            "blind_first_active": blind_first_active,
            "post_fixture_path_available": bool(self.config.fixture_post_path and not blind_first_active),
            "post_fixture_path": str(self.config.fixture_post_path) if self.config.fixture_post_path and not blind_first_active else "",
            "runtime_prerequisites": list(self.config.runtime_prerequisites),
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

        turn_base = paths.claude_dir / f"turn-{iteration:03d}"
        payload = self.claude_client.run_structured(
            prompt=prompt,
            schema_path=PROMPTS_DIR / "control_schema.json",
            output_path=turn_base.with_suffix(".output.json"),
            event_log_path=turn_base.with_suffix(".jsonl"),
            stderr_log_path=turn_base.with_suffix(".stderr.log"),
            add_dirs=qa_add_dirs(self.config.fixture_post_path if not blind_first_active else None),
            live_io=live_io,
            session_label=f"claude-control-{iteration:03d}",
        )
        return LoopDirective.from_dict(payload)

    def _run_external_command(
        self,
        *,
        command_text: str,
        iteration: int,
        paths: RunPaths,
        remaining_runtime_seconds: float,
        live_io: LiveIO,
    ) -> dict[str, Any]:
        append_jsonl(
            paths.interaction_log,
            {
                "time": utc_now(),
                "iteration": iteration,
                "kind": "command",
                "text": command_text,
                "cwd": self.config.command_cwd,
            },
        )
        live_io.stdout(
            f"[qa-loop] external command start in {self.config.container_name}:{self.config.command_cwd}\n"
            f"$ {command_text}\n"
        )
        try:
            completed = run_streaming_subprocess(
                cmd=self._docker_exec_command(["bash", "-lc", command_text], tty=False),
                cwd=self.config.host_cwd,
                env=os.environ.copy(),
                timeout_seconds=max(remaining_runtime_seconds, EXTERNAL_COMMAND_TIMEOUT_FLOOR_SECONDS),
                live_io=live_io,
                stdout_prefix="[qa-loop][external stdout] ",
                stderr_prefix="[qa-loop][external stderr] ",
            )
            returncode = completed["returncode"]
            stdout = completed["stdout"]
            stderr = completed["stderr"]
        except subprocess.TimeoutExpired as exc:
            returncode = 124
            stdout = coerce_text(exc.stdout or "")
            stderr = coerce_text(exc.stderr or "")

        transcript_lines = [
            f"[qa-loop] external command in {self.config.container_name}:{self.config.command_cwd}:",
            f"$ {command_text}",
        ]
        if stdout.strip():
            transcript_lines.extend(["[stdout]", stdout.rstrip()])
        if stderr.strip():
            transcript_lines.extend(["[stderr]", stderr.rstrip()])
        transcript_lines.append(f"[qa-loop] external command exit code: {returncode}")
        transcript = "\n".join(transcript_lines) + "\n"
        live_io.stdout(f"[qa-loop] external command exit code: {returncode}\n")

        append_jsonl(
            paths.interaction_log,
            {
                "time": utc_now(),
                "iteration": iteration,
                "kind": "command_result",
                "text": command_text,
                "cwd": self.config.command_cwd,
                "returncode": returncode,
            },
        )

        return {"returncode": returncode, "stdout": stdout, "stderr": stderr, "transcript": transcript}

    def _target_command(self) -> list[str]:
        return self._docker_exec_command(self.config.command, tty=True)

    def _docker_exec_command(self, inner_command: list[str], *, tty: bool) -> list[str]:
        cmd = [self.config.docker_bin, "exec", "-i"]
        if tty:
            cmd.append("-t")
        cmd.extend(["-w", self.config.command_cwd, self.config.container_name, *inner_command])
        return cmd

    def _command_matches_target(self, command_text: str) -> bool:
        try:
            requested = shlex.split(command_text)
        except ValueError:
            return False
        return requested == self.config.command

    def _docker_capture(self, command: list[str], *, timeout_seconds: float = 30.0) -> subprocess.CompletedProcess[str]:
        completed = subprocess.run(
            command,
            cwd=str(self.config.host_cwd),
            env=os.environ.copy(),
            text=True,
            capture_output=True,
            timeout=timeout_seconds,
            check=False,
        )
        if completed.returncode != 0:
            detail = completed.stderr.strip() or completed.stdout.strip() or f"exit code {completed.returncode}"
            raise DockerInvocationError(f"{shlex.join(command)} failed: {detail}")
        return completed

    def _capture_container_snapshot(
        self,
        *,
        iteration: int,
        paths: RunPaths,
        live_io: LiveIO,
    ) -> dict[str, Any]:
        snapshot_ref = f"concierge-qa-snapshots:{docker_tag_component(paths.run_id)}-turn-{iteration:03d}"
        live_io.stdout(f"[qa-loop] docker snapshot turn {iteration}: {snapshot_ref}\n")

        commit_completed = self._docker_capture(
            [self.config.docker_bin, "commit", self.config.container_name, snapshot_ref]
        )
        image_id = commit_completed.stdout.strip().splitlines()[-1] if commit_completed.stdout.strip() else snapshot_ref

        diff_completed = self._docker_capture([self.config.docker_bin, "diff", self.config.container_name])
        diff_path = paths.docker_dir / f"turn-{iteration:03d}.diff.txt"
        diff_path.write_text(diff_completed.stdout, encoding="utf-8")

        inspect_completed = self._docker_capture([self.config.docker_bin, "inspect", snapshot_ref])
        inspect_path = paths.docker_dir / f"turn-{iteration:03d}.inspect.json"
        inspect_path.write_text(inspect_completed.stdout, encoding="utf-8")

        snapshot = {
            "iteration": iteration,
            "image_ref": snapshot_ref,
            "image_id": image_id,
            "diff_path": str(diff_path),
            "inspect_path": str(inspect_path),
        }
        append_jsonl(
            paths.interaction_log,
            {
                "time": utc_now(),
                "iteration": iteration,
                "kind": "docker_snapshot",
                **snapshot,
            },
        )
        return snapshot

    def _export_container_artifacts(self, *, paths: RunPaths) -> list[dict[str, str]]:
        exported: list[dict[str, str]] = []
        export_root = paths.docker_dir / "export" / "workspace"
        check_script = "for path in " + " ".join(shlex.quote(path) for path in EXPORTED_WORKSPACE_PATHS) + (
            '; do if test -e "$path"; then printf "%s\\n" "$path"; fi; done'
        )
        existing = subprocess.run(
            self._docker_exec_command(["bash", "-lc", check_script], tty=False),
            cwd=str(self.config.host_cwd),
            env=os.environ.copy(),
            text=True,
            capture_output=True,
            timeout=15.0,
            check=False,
        )
        if existing.returncode != 0:
            detail = existing.stderr.strip() or existing.stdout.strip() or f"exit code {existing.returncode}"
            raise DockerInvocationError(f"{shlex.join(self._docker_exec_command(['bash', '-lc', check_script], tty=False))} failed: {detail}")

        for relative_path in [line.strip() for line in existing.stdout.splitlines() if line.strip()]:
            destination = export_root / relative_path
            destination.parent.mkdir(parents=True, exist_ok=True)
            source = f"{self.config.container_name}:{self.config.command_cwd}/{relative_path}"
            completed = subprocess.run(
                [self.config.docker_bin, "cp", source, str(destination)],
                cwd=str(self.config.host_cwd),
                env=os.environ.copy(),
                text=True,
                capture_output=True,
                timeout=30.0,
                check=False,
            )
            if completed.returncode != 0:
                detail = completed.stderr.strip() or completed.stdout.strip() or f"exit code {completed.returncode}"
                raise DockerInvocationError(f"{self.config.docker_bin} cp {source} {destination} failed: {detail}")
            exported.append({"source": source, "destination": str(destination)})
        return exported

    def _request_report(
        self,
        *,
        paths: RunPaths,
        summary: dict[str, Any],
        loop_state: str,
        live_io: LiveIO,
    ) -> QAReport:
        context = build_report_context(
            summary=summary,
            turns=load_jsonl(paths.turns_jsonl),
            loop_state=loop_state,
        )
        prompt = textwrap.dedent(
            f"""
            You are writing the final qualitative QA report for a Concierge terminal session.
            Use only the structured context below. Do not use tools or external files.
            Be concise:
            - `integration_progress` should be one short paragraph.
            - Each list should contain at most 4 items.
            - Each item should be one sentence.
            Return only JSON matching the provided schema with exactly these keys:
            - `title`
            - `overall_outcome`
            - `loop_state`
            - `integration_progress`
            - `ux_clarity`
            - `product_issues`
            - `agent_interaction_issues`
            - `suggestions`
            - `notable_moments`

            Context:
            ```json
            {json.dumps(context, indent=2)}
            ```
            """
        ).strip()
        report_base = paths.claude_dir / "final-report"
        payload = self.claude_client.run_structured(
            prompt=prompt,
            schema_path=PROMPTS_DIR / "report_schema.json",
            output_path=report_base.with_suffix(".output.json"),
            event_log_path=report_base.with_suffix(".jsonl"),
            stderr_log_path=report_base.with_suffix(".stderr.log"),
            add_dirs=(),
            allowed_tools=None,
            timeout_seconds=min(self.config.claude_timeout_seconds, DEFAULT_REPORT_TIMEOUT_SECONDS),
            live_io=live_io,
            session_label="claude-final-report",
        )
        return normalize_qa_report(payload, default_loop_state=loop_state)

    def _request_integration_review(
        self,
        *,
        paths: RunPaths,
        summary: dict[str, Any],
        live_io: LiveIO,
    ) -> IntegrationReview:
        if self.codex_client is None:
            raise CodexInvocationError("integration review requested without a configured Codex client")

        candidate_workspace = exported_workspace_review_root(paths)
        if candidate_workspace is None:
            return integration_review_error(
                "No exported integration files were available for comparison.",
                issues=["The QA loop did not export a generated integration bundle to compare against the post fixture."],
            )
        if self.config.fixture_post_path is None:
            return integration_review_error(
                "No post fixture path was available for comparison.",
                issues=["The QA loop could not find the ground-truth fixture checkout for expert review."],
            )

        context = build_integration_review_context(
            summary=summary,
            turns=load_jsonl(paths.turns_jsonl),
            candidate_workspace=candidate_workspace,
            fixture_post_path=self.config.fixture_post_path,
        )
        prompt = textwrap.dedent(
            f"""
            You are the final expert reviewer for a Tensorleap integration QA run.
            Compare the generated integration against the known-good post fixture and decide whether
            the generated code is functionally equivalent and appropriate for the scoped checkpoint.

            Rules:
            - Ground your judgment in the exported candidate workspace and the post fixture files.
            - Literal code identity is not required.
            - Functional equivalence and correct integration wiring are required.
            - Mark `status` as `pass` only when the generated integration is clearly correct for the scope.
            - Mark `status` as `fail` when the generated integration has literal bugs, missing wiring, incorrect semantics, or material divergence from the post fixture.
            - Mark `status` as `error` only when you genuinely cannot judge from the available artifacts.
            - Keep `verdict`, `functional_equivalence`, and `quality_assessment` concise.
            - Keep `issues` to at most 5 concrete items.

            Return only JSON matching the provided schema with exactly these keys:
            - `status`
            - `verdict`
            - `functional_equivalence`
            - `quality_assessment`
            - `issues`
            - `confidence`

            Context:
            ```json
            {json.dumps(context, indent=2)}
            ```
            """
        ).strip()
        review_base = paths.claude_dir / "integration-review"
        payload = self.codex_client.run_structured(
            prompt=prompt,
            schema_path=PROMPTS_DIR / "integration_review_schema.json",
            output_path=review_base.with_suffix(".output.json"),
            event_log_path=review_base.with_suffix(".jsonl"),
            stderr_log_path=review_base.with_suffix(".stderr.log"),
            add_dirs=[candidate_workspace, self.config.fixture_post_path],
            timeout_seconds=self.config.review_timeout_seconds,
            live_io=live_io,
            session_label="codex-integration-review",
        )
        return normalize_integration_review(payload)


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run Concierge inside a Docker container PTY and let Claude drive it like a QA engineer.",
    )
    parser.add_argument(
        "--artifacts-root",
        default=str(QA_DIR),
        help="Directory where runs/, transcripts/, and reports/ will be written. Defaults to QA/.",
    )
    parser.add_argument("--docker-bin", default=os.environ.get("DOCKER_BIN", "docker"))
    parser.add_argument("--container-name", required=True)
    parser.add_argument("--container-workdir", default="/workspace")
    parser.add_argument("--container-image", default=None)
    parser.add_argument("--claude-command", default=os.environ.get("CLAUDE_BIN", "claude"))
    parser.add_argument("--claude-timeout-seconds", type=float, default=DEFAULT_CODEX_TIMEOUT_SECONDS)
    parser.add_argument("--review-command", default=os.environ.get("CODEX_BIN", "codex"))
    parser.add_argument("--review-timeout-seconds", type=float, default=DEFAULT_CODEX_TIMEOUT_SECONDS)
    parser.add_argument(
        "--codex-command",
        dest="claude_command",
        default=argparse.SUPPRESS,
        help=argparse.SUPPRESS,
    )
    parser.add_argument("--model", default=None)
    parser.add_argument(
        "--codex-timeout-seconds",
        dest="claude_timeout_seconds",
        type=float,
        default=argparse.SUPPRESS,
        help=argparse.SUPPRESS,
    )
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
    parser.add_argument("--fixture-id", default="")
    parser.add_argument("--guide-step", default="")
    parser.add_argument("--ref-under-test", default="")
    parser.add_argument("--checkpoint-key", default="")
    parser.add_argument("--source-kind", default="")
    parser.add_argument("--source-id", default="")
    parser.add_argument("--runtime-prerequisites-json", default="[]")
    parser.add_argument(
        "--docker-snapshots",
        action="store_true",
        help="Capture a docker commit plus diff/inspect metadata after each supervisor turn.",
    )
    parser.add_argument(
        "command",
        nargs=argparse.REMAINDER,
        help="Command to run in the PTY. Prefix with -- to stop qa_loop option parsing.",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    artifacts_root = Path(args.artifacts_root).resolve()
    fixture_post_path = Path(args.fixture_post_path).resolve() if args.fixture_post_path else None
    runtime_prerequisites = parse_runtime_prerequisites(args.runtime_prerequisites_json)
    command = list(args.command)
    if command and command[0] == "--":
        command = command[1:]
    if not command:
        command = default_concierge_command(artifacts_root)

    config = LoopConfig(
        artifacts_root=artifacts_root,
        docker_bin=args.docker_bin,
        host_cwd=REPO_ROOT,
        container_name=args.container_name,
        container_image=args.container_image,
        command=command,
        command_cwd=args.container_workdir,
        claude_command=args.claude_command,
        claude_model=args.model,
        claude_timeout_seconds=args.claude_timeout_seconds,
        review_command=args.review_command,
        review_model=args.model,
        review_timeout_seconds=args.review_timeout_seconds,
        max_iterations=args.max_iterations,
        max_idle_turns=args.max_idle_turns,
        max_runtime_seconds=args.max_runtime_seconds,
        read_quiet_seconds=args.read_quiet_seconds,
        read_timeout_seconds=args.read_timeout_seconds,
        settle_timeout_seconds=args.settle_timeout_seconds,
        transcript_tail_chars=args.transcript_tail_chars,
        latest_output_chars=args.latest_output_chars,
        fixture_post_path=fixture_post_path,
        docker_snapshots_enabled=args.docker_snapshots,
        fixture_id=args.fixture_id,
        guide_step=args.guide_step,
        ref_under_test=args.ref_under_test,
        checkpoint_key=args.checkpoint_key,
        source_kind=args.source_kind,
        source_id=args.source_id,
        runtime_prerequisites=runtime_prerequisites,
    )
    role_prompt = (PROMPTS_DIR / "role_prompt.md").read_text(encoding="utf-8")
    nudge_prompt = (PROMPTS_DIR / "nudge_prompt.md").read_text(encoding="utf-8")
    claude_workspace = artifacts_root
    client = ClaudeClient(
        workspace_root=claude_workspace,
        artifacts_root=artifacts_root,
        command=config.claude_command,
        model=config.claude_model,
        timeout_seconds=config.claude_timeout_seconds,
    )
    codex_client = CodexClient(
        workspace_root=artifacts_root,
        artifacts_root=artifacts_root,
        command=config.review_command,
        model=config.review_model,
        timeout_seconds=config.review_timeout_seconds,
    )
    loop = SupervisorLoop(
        config=config,
        claude_client=client,
        codex_client=codex_client,
        role_prompt=role_prompt,
        nudge_prompt=nudge_prompt,
    )
    result = loop.run(run_id=args.run_id)
    print(f"[qa-loop] transcript: {result['full_transcript_path']}", flush=True)
    print(f"[qa-loop] report: {result['report_markdown_path']}", flush=True)
    return exit_code_for_loop_state(result["loop_state"])


def default_run_id() -> str:
    return f"{time.strftime('%Y%m%dT%H%M%SZ', time.gmtime())}-{uuid.uuid4().hex[:8]}"


def prepare_run_paths(artifacts_root: Path, run_id: str) -> RunPaths:
    run_dir = artifacts_root / "runs" / run_id
    claude_dir = run_dir / "claude"
    docker_dir = run_dir / "docker"
    transcripts_dir = artifacts_root / "transcripts"
    reports_dir = artifacts_root / "reports"
    for directory in (run_dir, claude_dir, docker_dir, transcripts_dir, reports_dir):
        directory.mkdir(parents=True, exist_ok=True)
    return RunPaths(
        run_id=run_id,
        run_dir=run_dir,
        claude_dir=claude_dir,
        docker_dir=docker_dir,
        summary_json=run_dir / "summary.json",
        turns_jsonl=run_dir / "turns.jsonl",
        terminal_raw=transcripts_dir / f"{run_id}.terminal.raw.txt",
        terminal_clean=transcripts_dir / f"{run_id}.terminal.txt",
        interaction_log=transcripts_dir / f"{run_id}.interaction.jsonl",
        full_transcript=transcripts_dir / f"{run_id}.full.txt",
        report_json=reports_dir / f"{run_id}.json",
        report_markdown=reports_dir / f"{run_id}.md",
    )


def ensure_run_artifact_files(paths: RunPaths) -> None:
    for path in (paths.terminal_raw, paths.terminal_clean, paths.interaction_log, paths.full_transcript):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.touch(exist_ok=True)


def append_terminal_output(
    paths: RunPaths,
    raw_text: str,
    clean_text: str,
    latest_output: str,
    *,
    live_io: LiveIO | None = None,
    echo_live: bool = True,
) -> tuple[str, str]:
    if not latest_output:
        return raw_text, clean_text
    raw_text += latest_output
    clean_chunk = normalize_terminal_text(latest_output)
    clean_text += clean_chunk
    if live_io is not None and echo_live:
        live_io.stdout(latest_output)
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


def parse_runtime_prerequisites(raw_value: str) -> list[dict[str, Any]]:
    candidate = str(raw_value or "").strip() or "[]"
    try:
        payload = json.loads(candidate)
    except json.JSONDecodeError as exc:
        raise ValueError(f"invalid runtime prerequisites JSON: {exc}") from exc
    if payload is None:
        return []
    if not isinstance(payload, list):
        raise ValueError("runtime prerequisites JSON must decode to a list")
    normalized: list[dict[str, Any]] = []
    for item in payload:
        if not isinstance(item, dict):
            raise ValueError("runtime prerequisite entries must be objects")
        normalized.append(item)
    return normalized


def qa_add_dirs(fixture_post_path: Path | None) -> list[Path]:
    add_dirs: list[Path] = []
    if fixture_post_path is not None:
        add_dirs.append(fixture_post_path if fixture_post_path.is_dir() else fixture_post_path.parent)
    return add_dirs


def load_jsonl(path: Path) -> list[dict[str, Any]]:
    if not path.is_file():
        return []

    records: list[dict[str, Any]] = []
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line:
            continue
        payload = json.loads(line)
        if isinstance(payload, dict):
            records.append(payload)
    return records


def build_report_context(*, summary: dict[str, Any], turns: list[dict[str, Any]], loop_state: str) -> dict[str, Any]:
    qa_context = summary.get("qa_context", {})
    if not isinstance(qa_context, dict):
        qa_context = {}

    context = {
        "run": {
            "run_id": str(summary.get("run_id", "")).strip(),
            "fixture_id": str(qa_context.get("fixture_id", "")).strip(),
            "guide_step": str(qa_context.get("guide_step", "")).strip(),
            "ref_under_test": str(qa_context.get("ref_under_test", "")).strip(),
            "loop_state": loop_state,
            "stop_reason": str(summary.get("stop_reason", "")).strip(),
            "iterations_completed": int(summary.get("iterations_completed", 0) or 0),
            "idle_turns": int(summary.get("idle_turns", 0) or 0),
        },
        "turns": [
            {
                "iteration": int(turn.get("iteration", 0) or 0),
                "action": str(directive.get("action", "")).strip(),
                "loop_state": str(directive.get("loop_state", "")).strip(),
                "summary": str(directive.get("summary", "")).strip(),
                "issues": clean_string_list(directive.get("issues", [])),
                "next_focus": str(directive.get("next_focus", "")).strip(),
            }
            for turn in turns
            for directive in [turn.get("directive", {}) if isinstance(turn.get("directive", {}), dict) else {}]
        ],
    }
    integration_review = summary.get("integration_review")
    if isinstance(integration_review, dict):
        context["integration_review"] = {
            "status": str(integration_review.get("status", "")).strip(),
            "verdict": str(integration_review.get("verdict", "")).strip(),
            "functional_equivalence": str(integration_review.get("functional_equivalence", "")).strip(),
            "quality_assessment": str(integration_review.get("quality_assessment", "")).strip(),
            "issues": clean_string_list(integration_review.get("issues", [])),
            "confidence": str(integration_review.get("confidence", "")).strip(),
        }
    return context


def build_integration_review_context(
    *,
    summary: dict[str, Any],
    turns: list[dict[str, Any]],
    candidate_workspace: Path,
    fixture_post_path: Path,
) -> dict[str, Any]:
    qa_context = summary.get("qa_context", {})
    if not isinstance(qa_context, dict):
        qa_context = {}

    return {
        "run": {
            "run_id": str(summary.get("run_id", "")).strip(),
            "fixture_id": str(qa_context.get("fixture_id", "")).strip(),
            "guide_step": str(qa_context.get("guide_step", "")).strip(),
            "ref_under_test": str(qa_context.get("ref_under_test", "")).strip(),
            "checkpoint_key": str(qa_context.get("checkpoint_key", "")).strip(),
            "source_kind": str(qa_context.get("source_kind", "")).strip(),
            "source_id": str(qa_context.get("source_id", "")).strip(),
            "stop_reason": str(summary.get("stop_reason", "")).strip(),
        },
        "candidate_workspace": str(candidate_workspace),
        "fixture_post_path": str(fixture_post_path),
        "recent_turn_summaries": [
            {
                "iteration": int(turn.get("iteration", 0) or 0),
                "action": str(directive.get("action", "")).strip(),
                "loop_state": str(directive.get("loop_state", "")).strip(),
                "summary": str(directive.get("summary", "")).strip(),
                "issues": clean_string_list(directive.get("issues", [])),
            }
            for turn in turns[-5:]
            for directive in [turn.get("directive", {}) if isinstance(turn.get("directive", {}), dict) else {}]
        ],
    }


def exported_workspace_review_root(paths: RunPaths) -> Path | None:
    export_root = paths.docker_dir / "export" / "workspace"
    if not export_root.is_dir():
        return None
    for relative_path in EXPORTED_REVIEW_PATHS:
        if (export_root / relative_path).exists():
            return export_root
    return None


def docker_tag_component(value: str) -> str:
    sanitized = re.sub(r"[^a-z0-9_.-]+", "-", value.lower()).strip(".-")
    return sanitized or "qa-run"


def blind_first_release_threshold(max_idle_turns: int) -> int:
    if max_idle_turns <= 1:
        return 1
    return min(max_idle_turns, max(3, max_idle_turns - 1))


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


def indent_lines(text: str, *, prefix: str = "    ") -> list[str]:
    body = text.rstrip("\n")
    if not body:
        return []
    return [f"{prefix}{line}" for line in body.splitlines()]


def persist_stream_artifacts(
    *,
    event_log_path: Path,
    stderr_log_path: Path,
    stdout: str | bytes | None,
    stderr: str | bytes | None,
) -> None:
    event_log_path.parent.mkdir(parents=True, exist_ok=True)
    event_log_path.write_text(coerce_text(stdout), encoding="utf-8")
    stderr_log_path.write_text(coerce_text(stderr), encoding="utf-8")


def extract_claude_structured_output(stdout: str) -> dict[str, Any]:
    stripped = stdout.strip()
    if not stripped:
        raise ValueError("claude did not emit JSON output")

    try:
        payload = json.loads(stripped)
    except json.JSONDecodeError as exc:
        raise ValueError(f"claude wrote invalid JSON output: {exc}") from exc

    if isinstance(payload, dict):
        structured_output = payload.get("structured_output")
        if isinstance(structured_output, dict):
            return structured_output

        result_text = payload.get("result")
        if isinstance(result_text, str) and result_text.strip():
            nested = extract_json_dict_from_text(result_text)
            if isinstance(nested, dict):
                return nested

    raise ValueError("claude JSON output did not include structured_output")


def extract_json_dict_from_text(text: str) -> dict[str, Any] | None:
    decoder = json.JSONDecoder()
    candidates = [match.group(1).strip() for match in JSON_CODE_FENCE_RE.finditer(text)]
    candidates.append(text.strip())

    for candidate in candidates:
        if not candidate:
            continue

        try:
            payload, _ = decoder.raw_decode(candidate)
        except json.JSONDecodeError:
            payload = None
        if isinstance(payload, dict):
            return payload

        for index, char in enumerate(candidate):
            if char not in "{[":
                continue
            try:
                payload, _ = decoder.raw_decode(candidate[index:])
            except json.JSONDecodeError:
                continue
            if isinstance(payload, dict):
                return payload

    return None


def summarize_report_item(item: Any) -> str:
    if isinstance(item, str):
        return item.strip()
    if not isinstance(item, dict):
        return str(item).strip()

    severity = str(item.get("severity", "")).strip()
    area = str(item.get("area", "")).strip()
    description = str(item.get("description", "")).strip()
    detail = str(item.get("detail", "")).strip()
    likely_cause = str(item.get("likely_cause", "")).strip()
    suggestion = str(item.get("suggestion", "")).strip()
    lead = description or detail or likely_cause

    prefixes = [value for value in (severity, area) if value]
    text = lead
    if prefixes:
        prefix = " / ".join(prefixes)
        text = f"[{prefix}] {lead or 'Report item'}"
    if suggestion:
        if text:
            text += f" Suggestion: {suggestion}"
        else:
            text = f"Suggestion: {suggestion}"
    return text.strip()


def clean_report_items(values: Any) -> list[str]:
    if isinstance(values, list):
        items = values
    elif values is None:
        items = []
    else:
        items = [values]

    cleaned: list[str] = []
    for item in items:
        text = summarize_report_item(item)
        if text:
            cleaned.append(text)
    return cleaned


def normalize_integration_progress(value: Any) -> str:
    if isinstance(value, str):
        return value.strip()
    if isinstance(value, dict):
        summary = str(value.get("summary", "")).strip()
        if summary:
            return summary
        completed = ", ".join(clean_string_list(value.get("completed_steps", [])))
        attempted = ", ".join(clean_string_list(value.get("attempted_steps", [])))
        blocked = ", ".join(clean_string_list(value.get("blocked_steps", [])))
        parts = []
        if completed:
            parts.append(f"Completed: {completed}.")
        if attempted:
            parts.append(f"Attempted: {attempted}.")
        if blocked:
            parts.append(f"Blocked: {blocked}.")
        return " ".join(parts).strip()
    return str(value).strip()


def normalize_integration_review(payload: dict[str, Any]) -> IntegrationReview:
    return IntegrationReview(
        status=str(payload.get("status", "")).strip() or "error",
        verdict=str(payload.get("verdict", "")).strip(),
        functional_equivalence=str(payload.get("functional_equivalence", "")).strip(),
        quality_assessment=str(payload.get("quality_assessment", "")).strip(),
        issues=clean_string_list(payload.get("issues", [])),
        confidence=str(payload.get("confidence", "")).strip() or "low",
    )


def integration_review_error(message: str, *, issues: Iterable[Any] = ()) -> IntegrationReview:
    return IntegrationReview(
        status="error",
        verdict=message.strip(),
        functional_equivalence="The QA loop could not complete a grounded comparison against the post fixture.",
        quality_assessment="The generated integration could not be accepted because the expert review did not complete successfully.",
        issues=clean_string_list(issues),
        confidence="high",
    )


def normalize_qa_report(payload: dict[str, Any], *, default_loop_state: str) -> QAReport:
    title = str(payload.get("title", "")).strip()
    if not title:
        status = str(payload.get("status", "")).strip()
        title = f"QA Loop Report ({status})" if status else "QA Loop Report"

    overall_outcome = str(payload.get("overall_outcome", "")).strip()
    if not overall_outcome:
        overall_outcome = str(payload.get("overall_notes", "")).strip()
    if not overall_outcome:
        overall_outcome = summarize_report_item(payload.get("stop_reason_assessment"))

    loop_state = str(payload.get("loop_state", "")).strip() or default_loop_state
    integration_progress = normalize_integration_progress(payload.get("integration_progress", ""))
    ux_clarity = clean_report_items(payload.get("ux_clarity", []))
    if not ux_clarity:
        ux_clarity = clean_report_items(payload.get("ux_observations", []))

    return QAReport(
        title=title,
        overall_outcome=overall_outcome,
        loop_state=loop_state,
        integration_progress=integration_progress,
        ux_clarity=ux_clarity,
        product_issues=clean_report_items(payload.get("product_issues", [])),
        agent_interaction_issues=clean_report_items(payload.get("agent_interaction_issues", [])),
        suggestions=clean_report_items(payload.get("suggestions", [])),
        notable_moments=clean_report_items(payload.get("notable_moments", [])),
    )


def command_looks_like_claude_cli(command: str) -> bool:
    tokens = shlex.split(command)
    if not tokens:
        return False
    for token in tokens[:2]:
        if "claude" in Path(token).name.lower():
            return True
    return False


def format_codex_agent_payload(prefix: str, payload: dict[str, Any]) -> str:
    lines: list[str] = []

    action = str(payload.get("action", "")).strip()
    if action:
        headline = f"{prefix} action: {action}"
        input_text = str(payload.get("input_text", ""))
        if input_text:
            headline += f" -> {input_text}"
        loop_state = str(payload.get("loop_state", "")).strip()
        if loop_state:
            headline += f" ({loop_state})"
        lines.append(headline)

        summary = str(payload.get("summary", "")).strip()
        if summary:
            lines.append(f"{prefix} summary: {summary}")
        for issue in clean_string_list(payload.get("issues", [])):
            lines.append(f"{prefix} issue: {issue}")
        next_focus = str(payload.get("next_focus", "")).strip()
        if next_focus:
            lines.append(f"{prefix} next: {next_focus}")
        return "\n".join(lines) + "\n"

    if "title" in payload or "overall_outcome" in payload:
        title = str(payload.get("title", "")).strip() or "QA report"
        lines.append(f"{prefix} report: {title}")
        overall_outcome = str(payload.get("overall_outcome", "")).strip()
        if overall_outcome:
            lines.append(f"{prefix} outcome: {overall_outcome}")
        loop_state = str(payload.get("loop_state", "")).strip()
        if loop_state:
            lines.append(f"{prefix} loop state: {loop_state}")
        integration_progress = str(payload.get("integration_progress", "")).strip()
        if integration_progress:
            lines.append(f"{prefix} integration: {integration_progress}")
        for field_name, label in (
            ("ux_clarity", "ux"),
            ("product_issues", "product issue"),
            ("agent_interaction_issues", "agent issue"),
            ("suggestions", "suggestion"),
            ("notable_moments", "notable"),
        ):
            for item in clean_string_list(payload.get(field_name, [])):
                lines.append(f"{prefix} {label}: {item}")
        return "\n".join(lines) + "\n"

    if "status" in payload or "verdict" in payload:
        status = str(payload.get("status", "")).strip() or "unknown"
        lines.append(f"{prefix} integration review: {status}")
        verdict = str(payload.get("verdict", "")).strip()
        if verdict:
            lines.append(f"{prefix} verdict: {verdict}")
        functional_equivalence = str(payload.get("functional_equivalence", "")).strip()
        if functional_equivalence:
            lines.append(f"{prefix} equivalence: {functional_equivalence}")
        quality_assessment = str(payload.get("quality_assessment", "")).strip()
        if quality_assessment:
            lines.append(f"{prefix} quality: {quality_assessment}")
        for issue in clean_string_list(payload.get("issues", [])):
            lines.append(f"{prefix} issue: {issue}")
        confidence = str(payload.get("confidence", "")).strip()
        if confidence:
            lines.append(f"{prefix} confidence: {confidence}")
        return "\n".join(lines) + "\n"

    return ""


def format_codex_stream_event(session_label: str, line: str) -> str:
    prefix = f"[qa-loop][{session_label}]"
    stripped = line.strip()
    if not stripped:
        return line

    try:
        event = json.loads(stripped)
    except json.JSONDecodeError:
        return f"[qa-loop][{session_label} stdout] {line}"

    event_type = str(event.get("type", "")).strip()
    if event_type == "thread.started":
        thread_id = str(event.get("thread_id", "")).strip() or "unknown"
        return f"{prefix} thread started: {thread_id}\n"

    item = event.get("item")
    if not isinstance(item, dict):
        if event_type:
            return f"{prefix} event: {event_type}\n"
        return f"[qa-loop][{session_label} stdout] {line}"

    item_type = str(item.get("type", "")).strip()
    if item_type == "agent_message":
        text = coerce_text(item.get("text"))
        if text.strip():
            try:
                payload = json.loads(text)
            except json.JSONDecodeError:
                return f"{prefix} agent: {text.rstrip()}\n"
            if isinstance(payload, dict):
                formatted = format_codex_agent_payload(prefix, payload)
                if formatted:
                    return formatted
        return f"{prefix} agent message received\n"

    if item_type == "command_execution":
        command = str(item.get("command", "")).strip() or "<unknown command>"
        if event_type == "item.started":
            return f"{prefix} command started: {command}\n"

        status = str(item.get("status", "")).strip() or event_type.replace("item.", "")
        exit_code = item.get("exit_code")
        if status == "completed":
            if exit_code is None:
                lines = [f"{prefix} command completed: {command}"]
            else:
                lines = [f"{prefix} command completed (exit {exit_code}): {command}"]
        elif status == "failed":
            lines = [f"{prefix} command failed: {command}"]
        else:
            lines = [f"{prefix} command {status}: {command}"]

        aggregated_output = coerce_text(item.get("aggregated_output"))
        if aggregated_output.strip():
            lines.append(f"{prefix} command output:")
            lines.extend(indent_lines(aggregated_output))
        return "\n".join(lines) + "\n"

    if event_type and item_type:
        return f"{prefix} {event_type}: {item_type}\n"
    if event_type:
        return f"{prefix} event: {event_type}\n"
    return f"[qa-loop][{session_label} stdout] {line}"


def format_claude_stream_event(session_label: str, line: str) -> str:
    prefix = f"[qa-loop][{session_label}]"
    stripped = line.strip()
    if not stripped:
        return line

    try:
        event = json.loads(stripped)
    except json.JSONDecodeError:
        return f"[qa-loop][{session_label} stdout] {line}"

    if not isinstance(event, dict):
        return f"[qa-loop][{session_label} stdout] {line}"

    structured_output = event.get("structured_output")
    if isinstance(structured_output, dict):
        formatted = format_codex_agent_payload(prefix, structured_output)
        if formatted:
            return formatted

    event_type = str(event.get("type", "")).strip()
    subtype = str(event.get("subtype", "")).strip()
    if event_type == "result":
        result_payload = extract_json_dict_from_text(str(event.get("result", "")))
        if isinstance(result_payload, dict):
            formatted = format_codex_agent_payload(prefix, result_payload)
            if formatted:
                return formatted
    if event_type == "result" and subtype:
        return f"{prefix} result: {subtype}\n"
    if event_type:
        return f"{prefix} event: {event_type}\n"
    return f"[qa-loop][{session_label} stdout] {line}"


def format_compat_stream_event(session_label: str, line: str) -> str:
    rendered = format_claude_stream_event(session_label, line)
    if f"[qa-loop][{session_label}] event: result" != rendered.strip():
        return rendered
    return format_codex_stream_event(session_label, line)


def format_codex_stderr_event(session_label: str, line: str) -> str:
    stripped = line.strip()
    if not stripped:
        return line
    if should_hide_codex_stderr_line(stripped):
        return ""
    return f"[qa-loop][{session_label} stderr] {line}"


def format_claude_stderr_event(session_label: str, line: str) -> str:
    stripped = line.strip()
    if not stripped:
        return line
    if should_hide_claude_stderr_line(stripped):
        return ""
    return f"[qa-loop][{session_label} stderr] {line}"


def should_hide_codex_stderr_line(line: str) -> bool:
    lowered = line.lower()
    return (
        "codex_core::shell_snapshot" in lowered
        or "shell snapshot validation failed" in lowered
        or "syntax error in conditional expression: unexpected token" in lowered
    )


def should_hide_claude_stderr_line(line: str) -> bool:
    lowered = line.lower()
    return (
        "shell snapshot validation failed" in lowered
        or "syntax error in conditional expression: unexpected token" in lowered
    )


def run_streaming_subprocess(
    *,
    cmd: list[str],
    cwd: Path,
    env: dict[str, str] | None,
    timeout_seconds: float,
    input_text: str | None = None,
    live_io: LiveIO | None = None,
    stdout_prefix: str = "",
    stderr_prefix: str = "",
    stdout_formatter: Callable[[str], str] | None = None,
    stderr_formatter: Callable[[str], str] | None = None,
) -> dict[str, Any]:
    process = subprocess.Popen(
        cmd,
        cwd=str(cwd),
        env=env,
        stdin=subprocess.PIPE if input_text is not None else None,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        bufsize=1,
    )

    stdout_chunks: list[str] = []
    stderr_chunks: list[str] = []

    def stream_pipe(
        pipe: TextIO | None,
        chunks: list[str],
        *,
        prefix: str,
        emit: Callable[[str], None] | None,
        formatter: Callable[[str], str] | None = None,
    ) -> None:
        if pipe is None:
            return
        try:
            for line in iter(pipe.readline, ""):
                text = coerce_text(line)
                chunks.append(text)
                if emit is not None:
                    if formatter is not None:
                        emit(formatter(text))
                    else:
                        emit(f"{prefix}{text}" if prefix else text)
        finally:
            pipe.close()

    stdout_thread = threading.Thread(
        target=stream_pipe,
        args=(process.stdout, stdout_chunks),
        kwargs={
            "prefix": stdout_prefix,
            "emit": live_io.stdout if live_io is not None else None,
            "formatter": stdout_formatter,
        },
        daemon=True,
    )
    stderr_thread = threading.Thread(
        target=stream_pipe,
        args=(process.stderr, stderr_chunks),
        kwargs={
            "prefix": stderr_prefix,
            "emit": live_io.stderr if live_io is not None else None,
            "formatter": stderr_formatter,
        },
        daemon=True,
    )
    stdout_thread.start()
    stderr_thread.start()

    if input_text is not None and process.stdin is not None:
        process.stdin.write(input_text)
        process.stdin.close()

    try:
        returncode = process.wait(timeout=timeout_seconds)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
        stdout_thread.join(timeout=2.0)
        stderr_thread.join(timeout=2.0)
        raise subprocess.TimeoutExpired(
            cmd=cmd,
            timeout=timeout_seconds,
            output="".join(stdout_chunks),
            stderr="".join(stderr_chunks),
        )

    stdout_thread.join(timeout=2.0)
    stderr_thread.join(timeout=2.0)

    return {
        "returncode": returncode,
        "stdout": "".join(stdout_chunks),
        "stderr": "".join(stderr_chunks),
    }


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
        suggestions=["Inspect the saved transcript and Claude stderr log for the interrupted report step."],
        notable_moments=notable_moments,
    )


def provisional_report(*, summary: dict[str, Any], turns: list[dict[str, Any]]) -> QAReport:
    last_summary = ""
    notable_moments: list[str] = []
    product_issues: list[str] = []
    for turn in turns:
        directive = turn.get("directive", {})
        summary_text = str(directive.get("summary", "")).strip()
        if summary_text:
            last_summary = summary_text
        notable_moments.extend(clean_string_list(directive.get("issues", [])))
    if summary.get("stop_reason"):
        product_issues.append(f"Run stopped with `{summary['stop_reason']}`.")
    return QAReport(
        title="QA Loop Report (provisional)",
        overall_outcome="A provisional report was written before final Claude report synthesis completed.",
        loop_state=str(summary.get("loop_state", "")).strip(),
        integration_progress=last_summary or "The run finished before the synthesized report was available.",
        ux_clarity=[],
        product_issues=product_issues,
        agent_interaction_issues=[],
        suggestions=["Wait for the final report synthesis to finish, or inspect this provisional report if the supervisor is interrupted."],
        notable_moments=notable_moments,
    )


def write_report_artifacts(*, paths: RunPaths, run_id: str, summary: dict[str, Any], report: QAReport) -> None:
    paths.report_json.write_text(json.dumps(asdict(report), indent=2) + "\n", encoding="utf-8")
    paths.report_markdown.write_text(render_markdown_report(run_id, summary, report), encoding="utf-8")


def render_markdown_report(run_id: str, summary: dict[str, Any], report: QAReport) -> str:
    integration_review = summary.get("integration_review")
    lines = [
        f"# {report.title or 'QA Loop Report'}",
        "",
        f"- Run ID: `{run_id}`",
        f"- Outcome: {report.overall_outcome}",
        f"- Loop state: `{summary['loop_state']}`",
        f"- Stop reason: `{summary['stop_reason']}`",
        f"- Container: `{summary.get('docker', {}).get('container_name', '')}`",
        f"- Container workspace: `{summary['command_cwd']}`",
        "",
        "## Integration Progress",
        report.integration_progress or "No integration progress summary was provided.",
        "",
        "## Integration Review",
        *render_integration_review(integration_review if isinstance(integration_review, dict) else None),
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


def render_integration_review(review: dict[str, Any] | None) -> list[str]:
    if not review:
        return ["No expert integration review was recorded for this run."]

    lines = [
        f"Status: `{str(review.get('status', '')).strip() or 'unknown'}`",
        f"Confidence: `{str(review.get('confidence', '')).strip() or 'unknown'}`",
    ]
    verdict = str(review.get("verdict", "")).strip()
    if verdict:
        lines.extend(["", f"Verdict: {verdict}"])
    functional_equivalence = str(review.get("functional_equivalence", "")).strip()
    if functional_equivalence:
        lines.extend(["", f"Functional equivalence: {functional_equivalence}"])
    quality_assessment = str(review.get("quality_assessment", "")).strip()
    if quality_assessment:
        lines.extend(["", f"Quality assessment: {quality_assessment}"])
    lines.extend(["", "Issues:"])
    lines.extend(render_bullets(clean_string_list(review.get("issues", []))))
    return lines


def default_concierge_command(artifacts_root: Path) -> list[str]:
    _ = artifacts_root
    return ["/usr/local/bin/concierge", "run"]


def exit_code_for_loop_state(loop_state: str) -> int:
    return {
        "STOP_REPORT": 0,
        "STOP_FIX": 2,
        "STOP_DEADEND": 3,
    }.get(loop_state, 1)


def supervisor_stop_reason(loop_state: str) -> str:
    return {
        "STOP_REPORT": "supervisor_stop_report",
        "STOP_FIX": "supervisor_stop_fix",
        "STOP_DEADEND": "supervisor_stop_deadend",
    }.get(loop_state, "claude_stop")


def utc_now() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def utc_from_timestamp(value: float) -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(value))


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        raise SystemExit(130)
