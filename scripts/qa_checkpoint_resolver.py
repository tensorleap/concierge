#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any, Mapping, TextIO

DEFAULT_BUILD_MODE = "cold"
VALID_BUILD_MODES = {"cold", "prewarmed"}
VALID_SOURCE_KINDS = {"variant", "case"}
VALID_RUNTIME_PREREQUISITE_KINDS = {"local_file"}
VALID_RUNTIME_PREREQUISITE_BACKENDS = {"local", "github_actions"}
GUIDE_STEP_ORDER = (
    "integration_script",
    "preprocess",
    "input_encoders",
    "model_acquisition",
    "integration_test",
    "ground_truth_encoders",
)
PRE_STEP = "pre"
RUNTIME_PREREQUISITES_LOCAL_CONFIG = Path("fixtures/runtime_prerequisites.local.json")
RUNTIME_PREREQUISITES_CONTAINER_ROOT = Path("/runtime-prerequisites")


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def fixture_entry(repo_root: Path, fixture_id: str) -> dict[str, Any]:
    fixtures_manifest = load_json(repo_root / "fixtures" / "manifest.json")
    for entry in fixtures_manifest.get("fixtures", []):
        if str(entry.get("id", "")).strip() == fixture_id:
            return entry
    raise ValueError(f"unknown fixture id {fixture_id!r}")


def normalize_runtime_prerequisite(raw: dict[str, Any]) -> dict[str, Any]:
    prerequisite_id = str(raw.get("id", "")).strip()
    if not prerequisite_id:
        raise ValueError("runtime prerequisite is missing id")

    kind = str(raw.get("kind", "")).strip()
    if kind not in VALID_RUNTIME_PREREQUISITE_KINDS:
        raise ValueError(f"runtime prerequisite {prerequisite_id!r} has unsupported kind {kind!r}")

    mount_path = str(raw.get("mount_path", "")).strip()
    if not mount_path:
        raise ValueError(f"runtime prerequisite {prerequisite_id!r} is missing mount_path")

    mount_path_obj = Path(mount_path)
    if not mount_path_obj.is_absolute():
        raise ValueError(f"runtime prerequisite {prerequisite_id!r} mount_path must be absolute")
    try:
        mount_path_obj.relative_to(RUNTIME_PREREQUISITES_CONTAINER_ROOT)
    except ValueError as exc:
        raise ValueError(
            f"runtime prerequisite {prerequisite_id!r} mount_path must stay under {RUNTIME_PREREQUISITES_CONTAINER_ROOT}"
        ) from exc

    local_resolution = raw.get("local_resolution", {})
    if local_resolution is None:
        local_resolution = {}
    if not isinstance(local_resolution, dict):
        raise ValueError(f"runtime prerequisite {prerequisite_id!r} local_resolution must be an object")

    github_actions = raw.get("github_actions", {})
    if github_actions is None:
        github_actions = {}
    if not isinstance(github_actions, dict):
        raise ValueError(f"runtime prerequisite {prerequisite_id!r} github_actions must be an object")

    validation = raw.get("validation", {})
    if validation is None:
        validation = {}
    if not isinstance(validation, dict):
        raise ValueError(f"runtime prerequisite {prerequisite_id!r} validation must be an object")

    env_vars = [str(value).strip() for value in local_resolution.get("env_vars", []) if str(value).strip()]
    config_keys = [str(value).strip() for value in local_resolution.get("config_keys", []) if str(value).strip()]
    auth_env_vars = [str(value).strip() for value in github_actions.get("auth_env_vars", []) if str(value).strip()]

    normalized = {
        "id": prerequisite_id,
        "kind": kind,
        "required": bool(raw.get("required", True)),
        "mount_path": mount_path,
        "description": str(raw.get("description", "")).strip(),
        "operator_guidance": str(raw.get("operator_guidance", "")).strip(),
        "local_resolution": {
            "env_vars": env_vars,
            "config_keys": config_keys,
        },
        "github_actions": {
            "fetch_kind": str(github_actions.get("fetch_kind", "")).strip(),
            "repo": str(github_actions.get("repo", "")).strip(),
            "ref": str(github_actions.get("ref", "")).strip(),
            "path": str(github_actions.get("path", "")).strip(),
            "auth_env_vars": auth_env_vars,
        },
        "validation": {
            "extension": str(validation.get("extension", "")).strip(),
            "filename_hint": str(validation.get("filename_hint", "")).strip(),
            "checksum_sha256": str(
                validation.get("checksum_sha256", "") or validation.get("sha256", "")
            ).strip(),
        },
    }
    return normalized


