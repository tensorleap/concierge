# Writing Tensorleap Integrations in the `leap_integration.py` Style

This guide is for developers who already know the old `leap_binder.py` workflow and need a precise migration path to the new decorator-based integration style.

The most important change is not only the API surface. The authoring loop changed.

In the old style, you mostly wrote a binder script and registered functions.

In the new style:

- `leap_integration.py` is the entrypoint.
- decorators are the authoring API.
- `@tensorleap_integration_test` is the wiring layer.
- you are expected to run the integration while it is still incomplete.
- the local run is supposed to act as a progressive validator.

Under the hood, the decorators still register into the internal binder object. That is an implementation detail. You should not keep using `leap_binder.set_*` as the main way to author new integrations.

## The central rule

Validation happens when a decorated function is called, not merely when it is defined.

That has three practical consequences:

1. A decorator can exist in the file and still show as "missing" from the validator if your `__main__` block never exercises it.
2. Early in the project, you should call new decorated functions directly from `__main__`.
3. As soon as a minimal model path exists, you should switch `__main__` to calling `integration_test(...)`, because that is what unlocks the mapping and binder-level checks.

## What changed from the old `LeapBinder` mentality

Old mentality:

- write a central `leap_binder.py`
- register everything with `leap_binder.set_preprocess`, `set_input`, `set_ground_truth`, `set_metadata`, and so on
- think about the integration as a static registration script

New mentality:

- write decorated interfaces
- make `leap_integration.py` the runnable entrypoint
- keep the integration test thin and declarative
- run partial code after every meaningful step
- use the validator output to decide the next correction

If you already have a legacy binder-based integration, treat it as a source of business logic to port, not as the API surface to continue extending.

## How the new validator actually works

There is not one validator. There are several layers that fire at different times.

### 1. Import-time and decoration-time feedback

Some problems appear as soon as Python imports the module and the decorators are applied.

Examples:

- duplicate input, GT, metadata, metric, visualizer, or loss names
- invalid decorator arguments such as bad `channel_dim`
- visualizer return type hint warnings

These do not require calling the function body.

### 2. Direct call validation on each decorated function

When you call a decorated function, `code-loader` validates:

- the number and types of arguments
- the return type
- output rank and dtype for arrays
- some decorator-specific invariants such as `channel_dim`

This is why early `__main__` should directly call `preprocess()`, `input_encoder(...)`, `gt_encoder(...)`, and `load_model()`.

### 3. Warnings emitted during execution

There are two kinds of warnings to watch for:

- Python warnings such as the batch-dimension warning on encoders
- deferred default-parameter warnings printed at process exit

The deferred warnings exist so the guide can tell you when you relied on defaults such as:

- `PreprocessResponse.state`
- input encoder `channel_dim`
- `tensorleap_load_model(prediction_types=...)`
- prediction type `channel_dim`
- metric `direction`

### 4. The local status table printed at process exit

Outside the Tensorleap platform, importing the decorators installs an exit hook.

That exit hook prints a status table only when the entry script filename is exactly `leap_integration.py`.

The table tracks whether execution successfully exercised:

- `tensorleap_preprocess`
- `tensorleap_integration_test`
- `tensorleap_input_encoder`
- `tensorleap_gt_encoder`
- `tensorleap_load_model`
- `tensorleap_custom_loss`
- `tensorleap_custom_metric (optional)`
- `tensorleap_metadata (optional)`
- `tensorleap_custom_visualizer (optional)`

Important detail:

- rows start as unknown
- if the script exits cleanly, any still-unknown row is converted to a cross
- a cross therefore often means "this interface was never exercised successfully in this run"
- if the script crashes early, unresolved rows stay unknown

The exit hook also prints a "recommended next interface to add" message based on the hardcoded order above.

### 5. `@tensorleap_integration_test` reruns itself in mapping mode

This is the key capability most old-style authors miss.

When you call `integration_test(sample_id, preprocess_response)`:

