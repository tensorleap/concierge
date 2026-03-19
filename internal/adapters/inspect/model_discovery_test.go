package inspect

import (
	"context"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestModelDiscoveryFromRepoSearchSingleVerifiedCandidate(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", simpleModelIntegrationSource())
	writeFixtureFile(t, root, "model/demo.h5", "binary")

	status := inspectStatusWithRuntimeProbe(t, root, func(snapshot core.WorkspaceSnapshot, path string, modelType string) ([]string, error) {
		if strings.HasSuffix(path, "model/other.h5") {
			return nil, context.DeadlineExceeded
		}
		return []string{"image"}, nil
	})

	if !hasModelCandidatePath(status, "model/demo.h5") {
		t.Fatalf("expected model candidate %q, got %+v", "model/demo.h5", status.Contracts)
	}
	if status.Contracts == nil || status.Contracts.ResolvedModelPath != "model/demo.h5" {
		t.Fatalf("expected resolved model path %q, got %+v", "model/demo.h5", status.Contracts)
	}
	if state := modelCandidateVerificationState(status, "model/demo.h5"); state != core.ModelCandidateVerificationStateVerified {
		t.Fatalf("expected verified candidate state, got %q", state)
	}
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelAcquisitionRequired)
	assertNoModelIssue(t, status.Issues, core.IssueCodeLoadModelDecoratorMissing)
}

func TestModelDiscoveryIncludesLoadModelDecoratorEvidenceForReadyArtifacts(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
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

	status := inspectStatusWithRuntimeProbe(t, root, func(snapshot core.WorkspaceSnapshot, path string, modelType string) ([]string, error) {
		if strings.HasSuffix(path, "model/other.h5") {
			return nil, context.DeadlineExceeded
		}
		return []string{"image"}, nil
	})

	if !hasModelCandidatePath(status, "model/from_decorator.onnx") {
		t.Fatalf("expected model candidate %q, got %+v", "model/from_decorator.onnx", status.Contracts)
	}
	source := modelCandidateSource(status, "model/from_decorator.onnx")
	if source != "load_model.load_model,repo_search" {
		t.Fatalf("expected candidate source %q, got %q", "load_model.load_model,repo_search", source)
	}
	if status.Contracts == nil || status.Contracts.ResolvedModelPath != "model/from_decorator.onnx" {
		t.Fatalf("expected resolved model path %q, got %+v", "model/from_decorator.onnx", status.Contracts)
	}
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelAcquisitionRequired)
}

func TestModelDiscoveryReportsAmbiguityForMultipleVerifiedArtifacts(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", simpleModelIntegrationSource())
	writeFixtureFile(t, root, "model/a.h5", "binary")
	writeFixtureFile(t, root, "model/b.onnx", "binary")

	status := inspectStatusWithRuntimeProbe(t, root, func(snapshot core.WorkspaceSnapshot, path string, modelType string) ([]string, error) {
		return []string{"image"}, nil
	})

	if !hasModelCandidatePath(status, "model/a.h5") || !hasModelCandidatePath(status, "model/b.onnx") {
		t.Fatalf("expected both model candidates, got %+v", status.Contracts)
	}
	if status.Contracts == nil || status.Contracts.ResolvedModelPath != "" {
		t.Fatalf("expected no resolved model path when multiple candidates verify, got %+v", status.Contracts)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeModelCandidatesAmbiguous) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeModelCandidatesAmbiguous, status.Issues)
	}
}

func TestModelDiscoveryReportsPassiveLeadWhenOnlyUnsupportedArtifactsExist(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", simpleModelIntegrationSource())
	writeFixtureFile(t, root, "model/demo.pt", "binary")

	status := inspectStatus(t, root)

	if !hasIssueCode(status.Issues, core.IssueCodeModelAcquisitionRequired) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeModelAcquisitionRequired, status.Issues)
	}
	if !hasPassiveLeadPath(status, "model/demo.pt") {
		t.Fatalf("expected passive lead %q in context, got %+v", "model/demo.pt", status.Contracts)
	}
}

func TestModelDiscoveryIgnoresVirtualenvArtifacts(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", simpleModelIntegrationSource())
	writeFixtureFile(t, root, ".venv/lib/python3.11/site-packages/onnx/backend/test/data/light/light_resnet50.onnx", "binary")
	writeFixtureFile(t, root, ".venv/lib/python3.11/site-packages/h5py/tests/data_files/demo.h5", "binary")

	status := inspectStatus(t, root)

	if status.Contracts == nil {
		t.Fatalf("expected contracts to be populated, got %+v", status)
	}
	if len(status.Contracts.ModelCandidates) != 0 {
		t.Fatalf("expected virtualenv artifacts to be ignored, got %+v", status.Contracts.ModelCandidates)
	}
	if status.Contracts.ModelAcquisition == nil {
		t.Fatalf("expected model acquisition artifacts, got %+v", status.Contracts)
	}
	if len(status.Contracts.ModelAcquisition.ReadyArtifacts) != 0 {
		t.Fatalf("expected no ready artifacts from virtualenv contents, got %+v", status.Contracts.ModelAcquisition.ReadyArtifacts)
	}
	if status.Contracts.ResolvedModelPath != "" {
		t.Fatalf("expected no resolved model path from virtualenv contents, got %q", status.Contracts.ResolvedModelPath)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeModelAcquisitionUnresolved) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeModelAcquisitionUnresolved, status.Issues)
	}
}

