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

	InvalidationReasonProjectRootChanged          = "project_root_changed"
	InvalidationReasonGitHeadChanged              = "git_head_changed"
	InvalidationReasonWorktreeFingerprintDiff     = "worktree_fingerprint_changed"
	InvalidationReasonRuntimePyProjectChanged     = "runtime_pyproject_changed"
	InvalidationReasonRuntimePoetryLockChanged    = "runtime_poetry_lock_changed"
	InvalidationReasonRuntimeInterpreterChanged   = "runtime_interpreter_changed"
	InvalidationReasonRuntimePythonVersionChanged = "runtime_python_version_changed"
)

// RunState captures mutable orchestration state persisted between runs.
type RunState struct {
	Version                 int                          `json:"version"`
	SelectedProjectRoot     string                       `json:"selectedProjectRoot"`
	SelectedModelPath       string                       `json:"selectedModelPath,omitempty"`
	ConfirmedEncoderMapping *core.EncoderMappingContract `json:"confirmedEncoderMapping,omitempty"`
	RuntimeProfile          *core.LocalRuntimeProfile    `json:"runtimeProfile,omitempty"`
	LastBlockingIssues      []core.Issue                 `json:"lastBlockingIssues,omitempty"`
	LastSnapshotID          string                       `json:"lastSnapshotId,omitempty"`
	LastHead                string                       `json:"lastHead,omitempty"`
	LastWorktreeFingerprint string                       `json:"lastWorktreeFingerprint,omitempty"`
	LastPrimaryStep         core.EnsureStepID            `json:"lastPrimaryStep,omitempty"`
	LastRunAt               time.Time                    `json:"lastRunAt,omitempty"`
	InvalidationReasons     []string                     `json:"invalidationReasons,omitempty"`
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
	reasons := make([]string, 0, 7)

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

	if previous.RuntimeProfile != nil {
		currentPyProjectHash := strings.TrimSpace(snapshot.FileHashes["pyproject.toml"])
		currentPoetryLockHash := strings.TrimSpace(snapshot.FileHashes["poetry.lock"])
		currentInterpreter := ""
		currentPythonVersion := ""
		if snapshot.RuntimeProfile != nil {
			currentInterpreter = strings.TrimSpace(snapshot.RuntimeProfile.InterpreterPath)
			currentPythonVersion = strings.TrimSpace(snapshot.RuntimeProfile.PythonVersion)
		}

		if previous.RuntimeProfile.Fingerprint.PyProjectHash != "" &&
			currentPyProjectHash != "" &&
			previous.RuntimeProfile.Fingerprint.PyProjectHash != currentPyProjectHash {
			reasons = append(reasons, InvalidationReasonRuntimePyProjectChanged)
		}
		if previous.RuntimeProfile.Fingerprint.PoetryLockHash != "" &&
			currentPoetryLockHash != "" &&
			previous.RuntimeProfile.Fingerprint.PoetryLockHash != currentPoetryLockHash {
			reasons = append(reasons, InvalidationReasonRuntimePoetryLockChanged)
		}
		if previous.RuntimeProfile.InterpreterPath != "" &&
			currentInterpreter != "" &&
			previous.RuntimeProfile.InterpreterPath != currentInterpreter {
			reasons = append(reasons, InvalidationReasonRuntimeInterpreterChanged)
		}
		if previous.RuntimeProfile.PythonVersion != "" &&
			currentPythonVersion != "" &&
			previous.RuntimeProfile.PythonVersion != currentPythonVersion {
			reasons = append(reasons, InvalidationReasonRuntimePythonVersionChanged)
		}
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
	confirmedMapping *core.EncoderMappingContract,
	runtimeProfile *core.LocalRuntimeProfile,
	invalidationReasons []string,
) RunState {
	next := previous
	next.Version = CurrentVersion
	next.SelectedProjectRoot = normalizeRoot(selectedProjectRoot)
	next.SelectedModelPath = normalizeModelPath(selectedModelPath)
	next.ConfirmedEncoderMapping = cloneEncoderMappingContract(confirmedMapping)
	next.RuntimeProfile = cloneRuntimeProfile(runtimeProfile)
	next.LastBlockingIssues = filterBlockingIssues(report.Validation.Issues)
	next.LastSnapshotID = snapshot.ID
	next.LastHead = snapshot.Repository.Head
	next.LastWorktreeFingerprint = snapshot.WorktreeFingerprint
	next.LastPrimaryStep = report.Step.ID
	next.LastRunAt = report.GeneratedAt
	next.InvalidationReasons = append([]string(nil), invalidationReasons...)
	return next
}

// FreshBlockingValidationIssues returns the last known blocking validation
// issues only when the current snapshot still matches the previously persisted
// workspace identity.
func FreshBlockingValidationIssues(previous RunState, snapshot core.WorkspaceSnapshot, selectedProjectRoot string) []core.Issue {
	if len(previous.LastBlockingIssues) == 0 {
		return nil
	}
	if len(ComputeInvalidationReasons(previous, snapshot, selectedProjectRoot)) > 0 {
		return nil
	}
	return cloneIssues(previous.LastBlockingIssues)
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

func cloneEncoderMappingContract(mapping *core.EncoderMappingContract) *core.EncoderMappingContract {
	if mapping == nil {
		return nil
	}
	cloned := *mapping
	if len(mapping.InputSymbols) > 0 {
		cloned.InputSymbols = append([]string(nil), mapping.InputSymbols...)
	}
	if len(mapping.GroundTruthSymbols) > 0 {
		cloned.GroundTruthSymbols = append([]string(nil), mapping.GroundTruthSymbols...)
	}
	if len(mapping.Notes) > 0 {
		cloned.Notes = append([]string(nil), mapping.Notes...)
	}
	return &cloned
}

func cloneRuntimeProfile(profile *core.LocalRuntimeProfile) *core.LocalRuntimeProfile {
	if profile == nil {
		return nil
	}
	cloned := *profile
	cloned.Fingerprint = profile.Fingerprint
	return &cloned
}

func filterBlockingIssues(issues []core.Issue) []core.Issue {
	if len(issues) == 0 {
		return nil
	}
	filtered := make([]core.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity != core.SeverityError {
			continue
		}
		filtered = append(filtered, cloneIssue(issue))
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func cloneIssues(issues []core.Issue) []core.Issue {
	if len(issues) == 0 {
		return nil
	}
	cloned := make([]core.Issue, 0, len(issues))
	for _, issue := range issues {
		cloned = append(cloned, cloneIssue(issue))
	}
	return cloned
}

func cloneIssue(issue core.Issue) core.Issue {
	cloned := issue
	if issue.Location != nil {
		location := *issue.Location
		cloned.Location = &location
	}
	return cloned
}
