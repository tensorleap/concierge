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
	trimmedDiff := strings.TrimSpace(diffSummary)
	if trimmedDiff == "" {
		trimmedDiff = "(no diff summary available)"
	}
	return fmt.Sprintf("Approve commit for %s?\n%s", step.ID, trimmedDiff)
}
