package validate

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

type pythonRuntimeCommandRunner func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error)

// PythonRuntimeCommandResult captures one Poetry-routed Python invocation.
type PythonRuntimeCommandResult struct {
	Command string
	Stdout  string
	Stderr  string
}

// PythonRuntimeRunner routes local Python subprocesses through the resolved Poetry boundary.
type PythonRuntimeRunner struct {
	runCommand pythonRuntimeCommandRunner
}

// NewPythonRuntimeRunner creates a Poetry-backed Python runner.
func NewPythonRuntimeRunner() *PythonRuntimeRunner {
	return &PythonRuntimeRunner{runCommand: runPythonRuntimeCommand}
}

// RunPython executes `poetry run python ...` for the resolved repo runtime.
func (r *PythonRuntimeRunner) RunPython(
	ctx context.Context,
	snapshot core.WorkspaceSnapshot,
	args ...string,
) (PythonRuntimeCommandResult, error) {
	if r == nil {
		r = NewPythonRuntimeRunner()
	}
	if r.runCommand == nil {
		r.runCommand = runPythonRuntimeCommand
	}
	if snapshot.RuntimeProfile == nil || strings.TrimSpace(snapshot.RuntimeProfile.InterpreterPath) == "" {
		return PythonRuntimeCommandResult{}, core.NewError(
			core.KindUnknown,
			"validate.python_runtime.profile_missing",
			"Poetry runtime profile is required before running local Python commands",
		)
	}

	commandArgs := append([]string{"run", "python"}, args...)
	stdout, stderr, err := r.runCommand(ctx, strings.TrimSpace(snapshot.Repository.Root), "poetry", commandArgs...)
	result := PythonRuntimeCommandResult{
		Command: "poetry " + strings.Join(commandArgs, " "),
		Stdout:  strings.TrimSpace(string(stdout)),
		Stderr:  strings.TrimSpace(string(stderr)),
	}
	if err != nil {
		message := strings.TrimSpace(strings.TrimSpace(result.Stdout) + "\n" + strings.TrimSpace(result.Stderr))
		if message == "" {
			message = err.Error()
		}
		return result, core.WrapError(core.KindUnknown, "validate.python_runtime.run", fmt.Errorf("%s", message))
	}

	return result, nil
}

func runPythonRuntimeCommand(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}
