package snapshot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
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

const (
	defaultProbeTimeout        = 2 * time.Second
	leapServerInfoProbeTimeout = 6 * time.Second
)

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

	porcelain, err := g.gitOutputRaw(ctx, root, "snapshot.worktree_status", "status", "--porcelain=v1", "--untracked-files=all", "-z")
	if err != nil {
		return core.WorkspaceSnapshot{}, err
	}

	worktreeFingerprint, err := fingerprintWorktreeState(root, porcelain)
	if err != nil {
		return core.WorkspaceSnapshot{}, core.WrapError(core.KindUnknown, "snapshot.worktree_fingerprint", err)
	}
	snapshotID := sha256Hex([]byte(strings.Join([]string{
		root,
		gitRoot,
		branch,
		head,
		worktreeFingerprint,
	}, "|")))

	fileHashes := captureFileHashes(root)
	runtimeState := newRuntimeSnapshotter(g.runCommand, g.lookPath).capture(ctx, root)
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

func fingerprintWorktreeState(repoRoot string, porcelain []byte) (string, error) {
	hasher := sha256.New()
	hasher.Write(porcelain)

	paths, err := parsePorcelainV1ZPaths(porcelain)
	if err != nil {
		return "", err
	}

	for _, path := range paths {
		hasher.Write([]byte{0})
		hasher.Write([]byte(path))
		hasher.Write([]byte{0})

		hash, err := hashWorktreePath(filepath.Join(repoRoot, filepath.FromSlash(path)))
		if err != nil {
			return "", err
		}
		hasher.Write([]byte(hash))
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func parsePorcelainV1ZPaths(porcelain []byte) ([]string, error) {
	if len(porcelain) == 0 {
		return nil, nil
	}

	paths := make([]string, 0)
	for cursor := 0; cursor < len(porcelain); {
		if cursor+3 > len(porcelain) {
			return nil, fmt.Errorf("truncated porcelain entry at byte %d", cursor)
		}
		status := porcelain[cursor : cursor+2]
		if porcelain[cursor+2] != ' ' {
			return nil, fmt.Errorf("invalid porcelain entry at byte %d", cursor)
		}
		cursor += 3

		nextNUL := bytes.IndexByte(porcelain[cursor:], 0)
		if nextNUL < 0 {
			return nil, fmt.Errorf("unterminated porcelain path at byte %d", cursor)
		}
		paths = append(paths, string(porcelain[cursor:cursor+nextNUL]))
		cursor += nextNUL + 1

		if status[0] == 'R' || status[0] == 'C' || status[1] == 'R' || status[1] == 'C' {
			renameNUL := bytes.IndexByte(porcelain[cursor:], 0)
			if renameNUL < 0 {
				return nil, fmt.Errorf("unterminated porcelain rename source at byte %d", cursor)
			}
			cursor += renameNUL + 1
		}
	}

	return paths, nil
}

func hashWorktreePath(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		return sha256Hex([]byte("symlink\x00" + target)), nil
	case info.IsDir():
		return hashDirectory(path)
	default:
		contents, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return sha256Hex(contents), nil
	}
}

func hashDirectory(root string) (string, error) {
	hasher := sha256.New()

	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relativePath == "." {
			return nil
		}
		relativePath = filepath.ToSlash(relativePath)
		if relativePath == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}

		hasher.Write([]byte(relativePath))
		hasher.Write([]byte{0})

		info, err := entry.Info()
		if err != nil {
			return err
		}

		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			hasher.Write([]byte("symlink"))
			hasher.Write([]byte{0})
			hasher.Write([]byte(target))
		case entry.IsDir():
			hasher.Write([]byte("dir"))
		default:
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hasher.Write([]byte("file"))
			hasher.Write([]byte{0})
			hasher.Write([]byte(sha256Hex(contents)))
		}

		hasher.Write([]byte{0})
		return nil
	}); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func captureFileHashes(repoRoot string) map[string]string {
	files := append([]string{"leap.yaml", core.CanonicalIntegrationEntryFile}, core.AllRequirementsFileCandidates()...)

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
func detectRequirementsFiles(repoRoot string) []string {
	found := make([]string, 0, len(core.RequirementsFileCandidates))
	for _, candidate := range core.RequirementsFileCandidates {
		if fileExistsSimple(filepath.Join(repoRoot, candidate)) {
			found = append(found, candidate)
		}
	}
	for _, pair := range core.RequirementsFilePairs {
		if fileExistsSimple(filepath.Join(repoRoot, pair[0])) && fileExistsSimple(filepath.Join(repoRoot, pair[1])) {
			found = append(found, pair[0], pair[1])
		}
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
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeoutForCommand(name, args...))
	defer cancel()

	stdout, stderr, err := g.runCommand(probeCtx, dir, name, args...)
	output := strings.TrimSpace(strings.TrimSpace(string(stdout)) + "\n" + strings.TrimSpace(string(stderr)))
	output = strings.TrimSpace(output)

	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("%s timed out", formatCommand(name, args...))
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

func probeTimeoutForCommand(name string, args ...string) time.Duration {
	if isLeapServerInfoCommand(name, args...) {
		return leapServerInfoProbeTimeout
	}
	return defaultProbeTimeout
}

func isLeapServerInfoCommand(name string, args ...string) bool {
	return strings.EqualFold(name, "leap") && len(args) >= 2 && args[0] == "server" && args[1] == "info"
}

func formatCommand(name string, args ...string) string {
	parts := append([]string{name}, args...)
	return strings.TrimSpace(strings.Join(parts, " "))
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
