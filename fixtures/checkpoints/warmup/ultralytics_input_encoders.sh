#!/usr/bin/env bash
set -euo pipefail

repo_root="$(pwd)"
data_root="${repo_root}/.concierge/materialized_data"
model_root="${repo_root}/.concierge/materialized_models"
pt_path="${data_root}/models/yolo11s.pt"
final_onnx_path="${model_root}/model.onnx"

mkdir -p "${data_root}/models" "${model_root}"

poetry run pip install --no-cache-dir "onnxslim==0.1.89"

poetry run python - <<'PY'
from pathlib import Path
from shutil import copyfile

import torch
from export_model_to_tf import onnx_exporter
from ultralytics.utils.downloads import attempt_download_asset

repo_root = Path.cwd()
data_root = repo_root / ".concierge" / "materialized_data"
model_root = repo_root / ".concierge" / "materialized_models"
pt_path = data_root / "models" / "yolo11s.pt"
final_onnx_path = model_root / "model.onnx"

# Torch 2.1.0 on Linux/arm64 segfaults in Ultralytics export with MKLDNN enabled.
torch.backends.mkldnn.enabled = False
attempt_download_asset(str(pt_path), repo="ultralytics/assets", release="v8.3.0")
exported_onnx_path = Path(onnx_exporter())
copyfile(exported_onnx_path, final_onnx_path)
PY

poetry run python - <<'PY'
from pathlib import Path

import leap_integration as li

responses = li.preprocess()
states = [str(response.state).lower() for response in responses]

if not any("training" in state for state in states):
    raise SystemExit(f"warmup missing training preprocess subset: {states}")
if not any("validation" in state for state in states):
    raise SystemExit(f"warmup missing validation preprocess subset: {states}")

li.load_model()

source = Path("leap_integration.py").read_text(encoding="utf-8")
required_markers = (
    "@tensorleap_integration_test()",
    "return None",
    "subset.sample_ids[:5]",
)
for marker in required_markers:
    if marker not in source:
        raise SystemExit(f"warmup missing integration-test scaffold marker: {marker}")

forbidden_markers = (
    "model.run(",
    "binder_input_encoder(",
)
for marker in forbidden_markers:
    if marker in source:
        raise SystemExit(f"warmup found legacy integration-test wiring marker: {marker}")
PY
