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

func TestGTEncoderReviewFocusMentionsGroundTruthTargets(t *testing.T) {
	step, ok := core.EnsureStepByID(core.EnsureStepGroundTruthEncoders)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepGroundTruthEncoders)
	}

	focus := strings.ToLower(ReviewFocus(step))
	if !strings.Contains(focus, "ground-truth") {
		t.Fatalf("expected GT focus to mention ground-truth context, got %q", focus)
	}
	if !strings.Contains(focus, "labeled") {
		t.Fatalf("expected GT focus to mention labeled-subset rule, got %q", focus)
	}
}
