package snapshot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

type gitRunner func(ctx context.Context, dir string, args ...string) ([]byte, []byte, error)

// GitSnapshotter captures workspace identity from git metadata and working-tree state.
type GitSnapshotter struct {
	clock  func() time.Time
	getwd  func() (string, error)
	runGit gitRunner
}

// NewGitSnapshotter returns a snapshotter backed by the local git CLI.
func NewGitSnapshotter() *GitSnapshotter {
	return &GitSnapshotter{
		clock: func() time.Time {
			return time.Now().UTC()
		},
		getwd:  os.Getwd,
		runGit: runGitCommand,
	}
}

// Snapshot captures a deterministic workspace snapshot for one orchestrator iteration.
func (g *GitSnapshotter) Snapshot(ctx context.Context, request core.SnapshotRequest) (core.WorkspaceSnapshot, error) {
	g.ensureDefaults()

	root, err := g.resolveRoot(request)
	if err != nil {
		return core.WorkspaceSnapshot{}, err
	}

	gitRoot, err := g.gitOutput(ctx, root, "snapshot.git_root", "rev-parse", "--show-toplevel")
	if err != nil {
		return core.WorkspaceSnapshot{}, err
	}

	branch, err := g.gitOutput(ctx, root, "snapshot.branch", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return core.WorkspaceSnapshot{}, err
	}

	head, err := g.gitOutput(ctx, root, "snapshot.head", "rev-parse", "HEAD")
	if err != nil {
		return core.WorkspaceSnapshot{}, err
	}

	porcelain, err := g.gitOutputRaw(ctx, root, "snapshot.worktree_status", "status", "--porcelain")
	if err != nil {
		return core.WorkspaceSnapshot{}, err
	}

	worktreeFingerprint := sha256Hex(porcelain)
	snapshotID := sha256Hex([]byte(strings.Join([]string{
		root,
		gitRoot,
		branch,
		head,
		worktreeFingerprint,
	}, "|")))

	return core.WorkspaceSnapshot{
		ID:                  snapshotID,
		CapturedAt:          g.clock(),
		WorktreeFingerprint: worktreeFingerprint,
		Repository: core.RepositoryState{
			Root:    root,
			GitRoot: gitRoot,
			Branch:  branch,
			Head:    head,
			Dirty:   len(porcelain) > 0,
		},
	}, nil
}

func (g *GitSnapshotter) ensureDefaults() {
	if g.clock == nil {
		g.clock = func() time.Time {
			return time.Now().UTC()
		}
	}
	if g.getwd == nil {
		g.getwd = os.Getwd
	}
	if g.runGit == nil {
		g.runGit = runGitCommand
	}
}

func (g *GitSnapshotter) resolveRoot(request core.SnapshotRequest) (string, error) {
	repoRoot := request.RepoRoot
	if strings.TrimSpace(repoRoot) == "" {
		cwd, err := g.getwd()
		if err != nil {
			return "", core.WrapError(core.KindUnknown, "snapshot.resolve_root", err)
		}
		repoRoot = cwd
	}

	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", core.WrapError(core.KindUnknown, "snapshot.resolve_root", err)
	}
	if resolvedRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolvedRoot
	}

	return absRoot, nil
}

func (g *GitSnapshotter) gitOutput(ctx context.Context, dir, op string, args ...string) (string, error) {
	stdout, err := g.gitOutputRaw(ctx, dir, op, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

func (g *GitSnapshotter) gitOutputRaw(ctx context.Context, dir, op string, args ...string) ([]byte, error) {
	stdout, stderr, err := g.runGit(ctx, dir, args...)
	if err != nil {
		return nil, g.mapGitError(op, args, stderr, err)
	}
	return stdout, nil
}

func (g *GitSnapshotter) mapGitError(op string, args []string, stderr []byte, err error) error {
	command := "git " + strings.Join(args, " ")
	stderrText := strings.TrimSpace(string(stderr))

	message := fmt.Sprintf("command %q failed", command)
	if stderrText != "" {
		message = fmt.Sprintf("%s (stderr: %s)", message, stderrText)
	}

	kind := core.KindUnknown
	if containsNotGitRepo(stderrText, err) {
		kind = core.KindNotGitRepo
	}

	return core.WrapError(kind, op, fmt.Errorf("%s: %w", message, err))
}

func containsNotGitRepo(stderrText string, err error) bool {
	text := strings.ToLower(stderrText)
	if err != nil {
		text = text + " " + strings.ToLower(err.Error())
	}
	return strings.Contains(text, "not a git repository")
}

func runGitCommand(ctx context.Context, dir string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
