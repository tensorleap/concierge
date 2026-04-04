package observe

import "github.com/tensorleap/concierge/internal/core"

// StepDisplayStatus tracks the visual state of a step in the split-screen panel.
type StepDisplayStatus string

const (
	StepPending    StepDisplayStatus = "pending"
	StepInProgress StepDisplayStatus = "in_progress"
	StepPass       StepDisplayStatus = "pass"
	StepWarning    StepDisplayStatus = "warning"
	StepFail       StepDisplayStatus = "fail"
)

// StepGroup defines a labelled group of ensure steps for the panel checklist.
type StepGroup struct {
	Label string
	Steps []core.EnsureStepID
}

// ChecklistGroups defines the visible step groups shown in the split-screen panel.
// Steps filtered by shouldRenderCheckStep (Complete, Investigate, UploadReadiness, UploadPush)
// are excluded.
var ChecklistGroups = []StepGroup{
	{
		Label: "Environment & Infrastructure",
		Steps: []core.EnsureStepID{
			core.EnsureStepRepositoryContext,
			core.EnsureStepPythonRuntime,
			core.EnsureStepLeapCLIAuth,
			core.EnsureStepSecretsContext,
		},
	},
	{
		Label: "Integration Scaffolding",
		Steps: []core.EnsureStepID{
			core.EnsureStepLeapYAML,
			core.EnsureStepIntegrationScript,
			core.EnsureStepIntegrationTestContract,
		},
	},
	{
		Label: "Data Pipeline",
		Steps: []core.EnsureStepID{
			core.EnsureStepPreprocessContract,
			core.EnsureStepInputEncoders,
			core.EnsureStepGroundTruthEncoders,
		},
	},
	{
		Label: "Model",
		Steps: []core.EnsureStepID{
			core.EnsureStepModelAcquisition,
			core.EnsureStepModelContract,
		},
	},
	{
		Label: "Validation",
		Steps: []core.EnsureStepID{
			core.EnsureStepHarnessValidation,
		},
	},
	{
		Label: "Connectivity",
		Steps: []core.EnsureStepID{
			core.EnsureStepServerConnectivity,
		},
	},
}

// ShortStepLabel returns a concise label for a step (used by panels and status bars).
func ShortStepLabel(stepID core.EnsureStepID) string {
	switch stepID {
	case core.EnsureStepRepositoryContext:
		return "Repository context"
	case core.EnsureStepPythonRuntime:
		return "Python / Poetry"
	case core.EnsureStepLeapCLIAuth:
		return "Leap CLI auth"
	case core.EnsureStepSecretsContext:
		return "Secrets"
	case core.EnsureStepLeapYAML:
		return "leap.yaml"
	case core.EnsureStepIntegrationScript:
		return "leap_integration.py"
	case core.EnsureStepIntegrationTestContract:
		return "Test scaffold"
	case core.EnsureStepPreprocessContract:
		return "Preprocess"
	case core.EnsureStepInputEncoders:
		return "Input encoders"
	case core.EnsureStepGroundTruthEncoders:
		return "GT encoders"
	case core.EnsureStepModelAcquisition:
		return "Model acquisition"
	case core.EnsureStepModelContract:
		return "Model contract"
	case core.EnsureStepHarnessValidation:
		return "Harness validation"
	case core.EnsureStepServerConnectivity:
		return "Server connectivity"
	default:
		return string(stepID)
	}
}

// CheckStatusToDisplay maps a core.CheckStatus to a StepDisplayStatus.
func CheckStatusToDisplay(status core.CheckStatus) StepDisplayStatus {
	switch status {
	case core.CheckStatusPass:
		return StepPass
	case core.CheckStatusWarning:
		return StepWarning
	case core.CheckStatusFail:
		return StepFail
	default:
		return StepPending
	}
}
