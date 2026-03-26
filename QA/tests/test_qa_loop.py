from __future__ import annotations

import json
import os
import shlex
import subprocess
import sys
import tempfile
import textwrap
import time
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
QA_LOOP = ROOT / "qa_loop.py"
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

import qa_loop


def wait_for_file(path: Path, *, timeout_seconds: float = 5.0) -> bool:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        if path.is_file():
            return True
        time.sleep(0.05)
    return path.is_file()


def write_fake_docker(tmp: Path) -> Path:
    fake_docker = tmp / "fake_docker.py"
    fake_docker.write_text(
        textwrap.dedent(
            """
            #!/usr/bin/env python3
            import json
            import os
            import shutil
            import sys
            from pathlib import Path


            def containers() -> dict[str, Path]:
                raw = os.environ.get("FAKE_DOCKER_CONTAINERS", "{}")
                payload = json.loads(raw)
                return {name: Path(path).resolve() for name, path in payload.items()}


            def workspace_for(container: str) -> Path:
                try:
                    return containers()[container]
                except KeyError as exc:
                    raise SystemExit(f"unknown fake container: {container}") from exc


            def resolve_workspace_path(workspace: Path, path_text: str) -> Path:
                normalized = path_text or "/workspace"
                if normalized == "/workspace":
                    return workspace
                if normalized.startswith("/workspace/"):
                    return workspace / normalized.removeprefix("/workspace/")
                raise SystemExit(f"unsupported fake docker path: {path_text}")


            def main() -> int:
                args = sys.argv[1:]
                if not args:
                    raise SystemExit("missing docker command")

                command = args[0]
                if command == "exec":
                    workdir = "/workspace"
                    index = 1
                    while index < len(args):
                        token = args[index]
                        if token in {"-i", "-t"}:
                            index += 1
                            continue
                        if token == "-w":
                            workdir = args[index + 1]
                            index += 2
                            continue
                        if token.startswith("-"):
                            raise SystemExit(f"unsupported fake docker exec flag: {token}")
                        break
                    container = args[index]
                    inner_command = args[index + 1 :]
                    workspace = workspace_for(container)
                    cwd = resolve_workspace_path(workspace, workdir)
                    os.chdir(cwd)
                    env = os.environ.copy()
                    env["FAKE_DOCKER_CONTAINER_NAME"] = container
                    os.execvpe(inner_command[0], inner_command, env)

                if command == "commit":
                    container = args[1]
                    image_ref = args[2]
                    print(f"sha256:{abs(hash((container, image_ref))) & 0xffffffff:08x}")
                    return 0

                if command == "diff":
                    container = args[1]
                    workspace = workspace_for(container)
                    for path in sorted(workspace.rglob("*")):
                        if path.is_file():
                            print(f"A {path.relative_to(workspace)}")
                    return 0

                if command == "inspect":
                    target = args[1]
                    print(json.dumps([{"Id": f"fake-{target}", "Name": target, "RepoTags": [target]}]))
                    return 0

                if command == "cp":
                    source = args[1]
                    destination = Path(args[2]).resolve()
                    container, inner_path = source.split(":", 1)
                    workspace = workspace_for(container)
                    source_path = resolve_workspace_path(workspace, inner_path)
                    if not source_path.exists():
                        return 1
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    if source_path.is_dir():
                        shutil.copytree(source_path, destination, dirs_exist_ok=True)
                    else:
                        shutil.copy2(source_path, destination)
                    return 0

                raise SystemExit(f"unsupported fake docker command: {command}")


            if __name__ == "__main__":
                raise SystemExit(main())
            """
        ).strip()
        + "\n",
        encoding="utf-8",
    )
    fake_docker.chmod(0o755)
    return fake_docker


def qa_loop_command(
    *,
    artifacts_root: Path,
    fake_docker: Path,
    container_name: str,
    claude_command: str,
    target_command: list[str],
    extra_args: list[str] | None = None,
) -> list[str]:
    command = [
        sys.executable,
        str(QA_LOOP),
        "--artifacts-root",
        str(artifacts_root),
        "--docker-bin",
        str(fake_docker),
        "--container-name",
        container_name,
        "--container-workdir",
        "/workspace",
        "--claude-command",
        claude_command,
    ]
    if extra_args:
        command.extend(extra_args)
    command.append("--")
    command.extend(target_command)
    return command


