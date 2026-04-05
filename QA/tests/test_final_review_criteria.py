from __future__ import annotations

import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
import sys

if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

import final_review_criteria


class FinalReviewCriteriaTest(unittest.TestCase):
    def test_criteria_for_pre_append_low_weight_subjective_review(self) -> None:
        criteria = final_review_criteria.criteria_for_step(
            "pre",
            [
                "checkpoint_appropriateness",
                "encoder_inventory_alignment",
                "prediction_surface_alignment",
                "preprocess_sample_model_alignment",
                "input_tensor_contract_alignment",
                "gt_contract_alignment",
            ],
        )

        self.assertEqual(
            [criterion["id"] for criterion in criteria],
            [
                "checkpoint_appropriateness",
                "encoder_inventory_alignment",
                "prediction_surface_alignment",
                "preprocess_sample_model_alignment",
                "input_tensor_contract_alignment",
                "gt_contract_alignment",
                "overall_subjective_judgment",
            ],
        )
        self.assertAlmostEqual(sum(criterion["weight"] for criterion in criteria), 1.0)
        self.assertLess(criteria[-1]["weight"], 0.05)

    def test_aggregate_review_passes_when_subjective_judgment_is_low_weight(self) -> None:
        criteria = final_review_criteria.criteria_for_step(
            "pre",
            [
                "checkpoint_appropriateness",
                "encoder_inventory_alignment",
                "prediction_surface_alignment",
                "preprocess_sample_model_alignment",
                "input_tensor_contract_alignment",
                "gt_contract_alignment",
            ],
        )

        results = [
            self._result("checkpoint_appropriateness", 0.86),
            self._result("encoder_inventory_alignment", 0.88),
            self._result("prediction_surface_alignment", 0.91),
            self._result("preprocess_sample_model_alignment", 0.84),
            self._result("input_tensor_contract_alignment", 0.87),
            self._result("gt_contract_alignment", 0.85),
            self._result("overall_subjective_judgment", 0.10, confidence="low"),
        ]

        review = final_review_criteria.aggregate_review(criteria=criteria, results=results)

        self.assertEqual(review["status"], "pass")
        self.assertIn(review["aggregate_band"], {"accept", "strong_accept"})
        self.assertGreaterEqual(review["aggregate_score"], final_review_criteria.AGGREGATE_PASS_THRESHOLD)
        self.assertEqual(review["blocking_criteria"], [])

    def test_aggregate_review_fails_when_primary_criterion_drops_below_floor(self) -> None:
        criteria = final_review_criteria.criteria_for_step(
            "pre",
            [
                "checkpoint_appropriateness",
                "encoder_inventory_alignment",
                "prediction_surface_alignment",
                "preprocess_sample_model_alignment",
                "input_tensor_contract_alignment",
                "gt_contract_alignment",
            ],
        )

        results = [
            self._result("checkpoint_appropriateness", 0.92),
            self._result("encoder_inventory_alignment", 0.18, concerns=["Input encoder inventory diverges from fixture."]),
            self._result("prediction_surface_alignment", 0.93),
            self._result("preprocess_sample_model_alignment", 0.91),
            self._result("input_tensor_contract_alignment", 0.92),
            self._result("gt_contract_alignment", 0.94),
            self._result("overall_subjective_judgment", 0.95),
        ]

        review = final_review_criteria.aggregate_review(criteria=criteria, results=results)

        self.assertEqual(review["status"], "fail")
        self.assertIn("encoder_inventory_alignment", review["blocking_criteria"])
        self.assertGreater(review["aggregate_score"], final_review_criteria.AGGREGATE_PASS_THRESHOLD)
        self.assertIn("controller thresholds", review["verdict"])

    def _result(
        self,
        criterion_id: str,
        score: float,
        *,
        confidence: str = "high",
        summary: str | None = None,
        evidence: list[str] | None = None,
        concerns: list[str] | None = None,
    ) -> dict[str, object]:
        return {
            "criterion_id": criterion_id,
            "score": score,
            "confidence": confidence,
            "summary": summary or f"{criterion_id} summary",
            "evidence": list(evidence or [f"{criterion_id} evidence"]),
            "concerns": list(concerns or []),
        }


if __name__ == "__main__":
    unittest.main()
