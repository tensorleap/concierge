#!/usr/bin/env python3
"""Entrypoint for the Concierge runtime harness."""

from __future__ import annotations

import argparse
import json
import sys


def _emit_runtime_failure(message: str) -> None:
    print(
        json.dumps(
            {
                "event": "runtime_failed",
                "status": "failed",
                "message": message,
            },
            sort_keys=True,
        ),
        flush=True,
    )


def _format_exception(exc: Exception) -> str:
    return f"{exc.__class__.__name__}: {exc}"


def main() -> int:
    parser = argparse.ArgumentParser(description="Concierge runtime harness")
    parser.add_argument("--repo-root", required=True)
    parser.add_argument("--entry-file", default="leap_integration.py")
    parser.add_argument("--sample-budget", type=int, default=3)
    args = parser.parse_args()

    try:
        from harness_lib.runner import HarnessRuntime
    except Exception as exc:
        _emit_runtime_failure(f"runtime harness bootstrap failed: {_format_exception(exc)}")
        return 0

    try:
        HarnessRuntime(
            repo_root=args.repo_root,
            entry_file=args.entry_file,
            sample_budget=args.sample_budget,
        ).run()
    except Exception as exc:
        _emit_runtime_failure(f"runtime harness crashed: {_format_exception(exc)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
