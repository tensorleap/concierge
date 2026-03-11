package inspect

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestContractDiscoveryFindsDecoratedFunctions(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from somewhere import decorators",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return None",
		"",
		"@decorators.tensorleap_preprocess()",
		"def preprocess_data():",
		"    return []",
		"",
		"@tensorleap_input_encoder('image')",
		"def encode_image():",
		"    return 1",
		"",
		"@decorators.tensorleap_gt_encoder('label')",
		"def encode_label():",
		"    return 1",
		"",
		"@tensorleap_integration_test()",
		"def integration_test_flow():",
		"    load_model()",
		"",
	}, "\n"))

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if status.Contracts == nil {
		t.Fatalf("expected discovered contracts, got nil")
	}

	contracts := status.Contracts
	if contracts.EntryFile != "leap_integration.py" {
		t.Fatalf("expected entry file %q, got %q", "leap_integration.py", contracts.EntryFile)
	}
	if !reflect.DeepEqual(contracts.LoadModelFunctions, []string{"load_model"}) {
		t.Fatalf("expected load model functions %v, got %v", []string{"load_model"}, contracts.LoadModelFunctions)
	}
	if !reflect.DeepEqual(contracts.PreprocessFunctions, []string{"preprocess_data"}) {
		t.Fatalf("expected preprocess functions %v, got %v", []string{"preprocess_data"}, contracts.PreprocessFunctions)
	}
	if !reflect.DeepEqual(contracts.InputEncoders, []string{"encode_image"}) {
		t.Fatalf("expected input encoders %v, got %v", []string{"encode_image"}, contracts.InputEncoders)
	}
	if !reflect.DeepEqual(contracts.GroundTruthEncoders, []string{"encode_label"}) {
		t.Fatalf("expected GT encoders %v, got %v", []string{"encode_label"}, contracts.GroundTruthEncoders)
	}
	if !reflect.DeepEqual(contracts.IntegrationTestFunctions, []string{"integration_test_flow"}) {
		t.Fatalf("expected integration test functions %v, got %v", []string{"integration_test_flow"}, contracts.IntegrationTestFunctions)
	}
}

func TestContractDiscoveryCapturesIntegrationTestCalls(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"@tensorleap_integration_test()",
		"def run_integration():",
		"    load_model()",
		"    preprocess_data()",
		"    encoders.input_image()",
		"    encode_label()",
		"",
	}, "\n"))

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if status.Contracts == nil {
		t.Fatalf("expected discovered contracts, got nil")
	}
	if !reflect.DeepEqual(status.Contracts.IntegrationTestCalls, []string{
		"load_model",
		"preprocess_data",
		"input_image",
		"encode_label",
	}) {
		t.Fatalf("expected integration test calls %v, got %v",
			[]string{"load_model", "preprocess_data", "input_image", "encode_label"},
			status.Contracts.IntegrationTestCalls,
		)
	}
}

func TestContractDiscoveryGracefullyHandlesMissingEntryFile(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: missing_entry.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('test')\n")

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}

	if status.Contracts != nil {
		t.Fatalf("expected no discovered contracts for missing entry file, got %+v", status.Contracts)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeLeapYAMLEntryFileNotFound) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeLeapYAMLEntryFileNotFound, status.Issues)
	}
	if hasIssueCode(status.Issues, core.IssueCodeIntegrationScriptImportFailed) {
		t.Fatalf("did not expect %q issue for missing entry file, got %+v", core.IssueCodeIntegrationScriptImportFailed, status.Issues)
	}
}

func TestContractDiscoveryGracefullyHandlesSyntaxErrors(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"@tensorleap_preprocess()",
		"def broken(",
		"    return []",
		"",
	}, "\n"))

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}

	issue, ok := firstIssueByCode(status.Issues, core.IssueCodeIntegrationScriptImportFailed)
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeIntegrationScriptImportFailed, status.Issues)
	}
	if !strings.Contains(issue.Message, "leap_integration.py") {
		t.Fatalf("expected issue message to include entry file path, got %q", issue.Message)
	}
	if issue.Location == nil || issue.Location.Path != "leap_integration.py" {
		t.Fatalf("expected issue location path %q, got %+v", "leap_integration.py", issue.Location)
	}
	if issue.Location.Line <= 0 {
		t.Fatalf("expected issue location to include a line, got %+v", issue.Location)
	}
}

func TestContractDiscoveryEmitsMissingPreprocessIssueForBinderEntryFile(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "def not_preprocess():\n    return []\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('test')\n")

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !hasIssueCode(status.Issues, core.IssueCodePreprocessFunctionMissing) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodePreprocessFunctionMissing, status.Issues)
	}
}

func firstIssueByCode(issues []core.Issue, code core.IssueCode) (core.Issue, bool) {
	for _, issue := range issues {
		if issue.Code == code {
			return issue, true
		}
	}
	return core.Issue{}, false
}
