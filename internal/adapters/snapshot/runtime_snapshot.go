package snapshot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

type runtimeSnapshotter struct {
	runCommand commandRunner
	lookPath   pathLookup
	getEnv     func(string) string
}

func newRuntimeSnapshotter(runCommand commandRunner, lookPath pathLookup) runtimeSnapshotter {
	return runtimeSnapshotter{
		runCommand: runCommand,
		lookPath:   lookPath,
		getEnv:     os.Getenv,
	}
}

func (s runtimeSnapshotter) capture(ctx context.Context, repoRoot string) core.RuntimeState {
	state := core.RuntimeState{
		ProbeRan:           true,
		RequirementsFiles:  detectRequirementsFiles(repoRoot),
		AmbientVirtualEnv:  strings.TrimSpace(s.getenv("VIRTUAL_ENV")),
		AmbientCondaPrefix: strings.TrimSpace(s.getenv("CONDA_PREFIX")),
	}

	pyprojectPath := filepath.Join(repoRoot, "pyproject.toml")
	poetryLockPath := filepath.Join(repoRoot, "poetry.lock")
	state.PyProjectPresent = fileExistsSimple(pyprojectPath)
	state.PoetryLockPresent = fileExistsSimple(poetryLockPath)
	state.SupportedProject, state.ProjectSupportReason = classifyPoetryProject(pyprojectPath)

	if s.lookPath != nil {
		if poetryPath, err := s.lookPath("poetry"); err == nil {
			state.PoetryFound = true
			state.PoetryExecutable = poetryPath
			if version, err := commandOutput(ctx, s.runCommand, repoRoot, "poetry", "--version"); err == nil {
				state.PoetryVersion = strings.TrimSpace(version)
			}
		}
	}

	return state
}

func (s runtimeSnapshotter) getenv(key string) string {
	if s.getEnv == nil {
		return ""
	}
	return s.getEnv(key)
}

func classifyPoetryProject(pyprojectPath string) (bool, string) {
	raw, err := os.ReadFile(pyprojectPath)
	if err != nil {
		return false, "pyproject.toml is missing"
	}

	content := string(raw)
	switch {
	case strings.Contains(content, "[tool.poetry]"):
		return true, ""
	case strings.Contains(content, "build-backend = \"poetry.core.masonry.api\""):
		return true, ""
	case strings.Contains(content, "build-backend='poetry.core.masonry.api'"):
		return true, ""
	case strings.Contains(content, "poetry-core"):
		return true, ""
	default:
		return false, "pyproject.toml does not declare a Poetry-managed project"
	}
}

func fileExistsSimple(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func commandOutput(ctx context.Context, runCommand commandRunner, dir string, name string, args ...string) (string, error) {
	if runCommand == nil {
		return "", fmt.Errorf("command runner is not configured")
	}

	probeCtx, cancel := context.WithTimeout(ctx, probeTimeoutForCommand(name, args...))
	defer cancel()

	stdout, stderr, err := runCommand(probeCtx, dir, name, args...)
	output := strings.TrimSpace(strings.TrimSpace(string(stdout)) + "\n" + strings.TrimSpace(string(stderr)))
	output = strings.TrimSpace(output)

	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("%s timed out after %s", formatCommand(name, args...), timeoutForDisplay(name, args...))
		}
		if output == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", output)
	}

	return output, nil
}

func timeoutForDisplay(name string, args ...string) time.Duration {
	return probeTimeoutForCommand(name, args...)
}
