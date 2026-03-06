package observe

import (
	"fmt"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func stageLabel(stage core.Stage) string {
	switch stage {
	case core.StageSnapshot:
		return "Checking workspace"
	case core.StageInspect:
		return "Inspecting Tensorleap artifacts"
	case core.StagePlan:
		return "Choosing the next fix"
	case core.StageExecute:
		return "Applying the selected fix"
	case core.StageValidate:
		return "Validating runtime behavior"
	case core.StageReport:
		return "Writing the run report"
	default:
		label := strings.TrimSpace(string(stage))
		if label == "" {
			return "Working"
		}
		return label
	}
}

func stepLabel(stepID core.EnsureStepID) string {
	if stepID == "" {
		return "the next step"
	}
	return core.HumanEnsureStepLabel(stepID)
}

func toolHeadline(toolName, detail string) string {
	name := strings.ToLower(strings.TrimSpace(toolName))
	detail = strings.TrimSpace(detail)
	switch name {
	case "read", "grep", "glob", "ls":
		if detail != "" {
			return fmt.Sprintf("Scanning repository code: %s", detail)
		}
		return "Scanning repository code"
	case "bash":
		if detail != "" {
			return fmt.Sprintf("Running repo check: %s", detail)
		}
		return "Running a repo check"
	case "edit", "multiedit", "write", "notebookedit":
		if detail != "" {
			return fmt.Sprintf("Editing %s", detail)
		}
		return "Editing repository files"
	default:
		if detail != "" {
			return fmt.Sprintf("Claude is using %s: %s", toolName, detail)
		}
		if toolName != "" {
			return fmt.Sprintf("Claude is using %s", toolName)
		}
		return "Claude is working"
	}
}
