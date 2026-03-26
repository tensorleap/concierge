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
	case core.EnsureStepModelAcquisition:
		return agent.AgentScopePolicy{
			AllowedFiles: append([]string{
				".concierge/materializers",
				".concierge/materialized_models",
			}, allowedFiles...),
			ForbiddenAreas: []string{
				"Do not modify leap_integration.py, @tensorleap_preprocess, @tensorleap_input_encoder, or @tensorleap_gt_encoder in this step",
				"Do not modify unrelated training/business logic",
				"Do not add durable helper scripts outside .concierge",
			},
			RequiredOutcomes: []string{
				"Materialize one concrete .onnx or .h5 artifact path that Concierge can use locally",
				"Prefer existing repository commands or entrypoints before creating a temporary helper",
			},
			StopAndAskTriggers: []string{
				"Repository evidence does not reveal any viable download/export path",
				"Materialization requires editing durable repository files outside .concierge",
			},
			DomainSections: []string{
				"load_model_contract",
			},
		}, nil
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
				"Do not inspect or depend on .concierge internal state in this step",
				"Do not install packages or mutate the Poetry/Python environment while discovering dataset paths for this step",
				"Do not probe global site-packages or system directories to infer dataset paths when repository helpers fail under the wrong interpreter",
			},
			RequiredOutcomes: []string{
				"Implement @tensorleap_preprocess with deterministic train and validation subset responses",
			},
			StopAndAskTriggers: []string{
				"The fix requires changing input encoders, ground-truth encoders, or integration-test wiring",
				"Repository evidence does not expose real train/validation identifiers and the only remaining option is guessed dataset paths or placeholder sample IDs",
				"The fix would hard-code installed package defaults, home-directory dataset paths, or new environment variables not already required by the repository instead of using a repo-supported dataset resolver",
				"The only remaining implementation path requires creating or writing to top-level absolute directories outside the repo/workspace, such as /datasets",
				"The only remaining implementation path would add vendored dataset/cache artifacts, extracted archives, or third-party dataset license/readme files to the repository working tree",
				"Making repo helper imports work would require pip install, poetry add, or other environment/package mutation just to inspect dataset logic",
				"The only concrete files available are generic repo assets, screenshots, or example images rather than an explicit dataset source",
				"The only remaining inspection path is probing global site-packages or system directories instead of the prepared repository runtime and manifest evidence",
			},
			DomainSections: []string{
				"preprocess_contract",
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
				"Implement missing @tensorleap_input_encoder functions for required input symbols using the exact Tensorleap symbol names",
				"Treat the first encoder argument as the Tensorleap sample_id matching PreprocessResponse.sample_id_type, not as a positional dataset index",
				"Keep encoder output shapes and dtypes stable across representative samples",
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
				"Treat the first GT encoder argument as the Tensorleap sample_id matching PreprocessResponse.sample_id_type, not as a positional dataset index",
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
	case core.EnsureStepIntegrationTestWiring:
		return agent.AgentScopePolicy{
			AllowedFiles: allowedFiles,
			ForbiddenAreas: []string{
				"Do not modify @tensorleap_preprocess subset semantics in this step",
				"Do not modify @tensorleap_input_encoder or @tensorleap_gt_encoder implementations in this step",
				"Do not modify unrelated training/business logic",
			},
			RequiredOutcomes: []string{
				"Repair @tensorleap_integration_test so required Tensorleap decorators are called",
				"Keep integration_test thin and declarative so mapping-mode re-execution succeeds",
			},
			StopAndAskTriggers: []string{
				"The repair requires moving logic outside leap_integration.py into unrelated project code",
				"The repair requires changing preprocess subset semantics or encoder implementations",
			},
			DomainSections: []string{
				"integration_test_wiring_contract",
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
	if plan := selectedModelAcquisitionPlan(snapshot, status); plan != nil {
		add(plan.ExpectedOutputPath)
		add(plan.HelperPath)
	}
	if status.Contracts != nil {
		add(status.Contracts.EntryFile)
		add(status.Contracts.ResolvedModelPath)
		modelCandidates := make([]string, 0, len(status.Contracts.ModelCandidates))
		for _, candidate := range status.Contracts.ModelCandidates {
			modelCandidates = append(modelCandidates, candidate.Path)
		}
		for _, candidatePath := range truncateCandidatePaths(modelCandidates, "", maxRepoContextModelCandidates) {
			add(candidatePath)
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
