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

	manager := NewManager(func(step core.EnsureStep, review ChangeReview) (ReviewDecision, error) {
		_ = step
		_ = review
		return ReviewDecision{KeepChanges: true, Commit: true}, nil
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

	manager := NewManager(func(step core.EnsureStep, review ChangeReview) (ReviewDecision, error) {
		_ = step
		if strings.TrimSpace(review.Stat) == "" {
			t.Fatalf("expected non-empty diff summary")
		}
		if strings.TrimSpace(review.Patch) == "" {
			t.Fatalf("expected non-empty patch output")
		}
		return ReviewDecision{KeepChanges: true, Commit: true}, nil
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

	manager := NewManager(func(step core.EnsureStep, review ChangeReview) (ReviewDecision, error) {
		_ = step
		_ = review
		return ReviewDecision{}, nil
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

func TestGitManagerSkipsConciergeOnlyChanges(t *testing.T) {
	repo := initGitRepo(t)
	runGit(t, repo, "checkout", "-B", "feature/test")
	writeFile(t, filepath.Join(repo, ".concierge", "reports", "snapshot.json"), "{}\n")

	approvalCalled := false
	manager := NewManager(func(step core.EnsureStep, review ChangeReview) (ReviewDecision, error) {
		_ = step
		_ = review
		approvalCalled = true
		return ReviewDecision{KeepChanges: true, Commit: true}, nil
	})

	decision, err := manager.Handle(context.Background(), core.WorkspaceSnapshot{Repository: core.RepositoryState{Root: repo}}, executionResult(core.EnsureStepIntegrationScript))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if approvalCalled {
		t.Fatal("expected approval callback to be skipped for .concierge-only changes")
	}
	if decision.Commit != nil {
		t.Fatalf("expected no commit metadata for .concierge-only changes, got %+v", decision.Commit)
	}
}

func TestGitManagerCommitExcludesConciergeArtifacts(t *testing.T) {
	repo := initGitRepo(t)
	runGit(t, repo, "checkout", "-B", "feature/test")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "changed\n")
	writeFile(t, filepath.Join(repo, ".concierge", "reports", "snapshot.json"), "{}\n")

	manager := NewManager(func(step core.EnsureStep, review ChangeReview) (ReviewDecision, error) {
		_ = step
		if strings.Contains(review.Stat, ".concierge") {
			t.Fatalf("expected diff summary to exclude .concierge, got %q", review.Stat)
		}
		if strings.Contains(review.Patch, ".concierge") {
			t.Fatalf("expected patch to exclude .concierge, got %q", review.Patch)
		}
		for _, file := range review.Files {
			if strings.Contains(file, ".concierge") {
				t.Fatalf("expected changed files list to exclude .concierge, got %v", review.Files)
			}
		}
		return ReviewDecision{KeepChanges: true, Commit: true}, nil
	})

	decision, err := manager.Handle(context.Background(), core.WorkspaceSnapshot{Repository: core.RepositoryState{Root: repo}}, executionResult(core.EnsureStepLeapYAML))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if decision.Commit == nil {
		t.Fatal("expected commit metadata when approved")
	}

	committedFiles := runGit(t, repo, "show", "--pretty=format:", "--name-only", "HEAD")
	if strings.Contains(committedFiles, ".concierge") {
		t.Fatalf("expected commit to exclude .concierge artifacts, got %q", committedFiles)
	}
	if !strings.Contains(committedFiles, "tracked.txt") {
		t.Fatalf("expected commit to include tracked.txt, got %q", committedFiles)
	}

	status := runGit(t, repo, "status", "--short")
	if !strings.Contains(status, "?? .concierge/") {
		t.Fatalf("expected .concierge artifacts to remain unstaged/uncommitted, got %q", status)
	}
}

func TestGitManagerRejectKeepsConciergeArtifacts(t *testing.T) {
	repo := initGitRepo(t)
	runGit(t, repo, "checkout", "-B", "feature/test")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "changed\n")
	writeFile(t, filepath.Join(repo, "new.txt"), "new\n")
	conciergeReport := filepath.Join(repo, ".concierge", "reports", "snapshot.json")
	writeFile(t, conciergeReport, "{}\n")

	manager := NewManager(func(step core.EnsureStep, review ChangeReview) (ReviewDecision, error) {
		_ = step
		_ = review
		return ReviewDecision{}, nil
	})

	decision, err := manager.Handle(context.Background(), core.WorkspaceSnapshot{Repository: core.RepositoryState{Root: repo}}, executionResult(core.EnsureStepIntegrationScript))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if decision.FinalResult.Applied {
		t.Fatal("expected final applied=false after rejection")
	}
	if _, err := os.Stat(conciergeReport); err != nil {
		t.Fatalf("expected .concierge artifact to remain after rejection, got stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected new.txt to be removed on rejection, got err=%v", err)
	}

	trackedContents, err := os.ReadFile(filepath.Join(repo, "tracked.txt"))
	if err != nil {
		t.Fatalf("ReadFile tracked.txt failed: %v", err)
	}
	if string(trackedContents) != "initial\n" {
		t.Fatalf("expected tracked file to be restored, got %q", string(trackedContents))
	}
}

func TestGitManagerKeepsUncommittedChangesForReview(t *testing.T) {
	repo := initGitRepo(t)
	runGit(t, repo, "checkout", "-B", "feature/test")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "changed\n")

	manager := NewManager(func(step core.EnsureStep, review ChangeReview) (ReviewDecision, error) {
		_ = step
		_ = review
		return ReviewDecision{KeepChanges: true, Commit: false}, nil
	})

	decision, err := manager.Handle(context.Background(), core.WorkspaceSnapshot{Repository: core.RepositoryState{Root: repo}}, executionResult(core.EnsureStepLeapYAML))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if decision.Commit != nil {
		t.Fatalf("expected no commit metadata when commit is deferred, got %+v", decision.Commit)
	}
	if !decision.FinalResult.Applied {
		t.Fatalf("expected applied result to remain true, got %+v", decision.FinalResult)
	}
	if !hasEvidence(decision.Evidence, "git.commit_pending_review", "true") {
		t.Fatalf("expected pending-review evidence, got %+v", decision.Evidence)
	}

	status := runGit(t, repo, "status", "--porcelain")
	if !strings.Contains(status, "M tracked.txt") {
		t.Fatalf("expected modified file to remain in working tree, got %q", status)
	}

	latestMessage := runGit(t, repo, "log", "-1", "--pretty=%s")
	if latestMessage != "initial" {
		t.Fatalf("expected no new commit, got latest message %q", latestMessage)
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll failed for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", path, err)
	}
}

func hasEvidence(items []core.EvidenceItem, name, value string) bool {
	for _, item := range items {
		if item.Name == name && item.Value == value {
			return true
		}
	}
	return false
}
