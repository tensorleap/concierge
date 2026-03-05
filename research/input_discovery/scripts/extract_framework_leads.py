#!/usr/bin/env python3
"""Temporary research script: extract multi-framework lead files for agent analysis.

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
METHOD_VERSION = "framework-leads-v1"


@dataclass(frozen=True)
class Signal:
    id: str
    description: str
    tier: str
    framework: str
    weight: float
    pattern: str
    flags: int = 0


SIGNALS = [
    # PyTorch-specific
    Signal(
        id="torch_import",
        description="Imports torch or modules under torch.*",
        tier="primary",
        framework="pytorch",
        weight=5.0,
        pattern=r"\bimport\s+torch\b|\bfrom\s+torch\b",
    ),
    Signal(
        id="dataloader_import",
        description="Imports DataLoader from torch.utils.data",
        tier="primary",
        framework="pytorch",
        weight=7.0,
        pattern=r"\bfrom\s+torch\.utils\.data\s+import\s+.*\bDataLoader\b|\bimport\s+torch\.utils\.data\b",
    ),
    Signal(
        id="dataloader_call",
        description="DataLoader(...) call site",
        tier="primary",
        framework="pytorch",
        weight=10.0,
        pattern=r"\bDataLoader\s*\(",
    ),
    Signal(
        id="dataset_subclass",
        description="class X(...Dataset...) definition",
        tier="primary",
        framework="pytorch",
        weight=8.0,
        pattern=r"^\s*class\s+\w+\s*\([^)]*Dataset[^)]*\)\s*:",
    ),
    Signal(
        id="dataset_import",
        description="Dataset import from torch.utils.data",
        tier="primary",
        framework="pytorch",
        weight=5.0,
        pattern=r"\bfrom\s+torch\.utils\.data\s+import\s+.*\bDataset\b",
    ),
    Signal(
        id="torch_forward_def",
        description="forward(...) method definition",
        tier="secondary",
        framework="pytorch",
        weight=4.0,
        pattern=r"^\s*def\s+forward\s*\(",
    ),
    Signal(
        id="torch_load",
        description="torch.load(...) call",
        tier="secondary",
        framework="pytorch",
        weight=3.0,
        pattern=r"\btorch\.load\s*\(",
    ),
    # TensorFlow/Keras-specific
    Signal(
        id="tensorflow_import",
        description="Imports tensorflow",
        tier="primary",
        framework="tensorflow",
        weight=6.0,
        pattern=r"\bimport\s+tensorflow\b|\bimport\s+tensorflow\s+as\s+tf\b|\bfrom\s+tensorflow\b",
    ),
    Signal(
        id="keras_import",
        description="Imports keras APIs",
        tier="primary",
        framework="tensorflow",
        weight=5.0,
        pattern=r"\bfrom\s+tensorflow\.keras\b|\bfrom\s+keras\b|\bimport\s+keras\b",
    ),
    Signal(
        id="tf_data_dataset",
        description="tf.data.Dataset usage",
        tier="primary",
        framework="tensorflow",
        weight=9.0,
        pattern=r"\btf\.data\.Dataset\b",
    ),
    Signal(
        id="tf_data_constructors",
        description="tf.data constructor usage",
        tier="primary",
        framework="tensorflow",
        weight=8.0,
        pattern=r"\b(from_tensor_slices|from_generator|list_files)\s*\(",
    ),
    Signal(
        id="tf_record_or_text_dataset",
        description="TFRecord/TextLine/Csv dataset reader usage",
        tier="primary",
        framework="tensorflow",
        weight=8.0,
        pattern=r"\b(TFRecordDataset|TextLineDataset|CsvDataset)\s*\(",
    ),
    Signal(
        id="tf_data_pipeline_ops",
        description="tf.data pipeline ops (map/batch/shuffle/prefetch/etc.)",
        tier="secondary",
        framework="tensorflow",
        weight=6.0,
        pattern=r"\.(map|batch|padded_batch|shuffle|repeat|prefetch|cache|interleave)\s*\(",
    ),
    Signal(
        id="keras_fit",
        description="Keras fit(...) call",
        tier="primary",
        framework="tensorflow",
        weight=8.0,
        pattern=r"\.fit\s*\(",
    ),
    Signal(
        id="keras_evaluate_or_predict",
        description="Keras evaluate/predict(...) call",
        tier="secondary",
        framework="tensorflow",
        weight=6.0,
        pattern=r"\.(evaluate|predict)\s*\(",
    ),
    Signal(
        id="keras_dataset_utils",
        description="Keras dataset loader utility usage",
        tier="primary",
        framework="tensorflow",
        weight=7.0,
        pattern=r"\b(image_dataset_from_directory|text_dataset_from_directory|audio_dataset_from_directory|timeseries_dataset_from_array)\s*\(",
    ),
    Signal(
        id="keras_sequence_or_pydataset",
        description="Keras Sequence/PyDataset usage",
        tier="secondary",
        framework="tensorflow",
        weight=5.0,
        pattern=r"\b(Sequence|PyDataset)\b",
    ),
    Signal(
        id="tfds_load",
        description="tensorflow_datasets usage",
        tier="secondary",
        framework="tensorflow",
        weight=6.0,
        pattern=r"\btfds\.load\s*\(|\btensorflow_datasets\b",
    ),
    # Framework-agnostic
    Signal(
        id="train_fn",
        description="Training function definition",
        tier="primary",
        framework="agnostic",
        weight=5.0,
        pattern=r"^\s*def\s+(train|fit|training_step)\b",
    ),
    Signal(
        id="validate_fn",
        description="Validation/evaluation function definition",
        tier="primary",
        framework="agnostic",
        weight=5.0,
        pattern=r"^\s*def\s+(validate|validation|val|evaluate|eval|test)\b",
    ),
    Signal(
        id="main_entry",
        description="Python __main__ entry point",
        tier="secondary",
        framework="agnostic",
        weight=3.0,
        pattern=r"__name__\s*==\s*['\"]__main__['\"]",
    ),
    Signal(
        id="batch_unpack_loop",
        description="for-loop tuple unpacking pattern",
        tier="secondary",
        framework="agnostic",
        weight=4.0,
        pattern=r"^\s*for\s+[^:\n]*,\s*[^:\n]*\s+in\s+.*:",
    ),
    Signal(
        id="loss_call",
        description="loss(...) or criterion(...) usage",
        tier="secondary",
        framework="agnostic",
        weight=3.0,
        pattern=r"\b(loss|criterion)\s*\(",
    ),
    Signal(
        id="model_call",
        description="model(...) invocation",
        tier="secondary",
        framework="agnostic",
        weight=3.0,
        pattern=r"\bmodel\s*\(",
    ),
]

ARTIFACT_SUFFIX_WEIGHTS = {
    "tensorflow": {
        ".h5": 14.0,
        ".keras": 14.0,
        ".tflite": 12.0,
        ".pb": 10.0,
    },
    "pytorch": {
        ".pt": 14.0,
        ".pth": 14.0,
    },
}

DEPENDENCY_FILES = [
    "requirements.txt",
    "pyproject.toml",
    "poetry.lock",
    "Pipfile",
    "environment.yml",
    "setup.py",
]

DEPENDENCY_PATTERNS = {
    "tensorflow": [
        r"\btensorflow\b",
        r"\bkeras\b",
        r"\btensorflow-datasets\b",
        r"\btfds\b",
    ],
    "pytorch": [
        r"\btorch\b",
        r"\btorchvision\b",
        r"\btorchaudio\b",
        r"\bpytorch-lightning\b",
        r"\blightning\b",
    ],
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Extract multi-framework leads for research")
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
        if skip_dirs.intersection(path.parts):
            continue
        yield path


def scan_file(path: Path, compiled_signals: list[tuple[Signal, re.Pattern[str]]], max_occurrences: int) -> dict | None:
    try:
        lines = path.read_text(encoding="utf-8", errors="ignore").splitlines()
    except OSError:
        return None

    signal_hits = []
    total_score = 0.0
    framework_scores = {"pytorch": 0.0, "tensorflow": 0.0}

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
        if signal.framework in framework_scores:
            framework_scores[signal.framework] += contribution

        signal_hits.append(
            {
                "signal_id": signal.id,
                "framework": signal.framework,
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
        "framework_scores": {k: round(v, 2) for k, v in framework_scores.items()},
        "signal_hits": signal_hits,
    }


def detect_framework_artifacts(repo_path: Path) -> tuple[dict[str, float], list[dict]]:
    scores = {"pytorch": 0.0, "tensorflow": 0.0}
    evidence: list[dict] = []
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

    for path in repo_path.rglob("*"):
        if not path.is_file():
            continue
        if skip_dirs.intersection(path.parts):
            continue

        suffix = path.suffix.lower()
        for framework, mapping in ARTIFACT_SUFFIX_WEIGHTS.items():
            if suffix in mapping:
                weight = mapping[suffix]
                scores[framework] += weight
                evidence.append(
                    {
                        "framework": framework,
                        "type": "model_artifact",
                        "path": str(path.relative_to(repo_path)),
                        "detail": f"suffix={suffix}",
                        "weight": round(weight, 2),
                    }
                )

    for rel_path in DEPENDENCY_FILES:
        file_path = repo_path / rel_path
        if not file_path.exists() or not file_path.is_file():
            continue
        text = file_path.read_text(encoding="utf-8", errors="ignore").lower()
        for framework, patterns in DEPENDENCY_PATTERNS.items():
            matched = []
            for pattern in patterns:
                if re.search(pattern, text):
                    matched.append(pattern.strip("\\b"))
            if not matched:
                continue
            weight = 4.0 + min(len(matched), 3)
            scores[framework] += weight
            evidence.append(
                {
                    "framework": framework,
                    "type": "dependency_file",
                    "path": rel_path,
                    "detail": f"matched={','.join(sorted(set(matched)))}",
                    "weight": round(weight, 2),
                }
            )

    evidence.sort(key=lambda x: (-x["weight"], x["path"]))
    return {k: round(v, 2) for k, v in scores.items()}, evidence


def classify_framework(pytorch_score: float, tensorflow_score: float) -> tuple[str, str]:
    if pytorch_score <= 0 and tensorflow_score <= 0:
        return "unknown", "low"

    top = max(pytorch_score, tensorflow_score)
    low = min(pytorch_score, tensorflow_score)
    if low > 0 and (low / top) >= 0.6:
        if low >= 12 and top >= 12:
            return "mixed", "high"
        return "mixed", "medium"

    if pytorch_score > tensorflow_score:
        framework = "pytorch"
        other = tensorflow_score
        major = pytorch_score
    else:
        framework = "tensorflow"
        other = pytorch_score
        major = tensorflow_score

    if major >= 20 and (other == 0 or major >= (other * 1.8)):
        return framework, "high"
    if major >= 8:
        return framework, "medium"
    return framework, "low"


def build_summary(
    *,
    experiment_id: str,
    repo_path: Path,
    files: list[dict],
    totals: dict,
    framework_detection: dict,
) -> str:
    lines = []
    lines.append(f"Experiment: {experiment_id}")
    lines.append(f"Repo: {repo_path}")
    lines.append(f"Method: {METHOD_VERSION}")
    lines.append(f"Python files scanned: {totals['python_files_scanned']}")
    lines.append(f"Files with hits: {totals['files_with_hits']}")
    lines.append(f"Total signal hits: {totals['signal_hit_count']}")
    lines.append(
        "Framework detection: "
        f"{framework_detection['candidate']} ({framework_detection['confidence']})"
    )
    lines.append(
        "Framework scores: "
        f"pytorch={framework_detection['scores']['pytorch']}, "
        f"tensorflow={framework_detection['scores']['tensorflow']}"
    )
    lines.append("")
    lines.append("Top lead files:")

    for idx, item in enumerate(files, start=1):
        lines.append(f"{idx}. {item['path']} (score={item['score']})")
        top_signals = item["signal_hits"][:3]
        for hit in top_signals:
            lines.append(
                f"   - {hit['signal_id']}[{hit['framework']}]: "
                f"count={hit['count']}, contribution={hit['contribution']}"
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
    file_hits: list[dict] = []
    signal_hit_count = 0
    signal_framework_scores = {"pytorch": 0.0, "tensorflow": 0.0}

    for path in iter_python_files(repo_path):
        scanned += 1
        item = scan_file(path, compiled_signals, args.max_occurrences)
        if item is None:
            continue

        item["path"] = str(Path(item["path"]).relative_to(repo_path))
        file_hits.append(item)
        signal_hit_count += sum(hit["count"] for hit in item["signal_hits"])
        signal_framework_scores["pytorch"] += item["framework_scores"].get("pytorch", 0.0)
        signal_framework_scores["tensorflow"] += item["framework_scores"].get("tensorflow", 0.0)

    artifact_framework_scores, artifact_evidence = detect_framework_artifacts(repo_path)
    framework_scores = {
        "pytorch": round(
            signal_framework_scores["pytorch"] + artifact_framework_scores["pytorch"], 2
        ),
        "tensorflow": round(
            signal_framework_scores["tensorflow"] + artifact_framework_scores["tensorflow"], 2
        ),
    }
    framework_candidate, framework_confidence = classify_framework(
        framework_scores["pytorch"], framework_scores["tensorflow"]
    )

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
        "framework_detection": {
            "candidate": framework_candidate,
            "confidence": framework_confidence,
            "scores": framework_scores,
            "components": {
                "signal_scores": {
                    "pytorch": round(signal_framework_scores["pytorch"], 2),
                    "tensorflow": round(signal_framework_scores["tensorflow"], 2),
                },
                "artifact_scores": artifact_framework_scores,
            },
            "evidence": artifact_evidence[:30],
        },
        "signals": [
            {
                "id": signal.id,
                "framework": signal.framework,
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
        build_summary(
            experiment_id=experiment_id,
            repo_path=repo_path,
            files=top_files,
            totals={
                "python_files_scanned": scanned,
                "files_with_hits": len(file_hits),
                "signal_hit_count": signal_hit_count,
            },
            framework_detection=lead_pack["framework_detection"],
        ),
        encoding="utf-8",
    )

    print(f"experiment_id={experiment_id}")
    print(f"lead_pack={lead_pack_path}")
    print(f"summary={summary_path}")
    print(f"python_files_scanned={scanned}")
    print(f"files_with_hits={len(file_hits)}")
    print(f"framework_candidate={framework_candidate}")
    print(f"framework_confidence={framework_confidence}")
    print(f"pytorch_score={framework_scores['pytorch']}")
    print(f"tensorflow_score={framework_scores['tensorflow']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
