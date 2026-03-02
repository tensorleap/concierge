package snapshot

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

func TestSnapshotCapturesGitIdentity(t *testing.T) {
	repo := initGitRepo(t)
	fixedNow := time.Date(2026, 2, 25, 16, 0, 0, 0, time.UTC)

	snapshotter := NewGitSnapshotter()
	snapshotter.clock = func() time.Time { return fixedNow }

	snapshot, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repo})
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}

	absRepo := canonicalPath(t, repo)

	if snapshot.Repository.Root != absRepo {
		t.Fatalf("expected root %q, got %q", absRepo, snapshot.Repository.Root)
	}
	if snapshot.Repository.GitRoot != absRepo {
		t.Fatalf("expected git root %q, got %q", absRepo, snapshot.Repository.GitRoot)
	}
	if snapshot.Repository.Branch == "" {
		t.Fatal("expected non-empty branch")
	}
	if snapshot.Repository.Head == "" {
		t.Fatal("expected non-empty head")
	}
	if snapshot.WorktreeFingerprint == "" {
		t.Fatal("expected non-empty worktree fingerprint")
	}
	if snapshot.ID == "" {
		t.Fatal("expected non-empty snapshot ID")
	}
	if snapshot.Repository.Dirty {
		t.Fatal("expected clean repository")
	}
	if !snapshot.CapturedAt.Equal(fixedNow) {
		t.Fatalf("expected capturedAt %s, got %s", fixedNow, snapshot.CapturedAt)
	}
}

func TestSnapshotWorktreeFingerprintChangesWhenWorkingTreeChanges(t *testing.T) {
	repo := initGitRepo(t)
	snapshotter := NewGitSnapshotter()

	first, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repo})
	if err != nil {
		t.Fatalf("first snapshot failed: %v", err)
	}

	writeFile(t, filepath.Join(repo, "tracked.txt"), "changed without commit\n")

	second, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repo})
	if err != nil {
		t.Fatalf("second snapshot failed: %v", err)
	}

	if first.WorktreeFingerprint == second.WorktreeFingerprint {
		t.Fatalf("expected worktree fingerprint to change, got %q", first.WorktreeFingerprint)
	}
	if !second.Repository.Dirty {
		t.Fatal("expected dirty repository after tracked file change")
	}
	if first.ID == second.ID {
		t.Fatalf("expected snapshot ID to change with worktree change, got %q", first.ID)
	}
}

func TestSnapshotIDStableForSameState(t *testing.T) {
	repo := initGitRepo(t)
	snapshotter := NewGitSnapshotter()

	first, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repo})
	if err != nil {
		t.Fatalf("first snapshot failed: %v", err)
	}

	second, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repo})
	if err != nil {
		t.Fatalf("second snapshot failed: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected stable snapshot ID, got %q then %q", first.ID, second.ID)
	}
	if first.WorktreeFingerprint != second.WorktreeFingerprint {
		t.Fatalf("expected stable worktree fingerprint, got %q then %q", first.WorktreeFingerprint, second.WorktreeFingerprint)
	}
}

func TestSnapshotIDChangesOnHeadChange(t *testing.T) {
	repo := initGitRepo(t)
	snapshotter := NewGitSnapshotter()

	first, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repo})
	if err != nil {
		t.Fatalf("first snapshot failed: %v", err)
	}

	writeFile(t, filepath.Join(repo, "tracked.txt"), "changed and committed\n")
	commitAll(t, repo, "update tracked file")

	second, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repo})
	if err != nil {
		t.Fatalf("second snapshot failed: %v", err)
	}

	if first.Repository.Head == second.Repository.Head {
		t.Fatalf("expected head to change, got %q", first.Repository.Head)
	}
	if first.ID == second.ID {
		t.Fatalf("expected snapshot ID to change on new commit, got %q", first.ID)
	}
	if second.Repository.Dirty {
		t.Fatal("expected clean repository after commit")
	}
}