1. it runs your real integration test body once
2. it reruns the integration test in a special mapping mode
3. in mapping mode, decorated functions return placeholders instead of real data
4. if the integration test does ordinary Python logic outside decorated calls, the mapping pass fails
5. after the mapping pass, `leap_binder.check()` runs dataset checks

This is why the integration test must be thin. It is not just a smoke test. It is also the code-flow declaration that the mapping engine inspects.

### 6. Binder-level checks after `integration_test`

After the integration test completes, `leap_binder.check()` validates:

- preprocess exists
- training subset exists
- validation subset exists
- subset lengths are positive
- the first sample in each subset can pass through the registered inputs, GT handlers, and metadata handlers

If all binder checks pass, it prints:

```text
Successful!
```

This is an important milestone, but it is still only a first-sample validation. It is not proof that the entire dataset is healthy.

### 7. Platform-style parsing via `LeapLoader.check_dataset()`

Separately, `LeapLoader.check_dataset()`:

- imports the entry module
- captures stdout
- checks preprocess
- checks the first sample through registered handlers
- returns a structured parse result

That parse result includes:

- `payloads`
- `is_valid`
- `setup`
- `model_setup`
- `general_error`
- `print_log`
- `engine_file_contract`

For automation, this is more useful than scraping the human-readable table.

## The recommended authoring order

The right order is not "write everything, then test." The right order is "write the minimum next piece that unlocks a more informative validator run."

Use this order:

1. Create `leap_integration.py` and `leap.yaml`.
2. Implement `@tensorleap_preprocess`.
3. Inspect the model I/O contract.
4. Implement the minimum required input encoder set for one real inference.
5. Implement `@tensorleap_load_model`.
6. Implement a minimal `@tensorleap_integration_test`.
7. Add remaining input encoders if the model has more inputs than your first pass covered.
8. Add GT encoder(s).
9. Add a decorated custom loss.
10. Add metadata, visualizers, and metrics one by one.
11. Expand validation from one sample to several samples in training and validation.

This order matters.

`load_model()` by itself can validate model type and declared outputs, but useful integration validation starts only when a real encoded sample can flow into the model.

## Files and project shape

Use `leap_integration.py` as the entry script:

```yaml
entryFile: leap_integration.py
include:
  - leap_integration.py
  - your_project/**/*.py
  - requirements.txt
  - model/**
  - config/**
```

Include every runtime dependency that the integration reads directly:

- model files
- tokenizer files
- label files
- config files
- helper Python modules
- anything opened from disk at runtime

If the code reads a file locally and that file is not included, local validation may pass while platform parsing fails.

## Step-by-step workflow with run points

The point of this section is not only to say what to write. It is to say when to run an incomplete integration and what kind of feedback that run should unlock.

### Step 1: Create `leap_integration.py` and a tiny `__main__`

Start with imports and a development harness.

```python
from typing import List

import numpy as np

from code_loader.contract.datasetclasses import PreprocessResponse, PredictionTypeHandler
from code_loader.contract.enums import DataStateType
from code_loader.inner_leap_binder.leapbinder_decorators import (
    tensorleap_preprocess,
    tensorleap_input_encoder,
    tensorleap_gt_encoder,
    tensorleap_load_model,
    tensorleap_custom_loss,
    tensorleap_metadata,
    tensorleap_custom_visualizer,
    tensorleap_custom_metric,
    tensorleap_integration_test,
)
```

Keep the first `__main__` block extremely small.

```python
if __name__ == "__main__":
    print("integration module imported")
```

Run it once immediately. This confirms:

- imports resolve
- the file is named correctly
- exit-hook behavior is attached

At this point, the status table will mostly show missing interfaces. That is expected.

### Step 2: Implement preprocess first

Preprocess is the root of the integration. Nothing useful can happen without it.

```python
@tensorleap_preprocess()
def preprocess() -> List[PreprocessResponse]:
    train_ids = [...]
    val_ids = [...]

    train = PreprocessResponse(
        sample_ids=train_ids,
        data={"records_by_id": {...}},
        state=DataStateType.training,
    )
    val = PreprocessResponse(
        sample_ids=val_ids,
        data={"records_by_id": {...}},
        state=DataStateType.validation,
    )
    return [train, val]
```

