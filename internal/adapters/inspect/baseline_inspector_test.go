package inspect

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	validateadapter "github.com/tensorleap/concierge/internal/adapters/validate"
	"github.com/tensorleap/concierge/internal/core"
)

func TestInspectorReportsAllMissingArtifacts(t *testing.T) {
	root := t.TempDir()
	inspector := NewBaselineInspector()

	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}

	expectedMissing := []string{"leap_integration.py", "leap.yaml"}
	if !reflect.DeepEqual(status.Missing, expectedMissing) {
		t.Fatalf("expected missing %v, got %v", expectedMissing, status.Missing)
	}

	expectedCodes := []core.IssueCode{
		core.IssueCodeIntegrationScriptMissing,
		core.IssueCodeLeapYAMLMissing,
	}
	if got := issueCodes(status.Issues); !reflect.DeepEqual(got, expectedCodes) {
		t.Fatalf("expected issue codes %v, got %v", expectedCodes, got)
	}
}

func TestInspectorRequiresIntegrationTestDecoratorInCanonicalEntryFile(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", minimalIntegrationSourceWithoutIntegrationTest())
	writeFixtureFile(t, root, "model/model.h5", "binary\n")

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeIntegrationTestMissing) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeIntegrationTestMissing, status.Issues)
	}
}

func TestInspectorNoIssuesWhenArtifactsExist(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", minimalIntegrationSourceWithLoadModel())
	writeFixtureFile(t, root, "model/model.h5", "binary\n")

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if len(status.Missing) != 0 {
		t.Fatalf("expected no missing artifacts, got %v", status.Missing)
	}
	if len(status.Issues) != 0 {
		t.Fatalf("expected no issues, got %+v", status.Issues)
	}
}

func TestInspectorIncludesIntegrationTestASTIssuesWhenRuntimeIsResolved(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", minimalIntegrationSource())

	interpreterPath := filepath.Join(root, ".venv", "bin", "python")
	if err := os.MkdirAll(filepath.Dir(interpreterPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(interpreterPath, []byte("#!/usr/bin/env python3\n"), 0o755); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	inspector := &BaselineInspector{
		integrationTestAnalyzer: fakeInspectIntegrationTestAnalyzer{
			result: validateadapter.IntegrationTestASTResult{
				Issues: []core.Issue{
					{
						Code:     core.IssueCodeIntegrationTestIllegalBodyLogic,
						Message:  "integration_test should stay declarative",
						Severity: core.SeverityError,
						Scope:    core.IssueScopeIntegrationTest,
					},
				},
			},
		},
	}
	status, err := inspector.Inspect(context.Background(), core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: root},
		RuntimeProfile: &core.LocalRuntimeProfile{
			InterpreterPath: interpreterPath,
		},
	})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeIntegrationTestIllegalBodyLogic) {
		t.Fatalf("expected AST-derived integration-test issue, got %+v", status.Issues)
	}
}

func TestInspectorIssueScopesAndSeverities(t *testing.T) {
	root := t.TempDir()
	inspector := NewBaselineInspector()

	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}

	issues := make(map[core.IssueCode]core.Issue, len(status.Issues))
	for _, issue := range status.Issues {
		issues[issue.Code] = issue
	}

	assertIssueScopeAndSeverity(t, issues, core.IssueCodeLeapYAMLMissing, core.IssueScopeLeapYAML)
	assertIssueScopeAndSeverity(t, issues, core.IssueCodeIntegrationScriptMissing, core.IssueScopeIntegrationScript)
}

func TestInspectorLeapYAMLUnparseableEmitsIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: [\n")
	writeFixtureFile(t, root, "leap_integration.py", minimalIntegrationSource())

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeLeapYAMLUnparseable) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeLeapYAMLUnparseable, status.Issues)
	}
}

func TestInspectorLeapYAMLEntryFileMissingEmitsIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "projectId: demo\n")
	writeFixtureFile(t, root, "leap_integration.py", minimalIntegrationSource())

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeLeapYAMLEntryFileMissing) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeLeapYAMLEntryFileMissing, status.Issues)
	}
}

