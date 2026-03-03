package state

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

const (
	// CurrentVersion is the on-disk schema version for state.json.
	CurrentVersion = 1

	InvalidationReasonProjectRootChanged      = "project_root_changed"
	InvalidationReasonGitHeadChanged          = "git_head_changed"
	InvalidationReasonWorktreeFingerprintDiff = "worktree_fingerprint_changed"
)

// RunState captures mutable orchestration state persisted between runs.
type RunState struct {
	Version                 int               `json:"version"`
	SelectedProjectRoot     string            `json:"selectedProjectRoot"`
	SelectedModelPath       string            `json:"selectedModelPath,omitempty"`
	LastSnapshotID          string            `json:"lastSnapshotId,omitempty"`
	LastHead                string            `json:"lastHead,omitempty"`
	LastWorktreeFingerprint string            `json:"lastWorktreeFingerprint,omitempty"`
	LastPrimaryStep         core.EnsureStepID `json:"lastPrimaryStep,omitempty"`
	LastRunAt               time.Time         `json:"lastRunAt,omitempty"`
	InvalidationReasons     []string          `json:"invalidationReasons,omitempty"`
}

// DefaultRunState returns a schema-initialized state for projectRoot.
func DefaultRunState(projectRoot string) RunState {
	return RunState{
		Version:             CurrentVersion,
		SelectedProjectRoot: normalizeRoot(projectRoot),
	}
}

// ComputeInvalidationReasons compares persisted state to a fresh snapshot.
func ComputeInvalidationReasons(previous RunState, snapshot core.WorkspaceSnapshot, selectedProjectRoot string) []string {
	reasons := make([]string, 0, 3)

	previousRoot := normalizeRoot(previous.SelectedProjectRoot)
	currentRoot := normalizeRoot(selectedProjectRoot)
	if previousRoot != "" && currentRoot != "" && previousRoot != currentRoot {
		reasons = append(reasons, InvalidationReasonProjectRootChanged)
	}

	if previous.LastHead != "" && snapshot.Repository.Head != "" && previous.LastHead != snapshot.Repository.Head {
		reasons = append(reasons, InvalidationReasonGitHeadChanged)
	}

	if previous.LastWorktreeFingerprint != "" && snapshot.WorktreeFingerprint != "" && previous.LastWorktreeFingerprint != snapshot.WorktreeFingerprint {
		reasons = append(reasons, InvalidationReasonWorktreeFingerprintDiff)
	}

	return reasons
}

// UpdateForIteration builds next persisted state from one iteration report.
func UpdateForIteration(
	previous RunState,
	snapshot core.WorkspaceSnapshot,
	report core.IterationReport,
	selectedProjectRoot string,
	selectedModelPath string,
	invalidationReasons []string,
) RunState {
	next := previous
	next.Version = CurrentVersion
	next.SelectedProjectRoot = normalizeRoot(selectedProjectRoot)
	next.SelectedModelPath = normalizeModelPath(selectedModelPath)
	next.LastSnapshotID = snapshot.ID
	next.LastHead = snapshot.Repository.Head
	next.LastWorktreeFingerprint = snapshot.WorktreeFingerprint
	next.LastPrimaryStep = report.Step.ID
	next.LastRunAt = report.GeneratedAt
	next.InvalidationReasons = append([]string(nil), invalidationReasons...)
	return next
}

func normalizeModelPath(modelPath string) string {
	trimmed := strings.TrimSpace(modelPath)
	if trimmed == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))
}

func normalizeRoot(projectRoot string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return ""
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return filepath.Clean(root)
}