func TestModelDiscoveryReportsRepositoryAcquisitionLeadsWithoutReadyArtifacts(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", simpleModelIntegrationSource())
	writeFixtureFile(t, root, "project_config.yaml", "description: Example model\n")
	writeFixtureFile(t, root, "tools/export_model.py", "model.export(format='onnx')\n")
	writeFixtureFile(t, root, "docker/Dockerfile", "ADD https://example.com/releases/demo.onnx .\n")

	status := inspectStatus(t, root)

	if !hasIssueCode(status.Issues, core.IssueCodeModelAcquisitionRequired) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeModelAcquisitionRequired, status.Issues)
	}
	if status.Contracts == nil || status.Contracts.ModelAcquisition == nil {
		t.Fatalf("expected model acquisition artifacts, got %+v", status.Contracts)
	}
	leads := status.Contracts.ModelAcquisition.AcquisitionLeads
	if !containsString(leads, "project_config.yaml") {
		t.Fatalf("expected project config lead, got %+v", leads)
	}
	if !containsString(leads, "tools/export_model.py") {
		t.Fatalf("expected export script lead, got %+v", leads)
	}
	if !containsString(leads, "docker/Dockerfile -> https://example.com/releases/demo.onnx") {
		t.Fatalf("expected direct artifact lead, got %+v", leads)
	}
}

func TestModelDiscoveryFindsMaterializedArtifactsUnderDotConcierge(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", simpleModelIntegrationSource())
	writeFixtureFile(t, root, ".concierge/materialized_models/model.onnx", "binary")

	status := inspectStatusWithRuntimeProbe(t, root, func(snapshot core.WorkspaceSnapshot, path string, modelType string) ([]string, error) {
		return []string{"image"}, nil
	})

	if !hasModelCandidatePath(status, ".concierge/materialized_models/model.onnx") {
		t.Fatalf("expected materialized model candidate %q, got %+v", ".concierge/materialized_models/model.onnx", status.Contracts)
	}
	if status.Contracts == nil || status.Contracts.ResolvedModelPath != ".concierge/materialized_models/model.onnx" {
		t.Fatalf("expected resolved model path %q, got %+v", ".concierge/materialized_models/model.onnx", status.Contracts)
	}
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelAcquisitionRequired)
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelAcquisitionUnresolved)
}

func TestModelDiscoveryRejectsSelectedOutputPathOutsideRepo(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", simpleModelIntegrationSource())

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), core.WorkspaceSnapshot{
		Repository:        core.RepositoryState{Root: root},
		SelectedModelPath: "../external/model.h5",
	})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}

	if !hasIssueCode(status.Issues, core.IssueCodeModelAcquisitionUnresolved) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeModelAcquisitionUnresolved, status.Issues)
	}
}

func TestModelDiscoveryDefersAmbiguityWhilePreprocessMissing(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", "def helper():\n    return None\n")
	writeFixtureFile(t, root, "model/a.h5", "binary")
	writeFixtureFile(t, root, "model/b.onnx", "binary")

	status := inspectStatus(t, root)

	if !hasIssueCode(status.Issues, core.IssueCodePreprocessFunctionMissing) {
		t.Fatalf("expected preprocess missing issue, got %+v", status.Issues)
	}
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelCandidatesAmbiguous)
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelAcquisitionRequired)
}

func TestModelDiscoveryResolvesSelectedModelPath(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", simpleModelIntegrationSource())
	writeFixtureFile(t, root, "model/a.h5", "binary")
	writeFixtureFile(t, root, "model/b.onnx", "binary")

	status := inspectStatusWithRuntimeProbeAndSelection(t, root, "model/b.onnx", func(snapshot core.WorkspaceSnapshot, path string, modelType string) ([]string, error) {
		return []string{"image"}, nil
	})

	if status.Contracts == nil || status.Contracts.ResolvedModelPath != "model/b.onnx" {
		t.Fatalf("expected selected resolved model path %q, got %+v", "model/b.onnx", status.Contracts)
	}
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelCandidatesAmbiguous)
}

func TestModelDiscoveryIgnoresLeapYAMLIncludeExcludeForModelResolution(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"include:",
		"  - leap.yaml",
		"  - leap_integration.py",
		"  - leap_integration.py",
		"exclude:",
		"  - model/private/**",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", simpleModelIntegrationSource())
	writeFixtureFile(t, root, "model/private/model.h5", "binary")

	status := inspectStatusWithRuntimeProbe(t, root, func(snapshot core.WorkspaceSnapshot, path string, modelType string) ([]string, error) {
		return []string{"image"}, nil
	})

	if !hasModelCandidatePath(status, "model/private/model.h5") {
		t.Fatalf("expected candidate %q in context, got %+v", "model/private/model.h5", status.Contracts)
	}
	if status.Contracts == nil || status.Contracts.ResolvedModelPath != "model/private/model.h5" {
		t.Fatalf("expected resolved model path %q, got %+v", "model/private/model.h5", status.Contracts)
	}
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelAcquisitionRequired)
}

