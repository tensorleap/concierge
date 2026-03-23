package inspect

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestGTEncoderDetectorEmitsMissingIssue(t *testing.T) {
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
		"    encode_label()",
		"",
	}, "\n"))

	status := inspectStatusWithConfirmedMapping(t, root, nil, []string{"label"})

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
	if strings.Contains(strings.ToLower(issue.Message), "symbol") || strings.Contains(strings.ToLower(issue.Message), "target") {
		t.Fatalf("expected user-facing ground-truth name wording, got %q", issue.Message)
	}
	if issue.Location == nil || issue.Location.Symbol != "label" {
		t.Fatalf("expected symbol location %q, got %+v", "label", issue.Location)
	}
}

func TestGTEncoderDetectorEmitsContractMismatchIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
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

	status := inspectStatusWithConfirmedMapping(t, root, nil, []string{"label"})

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
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
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

	status := inspectStatusWithConfirmedMapping(t, root, nil, []string{"label"})

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

func TestGTEncoderDetectorSupportsMultilineFunctionSignatures(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
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
		"@tensorleap_gt_encoder('classes')",
		"def encode_classes(idx: int, preprocess):",
		"    return idx",
		"",
		"@tensorleap_gt_encoder('bbs')",
		"def encode_bbs(",
		"    idx: int,",
		"    preprocess",
		") -> tuple[",
		"    int,",
		"    int,",
		"]:",
		"    return idx, idx",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    encode_classes(0, None)",
		"    encode_bbs(0, None)",
		"",
	}, "\n"))

	status := inspectStatusWithConfirmedMapping(t, root, nil, []string{"classes", "bbs"})

	if hasIssueCode(status.Issues, core.IssueCodeGTEncoderMissing) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodeGTEncoderMissing, status.Issues)
	}
	if hasIssueCode(status.Issues, core.IssueCodeGTEncoderCoverageIncomplete) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodeGTEncoderCoverageIncomplete, status.Issues)
	}
}
