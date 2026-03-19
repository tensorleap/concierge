#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path
from typing import Any, TextIO

DEFAULT_BUILD_MODE = "cold"
VALID_BUILD_MODES = {"cold", "prewarmed"}
VALID_SOURCE_KINDS = {"variant", "case"}
GUIDE_STEP_ORDER = (
    "integration_script",
    "preprocess",
    "input_encoders",
    "model_acquisition",
    "integration_test",
    "ground_truth_encoders",
)


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def normalize_build_mode(value: str | None) -> str:
    candidate = (value or DEFAULT_BUILD_MODE).strip()
    if candidate not in VALID_BUILD_MODES:
        raise ValueError(f"unsupported build mode {candidate!r}")
    return candidate


def fixture_ids(repo_root: Path) -> list[str]:
    repo_root = Path(repo_root).resolve()
    fixtures_manifest = load_json(repo_root / "fixtures" / "manifest.json")
    fixture_ids = [str(entry.get("id", "")).strip() for entry in fixtures_manifest.get("fixtures", [])]
    return [fixture_id for fixture_id in fixture_ids if fixture_id]


def guide_steps_for_fixture(repo_root: Path, *, fixture_id: str) -> list[str]:
    if fixture_id not in fixture_ids(repo_root):
        raise ValueError(f"unknown fixture id {fixture_id!r}")
    return list(GUIDE_STEP_ORDER)


def format_choices(values: list[str]) -> str:
    return ", ".join(values)


def prompt_for_choice(
    options: list[str],
    *,
    heading: str,
    prompt: str,
    input_stream: TextIO,
    output_stream: TextIO,
) -> str:
    output_stream.write(f"{heading}\n")
    for index, option in enumerate(options, start=1):
        output_stream.write(f"{index}. {option}\n")
    while True:
        output_stream.write(f"{prompt} [1-{len(options)}]: ")
        output_stream.flush()
        raw_value = input_stream.readline()
        if raw_value == "":
            raise ValueError("interactive selection ended before a valid choice was entered")
        candidate = raw_value.strip()
        if candidate.isdigit():
            selected_index = int(candidate)
            if 1 <= selected_index <= len(options):
                return options[selected_index - 1]
        output_stream.write(f"Invalid selection. Choose a number from 1 to {len(options)}.\n")


def select_runner_target(
    repo_root: Path,
    *,
    fixture_id: str | None = None,
    step: str | None = None,
    interactive: bool = False,
    input_stream: TextIO | None = None,
    output_stream: TextIO | None = None,
) -> dict[str, str]:
    input_stream = input_stream or sys.stdin
    output_stream = output_stream or sys.stderr

    available_fixtures = fixture_ids(repo_root)
    available_steps = list(GUIDE_STEP_ORDER)
    missing_selector_message = (
        "missing required QA selectors for non-interactive run. "
        f"Valid fixtures: {format_choices(available_fixtures)}. "
        f"Valid steps: {format_choices(available_steps)}"
    )

    selected_fixture = (fixture_id or "").strip()
    if not selected_fixture:
        if not interactive:
            raise ValueError(missing_selector_message)
        selected_fixture = prompt_for_choice(
            available_fixtures,
            heading="Choose a fixture:",
            prompt="Enter fixture number",
            input_stream=input_stream,
            output_stream=output_stream,
        )
    elif selected_fixture not in available_fixtures:
        raise ValueError(f"unknown fixture id {selected_fixture!r}. Valid fixtures: {format_choices(available_fixtures)}")

    selected_step = (step or "").strip()
    fixture_steps = guide_steps_for_fixture(repo_root, fixture_id=selected_fixture)
    if not selected_step:
        if not interactive:
            raise ValueError(missing_selector_message)
        selected_step = prompt_for_choice(
            fixture_steps,
            heading="Choose a starting step:",
            prompt="Enter step number",
            input_stream=input_stream,
            output_stream=output_stream,
        )
    elif selected_step not in fixture_steps:
        raise ValueError(f"unknown checkpoint step {selected_step!r}. Valid steps: {format_choices(fixture_steps)}")

    return {"fixture_id": selected_fixture, "step": selected_step}


