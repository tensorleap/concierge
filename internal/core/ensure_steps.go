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
	EnsureStepModelAcquisition        EnsureStepID = "ensure.model_acquisition"
	EnsureStepModelContract           EnsureStepID = "ensure.model_contract"
	EnsureStepIntegrationScript       EnsureStepID = "ensure.integration_script"
	EnsureStepPreprocessContract      EnsureStepID = "ensure.preprocess_contract"
	EnsureStepInputEncoders           EnsureStepID = "ensure.input_encoders"
	EnsureStepGroundTruthEncoders     EnsureStepID = "ensure.ground_truth_encoders"
	EnsureStepIntegrationTestContract EnsureStepID = "ensure.integration_test_contract"
	EnsureStepIntegrationTestWiring   EnsureStepID = "ensure.integration_test_wiring"
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
		Description: "Ensure Poetry runtime resolution and dependency prerequisites",
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
	EnsureStepModelAcquisition: {
		ID:          EnsureStepModelAcquisition,
		Description: "Ensure a Tensorleap-compatible model artifact can be materialized locally",
	},
	EnsureStepModelContract: {
		ID:          EnsureStepModelContract,
		Description: "Ensure model artifact format and shape contracts",
	},
	EnsureStepIntegrationScript: {
		ID:          EnsureStepIntegrationScript,
		Description: "Ensure root leap_integration.py exists and is the canonical entrypoint",
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
		Description: "Ensure integration test scaffold and __main__ block are present",
	},
	EnsureStepIntegrationTestWiring: {
		ID:          EnsureStepIntegrationTestWiring,
		Description: "Ensure integration test decorator calls are correctly wired",
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
	EnsureStepSecretsContext,
	EnsureStepLeapYAML,
	EnsureStepIntegrationScript,
	EnsureStepPreprocessContract,
	EnsureStepInputEncoders,
	EnsureStepModelAcquisition,
	EnsureStepModelContract,
	EnsureStepIntegrationTestContract,
	EnsureStepGroundTruthEncoders,
	EnsureStepIntegrationTestWiring,
	EnsureStepHarnessValidation,
	EnsureStepServerConnectivity,
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
		return "Integration is complete"
	case EnsureStepRepositoryContext:
		return "Repository context is ready"
	case EnsureStepPythonRuntime:
		return "Poetry environment is available and has the required packages"
	case EnsureStepLeapCLIAuth:
		return "Leap CLI is installed and authenticated"
	case EnsureStepServerConnectivity:
		return "Tensorleap server is reachable"
	case EnsureStepSecretsContext:
		return "Required secrets are configured"
	case EnsureStepLeapYAML:
		return "leap.yaml is present and valid"
	case EnsureStepModelAcquisition:
		return "A Tensorleap-compatible model artifact can be materialized locally"
	case EnsureStepModelContract:
		return "@tensorleap_load_model is wired to a supported model artifact"
	case EnsureStepIntegrationScript:
		return "Root leap_integration.py is present and canonical"
	case EnsureStepPreprocessContract:
		return "Dataset preprocessing is configured"
	case EnsureStepInputEncoders:
		return "Input encoders run successfully"
	case EnsureStepGroundTruthEncoders:
		return "Ground-truth encoders run successfully"
	case EnsureStepIntegrationTestContract:
		return "Integration test scaffold is present"
	case EnsureStepIntegrationTestWiring:
		return "Integration test wiring is complete"
	case EnsureStepHarnessValidation:
		return "Runtime validation checks pass"
	case EnsureStepUploadReadiness:
		return "Upload prerequisites are satisfied"
	case EnsureStepUploadPush:
		return "Integration is uploaded to Tensorleap"
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

// HumanEnsureStepRequirementLabel returns requirement wording for checks that need attention.
func HumanEnsureStepRequirementLabel(stepID EnsureStepID) string {
	switch stepID {
	case EnsureStepComplete:
		return "Integration should be complete"
	case EnsureStepRepositoryContext:
		return "Repository context should be ready"
	case EnsureStepPythonRuntime:
		return "Poetry environment should be available and have the required packages"
	case EnsureStepLeapCLIAuth:
		return "Leap CLI should be installed and authenticated"
	case EnsureStepServerConnectivity:
		return "Tensorleap server should be reachable"
	case EnsureStepSecretsContext:
		return "Required secrets should be configured"
	case EnsureStepLeapYAML:
		return "leap.yaml should be present and valid"
	case EnsureStepModelAcquisition:
		return "A Tensorleap-compatible model artifact should be materialized locally"
	case EnsureStepModelContract:
		return "@tensorleap_load_model should be wired to a supported model artifact"
	case EnsureStepIntegrationScript:
		return "Root leap_integration.py should be present and canonical"
	case EnsureStepPreprocessContract:
		return "Dataset preprocessing should be configured"
	case EnsureStepInputEncoders:
		return "Input encoders should run successfully"
	case EnsureStepGroundTruthEncoders:
		return "Ground-truth encoders should run successfully"
	case EnsureStepIntegrationTestContract:
		return "Integration test scaffold should be present"
	case EnsureStepIntegrationTestWiring:
		return "Integration test wiring should be complete"
	case EnsureStepHarnessValidation:
		return "Runtime validation checks should pass"
	case EnsureStepUploadReadiness:
		return "Upload prerequisites should be satisfied"
	case EnsureStepUploadPush:
		return "Integration should be uploaded to Tensorleap"
	case EnsureStepInvestigate:
		return "Remaining issues should be investigated"
	default:
		label := strings.TrimPrefix(string(stepID), "ensure.")
		label = strings.ReplaceAll(label, "_", " ")
		label = strings.TrimSpace(label)
		if label == "" {
			return "The next planned step should be completed"
		}
		return strings.ToUpper(label[:1]) + label[1:]
	}
}