Run it now with a direct smoke check:

```python
if __name__ == "__main__":
    subsets = preprocess()
    print([(subset.state, subset.length) for subset in subsets])
```

Why run now:

- direct preprocess validation fires immediately
- the exit table can now mark preprocess as exercised
- you can catch construction errors before model work exists

What a successful run means:

- the function took no arguments
- it returned a list of `PreprocessResponse`
- each element was valid enough for direct preprocess validation

What it does not prove yet:

- that `training` and `validation` both exist
- that subset lengths are positive
- that the first sample can be fetched through handlers

Those later checks happen only after `integration_test(...)` reaches `leap_binder.check()`.

### Step 3: Inspect the model contract before expecting model validation

Before writing `load_model()` or calling `integration_test`, answer these questions:

- How many model inputs exist?
- What is each input name and dtype?
- What shape does each input expect without the batch dimension?
- How many outputs exist?
- What does each output mean?
- What labels or `PredictionTypeHandler` definitions are required?

Do this inspection outside Tensorleap-specific code if that is easier.

The important rule is:

- author the minimum required set of input encoders for one real inference before you expect the validator to give useful model-path feedback

### Step 4: Implement the minimum input encoder set

Write one encoder per model input. If the model has multiple required inputs, you need enough encoders to perform a real inference.

```python
@tensorleap_input_encoder(name="image", channel_dim=-1)
def image_input(sample_id: str, preprocess: PreprocessResponse) -> np.ndarray:
    row = preprocess.data["records_by_id"][sample_id]
    image = load_image(row["image_path"])
    return image.astype(np.float32)
```

Run each new encoder directly:

```python
if __name__ == "__main__":
    subsets = preprocess()
    train = next(s for s in subsets if s.state == DataStateType.training)
    sample_id = train.sample_ids[0]
    x = image_input(sample_id, train)
    print(x.shape, x.dtype)
```

Why run now:

- direct signature and dtype checks fire here
- this is the fastest place to catch wrong shape, wrong dtype, wrong sample lookup, or accidental batching

Important:

- return a single sample, not a batch
- outside `integration_test`, you may manually add a batch dimension when testing raw model inference
- inside `integration_test`, Tensorleap adds the batch dimension automatically for input and GT encoders

### Step 5: Implement `@tensorleap_load_model`

Once at least one real encoded sample exists, add the model loader.

```python
prediction_types = [
    PredictionTypeHandler(
        name="classes",
        labels=["cat", "dog", "horse"],
        channel_dim=-1,
    )
]


@tensorleap_load_model(prediction_types)
def load_model():
    ...
```

Run it directly:

```python
if __name__ == "__main__":
    model = load_model()
    print(type(model))
```

Why run now:

- model-type validation fires here
- prediction-type declarations themselves are validated here
- repeated calls are cached by the decorator wrapper, so calling `load_model()` inside `integration_test` is not expected to reload the model every time in a normal local run

What this still does not validate:

- the full inference path
- that declared prediction types match the actual number of model outputs
- that encoder outputs line up with model expectations
- mapping-mode compatibility

### Step 6: Add a minimal `@tensorleap_integration_test` as soon as one real model path exists

Do not wait until the whole integration is finished.

As soon as preprocess, the minimum input encoders, and `load_model()` exist, add a minimal integration test and start running it.

```python
@tensorleap_integration_test()
def integration_test(sample_id: str, preprocess: PreprocessResponse):
    x = image_input(sample_id, preprocess)
    model = load_model()
    _ = model(...)
```

Use the exact call style required by the underlying runtime and model signature.

Run it now:

```python
if __name__ == "__main__":
    subsets = preprocess()
    train = next(s for s in subsets if s.state == DataStateType.training)
    integration_test(train.sample_ids[0], train)
```

Why run now:

- this is the first point where mapping-mode validation can run
- this is the first point where `leap_binder.check()` can run
- this is the first point where you can see `Successful!`

Keep the integration test thin:

- call decorated encoders
- call decorated model loader
- call decorated loss, metric, metadata, and visualizer functions as they are added
- avoid ordinary Python transformations in the integration test body

Do not do this in the integration test body:

- `np.squeeze`, `argmax`, `softmax`, decoding, formatting, clipping, thresholding
- direct Pandas logic
- arithmetic on arrays
- reading `sample_id` or `preprocess.data` directly
- indexing intermediate objects other than model predictions

Put that logic into decorated interfaces instead.

Why the restriction is so strict:

- in mapping mode, `integration_test` is rerun with `sample_id=None` and a dummy `PreprocessResponse`
- decorated functions know how to switch to placeholders in that mode
- your plain Python logic does not

### Step 7: Add GT encoder(s)

Once the input-to-model path works, add GT encoders.

```python
@tensorleap_gt_encoder(name="classes")
def class_gt(sample_id: str, preprocess: PreprocessResponse) -> np.ndarray:
    row = preprocess.data["records_by_id"][sample_id]
    return row["one_hot_label"].astype(np.float32)
```

First run the GT directly:

```python
if __name__ == "__main__":
    subsets = preprocess()
    train = next(s for s in subsets if s.state == DataStateType.training)
    sample_id = train.sample_ids[0]
    y_true = class_gt(sample_id, train)
    print(y_true.shape, y_true.dtype)
```

Then call `integration_test(...)` again.

Why this two-step loop matters:

- direct GT call tells you whether the encoder itself is wrong
- `integration_test` then tells you whether the whole path remains mappable and binder-valid

If the dataset is effectively unlabeled for a path, do not return `None`. Return `np.array([], dtype=np.float32)` when that is the intended Tensorleap contract.

### Step 8: Add a decorated custom loss

The current local status-table flow treats custom loss as part of the main path, not as an afterthought.

```python
@tensorleap_custom_loss(name="categorical_ce")
def categorical_ce(y_true: np.ndarray, y_pred: np.ndarray) -> np.ndarray:
    y_pred = np.clip(y_pred, 1e-7, 1.0)
    return -np.sum(y_true * np.log(y_pred), axis=-1)
```

Run it in two phases:

1. call the loss directly on representative batched arrays
2. then call it from `integration_test(...)`

The return value must be batch-aligned and one-dimensional.

### Step 9: Add optional pieces one at a time

After the core path is stable, add:

- metadata
- visualizers
- custom metrics

The rule is:

1. define one new decorated function
2. call it directly if possible
3. call it from `integration_test(...)`
4. rerun
5. fix any new feedback before adding the next optional interface

Do not add three optional interfaces at once. You lose the ability to tell which one introduced the regression.

### Step 10: Expand beyond the first sample

`leap_binder.check()` and `LeapLoader.check_dataset()` validate only the first sample for each registered handler.

That is necessary but not sufficient.

Once the integration passes for one sample, expand `__main__`:

```python
if __name__ == "__main__":
    subsets = preprocess()
    train = next(s for s in subsets if s.state == DataStateType.training)
    val = next(s for s in subsets if s.state == DataStateType.validation)

    for sample_id in train.sample_ids[:3]:
        integration_test(sample_id, train)

    for sample_id in val.sample_ids[:3]:
        integration_test(sample_id, val)
```

This catches problems the first-sample parser will miss:

- sample-specific missing files
- shape drift
- label edge cases
- metadata shape or type drift
- subset-specific logic bugs

## What success looks like at each stage

Use the validator output as a staged signal, not as a binary pass/fail oracle.

Early success:

- preprocess runs directly
- input encoder runs directly
- load model runs directly

Middle success:

- a minimal `integration_test(...)` runs
- the mapping rerun does not fail
- `Successful!` appears

Core success:

- status table shows preprocess, integration test, input encoder, GT encoder, load model, and custom loss as exercised
- exit message says all mandatory parts have been set, or only optional interfaces remain

Real success:

- several training and validation samples pass through `integration_test(...)`
- no accidental default warnings remain
- platform-style parsing returns clean structured payloads

## Known feedback by source and what it means

