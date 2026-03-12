#!/usr/bin/env python3
"""Runtime harness that expands guide-native validation to a few real samples."""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable, List, Optional

import numpy as np

from harness_lib.events import emit_event


def _normalize_subset_name(value: str) -> str:
    lowered = value.strip().lower()
    if lowered == "training":
        return "train"
    return lowered


def _normalize_shape(value: Any) -> List[int]:
    if value is None:
        return []
    if isinstance(value, np.ndarray):
        return [int(dim) for dim in value.shape]
    try:
        return [int(dim) for dim in np.asarray(value).shape]
    except Exception:
        return []


def _normalize_dtype(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, np.ndarray):
        return str(value.dtype)
    try:
        return str(np.asarray(value).dtype)
    except Exception:
        return type(value).__name__


def _is_supported_dtype(dtype: str) -> bool:
    if not dtype:
        return False
    try:
        kind = np.dtype(dtype).kind
    except Exception:
        return False
    return kind in {"b", "i", "u", "f", "c"}


def _is_finite(value: Any) -> Optional[bool]:
    try:
        array = np.asarray(value)
    except Exception:
        return None
    if array.dtype.kind not in {"b", "i", "u", "f", "c"}:
        return None
    try:
        return bool(np.all(np.isfinite(array)))
    except Exception:
        return None


def _fingerprint(value: Any) -> str:
    try:
        array = np.asarray(value)
        if array.dtype == object:
            encoded = json.dumps(value, default=str, sort_keys=True).encode("utf-8")
        else:
            encoded = array.tobytes()
            encoded += str(array.dtype).encode("utf-8")
            encoded += json.dumps(_normalize_shape(array), sort_keys=True).encode("utf-8")
    except Exception:
        encoded = json.dumps(value, default=str, sort_keys=True).encode("utf-8")
    return hashlib.sha1(encoded).hexdigest()


@dataclass
class ResultSummary:
    status: str
    shape: List[int]
    dtype: str
    expected_shape: List[int]
    expected_dtype: str
    finite: Optional[bool]
    fingerprint: str
    message: str


def _normalize_declared_dtype(value: Any) -> str:
    if value is None:
        return ""
    try:
        return str(np.dtype(value))
    except Exception:
        pass

    name = getattr(value, "name", None)
    if isinstance(name, str):
        try:
            return str(np.dtype(name))
        except Exception:
            return name
    return ""


def _expected_shape_for_handler(handler: Any) -> List[int]:
    candidates = [
        getattr(handler, "shape", None),
        getattr(handler, "expected_shape", None),
    ]
    contract = getattr(handler, "contract", None)
    if contract is not None:
        candidates.extend(
            [
                getattr(contract, "shape", None),
                getattr(contract, "expected_shape", None),
            ]
        )

    for candidate in candidates:
        shape = _normalize_shape(candidate)
        if shape:
            return shape
    return []


def _expected_dtype_for_handler(handler: Any) -> str:
    candidates = [
        getattr(handler, "dtype", None),
        getattr(handler, "expected_dtype", None),
        getattr(handler, "data_type", None),
        getattr(handler, "output_dtype", None),
    ]
    contract = getattr(handler, "contract", None)
    if contract is not None:
        candidates.extend(
            [
                getattr(contract, "dtype", None),
                getattr(contract, "expected_dtype", None),
                getattr(contract, "data_type", None),
                getattr(contract, "output_dtype", None),
            ]
        )

    for candidate in candidates:
        dtype = _normalize_declared_dtype(candidate)
        if dtype:
            return dtype
    return ""


def _shape_matches(actual: List[int], expected: List[int]) -> bool:
    if not expected:
        return True
    if len(actual) != len(expected):
        return False
    for actual_dim, expected_dim in zip(actual, expected):
        if expected_dim < 0:
            continue
        if actual_dim != expected_dim:
            return False
    return True


def _dtype_matches(actual: str, expected: str) -> bool:
    if not actual or not expected:
        return True
    try:
        return np.dtype(actual) == np.dtype(expected)
    except Exception:
        return actual == expected


def _summarize_result(value: Any, expected_shape: List[int], expected_dtype: str) -> ResultSummary:
    shape = _normalize_shape(value)
    dtype = _normalize_dtype(value)
    finite = _is_finite(value)
    fingerprint = _fingerprint(value)

    status = "ok"
    message = ""
    if not _is_supported_dtype(dtype):
        status = "dtype_invalid"
        message = f"unsupported dtype {dtype}"
    elif not _dtype_matches(dtype, expected_dtype):
        status = "dtype_invalid"
        message = f"expected dtype {expected_dtype}, got {dtype}"
    elif finite is False:
        status = "non_finite"
        message = "non-finite values detected"
    elif not _shape_matches(shape, expected_shape):
        status = "shape_invalid"
        message = f"expected shape {expected_shape}, got {shape}"

    return ResultSummary(
        status=status,
        shape=shape,
        dtype=dtype,
        expected_shape=expected_shape,
        expected_dtype=expected_dtype,
        finite=finite,
        fingerprint=fingerprint,
        message=message,
    )


