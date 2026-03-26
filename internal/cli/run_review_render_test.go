package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/gitmanager"
)

func TestPromptChangeReviewApprovalRendersUserFacingReview(t *testing.T) {
	step, ok := core.EnsureStepByID(core.EnsureStepLeapYAML)
	if !ok {
		t.Fatal("expected leap.yaml ensure-step in catalog")
	}

	review := gitmanager.ChangeReview{
		Focus: "leap.yaml should be present and valid",
		Files: []string{"A\tleap.yaml", "A\tleap_integration.py"},
		Stat:  " leap.yaml      | 10 ++++++++++\n leap_integration.py | 20 ++++++++++++++++++++\n 2 files changed, 30 insertions(+)",
		Patch: "diff --git a/leap.yaml b/leap.yaml\nnew file mode 100644\nindex 0000000..1111111\n--- /dev/null\n+++ b/leap.yaml\n@@ -0,0 +1 @@\n+entryFile: leap_integration.py",
	}

	output := new(bytes.Buffer)
	decision, err := promptChangeReviewApproval(
		bytes.NewBufferString("n\n"),
		output,
		step,
		review,
		changeReviewRenderOptions{EnableColor: false},
	)
	if err != nil {
		t.Fatalf("promptChangeReviewApproval returned error: %v", err)
	}
	if decision.KeepChanges {
		t.Fatal("expected keep=false for explicit no response")
	}

	rendered := output.String()
	assertContains(t, rendered, "Proposed Changes")
	assertContains(t, rendered, "Fixing: leap.yaml should be present and valid")
	assertContains(t, rendered, "Files changed:")
	assertContains(t, rendered, "Diff summary:")
	assertContains(t, rendered, "Patch:")
	assertContains(t, rendered, "What should I do with these reviewed changes? [y] commit / [K] keep for local review / [n] restore:")
	assertNotContains(t, rendered, "Keep these changes in your working tree for local review? [Y/n]:")
	assertNotContains(t, rendered, "Create a commit for these reviewed changes now? [y/N]:")
	if strings.Contains(rendered, "Step:") {
		t.Fatalf("expected internal Step label to be omitted, got: %q", rendered)
	}
}

func TestPromptChangeReviewApprovalDefaultsKeepWithoutCommit(t *testing.T) {
	step, ok := core.EnsureStepByID(core.EnsureStepLeapYAML)
	if !ok {
		t.Fatal("expected leap.yaml ensure-step in catalog")
	}

	output := new(bytes.Buffer)
	decision, err := promptChangeReviewApproval(
		bytes.NewBufferString("\n"),
		output,
		step,
		gitmanager.ChangeReview{Focus: "leap.yaml should be present and valid"},
		changeReviewRenderOptions{EnableColor: false},
	)
	if err != nil {
		t.Fatalf("promptChangeReviewApproval returned error: %v", err)
	}
	if !decision.KeepChanges {
		t.Fatal("expected empty keep input to default to yes")
	}
	if decision.Commit {
		t.Fatal("expected empty commit input to default to no")
	}
}

func TestPromptChangeReviewApprovalAllowsCommitImmediately(t *testing.T) {
	step, ok := core.EnsureStepByID(core.EnsureStepLeapYAML)
	if !ok {
		t.Fatal("expected leap.yaml ensure-step in catalog")
	}

	output := new(bytes.Buffer)
	decision, err := promptChangeReviewApproval(
		bytes.NewBufferString("y\n"),
		output,
		step,
		gitmanager.ChangeReview{Focus: "leap.yaml should be present and valid"},
		changeReviewRenderOptions{EnableColor: false},
	)
	if err != nil {
		t.Fatalf("promptChangeReviewApproval returned error: %v", err)
	}
	if !decision.KeepChanges {
		t.Fatal("expected commit choice to keep changes")
	}
	if !decision.Commit {
		t.Fatal("expected explicit yes response to commit immediately")
	}
}

func TestPromptChangeReviewApprovalWarnsAndDefaultsNoForRiskyArtifactDiff(t *testing.T) {
	step, ok := core.EnsureStepByID(core.EnsureStepPreprocessContract)
	if !ok {
		t.Fatal("expected preprocess ensure-step in catalog")
	}

	output := new(bytes.Buffer)
	decision, err := promptChangeReviewApproval(
		bytes.NewBufferString("\n"),
		output,
		step,
		gitmanager.ChangeReview{
			Focus: "Implement preprocess with train and validation subsets",
			Files: []string{
				"A\tleap_integration.py",
				"A\t.tensorleap_data/coco8/images/train/000000000009.jpg",
			},
			Stat:  " leap_integration.py | 12 ++++++++++++\n .tensorleap_data/coco8/images/train/000000000009.jpg | Bin 0 -> 45073 bytes\n 2 files changed, 12 insertions(+)",
			Patch: "diff --git a/leap_integration.py b/leap_integration.py\n...",
			Risk: gitmanager.ChangeReviewRisk{
				Level:     "high",
				Summary:   "This diff includes dataset/cache artifacts that should usually stay out of the repository working tree.",
				Reasons:   []string{"dataset/cache paths were added", "binary artifact files were added"},
				HidePatch: true,
			},
		},
		changeReviewRenderOptions{EnableColor: false},
	)
	if err != nil {
		t.Fatalf("promptChangeReviewApproval returned error: %v", err)
	}
	if decision.KeepChanges {
		t.Fatal("expected empty keep input to default to no for risky diff")
	}

	rendered := output.String()
	assertContains(t, rendered, "Risk warning:")
	assertContains(t, rendered, "dataset/cache artifacts")
	assertContains(t, rendered, "binary artifact files were added")
	assertContains(t, rendered, "Patch omitted because this diff is dominated by risky artifact changes.")
	assertContains(t, rendered, "What should I do with these risky reviewed changes? [y] commit / [k] keep / [N] restore:")
	assertNotContains(t, rendered, "Patch:")
}

func assertContains(t *testing.T, output string, expected string) {
	t.Helper()
	if !strings.Contains(output, expected) {
		t.Fatalf("expected output to contain %q, got: %q", expected, output)
	}
}

func assertNotContains(t *testing.T, output string, expected string) {
	t.Helper()
	if strings.Contains(output, expected) {
		t.Fatalf("did not expect output to contain %q, got: %q", expected, output)
	}
}
