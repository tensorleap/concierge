package inspect

import (
	"context"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestInputEncoderDetectorEmitsMissingEncoderIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_preprocess, tensorleap_integration_test",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/demo.h5'",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data():",
		"    return []",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    load_model()",
		"    preprocess_data()",
		"    encode_image()",
		"",
	}, "\n"))

	status := inspectStatusWithConfirmedMapping(t, root, []string{"image"}, nil)

	issue, ok := firstIssueByCode(status.Issues, core.IssueCodeInputEncoderMissing)
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeInputEncoderMissing, status.Issues)
	}
	if issue.Scope != core.IssueScopeInputEncoder {
		t.Fatalf("expected scope %q, got %q", core.IssueScopeInputEncoder, issue.Scope)
	}
	if !strings.Contains(strings.ToLower(issue.Message), "image") {
		t.Fatalf("expected symbol detail in issue message, got %q", issue.Message)
	}
	if issue.Location == nil || issue.Location.Symbol != "image" {
		t.Fatalf("expected symbol location %q, got %+v", "image", issue.Location)
	}

	step, ok := core.PreferredEnsureStepForIssueCode(issue.Code)
	if !ok {
		t.Fatalf("expected preferred step for issue %q", issue.Code)
	}
	if step.ID != core.EnsureStepInputEncoders {
		t.Fatalf("expected preferred step %q, got %q", core.EnsureStepInputEncoders, step.ID)
	}
}

func TestInputEncoderDetectorEmitsCoverageIncompleteIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_preprocess, tensorleap_input_encoder, tensorleap_integration_test",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/demo.h5'",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data():",
		"    return []",
		"",
		"@tensorleap_input_encoder('image')",
		"def encode_image():",
		"    return 1",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    encode_image()",
		"    encode_meta()",
		"",
	}, "\n"))

	status := inspectStatusWithConfirmedMapping(t, root, []string{"image", "meta"}, nil)

	issue, ok := firstIssueByCode(status.Issues, core.IssueCodeInputEncoderCoverageIncomplete)
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeInputEncoderCoverageIncomplete, status.Issues)
	}
	if !strings.Contains(strings.ToLower(issue.Message), "meta") {
		t.Fatalf("expected missing symbol detail in message, got %q", issue.Message)
	}
	if issue.Location == nil || issue.Location.Symbol != "meta" {
		t.Fatalf("expected location symbol %q, got %+v", "meta", issue.Location)
	}
}

func TestInputEncoderDetectorNoFalsePositiveWhenCoverageComplete(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_preprocess, tensorleap_input_encoder, tensorleap_integration_test",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/demo.h5'",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data():",
		"    return []",
		"",
		"@tensorleap_input_encoder('image')",
		"def encode_image():",
		"    return 1",
		"",
		"@tensorleap_input_encoder('meta')",
		"def encode_meta():",
		"    return 1",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    encode_image()",
		"    encode_meta()",
		"",
	}, "\n"))

	status := inspectStatusWithConfirmedMapping(t, root, []string{"image", "meta"}, nil)

	if hasIssueCode(status.Issues, core.IssueCodeInputEncoderMissing) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodeInputEncoderMissing, status.Issues)
	}
	if hasIssueCode(status.Issues, core.IssueCodeInputEncoderCoverageIncomplete) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodeInputEncoderCoverageIncomplete, status.Issues)
	}
}

func inspectStatusWithConfirmedMapping(
	t *testing.T,
	root string,
	inputSymbols []string,
	groundTruthSymbols []string,
) core.IntegrationStatus {
	t.Helper()
	snapshot := snapshotForRoot(root)
	snapshot.ConfirmedEncoderMapping = &core.EncoderMappingContract{
		InputSymbols:       append([]string(nil), inputSymbols...),
		GroundTruthSymbols: append([]string(nil), groundTruthSymbols...),
	}

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	return status
}
