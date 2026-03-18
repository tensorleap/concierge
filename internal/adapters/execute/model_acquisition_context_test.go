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

func containsConstraintContaining(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
