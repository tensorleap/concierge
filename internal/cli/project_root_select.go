package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

var detectProjectRootCandidatesFunc = detectProjectRootCandidates

func resolveProjectRoot(explicitProjectRoot string, cwd string, nonInteractive bool, in io.Reader, out io.Writer) (string, string, error) {
	explicit := strings.TrimSpace(explicitProjectRoot)
	if explicit != "" {
		resolved, err := canonicalPath(explicit)
		if err != nil {
			return "", "", core.WrapError(core.KindUnknown, "cli.run.project_root_explicit", err)
		}
		return resolved, fmt.Sprintf("project root: %s (flag)", resolved), nil
	}

	candidates, err := detectProjectRootCandidatesFunc(cwd)
	if err != nil {
		return "", "", err
	}
	if len(candidates) == 0 {
		resolved, err := canonicalPath(cwd)
		if err != nil {
			return "", "", err
		}
		return resolved, fmt.Sprintf("project root: %s (cwd)", resolved), nil
	}
	if len(candidates) == 1 {
		return candidates[0], fmt.Sprintf("project root: %s (auto)", candidates[0]), nil
	}

	if nonInteractive {
		return "", "", core.NewError(
			core.KindUnknown,
			"cli.run.project_root_ambiguous",
			fmt.Sprintf("multiple project roots detected (%d). rerun with --project-root or disable --non-interactive", len(candidates)),
		)
	}

	selected, err := promptProjectRootSelection(in, out, candidates)
	if err != nil {
		return "", "", core.WrapError(core.KindUnknown, "cli.run.project_root_prompt", err)
	}
	return selected, fmt.Sprintf("project root: %s (prompt)", selected), nil
}

func detectProjectRootCandidates(cwd string) ([]string, error) {
	resolvedCWD, err := canonicalPath(cwd)
	if err != nil {
		return nil, core.WrapError(core.KindUnknown, "cli.run.project_root_cwd", err)
	}

	if gitRoot, ok := discoverGitTopLevel(resolvedCWD); ok {
		canonicalGitRoot, err := canonicalPath(gitRoot)
		if err != nil {
			return nil, core.WrapError(core.KindUnknown, "cli.run.project_root_git", err)
		}
		return []string{canonicalGitRoot}, nil
	}

	candidateSet := map[string]struct{}{}
	addCandidate := func(path string) {
		canonical, err := canonicalPath(path)
		if err != nil {
			return
		}
		candidateSet[canonical] = struct{}{}
	}

	if hasFile(filepath.Join(resolvedCWD, "leap.yaml")) {
		addCandidate(resolvedCWD)
	}

	entries, err := os.ReadDir(resolvedCWD)
	if err != nil {
		return nil, core.WrapError(core.KindUnknown, "cli.run.project_root_scan", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		fullPath := filepath.Join(resolvedCWD, name)
		if hasDirectory(filepath.Join(fullPath, ".git")) || hasFile(filepath.Join(fullPath, "leap.yaml")) {
			addCandidate(fullPath)
		}
	}

	if len(candidateSet) == 0 {
		addCandidate(resolvedCWD)
	}

	candidates := make([]string, 0, len(candidateSet))
	for candidate := range candidateSet {
		candidates = append(candidates, candidate)
	}
	sort.Strings(candidates)
	return candidates, nil
}

func discoverGitTopLevel(dir string) (string, bool) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", false
	}
	return root, true
}

func canonicalPath(path string) (string, error) {
	absPath, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}
	return filepath.Clean(absPath), nil
}

func hasFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func hasDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