This section maps the important known feedback signals to their likely meaning and the next action to take.

### Exit-table feedback

`Warnings (Default use. It is recommended to set values explicitly):`

You relied on one or more defaults. Read the lines below it carefully. The run may still work, but the guide should treat this as unfinished authoring. Usually you should make the warned value explicit.

`Parameter 'PreprocessResponse.state' defaults to specific order`

You did not set `state=` on one or more `PreprocessResponse` objects. Tensorleap will infer subset identity by order. Fix it by setting `state=DataStateType.training`, `validation`, and so on explicitly.

`Parameter 'channel_dim' defaults to -1`

An input encoder omitted `channel_dim`. The validator will proceed, but you should set the channel axis explicitly unless the default is deliberately correct.

`Parameter 'prediction_types' defaults to []`

`load_model()` was decorated without explicit prediction types. That prevents a useful validation of output semantics. Add `PredictionTypeHandler` entries once you know the outputs.

`Parameter 'prediction_types[i].channel_dim' defaults to -1`

You defined prediction types but omitted their channel axis. Add it explicitly.

`Parameter 'direction' defaults to Downward`

A custom metric omitted the optimization direction. Add it explicitly so analysis intent is unambiguous.

`Some mandatory components have not yet been added to the Integration test. Recommended next interface to add is: ...`

The script exited cleanly, but one or more mandatory interfaces were never exercised successfully. The next recommended item is the next unchecked mandatory row in the built-in order.

`All mandatory parts have been successfully set. ... continue to the next optional ...`

The core path was exercised successfully. You may stop if the optional interfaces are not needed, or continue with metadata, visualizers, and metrics.

`Script crashed before completing all steps. crashed at function '...'`

An exception escaped before the run completed. The named function is the validator's best guess for where the run died. Fix that function before trusting any later rows.

`Tensorleap_integration_test code flow failed, check raised exception.`

The real integration test body may have started, but the mapping-mode rerun failed. This usually means the integration test contains plain Python logic instead of only calling Tensorleap decorators.

### Generic decorator feedback that appears in many interfaces

`validation failed: Missing required argument`

You called the decorated function with the wrong argument names or omitted a required argument. Fix the call site to match the decorator contract.

`validation failed: Expected exactly ... arguments`

The function was called with the wrong number of arguments. This often happens when legacy helper signatures leak into decorated interfaces.

`validation failed: Argument 'idx' expected type ...`

The call provided the wrong argument type. Most dataset-facing decorators expect `(sample_id, preprocess_response)`.

`validation failed: The function returned None`

The decorator expected one concrete return object and got `None`. In most cases this means you forgot a `return` statement or took a branch that returns nothing.

`validation failed: The function returned multiple outputs`

The decorator expects one object, not a tuple of several objects. Combine the values into one array or move the extra information into separate decorated interfaces.

`warning: Tensorleap will add a batch dimension at axis 0 ...`

An input or GT encoder returned something that already looks batched because axis 0 has size 1. Remove the manual batch dimension from the encoder output.

### Preprocess feedback

`preprocess() validation failed: The function should not take any arguments`

Preprocess in the new flow is argument-free. Move any external configuration lookup into module scope or ordinary helper code.

`expected return type list[PreprocessResponse]`

Preprocess must return a list, even if there is only one subset temporarily. Return a list of `PreprocessResponse` objects.

`expected to return a single list[PreprocessResponse] object, but returned ... objects instead`

You returned multiple top-level objects, usually by accident with a tuple. Wrap the subset responses in one list.

`Element #... in the return list should be a PreprocessResponse`

At least one list element is the wrong type. Construct proper `PreprocessResponse` instances.

`The return list should not contain duplicate PreprocessResponse objects`

You returned the same object instance more than once. Build distinct subset objects.

`length is deprecated, please use sample_ids instead.`

You initialized `PreprocessResponse` incorrectly. Do not set `length` in new code. Return real `sample_ids` for each subset and let Tensorleap derive the subset size from those IDs.

`Sample id should be of type str. Got: ...`

You declared or inferred string sample IDs but returned non-string sample IDs. Make the `sample_ids` list homogeneous.

