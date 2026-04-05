from __future__ import annotations

import textwrap
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
import sys

if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

import final_review_bundle


def write_file(path: Path, contents: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(textwrap.dedent(contents).strip() + "\n", encoding="utf-8")


class FinalReviewComparisonBundleTest(unittest.TestCase):
    def test_build_final_review_comparison_bundle_extracts_ultralytics_pre_surfaces(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            candidate = tmp / "candidate"
            fixture_post = tmp / "post"
            candidate.mkdir()
            fixture_post.mkdir()

            write_file(
                candidate / "leap.yaml",
                """
                entryFile: leap_integration.py
                include:
                  - leap_integration.py
                  - leap_binder.py
                """,
            )
            write_file(
                candidate / "leap_integration.py",
                """
                from pathlib import Path

                import onnxruntime as ort
                from code_loader.contract.datasetclasses import PredictionTypeHandler
                from code_loader.inner_leap_binder.leapbinder_decorators import (
                    tensorleap_integration_test,
                    tensorleap_load_model,
                    tensorleap_preprocess,
                )
                from leap_binder import preprocess_func_leap

                prediction_type1 = PredictionTypeHandler(name="object detection", labels=["x", "y"], channel_dim=1)

                @tensorleap_preprocess()
                def preprocess():
                    return preprocess_func_leap()

                @tensorleap_load_model([prediction_type1])
                def load_model():
                    model_path = Path(__file__).resolve().parent / ".concierge" / "materialized_models" / "model.onnx"
                    return ort.InferenceSession(str(model_path))

                @tensorleap_integration_test()
                def check_custom_test_mapping(idx, subset):
                    return None

                if __name__ == "__main__":
                    responses = preprocess()
                    for subset in responses:
                        for sample_id in subset.sample_ids[:5]:
                            check_custom_test_mapping(sample_id, subset)
                """,
            )
            write_file(
                candidate / "leap_binder.py",
                """
                import numpy as np
                from code_loader.contract.datasetclasses import DataStateType, PreprocessResponse

                def preprocess_func_leap():
                    return []

                def input_encoder(idx: int, preprocess: PreprocessResponse) -> np.ndarray:
                    return np.zeros((1, 3, 32, 32), dtype="float32")

                def gt_encoder(idx: int, preprocessing: PreprocessResponse) -> np.ndarray:
                    if preprocessing.state == DataStateType.unlabeled:
                        return np.zeros((1, 5), dtype="float32")
                    return np.ones((1, 5), dtype="float32")
                """,
            )

            write_file(
                fixture_post / "leap.yaml",
                """
                entryFile: leap_integration.py
                include:
                  - leap_integration.py
                  - leap_binder.py
                """,
            )
            write_file(
                fixture_post / "leap_integration.py",
                """
                from code_loader.contract.datasetclasses import PredictionTypeHandler, SamplePreprocessResponse
                from code_loader.inner_leap_binder.leapbinder_decorators import (
                    tensorleap_integration_test,
                    tensorleap_load_model,
                )
                from leap_binder import gt_encoder, input_encoder, preprocess_func_leap

                prediction_type1 = PredictionTypeHandler(name="object detection", labels=["x", "y"], channel_dim=1)

                @tensorleap_load_model([prediction_type1])
                def load_model():
                    return object()

                @tensorleap_integration_test()
                def check_custom_test_mapping(idx, subset):
                    sample = SamplePreprocessResponse(idx, subset)
                    image = input_encoder(idx, subset)
                    model = load_model()
                    predictions = model.run(None, {"images": image})
                    gt = gt_encoder(idx, subset)
                    return predictions, gt, sample

                if __name__ == "__main__":
                    check_custom_test_mapping(0, preprocess_func_leap()[1])
                """,
            )
            write_file(
                fixture_post / "leap_binder.py",
                """
                import numpy as np
                from code_loader.contract.datasetclasses import DataStateType, PreprocessResponse
                from code_loader.inner_leap_binder.leapbinder_decorators import (
                    tensorleap_gt_encoder,
                    tensorleap_input_encoder,
                    tensorleap_preprocess,
                )

                @tensorleap_preprocess()
                def preprocess_func_leap():
                    return []

                @tensorleap_input_encoder("image", channel_dim=1)
                def input_encoder(idx: int, preprocess: PreprocessResponse) -> np.ndarray:
                    return np.zeros((1, 3, 32, 32), dtype="float32")

                @tensorleap_gt_encoder("classes")
                def gt_encoder(idx: int, preprocessing: PreprocessResponse) -> np.ndarray:
                    if preprocessing.state == DataStateType.unlabeled:
                        return np.zeros((1, 5), dtype="float32")
                    return np.ones((1, 5), dtype="float32")
                """,
            )

            bundle = final_review_bundle.build_final_review_comparison_bundle(
                run_context={
                    "run_id": "nightly-ultralytics-pre-20260405",
                    "fixture_id": "ultralytics",
                    "guide_step": "pre",
                    "ref_under_test": "main@1234567",
                    "checkpoint_key": "ultralytics:pre",
                    "source_kind": "variant",
                    "source_id": "pre",
                    "stop_reason": "integration_review_failed",
                },
                candidate_workspace=candidate,
                fixture_post_path=fixture_post,
            )

            self.assertEqual(bundle["schema_version"], 1)
            self.assertEqual(bundle["run"]["guide_step"], "pre")
            self.assertEqual(bundle["step_context"]["primary_criteria"][0], "checkpoint_appropriateness")
            self.assertIn("prediction_surface_alignment", bundle["step_context"]["primary_criteria"])

            self.assertEqual(bundle["candidate"]["leap_yaml"]["entry_file"], "leap_integration.py")
            self.assertEqual(bundle["candidate"]["python"]["input_encoders"], [])
            self.assertEqual(bundle["fixture_post"]["python"]["input_encoders"][0]["name"], "image")
            self.assertEqual(bundle["fixture_post"]["python"]["input_encoders"][0]["channel_dim"], 1)
            self.assertEqual(
                bundle["fixture_post"]["python"]["input_encoders"][0]["provenance"]["path"],
                "leap_binder.py",
            )
            self.assertIn(
                "@tensorleap_input_encoder",
                bundle["fixture_post"]["python"]["input_encoders"][0]["provenance"]["snippet"],
            )

            self.assertEqual(
                bundle["comparison"]["input_encoders"]["missing_from_candidate"],
                ["image"],
            )
            self.assertEqual(
                bundle["comparison"]["gt_encoders"]["missing_from_candidate"],
                ["classes"],
            )
            self.assertEqual(
                bundle["comparison"]["prediction_types"]["shared"],
                ["object detection"],
            )

            self.assertEqual(
                [entry["callee"] for entry in bundle["fixture_post"]["python"]["integration_tests"][0]["call_sequence"]],
                ["SamplePreprocessResponse", "input_encoder", "load_model", "model.run", "gt_encoder"],
            )
            self.assertEqual(
                bundle["candidate"]["python"]["main_blocks"][0]["sample_id_loops"][0]["iter_expression"],
                "subset.sample_ids[:5]",
            )
            self.assertEqual(
                bundle["comparison"]["integration_tests"]["fixture_call_sequence"],
                ["SamplePreprocessResponse", "input_encoder", "load_model", "model.run", "gt_encoder"],
            )