class HarnessRuntime:
    def __init__(self, repo_root: str, entry_file: str, sample_budget: int):
        self.repo_root = Path(repo_root).resolve()
        self.entry_file = entry_file
        self.sample_budget = max(1, int(sample_budget))

    def run(self) -> None:
        emit_event(event="run_started", status="ok", message="runtime harness started")

        try:
            from code_loader.contract.enums import DataStateEnum
            from code_loader.inner_leap_binder.leapbinder import global_leap_binder
            from code_loader.leaploader import LeapLoader
        except Exception as exc:
            emit_event(event="runtime_failed", status="failed", message=f"code-loader import failed: {exc}")
            return

        try:
            loader = LeapLoader(str(self.repo_root), self.entry_file)
            loader.exec_script()
        except Exception as exc:
            emit_event(event="runtime_failed", status="failed", message=f"integration import failed: {exc}")
            return

        try:
            preprocess_result = global_leap_binder.get_preprocess_result()
            emit_event(event="preprocess", status="ok", message="preprocess succeeded")
        except Exception as exc:
            emit_event(event="preprocess", status="failed", message=str(exc))
            return

        inputs = list(global_leap_binder.setup_container.inputs)
        ground_truths = list(global_leap_binder.setup_container.ground_truths)

        for handler in inputs:
            emit_event(
                event="handler_inventory",
                status="ok",
                symbol=str(handler.name),
                name=str(handler.name),
                handler_kind="input",
            )
        for handler in ground_truths:
            emit_event(
                event="handler_inventory",
                status="ok",
                symbol=str(handler.name),
                name=str(handler.name),
                handler_kind="ground_truth",
            )

        mandatory_subsets_seen = set()
        for state, preprocess_response in preprocess_result.items():
            subset = _normalize_subset_name(state.name if hasattr(state, "name") else str(state))
            sample_ids = list(getattr(preprocess_response, "sample_ids", []) or [])
            emit_event(event="subset_count", status="ok", subset=subset, count=len(sample_ids))

            if subset not in {"train", "validation"}:
                continue
            mandatory_subsets_seen.add(subset)

            selected_ids = sample_ids[: self.sample_budget]
            for sample_offset, sample_id in enumerate(selected_ids):
                sample_id_text = str(sample_id)
                emit_event(
                    event="sample_selected",
                    status="ok",
                    subset=subset,
                    sample_id=sample_id_text,
                    sample_offset=sample_offset,
                )
                self._run_handlers(
                    preprocess_response=preprocess_response,
                    subset=subset,
                    sample_id=sample_id,
                    sample_id_text=sample_id_text,
                    sample_offset=sample_offset,
                    handlers=inputs,
                    handler_kind="input",
                )
                if subset != "unlabeled":
                    self._run_handlers(
                        preprocess_response=preprocess_response,
                        subset=subset,
                        sample_id=sample_id,
                        sample_id_text=sample_id_text,
                        sample_offset=sample_offset,
                        handlers=ground_truths,
                        handler_kind="ground_truth",
                    )

        for required_subset in ("train", "validation"):
            if required_subset not in mandatory_subsets_seen:
                emit_event(
                    event="subset_missing",
                    status="failed",
                    subset=required_subset,
                    message=f"{required_subset} subset is missing from preprocess output",
                )

        emit_event(event="summary", status="ok", message="runtime harness finished")

    def _run_handlers(
        self,
        preprocess_response: Any,
        subset: str,
        sample_id: Any,
        sample_id_text: str,
        sample_offset: int,
        handlers: Iterable[Any],
        handler_kind: str,
    ) -> None:
        for handler in handlers:
            symbol = str(getattr(handler, "name", ""))
            expected_shape = _expected_shape_for_handler(handler)
            expected_dtype = _expected_dtype_for_handler(handler)
            try:
                raw_result = handler.function(sample_id, preprocess_response)
                summary = _summarize_result(
                    raw_result,
                    expected_shape=expected_shape,
                    expected_dtype=expected_dtype,
                )
                emit_event(
                    event="handler_result",
                    status=summary.status,
                    message=summary.message,
                    subset=subset,
                    sample_id=sample_id_text,
                    sample_offset=sample_offset,
                    symbol=symbol,
                    name=symbol,
                    handler_kind=handler_kind,
                    shape=summary.shape,
                    dtype=summary.dtype,
                    expected_shape=summary.expected_shape,
                    expected_dtype=summary.expected_dtype,
                    finite=summary.finite,
                    fingerprint=summary.fingerprint,
                )
            except Exception as exc:
                emit_event(
                    event="handler_result",
                    status="failed",
                    message=str(exc),
                    subset=subset,
                    sample_id=sample_id_text,
                    sample_offset=sample_offset,
                    symbol=symbol,
                    name=symbol,
                    handler_kind=handler_kind,
                    expected_shape=expected_shape,
                    expected_dtype=expected_dtype,
                )