`PreprocessResponse.state must be of type DataStateType`

You set `state` to the wrong enum or a plain string. Use `DataStateType.training`, `DataStateType.validation`, and so on.

`Duplicate state ... in preprocess results`

Two subsets claimed the same `state`. Each state may appear only once.

`Training data is required`

Binder-level validation reached preprocess and did not find a training subset. Add one.

`Validation data is required`

Binder-level validation reached preprocess and did not find a validation subset. Add one.

`Invalid dataset length`

At least one subset had length `None` or `<= 0`. Make sure the subset contains at least one sample during validation.

`Sample id are too long. Max allowed length is 256 charecters.`

Your string sample IDs exceed the parser limit. Replace them with shorter stable identifiers.

### Input encoder feedback

`Input with name ... already exists. Please choose another`

Two input encoders used the same name. Input names must be unique.

`Channel dim for input ... is expected to be either -1 or positive`

You passed an invalid `channel_dim`. Use `-1` for last axis or a positive axis index.

`Argument sample_id should be as the same type as defined in the preprocess response`

The encoder was called with a sample ID of the wrong type for that subset. Keep `sample_ids` homogeneous and pass them through unchanged.

`Unsupported return type. Should be a numpy array`

The encoder returned a Python list, PIL image, tensor object, or other unsupported type. Convert it to `np.ndarray`.

`The return type should be a numpy array of type float32`

The encoder returned the wrong dtype, usually `float64`, `uint8`, or `int64`. Cast explicitly with `.astype(np.float32)`.

`The channel_dim (...) should be <= to the rank of the resulting input rank (...)`

The declared channel axis does not exist in the returned array. Fix the axis or the returned shape.

The batch-dimension warning described above

The encoder already returned a batch-like shape. Remove the leading batch dimension from the encoder itself.

### Ground-truth encoder feedback

`GT with name ... already exists. Please choose another`

Two GT encoders used the same name. GT names must be unique.

`Argument sample_id should be as the same type as defined in the preprocess response`

The GT encoder received the wrong sample ID type for the current subset.

`The function returned None. If you are working with an unlabeled dataset ... use 'return np.array([], dtype=np.float32)' instead`

The GT encoder returned `None`. For unlabeled paths, return an empty `float32` array instead of `None`.

`Unsupported return type. Should be a numpy array`

The GT encoder must return `np.ndarray`.

`The return type should be a numpy array of type float32`

Cast the GT output to `np.float32`.

The batch-dimension warning described above

The GT encoder already returned a batch-like shape. Remove the leading batch dimension from the GT encoder.

### `load_model` feedback

`prediction_types is an optional argument of type List[PredictionTypeHandler] but got ...`

You passed the wrong object to `@tensorleap_load_model(...)`. The decorator expects a list of `PredictionTypeHandler`.

`prediction_types at position ... must be of type PredictionTypeHandler`

One element of the list is wrong. Fix that element rather than the function body.

`Supported models are Keras and onnxruntime only and non of them was returned`

`load_model()` returned an unsupported type. Return a Keras model or an ONNX Runtime `InferenceSession`.

`number of declared prediction types(...) != number of model outputs(...)`

Your `PredictionTypeHandler` list does not match the actual output count. This appears when the wrapped model is first invoked, not when `load_model()` merely returns. Fix the declarations or the model you load.

`Missing required input(s): [...]`

The ONNX wrapper was called without all required input names. Supply every required ONNX input in the input dictionary.

`Unsupported ONNX input type: ...`

At least one ONNX input dtype is not covered by the coercion map. You may need to adapt the integration or the exported model.

One subtlety:

ONNX inputs are coerced to the input dtypes expected by the session when possible. That is helpful, but it does not replace having correct shapes and correctly named inputs.

### Integration-test feedback

`sample_id type (...) does not match the expected type (...) from the PreprocessResponse`

The integration test was called with a sample ID type that does not match the subset. Fix the call site or the preprocess sample IDs.

`indexing is supported only on the model's predictions inside the integration test`