def resolve_checkpoint(
    repo_root: Path,
    *,
    fixture_id: str,
    step: str,
    build_mode_override: str | None = None,
) -> dict[str, Any]:
    repo_root = Path(repo_root).resolve()
    fixtures_manifest = load_json(repo_root / "fixtures" / "manifest.json")
    checkpoint_manifest = load_json(repo_root / "fixtures" / "checkpoints" / "manifest.json")

    fixture_ids = {str(entry["id"]).strip() for entry in fixtures_manifest.get("fixtures", [])}
    if fixture_id not in fixture_ids:
        raise ValueError(f"unknown fixture id {fixture_id!r}")

    matches = [
        entry
        for entry in checkpoint_manifest.get("checkpoints", [])
        if str(entry.get("fixture_id", "")).strip() == fixture_id
        and str(entry.get("step", "")).strip() == step
    ]
    if len(matches) > 1:
        raise ValueError(f"duplicate checkpoint entries for {fixture_id!r} and step {step!r}")

    fallback = len(matches) == 0
    if fallback:
        source_kind = "variant"
        source_id = "pre"
        build_mode = normalize_build_mode(build_mode_override)
        expected_primary_step = None
        warmup_script = None
    else:
        entry = matches[0]
        source_kind = str(entry.get("source_kind", "")).strip()
        if source_kind not in VALID_SOURCE_KINDS:
            raise ValueError(f"unsupported source kind {source_kind!r}")
        source_id = str(entry.get("source_id", "")).strip()
        if not source_id:
            raise ValueError("checkpoint entry is missing source_id")
        build_mode = normalize_build_mode(build_mode_override or str(entry.get("build_mode", DEFAULT_BUILD_MODE)))
        expected_primary_step = str(entry.get("expected_primary_step", "")).strip() or None
        warmup_script = str(entry.get("warmup_script", "")).strip() or None

    if source_kind == "variant":
        repo_path = repo_root / ".fixtures" / fixture_id / source_id
    else:
        repo_path = repo_root / ".fixtures" / "cases" / source_id

    warmup_script_path = None
    if warmup_script is not None:
        warmup_script_path = str((repo_root / warmup_script).resolve())

    return {
        "fixture_id": fixture_id,
        "step": step,
        "checkpoint_key": f"{fixture_id}:{step}",
        "source_kind": source_kind,
        "source_id": source_id,
        "repo_path": str(repo_path),
        "build_mode": build_mode,
        "expected_primary_step": expected_primary_step,
        "warmup_script": warmup_script_path,
        "fallback": fallback,
    }


def compute_image_key(
    *,
    fixture_id: str,
    checkpoint_key: str,
    requested_step: str,
    source_kind: str,
    source_id: str,
    fixture_ref: str,
    python_version: str,
    poetry_version: str,
    claude_version: str,
    concierge_sha: str,
    build_mode: str,
    dockerfile_sha: str,
    runner_sha: str,
    sanitizer_sha: str,
    resolver_sha: str,
    warmup_sha: str,
) -> str:
    payload = {
        "fixture_id": fixture_id,
        "checkpoint_key": checkpoint_key,
        "requested_step": requested_step,
        "source_kind": source_kind,
        "source_id": source_id,
        "fixture_ref": fixture_ref,
        "python_version": python_version,
        "poetry_version": poetry_version,
        "claude_version": claude_version,
        "concierge_sha": concierge_sha,
        "build_mode": normalize_build_mode(build_mode),
        "dockerfile_sha": dockerfile_sha,
        "runner_sha": runner_sha,
        "sanitizer_sha": sanitizer_sha,
        "resolver_sha": resolver_sha,
        "warmup_sha": strings_or_empty(warmup_sha),
    }
    encoded = json.dumps(payload, sort_keys=True).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()[:16]