def runtime_prerequisites_for_fixture(repo_root: Path, *, fixture_id: str) -> list[dict[str, Any]]:
    entry = fixture_entry(repo_root, fixture_id)
    raw_prerequisites = entry.get("runtime_prerequisites", [])
    if raw_prerequisites in (None, ""):
        return []
    if not isinstance(raw_prerequisites, list):
        raise ValueError(f"fixture {fixture_id!r} runtime_prerequisites must be a list")
    return [normalize_runtime_prerequisite(item) for item in raw_prerequisites]


def runtime_prerequisites_local_config_path(repo_root: Path, override: Path | None = None) -> Path:
    base_path = override if override is not None else repo_root / RUNTIME_PREREQUISITES_LOCAL_CONFIG
    return Path(base_path).expanduser().resolve()


def load_runtime_prerequisites_local_config(path: Path) -> dict[str, str]:
    if not path.is_file():
        return {}

    payload = load_json(path)
    raw_values: Any = payload.get("runtime_prerequisites", payload)
    if not isinstance(raw_values, dict):
        raise ValueError(f"runtime prerequisites local config must contain an object: {path}")

    resolved: dict[str, str] = {}
    for key, value in raw_values.items():
        candidate = value.get("path", "") if isinstance(value, dict) else value
        text = str(candidate or "").strip()
        if text:
            resolved[str(key).strip()] = text
    return resolved


def validate_runtime_prerequisite_file(
    source_path: Path,
    *,
    prerequisite: dict[str, Any],
) -> None:
    prerequisite_id = prerequisite["id"]
    if not source_path.is_file():
        raise ValueError(f"runtime prerequisite {prerequisite_id!r} resolved to missing file: {source_path}")
    if not os.access(source_path, os.R_OK):
        raise ValueError(f"runtime prerequisite {prerequisite_id!r} is not readable: {source_path}")

    validation = prerequisite.get("validation", {})
    extension = str(validation.get("extension", "")).strip()
    if extension and source_path.suffix != extension:
        raise ValueError(
            f"runtime prerequisite {prerequisite_id!r} expected extension {extension!r}, got {source_path.suffix!r}"
        )

    filename_hint = str(validation.get("filename_hint", "")).strip()
    if filename_hint and source_path.name != filename_hint:
        raise ValueError(
            f"runtime prerequisite {prerequisite_id!r} expected filename {filename_hint!r}, got {source_path.name!r}"
        )

    expected_checksum = str(validation.get("checksum_sha256", "")).strip()
    if expected_checksum:
        digest = hashlib.sha256()
        with source_path.open("rb") as handle:
            for chunk in iter(lambda: handle.read(1024 * 1024), b""):
                digest.update(chunk)
        actual_checksum = digest.hexdigest()
        if actual_checksum != expected_checksum:
            raise ValueError(
                f"runtime prerequisite {prerequisite_id!r} checksum mismatch: expected {expected_checksum}, got {actual_checksum}"
            )


