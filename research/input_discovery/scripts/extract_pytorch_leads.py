#!/usr/bin/env python3
"""Temporary research script: extract PyTorch lead files for agent investigation.

This script is intentionally research-only and not production Concierge code.
"""

from __future__ import annotations

import argparse
import json
import re
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Iterable

SCHEMA_VERSION = "1.0.0"
METHOD_VERSION = "pytorch-leads-v1"


@dataclass(frozen=True)
class Signal:
    id: str
    description: str
    tier: str
    weight: float
    pattern: str
    flags: int = 0


SIGNALS = [
    Signal(
        id="dataloader_import",
        description="Imports from torch.utils.data related to DataLoader",
        tier="primary",
        weight=6.0,
        pattern=r"\bfrom\s+torch\.utils\.data\s+import\s+.*\bDataLoader\b|\bimport\s+torch\.utils\.data\b",
    ),
    Signal(
        id="dataloader_call",
        description="DataLoader(...) call site",
        tier="primary",
        weight=10.0,
        pattern=r"\bDataLoader\s*\(",
    ),
    Signal(
        id="dataset_subclass",
        description="class X(...Dataset...) definition",
        tier="primary",
        weight=8.0,
        pattern=r"^\s*class\s+\w+\s*\([^)]*Dataset[^)]*\)\s*:",
    ),
    Signal(
        id="dataset_import",
        description="Dataset import from torch.utils.data",
        tier="primary",
        weight=5.0,
        pattern=r"\bfrom\s+torch\.utils\.data\s+import\s+.*\bDataset\b",
    ),
    Signal(
        id="train_fn",
        description="Training function definition",
        tier="primary",
        weight=6.0,
        pattern=r"^\s*def\s+(train|fit|training_step)\b",
    ),
    Signal(
        id="validate_fn",
        description="Validation/evaluation function definition",
        tier="primary",
        weight=6.0,
        pattern=r"^\s*def\s+(validate|validation|val|evaluate|eval|test)\b",
    ),
    Signal(
        id="main_entry",
        description="Python __main__ entry point",
        tier="primary",
        weight=3.0,
        pattern=r"__name__\s*==\s*['\"]__main__['\"]",
    ),
    Signal(
        id="collate_fn",
        description="collate_fn mention",
        tier="secondary",
        weight=5.0,
        pattern=r"\bcollate_fn\b",
    ),
    Signal(
        id="batch_unpack_loop",
        description="for-loop tuple unpacking pattern",
        tier="secondary",
        weight=5.0,
        pattern=r"^\s*for\s+[^:\n]*,\s*[^:\n]*\s+in\s+.*:",
    ),
    Signal(
        id="transform_chain",
        description="transform/torchvision transform usage",
        tier="secondary",
        weight=4.0,
        pattern=r"\btransforms?\.",
    ),
    Signal(
        id="model_call",
        description="model(...) invocation",
        tier="secondary",
        weight=3.0,
        pattern=r"\bmodel\s*\(",
    ),
    Signal(
        id="criterion_call",
        description="criterion(...) invocation",
        tier="secondary",
        weight=2.0,
        pattern=r"\bcriterion\s*\(",
    ),
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Extract PyTorch lead files for research")
    parser.add_argument("--repo", required=True, help="Repository root to scan")
    parser.add_argument("--output-dir", required=True, help="Output root for experiment artifacts")
    parser.add_argument("--experiment-id", default="", help="Explicit experiment id")
    parser.add_argument("--repo-id", default="", help="Repository id for experiment naming")
    parser.add_argument("--repo-variant", default="pre", help="Repository variant (default: pre)")
    parser.add_argument("--run-id", default="", help="Run id (default: timestamp-derived)")
    parser.add_argument("--top-n", type=int, default=20, help="Max lead files to include")
    parser.add_argument(
        "--max-occurrences",
        type=int,
        default=5,
        help="Max stored line occurrences per signal per file",
    )
    return parser.parse_args()


def utc_now_iso() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat()


def default_run_id() -> str:
    return "r" + datetime.now(timezone.utc).strftime("%Y%m%d%H%M%S")


def clip_snippet(line: str, limit: int = 200) -> str:
    snippet = line.strip()
    return snippet[:limit] if len(snippet) > limit else snippet


def iter_python_files(root: Path) -> Iterable[Path]:
    skip_dirs = {
        ".git",
        ".venv",
        "venv",
        "__pycache__",
        "build",
        "dist",
        ".mypy_cache",
        ".pytest_cache",
    }
    for path in root.rglob("*.py"):
        parts = set(path.parts)
        if skip_dirs.intersection(parts):
            continue
        yield path


def scan_file(path: Path, compiled_signals: list[tuple[Signal, re.Pattern[str]]], max_occurrences: int) -> dict | None:
    try:
        lines = path.read_text(encoding="utf-8", errors="ignore").splitlines()
    except OSError:
        return None

    signal_hits = []
    total_score = 0.0

    for signal, regex in compiled_signals:
        occurrences = []
        count = 0
        for line_no, line in enumerate(lines, start=1):
            if regex.search(line):
                count += 1
                if len(occurrences) < max_occurrences:
                    occurrences.append({"line": line_no, "snippet": clip_snippet(line)})

        if count == 0:
            continue

        contribution = signal.weight * min(count, 5)
        total_score += contribution
        signal_hits.append(
            {
                "signal_id": signal.id,
                "count": count,
                "contribution": round(contribution, 2),
                "occurrences": occurrences,
            }
        )

    if not signal_hits:
        return None

    signal_hits.sort(key=lambda x: (-x["contribution"], x["signal_id"]))
    return {
        "path": str(path),
        "score": round(total_score, 2),
        "signal_hits": signal_hits,
    }


def build_summary(experiment_id: str, repo_path: Path, files: list[dict], totals: dict) -> str:
    lines = []
    lines.append(f"Experiment: {experiment_id}")
    lines.append(f"Repo: {repo_path}")
    lines.append(f"Method: {METHOD_VERSION}")
    lines.append(f"Python files scanned: {totals['python_files_scanned']}")
    lines.append(f"Files with hits: {totals['files_with_hits']}")
    lines.append(f"Total signal hits: {totals['signal_hit_count']}")
    lines.append("")
    lines.append("Top lead files:")

    for idx, item in enumerate(files, start=1):
        lines.append(f"{idx}. {item['path']} (score={item['score']})")
        top_signals = item["signal_hits"][:3]
        for hit in top_signals:
            lines.append(
                f"   - {hit['signal_id']}: count={hit['count']}, contribution={hit['contribution']}"
            )
        lines.append("")

    return "\n".join(lines).rstrip() + "\n"


def main() -> int:
    args = parse_args()

    repo_path = Path(args.repo).resolve()
    if not repo_path.exists() or not repo_path.is_dir():
        raise SystemExit(f"--repo does not exist or is not a directory: {repo_path}")

    output_root = Path(args.output_dir).resolve()
    output_root.mkdir(parents=True, exist_ok=True)

    repo_id = args.repo_id or repo_path.name
    run_id = args.run_id or default_run_id()
    experiment_id = (
        args.experiment_id
        if args.experiment_id
        else f"{repo_id}__{args.repo_variant}__{METHOD_VERSION}__{run_id}"
    )

    experiment_dir = output_root / experiment_id
    experiment_dir.mkdir(parents=True, exist_ok=False)

    compiled_signals = [(signal, re.compile(signal.pattern, signal.flags)) for signal in SIGNALS]

    scanned = 0
    file_hits = []
    signal_hit_count = 0

    for path in iter_python_files(repo_path):
        scanned += 1
        item = scan_file(path, compiled_signals, args.max_occurrences)
        if item is None:
            continue

        item["path"] = str(Path(item["path"]).relative_to(repo_path))
        file_hits.append(item)
        signal_hit_count += sum(hit["count"] for hit in item["signal_hits"])

    file_hits.sort(key=lambda x: (-x["score"], x["path"]))
    top_files = file_hits[: args.top_n]

    lead_pack = {
        "schema_version": SCHEMA_VERSION,
        "method_version": METHOD_VERSION,
        "generated_at": utc_now_iso(),
        "experiment": {
            "id": experiment_id,
            "repo_id": repo_id,
            "repo_variant": args.repo_variant,
            "run_id": run_id,
        },
        "repo": {
            "path": str(repo_path),
            "python_files_scanned": scanned,
        },
        "signals": [
            {
                "id": signal.id,
                "description": signal.description,
                "weight": signal.weight,
                "tier": signal.tier,
            }
            for signal in SIGNALS
        ],
        "files": top_files,
        "totals": {
            "files_with_hits": len(file_hits),
            "signal_hit_count": signal_hit_count,
        },
    }

    lead_pack_path = experiment_dir / "lead_pack.json"
    summary_path = experiment_dir / "lead_summary.txt"

    lead_pack_path.write_text(json.dumps(lead_pack, indent=2) + "\n", encoding="utf-8")
    summary_path.write_text(
        build_summary(experiment_id=experiment_id, repo_path=repo_path, files=top_files, totals={
            "python_files_scanned": scanned,
            "files_with_hits": len(file_hits),
            "signal_hit_count": signal_hit_count,
        }),
        encoding="utf-8",
    )

    print(f"experiment_id={experiment_id}")
    print(f"lead_pack={lead_pack_path}")
    print(f"summary={summary_path}")
    print(f"python_files_scanned={scanned}")
    print(f"files_with_hits={len(file_hits)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
