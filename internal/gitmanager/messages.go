package gitmanager

import (
	"fmt"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

// CommitMessage returns the structured commit subject for one ensure-step.
func CommitMessage(step core.EnsureStep, summary string) string {
	normalizedSummary := strings.TrimSpace(summary)
	if normalizedSummary == "" {
		normalizedSummary = "apply deterministic changes"
	}
	return fmt.Sprintf("concierge(%s): %s", step.ID, normalizedSummary)
}

// ApprovalMessage is the prompt text shown before commit approval.
func ApprovalMessage(step core.EnsureStep, diffSummary string) string {
	stepSummary := strings.TrimSpace(step.Description)
	if stepSummary == "" {
		stepSummary = "Apply the planned change"
	}

	trimmedDiff := strings.TrimSpace(diffSummary)
	if trimmedDiff == "" {
		trimmedDiff = "No diff summary is available."
	}

	return fmt.Sprintf(
		"Review proposed changes\nStep: %s\nDiff summary:\n%s\nCreate a commit for these changes?",
		stepSummary,
		trimmedDiff,
	)
}
