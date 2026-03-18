#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any

DEFAULT_BUILD_MODE = "cold"
VALID_BUILD_MODES = {"cold", "prewarmed"}
VALID_SOURCE_KINDS = {"variant", "case"}


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def normalize_build_mode(value: str | None) -> str:
    candidate = (value or DEFAULT_BUILD_MODE).strip()
    if candidate not in VALID_BUILD_MODES:
        raise ValueError(f"unsupported build mode {candidate!r}")
    return candidate


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

    if source_kind == "variant":
        repo_path = repo_root / ".fixtures" / fixture_id / source_id
    else:
        repo_path = repo_root / ".fixtures" / "cases" / source_id

    return {
        "fixture_id": fixture_id,
        "step": step,
        "checkpoint_key": f"{fixture_id}:{step}",
        "source_kind": source_kind,
        "source_id": source_id,
        "repo_path": str(repo_path),
        "build_mode": build_mode,
        "expected_primary_step": expected_primary_step,
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
    resolver_sha: str,
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
        "resolver_sha": resolver_sha,
    }
    encoded = json.dumps(payload, sort_keys=True).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()[:16]


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)

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
    key_parser.add_argument("--resolver-sha", required=True)
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

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
                resolver_sha=args.resolver_sha,
            )
        )
        return 0

    parser.error(f"unknown command {args.command!r}")
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
