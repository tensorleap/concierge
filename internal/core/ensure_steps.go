package core

import "strings"

// EnsureStepID is a stable machine-readable orchestration step identifier.
type EnsureStepID string

const (
	EnsureStepComplete                EnsureStepID = "ensure.complete"
	EnsureStepInvestigate             EnsureStepID = "ensure.investigate"
	EnsureStepRepositoryContext       EnsureStepID = "ensure.repository_context"
	EnsureStepPythonRuntime           EnsureStepID = "ensure.python_runtime"
	EnsureStepLeapCLIAuth             EnsureStepID = "ensure.leap_cli_auth"
	EnsureStepServerConnectivity      EnsureStepID = "ensure.server_connectivity"
	EnsureStepSecretsContext          EnsureStepID = "ensure.secrets_context"
	EnsureStepLeapYAML                EnsureStepID = "ensure.leap_yaml"
	EnsureStepModelContract           EnsureStepID = "ensure.model_contract"
	EnsureStepIntegrationScript       EnsureStepID = "ensure.integration_script"
	EnsureStepPreprocessContract      EnsureStepID = "ensure.preprocess_contract"
	EnsureStepInputEncoders           EnsureStepID = "ensure.input_encoders"
	EnsureStepGroundTruthEncoders     EnsureStepID = "ensure.ground_truth_encoders"
	EnsureStepIntegrationTestContract EnsureStepID = "ensure.integration_test_contract"
	EnsureStepOptionalHooks           EnsureStepID = "ensure.optional_hooks"
	EnsureStepHarnessValidation       EnsureStepID = "ensure.harness_validation"
	EnsureStepUploadReadiness         EnsureStepID = "ensure.upload_readiness"
	EnsureStepUploadPush              EnsureStepID = "ensure.upload_push"
)

var ensureStepCatalog = map[EnsureStepID]EnsureStep{
	EnsureStepComplete: {
		ID:          EnsureStepComplete,
		Description: "No additional ensure-step is required; integration state is complete",
	},
	EnsureStepInvestigate: {
		ID:          EnsureStepInvestigate,
		Description: "Investigate unmapped issues and enrich planner rules",
	},
	EnsureStepRepositoryContext: {
		ID:          EnsureStepRepositoryContext,
		Description: "Ensure repository context is valid (git/project root/branch/worktree)",
	},
	EnsureStepPythonRuntime: {
		ID:          EnsureStepPythonRuntime,
		Description: "Ensure Python runtime and dependency prerequisites",
	},
	EnsureStepLeapCLIAuth: {
		ID:          EnsureStepLeapCLIAuth,
		Description: "Ensure Tensorleap CLI availability and authentication",
	},
	EnsureStepServerConnectivity: {
		ID:          EnsureStepServerConnectivity,
		Description: "Ensure Tensorleap server connectivity and server info checks",
	},
	EnsureStepSecretsContext: {
		ID:          EnsureStepSecretsContext,
		Description: "Ensure required secrets context is configured",
	},
	EnsureStepLeapYAML: {
		ID:          EnsureStepLeapYAML,
		Description: "Ensure leap.yaml exists and satisfies upload boundary contract",
	},
	EnsureStepModelContract: {
		ID:          EnsureStepModelContract,
		Description: "Ensure model artifact format and shape contracts",
	},
	EnsureStepIntegrationScript: {
		ID:          EnsureStepIntegrationScript,
		Description: "Ensure integration script exists and can be imported",
	},
	EnsureStepPreprocessContract: {
		ID:          EnsureStepPreprocessContract,
		Description: "Ensure preprocess function contract and dataset subset requirements",
	},
	EnsureStepInputEncoders: {
		ID:          EnsureStepInputEncoders,
		Description: "Ensure input encoders exist and execute with valid outputs",
	},
	EnsureStepGroundTruthEncoders: {
		ID:          EnsureStepGroundTruthEncoders,
		Description: "Ensure ground-truth encoders exist and execute on labeled subsets",
	},
	EnsureStepIntegrationTestContract: {
		ID:          EnsureStepIntegrationTestContract,
		Description: "Ensure integration test wiring and decorator-call contract",
	},
	EnsureStepOptionalHooks: {
		ID:          EnsureStepOptionalHooks,
		Description: "Ensure optional metadata/visualizer/metric/loss hooks are valid",
	},
	EnsureStepHarnessValidation: {
		ID:          EnsureStepHarnessValidation,
		Description: "Ensure Concierge harness and anti-stub validation checks pass",
	},
	EnsureStepUploadReadiness: {
		ID:          EnsureStepUploadReadiness,
		Description: "Ensure upload readiness including mounts, assets, and confirmation gates",
	},
	EnsureStepUploadPush: {
		ID:          EnsureStepUploadPush,
		Description: "Perform and validate leap push",
	},
}

