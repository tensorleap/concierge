#!/usr/bin/env python3
"""Temporary research utility to normalize Claude free-form output to findings schema."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from typing import Any


FENCED_JSON_RE = re.compile(r"```json\s*(\{.*?\})\s*```", re.DOTALL)
CALL_SITE_RE = re.compile(r"([^:]+):(\d+)(?:-(\d+))?")


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="Normalize Claude output into findings schema")
    p.add_argument("--input", required=True, help="Claude final text file")
    p.add_argument("--experiment-id", required=True)
    p.add_argument("--repo", required=True)
    p.add_argument("--output", required=True)
    return p.parse_args()


def load_json_from_text(text: str) -> dict[str, Any]:
    text = text.strip()

    # Case 1: whole text is JSON.
    try:
        obj = json.loads(text)
        if isinstance(obj, dict):
            return obj
    except json.JSONDecodeError:
        pass

    # Case 2: fenced JSON block.
    m = FENCED_JSON_RE.search(text)
    if not m:
        raise SystemExit("Could not find JSON payload in Claude output")

    payload = m.group(1)
    obj = json.loads(payload)
    if not isinstance(obj, dict):
        raise SystemExit("Extracted JSON payload is not an object")
    return obj


def parse_call_site(call_site: str) -> tuple[str, int]:
    m = CALL_SITE_RE.search(call_site)
    if not m:
        return "<unknown>", 1
    file_path = m.group(1)
    line = int(m.group(2))
    return file_path, line


def normalize_candidate(item: dict[str, Any]) -> dict[str, Any]:
    name = item.get("name") or item.get("source_function") or "unknown"
    confidence = item.get("confidence", "medium")
    if confidence not in {"high", "medium", "low"}:
        confidence = "medium"

    evidence: list[dict[str, Any]] = []

    call_site = item.get("call_site")
    snippet = item.get("call_site_snippet") or item.get("description") or ""
    if isinstance(call_site, str):
        file_path, line = parse_call_site(call_site)
        evidence.append(
            {
                "file": file_path,
                "line": line,
                "snippet": str(snippet),
            }
        )

    raw_evidence = item.get("evidence")
    if isinstance(raw_evidence, list):
        for ev in raw_evidence:
            if not isinstance(ev, dict):
                continue
            file_path = ev.get("file")
            if not isinstance(file_path, str) or not file_path.strip():
                continue
            line_val = ev.get("line", ev.get("lines", 1))
            if isinstance(line_val, list) and line_val:
                first = line_val[0]
                line_val = first if isinstance(first, int) else 1
            if not isinstance(line_val, int) or line_val < 1:
                line_val = 1
            snippet_val = ev.get("snippet") or ev.get("description") or ""
            evidence.append(
                {
                    "file": file_path,
                    "line": line_val,
                    "snippet": str(snippet_val),
                }
            )

    if not evidence:
        source_module = str(item.get("source_module") or "<unknown>")
        evidence.append(
            {
                "file": source_module,
                "line": 1,
                "snippet": str(item.get("description") or "No explicit snippet"),
            }
        )

    out: dict[str, Any] = {
        "name": str(name),
        "confidence": confidence,
        "evidence": evidence,
    }

    if item.get("description"):
        out["semantic_hint"] = str(item["description"])
    shape_hint = item.get("shape")
    if shape_hint is None:
        shape_hint = item.get("shape_hint")
    if shape_hint is not None:
        out["shape_hint"] = str(shape_hint)

    dtype_hint = item.get("dtype")
    if dtype_hint is None:
        dtype_hint = item.get("dtype_hint")
    if dtype_hint is not None:
        out["dtype_hint"] = str(dtype_hint)

    return out


def normalize_mapping(mapping: dict[str, Any]) -> dict[str, Any]:
    role = str(mapping.get("role") or "").strip().lower()
    encoder_type = "ground_truth" if role in {"ground_truth", "gt"} else "input"

    confidence = str(mapping.get("confidence") or "medium").lower()
    if confidence not in {"high", "medium", "low"}:
        confidence = "medium"

    name = str(
        mapping.get("leap_binder_function")
        or mapping.get("encoder_name")
        or mapping.get("name")
        or "unknown"
    )
    source = str(mapping.get("maps_to_candidate") or mapping.get("maps_to") or name)

    notes_parts = []
    if mapping.get("signature"):
        notes_parts.append(f"signature={mapping['signature']}")
    if mapping.get("output_shape"):
        notes_parts.append(f"output_shape={mapping['output_shape']}")
    if mapping.get("coordinate_format"):
        notes_parts.append(f"coordinate_format={mapping['coordinate_format']}")
    if mapping.get("notes"):
        notes_parts.append(str(mapping["notes"]))
    if mapping.get("rationale"):
        notes_parts.append(str(mapping["rationale"]))
    if mapping.get("description"):
        notes_parts.append(str(mapping["description"]))

    out = {
        "encoder_type": encoder_type,
        "name": name,
        "source_candidate": source,
        "confidence": confidence,
    }
    if notes_parts:
        out["notes"] = "; ".join(notes_parts)
    return out


def normalize_unknowns(raw: Any) -> list[str]:
    if not isinstance(raw, list):
        return []
    out: list[str] = []
    for item in raw:
        if isinstance(item, str):
            out.append(item)
            continue
        if isinstance(item, dict):
            desc = item.get("description")
            if isinstance(desc, str) and desc.strip():
                out.append(desc.strip())
    return out


def normalize_comments(raw: Any) -> str:
    if isinstance(raw, str):
        return raw.strip()
    if isinstance(raw, list):
        parts = [str(x).strip() for x in raw if str(x).strip()]
        return "\n".join(parts).strip()
    if isinstance(raw, dict):
        return json.dumps(raw, ensure_ascii=False)
    return ""


def main() -> int:
    args = parse_args()

    src_text = Path(args.input).read_text(encoding="utf-8", errors="ignore")
    raw = load_json_from_text(src_text)

    # Pass-through if already in expected schema.
    if {"schema_version", "method_version", "experiment_id", "repo", "inputs", "ground_truths", "proposed_mapping", "unknowns"}.issubset(raw.keys()):
        normalized = raw
    else:
        if isinstance(raw.get("inputs"), list) and isinstance(raw.get("ground_truths"), list):
            raw_inputs = raw.get("inputs", [])
            raw_gts = raw.get("ground_truths", [])
            if isinstance(raw.get("proposed_mapping"), list):
                raw_mapping = raw.get("proposed_mapping", [])
            elif isinstance(raw.get("encoder_mapping"), list):
                raw_mapping = raw.get("encoder_mapping", [])
            else:
                raw_mapping = []
        elif isinstance(raw.get("model_inputs"), list) and isinstance(raw.get("ground_truths"), list):
            raw_inputs = raw.get("model_inputs", [])
            raw_gts = raw.get("ground_truths", [])
            if isinstance(raw.get("proposed_mapping"), list):
                raw_mapping = raw.get("proposed_mapping", [])
            elif isinstance(raw.get("proposed_encoder_mapping"), list):
                raw_mapping = raw.get("proposed_encoder_mapping", [])
            elif isinstance(raw.get("encoder_mapping"), list):
                raw_mapping = raw.get("encoder_mapping", [])
            else:
                raw_mapping = []
        elif isinstance(raw.get("candidate_inputs"), list) and isinstance(raw.get("candidate_ground_truths"), list):
            raw_inputs = raw.get("candidate_inputs", [])
            raw_gts = raw.get("candidate_ground_truths", [])
            if isinstance(raw.get("proposed_mapping"), list):
                raw_mapping = raw.get("proposed_mapping", [])
            elif isinstance(raw.get("proposed_encoder_mapping"), dict):
                proposed = raw.get("proposed_encoder_mapping", {})
                raw_mapping = []
                if isinstance(proposed.get("input_encoder"), dict):
                    item = dict(proposed["input_encoder"])
                    item.setdefault("role", "input")
                    raw_mapping.append(item)
                if isinstance(proposed.get("ground_truth_encoder"), dict):
                    item = dict(proposed["ground_truth_encoder"])
                    item.setdefault("role", "ground_truth")
                    raw_mapping.append(item)
            elif isinstance(raw.get("encoder_mapping"), list):
                raw_mapping = raw.get("encoder_mapping", [])
            else:
                raw_mapping = []
        else:
            candidates = raw.get("candidates", {}) if isinstance(raw.get("candidates"), dict) else {}
            raw_inputs = candidates.get("inputs", []) if isinstance(candidates.get("inputs"), list) else []
            raw_gts = candidates.get("ground_truths", []) if isinstance(candidates.get("ground_truths"), list) else []
            if isinstance(candidates.get("encoder_mapping"), list):
                raw_mapping = candidates.get("encoder_mapping", [])
            elif isinstance(raw.get("encoder_mapping"), list):
                raw_mapping = raw.get("encoder_mapping", [])
            else:
                raw_mapping = []

        normalized = {
            "schema_version": "1.0.0",
            "method_version": "pytorch-agent-findings-v1",
            "experiment_id": args.experiment_id,
            "repo": {"path": args.repo},
            "inputs": [normalize_candidate(x) for x in raw_inputs if isinstance(x, dict)],
            "ground_truths": [normalize_candidate(x) for x in raw_gts if isinstance(x, dict)],
            "proposed_mapping": [normalize_mapping(x) for x in raw_mapping if isinstance(x, dict)],
            "unknowns": normalize_unknowns(raw.get("unknowns", [])),
        }

        comments = normalize_comments(raw.get("comments"))
        if comments:
            normalized["comments"] = comments

    out_path = Path(args.output)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(normalized, indent=2) + "\n", encoding="utf-8")

    print(f"output={out_path}")
    print(f"inputs={len(normalized.get('inputs', []))}")
    print(f"ground_truths={len(normalized.get('ground_truths', []))}")
    print(f"proposed_mapping={len(normalized.get('proposed_mapping', []))}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
