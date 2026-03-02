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
		Files: []string{"A\tleap.yaml", "A\tleap_binder.py"},
		Stat:  " leap.yaml      | 10 ++++++++++\n leap_binder.py | 20 ++++++++++++++++++++\n 2 files changed, 30 insertions(+)",
		Patch: "diff --git a/leap.yaml b/leap.yaml\nnew file mode 100644\nindex 0000000..1111111\n--- /dev/null\n+++ b/leap.yaml\n@@ -0,0 +1 @@\n+entryFile: leap_binder.py",
	}

	output := new(bytes.Buffer)
	approved, err := promptChangeReviewApproval(
		bytes.NewBufferString("n\n"),
		output,
		step,
		review,
		changeReviewRenderOptions{EnableColor: false},
	)
	if err != nil {
		t.Fatalf("promptChangeReviewApproval returned error: %v", err)
	}
	if approved {
		t.Fatal("expected rejection for explicit no response")
	}

	rendered := output.String()
	assertContains(t, rendered, "Proposed Changes")
	assertContains(t, rendered, "Fixing: leap.yaml should be present and valid")
	assertContains(t, rendered, "Files changed:")
	assertContains(t, rendered, "Diff summary:")
	assertContains(t, rendered, "Patch:")
	assertContains(t, rendered, "Apply and commit these changes? [Y/n]:")
	if strings.Contains(rendered, "Step:") {
		t.Fatalf("expected internal Step label to be omitted, got: %q", rendered)
	}
}

func TestPromptChangeReviewApprovalDefaultsToYes(t *testing.T) {
	step, ok := core.EnsureStepByID(core.EnsureStepLeapYAML)
	if !ok {
		t.Fatal("expected leap.yaml ensure-step in catalog")
	}

	output := new(bytes.Buffer)
	approved, err := promptChangeReviewApproval(
		bytes.NewBufferString("\n"),
		output,
		step,
		gitmanager.ChangeReview{Focus: "leap.yaml should be present and valid"},
		changeReviewRenderOptions{EnableColor: false},
	)
	if err != nil {
		t.Fatalf("promptChangeReviewApproval returned error: %v", err)
	}
	if !approved {
		t.Fatal("expected empty input to default to yes")
	}
}

func assertContains(t *testing.T, output string, expected string) {
	t.Helper()
	if !strings.Contains(output, expected) {
		t.Fatalf("expected output to contain %q, got: %q", expected, output)
	}
}
