package execute

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestBuildIntegrationTestAuthoringRecommendationSeparatesMissingCallsFromBodyLogic(t *testing.T) {
	recommendation, err := BuildIntegrationTestAuthoringRecommendation(core.WorkspaceSnapshot{}, core.IntegrationStatus{
		Issues: []core.Issue{
			{
				Code:     core.IssueCodeIntegrationTestMissingRequiredCalls,
				Message:  "integration_test does not call the decorated input encoder for required input \"image\"",
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
				Location: &core.IssueLocation{Symbol: "image"},
			},
			{
				Code:     core.IssueCodeIntegrationTestIllegalBodyLogic,
				Message:  "integration_test should stay declarative",
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
				Location: &core.IssueLocation{Symbol: "body_logic"},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildIntegrationTestAuthoringRecommendation returned error: %v", err)
	}

	if recommendation.StepID != core.EnsureStepIntegrationTestWiring {
		t.Fatalf("expected step %q, got %q", core.EnsureStepIntegrationTestWiring, recommendation.StepID)
	}
	if recommendation.Target != "image" {
		t.Fatalf("expected primary target %q, got %q", "image", recommendation.Target)
	}
	if !strings.Contains(strings.Join(recommendation.Constraints, " | "), "First repair missing decorated calls: image.") {
		t.Fatalf("expected missing-call constraint, got %+v", recommendation.Constraints)
	}
	if !strings.Contains(strings.Join(recommendation.Constraints, " | "), "Then remove illegal body logic: body_logic.") {
		t.Fatalf("expected body-logic constraint, got %+v", recommendation.Constraints)
	}
}

func TestBuildIntegrationTestAuthoringRecommendationFallsBackToGenericRationale(t *testing.T) {
	recommendation, err := BuildIntegrationTestAuthoringRecommendation(core.WorkspaceSnapshot{}, core.IntegrationStatus{})
	if err != nil {
		t.Fatalf("BuildIntegrationTestAuthoringRecommendation returned error: %v", err)
	}
	if recommendation.Rationale == "" {
		t.Fatal("expected non-empty rationale")
	}
	if len(recommendation.Constraints) == 0 {
		t.Fatal("expected default constraints")
	}
	if !strings.Contains(strings.Join(recommendation.Constraints, " | "), "do not stop after load_model()") {
		t.Fatalf("expected default constraints to require real inference wiring, got %+v", recommendation.Constraints)
	}
}

func TestBuildIntegrationTestAuthoringRecommendationWarnsAgainstManualBatchingAndTensorTransforms(t *testing.T) {
	recommendation, err := BuildIntegrationTestAuthoringRecommendation(core.WorkspaceSnapshot{}, core.IntegrationStatus{
		Issues: []core.Issue{
			{
				Code:     core.IssueCodeIntegrationTestManualBatchManipulation,
				Message:  "Tensorleap adds the batch dimension automatically inside integration_test",
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
				Location: &core.IssueLocation{Symbol: "manual_batching"},
			},
			{
				Code:     core.IssueCodeIntegrationTestIllegalBodyLogic,
				Message:  "integration_test should not call external-library transforms directly; move that logic into decorated interfaces",
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
				Location: &core.IssueLocation{Symbol: "body_logic"},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildIntegrationTestAuthoringRecommendation returned error: %v", err)
	}

	joined := strings.Join(recommendation.Constraints, " | ")
	if !strings.Contains(joined, "np.expand_dims") || !strings.Contains(joined, "transpose") {
		t.Fatalf("expected explicit batching/transform constraints, got %+v", recommendation.Constraints)
	}
	if strings.Contains(joined, "raw runtime-session calls") {
		t.Fatalf("did not expect final ONNX runtime calls to be forbidden, got %+v", recommendation.Constraints)
	}
	if !strings.Contains(joined, "runtime-correct inference wiring") {
		t.Fatalf("expected explicit runtime-correct inference guidance, got %+v", recommendation.Constraints)
	}
}

func TestBuildIntegrationTestAuthoringRecommendationExplainsHowToConsumePredictions(t *testing.T) {
	recommendation, err := BuildIntegrationTestAuthoringRecommendation(core.WorkspaceSnapshot{}, core.IntegrationStatus{
		Issues: []core.Issue{
			{
				Code:     core.IssueCodeIntegrationTestMissingRequiredCalls,
				Message:  "integration_test executes model inference but never consumes model outputs",
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
				Location: &core.IssueLocation{Symbol: "prediction_outputs"},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildIntegrationTestAuthoringRecommendation returned error: %v", err)
	}

	joined := strings.Join(recommendation.Constraints, " | ")
	if !strings.Contains(joined, "Bare assignment is insufficient") {
		t.Fatalf("expected explicit bare-assignment warning, got %+v", recommendation.Constraints)
	}
	if !strings.Contains(joined, "_ = prediction_outputs[0]") {
		t.Fatalf("expected explicit valid consumption example, got %+v", recommendation.Constraints)
	}
}
