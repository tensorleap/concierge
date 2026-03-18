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
		Version:             CurrentVersion,
		SelectedProjectRoot: root,
		LastBlockingIssues: []core.Issue{{
			Code:     core.IssueCodePreprocessExecutionFailed,
			Message:  "preprocess failed during Tensorleap parser validation: deprecated length",
			Severity: core.SeverityError,
			Scope:    core.IssueScopePreprocess,
			Location: &core.IssueLocation{Path: "datasetclasses.py", Line: 60},
		}},
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
		Repository:          core.RepositoryState{Head: "head-new"},
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

	state = UpdateForIteration(state, firstSnapshot, firstReport, root, "", nil, nil, nil)
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

	updated := UpdateForIteration(loaded, secondSnapshot, secondReport, root, "", nil, nil, reasons)
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

func TestUpdateForIterationPersistsOnlyBlockingValidationIssues(t *testing.T) {
	root := t.TempDir()
	snapshot := core.WorkspaceSnapshot{
		ID:                  "snapshot-1",
		Repository:          core.RepositoryState{Head: "head-1"},
		WorktreeFingerprint: "fp-1",
	}
	report := core.IterationReport{
		GeneratedAt: time.Date(2026, 3, 17, 20, 0, 0, 0, time.UTC),
		Step:        core.EnsureStep{ID: core.EnsureStepPreprocessContract},
		Validation: core.ValidationResult{
			Passed: false,
			Issues: []core.Issue{
				{
					Code:     core.IssueCodePreprocessExecutionFailed,
					Message:  "blocking preprocess failure",
					Severity: core.SeverityError,
					Scope:    core.IssueScopePreprocess,
				},
				{
					Code:     core.IssueCodeUnknown,
					Message:  "non-blocking note",
					Severity: core.SeverityInfo,
					Scope:    core.IssueScopeValidation,
				},
			},
		},
	}

	updated := UpdateForIteration(DefaultRunState(root), snapshot, report, root, "", nil, nil, nil)
	if len(updated.LastBlockingIssues) != 1 {
		t.Fatalf("expected one persisted blocking issue, got %+v", updated.LastBlockingIssues)
	}
	if updated.LastBlockingIssues[0].Message != "blocking preprocess failure" {
		t.Fatalf("unexpected persisted issue %+v", updated.LastBlockingIssues[0])
	}
}

func TestFreshBlockingValidationIssuesReturnsCloneWhenSnapshotMatches(t *testing.T) {
	previous := RunState{
		SelectedProjectRoot:     "/repo",
		LastHead:                "head-1",
		LastWorktreeFingerprint: "fp-1",
		LastBlockingIssues: []core.Issue{{
			Code:     core.IssueCodePreprocessExecutionFailed,
			Message:  "blocking preprocess failure",
			Severity: core.SeverityError,
			Scope:    core.IssueScopePreprocess,
			Location: &core.IssueLocation{Path: "leap_integration.py", Line: 42},
		}},
	}
	snapshot := core.WorkspaceSnapshot{
		Repository:          core.RepositoryState{Head: "head-1"},
		WorktreeFingerprint: "fp-1",
	}

	issues := FreshBlockingValidationIssues(previous, snapshot, "/repo")
	if len(issues) != 1 {
		t.Fatalf("expected one fresh blocking issue, got %+v", issues)
	}
	if issues[0].Message != "blocking preprocess failure" {
		t.Fatalf("unexpected issue %+v", issues[0])
	}
	issues[0].Message = "mutated copy"
	if previous.LastBlockingIssues[0].Message != "blocking preprocess failure" {
		t.Fatalf("expected persisted issues to stay immutable, got %+v", previous.LastBlockingIssues)
	}
}

func TestFreshBlockingValidationIssuesDropsPersistedIssuesWhenWorkspaceDrifts(t *testing.T) {
	previous := RunState{
		SelectedProjectRoot:     "/repo",
		LastHead:                "head-1",
		LastWorktreeFingerprint: "fp-1",
		LastBlockingIssues: []core.Issue{{
			Code:     core.IssueCodePreprocessExecutionFailed,
			Message:  "blocking preprocess failure",
			Severity: core.SeverityError,
			Scope:    core.IssueScopePreprocess,
		}},
	}
	snapshot := core.WorkspaceSnapshot{
		Repository:          core.RepositoryState{Head: "head-2"},
		WorktreeFingerprint: "fp-2",
	}

	issues := FreshBlockingValidationIssues(previous, snapshot, "/repo")
	if len(issues) != 0 {
		t.Fatalf("expected persisted blocking issues to be discarded after drift, got %+v", issues)
	}
}