func TestSnapshotUsesRequestRepoRoot(t *testing.T) {
	repo := initGitRepo(t)
	snapshotter := NewGitSnapshotter()
	snapshotter.getwd = func() (string, error) {
		return t.TempDir(), nil
	}

	snapshot, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repo})
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	absRepo := canonicalPath(t, repo)
	if snapshot.Repository.Root != absRepo {
		t.Fatalf("expected snapshot root %q, got %q", absRepo, snapshot.Repository.Root)
	}
}

func TestSnapshotErrorsOutsideGitRepo(t *testing.T) {
	notRepo := t.TempDir()
	snapshotter := NewGitSnapshotter()

	_, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: notRepo})
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}

	if kind := core.KindOf(err); kind != core.KindNotGitRepo {
		t.Fatalf("expected kind %q, got %q (err=%v)", core.KindNotGitRepo, kind, err)
	}
	if !strings.Contains(err.Error(), "git rev-parse --show-toplevel") {
		t.Fatalf("expected error to include failing command, got: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not a git repository") {
		t.Fatalf("expected error to include stderr details, got: %v", err)
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

	writeFile(t, filepath.Join(repo, "tracked.txt"), "initial content\n")
	commitAll(t, repo, "initial commit")

	return repo
}

func commitAll(t *testing.T, repo string, message string) {
	t.Helper()
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", message)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", path, err)
	}
}

func canonicalPath(t *testing.T, path string) string {
	t.Helper()

	absPath, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("filepath.Abs failed for %q: %v", path, err)
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}
	return absPath
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

func TestSnapshotResolveRootErrorIsTyped(t *testing.T) {
	snapshotter := NewGitSnapshotter()
	snapshotter.getwd = func() (string, error) {
		return "", errors.New("cwd unavailable")
	}

	_, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if kind := core.KindOf(err); kind != core.KindUnknown {
		t.Fatalf("expected kind %q, got %q", core.KindUnknown, kind)
	}
}

func TestSnapshotIncludesEnvironmentFingerprints(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "requirements.txt"), "numpy\n")

	snapshotter := NewGitSnapshotter()
	snapshotter.lookPath = func(file string) (string, error) {
		switch file {
		case "python3":
			return "/usr/bin/python3", nil
		case "leap":
			return "/usr/local/bin/leap", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	snapshotter.runCommand = func(ctx context.Context, dir string, name string, args ...string) ([]byte, []byte, error) {
		_ = ctx
		_ = dir
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "python3 --version":
			return []byte("Python 3.11.8\n"), nil, nil
		case "leap --version":
			return []byte("leap v0.2.0\n"), nil, nil
		case "leap auth whoami":
			return []byte("concierge@example.com\n"), nil, nil
		case "leap server info":
			return []byte("datasetvolumes: []\n"), nil, nil
		default:
			return nil, []byte("unknown command"), errors.New("command failed")
		}
	}

	snapshotValue, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repo})
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}

	if !snapshotValue.Runtime.PythonFound {
		t.Fatal("expected Python to be marked as found")
	}
	if snapshotValue.Runtime.PythonExecutable != "python3" {
		t.Fatalf("expected python executable %q, got %q", "python3", snapshotValue.Runtime.PythonExecutable)
	}
	if snapshotValue.Runtime.PythonVersion != "Python 3.11.8" {
		t.Fatalf("expected python version %q, got %q", "Python 3.11.8", snapshotValue.Runtime.PythonVersion)
	}
	if len(snapshotValue.Runtime.RequirementsFiles) != 1 || snapshotValue.Runtime.RequirementsFiles[0] != "requirements.txt" {
		t.Fatalf("expected requirements file detection, got %+v", snapshotValue.Runtime.RequirementsFiles)
	}

	if !snapshotValue.LeapCLI.Available {
		t.Fatal("expected leap CLI availability to be true")
	}
	if snapshotValue.LeapCLI.Version != "leap v0.2.0" {
		t.Fatalf("expected leap version %q, got %q", "leap v0.2.0", snapshotValue.LeapCLI.Version)
	}
	if !snapshotValue.LeapCLI.Authenticated {
		t.Fatal("expected leap auth probe to pass")
	}
	if !snapshotValue.LeapCLI.ServerInfoReachable {
		t.Fatal("expected leap server info probe to pass")
	}

	if snapshotValue.FileHashes["requirements.txt"] == "" {
		t.Fatalf("expected requirements.txt hash in snapshot, got %+v", snapshotValue.FileHashes)
	}
}