class QALoopTest(unittest.TestCase):
    def test_parse_args_disables_docker_snapshots_by_default(self) -> None:
        args = qa_loop.parse_args(["--container-name", "fixture"])
        self.assertFalse(args.docker_snapshots)
        self.assertEqual(args.claude_command, "claude")
        self.assertEqual(args.claude_timeout_seconds, qa_loop.DEFAULT_CODEX_TIMEOUT_SECONDS)

    def test_parse_args_uses_50_control_turns_by_default(self) -> None:
        args = qa_loop.parse_args(["--container-name", "fixture"])
        self.assertEqual(qa_loop.DEFAULT_MAX_ITERATIONS, 50)
        self.assertEqual(args.max_iterations, 50)

    def test_parse_args_accepts_claude_runner_surface(self) -> None:
        args = qa_loop.parse_args(
            [
                "--container-name",
                "fixture",
                "--claude-command",
                "claude",
                "--claude-timeout-seconds",
                "45",
            ]
        )

        self.assertEqual(args.claude_command, "claude")
        self.assertEqual(args.claude_timeout_seconds, 45.0)

    def test_parse_args_accepts_issue_evidence_context(self) -> None:
        args = qa_loop.parse_args(
            [
                "--container-name",
                "fixture",
                "--fixture-id",
                "mnist",
                "--guide-step",
                "preprocess",
                "--ref-under-test",
                "feature/issue-90@abc1234",
                "--checkpoint-key",
                "mnist:preprocess",
                "--source-kind",
                "case",
                "--source-id",
                "mnist_missing_entrypoint",
            ]
        )

        self.assertEqual(args.fixture_id, "mnist")
        self.assertEqual(args.guide_step, "preprocess")
        self.assertEqual(args.ref_under_test, "feature/issue-90@abc1234")
        self.assertEqual(args.checkpoint_key, "mnist:preprocess")
        self.assertEqual(args.source_kind, "case")
        self.assertEqual(args.source_id, "mnist_missing_entrypoint")

    def test_prepare_run_paths_uses_claude_artifact_directory(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            paths = qa_loop.prepare_run_paths(Path(tmpdir), "run-123")

            self.assertEqual(paths.claude_dir, Path(tmpdir) / "runs" / "run-123" / "claude")
            self.assertTrue(paths.claude_dir.is_dir())
            self.assertFalse((Path(tmpdir) / "runs" / "run-123" / "codex").exists())

    def test_request_report_caps_final_report_timeout_at_120_seconds(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            artifacts_root.mkdir()
            paths = qa_loop.prepare_run_paths(artifacts_root, "run-123")
            live_io = qa_loop.LiveIO(transcript_path=paths.full_transcript)

            class FakeClaudeClient:
                def __init__(self) -> None:
                    self.calls: list[dict[str, object]] = []

                def run_structured(self, **kwargs: object) -> dict[str, object]:
                    self.calls.append(kwargs)
                    return {
                        "title": "Synthetic QA Report",
                        "overall_outcome": "Reached the completion path.",
                        "loop_state": "STOP_REPORT",
                        "integration_progress": "The report writer completed.",
                        "ux_clarity": [],
                        "product_issues": [],
                        "agent_interaction_issues": [],
                        "suggestions": [],
                        "notable_moments": [],
                    }

            fake_client = FakeClaudeClient()
            config = qa_loop.LoopConfig(
                artifacts_root=artifacts_root,
                docker_bin="docker",
                host_cwd=tmp,
                container_name="fixture",
                container_image=None,
                command=["/usr/local/bin/concierge", "run"],
                command_cwd="/workspace",
                claude_command="claude",
                claude_model=None,
                claude_timeout_seconds=qa_loop.DEFAULT_CODEX_TIMEOUT_SECONDS,
                max_iterations=qa_loop.DEFAULT_MAX_ITERATIONS,
                max_idle_turns=qa_loop.DEFAULT_MAX_IDLE_TURNS,
                max_runtime_seconds=qa_loop.DEFAULT_MAX_RUNTIME_SECONDS,
                read_quiet_seconds=qa_loop.DEFAULT_READ_QUIET_SECONDS,
                read_timeout_seconds=qa_loop.DEFAULT_READ_TIMEOUT_SECONDS,
                settle_timeout_seconds=qa_loop.DEFAULT_SETTLE_TIMEOUT_SECONDS,
                transcript_tail_chars=qa_loop.DEFAULT_TRANSCRIPT_TAIL_CHARS,
                latest_output_chars=qa_loop.DEFAULT_LATEST_OUTPUT_CHARS,
                fixture_post_path=None,
                docker_snapshots_enabled=False,
                fixture_id="ultralytics",
                guide_step="pre",
                ref_under_test="main@abc1234",
                checkpoint_key="ultralytics:pre",
                source_kind="variant",
                source_id="pre",
            )
            supervisor = qa_loop.SupervisorLoop(
                config=config,
                claude_client=fake_client,
                role_prompt="role",
                nudge_prompt="nudge",
            )

            supervisor._request_report(
                paths=paths,
                summary={"blind_first_released": False},
                loop_state="STOP_REPORT",
                live_io=live_io,
            )

            self.assertEqual(fake_client.calls[0]["timeout_seconds"], 120.0)

    def test_request_report_uses_compact_context_without_artifact_tooling(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            artifacts_root.mkdir()
            paths = qa_loop.prepare_run_paths(artifacts_root, "run-123")
            live_io = qa_loop.LiveIO(transcript_path=paths.full_transcript)
            paths.turns_jsonl.write_text(
                "\n".join(
                    [
                        json.dumps(
                            {
                                "iteration": 1,
                                "directive": {
                                    "action": "SEND_INPUT",
                                    "input_text": "y",
                                    "loop_state": "CONTINUE",
                                    "summary": "Accepted the environment repair step.",
                                    "issues": [],
                                    "next_focus": "Wait for validation.",
                                },
                            }
                        ),
                        json.dumps(
                            {
                                "iteration": 2,
                                "directive": {
                                    "action": "WAIT",
                                    "input_text": "",
                                    "loop_state": "STOP_FIX",
                                    "summary": "Validation kept reporting the same missing decorator.",
                                    "issues": ["The same encoder failure repeated after a commit."],
                                    "next_focus": "Escalate the loop.",
                                },
                            }
                        ),
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            class FakeClaudeClient:
                def __init__(self) -> None:
                    self.calls: list[dict[str, object]] = []

                def run_structured(self, **kwargs: object) -> dict[str, object]:
                    self.calls.append(kwargs)
                    return {
                        "title": "Synthetic QA Report",
                        "overall_outcome": "Reached the completion path.",
                        "loop_state": "STOP_FIX",
                        "integration_progress": "The report writer completed.",
                        "ux_clarity": [],
                        "product_issues": [],
                        "agent_interaction_issues": [],
                        "suggestions": [],
                        "notable_moments": [],
                    }

            fake_client = FakeClaudeClient()
            config = qa_loop.LoopConfig(
                artifacts_root=artifacts_root,
                docker_bin="docker",
                host_cwd=tmp,
                container_name="fixture",
                container_image=None,
                command=["/usr/local/bin/concierge", "run"],
                command_cwd="/workspace",
                claude_command="claude",
                claude_model=None,
                claude_timeout_seconds=qa_loop.DEFAULT_CODEX_TIMEOUT_SECONDS,
                max_iterations=qa_loop.DEFAULT_MAX_ITERATIONS,
                max_idle_turns=qa_loop.DEFAULT_MAX_IDLE_TURNS,
                max_runtime_seconds=qa_loop.DEFAULT_MAX_RUNTIME_SECONDS,
                read_quiet_seconds=qa_loop.DEFAULT_READ_QUIET_SECONDS,
                read_timeout_seconds=qa_loop.DEFAULT_READ_TIMEOUT_SECONDS,
                settle_timeout_seconds=qa_loop.DEFAULT_SETTLE_TIMEOUT_SECONDS,
                transcript_tail_chars=qa_loop.DEFAULT_TRANSCRIPT_TAIL_CHARS,
                latest_output_chars=qa_loop.DEFAULT_LATEST_OUTPUT_CHARS,
                fixture_post_path=None,
                docker_snapshots_enabled=False,
                fixture_id="ultralytics",
                guide_step="pre",
                ref_under_test="main@abc1234",
                checkpoint_key="ultralytics:pre",
                source_kind="variant",
                source_id="pre",
            )
            supervisor = qa_loop.SupervisorLoop(
                config=config,
                claude_client=fake_client,
                role_prompt="role",
                nudge_prompt="nudge",
            )

            summary = {
                "run_id": "run-123",
                "loop_state": "STOP_FIX",
                "stop_reason": "supervisor_stop_fix",
                "iterations_completed": 2,
                "idle_turns": 0,
                "blind_first_released": False,
                "qa_context": {
                    "fixture_id": "ultralytics",
                    "guide_step": "pre",
                    "ref_under_test": "main@abc1234",
                    "checkpoint_key": "ultralytics:pre",
                    "source_kind": "variant",
                    "source_id": "pre",
                },
            }

            supervisor._request_report(
                paths=paths,
                summary=summary,
                loop_state="STOP_FIX",
                live_io=live_io,
            )

            call = fake_client.calls[0]
            prompt = str(call["prompt"])

            self.assertEqual(call["allowed_tools"], None)
            self.assertEqual(list(call["add_dirs"]), [])
            self.assertIn('"run_id": "run-123"', prompt)
            self.assertIn("Accepted the environment repair step.", prompt)
            self.assertIn("Validation kept reporting the same missing decorator.", prompt)
            self.assertNotIn("Review these files if needed", prompt)
            self.assertNotIn(str(paths.summary_json), prompt)
            self.assertNotIn(str(paths.terminal_clean), prompt)

    def test_supervisor_loop_writes_transcript_and_report(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()
            fake_docker = write_fake_docker(tmp)
            container_name = "fixture"

            concierge_script = tmp / "fake_concierge.py"
            concierge_script.write_text(
                textwrap.dedent(
                    """
                    import sys

                    print("Welcome to Concierge", flush=True)
                    print("Type YES to continue:", flush=True)
                    answer = input()
                    print(f"Input received: {answer}", flush=True)
                    if answer.strip().lower() == "yes":
                        print("Integration complete", flush=True)
                    else:
                        print("Blocked", flush=True)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            claude_script = tmp / "fake_claude.py"
            claude_script.write_text(
                textwrap.dedent(
                    """
                    #!/usr/bin/env python3
                    import json
                    import os
                    import sys
                    from pathlib import Path

                    prompt = sys.stdin.read()
                    state_path = Path(os.environ["FAKE_CODEX_STATE"])
                    if state_path.exists():
                        state = json.loads(state_path.read_text(encoding="utf-8"))
                    else:
                        state = {"turn": 0}

                    if "final qualitative QA report" in prompt:
                        payload = {
                            "title": "Synthetic QA Report",
                            "overall_outcome": "Reached the completion path.",
                            "loop_state": "STOP_REPORT",
                            "integration_progress": "The flow reached the final completion message after one affirmative input.",
                            "ux_clarity": ["The prompt for YES was easy to follow."],
                            "product_issues": [],
                            "agent_interaction_issues": [],
                            "suggestions": ["Keep the completion message concise."],
                            "notable_moments": ["Claude answered YES and Concierge completed immediately."]
                        }
                    else:
                        state["turn"] += 1
                        if state["turn"] == 1:
                            payload = {
                                "action": "SEND_INPUT",
                                "input_text": "YES",
                                "loop_state": "CONTINUE",
                                "summary": "Advancing through the first prompt.",
                                "issues": [],
                                "next_focus": "Wait for the resulting terminal output."
                            }
                        else:
                            payload = {
                                "action": "WAIT",
                                "input_text": "",
                                "loop_state": "STOP_REPORT",
                                "summary": "The session reached a clean stopping point.",
                                "issues": [],
                                "next_focus": "Write the final report."
                            }
                        state_path.write_text(json.dumps(state), encoding="utf-8")

                    print(json.dumps({"type": "result", "subtype": "success", "structured_output": payload}))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            claude_script.chmod(0o755)

            env = os.environ.copy()
            env["FAKE_CODEX_STATE"] = str(tmp / "fake_codex_state.json")
            env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})

            completed = subprocess.run(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    claude_command=str(claude_script),
                    target_command=[sys.executable, str(concierge_script)],
                    extra_args=["--docker-snapshots"],
                ),
                cwd=str(ROOT),
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout:\n{completed.stdout}\n\nstderr:\n{completed.stderr}",
            )
            self.assertIn("Welcome to Concierge", completed.stdout)
            self.assertIn("[qa-loop][claude-control-001] action: SEND_INPUT -> YES (CONTINUE)", completed.stdout)

            run_dirs = sorted((artifacts_root / "runs").iterdir())
            self.assertEqual(len(run_dirs), 1)
            run_dir = run_dirs[0]

            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["loop_state"], "STOP_REPORT")
            self.assertEqual(summary["report_status"], "ready")
            self.assertEqual(summary["docker"]["container_name"], container_name)
            self.assertTrue(summary["docker"]["snapshots_enabled"])
            self.assertEqual(len(summary["docker"]["snapshots"]), 2)
            first_snapshot = summary["docker"]["snapshots"][0]
            self.assertTrue(Path(first_snapshot["diff_path"]).is_file())
            self.assertTrue(Path(first_snapshot["inspect_path"]).is_file())

            report_path = artifacts_root / "reports" / f"{run_dir.name}.md"
            self.assertTrue(report_path.is_file())
            report_body = report_path.read_text(encoding="utf-8")
            self.assertIn("Synthetic QA Report", report_body)
            self.assertIn("Reached the completion path.", report_body)

            transcript_path = artifacts_root / "transcripts" / f"{run_dir.name}.terminal.txt"
            transcript = transcript_path.read_text(encoding="utf-8")
            self.assertIn("Welcome to Concierge", transcript)
            self.assertIn("Integration complete", transcript)

            full_transcript_path = artifacts_root / "transcripts" / f"{run_dir.name}.full.txt"
            self.assertTrue(full_transcript_path.is_file())
            self.assertEqual(Path(summary["paths"]["full_transcript"]).resolve(), full_transcript_path.resolve())
            full_transcript = full_transcript_path.read_text(encoding="utf-8")
            self.assertIn("Welcome to Concierge", full_transcript)
            self.assertIn("[qa-loop] input -> YES", full_transcript)
            self.assertIn("[qa-loop] --- claude-control-001 prompt begin ---", full_transcript)
            self.assertIn("[qa-loop][claude-control-001] action: SEND_INPUT -> YES (CONTINUE)", full_transcript)
            self.assertIn(f"[qa-loop] transcript: {full_transcript_path.resolve()}", completed.stdout)

            event_log_path = run_dir / "claude" / "turn-001.jsonl"
            self.assertTrue(event_log_path.is_file())
            event_log = event_log_path.read_text(encoding="utf-8")
            self.assertIn('"structured_output"', event_log)

    def test_format_codex_stream_event_renders_command_execution_as_text(self) -> None:
        rendered = qa_loop.format_codex_stream_event(
            "codex-final-report",
            json.dumps(
                {
                    "type": "item.completed",
                    "item": {
                        "id": "item-9",
                        "type": "command_execution",
                        "command": "/bin/bash -lc 'echo hi'",
                        "aggregated_output": "first line\nsecond line\n",
                        "exit_code": 0,
                        "status": "completed",
                    },
                }
            )
            + "\n",
        )

        self.assertIn("[qa-loop][codex-final-report] command completed (exit 0): /bin/bash -lc 'echo hi'", rendered)
        self.assertIn("[qa-loop][codex-final-report] command output:", rendered)
        self.assertIn("    first line", rendered)
        self.assertIn("    second line", rendered)

    def test_format_claude_stream_event_renders_structured_output_as_text(self) -> None:
        rendered = qa_loop.format_claude_stream_event(
            "claude-final-report",
            json.dumps(
                {
                    "type": "result",
                    "subtype": "success",
                    "structured_output": {
                        "title": "Synthetic QA Report",
                        "overall_outcome": "Reached the completion path.",
                        "loop_state": "STOP_REPORT",
                        "integration_progress": "The flow completed after one response.",
                        "ux_clarity": ["The primary prompt was clear."],
                        "product_issues": [],
                        "agent_interaction_issues": [],
                        "suggestions": ["Keep the summary short."],
                        "notable_moments": ["Claude answered YES and Concierge completed immediately."],
                    },
                }
            )
            + "\n",
        )

        self.assertIn("[qa-loop][claude-final-report] report: Synthetic QA Report", rendered)
        self.assertIn("[qa-loop][claude-final-report] outcome: Reached the completion path.", rendered)
        self.assertIn("[qa-loop][claude-final-report] notable: Claude answered YES and Concierge completed immediately.", rendered)

    def test_format_claude_stream_event_renders_fenced_result_payload_as_text(self) -> None:
        rendered = qa_loop.format_claude_stream_event(
            "claude-control-001",
            json.dumps(
                {
                    "type": "result",
                    "subtype": "success",
                    "result": "```json\n"
                    '{"action": "SEND_INPUT", "input_text": "y", "loop_state": "CONTINUE", "observation": "Approve the requested action."}'
                    "\n```",
                }
            )
            + "\n",
        )

        self.assertIn("[qa-loop][claude-control-001] action: SEND_INPUT -> y (CONTINUE)", rendered)

    def test_extract_claude_structured_output_parses_fenced_result_payload(self) -> None:
        payload = qa_loop.extract_claude_structured_output(
            json.dumps(
                {
                    "type": "result",
                    "subtype": "success",
                    "result": "```json\n"
                    '{"action": "SEND_INPUT", "input_text": "y", "loop_state": "CONTINUE", "observation": "Approve the requested action."}'
                    "\n```",
                }
            )
        )

        self.assertEqual(payload["action"], "SEND_INPUT")
        self.assertEqual(payload["input_text"], "y")
        self.assertEqual(payload["loop_state"], "CONTINUE")

    def test_normalize_qa_report_handles_alternative_claude_result_shape(self) -> None:
        report = qa_loop.normalize_qa_report(
            {
                "status": "fail",
                "integration_progress": {
                    "summary": "Session failed at the first actionable step.",
                },
                "ux_observations": [
                    "The confirmation prompt was clear.",
                ],
                "product_issues": [
                    {
                        "severity": "critical",
                        "area": "loop_control",
                        "description": "Structured output was missing from the Claude response.",
                        "suggestion": "Retry once before declaring a dead-end.",
                    }
                ],
                "agent_interaction_issues": [
                    {"description": "The harness could not parse the model response."}
                ],
                "suggestions": [
                    "Surface a clearer supervisor error in the terminal.",
                ],
                "overall_notes": "The run failed due to a harness-level control protocol issue.",
            },
            default_loop_state="STOP_DEADEND",
        )

        self.assertEqual(report.title, "QA Loop Report (fail)")
        self.assertEqual(report.loop_state, "STOP_DEADEND")
        self.assertEqual(report.integration_progress, "Session failed at the first actionable step.")
        self.assertEqual(report.ux_clarity, ["The confirmation prompt was clear."])
        self.assertEqual(
            report.product_issues,
            [
                "[critical / loop_control] Structured output was missing from the Claude response. Suggestion: Retry once before declaring a dead-end."
            ],
        )
        self.assertEqual(report.agent_interaction_issues, ["The harness could not parse the model response."])
        self.assertEqual(report.suggestions, ["Surface a clearer supervisor error in the terminal."])
        self.assertEqual(report.overall_outcome, "The run failed due to a harness-level control protocol issue.")

    def test_claude_client_extracts_structured_output_from_result_envelope(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            workspace_root = tmp / "workspace"
            artifacts_root = tmp / "artifacts"
            workspace_root.mkdir()
            artifacts_root.mkdir()

            claude_script = tmp / "fake_claude_stderr.py"
            claude_script.write_text(
                textwrap.dedent(
                    """
                    import json
                    import sys

                    sys.stderr.write("Shell snapshot validation failed\\n")
                    payload = {
                        "type": "result",
                        "subtype": "success",
                        "structured_output": {
                            "action": "WAIT",
                            "summary": "Keep waiting for the next prompt.",
                        },
                    }
                    print(json.dumps(payload))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            claude_script.chmod(0o755)

            client = qa_loop.ClaudeClient(
                workspace_root=workspace_root,
                artifacts_root=artifacts_root,
                command=f"{sys.executable} {claude_script}",
            )
            transcript_path = tmp / "transcript.txt"
            output_path = tmp / "output.json"
            event_log_path = tmp / "events.jsonl"
            stderr_log_path = tmp / "stderr.log"
            schema_path = tmp / "schema.json"
            schema_path.write_text("{}", encoding="utf-8")

            payload = client.run_structured(
                prompt="probe",
                schema_path=schema_path,
                output_path=output_path,
                event_log_path=event_log_path,
                stderr_log_path=stderr_log_path,
                live_io=qa_loop.LiveIO(transcript_path=transcript_path),
                session_label="claude-control-001",
            )

            self.assertEqual(payload["action"], "WAIT")
            self.assertEqual(payload["summary"], "Keep waiting for the next prompt.")
            self.assertEqual(json.loads(output_path.read_text(encoding="utf-8"))["action"], "WAIT")

            event_log = event_log_path.read_text(encoding="utf-8")
            self.assertIn('"structured_output"', event_log)

            stderr_log = stderr_log_path.read_text(encoding="utf-8")
            self.assertIn("Shell snapshot validation failed", stderr_log)

            transcript = transcript_path.read_text(encoding="utf-8")
            self.assertNotIn("Shell snapshot validation failed", transcript)

    def test_claude_client_extracts_fenced_json_from_result_text(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            workspace_root = tmp / "workspace"
            artifacts_root = tmp / "artifacts"
            workspace_root.mkdir()
            artifacts_root.mkdir()

            claude_script = tmp / "fake_claude_markdown.py"
            claude_script.write_text(
                textwrap.dedent(
                    """
                    import json

                    payload = {
                        "type": "result",
                        "subtype": "success",
                        "result": "```json\\n"
                        "{\\"action\\": \\"SEND_INPUT\\", \\"input_text\\": \\"y\\", \\"loop_state\\": \\"CONTINUE\\", \\"observation\\": \\"Approve the requested action.\\"}"
                        "\\n```",
                    }
                    print(json.dumps(payload))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            claude_script.chmod(0o755)

            client = qa_loop.ClaudeClient(
                workspace_root=workspace_root,
                artifacts_root=artifacts_root,
                command=f"{sys.executable} {claude_script}",
            )
            output_path = tmp / "output.json"
            event_log_path = tmp / "events.jsonl"
            stderr_log_path = tmp / "stderr.log"
            schema_path = tmp / "schema.json"
            schema_path.write_text("{}", encoding="utf-8")

            payload = client.run_structured(
                prompt="probe",
                schema_path=schema_path,
                output_path=output_path,
                event_log_path=event_log_path,
                stderr_log_path=stderr_log_path,
            )

            self.assertEqual(payload["action"], "SEND_INPUT")
            self.assertEqual(payload["input_text"], "y")
            self.assertEqual(payload["loop_state"], "CONTINUE")

    def test_claude_client_sends_prompt_via_stdin_for_claude_cli(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            workspace_root = tmp / "workspace"
            artifacts_root = tmp / "artifacts"
            workspace_root.mkdir()
            artifacts_root.mkdir()

            claude_script = tmp / "claude"
            claude_script.write_text(
                textwrap.dedent(
                    """
                    #!/usr/bin/env python3
                    import json
                    import sys

                    args = sys.argv[1:]
                    index = 0
                    while index < len(args):
                        token = args[index]
                        if token in {"--print", "--no-session-persistence"}:
                            index += 1
                            continue
                        if token in {
                            "--output-format",
                            "--json-schema",
                            "--permission-mode",
                            "--allowedTools",
                            "--allowed-tools",
                            "--model",
                            "--add-dir",
                        }:
                            index += 2
                            continue
                        sys.stderr.write(f"unexpected positional argument: {token}\\n")
                        raise SystemExit(2)

                    prompt = sys.stdin.read()
                    if not prompt.strip():
                        sys.stderr.write(
                            "Error: Input must be provided either through stdin or as a prompt argument when using --print\\n"
                        )
                        raise SystemExit(1)

                    payload = {
                        "type": "result",
                        "subtype": "success",
                        "structured_output": {
                            "action": "WAIT",
                            "summary": "Keep waiting for the next prompt.",
                        },
                    }
                    print(json.dumps(payload))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            claude_script.chmod(0o755)

            client = qa_loop.ClaudeClient(
                workspace_root=workspace_root,
                artifacts_root=artifacts_root,
                command=str(claude_script),
            )
            transcript_path = tmp / "transcript.txt"
            output_path = tmp / "output.json"
            event_log_path = tmp / "events.jsonl"
            stderr_log_path = tmp / "stderr.log"
            schema_path = tmp / "schema.json"
            schema_path.write_text("{}", encoding="utf-8")

            payload = client.run_structured(
                prompt="probe",
                schema_path=schema_path,
                output_path=output_path,
                event_log_path=event_log_path,
                stderr_log_path=stderr_log_path,
                live_io=qa_loop.LiveIO(transcript_path=transcript_path),
                session_label="claude-control-stdin",
            )

            self.assertEqual(payload["action"], "WAIT")
            self.assertEqual(payload["summary"], "Keep waiting for the next prompt.")

    def test_claude_client_persists_partial_streams_when_report_times_out(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            workspace_root = tmp / "workspace"
            artifacts_root = tmp / "artifacts"
            workspace_root.mkdir()
            artifacts_root.mkdir()

            claude_script = tmp / "claude"
            claude_script.write_text(
                textwrap.dedent(
                    """
                    #!/usr/bin/env python3
                    import json
                    import sys
                    import time

                    sys.stdin.read()
                    for index in range(3):
                        print(json.dumps({"type": "thread.started", "thread_id": f"final-report-thread-{index}"}), flush=True)
                        sys.stderr.write(f"final report synthesis is still running {index}\\n")
                        sys.stderr.flush()
                        time.sleep(0.1)
                    time.sleep(2.0)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            claude_script.chmod(0o755)

            client = qa_loop.ClaudeClient(
                workspace_root=workspace_root,
                artifacts_root=artifacts_root,
                command=str(claude_script),
                timeout_seconds=0.5,
            )
            output_path = tmp / "output.json"
            event_log_path = tmp / "events.jsonl"
            stderr_log_path = tmp / "stderr.log"
            schema_path = tmp / "schema.json"
            schema_path.write_text("{}", encoding="utf-8")

            with self.assertRaises(qa_loop.ClaudeInvocationError) as exc:
                client.run_structured(
                    prompt="probe",
                    schema_path=schema_path,
                    output_path=output_path,
                    event_log_path=event_log_path,
                    stderr_log_path=stderr_log_path,
                )

            self.assertIn(str(stderr_log_path), str(exc.exception))
            self.assertTrue(event_log_path.is_file())
            self.assertTrue(stderr_log_path.is_file())
            self.assertIn("final-report-thread-0", event_log_path.read_text(encoding="utf-8"))
            self.assertIn("final report synthesis is still running 0", stderr_log_path.read_text(encoding="utf-8"))
            self.assertFalse(output_path.exists())

    def test_codex_client_keeps_shell_snapshot_stderr_in_log_but_hides_it_from_live_transcript(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            workspace_root = tmp / "workspace"
            artifacts_root = tmp / "artifacts"
            workspace_root.mkdir()
            artifacts_root.mkdir()

            codex_script = tmp / "fake_codex_stderr.py"
            codex_script.write_text(
                textwrap.dedent(
                    """
                    import json
                    import sys
                    from pathlib import Path

                    output_path = None
                    for index, value in enumerate(sys.argv):
                        if value == "-o":
                            output_path = Path(sys.argv[index + 1])
                            break
                    if output_path is None:
                        raise SystemExit("missing -o")

                    sys.stderr.write("ERROR codex_core::shell_snapshot: Shell snapshot validation failed\\n")
                    sys.stderr.write("/bin/bash: line 1: syntax error in conditional expression: unexpected token '('\\n")
                    output_path.write_text(json.dumps({"action": "WAIT"}), encoding="utf-8")
                    print(json.dumps({"type": "thread.started", "thread_id": "fake-thread"}))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            codex_script.chmod(0o755)

            client = qa_loop.CodexClient(
                workspace_root=workspace_root,
                artifacts_root=artifacts_root,
                command=f"{sys.executable} {codex_script}",
            )
            transcript_path = tmp / "transcript.txt"
            output_path = tmp / "output.json"
            event_log_path = tmp / "events.jsonl"
            stderr_log_path = tmp / "stderr.log"
            schema_path = tmp / "schema.json"
            schema_path.write_text("{}", encoding="utf-8")

            payload = client.run_structured(
                prompt="probe",
                schema_path=schema_path,
                output_path=output_path,
                event_log_path=event_log_path,
                stderr_log_path=stderr_log_path,
                live_io=qa_loop.LiveIO(transcript_path=transcript_path),
                session_label="codex-control-001",
            )

            self.assertEqual(payload["action"], "WAIT")
            stderr_log = stderr_log_path.read_text(encoding="utf-8")
            self.assertIn("codex_core::shell_snapshot", stderr_log)
            self.assertIn("unexpected token '('", stderr_log)

            transcript = transcript_path.read_text(encoding="utf-8")
            self.assertNotIn("codex_core::shell_snapshot", transcript)
            self.assertNotIn("unexpected token '('", transcript)

    def test_supervisor_loop_stops_cleanly_when_target_exits_before_followup_input(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()
            fake_docker = write_fake_docker(tmp)
            container_name = "fixture"

            concierge_script = tmp / "fake_concierge_exit.py"
            concierge_script.write_text(
                textwrap.dedent(
                    """
                    print("Manual step required: run poetry install", flush=True)
                    raise SystemExit(1)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            codex_script = tmp / "fake_codex_exit.py"
            codex_script.write_text(
                textwrap.dedent(
                    """
                    import json
                    import os
                    import sys
                    from pathlib import Path

                    args = sys.argv[1:]
                    output_path = None
                    for index, value in enumerate(args):
                        if value == "-o":
                            output_path = Path(args[index + 1])
                            break
                    if output_path is None:
                        raise SystemExit("missing -o")

                    prompt = sys.stdin.read()
                    state_path = Path(os.environ["FAKE_CODEX_STATE"])
                    if state_path.exists():
                        state = json.loads(state_path.read_text(encoding="utf-8"))
                    else:
                        state = {"turn": 0}

                    if "final qualitative QA report" in prompt:
                        payload = {
                            "title": "Exit handling report",
                            "overall_outcome": "The target exited before follow-up input could be sent.",
                            "loop_state": "STOP_REPORT",
                            "integration_progress": "The harness produced a report instead of crashing.",
                            "ux_clarity": [],
                            "product_issues": [],
                            "agent_interaction_issues": [],
                            "suggestions": [],
                            "notable_moments": ["The target exited before the synthetic user could continue."]
                        }
                    else:
                        state["turn"] += 1
                        payload = {
                            "action": "SEND_INPUT",
                            "input_text": "poetry install",
                            "loop_state": "CONTINUE",
                            "summary": "Attempt the suggested manual recovery step.",
                            "issues": [],
                            "next_focus": "See whether the target is still available for input."
                        }
                        state_path.write_text(json.dumps(state), encoding="utf-8")

                    output_path.parent.mkdir(parents=True, exist_ok=True)
                    output_path.write_text(json.dumps(payload), encoding="utf-8")
                    print(json.dumps({"type": "thread.started", "thread_id": "fake-thread"}))
                    print(json.dumps({"type": "item.completed", "item": {"id": "item-1", "type": "agent_message", "text": json.dumps(payload)}}))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            codex_script.chmod(0o755)

            env = os.environ.copy()
            env["FAKE_CODEX_STATE"] = str(tmp / "fake_codex_exit_state.json")
            env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})

            completed = subprocess.run(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    claude_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                ),
                cwd=str(ROOT),
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout:\n{completed.stdout}\n\nstderr:\n{completed.stderr}",
            )

            run_dir = next((artifacts_root / "runs").iterdir())
            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["stop_reason"], "process_exit_before_input")

            interaction_log = artifacts_root / "transcripts" / f"{run_dir.name}.interaction.jsonl"
            interaction_body = interaction_log.read_text(encoding="utf-8")
            self.assertIn("input_skipped", interaction_body)

            report_path = artifacts_root / "reports" / f"{run_dir.name}.md"
            self.assertTrue(report_path.is_file())

    def test_supervisor_loop_preserves_semantic_stop_reason_for_non_continue_directives(self) -> None:
        cases = [
            ("STOP_REPORT", "supervisor_stop_report", 0),
            ("STOP_FIX", "supervisor_stop_fix", 2),
            ("STOP_DEADEND", "supervisor_stop_deadend", 3),
        ]

        for loop_state, expected_reason, expected_returncode in cases:
            with self.subTest(loop_state=loop_state):
                with tempfile.TemporaryDirectory() as tmpdir:
                    tmp = Path(tmpdir)
                    artifacts_root = tmp / "artifacts"
                    command_cwd = tmp / "fixture"
                    artifacts_root.mkdir()
                    command_cwd.mkdir()
                    fake_docker = write_fake_docker(tmp)
                    container_name = "fixture"

                    concierge_script = tmp / "fake_concierge_waiting.py"
                    concierge_script.write_text(
                        textwrap.dedent(
                            """
                            import time

                            print("Continue now? [y/N]: ", end="", flush=True)
                            time.sleep(30)
                            """
                        ).strip()
                        + "\n",
                        encoding="utf-8",
                    )

                    codex_script = tmp / "fake_codex_semantic_stop.py"
                    codex_script.write_text(
                        textwrap.dedent(
                            """
                            import json
                            import os
                            import sys
                            from pathlib import Path

                            args = sys.argv[1:]
                            output_path = None
                            for index, value in enumerate(args):
                                if value == "-o":
                                    output_path = Path(args[index + 1])
                                    break
                            if output_path is None:
                                raise SystemExit("missing -o")

                            prompt = sys.stdin.read()
                            loop_state = os.environ["FAKE_LOOP_STATE"]
                            if "final qualitative QA report" in prompt:
                                payload = {
                                    "title": f"{loop_state} report",
                                    "overall_outcome": f"The supervisor ended the loop with {loop_state}.",
                                    "loop_state": loop_state,
                                    "integration_progress": "The QA loop persisted the supervisor decision.",
                                    "ux_clarity": [],
                                    "product_issues": [],
                                    "agent_interaction_issues": [],
                                    "suggestions": [],
                                    "notable_moments": [f"The supervisor deliberately chose {loop_state}."]
                                }
                            else:
                                payload = {
                                    "action": "WAIT",
                                    "input_text": "",
                                    "loop_state": loop_state,
                                    "summary": f"Stop the run with {loop_state}.",
                                    "issues": [],
                                    "next_focus": "Write the final report."
                                }

                            output_path.parent.mkdir(parents=True, exist_ok=True)
                            output_path.write_text(json.dumps(payload), encoding="utf-8")
                            print(json.dumps({"type": "thread.started", "thread_id": "fake-thread"}))
                            print(json.dumps({"type": "item.completed", "item": {"id": "item-1", "type": "agent_message", "text": json.dumps(payload)}}))
                            """
                        ).strip()
                        + "\n",
                        encoding="utf-8",
                    )
                    codex_script.chmod(0o755)

                    env = os.environ.copy()
                    env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})
                    env["FAKE_LOOP_STATE"] = loop_state

                    completed = subprocess.run(
                        qa_loop_command(
                            artifacts_root=artifacts_root,
                            fake_docker=fake_docker,
                            container_name=container_name,
                            claude_command=f"{sys.executable} {codex_script}",
                            target_command=[sys.executable, str(concierge_script)],
                        ),
                        cwd=str(ROOT),
                        env=env,
                        text=True,
                        capture_output=True,
                        check=False,
                    )
                    self.assertEqual(
                        completed.returncode,
                        expected_returncode,
                        msg=f"stdout:\n{completed.stdout}\n\nstderr:\n{completed.stderr}",
                    )

                    run_dir = next((artifacts_root / "runs").iterdir())
                    summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
                    self.assertEqual(summary["loop_state"], loop_state)
                    self.assertEqual(summary["stop_reason"], expected_reason)
                    self.assertTrue(summary["terminal_stopped_by_supervisor"])

                    report_path = artifacts_root / "reports" / f"{run_dir.name}.md"
                    self.assertTrue(report_path.is_file())
                    report_body = report_path.read_text(encoding="utf-8")
                    self.assertIn(f"Loop state: `{loop_state}`", report_body)
                    self.assertIn(f"Stop reason: `{expected_reason}`", report_body)

    def test_supervisor_loop_runs_manual_command_and_restarts_concierge(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()
            fake_docker = write_fake_docker(tmp)
            container_name = "fixture"

            concierge_script = tmp / "fake_concierge_restart.py"
            concierge_script.write_text(
                textwrap.dedent(
                    """
                    from pathlib import Path

                    marker = Path("prepared.txt")
                    if not marker.exists():
                        print("Manual step required: run touch prepared.txt", flush=True)
                        raise SystemExit(0)

                    print("Type YES to continue:", flush=True)
                    answer = input()
                    print(f"Input received: {answer}", flush=True)
                    print("Integration complete", flush=True)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            codex_script = tmp / "fake_codex_restart.py"
            codex_script.write_text(
                textwrap.dedent(
                    """
                    import json
                    import os
                    import sys
                    from pathlib import Path

                    args = sys.argv[1:]
                    output_path = None
                    for index, value in enumerate(args):
                        if value == "-o":
                            output_path = Path(args[index + 1])
                            break
                    if output_path is None:
                        raise SystemExit("missing -o")

                    prompt = sys.stdin.read()
                    state_path = Path(os.environ["FAKE_CODEX_STATE"])
                    if state_path.exists():
                        state = json.loads(state_path.read_text(encoding="utf-8"))
                    else:
                        state = {"turn": 0}

                    if "final qualitative QA report" in prompt:
                        payload = {
                            "title": "Manual handoff recovery report",
                            "overall_outcome": "The supervisor ran the manual command, restarted Concierge, and completed the session.",
                            "loop_state": "STOP_REPORT",
                            "integration_progress": "The manual prerequisite was handled outside Concierge and the relaunched session reached completion.",
                            "ux_clarity": [],
                            "product_issues": [],
                            "agent_interaction_issues": [],
                            "suggestions": [],
                            "notable_moments": ["The supervisor restarted Concierge after the external command succeeded."]
                        }
                    else:
                        state["turn"] += 1
                        if state["turn"] == 1:
                            payload = {
                                "action": "RUN_COMMAND",
                                "input_text": "touch prepared.txt",
                                "loop_state": "CONTINUE",
                                "summary": "Handle the manual prerequisite outside Concierge.",
                                "issues": [],
                                "next_focus": "Relaunch Concierge and continue the flow."
                            }
                        elif state["turn"] == 2:
                            payload = {
                                "action": "SEND_INPUT",
                                "input_text": "YES",
                                "loop_state": "CONTINUE",
                                "summary": "Continue the restarted session.",
                                "issues": [],
                                "next_focus": "Wait for the completion message."
                            }
                        else:
                            payload = {
                                "action": "WAIT",
                                "input_text": "",
                                "loop_state": "STOP_REPORT",
                                "summary": "The restarted session reached a clean stopping point.",
                                "issues": [],
                                "next_focus": "Write the final report."
                            }
                        state_path.write_text(json.dumps(state), encoding="utf-8")

                    output_path.parent.mkdir(parents=True, exist_ok=True)
                    output_path.write_text(json.dumps(payload), encoding="utf-8")
                    print(json.dumps({"type": "thread.started", "thread_id": "fake-thread"}))
                    print(json.dumps({"type": "item.completed", "item": {"id": "item-1", "type": "agent_message", "text": json.dumps(payload)}}))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            codex_script.chmod(0o755)

            env = os.environ.copy()
            env["FAKE_CODEX_STATE"] = str(tmp / "fake_codex_restart_state.json")
            env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})

            completed = subprocess.run(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    claude_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                ),
                cwd=str(ROOT),
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout:\n{completed.stdout}\n\nstderr:\n{completed.stderr}",
            )

            run_dir = next((artifacts_root / "runs").iterdir())
            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["loop_state"], "STOP_REPORT")
            self.assertEqual(summary["terminal_exit_code"], 0)
            self.assertFalse(summary["terminal_stopped_by_supervisor"])
            self.assertEqual(summary["report_status"], "ready")

            interaction_log = artifacts_root / "transcripts" / f"{run_dir.name}.interaction.jsonl"
            interaction_body = interaction_log.read_text(encoding="utf-8")
            self.assertIn('"kind": "command"', interaction_body)
            self.assertIn('"kind": "process_restart"', interaction_body)

            transcript_path = artifacts_root / "transcripts" / f"{run_dir.name}.terminal.txt"
            transcript = transcript_path.read_text(encoding="utf-8")
            self.assertIn("[qa-loop] external command", transcript)
            self.assertIn("Type YES to continue:", transcript)
            self.assertIn("Integration complete", transcript)

    def test_supervisor_loop_restarts_target_for_interactive_rerun_command(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()
            fake_docker = write_fake_docker(tmp)
            container_name = "fixture"

            concierge_script = tmp / "fake_concierge_rerun.py"
            concierge_script.write_text(
                textwrap.dedent(
                    """
                    from pathlib import Path

                    marker = Path("rerun-ready.txt")
                    if not marker.exists():
                        marker.write_text("ready\\n", encoding="utf-8")
                        print("Changes are in your working tree for local review. After reviewing or committing them, rerun `concierge run`.", flush=True)
                        raise SystemExit(0)

                    print("Continue to the next step? [y/N]: ", end="", flush=True)
                    answer = input()
                    print(f"Input received: {answer}", flush=True)
                    print("Integration complete", flush=True)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            codex_script = tmp / "fake_codex_rerun.py"
            codex_script.write_text(
                textwrap.dedent(
                    """
                    import json
                    import os
                    import sys
                    from pathlib import Path

                    args = sys.argv[1:]
                    output_path = None
                    for index, value in enumerate(args):
                        if value == "-o":
                            output_path = Path(args[index + 1])
                            break
                    if output_path is None:
                        raise SystemExit("missing -o")

                    prompt = sys.stdin.read()
                    state_path = Path(os.environ["FAKE_CODEX_STATE"])
                    if state_path.exists():
                        state = json.loads(state_path.read_text(encoding="utf-8"))
                    else:
                        state = {"turn": 0}

                    if "final qualitative QA report" in prompt:
                        payload = {
                            "title": "Interactive rerun report",
                            "overall_outcome": "The supervisor restarted the target command as a PTY-backed interactive session.",
                            "loop_state": "STOP_REPORT",
                            "integration_progress": "The rerun prompt was followed by an interactive second session that reached completion.",
                            "ux_clarity": [],
                            "product_issues": [],
                            "agent_interaction_issues": [],
                            "suggestions": [],
                            "notable_moments": ["The rerun command became the new interactive target instead of blocking as a one-off subprocess."]
                        }
                    else:
                        state["turn"] += 1
                        if state["turn"] == 1:
                            payload = {
                                "action": "RUN_COMMAND",
                                "input_text": os.environ["FAKE_TARGET_COMMAND"],
                                "loop_state": "CONTINUE",
                                "summary": "Rerun Concierge interactively after the review-only stop.",
                                "issues": [],
                                "next_focus": "Wait for the next prompt in the restarted session."
                            }
                        elif state["turn"] == 2:
                            if "Continue to the next step? [y/N]:" not in prompt:
                                payload = {
                                    "action": "WAIT",
                                    "input_text": "",
                                    "loop_state": "STOP_FIX",
                                    "summary": "The restarted target prompt was not visible to the supervisor.",
                                    "issues": ["Interactive rerun did not surface the next prompt."],
                                    "next_focus": "Restart the target command in PTY mode so prompts remain interactive."
                                }
                            else:
                                payload = {
                                    "action": "SEND_INPUT",
                                    "input_text": "y",
                                    "loop_state": "CONTINUE",
                                    "summary": "Approve the restarted interactive prompt.",
                                    "issues": [],
                                    "next_focus": "Wait for completion."
                                }
                        else:
                            payload = {
                                "action": "WAIT",
                                "input_text": "",
                                "loop_state": "STOP_REPORT",
                                "summary": "The restarted session completed cleanly.",
                                "issues": [],
                                "next_focus": "Write the final report."
                            }
                        state_path.write_text(json.dumps(state), encoding="utf-8")

                    output_path.parent.mkdir(parents=True, exist_ok=True)
                    output_path.write_text(json.dumps(payload), encoding="utf-8")
                    print(json.dumps({"type": "thread.started", "thread_id": "fake-thread"}))
                    print(json.dumps({"type": "item.completed", "item": {"id": "item-1", "type": "agent_message", "text": json.dumps(payload)}}))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            codex_script.chmod(0o755)

            target_command = [sys.executable, str(concierge_script)]

            env = os.environ.copy()
            env["FAKE_CODEX_STATE"] = str(tmp / "fake_codex_rerun_state.json")
            env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})
            env["FAKE_TARGET_COMMAND"] = shlex.join(target_command)

            completed = subprocess.run(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    claude_command=f"{sys.executable} {codex_script}",
                    target_command=target_command,
                    extra_args=["--max-runtime-seconds", "10"],
                ),
                cwd=str(ROOT),
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout:\n{completed.stdout}\n\nstderr:\n{completed.stderr}",
            )

            run_dir = next((artifacts_root / "runs").iterdir())
            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["loop_state"], "STOP_REPORT")
            self.assertEqual(summary["report_status"], "ready")

            interaction_log = artifacts_root / "transcripts" / f"{run_dir.name}.interaction.jsonl"
            interaction_body = interaction_log.read_text(encoding="utf-8")
            self.assertIn('"kind": "process_restart"', interaction_body)

            transcript_path = artifacts_root / "transcripts" / f"{run_dir.name}.terminal.txt"
            transcript = transcript_path.read_text(encoding="utf-8")
            self.assertIn("rerun `concierge run`", transcript)
            self.assertIn("Continue to the next step? [y/N]:", transcript)
            self.assertIn("Integration complete", transcript)

    def test_supervisor_prompt_requires_rerun_continuation_for_claude(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()
            fake_docker = write_fake_docker(tmp)
            container_name = "fixture"

            concierge_script = tmp / "fake_concierge_rerun.py"
            concierge_script.write_text(
                textwrap.dedent(
                    """
                    from pathlib import Path

                    marker = Path("rerun-ready.txt")
                    if not marker.exists():
                        marker.write_text("ready\\n", encoding="utf-8")
                        print("Changes are in your working tree for local review. After reviewing or committing them, rerun `concierge run`.", flush=True)
                        raise SystemExit(0)

                    print("Continue to the next step? [y/N]: ", end="", flush=True)
                    answer = input()
                    print(f"Input received: {answer}", flush=True)
                    print("Integration complete", flush=True)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            claude_script = tmp / "fake_claude_prompt_sensitive.py"
            claude_script.write_text(
                textwrap.dedent(
                    """
                    #!/usr/bin/env python3
                    import json
                    import os
                    import sys
                    from pathlib import Path

                    prompt = sys.stdin.read()
                    state_path = Path(os.environ["FAKE_CLAUDE_STATE"])
                    if state_path.exists():
                        state = json.loads(state_path.read_text(encoding="utf-8"))
                    else:
                        state = {"turn": 0}

                    rerun_guidance = (
                        "If Concierge exits cleanly and tells the user to rerun `concierge run`, "
                        "do not stop the QA session just because that single run ended."
                    ) in prompt

                    if "final qualitative QA report" in prompt:
                        payload = {
                            "type": "result",
                            "subtype": "success",
                            "structured_output": {
                                "title": "Interactive rerun report",
                                "overall_outcome": "The supervisor restarted the target command as a PTY-backed interactive session.",
                                "loop_state": "STOP_REPORT",
                                "integration_progress": "The rerun prompt was followed by an interactive second session that reached completion.",
                                "ux_clarity": [],
                                "product_issues": [],
                                "agent_interaction_issues": [],
                                "suggestions": [],
                                "notable_moments": [
                                    "The rerun command became the new interactive target instead of stopping the QA session."
                                ],
                            },
                        }
                    else:
                        state["turn"] += 1
                        if state["turn"] == 1:
                            if rerun_guidance:
                                directive = {
                                    "action": "RUN_COMMAND",
                                    "input_text": os.environ["FAKE_TARGET_COMMAND"],
                                    "loop_state": "CONTINUE",
                                    "summary": "Rerun Concierge interactively after the review-only stop.",
                                    "issues": [],
                                    "next_focus": "Wait for the next prompt in the restarted session.",
                                }
                            else:
                                directive = {
                                    "action": "WAIT",
                                    "input_text": "",
                                    "loop_state": "STOP_REPORT",
                                    "summary": "The run reached a useful conclusion after a clean review-only exit.",
                                    "issues": [],
                                    "next_focus": "Write the final report.",
                                }
                        elif state["turn"] == 2:
                            if "Continue to the next step? [y/N]:" not in prompt:
                                directive = {
                                    "action": "WAIT",
                                    "input_text": "",
                                    "loop_state": "STOP_FIX",
                                    "summary": "The restarted target prompt was not visible to the supervisor.",
                                    "issues": ["Interactive rerun did not surface the next prompt."],
                                    "next_focus": "Restart the target command in PTY mode so prompts remain interactive.",
                                }
                            else:
                                directive = {
                                    "action": "SEND_INPUT",
                                    "input_text": "y",
                                    "loop_state": "CONTINUE",
                                    "summary": "Approve the restarted interactive prompt.",
                                    "issues": [],
                                    "next_focus": "Wait for completion.",
                                }
                        else:
                            directive = {
                                "action": "WAIT",
                                "input_text": "",
                                "loop_state": "STOP_REPORT",
                                "summary": "The restarted session completed cleanly.",
                                "issues": [],
                                "next_focus": "Write the final report.",
                            }
                        state_path.write_text(json.dumps(state), encoding="utf-8")
                        payload = {"type": "result", "subtype": "success", "structured_output": directive}

                    print(json.dumps(payload))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            claude_script.chmod(0o755)

            target_command = [sys.executable, str(concierge_script)]

            env = os.environ.copy()
            env["FAKE_CLAUDE_STATE"] = str(tmp / "fake_claude_rerun_state.json")
            env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})
            env["FAKE_TARGET_COMMAND"] = shlex.join(target_command)

            completed = subprocess.run(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    claude_command=str(claude_script),
                    target_command=target_command,
                    extra_args=["--max-runtime-seconds", "10"],
                ),
                cwd=str(ROOT),
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout:\n{completed.stdout}\n\nstderr:\n{completed.stderr}",
            )

            run_dir = next((artifacts_root / "runs").iterdir())
            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["loop_state"], "STOP_REPORT")
            self.assertEqual(summary["report_status"], "ready")

            interaction_log = artifacts_root / "transcripts" / f"{run_dir.name}.interaction.jsonl"
            interaction_body = interaction_log.read_text(encoding="utf-8")
            self.assertIn('"kind": "process_restart"', interaction_body)

            transcript_path = artifacts_root / "transcripts" / f"{run_dir.name}.terminal.txt"
            transcript = transcript_path.read_text(encoding="utf-8")
            self.assertIn("rerun `concierge run`", transcript)
            self.assertIn("Continue to the next step? [y/N]:", transcript)
            self.assertIn("Integration complete", transcript)

    def test_supervisor_loop_waits_for_delayed_followup_prompt(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()
            fake_docker = write_fake_docker(tmp)
            container_name = "fixture"

            concierge_script = tmp / "fake_concierge_delayed.py"
            concierge_script.write_text(
                textwrap.dedent(
                    """
                    import time

                    print("Continue now? [y/N]: ", end="", flush=True)
                    first = input()
                    print("Validating runtime behavior", flush=True)
                    time.sleep(0.6)
                    print("Validation finished", flush=True)
                    print("Continue to the next step? [y/N]: ", end="", flush=True)
                    second = input()
                    print(f"Second input: {second}", flush=True)
                    print("Integration complete", flush=True)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            codex_script = tmp / "fake_codex_delayed.py"
            codex_script.write_text(
                textwrap.dedent(
                    """
                    import json
                    import os
                    import sys
                    from pathlib import Path

                    args = sys.argv[1:]
                    output_path = None
                    for index, value in enumerate(args):
                        if value == "-o":
                            output_path = Path(args[index + 1])
                            break
                    if output_path is None:
                        raise SystemExit("missing -o")

                    prompt = sys.stdin.read()
                    state_path = Path(os.environ["FAKE_CODEX_STATE"])
                    if state_path.exists():
                        state = json.loads(state_path.read_text(encoding="utf-8"))
                    else:
                        state = {"turn": 0}

                    if "final qualitative QA report" in prompt:
                        payload = {
                            "title": "Delayed prompt QA report",
                            "overall_outcome": "The supervisor waited for the real follow-up prompt.",
                            "loop_state": "STOP_REPORT",
                            "integration_progress": "The session survived a long validation pause and still reached completion.",
                            "ux_clarity": [],
                            "product_issues": [],
                            "agent_interaction_issues": [],
                            "suggestions": [],
                            "notable_moments": ["The second approval prompt was visible before Codex made the next decision."]
                        }
                    else:
                        state["turn"] += 1
                        if state["turn"] == 1:
                            payload = {
                                "action": "SEND_INPUT",
                                "input_text": "y",
                                "loop_state": "CONTINUE",
                                "summary": "Approve the first prompt.",
                                "issues": [],
                                "next_focus": "Wait for the delayed follow-up prompt."
                            }
                        elif state["turn"] == 2:
                            if "Continue to the next step? [y/N]:" not in prompt:
                                payload = {
                                    "action": "WAIT",
                                    "input_text": "",
                                    "loop_state": "STOP_FIX",
                                    "summary": "The supervisor asked too early, before the second prompt appeared.",
                                    "issues": ["The delayed prompt was not visible yet."],
                                    "next_focus": "Wait for the terminal to surface the next prompt before asking Codex again."
                                }
                            else:
                                payload = {
                                    "action": "SEND_INPUT",
                                    "input_text": "y",
                                    "loop_state": "CONTINUE",
                                    "summary": "Approve the delayed follow-up prompt.",
                                    "issues": [],
                                    "next_focus": "Wait for the completion message."
                                }
                        else:
                            payload = {
                                "action": "WAIT",
                                "input_text": "",
                                "loop_state": "STOP_REPORT",
                                "summary": "The flow reached completion after the delayed prompt.",
                                "issues": [],
                                "next_focus": "Write the final report."
                            }
                        state_path.write_text(json.dumps(state), encoding="utf-8")

                    output_path.parent.mkdir(parents=True, exist_ok=True)
                    output_path.write_text(json.dumps(payload), encoding="utf-8")
                    print(json.dumps({"type": "thread.started", "thread_id": "fake-thread"}))
                    print(json.dumps({"type": "item.completed", "item": {"id": "item-1", "type": "agent_message", "text": json.dumps(payload)}}))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            codex_script.chmod(0o755)

            env = os.environ.copy()
            env["FAKE_CODEX_STATE"] = str(tmp / "fake_codex_delayed_state.json")
            env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})

            completed = subprocess.run(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    claude_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                    extra_args=[
                        "--read-timeout-seconds",
                        "0.2",
                        "--settle-timeout-seconds",
                        "1.2",
                    ],
                ),
                cwd=str(ROOT),
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout:\n{completed.stdout}\n\nstderr:\n{completed.stderr}",
            )

            run_dir = next((artifacts_root / "runs").iterdir())
            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["loop_state"], "STOP_REPORT")
            self.assertFalse(summary["blind_first_released"])

            turns_path = run_dir / "turns.jsonl"
            turns_body = turns_path.read_text(encoding="utf-8")
            self.assertNotIn("STOP_FIX", turns_body)

            transcript_path = artifacts_root / "transcripts" / f"{run_dir.name}.terminal.txt"
            transcript = transcript_path.read_text(encoding="utf-8")
            self.assertIn("Validation finished", transcript)
            self.assertIn("Continue to the next step? [y/N]:", transcript)
            self.assertIn("Integration complete", transcript)

    def test_supervisor_loop_keeps_blind_first_locked_during_long_active_run(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            fixture_post = tmp / "post"
            artifacts_root.mkdir()
            command_cwd.mkdir()
            fixture_post.mkdir()
            fake_docker = write_fake_docker(tmp)
            container_name = "fixture"
            (fixture_post / "expected.txt").write_text("post fixture sentinel\n", encoding="utf-8")

            concierge_script = tmp / "fake_concierge_blind_first.py"
            concierge_script.write_text(
                textwrap.dedent(
                    """
                    import time

                    print("Continue now? [y/N]: ", end="", flush=True)
                    input()
                    print("Working...", flush=True)
                    time.sleep(1.0)
                    print("Continue to the next step? [y/N]: ", end="", flush=True)
                    input()
                    print("Integration complete", flush=True)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            codex_script = tmp / "fake_codex_blind_first.py"
            codex_script.write_text(
                textwrap.dedent(
                    """
                    import json
                    import os
                    import sys
                    from pathlib import Path

                    args = sys.argv[1:]
                    output_path = None
                    for index, value in enumerate(args):
                        if value == "-o":
                            output_path = Path(args[index + 1])
                            break
                    if output_path is None:
                        raise SystemExit("missing -o")

                    prompt = sys.stdin.read()
                    state_path = Path(os.environ["FAKE_CODEX_STATE"])
                    if state_path.exists():
                        state = json.loads(state_path.read_text(encoding="utf-8"))
                    else:
                        state = {"first_sent": False, "second_sent": False}

                    if "final qualitative QA report" in prompt:
                        payload = {
                            "title": "Blind-first guard report",
                            "overall_outcome": "Blind-first stayed locked until the active session progressed to the next prompt.",
                            "loop_state": "STOP_REPORT",
                            "integration_progress": "The run reached the second prompt without exposing the post fixture.",
                            "ux_clarity": [],
                            "product_issues": [],
                            "agent_interaction_issues": [],
                            "suggestions": [],
                            "notable_moments": ["The post fixture was not exposed during the long active wait."]
                        }
                    elif '"post_fixture_path_available": true' in prompt:
                        payload = {
                            "action": "WAIT",
                            "input_text": "",
                            "loop_state": "STOP_FIX",
                            "summary": "Blind-first released too early while the session was still active.",
                            "issues": ["The post fixture became visible before the next prompt appeared."],
                            "next_focus": "Keep blind-first locked until the run genuinely stalls."
                        }
                    elif "Continue now? [y/N]:" in prompt and not state["first_sent"]:
                        state["first_sent"] = True
                        payload = {
                            "action": "SEND_INPUT",
                            "input_text": "y",
                            "loop_state": "CONTINUE",
                            "summary": "Approve the first prompt.",
                            "issues": [],
                            "next_focus": "Wait through the active processing window."
                        }
                    elif "Continue to the next step? [y/N]:" in prompt and not state["second_sent"]:
                        state["second_sent"] = True
                        payload = {
                            "action": "SEND_INPUT",
                            "input_text": "y",
                            "loop_state": "CONTINUE",
                            "summary": "Approve the second prompt after the active wait.",
                            "issues": [],
                            "next_focus": "Wait for completion."
                        }
                    elif "Integration complete" in prompt:
                        payload = {
                            "action": "WAIT",
                            "input_text": "",
                            "loop_state": "STOP_REPORT",
                            "summary": "The run completed without needing the post fixture.",
                            "issues": [],
                            "next_focus": "Write the final report."
                        }
                    else:
                        payload = {
                            "action": "WAIT",
                            "input_text": "",
                            "loop_state": "CONTINUE",
                            "summary": "The session is still actively working; keep waiting.",
                            "issues": [],
                            "next_focus": "Wait for the next real prompt."
                        }

                    state_path.write_text(json.dumps(state), encoding="utf-8")
                    output_path.parent.mkdir(parents=True, exist_ok=True)
                    output_path.write_text(json.dumps(payload), encoding="utf-8")
                    print(json.dumps({"type": "thread.started", "thread_id": "fake-thread"}))
                    print(json.dumps({"type": "item.completed", "item": {"id": "item-1", "type": "agent_message", "text": json.dumps(payload)}}))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            codex_script.chmod(0o755)

            env = os.environ.copy()
            env["FAKE_CODEX_STATE"] = str(tmp / "fake_codex_blind_first_state.json")
            env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})

            completed = subprocess.run(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    claude_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                    extra_args=[
                        "--fixture-post-path",
                        str(fixture_post),
                        "--max-idle-turns",
                        "6",
                        "--read-timeout-seconds",
                        "0.15",
                        "--settle-timeout-seconds",
                        "0.4",
                    ],
                ),
                cwd=str(ROOT),
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout:\n{completed.stdout}\n\nstderr:\n{completed.stderr}",
            )

            run_dir = next((artifacts_root / "runs").iterdir())
            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertFalse(summary["blind_first_released"])

            turns_path = run_dir / "turns.jsonl"
            turns_body = turns_path.read_text(encoding="utf-8")
            self.assertNotIn("Blind-first released too early", turns_body)

    def test_supervisor_loop_writes_provisional_report_before_final_report_finishes(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()
            fake_docker = write_fake_docker(tmp)
            container_name = "fixture"

            concierge_script = tmp / "fake_concierge_provisional.py"
            concierge_script.write_text(
                textwrap.dedent(
                    """
                    print("Type YES to continue:", flush=True)
                    input()
                    print("Integration complete", flush=True)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            codex_script = tmp / "fake_codex_provisional.py"
            codex_script.write_text(
                textwrap.dedent(
                    """
                    import json
                    import os
                    import sys
                    import time
                    from pathlib import Path

                    args = sys.argv[1:]
                    output_path = None
                    for index, value in enumerate(args):
                        if value == "-o":
                            output_path = Path(args[index + 1])
                            break
                    if output_path is None:
                        raise SystemExit("missing -o")

                    prompt = sys.stdin.read()
                    state_path = Path(os.environ["FAKE_CODEX_STATE"])
                    if state_path.exists():
                        state = json.loads(state_path.read_text(encoding="utf-8"))
                    else:
                        state = {"turn": 0}

                    if "final qualitative QA report" in prompt:
                        time.sleep(2.5)
                        raise SystemExit(0)

                    state["turn"] += 1
                    if state["turn"] == 1:
                        payload = {
                            "action": "SEND_INPUT",
                            "input_text": "YES",
                            "loop_state": "CONTINUE",
                            "summary": "Advance through the only prompt.",
                            "issues": [],
                            "next_focus": "Wait for the completion output."
                        }
                    else:
                        payload = {
                            "action": "WAIT",
                            "input_text": "",
                            "loop_state": "STOP_REPORT",
                            "summary": "The flow reached a clean completion point.",
                            "issues": [],
                            "next_focus": "Write the final report."
                        }
                    state_path.write_text(json.dumps(state), encoding="utf-8")

                    output_path.parent.mkdir(parents=True, exist_ok=True)
                    output_path.write_text(json.dumps(payload), encoding="utf-8")
                    print(json.dumps({"type": "thread.started", "thread_id": "fake-thread"}))
                    print(json.dumps({"type": "item.completed", "item": {"id": "item-1", "type": "agent_message", "text": json.dumps(payload)}}))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            codex_script.chmod(0o755)

            env = os.environ.copy()
            env["FAKE_CODEX_STATE"] = str(tmp / "fake_codex_provisional_state.json")
            env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})

            proc = subprocess.Popen(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    claude_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                    extra_args=[
                        "--run-id",
                        "provisional-report",
                        "--claude-timeout-seconds",
                        "1.5",
                    ],
                ),
                cwd=str(ROOT),
                env=env,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            try:
                run_dir = artifacts_root / "runs" / "provisional-report"
                summary_path = run_dir / "summary.json"
                report_path = artifacts_root / "reports" / "provisional-report.md"
                self.assertTrue(wait_for_file(summary_path))
                self.assertTrue(wait_for_file(report_path))
                self.assertIsNone(proc.poll())

                summary_pending = json.loads(summary_path.read_text(encoding="utf-8"))
                self.assertEqual(summary_pending["report_status"], "pending")
                report_body_pending = report_path.read_text(encoding="utf-8")
                self.assertIn("QA Loop Report (provisional)", report_body_pending)

                stdout, stderr = proc.communicate(timeout=5.0)
            finally:
                if proc.poll() is None:
                    proc.terminate()
                    proc.communicate(timeout=5.0)

            self.assertEqual(proc.returncode, 0, msg=f"stdout:\n{stdout}\n\nstderr:\n{stderr}")

            summary_final = json.loads(summary_path.read_text(encoding="utf-8"))
            self.assertEqual(summary_final["report_status"], "fallback")
            report_body_final = report_path.read_text(encoding="utf-8")
            self.assertIn("QA Loop Report (fallback)", report_body_final)

    def test_supervisor_loop_writes_fallback_report_when_report_codex_times_out(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()
            fake_docker = write_fake_docker(tmp)
            container_name = "fixture"

            concierge_script = tmp / "fake_concierge_report_timeout.py"
            concierge_script.write_text(
                textwrap.dedent(
                    """
                    print("Type YES to continue:", flush=True)
                    answer = input()
                    print(f"Input received: {answer}", flush=True)
                    print("Integration complete", flush=True)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            codex_script = tmp / "fake_codex_report_timeout.py"
            codex_script.write_text(
                textwrap.dedent(
                    """
                    import json
                    import os
                    import sys
                    import time
                    from pathlib import Path

                    args = sys.argv[1:]
                    output_path = None
                    for index, value in enumerate(args):
                        if value == "-o":
                            output_path = Path(args[index + 1])
                            break
                    if output_path is None:
                        raise SystemExit("missing -o")

                    prompt = sys.stdin.read()
                    state_path = Path(os.environ["FAKE_CODEX_STATE"])
                    if state_path.exists():
                        state = json.loads(state_path.read_text(encoding="utf-8"))
                    else:
                        state = {"turn": 0}

                    if "final qualitative QA report" in prompt:
                        print(json.dumps({"type": "thread.started", "thread_id": "final-report-thread"}), flush=True)
                        sys.stderr.write("final report synthesis still running\\n")
                        sys.stderr.flush()
                        time.sleep(2.0)
                        raise SystemExit(0)

                    state["turn"] += 1
                    if state["turn"] == 1:
                        payload = {
                            "action": "SEND_INPUT",
                            "input_text": "YES",
                            "loop_state": "CONTINUE",
                            "summary": "Advance through the only prompt.",
                            "issues": [],
                            "next_focus": "Wait for the completion output."
                        }
                    else:
                        payload = {
                            "action": "WAIT",
                            "input_text": "",
                            "loop_state": "STOP_REPORT",
                            "summary": "The flow reached a clean completion point.",
                            "issues": [],
                            "next_focus": "Write the final report."
                        }
                    state_path.write_text(json.dumps(state), encoding="utf-8")

                    output_path.parent.mkdir(parents=True, exist_ok=True)
                    output_path.write_text(json.dumps(payload), encoding="utf-8")
                    print(json.dumps({"type": "thread.started", "thread_id": "fake-thread"}))
                    print(json.dumps({"type": "item.completed", "item": {"id": "item-1", "type": "agent_message", "text": json.dumps(payload)}}))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            codex_script.chmod(0o755)

            env = os.environ.copy()
            env["FAKE_CODEX_STATE"] = str(tmp / "fake_codex_report_timeout_state.json")
            env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})

            completed = subprocess.run(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    claude_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                    extra_args=[
                        "--claude-timeout-seconds",
                        "0.5",
                    ],
                ),
                cwd=str(ROOT),
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout:\n{completed.stdout}\n\nstderr:\n{completed.stderr}",
            )

            run_dir = next((artifacts_root / "runs").iterdir())
            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["loop_state"], "STOP_REPORT")
            self.assertEqual(summary["report_status"], "fallback")

            report_path = artifacts_root / "reports" / f"{run_dir.name}.md"
            self.assertTrue(report_path.is_file())
            report_body = report_path.read_text(encoding="utf-8")
            self.assertIn("QA Loop Report (fallback)", report_body)
            final_report_base = run_dir / "claude" / "final-report"
            final_report_event_log = final_report_base.with_suffix(".jsonl")
            final_report_stderr_log = final_report_base.with_suffix(".stderr.log")
            self.assertTrue(final_report_event_log.is_file())
            self.assertTrue(final_report_stderr_log.is_file())
            self.assertIn("final-report-thread", final_report_event_log.read_text(encoding="utf-8"))
            self.assertIn("final report synthesis still running", final_report_stderr_log.read_text(encoding="utf-8"))
            self.assertIn(str(final_report_stderr_log), report_body)

            interaction_log = artifacts_root / "transcripts" / f"{run_dir.name}.interaction.jsonl"
            interaction_body = interaction_log.read_text(encoding="utf-8")
            self.assertIn("report_fallback", interaction_body)

    def test_supervisor_loop_handles_control_timeout_with_partial_output(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()
            fake_docker = write_fake_docker(tmp)
            container_name = "fixture"

            concierge_script = tmp / "fake_concierge_control_timeout.py"
            concierge_script.write_text(
                textwrap.dedent(
                    """
                    print("Continue now? [y/N]: ", end="", flush=True)
                    input()
                    print("done", flush=True)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            codex_script = tmp / "fake_codex_control_timeout.py"
            codex_script.write_text(
                textwrap.dedent(
                    """
                    import json
                    import sys
                    import time
                    from pathlib import Path

                    args = sys.argv[1:]
                    output_path = None
                    for index, value in enumerate(args):
                        if value == "-o":
                            output_path = Path(args[index + 1])
                            break
                    if output_path is None:
                        raise SystemExit("missing -o")

                    prompt = sys.stdin.read()
                    if "final qualitative QA report" in prompt:
                        payload = {
                            "title": "Control timeout fallback",
                            "overall_outcome": "The loop recovered after a control-time timeout.",
                            "loop_state": "STOP_DEADEND",
                            "integration_progress": "The run stopped before a user input could be sent.",
                            "ux_clarity": [],
                            "product_issues": [],
                            "agent_interaction_issues": [],
                            "suggestions": [],
                            "notable_moments": []
                        }
                        output_path.parent.mkdir(parents=True, exist_ok=True)
                        output_path.write_text(json.dumps(payload), encoding="utf-8")
                        raise SystemExit(0)

                    sys.stdout.write('{"type":"thread.started","thread_id":"fake-thread"}\\n')
                    sys.stdout.flush()
                    time.sleep(2.0)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            codex_script.chmod(0o755)

            completed = subprocess.run(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    claude_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                    extra_args=[
                        "--claude-timeout-seconds",
                        "0.5",
                    ],
                ),
                cwd=str(ROOT),
                env={**os.environ, "FAKE_DOCKER_CONTAINERS": json.dumps({container_name: str(command_cwd)})},
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(
                completed.returncode,
                3,
                msg=f"stdout:\n{completed.stdout}\n\nstderr:\n{completed.stderr}",
            )

            run_dir = next((artifacts_root / "runs").iterdir())
            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["stop_reason"], "claude_control_error")

            report_path = artifacts_root / "reports" / f"{run_dir.name}.md"
            self.assertTrue(report_path.is_file())
            report_body = report_path.read_text(encoding="utf-8")
            self.assertIn("Loop state: `STOP_DEADEND`", report_body)

            interaction_log = artifacts_root / "transcripts" / f"{run_dir.name}.interaction.jsonl"
            interaction_body = interaction_log.read_text(encoding="utf-8")
            self.assertIn("control_error", interaction_body)

    def test_supervisor_loop_auto_waits_after_control_timeout_when_no_prompt_is_visible(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()
            fake_docker = write_fake_docker(tmp)
            container_name = "fixture"

            concierge_script = tmp / "fake_concierge_autowait.py"
            concierge_script.write_text(
                textwrap.dedent(
                    """
                    import time

                    print("Continue now? [y/N]: ", end="", flush=True)
                    input()
                    print("Working...", flush=True)
                    time.sleep(0.8)
                    print("Next prompt [y/N]: ", end="", flush=True)
                    input()
                    print("Integration complete", flush=True)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            codex_script = tmp / "fake_codex_autowait.py"
            codex_script.write_text(
                textwrap.dedent(
                    """
                    import json
                    import os
                    import sys
                    import time
                    from pathlib import Path

                    args = sys.argv[1:]
                    output_path = None
                    for index, value in enumerate(args):
                        if value == "-o":
                            output_path = Path(args[index + 1])
                            break
                    if output_path is None:
                        raise SystemExit("missing -o")

                    prompt = sys.stdin.read()
                    state_path = Path(os.environ["FAKE_CODEX_STATE"])
                    if state_path.exists():
                        state = json.loads(state_path.read_text(encoding="utf-8"))
                    else:
                        state = {"turn": 0}

                    if "final qualitative QA report" in prompt:
                        payload = {
                            "title": "Auto-wait timeout recovery",
                            "overall_outcome": "The supervisor recovered from a no-prompt timeout and still completed the run.",
                            "loop_state": "STOP_REPORT",
                            "integration_progress": "The delayed prompt appeared after an automatic wait.",
                            "ux_clarity": [],
                            "product_issues": [],
                            "agent_interaction_issues": [],
                            "suggestions": [],
                            "notable_moments": []
                        }
                        output_path.parent.mkdir(parents=True, exist_ok=True)
                        output_path.write_text(json.dumps(payload), encoding="utf-8")
                        raise SystemExit(0)

                    state["turn"] += 1
                    state_path.write_text(json.dumps(state), encoding="utf-8")
                    if state["turn"] == 1:
                        payload = {
                            "action": "SEND_INPUT",
                            "input_text": "y",
                            "loop_state": "CONTINUE",
                            "summary": "Approve the first prompt.",
                            "issues": [],
                            "next_focus": "Wait for the next prompt."
                        }
                        output_path.parent.mkdir(parents=True, exist_ok=True)
                        output_path.write_text(json.dumps(payload), encoding="utf-8")
                        raise SystemExit(0)
                    if state["turn"] == 2:
                        sys.stdout.write('{"type":"thread.started","thread_id":"fake-thread"}\\n')
                        sys.stdout.flush()
                        time.sleep(2.0)
                        raise SystemExit(0)
                    if state["turn"] == 3:
                        payload = {
                            "action": "SEND_INPUT",
                            "input_text": "y",
                            "loop_state": "CONTINUE",
                            "summary": "Approve the delayed second prompt.",
                            "issues": [],
                            "next_focus": "Wait for completion."
                        }
                    else:
                        payload = {
                            "action": "WAIT",
                            "input_text": "",
                            "loop_state": "STOP_REPORT",
                            "summary": "The run completed after the auto-wait recovery.",
                            "issues": [],
                            "next_focus": "Write the final report."
                        }
                    output_path.parent.mkdir(parents=True, exist_ok=True)
                    output_path.write_text(json.dumps(payload), encoding="utf-8")
                    raise SystemExit(0)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            codex_script.chmod(0o755)

            env = os.environ.copy()
            env["FAKE_CODEX_STATE"] = str(tmp / "fake_codex_autowait_state.json")
            env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})

            completed = subprocess.run(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    claude_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                    extra_args=[
                        "--claude-timeout-seconds",
                        "0.5",
                        "--read-timeout-seconds",
                        "0.2",
                        "--settle-timeout-seconds",
                        "1.2",
                    ],
                ),
                cwd=str(ROOT),
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout:\n{completed.stdout}\n\nstderr:\n{completed.stderr}",
            )

            run_dir = next((artifacts_root / "runs").iterdir())
            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["loop_state"], "STOP_REPORT")

            interaction_log = artifacts_root / "transcripts" / f"{run_dir.name}.interaction.jsonl"
            interaction_body = interaction_log.read_text(encoding="utf-8")
            self.assertIn("control_timeout_autowait", interaction_body)

            transcript_path = artifacts_root / "transcripts" / f"{run_dir.name}.terminal.txt"
            transcript = transcript_path.read_text(encoding="utf-8")
            self.assertIn("Next prompt [y/N]:", transcript)
            self.assertIn("Integration complete", transcript)

    def test_supervisor_loop_does_not_spend_iterations_on_post_wait_auto_polling(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()
            fake_docker = write_fake_docker(tmp)
            container_name = "fixture"

            concierge_script = tmp / "fake_concierge_post_wait_progress.py"
            concierge_script.write_text(
                textwrap.dedent(
                    """
                    import time

                    print("Continue now? [y/N]: ", end="", flush=True)
                    input()
                    print("Running repo check", flush=True)
                    time.sleep(0.3)
                    print("Editing repository files", flush=True)
                    time.sleep(0.3)
                    print("Integration complete", flush=True)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            codex_script = tmp / "fake_codex_post_wait_progress.py"
            codex_script.write_text(
                textwrap.dedent(
                    """
                    import json
                    import os
                    import sys
                    from pathlib import Path

                    args = sys.argv[1:]
                    output_path = None
                    for index, value in enumerate(args):
                        if value == "-o":
                            output_path = Path(args[index + 1])
                            break
                    if output_path is None:
                        raise SystemExit("missing -o")

                    prompt = sys.stdin.read()
                    state_path = Path(os.environ["FAKE_CODEX_STATE"])
                    if state_path.exists():
                        state = json.loads(state_path.read_text(encoding="utf-8"))
                    else:
                        state = {"turn": 0}

                    if "final qualitative QA report" in prompt:
                        payload = {
                            "title": "Post-WAIT progress report",
                            "overall_outcome": "The supervisor let the active session finish after a WAIT decision.",
                            "loop_state": "STOP_REPORT",
                            "integration_progress": "The run kept surfacing progress after WAIT and reached completion without burning the control-turn budget.",
                            "ux_clarity": [],
                            "product_issues": [],
                            "agent_interaction_issues": [],
                            "suggestions": [],
                            "notable_moments": ["Visible post-WAIT progress did not count as extra control turns."]
                        }
                    else:
                        state["turn"] += 1
                        if state["turn"] == 1:
                            payload = {
                                "action": "SEND_INPUT",
                                "input_text": "y",
                                "loop_state": "CONTINUE",
                                "summary": "Approve the first prompt.",
                                "issues": [],
                                "next_focus": "Wait for the repo work to continue."
                            }
                        elif "Integration complete" in prompt:
                            payload = {
                                "action": "WAIT",
                                "input_text": "",
                                "loop_state": "STOP_REPORT",
                                "summary": "The run completed after the active wait window.",
                                "issues": [],
                                "next_focus": "Write the final report."
                            }
                        else:
                            payload = {
                                "action": "WAIT",
                                "input_text": "",
                                "loop_state": "CONTINUE",
                                "summary": "The session is still making visible progress; keep waiting.",
                                "issues": [],
                                "next_focus": "Let the active step finish."
                            }
                        state_path.write_text(json.dumps(state), encoding="utf-8")

                    output_path.parent.mkdir(parents=True, exist_ok=True)
                    output_path.write_text(json.dumps(payload), encoding="utf-8")
                    print(json.dumps({"type": "thread.started", "thread_id": "fake-thread"}))
                    print(json.dumps({"type": "item.completed", "item": {"id": "item-1", "type": "agent_message", "text": json.dumps(payload)}}))
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )
            codex_script.chmod(0o755)

            env = os.environ.copy()
            env["FAKE_CODEX_STATE"] = str(tmp / "fake_codex_post_wait_progress_state.json")
            env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})

            completed = subprocess.run(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    claude_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                    extra_args=[
                        "--max-iterations",
                        "4",
                        "--read-timeout-seconds",
                        "0.15",
                        "--settle-timeout-seconds",
                        "0.2",
                    ],
                ),
                cwd=str(ROOT),
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout:\n{completed.stdout}\n\nstderr:\n{completed.stderr}",
            )

            run_dir = next((artifacts_root / "runs").iterdir())
            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["loop_state"], "STOP_REPORT")
            self.assertEqual(summary["stop_reason"], "supervisor_stop_report")
            self.assertEqual(summary["iterations_completed"], 3)
            self.assertFalse(summary["terminal_stopped_by_supervisor"])

            transcript_path = artifacts_root / "transcripts" / f"{run_dir.name}.terminal.txt"
            transcript = transcript_path.read_text(encoding="utf-8")
            self.assertIn("Running repo check", transcript)
            self.assertIn("Editing repository files", transcript)
            self.assertIn("Integration complete", transcript)


if __name__ == "__main__":
    unittest.main()
