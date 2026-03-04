package inspect

import (
	"context"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestModelDiscoveryFromRepoSearchSingleCandidate(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_custom_test.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_custom_test.py", simpleIntegrationTestSource())
	writeFixtureFile(t, root, "model/demo.h5", "binary")

	status := inspectStatus(t, root)

	if !hasModelCandidatePath(status, "model/demo.h5") {
		t.Fatalf("expected model candidate %q, got %+v", "model/demo.h5", status.Contracts)
	}
	if status.Contracts == nil || status.Contracts.ResolvedModelPath != "model/demo.h5" {
		t.Fatalf("expected resolved model path %q, got %+v", "model/demo.h5", status.Contracts)
	}
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelFileMissing)
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelCandidatesAmbiguous)
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelFormatUnsupported)
}

func TestModelDiscoveryFromLoadModelDecorator(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_custom_test.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_custom_test.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_integration_test",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    model_path = 'model/from_decorator.onnx'",
		"    return model_path",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    load_model()",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "model/from_decorator.onnx", "binary")
	writeFixtureFile(t, root, "model/other.h5", "binary")

	status := inspectStatus(t, root)

	if !hasModelCandidatePath(status, "model/from_decorator.onnx") {
		t.Fatalf("expected model candidate %q, got %+v", "model/from_decorator.onnx", status.Contracts)
	}
	source := modelCandidateSource(status, "model/from_decorator.onnx")
	if !strings.Contains(source, "load_model.load_model") {
		t.Fatalf("expected candidate source to include load-model discovery, got %q", source)
	}
	if status.Contracts == nil || status.Contracts.ResolvedModelPath != "model/from_decorator.onnx" {
		t.Fatalf("expected resolved model path %q, got %+v", "model/from_decorator.onnx", status.Contracts)
	}
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelCandidatesAmbiguous)
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelFileMissing)
}

func TestModelDiscoveryReportsAmbiguousCandidates(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_custom_test.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_custom_test.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess, tensorleap_integration_test",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data():",
		"    return []",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    return None",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "model/a.h5", "binary")
	writeFixtureFile(t, root, "model/b.onnx", "binary")

	status := inspectStatus(t, root)

	if !hasIssueCode(status.Issues, core.IssueCodeModelCandidatesAmbiguous) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeModelCandidatesAmbiguous, status.Issues)
	}
	if !hasModelCandidatePath(status, "model/a.h5") || !hasModelCandidatePath(status, "model/b.onnx") {
		t.Fatalf("expected both model candidates, got %+v", status.Contracts)
	}
}

func TestModelDiscoveryRejectsUnsupportedFormat(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_custom_test.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_custom_test.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    model_path = 'model/demo.pt'",
		"    return model_path",
		"",
	}, "\n"))

	status := inspectStatus(t, root)

	if !hasIssueCode(status.Issues, core.IssueCodeModelFormatUnsupported) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeModelFormatUnsupported, status.Issues)
	}
	if !hasModelCandidatePath(status, "model/demo.pt") {
		t.Fatalf("expected candidate %q in context, got %+v", "model/demo.pt", status.Contracts)
	}
}

func TestModelDiscoveryRejectsOutsideRepoModelPath(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_custom_test.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_custom_test.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    model_path = '../external/model.h5'",
		"    return model_path",
		"",
	}, "\n"))

	status := inspectStatus(t, root)

	if !hasIssueCode(status.Issues, core.IssueCodeModelFileMissing) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeModelFileMissing, status.Issues)
	}
}

func TestModelDiscoveryDefersAmbiguityWhilePreprocessMissing(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_binder.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_binder.py", "def helper():\n    return None\n")
	writeFixtureFile(t, root, "integration_test.py", "print('test')\n")
	writeFixtureFile(t, root, "model/a.h5", "binary")
	writeFixtureFile(t, root, "model/b.onnx", "binary")

	status := inspectStatus(t, root)

	if !hasIssueCode(status.Issues, core.IssueCodePreprocessFunctionMissing) {
		t.Fatalf("expected preprocess missing issue, got %+v", status.Issues)
	}
	if hasIssueCode(status.Issues, core.IssueCodeModelCandidatesAmbiguous) {
		t.Fatalf("did not expect ambiguous model issue while preprocess is missing, got %+v", status.Issues)
	}
}

func TestModelDiscoveryResolvesSelectedModelPath(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_custom_test.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_custom_test.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/a.h5'",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "model/a.h5", "binary")
	writeFixtureFile(t, root, "model/b.onnx", "binary")

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), core.WorkspaceSnapshot{
		Repository:        core.RepositoryState{Root: root},
		SelectedModelPath: "model/b.onnx",
	})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}

	if status.Contracts == nil || status.Contracts.ResolvedModelPath != "model/b.onnx" {
		t.Fatalf("expected selected resolved model path %q, got %+v", "model/b.onnx", status.Contracts)
	}
	if hasIssueCode(status.Issues, core.IssueCodeModelCandidatesAmbiguous) {
		t.Fatalf("did not expect ambiguous model issue when selected path is provided, got %+v", status.Issues)
	}
}

func TestModelDiscoveryIgnoresLeapYAMLIncludeExcludeForModelResolution(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_custom_test.py",
		"include:",
		"  - leap.yaml",
		"  - leap_binder.py",
		"  - leap_custom_test.py",
		"exclude:",
		"  - model/private/**",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_custom_test.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    model_path = 'model/private/model.h5'",
		"    return model_path",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "model/private/model.h5", "binary")

	status := inspectStatus(t, root)

	if !hasModelCandidatePath(status, "model/private/model.h5") {
		t.Fatalf("expected candidate %q in context, got %+v", "model/private/model.h5", status.Contracts)
	}
	if status.Contracts == nil || status.Contracts.ResolvedModelPath != "model/private/model.h5" {
		t.Fatalf("expected resolved model path %q, got %+v", "model/private/model.h5", status.Contracts)
	}
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelFileMissing)
}

func inspectStatus(t *testing.T, root string) core.IntegrationStatus {
	t.Helper()
	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	return status
}

func simpleIntegrationTestSource() string {
	return strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_integration_test",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    return None",
		"",
	}, "\n")
}

func hasModelCandidatePath(status core.IntegrationStatus, path string) bool {
	if status.Contracts == nil {
		return false
	}
	for _, candidate := range status.Contracts.ModelCandidates {
		if candidate.Path == path {
			return true
		}
	}
	return false
}

func modelCandidateSource(status core.IntegrationStatus, path string) string {
	if status.Contracts == nil {
		return ""
	}
	for _, candidate := range status.Contracts.ModelCandidates {
		if candidate.Path == path {
			return candidate.Source
		}
	}
	return ""
}

func assertNoModelIssue(t *testing.T, issues []core.Issue, code core.IssueCode) {
	t.Helper()
	if hasIssueCode(issues, code) {
		t.Fatalf("did not expect %q issue, got %+v", code, issues)
	}
}
