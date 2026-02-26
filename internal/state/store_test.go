package state

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

func TestLoadStateReturnsDefaultWhenMissing(t *testing.T) {
	root := t.TempDir()

	loaded, err := LoadState(root)
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}

	if loaded.Version != CurrentVersion {
		t.Fatalf("expected version %d, got %d", CurrentVersion, loaded.Version)
	}
	expectedRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}
	if loaded.SelectedProjectRoot != expectedRoot {
		t.Fatalf("expected selected project root %q, got %q", expectedRoot, loaded.SelectedProjectRoot)
	}
	if loaded.LastSnapshotID != "" {
		t.Fatalf("expected empty snapshot ID, got %q", loaded.LastSnapshotID)
	}
}

func TestSaveStateAtomicRoundTrip(t *testing.T) {
	root := t.TempDir()
	fixedRunAt := time.Date(2026, 2, 26, 8, 0, 0, 0, time.UTC)

	input := RunState{
		Version:                 CurrentVersion,
		SelectedProjectRoot:     root,
		LastSnapshotID:          "snapshot-1",
		LastHead:                "head-1",
		LastWorktreeFingerprint: "fp-1",
		LastPrimaryStep:         core.EnsureStepLeapYAML,
		LastRunAt:               fixedRunAt,
		InvalidationReasons:     []string{InvalidationReasonGitHeadChanged},
	}

	if err := SaveState(root, input); err != nil {
		t.Fatalf("SaveState returned error: %v", err)
	}

	loaded, err := LoadState(root)
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}

	expectedRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}
	input.SelectedProjectRoot = expectedRoot
	if !reflect.DeepEqual(loaded, input) {
		t.Fatalf("expected loaded state %+v, got %+v", input, loaded)
	}
}

func TestInvalidationReasonsOnHeadAndWorktreeChange(t *testing.T) {
	previous := RunState{
		SelectedProjectRoot:     "/repo/old",
		LastHead:                "head-old",
		LastWorktreeFingerprint: "fingerprint-old",
	}

	snapshot := core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Head: "head-new"},
		WorktreeFingerprint: "fingerprint-new",
	}

	reasons := ComputeInvalidationReasons(previous, snapshot, "/repo/new")
	expected := []string{
		InvalidationReasonProjectRootChanged,
		InvalidationReasonGitHeadChanged,
		InvalidationReasonWorktreeFingerprintDiff,
	}

	if !reflect.DeepEqual(reasons, expected) {
		t.Fatalf("expected reasons %v, got %v", expected, reasons)
	}
}

func TestStatePersistsAcrossMultipleIterations(t *testing.T) {
	root := t.TempDir()

	state := DefaultRunState(root)
	firstSnapshot := core.WorkspaceSnapshot{
		ID:                  "snapshot-1",
		Repository:          core.RepositoryState{Head: "head-1"},
		WorktreeFingerprint: "fp-1",
	}
	firstReport := core.IterationReport{
		GeneratedAt: time.Date(2026, 2, 26, 9, 0, 0, 0, time.UTC),
		Step:        core.EnsureStep{ID: core.EnsureStepLeapYAML},
	}

	state = UpdateForIteration(state, firstSnapshot, firstReport, root, nil)
	if err := SaveState(root, state); err != nil {
		t.Fatalf("SaveState first iteration failed: %v", err)
	}

	loaded, err := LoadState(root)
	if err != nil {
		t.Fatalf("LoadState first iteration failed: %v", err)
	}
	if loaded.LastSnapshotID != "snapshot-1" {
		t.Fatalf("expected last snapshot %q, got %q", "snapshot-1", loaded.LastSnapshotID)
	}

	secondSnapshot := core.WorkspaceSnapshot{
		ID:                  "snapshot-2",
		Repository:          core.RepositoryState{Head: "head-2"},
		WorktreeFingerprint: "fp-2",
	}
	reasons := ComputeInvalidationReasons(loaded, secondSnapshot, root)
	secondReport := core.IterationReport{
		GeneratedAt: time.Date(2026, 2, 26, 9, 5, 0, 0, time.UTC),
		Step:        core.EnsureStep{ID: core.EnsureStepIntegrationScript},
	}

	updated := UpdateForIteration(loaded, secondSnapshot, secondReport, root, reasons)
	if err := SaveState(root, updated); err != nil {
		t.Fatalf("SaveState second iteration failed: %v", err)
	}

	loadedAgain, err := LoadState(root)
	if err != nil {
		t.Fatalf("LoadState second iteration failed: %v", err)
	}
	if loadedAgain.LastSnapshotID != "snapshot-2" {
		t.Fatalf("expected last snapshot %q, got %q", "snapshot-2", loadedAgain.LastSnapshotID)
	}
	if loadedAgain.LastPrimaryStep != core.EnsureStepIntegrationScript {
		t.Fatalf("expected last primary step %q, got %q", core.EnsureStepIntegrationScript, loadedAgain.LastPrimaryStep)
	}
	if !reflect.DeepEqual(loadedAgain.InvalidationReasons, []string{InvalidationReasonGitHeadChanged, InvalidationReasonWorktreeFingerprintDiff}) {
		t.Fatalf("expected invalidation reasons to persist, got %v", loadedAgain.InvalidationReasons)
	}
}
