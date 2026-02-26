package execute

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestExecutorCreatesLeapYAMLWhenMissing(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepLeapYAML)

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected applied=true when leap.yaml is missing")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "leap.yaml")); err != nil {
		t.Fatalf("expected leap.yaml to be created: %v", err)
	}
	assertEvidenceValue(t, result.Evidence, "executor.before_checksum", "missing")
}

func TestExecutorCreatesIntegrationScriptTemplate(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepIntegrationScript)

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected applied=true when leap_binder.py is missing")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "leap_binder.py")); err != nil {
		t.Fatalf("expected leap_binder.py to be created: %v", err)
	}
}

func TestExecutorCreatesIntegrationTestTemplate(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepIntegrationTestContract)

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected applied=true when leap_custom_test.py is missing")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "leap_custom_test.py")); err != nil {
		t.Fatalf("expected leap_custom_test.py to be created: %v", err)
	}
}

func TestExecutorIdempotentOnSecondRun(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepLeapYAML)

	first, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("first Execute returned error: %v", err)
	}
	if !first.Applied {
		t.Fatal("expected first run to apply changes")
	}

	second, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}
	if second.Applied {
		t.Fatal("expected second run to be idempotent with applied=false")
	}
	before := evidenceValue(second.Evidence, "executor.before_checksum")
	after := evidenceValue(second.Evidence, "executor.after_checksum")
	if before == "" || after == "" {
		t.Fatalf("expected checksum evidence on second run, got %+v", second.Evidence)
	}
	if before != after {
		t.Fatalf("expected matching checksums on idempotent second run, got before=%q after=%q", before, after)
	}
}

func snapshotForRepo(root string) core.WorkspaceSnapshot {
	return core.WorkspaceSnapshot{Repository: core.RepositoryState{Root: root}}
}

func assertEvidenceValue(t *testing.T, evidence []core.EvidenceItem, name string, expected string) {
	t.Helper()
	value := evidenceValue(evidence, name)
	if value != expected {
		t.Fatalf("expected evidence %q=%q, got %q", name, expected, value)
	}
}

func evidenceValue(evidence []core.EvidenceItem, name string) string {
	for _, item := range evidence {
		if item.Name == name {
			return item.Value
		}
	}
	return ""
}
