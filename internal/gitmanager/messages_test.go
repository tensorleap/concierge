package gitmanager

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestPreprocessAuthoringReviewFocusHighlightsSubsetRequirement(t *testing.T) {
	step, ok := core.EnsureStepByID(core.EnsureStepPreprocessContract)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepPreprocessContract)
	}

	focus := strings.ToLower(ReviewFocus(step))
	if !strings.Contains(focus, "train") || !strings.Contains(focus, "validation") {
		t.Fatalf("expected preprocess focus to mention train and validation subsets, got %q", focus)
	}
}
