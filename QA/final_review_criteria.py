from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Iterable, Mapping

SCHEMA_VERSION = 1
AGGREGATE_PASS_THRESHOLD = 0.70
AGGREGATE_STRONG_THRESHOLD = 0.85
AGGREGATE_BORDERLINE_THRESHOLD = 0.55
CRITERION_BLOCKING_SCORE = 0.35
LOW_WEIGHT_SUBJECTIVE_CRITERION = "overall_subjective_judgment"

CONFIDENCE_VALUES = {
    "low": 1,
    "medium": 2,
    "high": 3,
}

CRITERION_DEFINITIONS: dict[str, dict[str, Any]] = {
    "checkpoint_appropriateness": {
        "title": "Checkpoint Appropriateness",
        "weight": 0.10,
        "minimum_score": CRITERION_BLOCKING_SCORE,
        "focus": "Judge whether the generated integration is appropriate for the active guide checkpoint rather than over- or under-shooting the scoped step.",
        "guidance": "Treat missing downstream surfaces as acceptable when the checkpoint does not require them yet.",
    },
    "encoder_inventory_alignment": {
        "title": "Encoder Inventory Alignment",
        "weight": 0.18,
        "minimum_score": CRITERION_BLOCKING_SCORE,
        "focus": "Compare registered preprocess, input encoder, and GT encoder surfaces against the post fixture inventory.",
        "guidance": "Focus on missing, extra, or mislabeled encoder registrations and whether those differences matter at the current checkpoint.",
    },
    "prediction_surface_alignment": {
        "title": "Prediction Surface Alignment",
        "weight": 0.18,
        "minimum_score": CRITERION_BLOCKING_SCORE,
        "focus": "Compare prediction handlers, model outputs, and integration-test consumption of predictions against the post fixture.",
        "guidance": "Focus on behaviorally meaningful divergence in prediction wiring rather than cosmetic code differences.",
    },
    "preprocess_sample_model_alignment": {
        "title": "Preprocess / Sample / Model Alignment",
        "weight": 0.16,
        "minimum_score": CRITERION_BLOCKING_SCORE,
        "focus": "Check whether preprocess usage, sample iteration, model loading, and integration-test call shape align with the post fixture for this checkpoint.",
        "guidance": "Focus on whether samples flow through preprocess and model wiring in a compatible way.",
    },
    "input_tensor_contract_alignment": {
        "title": "Input Tensor Contract Alignment",
        "weight": 0.18,
        "minimum_score": CRITERION_BLOCKING_SCORE,
        "focus": "Compare tensor names, shapes, channel dimensions, and input handoff semantics for input encoders.",
        "guidance": "Focus on mismatches that would change runtime behavior or model compatibility.",
    },
    "gt_contract_alignment": {
        "title": "GT Contract Alignment",
        "weight": 0.18,
        "minimum_score": CRITERION_BLOCKING_SCORE,
        "focus": "Compare GT encoder presence, labels, and call usage against the post fixture.",
        "guidance": "Focus on ground-truth contract divergence that would change training or evaluation semantics.",
    },
    LOW_WEIGHT_SUBJECTIVE_CRITERION: {
        "title": "Overall Subjective Judgment",
        "weight": 0.02,
        "minimum_score": 0.0,
        "focus": "Give a low-weight holistic read on whether the generated integration looks trustworthy for the checkpoint after considering the extracted evidence.",
        "guidance": "This criterion is intentionally low weight and must not override clear evidence from the narrower rubric criteria.",
    },
}

DEFAULT_CRITERION_ORDER = (
    "checkpoint_appropriateness",
    "encoder_inventory_alignment",
    "prediction_surface_alignment",
    "preprocess_sample_model_alignment",
    "input_tensor_contract_alignment",
    "gt_contract_alignment",
)


def criteria_for_step(guide_step: str, primary_criteria: Iterable[Any]) -> list[dict[str, Any]]:
    _ = guide_step
    primary_ids = [
        str(item).strip()
        for item in primary_criteria
        if str(item).strip() and str(item).strip() in CRITERION_DEFINITIONS
    ]
    ordered_ids: list[str] = []
    for criterion_id in primary_ids:
        if criterion_id == LOW_WEIGHT_SUBJECTIVE_CRITERION or criterion_id in ordered_ids:
            continue
        ordered_ids.append(criterion_id)
    for criterion_id in DEFAULT_CRITERION_ORDER:
        if criterion_id not in ordered_ids:
            ordered_ids.append(criterion_id)
    ordered_ids.append(LOW_WEIGHT_SUBJECTIVE_CRITERION)

    primary_set = set(primary_ids or DEFAULT_CRITERION_ORDER)
    return [criterion_definition(criterion_id, is_primary=criterion_id in primary_set) for criterion_id in ordered_ids]


