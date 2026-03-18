package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var agentGuardedPathCommands = []string{
	"find",
	"ls",
	"grep",
	"cat",
	"sed",
	"head",
	"tail",
	"wc",
}

func prepareAgentTask(task AgentTask) (AgentTask, []string, func(), error) {
	originalRoot := strings.TrimSpace(task.RepoRoot)
	if originalRoot == "" {
		return AgentTask{}, nil, func() {}, fmt.Errorf("agent task repo root is required")
	}

	tempRoot, wrapperRoot, cleanup, err := createAgentExecutionRoots(originalRoot, allowedAgentConciergePaths(task.ScopePolicy))
	if err != nil {
		return AgentTask{}, nil, func() {}, err
	}

	prepared := rewriteAgentTaskForView(task, originalRoot, tempRoot)
	runtimeInterpreter := ""
	if prepared.RepoContext != nil {
		runtimeInterpreter = strings.TrimSpace(prepared.RepoContext.RuntimeInterpreter)
	}

	if err := installAgentGuardWrappers(wrapperRoot, tempRoot, runtimeInterpreter); err != nil {
		cleanup()
		return AgentTask{}, nil, func() {}, err
	}

	env := prependPath(os.Environ(), wrapperRoot)
	return prepared, env, cleanup, nil
}

func createAgentExecutionRoots(originalRoot string, allowedConciergePaths []string) (string, string, func(), error) {
	tempBase, err := os.MkdirTemp("", "concierge-agent-view-*")
	if err != nil {
		return "", "", func() {}, err
	}

	cleanup := func() {
		_ = os.RemoveAll(tempBase)
	}

	baseName := filepath.Base(originalRoot)
	if strings.TrimSpace(baseName) == "" || baseName == "." || baseName == string(filepath.Separator) {
		baseName = "repo"
	}

	viewRoot := filepath.Join(tempBase, baseName)
	if err := os.MkdirAll(viewRoot, 0o755); err != nil {
		cleanup()
		return "", "", func() {}, err
	}

	entries, err := os.ReadDir(originalRoot)
	if err != nil {
		cleanup()
		return "", "", func() {}, err
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == ".concierge" {
			continue
		}
		sourcePath := filepath.Join(originalRoot, name)
		targetPath := filepath.Join(viewRoot, name)
		if err := os.Symlink(sourcePath, targetPath); err != nil {
			cleanup()
			return "", "", func() {}, err
		}
	}

	if err := exposeAllowedConciergePaths(originalRoot, viewRoot, allowedConciergePaths); err != nil {
		cleanup()
		return "", "", func() {}, err
	}

	wrapperRoot := filepath.Join(tempBase, "bin")
	if err := os.MkdirAll(wrapperRoot, 0o755); err != nil {
		cleanup()
		return "", "", func() {}, err
	}

	return viewRoot, wrapperRoot, cleanup, nil
}

func allowedAgentConciergePaths(policy *AgentScopePolicy) []string {
	if policy == nil || len(policy.AllowedFiles) == 0 {
		return nil
	}

	paths := make([]string, 0, len(policy.AllowedFiles))
	seen := make(map[string]struct{}, len(policy.AllowedFiles))
	for _, path := range policy.AllowedFiles {
		normalized := normalizeAllowedConciergePath(path)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		paths = append(paths, normalized)
	}
	return paths
}

func normalizeAllowedConciergePath(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if cleaned == "" || cleaned == "." || cleaned == ".concierge" {
		return ""
	}
	if !strings.HasPrefix(cleaned, ".concierge/") {
		return ""
	}
	if strings.HasPrefix(cleaned, ".concierge/../") {
		return ""
	}
	return cleaned
}

