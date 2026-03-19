package execute

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/persistence"
)

const defaultMaterializedModelName = "model.onnx"
const defaultMaterializerHelperName = "materialize_model.py"

// BuildModelAcquisitionRecommendation builds deterministic acquisition guidance for ensure.model_acquisition.
func BuildModelAcquisitionRecommendation(snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) (core.AuthoringRecommendation, error) {
	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return core.AuthoringRecommendation{}, core.NewError(
			core.KindUnknown,
			"execute.model_acquisition.repo_root",
			"snapshot repository root is empty",
		)
	}

	if _, err := persistence.NewPaths(repoRoot); err != nil {
		return core.AuthoringRecommendation{}, core.WrapError(core.KindUnknown, "execute.model_acquisition.paths", err)
	}

	target := strings.TrimSpace(snapshot.SelectedModelPath)
	if target == "" && status.Contracts != nil {
		target = strings.TrimSpace(status.Contracts.ResolvedModelPath)
	}
	if target == "" {
		target = defaultMaterializedModelPath(status)
	}

	helperPath := filepath.ToSlash(filepath.Join(".concierge", "materializers", defaultMaterializerHelperName))
	target = filepath.ToSlash(filepath.Clean(target))

	plan := selectedModelAcquisitionPlan(snapshot, status)
	if plan != nil && strings.TrimSpace(plan.ExpectedOutputPath) != "" {
		target = strings.TrimSpace(plan.ExpectedOutputPath)
	}

	candidates := make([]string, 0, 8)
	leadHints := make([]string, 0, 8)
	if status.Contracts != nil {
		if status.Contracts.ModelAcquisition != nil {
			for _, candidate := range status.Contracts.ModelAcquisition.PassiveLeads {
				candidates = append(candidates, strings.TrimSpace(candidate.Path))
			}
			for _, candidate := range status.Contracts.ModelAcquisition.ReadyArtifacts {
				candidates = append(candidates, strings.TrimSpace(candidate.Path))
			}
			leadHints = append(leadHints, status.Contracts.ModelAcquisition.AcquisitionLeads...)
		}
		for _, candidate := range status.Contracts.ModelCandidates {
			candidates = append(candidates, strings.TrimSpace(candidate.Path))
		}
	}
	candidates = ensureValuePresent(uniqueSortedStrings(candidates), target)
	candidates = truncateCandidatePaths(candidates, target, maxRepoContextModelCandidates)
	leadHints = truncateRepoContextValues(uniqueSortedStrings(leadHints), maxRepoContextModelCandidates)

	if plan != nil {
		constraints := []string{
			fmt.Sprintf("Execute the selected acquisition strategy %q instead of rediscovering how this repository should obtain its model.", strings.TrimSpace(plan.Strategy)),
		}
		if len(plan.RuntimeInvocation) > 0 {
			constraints = append(constraints, fmt.Sprintf("Run the planned repository invocation: %s", strings.Join(plan.RuntimeInvocation, " ")))
		}
		if workingDir := strings.TrimSpace(plan.WorkingDir); workingDir != "" {
			constraints = append(constraints, fmt.Sprintf("Run the planned invocation from %q.", workingDir))
		}
		if target != "" {
			constraints = append(constraints, fmt.Sprintf("Expected supported artifact output: %q.", target))
		}
		if helper := strings.TrimSpace(plan.HelperPath); helper != "" {
			constraints = append(constraints, fmt.Sprintf("If a helper script is required, use %q.", helper))
		}
		if plan.RequiresNetwork {
			constraints = append(constraints, "This strategy may require network access; if the current environment cannot reach the network, stop and report that blocker instead of inventing a different strategy.")
		}
		for _, detail := range modelAcquisitionPlanConstraintEvidence(plan) {
			constraints = append(constraints, fmt.Sprintf("Plan evidence: %s", detail))
		}
		return core.AuthoringRecommendation{
			StepID:      core.EnsureStepModelAcquisition,
			Target:      target,
			Rationale:   "execute_selected_model_acquisition_strategy",
			Candidates:  []string{target},
			Constraints: constraints,
		}, nil
	}

	constraints := []string{
		fmt.Sprintf("Prefer existing repository commands or Python entrypoints to materialize %q.", target),
		"If no single runnable path exists, create a temporary helper only under .concierge/materializers.",
		fmt.Sprintf("If a helper is required, use %q or another path under .concierge/materializers.", helperPath),
		"Materialize the final supported artifact under .concierge/materialized_models unless repository evidence proves a stable repo-local output path already exists.",
		"Do not modify unrelated training/business logic or commit model binaries.",
		"Model binaries are uploaded separately by leap CLI; do not rely on Tensorleap rerunning model acquisition on the server.",
		"If repository-local export or model imports fail under the prepared runtime, treat that export path as unavailable in the current repo state instead of debugging package imports or mutating the environment.",
		"If repository evidence includes a direct supported .onnx/.h5 artifact or a documented public example artifact, prefer materializing that direct artifact over exporting from unsupported weight files.",
	}
	if len(leadHints) > 0 {
		constraints = append(constraints, fmt.Sprintf("Inspect and reuse repository model acquisition leads before inventing helpers: %s", strings.Join(leadHints, ", ")))
	}

	return core.AuthoringRecommendation{
		StepID:      core.EnsureStepModelAcquisition,
		Target:      target,
		Rationale:   "materialize one supported .onnx/.h5 artifact before wiring @tensorleap_load_model",
		Candidates:  candidates,
		Constraints: constraints,
	}, nil
}

func defaultMaterializedModelPath(status core.IntegrationStatus) string {
	baseName := defaultMaterializedModelName
	if status.Contracts != nil && status.Contracts.ModelAcquisition != nil {
		for _, lead := range status.Contracts.ModelAcquisition.PassiveLeads {
			name := strings.TrimSpace(filepath.Base(lead.Path))
			if name == "" {
				continue
			}
			ext := filepath.Ext(name)
			base := strings.TrimSpace(strings.TrimSuffix(name, ext))
			if base == "" {
				continue
			}
			baseName = base + ".onnx"
			break
		}
	}
	return filepath.ToSlash(filepath.Join(".concierge", "materialized_models", baseName))
}
