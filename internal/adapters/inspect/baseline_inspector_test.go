package inspect

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestInspectorReportsAllMissingArtifacts(t *testing.T) {
	root := t.TempDir()
	inspector := NewBaselineInspector()

	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}

	expectedMissing := []string{"leap.yaml", "leap_binder.py", "integration_test"}
	if !reflect.DeepEqual(status.Missing, expectedMissing) {
		t.Fatalf("expected missing %v, got %v", expectedMissing, status.Missing)
	}

	expectedCodes := []core.IssueCode{
		core.IssueCodeLeapYAMLMissing,
		core.IssueCodeIntegrationScriptMissing,
		core.IssueCodeIntegrationTestMissing,
	}
	if got := issueCodes(status.Issues); !reflect.DeepEqual(got, expectedCodes) {
		t.Fatalf("expected issue codes %v, got %v", expectedCodes, got)
	}
}

func TestInspectorAcceptsEitherIntegrationTestFileName(t *testing.T) {
	testCases := []struct {
		name     string
		testFile string
	}{
		{name: "leap custom test", testFile: "leap_custom_test.py"},
		{name: "integration test", testFile: "integration_test.py"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_binder.py\n")
			writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
			writeFixtureFile(t, root, tc.testFile, "print('test')\n")

			inspector := NewBaselineInspector()
			status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
			if err != nil {
				t.Fatalf("Inspect returned error: %v", err)
			}
			if containsMissing(status.Missing, "integration_test") {
				t.Fatalf("did not expect integration_test to be missing: %v", status.Missing)
			}
			if hasIssueCode(status.Issues, core.IssueCodeIntegrationTestMissing) {
				t.Fatalf("did not expect issue code %q in %+v", core.IssueCodeIntegrationTestMissing, status.Issues)
			}
			if !status.Ready() {
				t.Fatalf("expected ready status, got missing=%v issues=%+v", status.Missing, status.Issues)
			}
		})
	}
}

func TestInspectorNoIssuesWhenArtifactsExist(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_binder.py\n")
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "integration_test.py", "print('test')\n")

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
	assertIssueScopeAndSeverity(t, issues, core.IssueCodeIntegrationTestMissing, core.IssueScopeIntegrationTest)
}

func TestInspectorLeapYAMLUnparseableEmitsIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: [\n")
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "integration_test.py", "print('test')\n")

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
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "integration_test.py", "print('test')\n")

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
	writeFixtureFile(t, root, "leap_binder.py", "print('binder')\n")
	writeFixtureFile(t, root, "integration_test.py", "print('test')\n")

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotForRoot(root))
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !hasIssueCode(status.Issues, core.IssueCodeLeapYAMLEntryFileNotFound) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeLeapYAMLEntryFileNotFound, status.Issues)
	}
}

func snapshotForRoot(root string) core.WorkspaceSnapshot {
	return core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: root},
	}
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