def resolve_local_runtime_prerequisite_source(
    prerequisite: dict[str, Any],
    *,
    config_values: dict[str, str],
    env: Mapping[str, str],
) -> tuple[Path | None, str]:
    local_resolution = prerequisite.get("local_resolution", {})
    for env_var in local_resolution.get("env_vars", []):
        candidate = str(env.get(env_var, "")).strip()
        if candidate:
            return Path(candidate).expanduser().resolve(), f"env:{env_var}"

    for config_key in local_resolution.get("config_keys", []):
        candidate = str(config_values.get(config_key, "")).strip()
        if candidate:
            return Path(candidate).expanduser().resolve(), f"config:{config_key}"

    return None, ""


def authenticated_repo_url(repo: str, *, auth_env_vars: list[str], env: Mapping[str, str]) -> str:
    token = ""
    for env_var in auth_env_vars:
        candidate = str(env.get(env_var, "")).strip()
        if candidate:
            token = candidate
            break

    if token and repo.startswith("https://github.com/"):
        return f"https://x-access-token:{token}@github.com/{repo.removeprefix('https://github.com/')}"
    return repo


def fetch_github_actions_runtime_prerequisite_source(
    prerequisite: dict[str, Any],
    *,
    env: Mapping[str, str],
    scratch_root: Path,
) -> tuple[Path | None, str]:
    github_actions = prerequisite.get("github_actions", {})
    fetch_kind = str(github_actions.get("fetch_kind", "")).strip()
    if not fetch_kind:
        return None, ""
    if fetch_kind != "git_repo_file":
        raise ValueError(
            f"runtime prerequisite {prerequisite['id']!r} has unsupported github_actions fetch_kind {fetch_kind!r}"
        )

    repo = str(github_actions.get("repo", "")).strip()
    ref = str(github_actions.get("ref", "")).strip()
    rel_path = str(github_actions.get("path", "")).strip().lstrip("/")
    if not repo or not ref or not rel_path:
        raise ValueError(
            f"runtime prerequisite {prerequisite['id']!r} github_actions git_repo_file fetch is missing repo/ref/path"
        )

    checkout_dir = scratch_root / prerequisite["id"]
    if checkout_dir.exists():
        shutil.rmtree(checkout_dir)

    clone_repo = authenticated_repo_url(
        repo,
        auth_env_vars=github_actions.get("auth_env_vars", []),
        env=env,
    )
    try:
        subprocess.run(
            ["git", "clone", "--quiet", "--no-checkout", "--filter=blob:none", clone_repo, str(checkout_dir)],
            check=True,
            capture_output=True,
            text=True,
        )
        subprocess.run(
            ["git", "-C", str(checkout_dir), "sparse-checkout", "init", "--no-cone"],
            check=True,
            capture_output=True,
            text=True,
        )
        subprocess.run(
            ["git", "-C", str(checkout_dir), "sparse-checkout", "set", "--skip-checks", "--", rel_path],
            check=True,
            capture_output=True,
            text=True,
        )
        subprocess.run(
            ["git", "-C", str(checkout_dir), "checkout", "--quiet", ref],
            check=True,
            capture_output=True,
            text=True,
        )
    except subprocess.CalledProcessError as exc:
        stderr = exc.stderr.strip() if exc.stderr else exc.stdout.strip()
        raise ValueError(
            f"runtime prerequisite {prerequisite['id']!r} github_actions fetch failed: {stderr or exc}"
        ) from exc

    source_path = (checkout_dir / rel_path).resolve()
    return source_path, "github_actions:git_repo_file"


