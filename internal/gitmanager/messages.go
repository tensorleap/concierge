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

// ChangeReview contains the user-facing review payload before final confirmation.
type ChangeReview struct {
	Focus string
	Files []string
	Stat  string
	Patch string
}

// ReviewFocus returns user-facing wording for what Concierge is fixing.
func ReviewFocus(step core.EnsureStep) string {
	switch step.ID {
	case core.EnsureStepPreprocessContract:
		return "Implement preprocess with train and validation subsets"
	case core.EnsureStepGroundTruthEncoders:
		return "Implement ground-truth encoders for labeled subsets only"
	}
	focus := strings.TrimSpace(core.HumanEnsureStepRequirementLabel(step.ID))
	if focus == "" {
		focus = strings.TrimSpace(step.Description)
	}
	if focus == "" {
		focus = "Apply the planned change"
	}
	return focus
}
