package inspect

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestGTEncoderDetectorEmitsMissingIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_custom_test.py\n")
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_custom_test.py", strings.Join([]string{
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
		"    encode_label()",
		"",
	}, "\n"))

	status := inspectStatus(t, root)

	issue, ok := firstIssueByCode(status.Issues, core.IssueCodeGTEncoderMissing)
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeGTEncoderMissing, status.Issues)
	}
	if issue.Scope != core.IssueScopeGroundTruthEncoder {
		t.Fatalf("expected scope %q, got %q", core.IssueScopeGroundTruthEncoder, issue.Scope)
	}
	if !strings.Contains(strings.ToLower(issue.Message), "label") {
		t.Fatalf("expected symbol detail in issue message, got %q", issue.Message)
	}
	if issue.Location == nil || issue.Location.Symbol != "label" {
		t.Fatalf("expected symbol location %q, got %+v", "label", issue.Location)
	}
}

func TestGTEncoderDetectorEmitsContractMismatchIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_custom_test.py\n")
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_custom_test.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_preprocess, tensorleap_gt_encoder, tensorleap_integration_test",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/demo.h5'",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data():",
		"    return []",
		"",
		"@tensorleap_gt_encoder()",
		"def encode_label():",
		"    return 1",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    encode_label()",
		"",
	}, "\n"))

	status := inspectStatus(t, root)

	issue, ok := firstIssueByCode(status.Issues, core.IssueCodeGTEncoderCoverageIncomplete)
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeGTEncoderCoverageIncomplete, status.Issues)
	}
	if !strings.Contains(strings.ToLower(issue.Message), "ambiguous") {
		t.Fatalf("expected mismatch/ambiguity details, got %q", issue.Message)
	}
}

func TestGTEncoderDetectorRespectsUnlabeledSubsetRule(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_custom_test.py\n")
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_custom_test.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_preprocess, tensorleap_gt_encoder, tensorleap_integration_test",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/demo.h5'",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data():",
		"    return []",
		"",
		"@tensorleap_gt_encoder('label', subsets=['unlabeled'])",
		"def encode_label():",
		"    return 1",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    encode_label()",
		"",
	}, "\n"))

	status := inspectStatus(t, root)

	issue, ok := firstIssueByCode(status.Issues, core.IssueCodeUnlabeledSubsetGTInvocation)
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeUnlabeledSubsetGTInvocation, status.Issues)
	}
	if !strings.Contains(strings.ToLower(issue.Message), "labeled") {
		t.Fatalf("expected labeled-subset rule detail, got %q", issue.Message)
	}
	if issue.Location == nil || issue.Location.Symbol != "label" {
		t.Fatalf("expected symbol location %q, got %+v", "label", issue.Location)
	}
}
