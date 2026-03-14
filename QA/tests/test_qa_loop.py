from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
QA_LOOP = ROOT / "qa_loop.py"


class QALoopTest(unittest.TestCase):
    def test_supervisor_loop_writes_transcript_and_report(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()

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

            completed = subprocess.run(
                [
                    sys.executable,
                    str(QA_LOOP),
                    "--artifacts-root",
                    str(artifacts_root),
                    "--command-cwd",
                    str(command_cwd),
                    "--codex-command",
                    f"{sys.executable} {codex_script}",
                    "--",
                    sys.executable,
                    str(concierge_script),
                ],
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

            run_dirs = sorted((artifacts_root / "runs").iterdir())
            self.assertEqual(len(run_dirs), 1)
            run_dir = run_dirs[0]

            summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["loop_state"], "STOP_REPORT")

            report_path = artifacts_root / "reports" / f"{run_dir.name}.md"
            self.assertTrue(report_path.is_file())
            report_body = report_path.read_text(encoding="utf-8")
            self.assertIn("Synthetic QA Report", report_body)
            self.assertIn("Reached the completion path.", report_body)

            transcript_path = artifacts_root / "transcripts" / f"{run_dir.name}.terminal.txt"
            transcript = transcript_path.read_text(encoding="utf-8")
            self.assertIn("Welcome to Concierge", transcript)
            self.assertIn("Integration complete", transcript)

    def test_supervisor_loop_stops_cleanly_when_target_exits_before_followup_input(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()

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

            completed = subprocess.run(
                [
                    sys.executable,
                    str(QA_LOOP),
                    "--artifacts-root",
                    str(artifacts_root),
                    "--command-cwd",
                    str(command_cwd),
                    "--codex-command",
                    f"{sys.executable} {codex_script}",
                    "--",
                    sys.executable,
                    str(concierge_script),
                ],
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

    def test_supervisor_loop_waits_for_delayed_followup_prompt(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()

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

            completed = subprocess.run(
                [
                    sys.executable,
                    str(QA_LOOP),
                    "--artifacts-root",
                    str(artifacts_root),
                    "--command-cwd",
                    str(command_cwd),
                    "--codex-command",
                    f"{sys.executable} {codex_script}",
                    "--read-timeout-seconds",
                    "0.2",
                    "--settle-timeout-seconds",
                    "1.2",
                    "--",
                    sys.executable,
                    str(concierge_script),
                ],
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

            turns_path = run_dir / "turns.jsonl"
            turns_body = turns_path.read_text(encoding="utf-8")
            self.assertNotIn("STOP_FIX", turns_body)

            transcript_path = artifacts_root / "transcripts" / f"{run_dir.name}.terminal.txt"
            transcript = transcript_path.read_text(encoding="utf-8")
            self.assertIn("Validation finished", transcript)
            self.assertIn("Continue to the next step? [y/N]:", transcript)
            self.assertIn("Integration complete", transcript)

    def test_supervisor_loop_writes_fallback_report_when_report_codex_times_out(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            artifacts_root = tmp / "artifacts"
            command_cwd = tmp / "fixture"
            artifacts_root.mkdir()
            command_cwd.mkdir()

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

            completed = subprocess.run(
                [
                    sys.executable,
                    str(QA_LOOP),
                    "--artifacts-root",
                    str(artifacts_root),
                    "--command-cwd",
                    str(command_cwd),
                    "--codex-command",
                    f"{sys.executable} {codex_script}",
                    "--codex-timeout-seconds",
                    "0.5",
                    "--",
                    sys.executable,
                    str(concierge_script),
                ],
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
                [
                    sys.executable,
                    str(QA_LOOP),
                    "--artifacts-root",
                    str(artifacts_root),
                    "--command-cwd",
                    str(command_cwd),
                    "--codex-command",
                    f"{sys.executable} {codex_script}",
                    "--codex-timeout-seconds",
                    "0.5",
                    "--",
                    sys.executable,
                    str(concierge_script),
                ],
                cwd=str(ROOT),
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

            completed = subprocess.run(
                [
                    sys.executable,
                    str(QA_LOOP),
                    "--artifacts-root",
                    str(artifacts_root),
                    "--command-cwd",
                    str(command_cwd),
                    "--codex-command",
                    f"{sys.executable} {codex_script}",
                    "--codex-timeout-seconds",
                    "0.5",
                    "--read-timeout-seconds",
                    "0.2",
                    "--settle-timeout-seconds",
                    "1.2",
                    "--",
                    sys.executable,
                    str(concierge_script),
                ],
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