def criterion_definition(criterion_id: str, *, is_primary: bool) -> dict[str, Any]:
    payload = dict(CRITERION_DEFINITIONS[criterion_id])
    payload["id"] = criterion_id
    payload["primary"] = is_primary
    return payload


def normalize_criterion_result(*, criterion: Mapping[str, Any], payload: Mapping[str, Any]) -> dict[str, Any]:
    score = clamp_score(payload.get("score", 0.0))
    normalized = {
        "id": str(criterion.get("id", "")).strip(),
        "title": str(criterion.get("title", "")).strip(),
        "weight": float(criterion.get("weight", 0.0) or 0.0),
        "primary": bool(criterion.get("primary", False)),
        "minimum_score": float(criterion.get("minimum_score", 0.0) or 0.0),
        "score": score,
        "score_band": criterion_score_band(score),
        "confidence": normalize_confidence(payload.get("confidence", "")),
        "summary": str(payload.get("summary", "")).strip(),
        "evidence": clean_string_list(payload.get("evidence", [])),
        "concerns": clean_string_list(payload.get("concerns", [])),
        "status": "scored",
    }
    artifact_paths = payload.get("artifact_paths")
    if isinstance(artifact_paths, dict):
        normalized["artifact_paths"] = {str(key): str(value) for key, value in artifact_paths.items()}
    return normalized


def criterion_error_result(*, criterion: Mapping[str, Any], message: str) -> dict[str, Any]:
    return {
        "id": str(criterion.get("id", "")).strip(),
        "title": str(criterion.get("title", "")).strip(),
        "weight": float(criterion.get("weight", 0.0) or 0.0),
        "primary": bool(criterion.get("primary", False)),
        "minimum_score": float(criterion.get("minimum_score", 0.0) or 0.0),
        "score": 0.0,
        "score_band": "error",
        "confidence": "low",
        "summary": message.strip(),
        "evidence": [],
        "concerns": [message.strip()],
        "status": "error",
    }


def aggregate_review(*, criteria: Iterable[Mapping[str, Any]], results: Iterable[Mapping[str, Any]]) -> dict[str, Any]:
    criteria_list = [dict(criterion) for criterion in criteria]
    result_by_id = {
        str(result.get("criterion_id") or result.get("id") or "").strip(): result
        for result in results
        if str(result.get("criterion_id") or result.get("id") or "").strip()
    }

    normalized_results: list[dict[str, Any]] = []
    for criterion in criteria_list:
        criterion_id = str(criterion.get("id", "")).strip()
        payload = result_by_id.get(criterion_id)
        if payload is None:
            normalized_results.append(
                criterion_error_result(
                    criterion=criterion,
                    message=f"No result was recorded for review criterion `{criterion_id}`.",
                )
            )
            continue
        normalized_results.append(normalize_criterion_result(criterion=criterion, payload=payload))

    total_weight = sum(float(criterion.get("weight", 0.0) or 0.0) for criterion in criteria_list) or 1.0
    aggregate_score = round(
        sum(float(result.get("score", 0.0) or 0.0) * float(result.get("weight", 0.0) or 0.0) for result in normalized_results)
        / total_weight,
        3,
    )
    criteria_errors = [str(result["id"]) for result in normalized_results if result.get("status") == "error"]
    blocking_criteria = [
        str(result["id"])
        for result in normalized_results
        if result.get("primary")
        and result.get("status") != "error"
        and float(result.get("score", 0.0) or 0.0) < float(result.get("minimum_score", 0.0) or 0.0)
    ]

    if criteria_errors:
        aggregate_band = "error"
        status = "error"
    else:
        aggregate_band = aggregate_score_band(aggregate_score)
        status = "pass" if aggregate_score >= AGGREGATE_PASS_THRESHOLD and not blocking_criteria else "fail"

    issues = aggregate_issues(normalized_results)
    if status == "error":
        verdict = "Weighted final review could not complete because one or more criterion runs failed."
        functional_equivalence = "The QA loop could not produce a complete criterion-by-criterion equivalence review against the post fixture."
        quality_assessment = "The generated integration should not be accepted until the failed criterion runs are rerun successfully."
    elif status == "pass":
        verdict = f"Weighted final review passed at {aggregate_score:.2f} with band `{aggregate_band}`."
        functional_equivalence = "The primary rubric criteria support functional equivalence for the scoped checkpoint."
        quality_assessment = "The generated integration looks acceptable for the active checkpoint under the controller-owned rubric."
    elif blocking_criteria:
        verdict = (
            f"Weighted final review failed controller thresholds even though the aggregate score was {aggregate_score:.2f}; "
            f"blocking criteria: {', '.join(blocking_criteria)}."
        )
        functional_equivalence = "One or more primary rubric criteria show behaviorally significant divergence from the post fixture."
        quality_assessment = "The generated integration should not be accepted until the blocking rubric criteria are repaired."
    else:
        verdict = (
            f"Weighted final review failed because the aggregate score {aggregate_score:.2f} stayed below the pass threshold "
            f"{AGGREGATE_PASS_THRESHOLD:.2f}."
        )
        functional_equivalence = "The narrow rubric criteria do not support equivalence strongly enough for the scoped checkpoint."
        quality_assessment = "The generated integration remains too inconsistent across the rubric to pass final review."

    return {
        "schema_version": SCHEMA_VERSION,
        "review_mode": "criteria_scorecard",
        "status": status,
        "verdict": verdict,
        "functional_equivalence": functional_equivalence,
        "quality_assessment": quality_assessment,
        "issues": issues,
        "confidence": aggregate_confidence(normalized_results),
        "aggregate_score": aggregate_score,
        "aggregate_band": aggregate_band,
        "passing_score_threshold": AGGREGATE_PASS_THRESHOLD,
        "blocking_score_threshold": CRITERION_BLOCKING_SCORE,
        "blocking_criteria": blocking_criteria,
        "criteria_errors": criteria_errors,
        "criteria": normalized_results,
        "criterion_order": [str(criterion["id"]) for criterion in criteria_list],
    }