def stage_runtime_prerequisites(
    repo_root: Path,
    *,
    fixture_id: str,
    stage_root: Path,
    backend: str,
    env: Mapping[str, str] | None = None,
    local_config_path: Path | None = None,
) -> dict[str, Any]:
    repo_root = Path(repo_root).resolve()
    stage_root = Path(stage_root).resolve()
    backend = str(backend or "").strip()
    if backend not in VALID_RUNTIME_PREREQUISITE_BACKENDS:
        raise ValueError(f"unsupported runtime prerequisite backend {backend!r}")

    env_map: Mapping[str, str] = os.environ if env is None else env
    prerequisites = runtime_prerequisites_for_fixture(repo_root, fixture_id=fixture_id)
    config_path = runtime_prerequisites_local_config_path(repo_root, override=local_config_path)
    config_values = load_runtime_prerequisites_local_config(config_path) if backend == "local" else {}
    stage_root.mkdir(parents=True, exist_ok=True)

    staged_runtime_prerequisites: list[dict[str, Any]] = []
    with tempfile.TemporaryDirectory(prefix="qa-runtime-prereq-fetch-", dir=str(stage_root.parent)) as scratch_dir:
        scratch_root = Path(scratch_dir)
        for prerequisite in prerequisites:
            source_path: Path | None
            resolution_source = ""
            if backend == "local":
                source_path, resolution_source = resolve_local_runtime_prerequisite_source(
                    prerequisite,
                    config_values=config_values,
                    env=env_map,
                )
            else:
                source_path, resolution_source = fetch_github_actions_runtime_prerequisite_source(
                    prerequisite,
                    env=env_map,
                    scratch_root=scratch_root,
                )

            if source_path is None:
                if prerequisite.get("required", True):
                    resolution_keys = prerequisite.get("local_resolution", {}) if backend == "local" else prerequisite.get("github_actions", {})
                    raise ValueError(
                        f"required runtime prerequisite {prerequisite['id']!r} is unresolved for backend {backend!r}: {resolution_keys}"
                    )
                continue

            validate_runtime_prerequisite_file(source_path, prerequisite=prerequisite)

            relative_mount_path = Path(prerequisite["mount_path"]).relative_to(RUNTIME_PREREQUISITES_CONTAINER_ROOT)
            staged_path = (stage_root / relative_mount_path).resolve()
            staged_path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source_path, staged_path)

            staged_runtime_prerequisite = json.loads(json.dumps(prerequisite))
            staged_runtime_prerequisite["resolution_source"] = resolution_source
            staged_runtime_prerequisite["staged_host_path"] = str(staged_path)
            staged_runtime_prerequisites.append(staged_runtime_prerequisite)

    return {
        "fixture_id": fixture_id,
        "backend": backend,
        "stage_root": str(stage_root),
        "docker_mount_target": str(RUNTIME_PREREQUISITES_CONTAINER_ROOT),
        "runtime_prerequisites": staged_runtime_prerequisites,
        "local_config_path": str(config_path),
    }


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
    return [PRE_STEP, *GUIDE_STEP_ORDER]


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
    available_steps = [PRE_STEP, *GUIDE_STEP_ORDER]
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
    runtime_prerequisites = runtime_prerequisites_for_fixture(repo_root, fixture_id=fixture_id)

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

    fallback = step == PRE_STEP or len(matches) == 0
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

    prepare_case_id = source_id if source_kind == "case" else None

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
        "requires_case_generation": prepare_case_id is not None,
        "prepare_case_id": prepare_case_id,
        "runtime_prerequisites": runtime_prerequisites,
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

    stage_runtime_prerequisites_parser = subparsers.add_parser("stage-runtime-prerequisites")
    stage_runtime_prerequisites_parser.add_argument("--repo-root", required=True)
    stage_runtime_prerequisites_parser.add_argument("--fixture-id", required=True)
    stage_runtime_prerequisites_parser.add_argument("--stage-root", required=True)
    stage_runtime_prerequisites_parser.add_argument("--backend", required=True)
    stage_runtime_prerequisites_parser.add_argument("--local-config-path", default=None)

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

        if args.command == "stage-runtime-prerequisites":
            payload = stage_runtime_prerequisites(
                Path(args.repo_root),
                fixture_id=args.fixture_id,
                stage_root=Path(args.stage_root),
                backend=args.backend,
                local_config_path=Path(args.local_config_path) if args.local_config_path else None,
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
