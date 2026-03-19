#!/usr/bin/env bash
set -euo pipefail

repo_root="$(pwd)"
data_root="${repo_root}/.concierge/materialized_data"
model_root="${repo_root}/.concierge/materialized_models"
pt_path="${data_root}/models/yolo11s.pt"
downloaded_onnx_path="${data_root}/models/yolo11n.onnx"
final_onnx_path="${model_root}/model.onnx"

mkdir -p "${data_root}/models" "${model_root}"

poetry run pip install --no-cache-dir "onnxruntime==1.21.1"

poetry run python - <<'PY'
from pathlib import Path
from shutil import copyfile

from ultralytics.utils.downloads import attempt_download_asset

repo_root = Path.cwd()
data_root = repo_root / ".concierge" / "materialized_data"
model_root = repo_root / ".concierge" / "materialized_models"
pt_path = data_root / "models" / "yolo11s.pt"
downloaded_onnx_path = data_root / "models" / "yolo11n.onnx"
final_onnx_path = model_root / "model.onnx"

attempt_download_asset(str(pt_path), repo="ultralytics/assets", release="v8.3.0")
attempt_download_asset(str(downloaded_onnx_path), repo="ultralytics/assets", release="v8.3.0")
copyfile(downloaded_onnx_path, final_onnx_path)
PY

poetry run python - <<'PY'
import leap_integration as li

responses = li.preprocess()
states = [str(response.state).lower() for response in responses]

if not any("training" in state for state in states):
    raise SystemExit(f"warmup missing training preprocess subset: {states}")
if not any("validation" in state for state in states):
    raise SystemExit(f"warmup missing validation preprocess subset: {states}")

li.load_model()
PY