The mapping-mode rerun saw indexing on something other than model predictions. Remove that indexing from the integration test body and move the logic into a decorated interface.

`Integration test is only allowed to call Tensorleap decorators. Ensure any arithmetics, external library use, Python logic is placed within Tensorleap decoders`

This is the main mapping-mode failure. The integration test body contains plain Python logic that the mapping engine cannot trace. Move the logic into decorated functions and keep the integration test declarative.

`Successful!`

The real run, the mapping rerun, and binder-level first-sample handler checks all passed for that invocation.

Your own `print(...)` output

The parser captures stdout into `print_log`. This is useful for automation, but keep prints small and diagnostic. Do not make parser success depend on a specific print message.

### Custom-loss feedback

`Custom loss with name ... already exists. Please choose another`

Loss names must be unique.

`Expected at least one positional|key-word argument ...`

The loss was called incorrectly. Loss functions operate on already-batched arrays when used from the integration test.

`Argument #... should be a numpy array`

The loss received an unsupported object. Make sure you pass model predictions, GT arrays, or the supported placeholder type.

`The return type should be a numpy array`

Return `np.ndarray`, not a Python float.

`The return type should be a 1Dim numpy array but got ...`

Loss must be batch-aligned and one-dimensional. Reduce across feature axes, not across the batch axis.

### Metadata feedback

`Metadata with name ... already exists. Please choose another`

Metadata handler names must be unique.

`Unsupported return type. Got ... should be any of (...)`

Metadata may return a scalar, `None`, or a flat dictionary of scalar values. It may not return arrays, lists, or nested objects.

`Keys in the return dict should be of type str`

If metadata returns a dictionary, every key must be a string.

`Values in the return dict should be of type ...`

If metadata returns a dictionary, every value must be scalar-like and of a supported type.

`Unsupported return type of metadata ... The return type should be one of [int, float, str, bool, None]`

Binder-level metadata validation encountered an unsupported scalar type. Convert it to a supported Python scalar.

`Metadata ... is None and no metadata type is provided`

You returned `None` for metadata without declaring `metadata_type`. If metadata may be missing, declare its type explicitly.

`Metadata ... is None and metadata type is not a dict`

A dictionary-shaped metadata handler returned `None` for a key, but the declared metadata type did not match that shape. Fix `metadata_type`.

`More than 100 metadata function are not allowed`

The parser imposes a hard limit on the number of metadata handlers.

`More than 800 metadata keys are not allowed`

The parser imposes a hard limit on total flattened metadata keys across handlers.

### Visualizer feedback

`Visualizer with name ... already exists. Please choose another`

Visualizer names must be unique.

`visualizer_type should be of type LeapDataType`

The decorator argument itself is wrong. Pass a `LeapDataType` enum value.

`Argument #... should be a numpy array`

Visualizer inputs must be arrays or the supported preprocess placeholder type.

`Argument #... should be without batch dimension`

Unlike metrics and losses, visualizers expect unbatched sample-level arrays. Remove the batch dimension before calling the visualizer.

`The return type should be ...`

The returned object does not match the declared visualizer type. For example, `LeapDataType.Image` requires returning `LeapImage`.

`Tensorleap Warning: no return type hint for function ...`

This warning is emitted when registering a visualizer if the function lacks a return type annotation. Add the proper return type hint so the visualizer contract is explicit.

### Metric feedback

`Metric with name ... already exists. Please choose another`

Metric names must be unique.

`name must be a string`

The decorator arguments are malformed.

`direction must be a MetricDirection or a Dict[str, MetricDirection]`

The metric direction declaration is malformed.

`compute_insights must be a bool or a Dict[str, bool]`

The metric insight declaration is malformed.

`Expected at least one positional|key-word argument of type np.ndarray`

The metric was called without inputs. Metrics operate on batched arrays.

`Argument #... first dim should be as the batch size`

The metric received an array whose leading dimension does not match the current validation batch size. Metrics operate on batched data.

`has returned unsupported type`

Metric outputs are limited to batch-aligned 1D arrays, lists of scalars, confusion-matrix structures, or dictionaries of those.