func TestInspectorLeapYAMLEntryFileNotFoundEmitsIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: missing_entry.py\n")
	writeFixtureFile(t, root, "leap_integration.py", minimalIntegrationSource())

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeLeapYAMLEntryFileNotFound) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeLeapYAMLEntryFileNotFound, status.Issues)
	}
}

func TestInspectorAllowsProjectAndSecretIdentifiersInLeapYAML(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"projectId: demo-project",
		"secretId: demo-secret",
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", minimalIntegrationSourceWithLoadModel())
	writeFixtureFile(t, root, "model/model.h5", "binary\n")

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if len(status.Issues) != 0 {
		t.Fatalf("expected no issues, got %+v", status.Issues)
	}
}

func TestInspectorAllowsEntryFileExcludedByLeapYAML(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"include:",
		"  - leap.yaml",
		"  - leap_integration.py",
		"  - model/**",
		"exclude:",
		"  - leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", minimalIntegrationSourceWithLoadModel())
	writeFixtureFile(t, root, "model/model.h5", "binary\n")

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if hasIssueCode(status.Issues, core.IssueCodeLeapYAMLEntryFileExcluded) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodeLeapYAMLEntryFileExcluded, status.Issues)
	}
	if hasIssueCode(status.Issues, core.IssueCodeLeapYAMLIncludeMissingRequiredFiles) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodeLeapYAMLIncludeMissingRequiredFiles, status.Issues)
	}
	if hasIssueCode(status.Issues, core.IssueCodeLeapYAMLExcludeBlocksRequiredFiles) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodeLeapYAMLExcludeBlocksRequiredFiles, status.Issues)
	}
	if !status.Ready() {
		t.Fatalf("expected ready status, got missing=%v issues=%+v", status.Missing, status.Issues)
	}
}

func TestInspectorReportsNonCanonicalEntryFile(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: wrong_entry.py\n")
	writeFixtureFile(t, root, "leap_integration.py", minimalIntegrationSource())
	writeFixtureFile(t, root, "wrong_entry.py", minimalIntegrationSource())

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeIntegrationScriptNonCanonical) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeIntegrationScriptNonCanonical, status.Issues)
	}
}

func TestInspectorDetectsModelAcquisitionRequirementFromPassiveLead(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", strings.Join([]string{
		"entryFile: leap_integration.py",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_preprocess, tensorleap_integration_test",
		"",
		"@tensorleap_preprocess()",
		"def preprocess():",
		"    return []",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    model_path = 'model.pt'",
		"    return model_path",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess_response):",
		"    return None",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "model/model.pt", "binary\n")

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeModelAcquisitionRequired) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeModelAcquisitionRequired, status.Issues)
	}
}

func TestInspectorDetectsMissingLeapCLI(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", minimalIntegrationSource())

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: root},
		LeapCLI:    core.LeapCLIState{ProbeRan: true, Available: false},
	})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeLeapCLINotFound) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeLeapCLINotFound, status.Issues)
	}
}

func TestInspectorWarnsWhenCodeLoaderLacksGuideLocalStatusTable(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", minimalIntegrationSourceWithLoadModel())
	writeFixtureFile(t, root, "model/model.h5", "binary\n")

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: root},
		Runtime: core.RuntimeState{
			ProbeRan:         true,
			PyProjectPresent: true,
			SupportedProject: true,
			PoetryFound:      true,
			PoetryExecutable: "poetry",
			PoetryVersion:    "Poetry 2.0.0",
		},
		RuntimeProfile: &core.LocalRuntimeProfile{
			Kind:              "poetry",
			PoetryExecutable:  "poetry",
			PoetryVersion:     "Poetry 2.0.0",
			InterpreterPath:   "/repo/.venv/bin/python",
			PythonVersion:     "Python 3.10.16",
			DependenciesReady: true,
			CodeLoaderReady:   true,
			CodeLoader: core.CodeLoaderCapabilityState{
				ProbeSucceeded:                true,
				Version:                       "1.0.138",
				SupportsGuideLocalStatusTable: false,
				SupportsCheckDataset:          true,
			},
		},
	})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeCodeLoaderLegacy) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeCodeLoaderLegacy, status.Issues)
	}
}

