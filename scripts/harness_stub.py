#!/usr/bin/env python3
"""Concierge harness stub: emits deterministic NDJSON events for validator tests."""

import argparse
import json
import os
import time


SUCCESS_EVENTS = [
    {"event": "subset_count", "subset": "train", "count": 10},
    {"event": "subset_count", "subset": "validation", "count": 5},
    {"event": "input_fingerprint", "name": "input", "fingerprint": "a"},
    {"event": "input_fingerprint", "name": "input", "fingerprint": "b"},
    {"event": "label_fingerprint", "name": "label", "fingerprint": "1"},
    {"event": "label_fingerprint", "name": "label", "fingerprint": "2"},
]


def main() -> int:
    parser = argparse.ArgumentParser(description="Concierge harness stub")
    parser.add_argument("--repo-root", default=".")
    parser.parse_args()

    mode = os.environ.get("CONCIERGE_HARNESS_STUB_MODE", "success").strip().lower()

    if mode == "sleep":
        seconds = float(os.environ.get("CONCIERGE_HARNESS_STUB_SLEEP", "0"))
        time.sleep(seconds)
        return 0

    if mode == "preprocess_fail":
        events = [{"event": "preprocess", "status": "failed", "message": "preprocess failed"}]
    elif mode == "coverage_incomplete":
        events = [{"event": "encoder_coverage", "status": "incomplete", "message": "missing encoders"}]
    elif mode == "validation_fail":
        events = [{"event": "validation", "status": "failed", "message": "validation failed"}]
    elif mode == "constant_inputs":
        events = [
            {"event": "input_fingerprint", "name": "input", "fingerprint": "same"},
            {"event": "input_fingerprint", "name": "input", "fingerprint": "same"},
        ]
    elif mode == "constant_labels":
        events = [
            {"event": "label_fingerprint", "name": "label", "fingerprint": "same"},
            {"event": "label_fingerprint", "name": "label", "fingerprint": "same"},
        ]
    elif mode == "empty_subset":
        events = [{"event": "subset_count", "subset": "train", "count": 0}]
    else:
        events = SUCCESS_EVENTS

    for event in events:
        print(json.dumps(event), flush=True)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
