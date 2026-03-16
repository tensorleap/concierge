from __future__ import annotations

import json
import os
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
    codex_command: str,
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
        "--codex-command",
        codex_command,
    ]
    if extra_args:
        command.extend(extra_args)
    command.append("--")
    command.extend(target_command)
    return command


class QALoopTest(unittest.TestCase):
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

            codex_script = tmp / "fake_codex.py"
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
                            "title": "Synthetic QA Report",
                            "overall_outcome": "Reached the completion path.",
                            "loop_state": "STOP_REPORT",
                            "integration_progress": "The flow reached the final completion message after one affirmative input.",
                            "ux_clarity": ["The prompt for YES was easy to follow."],
                            "product_issues": [],
                            "agent_interaction_issues": [],
                            "suggestions": ["Keep the completion message concise."],
                            "notable_moments": ["Codex answered YES and Concierge completed immediately."]
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
            env["FAKE_CODEX_STATE"] = str(tmp / "fake_codex_state.json")
            env["FAKE_DOCKER_CONTAINERS"] = json.dumps({container_name: str(command_cwd)})

            completed = subprocess.run(
                qa_loop_command(
                    artifacts_root=artifacts_root,
                    fake_docker=fake_docker,
                    container_name=container_name,
                    codex_command=f"{sys.executable} {codex_script}",
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
            self.assertIn("Welcome to Concierge", completed.stdout)
            self.assertIn("[qa-loop][codex-control-001] thread started: fake-thread", completed.stdout)
            self.assertIn("[qa-loop][codex-control-001] action: SEND_INPUT -> YES (CONTINUE)", completed.stdout)

            run_dirs = sorted((artifacts_root / "runs").iterdir())
            self.assertEqual(len(run_dirs), 1)
            run_dir = run_dirs[0]

            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["loop_state"], "STOP_REPORT")
            self.assertEqual(summary["report_status"], "ready")
            self.assertEqual(summary["docker"]["container_name"], container_name)
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
            self.assertIn("[qa-loop] --- codex-control-001 stdin begin ---", full_transcript)
            self.assertIn("[qa-loop][codex-control-001] action: SEND_INPUT -> YES (CONTINUE)", full_transcript)
            self.assertIn(f"[qa-loop] transcript: {full_transcript_path.resolve()}", completed.stdout)

            event_log_path = run_dir / "codex" / "turn-001.jsonl"
            self.assertTrue(event_log_path.is_file())
            event_log = event_log_path.read_text(encoding="utf-8")
            self.assertIn('"type": "thread.started"', event_log)
            self.assertIn('"type": "item.completed"', event_log)

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
                    codex_command=f"{sys.executable} {codex_script}",
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
                    codex_command=f"{sys.executable} {codex_script}",
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
                    codex_command=f"{sys.executable} {codex_script}",
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
                    codex_command=f"{sys.executable} {codex_script}",
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
                    codex_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                    extra_args=[
                        "--run-id",
                        "provisional-report",
                        "--codex-timeout-seconds",
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
                    codex_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                    extra_args=[
                        "--codex-timeout-seconds",
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
                    codex_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                    extra_args=[
                        "--codex-timeout-seconds",
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
            self.assertEqual(summary["stop_reason"], "codex_control_error")

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
                    codex_command=f"{sys.executable} {codex_script}",
                    target_command=[sys.executable, str(concierge_script)],
                    extra_args=[
                        "--codex-timeout-seconds",
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


if __name__ == "__main__":
    unittest.main()