func TestInspectorDetectsServerInfoFailures(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", minimalIntegrationSource())

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: root},
		LeapCLI: core.LeapCLIState{
			ProbeRan:            true,
			Available:           true,
			Version:             "leap v0.2.0",
			Authenticated:       true,
			ServerInfoReachable: false,
			ServerInfoError:     "connection refused",
		},
	})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeLeapServerUnreachable) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeLeapServerUnreachable, status.Issues)
	}
	issueByCode := make(map[core.IssueCode]core.Issue, len(status.Issues))
	for _, issue := range status.Issues {
		issueByCode[issue.Code] = issue
	}
	issue, ok := issueByCode[core.IssueCodeLeapServerUnreachable]
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeLeapServerUnreachable, status.Issues)
	}
	if issue.Severity != core.SeverityWarning {
		t.Fatalf("expected server info failures to be warning severity %q, got %q", core.SeverityWarning, issue.Severity)
	}
}

func snapshotForRoot(root string) core.WorkspaceSnapshot {
	return core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: root},
	}
}

func minimalIntegrationSourceWithoutIntegrationTest() string {
	return strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess",
		"",
		"@tensorleap_preprocess()",
		"def preprocess():",
		"    return []",
		"",
	}, "\n")
}

func minimalIntegrationSource() string {
	return strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_integration_test, tensorleap_preprocess",
		"",
		"@tensorleap_preprocess()",
		"def preprocess():",
		"    return []",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess_response):",
		"    return None",
		"",
		`if __name__ == "__main__":`,
		"    responses = preprocess()",
		"    for subset in responses:",
		"        for i in range(5):",
		"            integration_test(i, subset)",
		"",
	}, "\n")
}

func minimalIntegrationSourceWithLoadModel() string {
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
		"def integration_test(sample_id, preprocess_response):",
		"    return None",
		"",
		`if __name__ == "__main__":`,
		"    responses = preprocess()",
		"    for subset in responses:",
		"        for i in range(5):",
		"            integration_test(i, subset)",
		"",
	}, "\n")
}

func writeFixtureFile(t *testing.T, root, relativePath, contents string) {
	t.Helper()
	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll failed for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", path, err)
	}
}

type fakeInspectIntegrationTestAnalyzer struct {
	result validateadapter.IntegrationTestASTResult
	err    error
}

func (f fakeInspectIntegrationTestAnalyzer) Analyze(ctx context.Context, snapshot core.WorkspaceSnapshot) (validateadapter.IntegrationTestASTResult, error) {
	_ = ctx
	_ = snapshot
	return f.result, f.err
}

func issueCodes(issues []core.Issue) []core.IssueCode {
	codes := make([]core.IssueCode, 0, len(issues))
	for _, issue := range issues {
		codes = append(codes, issue.Code)
	}
	return codes
}

func hasIssueCode(issues []core.Issue, code core.IssueCode) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func containsMissing(missing []string, label string) bool {
	for _, item := range missing {
		if item == label {
			return true
		}
	}
	return false
}

func assertIssueScopeAndSeverity(t *testing.T, issues map[core.IssueCode]core.Issue, code core.IssueCode, scope core.IssueScope) {
	t.Helper()
	issue, ok := issues[code]
	if !ok {
		t.Fatalf("expected issue %q to exist", code)
	}
	if issue.Scope != scope {
		t.Fatalf("expected issue %q scope %q, got %q", code, scope, issue.Scope)
	}
	if issue.Severity != core.SeverityError {
		t.Fatalf("expected issue %q severity %q, got %q", code, core.SeverityError, issue.Severity)
	}
}
