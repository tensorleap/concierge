package execute

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

type poetryDependencyCommandRunner func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error)

// PoetryDependencyExecutor repairs Poetry runtime readiness for the selected repo.
type PoetryDependencyExecutor struct {
	runCommand poetryDependencyCommandRunner
}

// NewPoetryDependencyExecutor creates a Poetry-backed runtime executor.
func NewPoetryDependencyExecutor() *PoetryDependencyExecutor {
	return &PoetryDependencyExecutor{runCommand: runPoetryDependencyCommand}
}

// Execute applies deterministic Poetry repair actions for ensure.python_runtime.
func (e *PoetryDependencyExecutor) Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return core.ExecutionResult{}, core.NewError(core.KindUnknown, "execute.poetry.repo_root", "snapshot repository root is empty")
	}
	if step.ID != core.EnsureStepPythonRuntime {
		return core.ExecutionResult{}, core.WrapError(
			core.KindStepNotApplicable,
			"execute.poetry.unsupported_step",
			fmt.Errorf("ensure-step %q is not supported by poetry dependency executor", step.ID),
		)
	}

	profile := snapshot.RuntimeProfile
	if profile == nil || strings.TrimSpace(profile.InterpreterPath) == "" {
		return core.ExecutionResult{
			Step:    step,
			Applied: false,
			Summary: "Poetry environment is not resolved yet",
			Evidence: []core.EvidenceItem{
				{Name: "executor.mode", Value: "poetry_dependency"},
			},
		}, nil
	}

	commandArgs := []string{"install"}
	summary := "installed project dependencies in the resolved Poetry environment"
	if !profile.CodeLoaderReady {
		commandArgs = []string{"add", "code-loader@^1.0"}
		summary = "added code-loader@^1.0 through Poetry"
	}

	stdout, stderr, err := e.runCommand(ctx, repoRoot, "poetry", commandArgs...)
	if err != nil {
		errText := strings.TrimSpace(strings.TrimSpace(string(stdout)) + "\n" + strings.TrimSpace(string(stderr)))
		if errText == "" {
			errText = err.Error()
		}
		return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.poetry.run", fmt.Errorf("%s", errText))
	}

	return core.ExecutionResult{
		Step:    step,
		Applied: true,
		Summary: summary,
		Evidence: []core.EvidenceItem{
			{Name: "executor.mode", Value: "poetry_dependency"},
			{Name: "runtime.command", Value: "poetry " + strings.Join(commandArgs, " ")},
			{Name: "runtime.stdout", Value: strings.TrimSpace(string(stdout))},
			{Name: "runtime.stderr", Value: strings.TrimSpace(string(stderr))},
		},
	}, nil
}

func runPoetryDependencyCommand(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}
