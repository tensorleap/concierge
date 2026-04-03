# Tensorleap Knowledge Pack

version: tlkp-v1
scope: mandatory onboarding contracts for Concierge v1

## leap_yaml_contract

- Canonical location: `leap.yaml` at repository root.
- `leap.yaml` is mandatory and must be at the integration repository root.
- `entryFile` must point to the uploaded Python entry file used for integration execution.
- `include` and `exclude` define upload boundaries; required integration artifacts must be included.
- For initial integration, avoid hardcoding identifiers such as `projectId` and `secretId`.
- Expected minimal shape:

```yaml
entryFile: leap_integration.py
include:
  - leap.yaml
  - leap_integration.py
exclude:
  - .git/**
```

- If `load_model()` depends on a repo-local model artifact, that artifact path must be covered by `include`; do not blanket-exclude `.concierge/**` when Concierge materializes the selected model under `.concierge/materialized_models/`.

## preprocess_contract

- Preprocess must be registered with `@tensorleap_preprocess`.
- Preprocess returns `PreprocessResponse` items for dataset subsets.
- Train and validation subsets are mandatory.
- Preprocess should prepare deterministic dataset access used by encoders.
- Prefer explicit dataset manifests, loader code, or existing Tensorleap integration examples over arbitrary repository files.
- If the repository already declares train/validation subsets in a dataset manifest, reuse those declared subsets instead of inventing a new split from arbitrary images.
- If the repository exposes a dataset manifest and a supported resolver/downloader, use that repo-supported path instead of hard-coding cache directories or scanning generic assets.
- Smoke-test repository dataset helpers before depending on them; if the helper import fails in the current repo state, fall back to the manifest's explicit download/path/train/val information or stop with the blocker.
- If a repo helper import fails because project dependencies are missing, do not reverse-engineer internal cache constants or framework settings paths; use explicit manifest train/val/download evidence or stop with the blocker.
- If Repository Facts provide a prepared runtime interpreter, use that interpreter for Python repo checks instead of bare `python`/`python3`; treat failures under the wrong interpreter as environment mismatch evidence rather than dataset-path evidence.
- If a repository-specific sibling-datasets convention would resolve to a top-level filesystem path in the current runtime, such as repo root `/workspace` producing `/datasets`, do not create that path unless the runtime explicitly provides it as writable; prefer a repo-local writable path backed by repository evidence or stop with the blocker.
- Do not set deprecated `PreprocessResponse.length`; provide real `sample_ids` and `state` values for each subset instead.
- `sample_id_type` controls the runtime type passed as the first argument to input encoders, GT encoders, and integration tests; when `sample_ids` are provided and `sample_id_type` is omitted, code_loader defaults it to `str`.
- Framework-managed dataset cache paths are acceptable only when reached through repository-supported loaders or manifest resolution; do not invent or hard-code them yourself.
- Generic repo assets, screenshots, docs media, and example images are not valid dataset evidence unless the repository explicitly identifies them as the real train/validation data.
- If real train/validation identifiers cannot be derived from repository evidence or a repo-supported acquisition flow, stop and surface the data blocker instead of guessing.
- Canonical type definitions:

```python
from dataclasses import dataclass
from typing import Any, List, Optional, Type, Union
from code_loader.contract.enums import DataStateType

class DataStateType(Enum):
    training = "training"
    validation = "validation"
    test = "test"
    unlabeled = "unlabeled"

@dataclass
class PreprocessResponse:
    data: Any = None
    sample_ids: Optional[Union[List[str], List[int]]] = None
    state: Optional[DataStateType] = None
    sample_id_type: Optional[Union[Type[str], Type[int]]] = None
```

- Canonical preprocess signature:

```python
from typing import List
from code_loader.contract.datasetclasses import PreprocessResponse
from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess

@tensorleap_preprocess()
def preprocess_func() -> List[PreprocessResponse]:
    ...
```

- Required invariant: return at least one `DataStateType.training` and one `DataStateType.validation` response.

## input_encoder_contract

- Input encoders are registered with `@tensorleap_input_encoder`.
- Input encoders receive the Tensorleap `sample_id` from `PreprocessResponse.sample_ids`, not a guaranteed integer index.
- Input encoders must execute reliably across representative samples for each required input symbol.
- Register each input encoder with the exact required Tensorleap symbol name; do not substitute raw model tensor aliases such as `images` for a required symbol such as `image`.
- Canonical signature:

