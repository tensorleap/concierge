#!/usr/bin/env python3
"""NDJSON event helpers for the Concierge runtime harness."""

from __future__ import annotations

import json
from dataclasses import asdict, dataclass, field
from typing import Any, List, Optional


@dataclass
class HarnessEvent:
    event: str
    status: Optional[str] = None
    message: Optional[str] = None
    name: Optional[str] = None
    symbol: Optional[str] = None
    handler_kind: Optional[str] = None
    subset: Optional[str] = None
    sample_id: Optional[str] = None
    sample_offset: Optional[int] = None
    fingerprint: Optional[str] = None
    count: Optional[int] = None
    shape: List[int] = field(default_factory=list)
    dtype: Optional[str] = None
    expected_shape: List[int] = field(default_factory=list)
    expected_dtype: Optional[str] = None
    finite: Optional[bool] = None

    def to_json(self) -> str:
        payload = {
            key: value
            for key, value in asdict(self).items()
            if value not in (None, [], "")
        }
        return json.dumps(payload, sort_keys=True)


def emit_event(**kwargs: Any) -> None:
    print(HarnessEvent(**kwargs).to_json(), flush=True)