`The return shape should be 1D`

If returning a NumPy array, return a one-dimensional batch-aligned result.

`The return len ... should be as the batch size`

Each metric output must have one value per sample in the current batch.

### Old-binder compatibility feedback

`Please remove the metadata_type on leap_binder.set_metadata in your dataset script`

This comes from the parser when legacy binder-style metadata registration is still present in a way the new parser rejects. In new integrations, do not keep using `leap_binder.set_metadata(...)` as the authoring API.

## What to avoid when migrating from the old style

Do not carry these habits into the new flow:

- Do not keep adding new `leap_binder.set_*` registrations.
- Do not make `leap_binder.py` the entrypoint.
- Do not postpone execution until the end. Run partial code after every step.
- Do not keep heavy business logic inside `integration_test`.
- Do not read `sample_id` or `preprocess.data` directly inside `integration_test`.
- Do not manually batch input or GT encoder outputs inside `integration_test`.
- Do not return Python lists where Tensorleap expects `np.ndarray`.
- Do not leave `state`, `channel_dim`, or prediction semantics implicit.
- Do not assume a green first-sample run means the whole dataset is correct.

## A practical `__main__` evolution path

Use `__main__` differently at different stages.

Early stage:

```python
if __name__ == "__main__":
    subsets = preprocess()
    print([(subset.state, subset.length) for subset in subsets])
```

Encoder stage:

```python
if __name__ == "__main__":
    subsets = preprocess()
    train = next(s for s in subsets if s.state == DataStateType.training)
    sample_id = train.sample_ids[0]
    x = image_input(sample_id, train)
    print(x.shape, x.dtype)
```

Model stage:

```python
if __name__ == "__main__":
    subsets = preprocess()
    train = next(s for s in subsets if s.state == DataStateType.training)
    sample_id = train.sample_ids[0]
    x = image_input(sample_id, train)
    model = load_model()
    _ = model(...)
```

Integration-test stage:

```python
if __name__ == "__main__":
    subsets = preprocess()
    train = next(s for s in subsets if s.state == DataStateType.training)
    integration_test(train.sample_ids[0], train)
```

Hardening stage:

```python
if __name__ == "__main__":
    subsets = preprocess()
    for subset in subsets:
        if subset.state not in {DataStateType.training, DataStateType.validation}:
            continue
        for sample_id in subset.sample_ids[:3]:
            integration_test(sample_id, subset)
```

## How to use this programmatically

If you eventually want a coding assistant or automation layer to write integrations, use the validator in two modes.

### Human-oriented mode

Run `python leap_integration.py` and inspect:

- raised exceptions
- warnings
- the exit status table
- the `Successful!` marker

This is useful while a human is iterating.

### Machine-oriented mode

Prefer the structured parse path via `LeapLoader.check_dataset()` and consume:

- `DatasetIntegParseResult.is_valid`
- `DatasetIntegParseResult.payloads`
- `DatasetIntegParseResult.general_error`
- `DatasetIntegParseResult.print_log`
- `DatasetIntegParseResult.setup`
- `DatasetIntegParseResult.model_setup`
- `DatasetIntegParseResult.engine_file_contract`

That gives an automation layer both:

- human-readable clues in `print_log`
- structured per-handler pass/fail data in `payloads`

Do not treat the direct-script exit table as the primary machine interface.

The exit table is mainly a local authoring experience. The structured parser result is the better automation surface.

In other words:

- the local script is a useful progressive validator for authors
- the parser result is the better interface for tooling

## Final guidance

The right way to author a new Tensorleap integration is:

1. make `leap_integration.py` the canonical entrypoint
2. write decorated interfaces in the order that unlocks the next useful validator signal
3. call new pieces directly from `__main__` until a minimal model path exists
4. switch early to calling `integration_test(...)`
5. treat every warning, exception, and status-table row as feedback on the next correction

If you adopt that loop, `leap_integration.py` stops being just "the new file name" and becomes what the current `code-loader` implementation actually intends it to be: a progressive validator for the integration effort itself.