var ensureStepPriority = []EnsureStepID{
	EnsureStepRepositoryContext,
	EnsureStepPythonRuntime,
	EnsureStepLeapCLIAuth,
	EnsureStepServerConnectivity,
	EnsureStepSecretsContext,
	EnsureStepLeapYAML,
	EnsureStepModelContract,
	EnsureStepIntegrationScript,
	EnsureStepPreprocessContract,
	EnsureStepInputEncoders,
	EnsureStepGroundTruthEncoders,
	EnsureStepIntegrationTestContract,
	EnsureStepOptionalHooks,
	EnsureStepHarnessValidation,
	EnsureStepUploadReadiness,
	EnsureStepUploadPush,
	EnsureStepInvestigate,
}

// EnsureStepByID returns the catalog entry for a known step ID.
func EnsureStepByID(id EnsureStepID) (EnsureStep, bool) {
	step, ok := ensureStepCatalog[id]
	return step, ok
}

// KnownEnsureSteps returns the canonical ensure-step catalog in planner priority order.
func KnownEnsureSteps() []EnsureStep {
	steps := make([]EnsureStep, 0, len(ensureStepPriority))
	for _, id := range ensureStepPriority {
		step, ok := ensureStepCatalog[id]
		if !ok {
			continue
		}
		steps = append(steps, step)
	}
	return steps
}

// HumanEnsureStepLabel returns user-facing wording for one ensure-step.
func HumanEnsureStepLabel(stepID EnsureStepID) string {
	switch stepID {
	case EnsureStepComplete:
		return "Integration complete"
	case EnsureStepRepositoryContext:
		return "Check repository setup"
	case EnsureStepPythonRuntime:
		return "Check Python setup"
	case EnsureStepLeapCLIAuth:
		return "Check Leap CLI installation and login"
	case EnsureStepServerConnectivity:
		return "Check Tensorleap server connection"
	case EnsureStepSecretsContext:
		return "Check required secrets"
	case EnsureStepLeapYAML:
		return "Check leap.yaml setup"
	case EnsureStepModelContract:
		return "Check model compatibility"
	case EnsureStepIntegrationScript:
		return "Check integration script setup"
	case EnsureStepPreprocessContract:
		return "Check preprocess setup"
	case EnsureStepInputEncoders:
		return "Check input encoders"
	case EnsureStepGroundTruthEncoders:
		return "Check ground-truth encoders"
	case EnsureStepIntegrationTestContract:
		return "Check integration test wiring"
	case EnsureStepOptionalHooks:
		return "Check optional integration hooks"
	case EnsureStepHarnessValidation:
		return "Run runtime checks"
	case EnsureStepUploadReadiness:
		return "Check upload readiness"
	case EnsureStepUploadPush:
		return "Upload integration to Tensorleap"
	case EnsureStepInvestigate:
		return "Investigate remaining issues"
	default:
		label := strings.TrimPrefix(string(stepID), "ensure.")
		label = strings.ReplaceAll(label, "_", " ")
		label = strings.TrimSpace(label)
		if label == "" {
			return "Run the next planned step"
		}
		return strings.ToUpper(label[:1]) + label[1:]
	}
}
