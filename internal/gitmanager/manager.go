package gitmanager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

// ApprovalFunc decides whether to approve committing the current diff.
type ApprovalFunc func(step core.EnsureStep, diffSummary string) (bool, error)

// Manager enforces branch safety and audited commit/reject flow.
type Manager struct {
	approve    ApprovalFunc
	runGit     func(ctx context.Context, dir string, args ...string) (string, error)
	removePath func(path string) error
}

// NewManager creates a git manager with approval callback.
func NewManager(approve ApprovalFunc) *Manager {
	if approve == nil {
		approve = func(step core.EnsureStep, diffSummary string) (bool, error) {
			_ = step
			_ = diffSummary
			return false, nil
		}
	}

	return &Manager{
		approve:    approve,
		runGit:     runGitCombined,
		removePath: os.RemoveAll,
	}
}

// Handle executes diff review, branch guard, and approved commit/reject restoration.
func (m *Manager) Handle(ctx context.Context, snapshot core.WorkspaceSnapshot, result core.ExecutionResult) (core.GitDecision, error) {
	decision := core.GitDecision{FinalResult: result}
	if !result.Applied {
		return decision, nil
	}

	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return core.GitDecision{}, core.NewError(core.KindUnknown, "gitmanager.handle.repo_root", "snapshot repository root is empty")
	}

	statusPorcelain, err := m.runGit(ctx, repoRoot, "status", "--porcelain")
	if err != nil {
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.status", err)
	}
	if strings.TrimSpace(statusPorcelain) == "" {
		return decision, nil
	}

	branch, err := m.runGit(ctx, repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.branch", err)
	}
	branch = strings.TrimSpace(branch)
	if branch == "main" || branch == "master" {
		return core.GitDecision{}, core.NewError(
			core.KindUnknown,
			"gitmanager.handle.protected_branch",
			"refusing to commit on protected branch main/master",
		)
	}

	diffSummary, err := m.runGit(ctx, repoRoot, "diff", "--stat")
	if err != nil {
		diffSummary = statusPorcelain
	}
	if strings.TrimSpace(diffSummary) == "" {
		diffSummary = statusPorcelain
	}

	approved, err := m.approve(result.Step, diffSummary)
	if err != nil {
		if restoreErr := m.restoreWorkingTree(ctx, repoRoot); restoreErr != nil {
			return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.approval_restore", restoreErr)
		}
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.approval", err)
	}

	decision.Evidence = append(decision.Evidence,
		core.EvidenceItem{Name: "git.diff_summary", Value: strings.TrimSpace(diffSummary)},
	)

	if !approved {
		if err := m.restoreWorkingTree(ctx, repoRoot); err != nil {
			return core.GitDecision{}, err
		}
		decision.FinalResult = core.ExecutionResult{
			Step:     result.Step,
			Applied:  false,
			Summary:  "changes rejected and restored",
			Evidence: append([]core.EvidenceItem(nil), result.Evidence...),
		}
		decision.Notes = append(decision.Notes, fmt.Sprintf("changes for %s were rejected and reverted", result.Step.ID))
		decision.Evidence = append(decision.Evidence, core.EvidenceItem{Name: "git.approval", Value: "rejected"})
		return decision, nil
	}

	message := CommitMessage(result.Step, result.Summary)
	if _, err := m.runGit(ctx, repoRoot, "add", "-A"); err != nil {
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.add", err)
	}
	if _, err := m.runGit(ctx, repoRoot, "commit", "-m", message); err != nil {
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.commit", err)
	}

	hash, err := m.runGit(ctx, repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.commit_hash", err)
	}
	hash = strings.TrimSpace(hash)

	decision.Commit = &core.CommitMetadata{Hash: hash, Message: message}
	decision.Notes = append(decision.Notes, fmt.Sprintf("changes committed on branch %s", branch))
	decision.Evidence = append(decision.Evidence,
		core.EvidenceItem{Name: "git.approval", Value: "approved"},
		core.EvidenceItem{Name: "git.commit_hash", Value: hash},
	)

	return decision, nil
}

func (m *Manager) restoreWorkingTree(ctx context.Context, repoRoot string) error {
	if _, err := m.runGit(ctx, repoRoot, "restore", "--staged", "--worktree", "--", "."); err != nil {
		return core.WrapError(core.KindUnknown, "gitmanager.restore.restore", err)
	}

	untracked, err := m.runGit(ctx, repoRoot, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return core.WrapError(core.KindUnknown, "gitmanager.restore.untracked", err)
	}

	for _, relativePath := range strings.Split(strings.TrimSpace(untracked), "\n") {
		rel := strings.TrimSpace(relativePath)
		if rel == "" {
			continue
		}
		cleanRel := filepath.Clean(rel)
		if cleanRel == "." || strings.HasPrefix(cleanRel, "..") {
			continue
		}
		if err := m.removePath(filepath.Join(repoRoot, cleanRel)); err != nil {
			return core.WrapError(core.KindUnknown, "gitmanager.restore.remove_path", err)
		}
	}

	return nil
}

func runGitCombined(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), trimmed)
	}
	return trimmed, nil
}

func IsProtectedBranchError(err error) bool {
	if err == nil {
		return false
	}
	return core.KindOf(err) == core.KindUnknown && strings.Contains(strings.ToLower(err.Error()), "protected branch")
}

func IsApprovalRejected(decision core.GitDecision) bool {
	for _, evidence := range decision.Evidence {
		if evidence.Name == "git.approval" && evidence.Value == "rejected" {
			return true
		}
	}
	return false
}
