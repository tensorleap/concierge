#!/usr/bin/env python3
"""Temporary research script: compare agent findings against fixture post oracle.

Oracle extraction is regex-based from leap_binder decorators.
"""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path


INPUT_DECORATOR_RE = re.compile(r"@tensorleap_input_encoder\(\s*['\"]([^'\"]+)['\"]")
GT_DECORATOR_RE = re.compile(r"@tensorleap_gt_encoder\(\s*['\"]([^'\"]+)['\"]")


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="Compare findings to fixture oracle")
    p.add_argument("--findings", required=True, help="Agent findings JSON path")
    p.add_argument("--fixture-post", required=True, help="Fixture post root path")
    p.add_argument("--output", required=True, help="Output JSON report path")
    return p.parse_args()


def parse_oracle(fixture_post_root: Path) -> tuple[set[str], set[str]]:
    binder = fixture_post_root / "leap_binder.py"
    if not binder.exists():
        raise SystemExit(f"oracle binder missing: {binder}")
    text = binder.read_text(encoding="utf-8", errors="ignore")
    inputs = set(INPUT_DECORATOR_RE.findall(text))
    gts = set(GT_DECORATOR_RE.findall(text))
    return inputs, gts


def count_hint(items: list[dict], key: str) -> int:
    n = 0
    for item in items:
        value = item.get(key)
        if isinstance(value, str) and value.strip():
            n += 1
    return n


def main() -> int:
    args = parse_args()
    findings_path = Path(args.findings).resolve()
    fixture_post = Path(args.fixture_post).resolve()
    output_path = Path(args.output).resolve()

    findings = json.loads(findings_path.read_text(encoding="utf-8"))
    oracle_inputs, oracle_gts = parse_oracle(fixture_post)

    input_items = [x for x in findings.get("inputs", []) if isinstance(x, dict)]
    gt_items = [x for x in findings.get("ground_truths", []) if isinstance(x, dict)]
    found_inputs = {item.get("name", "") for item in input_items if item.get("name")}
    found_gts = {item.get("name", "") for item in gt_items if item.get("name")}

    oracle_input_count = len(oracle_inputs)
    oracle_gt_count = len(oracle_gts)
    agent_input_count = len(found_inputs)
    agent_gt_count = len(found_gts)

    exact_input_match_count = len(oracle_inputs & found_inputs)
    exact_gt_match_count = len(oracle_gts & found_gts)
    exact_input_match_rate = (exact_input_match_count / oracle_input_count) if oracle_input_count else 1.0
    exact_gt_match_rate = (exact_gt_match_count / oracle_gt_count) if oracle_gt_count else 1.0

    input_shape_hint_count = count_hint(input_items, "shape_hint")
    input_dtype_hint_count = count_hint(input_items, "dtype_hint")
    gt_shape_hint_count = count_hint(gt_items, "shape_hint")
    gt_dtype_hint_count = count_hint(gt_items, "dtype_hint")

    semantic_inputs_present = agent_input_count > 0 if oracle_input_count > 0 else True
    semantic_gts_present = agent_gt_count > 0 if oracle_gt_count > 0 else True
    semantic_count_match = (agent_input_count == oracle_input_count) and (agent_gt_count == oracle_gt_count)
    semantic_verdict = "pass" if semantic_inputs_present and semantic_gts_present else "needs_review"

    report = {
        "oracle": {
            "inputs": sorted(oracle_inputs),
            "ground_truths": sorted(oracle_gts),
        },
        "agent": {
            "inputs": sorted(found_inputs),
            "ground_truths": sorted(found_gts),
        },
        "comparison": {
            "exact_names": {
                "inputs": {
                    "matched": sorted(oracle_inputs & found_inputs),
                    "missing": sorted(oracle_inputs - found_inputs),
                    "extra": sorted(found_inputs - oracle_inputs),
                    "match_rate": round(exact_input_match_rate, 3),
                },
                "ground_truths": {
                    "matched": sorted(oracle_gts & found_gts),
                    "missing": sorted(oracle_gts - found_gts),
                    "extra": sorted(found_gts - oracle_gts),
                    "match_rate": round(exact_gt_match_rate, 3),
                },
            },
            "semantic": {
                "inputs_present": semantic_inputs_present,
                "ground_truths_present": semantic_gts_present,
                "count_match": semantic_count_match,
                "input_counts": {
                    "oracle": oracle_input_count,
                    "agent": agent_input_count,
                },
                "ground_truth_counts": {
                    "oracle": oracle_gt_count,
                    "agent": agent_gt_count,
                },
                "hint_coverage": {
                    "input_shape_hints": input_shape_hint_count,
                    "input_dtype_hints": input_dtype_hint_count,
                    "ground_truth_shape_hints": gt_shape_hint_count,
                    "ground_truth_dtype_hints": gt_dtype_hint_count,
                },
                "verdict": semantic_verdict,
            },
        },
    }

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")

    print(f"output={output_path}")
    print(f"semantic_verdict={semantic_verdict}")
    print(f"oracle_inputs={oracle_input_count} agent_inputs={agent_input_count}")
    print(f"oracle_gts={oracle_gt_count} agent_gts={agent_gt_count}")
    print(f"exact_input_match_rate={exact_input_match_rate:.3f}")
    print(f"exact_gt_match_rate={exact_gt_match_rate:.3f}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