def write_final_review_scorecard(scorecard: Mapping[str, Any], path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(scorecard, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def clean_string_list(values: Iterable[Any]) -> list[str]:
    cleaned: list[str] = []
    for value in values:
        item = str(value).strip()
        if item:
            cleaned.append(item)
    return cleaned


def normalize_confidence(value: Any) -> str:
    confidence = str(value).strip().lower()
    if confidence in CONFIDENCE_VALUES:
        return confidence
    return "low"


def clamp_score(value: Any) -> float:
    try:
        score = float(value)
    except (TypeError, ValueError):
        return 0.0
    return round(max(0.0, min(1.0, score)), 3)


def criterion_score_band(score: float) -> str:
    if score >= 0.85:
        return "aligned"
    if score >= 0.65:
        return "mostly_aligned"
    if score >= CRITERION_BLOCKING_SCORE:
        return "mixed"
    return "misaligned"


def aggregate_score_band(score: float) -> str:
    if score >= AGGREGATE_STRONG_THRESHOLD:
        return "strong_accept"
    if score >= AGGREGATE_PASS_THRESHOLD:
        return "accept"
    if score >= AGGREGATE_BORDERLINE_THRESHOLD:
        return "borderline"
    return "reject"


def aggregate_confidence(results: Iterable[Mapping[str, Any]]) -> str:
    results_list = list(results)
    if any(result.get("status") == "error" for result in results_list):
        return "low"
    total_weight = sum(float(result.get("weight", 0.0) or 0.0) for result in results_list) or 1.0
    weighted_value = sum(
        CONFIDENCE_VALUES.get(normalize_confidence(result.get("confidence", "")), 1)
        * float(result.get("weight", 0.0) or 0.0)
        for result in results_list
    ) / total_weight
    if weighted_value >= 2.5:
        return "high"
    if weighted_value >= 1.75:
        return "medium"
    return "low"


def aggregate_issues(results: Iterable[Mapping[str, Any]]) -> list[str]:
    ordered = sorted(
        list(results),
        key=lambda result: (
            0 if result.get("status") == "error" else 1,
            float(result.get("score", 0.0) or 0.0),
            str(result.get("id", "")),
        ),
    )
    issues: list[str] = []
    seen: set[str] = set()
    for result in ordered:
        candidates = clean_string_list(result.get("concerns", []))
        if not candidates and result.get("status") == "error":
            candidates = clean_string_list([result.get("summary", "")])
        for candidate in candidates:
            if candidate in seen:
                continue
            seen.add(candidate)
            issues.append(candidate)
            if len(issues) >= 5:
                return issues
    return issues
