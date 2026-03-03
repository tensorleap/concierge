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
entryFile: leap_binder.py
include:
  - leap.yaml
  - leap_binder.py
  - leap_integration.py
exclude:
  - .git/**
  - .concierge/**
```

## preprocess_contract

- Preprocess must be registered with `@tensorleap_preprocess`.
- Preprocess returns `PreprocessResponse` items for dataset subsets.
- Train and validation subsets are mandatory.
- Preprocess should prepare deterministic dataset access used by encoders.
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
    length: Optional[int] = None
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
- Input encoders run per sample index and must be consistent with model input shapes/dtypes.
- Input encoders must execute reliably across multiple indices for each required input symbol.
- Canonical signature:

```python
import numpy as np
from code_loader.contract.datasetclasses import PreprocessResponse
from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_input_encoder

@tensorleap_input_encoder(name="image", channel_dim=-1)
def input_encoder(idx: int, preprocess: PreprocessResponse) -> np.ndarray:
    ...
```

- Returns `np.ndarray` input without batch dimension.
- `channel_dim` defaults to `-1` (channels last); use `1` for channels-first outputs.

## ground_truth_encoder_contract

- Ground-truth encoders are registered with `@tensorleap_gt_encoder`.
- GT encoders run on labeled subsets and should align with declared target semantics.
- Unlabeled subsets do not require GT encoder execution.
- Canonical signature:

```python
import numpy as np
from code_loader.contract.datasetclasses import PreprocessResponse
from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_gt_encoder

@tensorleap_gt_encoder(name="classes")
def gt_encoder(idx: int, preprocess: PreprocessResponse) -> np.ndarray:
    ...
```

- Returns `np.ndarray` ground truth without batch dimension.

## integration_test_wiring_contract

- Integration test is mandatory and must be decorated with `@tensorleap_integration_test`.
- Only decorators called within the integration-test path are used during platform analysis.
- Integration test should wire preprocess, encoders, model loading, and required interfaces explicitly.
- Canonical signature:

```python
from code_loader.contract.datasetclasses import PreprocessResponse
from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_integration_test

@tensorleap_integration_test()
def integration_test(idx: int, subset: PreprocessResponse) -> None:
    ...
```

- Required wiring order:
  1. Fetch input(s) via decorated input encoder(s).
  2. Load model via decorated `load_model()`.
  3. Run inference.
  4. Fetch GT via decorated GT encoder(s) for labeled subsets.
  5. Call decorated metadata/metrics/loss/visualizers as needed.
- Do not add/remove batch dimension manually in integration test; Tensorleap handles batching around encoder calls.

## load_model_contract

- Model loading must be declared with `@tensorleap_load_model`.
- Supported artifact formats are `.onnx` and `.h5`.
- Model inputs/outputs must follow Tensorleap batch-dimension expectations (`[Batch, ...]`).
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