def strings_or_empty(value: str | None) -> str:
    return (value or "").strip()


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)

    list_fixtures_parser = subparsers.add_parser("list-fixtures")
    list_fixtures_parser.add_argument("--repo-root", required=True)

    list_steps_parser = subparsers.add_parser("list-steps")
    list_steps_parser.add_argument("--repo-root", required=True)
    list_steps_parser.add_argument("--fixture-id", required=True)

    select_runner_parser = subparsers.add_parser("select-runner")
    select_runner_parser.add_argument("--repo-root", required=True)
    select_runner_parser.add_argument("--fixture-id", default=None)
    select_runner_parser.add_argument("--step", default=None)
    select_runner_parser.add_argument("--interactive", action="store_true")

    resolve_parser = subparsers.add_parser("resolve")
    resolve_parser.add_argument("--repo-root", required=True)
    resolve_parser.add_argument("--fixture-id", required=True)
    resolve_parser.add_argument("--step", required=True)
    resolve_parser.add_argument("--build-mode-override", default=None)

    key_parser = subparsers.add_parser("image-key")
    key_parser.add_argument("--fixture-id", required=True)
    key_parser.add_argument("--checkpoint-key", required=True)
    key_parser.add_argument("--requested-step", required=True)
    key_parser.add_argument("--source-kind", required=True)
    key_parser.add_argument("--source-id", required=True)
    key_parser.add_argument("--fixture-ref", required=True)
    key_parser.add_argument("--python-version", required=True)
    key_parser.add_argument("--poetry-version", required=True)
    key_parser.add_argument("--claude-version", required=True)
    key_parser.add_argument("--concierge-sha", required=True)
    key_parser.add_argument("--build-mode", required=True)
    key_parser.add_argument("--dockerfile-sha", required=True)
    key_parser.add_argument("--runner-sha", required=True)
    key_parser.add_argument("--sanitizer-sha", required=True)
    key_parser.add_argument("--resolver-sha", required=True)
    key_parser.add_argument("--warmup-sha", required=True)
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    try:
        if args.command == "list-fixtures":
            print(json.dumps(fixture_ids(Path(args.repo_root))))
            return 0

        if args.command == "list-steps":
            print(json.dumps(guide_steps_for_fixture(Path(args.repo_root), fixture_id=args.fixture_id)))
            return 0

        if args.command == "select-runner":
            print(
                json.dumps(
                    select_runner_target(
                        Path(args.repo_root),
                        fixture_id=args.fixture_id,
                        step=args.step,
                        interactive=args.interactive,
                    ),
                    sort_keys=True,
                )
            )
            return 0

        if args.command == "resolve":
            payload = resolve_checkpoint(
                Path(args.repo_root),
                fixture_id=args.fixture_id,
                step=args.step,
                build_mode_override=args.build_mode_override,
            )
            print(json.dumps(payload, sort_keys=True))
            return 0

        if args.command == "image-key":
            print(
                compute_image_key(
                    fixture_id=args.fixture_id,
                    checkpoint_key=args.checkpoint_key,
                    requested_step=args.requested_step,
                    source_kind=args.source_kind,
                    source_id=args.source_id,
                    fixture_ref=args.fixture_ref,
                    python_version=args.python_version,
                    poetry_version=args.poetry_version,
                    claude_version=args.claude_version,
                    concierge_sha=args.concierge_sha,
                    build_mode=args.build_mode,
                    dockerfile_sha=args.dockerfile_sha,
                    runner_sha=args.runner_sha,
                    sanitizer_sha=args.sanitizer_sha,
                    resolver_sha=args.resolver_sha,
                    warmup_sha=args.warmup_sha,
                )
            )
            return 0
    except ValueError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    parser.error(f"unknown command {args.command!r}")
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
