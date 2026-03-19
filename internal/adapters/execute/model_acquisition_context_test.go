package execute

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestBuildModelAcquisitionRecommendationIncludesLeadHintsAndFallbackGuidance(t *testing.T) {
	recommendation, err := BuildModelAcquisitionRecommendation(
		core.WorkspaceSnapshot{
			Repository:        core.RepositoryState{Root: t.TempDir()},
			SelectedModelPath: ".concierge/materialized_models/model.onnx",
		},
		core.IntegrationStatus{
			Contracts: &core.IntegrationContracts{
				ModelAcquisition: &core.ModelAcquisitionArtifacts{
					AcquisitionLeads: []string{
						"project_config.yaml",
						"docker/Dockerfile-cpu -> https://github.com/example/assets/model.onnx",
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildModelAcquisitionRecommendation returned error: %v", err)
	}

	if !containsString(recommendation.Constraints, "If repository-local export or model imports fail under the prepared runtime, treat that export path as unavailable in the current repo state instead of debugging package imports or mutating the environment.") {
		t.Fatalf("expected runtime-import fallback guidance, got %+v", recommendation.Constraints)
	}
	if !containsConstraintContaining(recommendation.Constraints, "Inspect and reuse repository model acquisition leads before inventing helpers:") {
		t.Fatalf("expected lead hint constraint, got %+v", recommendation.Constraints)
	}
}

func TestBuildModelAcquisitionRecommendationUsesStructuredPlanContext(t *testing.T) {
	recommendation, err := BuildModelAcquisitionRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: t.TempDir()},
		},
		core.IntegrationStatus{
			Contracts: &core.IntegrationContracts{
				ModelAcquisition: &core.ModelAcquisitionArtifacts{
					NormalizedPlan: &core.ModelAcquisitionPlan{
						Strategy:           "repo_helper_export",
						RuntimeInvocation:  []string{"poetry", "run", "python", "tools/export_model.py"},
						WorkingDir:         "tools",
						ExpectedOutputPath: ".concierge/materialized_models/model.onnx",
						HelperPath:         ".concierge/materializers/materialize_model.py",
						RequiresNetwork:    true,
						Evidence: []core.ModelAcquisitionPlanEvidence{
							{Path: "README.md", Line: 12, Detail: "documents export helper", Snippet: "python tools/export_model.py"},
						},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildModelAcquisitionRecommendation returned error: %v", err)
	}

	if recommendation.Target != ".concierge/materialized_models/model.onnx" {
		t.Fatalf("expected recommendation target %q, got %+v", ".concierge/materialized_models/model.onnx", recommendation)
	}
	if recommendation.Rationale != "execute_selected_model_acquisition_strategy" {
		t.Fatalf("expected strategy-aware rationale, got %+v", recommendation)
	}
	requiredConstraints := []string{
		`Execute the selected acquisition strategy "repo_helper_export" instead of rediscovering how this repository should obtain its model.`,
		`Run the planned repository invocation: poetry run python tools/export_model.py`,
		`Run the planned invocation from "tools".`,
		`Expected supported artifact output: ".concierge/materialized_models/model.onnx".`,
		`If a helper script is required, use ".concierge/materializers/materialize_model.py".`,
		`This strategy may require network access; if the current environment cannot reach the network, stop and report that blocker instead of inventing a different strategy.`,
	}
	for _, want := range requiredConstraints {
		if !containsString(recommendation.Constraints, want) {
			t.Fatalf("expected plan-aware constraint %q in %+v", want, recommendation.Constraints)
		}
	}
	if containsString(recommendation.Constraints, "If repository evidence includes a direct supported .onnx/.h5 artifact or a documented public example artifact, prefer materializing that direct artifact over exporting from unsupported weight files.") {
		t.Fatalf("expected structured strategy to replace generic artifact preference guidance, got %+v", recommendation.Constraints)
	}
	if !containsConstraintContaining(recommendation.Constraints, "Plan evidence: README.md:12 documents export helper") {
		t.Fatalf("expected plan evidence in constraints, got %+v", recommendation.Constraints)
	}
}

func containsConstraintContaining(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