func TestModelDiscoveryResolvesOnlyVerifiedArtifactAmongMultipleCandidates(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", simpleModelIntegrationSource())
	writeFixtureFile(t, root, "model/a.h5", "binary")
	writeFixtureFile(t, root, "model/b.onnx", "binary")

	status := inspectStatusWithRuntimeProbe(t, root, func(snapshot core.WorkspaceSnapshot, path string, modelType string) ([]string, error) {
		if strings.HasSuffix(path, "model/a.h5") {
			return nil, context.DeadlineExceeded
		}
		return []string{"image"}, nil
	})

	if status.Contracts == nil || status.Contracts.ResolvedModelPath != "model/b.onnx" {
		t.Fatalf("expected the only verified model path %q to resolve, got %+v", "model/b.onnx", status.Contracts)
	}
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelCandidatesAmbiguous)
}

func TestModelDiscoveryDoesNotResolveUnloadableArtifact(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", simpleModelIntegrationSource())
	writeFixtureFile(t, root, "model/demo.onnx", "binary")

	status := inspectStatusWithRuntimeProbe(t, root, func(snapshot core.WorkspaceSnapshot, path string, modelType string) ([]string, error) {
		return nil, context.DeadlineExceeded
	})

	if status.Contracts == nil || status.Contracts.ResolvedModelPath != "" {
		t.Fatalf("expected unloadable candidate to stay unresolved, got %+v", status.Contracts)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeModelAcquisitionUnresolved) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeModelAcquisitionUnresolved, status.Issues)
	}
	if state := modelCandidateVerificationState(status, "model/demo.onnx"); state != core.ModelCandidateVerificationStateFailed {
		t.Fatalf("expected failed candidate state, got %q", state)
	}
	if errMessage := modelCandidateVerificationError(status, "model/demo.onnx"); strings.TrimSpace(errMessage) == "" {
		t.Fatalf("expected verification error to be recorded, got %q", errMessage)
	}
}

func TestModelDiscoveryLeavesReadyArtifactsUnverifiedWithoutRuntimeProfile(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", simpleModelIntegrationSource())
	writeFixtureFile(t, root, "model/demo.onnx", "binary")

	status := inspectStatus(t, root)

	if status.Contracts == nil || status.Contracts.ResolvedModelPath != "" {
		t.Fatalf("expected no resolved model path without runtime verification, got %+v", status.Contracts)
	}
	if state := modelCandidateVerificationState(status, "model/demo.onnx"); state != core.ModelCandidateVerificationStateUnverified {
		t.Fatalf("expected unverified candidate state, got %q", state)
	}
	assertNoModelIssue(t, status.Issues, core.IssueCodeModelAcquisitionUnresolved)
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

func inspectStatusWithRuntimeProbe(
	t *testing.T,
	root string,
	probe func(snapshot core.WorkspaceSnapshot, path string, modelType string) ([]string, error),
) core.IntegrationStatus {
	t.Helper()
	return inspectStatusWithRuntimeProbeAndSelection(t, root, "", probe)
}

func inspectStatusWithRuntimeProbeAndSelection(
	t *testing.T,
	root string,
	selectedModelPath string,
	probe func(snapshot core.WorkspaceSnapshot, path string, modelType string) ([]string, error),
) core.IntegrationStatus {
	t.Helper()

	previousRunner := runtimeSignatureProbeRunner
	runtimeSignatureProbeRunner = probe
	t.Cleanup(func() {
		runtimeSignatureProbeRunner = previousRunner
	})

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), core.WorkspaceSnapshot{
		Repository:        core.RepositoryState{Root: root},
		SelectedModelPath: selectedModelPath,
		RuntimeProfile: &core.LocalRuntimeProfile{
			InterpreterPath: "/tmp/fake-python",
		},
	})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	return status
}

func simpleModelIntegrationSource() string {
	return strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_integration_test, tensorleap_load_model, tensorleap_preprocess",
		"",
		"@tensorleap_preprocess()",
		"def preprocess():",
		"    return []",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return None",
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

func hasPassiveLeadPath(status core.IntegrationStatus, path string) bool {
	if status.Contracts == nil || status.Contracts.ModelAcquisition == nil {
		return false
	}
	for _, candidate := range status.Contracts.ModelAcquisition.PassiveLeads {
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

func modelCandidateVerificationState(status core.IntegrationStatus, path string) core.ModelCandidateVerificationState {
	if status.Contracts == nil {
		return ""
	}
	for _, candidate := range status.Contracts.ModelCandidates {
		if candidate.Path == path {
			return candidate.VerificationState
		}
	}
	return ""
}

func modelCandidateVerificationError(status core.IntegrationStatus, path string) string {
	if status.Contracts == nil {
		return ""
	}
	for _, candidate := range status.Contracts.ModelCandidates {
		if candidate.Path == path {
			return candidate.VerificationError
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
