package persistence

import (
	"path/filepath"
	"testing"
)

func TestPathsBuilderReturnsExpectedLayout(t *testing.T) {
	root := t.TempDir()
	paths, err := NewPaths(root)
	if err != nil {
		t.Fatalf("NewPaths returned error: %v", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}

	if paths.ProjectRoot() != absRoot {
		t.Fatalf("expected project root %q, got %q", absRoot, paths.ProjectRoot())
	}
	if got := paths.StateFile(); got != filepath.Join(absRoot, ".concierge", "state", "state.json") {
		t.Fatalf("unexpected state file path: %q", got)
	}
	if got := paths.ReportFile("snapshot-123"); got != filepath.Join(absRoot, ".concierge", "reports", "snapshot-123.json") {
		t.Fatalf("unexpected report file path: %q", got)
	}
	if got := paths.EvidenceFile("snapshot-123", "executor/mode"); got != filepath.Join(absRoot, ".concierge", "evidence", "snapshot-123", "executor_mode.log") {
		t.Fatalf("unexpected evidence file path: %q", got)
	}
}

func TestNewPathsRequiresProjectRoot(t *testing.T) {
	if _, err := NewPaths(" "); err == nil {
		t.Fatal("expected error for empty project root")
	}
}
