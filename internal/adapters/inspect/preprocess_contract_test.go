package inspect

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestPreprocessDetectorEmitsMissingFunctionIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_integration_test",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    return None",
		"",
	}, "\n"))

	status := inspectStatus(t, root)

	issue, ok := firstIssueByCode(status.Issues, core.IssueCodePreprocessFunctionMissing)
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodePreprocessFunctionMissing, status.Issues)
	}
	if issue.Scope != core.IssueScopePreprocess {
		t.Fatalf("expected preprocess scope, got %q", issue.Scope)
	}

	step, ok := core.PreferredEnsureStepForIssueCode(issue.Code)
	if !ok {
		t.Fatalf("expected preferred step for issue %q", issue.Code)
	}
	if step.ID != core.EnsureStepPreprocessContract {
		t.Fatalf("expected preferred step %q, got %q", core.EnsureStepPreprocessContract, step.ID)
	}
}

func TestPreprocessDetectorEmitsInvalidSignatureIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data(dataset):",
		"    return []",
		"",
	}, "\n"))

	status := inspectStatus(t, root)

	issue, ok := firstIssueByCode(status.Issues, core.IssueCodePreprocessResponseInvalid)
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodePreprocessResponseInvalid, status.Issues)
	}
	if !strings.Contains(issue.Message, "must not accept parameters") {
		t.Fatalf("expected invalid-signature message, got %q", issue.Message)
	}
	if issue.Location == nil || issue.Location.Symbol != "preprocess_data" {
		t.Fatalf("expected preprocess symbol location, got %+v", issue.Location)
	}
}

func TestPreprocessDetectorDoesNotFlagValidDefinitions(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess, tensorleap_load_model",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data():",
		"    responses = []",
		"    return responses",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/demo.h5'",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "model/demo.h5", "binary")

	status := inspectStatus(t, root)

	if hasIssueCode(status.Issues, core.IssueCodePreprocessFunctionMissing) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodePreprocessFunctionMissing, status.Issues)
	}
	if hasIssueCode(status.Issues, core.IssueCodePreprocessResponseInvalid) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodePreprocessResponseInvalid, status.Issues)
	}
}
