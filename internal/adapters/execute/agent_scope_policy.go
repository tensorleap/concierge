package execute

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/agent"
	"github.com/tensorleap/concierge/internal/core"
)

var defaultAgentAllowedFiles = []string{
	"leap.yaml",
	core.CanonicalIntegrationEntryFile,
}

// PolicyForStep returns deterministic scope boundaries and domain rule slices for one ensure-step.
func PolicyForStep(step core.EnsureStepID, snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) (agent.AgentScopePolicy, error) {
	allowedFiles := resolveAgentAllowedFiles(snapshot, status)

	switch step {
	case core.EnsureStepModelContract:
		return agent.AgentScopePolicy{
			AllowedFiles: allowedFiles,
			ForbiddenAreas: []string{
				"Do not modify @tensorleap_preprocess definitions or subset semantics in this step",
				"Do not modify @tensorleap_input_encoder or @tensorleap_gt_encoder definitions in this step",
				"Do not modify unrelated training/business logic",
			},
			RequiredOutcomes: []string{
				"Resolve one concrete @tensorleap_load_model path pointing to a supported .onnx or .h5 artifact",
			},
			StopAndAskTriggers: []string{
				"No supported .onnx/.h5 candidates can be resolved from repository evidence",
				"Fix requires refactoring unrelated training/business logic",
			},
			DomainSections: []string{
				"load_model_contract",
			},
		}, nil
	case core.EnsureStepPreprocessContract:
		return agent.AgentScopePolicy{
			AllowedFiles: allowedFiles,
			ForbiddenAreas: []string{
				"Do not modify @tensorleap_input_encoder definitions or registrations",
				"Do not modify @tensorleap_gt_encoder definitions or registrations",
				"Do not rewire @tensorleap_integration_test calls in this step",
			},
			RequiredOutcomes: []string{
				"Implement @tensorleap_preprocess with deterministic train and validation subset responses",
				"Implement @tensorleap_load_model wiring with a supported .onnx or .h5 artifact path",
			},
			StopAndAskTriggers: []string{
				"Repository evidence does not identify a model artifact path for @tensorleap_load_model",
				"The fix requires changing input encoders, ground-truth encoders, or integration-test wiring",
			},
			DomainSections: []string{
				"preprocess_contract",
				"load_model_contract",
			},
		}, nil
	case core.EnsureStepInputEncoders:
		return agent.AgentScopePolicy{
			AllowedFiles: allowedFiles,
			ForbiddenAreas: []string{
				"Do not modify @tensorleap_preprocess definitions or subset semantics",
				"Do not modify @tensorleap_gt_encoder definitions or registrations",
				"Do not rewire @tensorleap_integration_test calls in this step",
			},
			RequiredOutcomes: []string{
				"Implement missing @tensorleap_input_encoder functions for required input symbols",
				"Keep encoder output shapes and dtypes stable across multiple sample indices",
			},
			StopAndAskTriggers: []string{
				"Required input symbols or signatures cannot be resolved from repository evidence",
				"The fix requires changing ground-truth encoders or integration-test wiring",
			},
			DomainSections: []string{
				"input_encoder_contract",
			},
		}, nil
	case core.EnsureStepGroundTruthEncoders:
		return agent.AgentScopePolicy{
			AllowedFiles: allowedFiles,
			ForbiddenAreas: []string{
				"Do not modify @tensorleap_preprocess definitions or subset semantics",
				"Do not modify @tensorleap_input_encoder definitions or registrations",
				"Do not rewire @tensorleap_integration_test calls in this step",
			},
			RequiredOutcomes: []string{
				"Implement missing @tensorleap_gt_encoder functions for required target symbols",
				"Ensure GT encoder behavior remains limited to labeled subsets",
			},
			StopAndAskTriggers: []string{
				"Required target symbols or subset-label semantics cannot be resolved from repository evidence",
				"The fix requires changing input encoders or integration-test wiring",
			},
			DomainSections: []string{
				"ground_truth_encoder_contract",
			},
		}, nil
	case core.EnsureStepHarnessValidation:
		return agent.AgentScopePolicy{
			AllowedFiles: allowedFiles,
			ForbiddenAreas: []string{
				"Do not edit unrelated training/business logic outside Tensorleap integration paths",
			},
			RequiredOutcomes: []string{
				"Resolve reported harness or anti-stub integration contract failures",
			},
			StopAndAskTriggers: []string{
				"Fix requires broad refactors outside Tensorleap integration artifacts",
			},
			DomainSections: []string{
				"preprocess_contract",
				"input_encoder_contract",
				"ground_truth_encoder_contract",
				"integration_test_wiring_contract",
				"load_model_contract",
			},
		}, nil
	case core.EnsureStepInvestigate:
		return agent.AgentScopePolicy{
			AllowedFiles: allowedFiles,
			ForbiddenAreas: []string{
				"Do not change unrelated repository logic while investigating integration blockers",
			},
			RequiredOutcomes: []string{
				"Produce a minimal, deterministic fix or clearly identify the remaining blocker",
			},
			StopAndAskTriggers: []string{
				"Investigation requires edits outside Tensorleap integration artifacts",
			},
			DomainSections: []string{
				"leap_yaml_contract",
				"preprocess_contract",
				"input_encoder_contract",
				"ground_truth_encoder_contract",
				"integration_test_wiring_contract",
				"load_model_contract",
			},
		}, nil
	default:
		return agent.AgentScopePolicy{}, core.WrapError(
			core.KindStepNotApplicable,
			"execute.agent.scope_policy",
			fmt.Errorf("cannot resolve scope policy for ensure-step %q", step),
		)
	}
}

func resolveAgentAllowedFiles(snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) []string {
	unique := map[string]struct{}{}
	add := func(path string) {
		normalized := normalizeAgentScopePath(path)
		if normalized == "" {
			return
		}
		unique[normalized] = struct{}{}
	}

	for _, path := range defaultAgentAllowedFiles {
		add(path)
	}

	add(snapshot.SelectedModelPath)
	if status.Contracts != nil {
		add(status.Contracts.EntryFile)
		add(status.Contracts.ResolvedModelPath)
		for _, candidate := range status.Contracts.ModelCandidates {
			add(candidate.Path)
		}
	}

	allowed := make([]string, 0, len(unique))
	for path := range unique {
		allowed = append(allowed, path)
	}
	sort.Strings(allowed)
	return allowed
}

func normalizeAgentScopePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	cleaned := filepath.ToSlash(filepath.Clean(trimmed))
	if cleaned == "." || cleaned == "/" || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return ""
	}
	return cleaned
}