```python
import numpy as np
from typing import Union
from code_loader.contract.datasetclasses import PreprocessResponse
from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_input_encoder

@tensorleap_input_encoder(name="image", channel_dim=-1)
def input_encoder(sample_id: Union[int, str], preprocess: PreprocessResponse) -> np.ndarray:
    ...
```

- `sample_id` must be handled according to `PreprocessResponse.sample_id_type`; it is not guaranteed to be `int`.
- Returns `np.ndarray` input without batch dimension.
- `channel_dim` defaults to `-1` (channels last); use `1` for channels-first outputs.

## ground_truth_encoder_contract

- Ground-truth encoders are registered with `@tensorleap_gt_encoder`.
- GT encoders run on labeled subsets and should align with declared target semantics.
- Unlabeled subsets do not require GT encoder execution.
- GT encoders receive the Tensorleap `sample_id` from `PreprocessResponse.sample_ids`; it is not guaranteed to be `int`.
- Canonical signature:

```python
import numpy as np
from typing import Union
from code_loader.contract.datasetclasses import PreprocessResponse
from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_gt_encoder

@tensorleap_gt_encoder(name="classes")
def gt_encoder(sample_id: Union[int, str], preprocess: PreprocessResponse) -> np.ndarray:
    ...
```

- Returns `np.ndarray` ground truth without batch dimension.

## integration_test_wiring_contract

- Integration test is mandatory and must be decorated with `@tensorleap_integration_test`.
- Only decorators called within the integration-test path are used during platform analysis.
- Integration test should wire preprocess, encoders, model loading, and required interfaces explicitly.
- The first integration-test argument is the Tensorleap `sample_id` from `PreprocessResponse.sample_ids`, not a guaranteed integer index.
- Canonical signature:

```python
from typing import Union
from code_loader.contract.datasetclasses import PreprocessResponse
from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_integration_test

@tensorleap_integration_test()
def integration_test(sample_id: Union[int, str], subset: PreprocessResponse) -> None:
    ...
```

- Required wiring order:
  1. Fetch input(s) via decorated input encoder(s).
  2. Load model via decorated `load_model()`.
  3. Run inference.
  4. Fetch GT via decorated GT encoder(s) for labeled subsets.
  5. Call decorated metadata/metrics/loss/visualizers as needed.
- Do not add/remove batch dimension manually in integration test; Tensorleap handles batching around encoder calls.
- Keep placeholder scaffolds minimal while the integration is incomplete, but use the runtime-correct final inference path once `load_model()` is real.
- For ONNX Runtime sessions returned from `load_model()`, `model.get_inputs()` / `model.run(...)` are valid final integration-test wiring.
- Still move unrelated transforms, decoding, formatting, and business logic into decorated interfaces instead of placing them in `integration_test()`.

## load_model_contract

- Model loading must be declared with `@tensorleap_load_model`.
- Supported artifact formats are `.onnx` and `.h5`.
- Model inputs/outputs must follow Tensorleap batch-dimension expectations (`[Batch, ...]`).
- Analyze repository docs, scripts, config files, and public example assets to find how the project obtains or exports its model before inventing new helpers.
- If repository-local model export requires importing the workspace package and that import fails under the prepared runtime, treat that export path as unavailable in the current repo state instead of debugging package imports or mutating the environment.
- If repository evidence exposes a direct supported `.onnx`/`.h5` artifact or a documented public example artifact, prefer materializing that direct artifact over exporting from unsupported weight files.
- Public example models are acceptable acquisition evidence when the repository itself uses them for tutorials, Docker images, examples, or onboarding flows.
- Canonical output descriptor:

```python
from dataclasses import dataclass
from typing import List

@dataclass
class PredictionTypeHandler:
    name: str
    labels: List[str]
    channel_dim: int = -1
```

- Canonical signature:

```python
from code_loader.contract.datasetclasses import PredictionTypeHandler
from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model

prediction_type1 = PredictionTypeHandler("classes", [str(i) for i in range(10)], channel_dim=-1)

@tensorleap_load_model([prediction_type1])
def load_model():
    ...
```

- Number of `PredictionTypeHandler` entries must match number of model outputs.