func exposeAllowedConciergePaths(originalRoot, viewRoot string, allowedConciergePaths []string) error {
	if len(allowedConciergePaths) == 0 {
		return nil
	}

	conciergeViewRoot := filepath.Join(viewRoot, ".concierge")
	if err := os.MkdirAll(conciergeViewRoot, 0o755); err != nil {
		return err
	}

	allowedSet := make(map[string]struct{}, len(allowedConciergePaths))
	for _, relPath := range allowedConciergePaths {
		allowedSet[relPath] = struct{}{}
	}

	for _, relPath := range allowedConciergePaths {
		sourcePath := filepath.Join(originalRoot, filepath.FromSlash(relPath))
		if err := ensureAllowedConciergeSourcePath(sourcePath, relPath, allowedSet); err != nil {
			return err
		}

		targetPath := filepath.Join(viewRoot, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if _, err := os.Lstat(targetPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.Symlink(sourcePath, targetPath); err != nil {
			return err
		}
	}

	return nil
}

func ensureAllowedConciergeSourcePath(sourcePath, relPath string, allowedSet map[string]struct{}) error {
	info, err := os.Lstat(sourcePath)
	if err == nil {
		if info.IsDir() {
			return os.MkdirAll(sourcePath, 0o755)
		}
		return os.MkdirAll(filepath.Dir(sourcePath), 0o755)
	}
	if !os.IsNotExist(err) {
		return err
	}
	if allowedConciergePathLooksLikeDirectory(relPath, allowedSet) {
		return os.MkdirAll(sourcePath, 0o755)
	}
	return os.MkdirAll(filepath.Dir(sourcePath), 0o755)
}

func allowedConciergePathLooksLikeDirectory(relPath string, allowedSet map[string]struct{}) bool {
	prefix := relPath + "/"
	for candidate := range allowedSet {
		if strings.HasPrefix(candidate, prefix) {
			return true
		}
	}
	return filepath.Ext(relPath) == ""
}

func rewriteAgentTaskForView(task AgentTask, originalRoot, viewRoot string) AgentTask {
	prepared := task
	prepared.RepoRoot = viewRoot
	if task.RepoContext == nil {
		return prepared
	}

	repoContext := *task.RepoContext
	repoContext.RepoRoot = viewRoot
	repoContext.RuntimeInterpreter = rewriteAgentViewPath(repoContext.RuntimeInterpreter, originalRoot, viewRoot)
	repoContext.SelectedModelPath = rewriteAgentViewPath(repoContext.SelectedModelPath, originalRoot, viewRoot)
	repoContext.ModelCandidates = rewriteAgentViewPaths(repoContext.ModelCandidates, originalRoot, viewRoot)
	repoContext.ReadyModelArtifacts = rewriteAgentViewPaths(repoContext.ReadyModelArtifacts, originalRoot, viewRoot)
	repoContext.ModelAcquisitionLeads = rewriteAgentViewPaths(repoContext.ModelAcquisitionLeads, originalRoot, viewRoot)
	prepared.RepoContext = &repoContext
	return prepared
}

func rewriteAgentViewPaths(values []string, originalRoot, viewRoot string) []string {
	if len(values) == 0 {
		return nil
	}
	rewritten := make([]string, 0, len(values))
	for _, value := range values {
		rewritten = append(rewritten, rewriteAgentViewPath(value, originalRoot, viewRoot))
	}
	return rewritten
}

func rewriteAgentViewPath(value, originalRoot, viewRoot string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	normalizedOriginal := filepath.Clean(originalRoot)
	normalizedValue := filepath.Clean(trimmed)
	if normalizedValue == normalizedOriginal {
		return viewRoot
	}
	if !strings.HasPrefix(normalizedValue, normalizedOriginal+string(filepath.Separator)) {
		return value
	}

	suffix := strings.TrimPrefix(normalizedValue, normalizedOriginal)
	return filepath.Clean(viewRoot + suffix)
}

func installAgentGuardWrappers(wrapperRoot, viewRoot, runtimeInterpreter string) error {
	for _, name := range agentGuardedPathCommands {
		realPath, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if err := writeExecutable(wrapperPath(wrapperRoot, name), buildGuardedPathWrapper(realPath, viewRoot)); err != nil {
			return err
		}
	}

	for _, name := range []string{"python", "python3"} {
		realPath, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if err := writeExecutable(wrapperPath(wrapperRoot, name), buildPythonWrapper(realPath, runtimeInterpreter)); err != nil {
			return err
		}
	}

	for _, name := range []string{"pip", "pip3"} {
		if err := writeExecutable(wrapperPath(wrapperRoot, name), buildPipWrapper()); err != nil {
			return err
		}
	}

	return nil
}

func wrapperPath(root, name string) string {
	return filepath.Join(root, name)
}

func buildGuardedPathWrapper(realPath, viewRoot string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
allowed_root=%q
for arg in "$@"; do
  case "$arg" in
    /dev/null)
      continue
      ;;
    /*)
      if [[ "$arg" != "$allowed_root" && "$arg" != "$allowed_root"/* ]]; then
        echo "concierge-agent-guard: refusing path outside agent repo view: $arg" >&2
        exit 64
      fi
      ;;
  esac
done
exec %q "$@"
`, viewRoot, realPath)
}

func buildPythonWrapper(realPath, runtimeInterpreter string) string {
	if strings.TrimSpace(runtimeInterpreter) == "" {
		return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
exec %q "$@"
`, realPath)
	}

	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
echo "concierge-agent-guard: bare %s is disabled for agent tasks; use %s" >&2
exit 64
`, filepath.Base(realPath), runtimeInterpreter)
}

func buildPipWrapper() string {
	return `#!/usr/bin/env bash
set -euo pipefail
echo "concierge-agent-guard: pip is disabled for agent tasks" >&2
exit 64
`
}

func writeExecutable(path, contents string) error {
	return os.WriteFile(path, []byte(contents), 0o755)
}

func prependPath(env []string, wrapperRoot string) []string {
	if len(env) == 0 {
		return []string{"PATH=" + wrapperRoot}
	}

	result := make([]string, 0, len(env)+1)
	pathUpdated := false
	for _, entry := range env {
		if !strings.HasPrefix(entry, "PATH=") {
			result = append(result, entry)
			continue
		}
		current := strings.TrimPrefix(entry, "PATH=")
		result = append(result, "PATH="+wrapperRoot+string(os.PathListSeparator)+current)
		pathUpdated = true
	}
	if !pathUpdated {
		result = append(result, "PATH="+wrapperRoot)
	}
	return result
}