func TestSnapshotLeapVersionProbeDoesNotFallbackToLegacyVersionCommand(t *testing.T) {
	repo := initGitRepo(t)

	snapshotter := NewGitSnapshotter()
	snapshotter.lookPath = func(file string) (string, error) {
		switch file {
		case "python3":
			return "/usr/bin/python3", nil
		case "leap":
			return "/usr/local/bin/leap", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	snapshotter.runCommand = func(ctx context.Context, dir string, name string, args ...string) ([]byte, []byte, error) {
		_ = ctx
		_ = dir
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "python3 --version":
			return []byte("Python 3.11.8\n"), nil, nil
		case "leap --version":
			return nil, []byte("unknown flag: --version"), errors.New("command failed")
		case "leap auth whoami":
			return []byte("concierge@example.com\n"), nil, nil
		case "leap server info":
			return []byte("datasetvolumes: []\n"), nil, nil
		default:
			return nil, []byte("unknown command"), errors.New("command failed")
		}
	}

	snapshotValue, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repo})
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}

	if snapshotValue.LeapCLI.Version != "" {
		t.Fatalf("expected leap version to be empty when --version probe fails, got %q", snapshotValue.LeapCLI.Version)
	}
}

func TestSnapshotServerInfoProbeFailsWhenInstallationInfoMissing(t *testing.T) {
	repo := initGitRepo(t)

	snapshotter := NewGitSnapshotter()
	snapshotter.lookPath = func(file string) (string, error) {
		switch file {
		case "python3":
			return "/usr/bin/python3", nil
		case "leap":
			return "/usr/local/bin/leap", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	snapshotter.runCommand = func(ctx context.Context, dir string, name string, args ...string) ([]byte, []byte, error) {
		_ = ctx
		_ = dir
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "python3 --version":
			return []byte("Python 3.11.8\n"), nil, nil
		case "leap --version":
			return []byte("leap v0.2.0\n"), nil, nil
		case "leap auth whoami":
			return []byte("concierge@example.com\n"), nil, nil
		case "leap server info":
			return []byte("\x1b[36mINFO\x1b[0m No installation information found\n"), nil, nil
		default:
			return nil, []byte("unknown command"), errors.New("command failed")
		}
	}

	snapshotValue, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repo})
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if snapshotValue.LeapCLI.ServerInfoReachable {
		t.Fatal("expected server info probe to fail when installation info is missing")
	}
	if !strings.Contains(strings.ToLower(snapshotValue.LeapCLI.ServerInfoError), "no installation information found") {
		t.Fatalf("expected missing-installation error details, got %q", snapshotValue.LeapCLI.ServerInfoError)
	}
}

func TestSnapshotServerInfoProbeFailsWhenServerNotRunning(t *testing.T) {
	repo := initGitRepo(t)

	snapshotter := NewGitSnapshotter()
	snapshotter.lookPath = func(file string) (string, error) {
		switch file {
		case "python3":
			return "/usr/bin/python3", nil
		case "leap":
			return "/usr/local/bin/leap", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	snapshotter.runCommand = func(ctx context.Context, dir string, name string, args ...string) ([]byte, []byte, error) {
		_ = ctx
		_ = dir
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "python3 --version":
			return []byte("Python 3.11.8\n"), nil, nil
		case "leap --version":
			return []byte("leap v0.2.0\n"), nil, nil
		case "leap auth whoami":
			return []byte("concierge@example.com\n"), nil, nil
		case "leap server info":
			return []byte("INFO Installation information:\nINFO Server is not running (cluster not found)\n"), nil, nil
		default:
			return nil, []byte("unknown command"), errors.New("command failed")
		}
	}

	snapshotValue, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repo})
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if snapshotValue.LeapCLI.ServerInfoReachable {
		t.Fatal("expected server info probe to fail when server is not running")
	}
	if !strings.Contains(strings.ToLower(snapshotValue.LeapCLI.ServerInfoError), "server is not running") {
		t.Fatalf("expected server-not-running error details, got %q", snapshotValue.LeapCLI.ServerInfoError)
	}
}
