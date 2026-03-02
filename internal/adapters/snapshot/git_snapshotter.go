package snapshot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

type gitRunner func(ctx context.Context, dir string, args ...string) ([]byte, []byte, error)
type commandRunner func(ctx context.Context, dir string, name string, args ...string) ([]byte, []byte, error)
type pathLookup func(file string) (string, error)

// GitSnapshotter captures workspace identity from git metadata and working-tree state.
type GitSnapshotter struct {
	clock      func() time.Time
	getwd      func() (string, error)
	runGit     gitRunner
	runCommand commandRunner
	lookPath   pathLookup
}

// NewGitSnapshotter returns a snapshotter backed by the local git CLI.
func NewGitSnapshotter() *GitSnapshotter {
	return &GitSnapshotter{
		clock: func() time.Time {
			return time.Now().UTC()
		},
		getwd:      os.Getwd,
		runGit:     runGitCommand,
		runCommand: runCommandInDir,
		lookPath:   exec.LookPath,
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

	fileHashes := captureFileHashes(root)
	runtimeState := g.captureRuntimeState(ctx, root)
	leapCLIState := g.captureLeapCLIState(ctx, root)

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
		FileHashes: fileHashes,
		Runtime:    runtimeState,
		LeapCLI:    leapCLIState,
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
	if g.runCommand == nil {
		g.runCommand = runCommandInDir
	}
	if g.lookPath == nil {
		g.lookPath = exec.LookPath
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

func runCommandInDir(ctx context.Context, dir string, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
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

func captureFileHashes(repoRoot string) map[string]string {
	files := []string{
		"leap.yaml",
		"leap_binder.py",
		"leap_custom_test.py",
		"integration_test.py",
		"requirements.txt",
		"pyproject.toml",
	}

	hashes := make(map[string]string, len(files))
	for _, relativePath := range files {
		absolutePath := filepath.Join(repoRoot, relativePath)
		info, err := os.Stat(absolutePath)
		if err != nil || info.IsDir() {
			continue
		}
		contents, err := os.ReadFile(absolutePath)
		if err != nil {
			continue
		}
		hashes[relativePath] = sha256Hex(contents)
	}

	if len(hashes) == 0 {
		return nil
	}

	keys := make([]string, 0, len(hashes))
	for key := range hashes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(hashes))
	for _, key := range keys {
		ordered[key] = hashes[key]
	}

	return ordered
}

func (g *GitSnapshotter) captureRuntimeState(ctx context.Context, repoRoot string) core.RuntimeState {
	state := core.RuntimeState{
		ProbeRan:          true,
		RequirementsFiles: detectRequirementsFiles(repoRoot),
	}

	pythonCandidates := []string{"python3", "python"}
	for _, executable := range pythonCandidates {
		if _, err := g.lookPath(executable); err != nil {
			continue
		}

		output, err := g.commandOutput(ctx, repoRoot, executable, "--version")
		if err != nil {
			continue
		}

		state.PythonFound = true
		state.PythonExecutable = executable
		state.PythonVersion = strings.TrimSpace(output)
		break
	}

	return state
}

func detectRequirementsFiles(repoRoot string) []string {
	candidates := []string{"requirements.txt", "pyproject.toml"}
	found := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		path := filepath.Join(repoRoot, candidate)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		found = append(found, candidate)
	}
	if len(found) == 0 {
		return nil
	}
	return found
}

func (g *GitSnapshotter) captureLeapCLIState(ctx context.Context, repoRoot string) core.LeapCLIState {
	state := core.LeapCLIState{ProbeRan: true}
	if _, err := g.lookPath("leap"); err != nil {
		return state
	}

	state.Available = true

	if output, err := g.commandOutput(ctx, repoRoot, "leap", "--version"); err == nil {
		state.Version = strings.TrimSpace(output)
	}

	if _, err := g.commandOutput(ctx, repoRoot, "leap", "auth", "whoami"); err == nil {
		state.Authenticated = true
	}

	if output, err := g.commandOutputFull(ctx, repoRoot, "leap", "server", "info"); err != nil {
		state.ServerInfoError = strings.TrimSpace(err.Error())
	} else if reason, failed := leapServerInfoFailureReason(output); failed {
		state.ServerInfoError = reason
	} else {
		state.ServerInfoReachable = true
	}

	return state
}

func (g *GitSnapshotter) commandOutput(ctx context.Context, dir string, name string, args ...string) (string, error) {
	output, err := g.commandOutputFull(ctx, dir, name, args...)
	if err != nil {
		return "", err
	}
	if output == "" {
		return "", nil
	}

	lines := strings.Split(output, "\n")
	return strings.TrimSpace(lines[0]), nil
}

func (g *GitSnapshotter) commandOutputFull(ctx context.Context, dir string, name string, args ...string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	stdout, stderr, err := g.runCommand(probeCtx, dir, name, args...)
	output := strings.TrimSpace(strings.TrimSpace(string(stdout)) + "\n" + strings.TrimSpace(string(stderr)))
	output = strings.TrimSpace(output)

	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("%s timed out", name)
		}
		if output == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", output)
	}
	if output == "" {
		return "", nil
	}

	return output, nil
}

func leapServerInfoFailureReason(output string) (string, bool) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "leap server info returned no output", true
	}

	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "no installation information found"):
		return "leap server info reported no installation information found", true
	case strings.Contains(lower, "server is not running"):
		return "leap server info reported server is not running", true
	case strings.Contains(lower, "cluster not found"):
		return "leap server info reported cluster not found", true
	default:
		return "", false
	}
}
