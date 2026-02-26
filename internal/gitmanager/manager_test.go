package gitmanager

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestGitManagerRejectsMainBranchCommit(t *testing.T) {
	repo := initGitRepo(t)
	runGit(t, repo, "checkout", "-B", "main")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "changed\n")

	manager := NewManager(func(step core.EnsureStep, diffSummary string) (bool, error) {
		_ = step
		_ = diffSummary
		return true, nil
	})

	_, err := manager.Handle(context.Background(), core.WorkspaceSnapshot{Repository: core.RepositoryState{Root: repo}}, executionResult(core.EnsureStepLeapYAML))
	if err == nil {
		t.Fatal("expected protected-branch error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "protected branch") {
		t.Fatalf("expected protected branch error, got: %v", err)
	}
}

func TestGitManagerApproveCreatesCommit(t *testing.T) {
	repo := initGitRepo(t)
	runGit(t, repo, "checkout", "-B", "feature/test")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "changed\n")

	manager := NewManager(func(step core.EnsureStep, diffSummary string) (bool, error) {
		_ = step
		if strings.TrimSpace(diffSummary) == "" {
			t.Fatalf("expected non-empty diff summary")
		}
		return true, nil
	})

	decision, err := manager.Handle(context.Background(), core.WorkspaceSnapshot{Repository: core.RepositoryState{Root: repo}}, executionResult(core.EnsureStepLeapYAML))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if decision.Commit == nil {
		t.Fatal("expected commit metadata when approved")
	}
	if decision.Commit.Hash == "" {
		t.Fatal("expected commit hash")
	}
	if !strings.HasPrefix(decision.Commit.Message, "concierge(ensure.leap_yaml):") {
		t.Fatalf("unexpected commit message %q", decision.Commit.Message)
	}

	latestMessage := runGit(t, repo, "log", "-1", "--pretty=%s")
	if latestMessage != decision.Commit.Message {
		t.Fatalf("expected latest commit message %q, got %q", decision.Commit.Message, latestMessage)
	}

	status := runGit(t, repo, "status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Fatalf("expected clean worktree after commit, got %q", status)
	}
}

func TestGitManagerRejectRestoresTree(t *testing.T) {
	repo := initGitRepo(t)
	runGit(t, repo, "checkout", "-B", "feature/test")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "changed\n")
	writeFile(t, filepath.Join(repo, "new.txt"), "new\n")

	manager := NewManager(func(step core.EnsureStep, diffSummary string) (bool, error) {
		_ = step
		_ = diffSummary
		return false, nil
	})

	decision, err := manager.Handle(context.Background(), core.WorkspaceSnapshot{Repository: core.RepositoryState{Root: repo}}, executionResult(core.EnsureStepIntegrationScript))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if decision.Commit != nil {
		t.Fatalf("expected no commit on rejection, got %+v", decision.Commit)
	}
	if decision.FinalResult.Applied {
		t.Fatal("expected final applied=false after rejection")
	}

	status := runGit(t, repo, "status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Fatalf("expected clean worktree after rejection restore, got %q", status)
	}

	trackedContents, err := os.ReadFile(filepath.Join(repo, "tracked.txt"))
	if err != nil {
		t.Fatalf("ReadFile tracked.txt failed: %v", err)
	}
	if string(trackedContents) != "initial\n" {
		t.Fatalf("expected tracked file to be restored, got %q", string(trackedContents))
	}
	if _, err := os.Stat(filepath.Join(repo, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected new.txt to be removed on rejection, got err=%v", err)
	}
}

func executionResult(stepID core.EnsureStepID) core.ExecutionResult {
	step, _ := core.EnsureStepByID(stepID)
	return core.ExecutionResult{
		Step:    step,
		Applied: true,
		Summary: "apply step changes",
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.name", "Concierge Test")
	runGit(t, repo, "config", "user.email", "concierge@example.com")
	runGit(t, repo, "config", "commit.gpgsign", "false")

	writeFile(t, filepath.Join(repo, "tracked.txt"), "initial\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")

	return repo
}

func runGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed in %s: %v\n%s", strings.Join(args, " "), repo, err, output)
	}
	return strings.TrimSpace(string(output))
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", path, err)
	}
}
