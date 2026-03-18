package execute

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/observe"
)

type poetryDependencyCommandRunner func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error)

// PoetryDependencyExecutor repairs Poetry runtime readiness for the selected repo.
type PoetryDependencyExecutor struct {
	runCommand       poetryDependencyCommandRunner
	observer         observe.Sink
	progressInterval time.Duration
}

const minimumCodeLoaderConstraint = "code-loader@^1.0.165"
const poetryDependencyProgressInterval = 15 * time.Second

// NewPoetryDependencyExecutor creates a Poetry-backed runtime executor.
func NewPoetryDependencyExecutor() *PoetryDependencyExecutor {
	return &PoetryDependencyExecutor{runCommand: runPoetryDependencyCommand}
}

// SetObserver configures the live event sink used for deterministic runtime-repair updates.
func (e *PoetryDependencyExecutor) SetObserver(sink observe.Sink) {
	e.observer = sink
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
			Summary: runtimeSelfServiceSummary(snapshot),
			Evidence: []core.EvidenceItem{
				{Name: "executor.mode", Value: "self_service"},
				{Name: "executor.actionable", Value: "false"},
			},
		}, nil
	}

	commandArgs := []string{"install"}
	summary := "installed project dependencies in the resolved Poetry environment"
	if !profile.CodeLoaderReady && !profile.CodeLoaderDeclaredInProject {
		commandArgs = []string{"add", minimumCodeLoaderConstraint}
		summary = fmt.Sprintf("added %s through Poetry", minimumCodeLoaderConstraint)
	}

	commandMessage := fmt.Sprintf("Running poetry %s to repair the Poetry environment", strings.Join(commandArgs, " "))
	stdout, stderr, err := e.runCommandWithProgress(ctx, repoRoot, commandMessage, "poetry", commandArgs...)
	if err != nil {
		errText := strings.TrimSpace(strings.TrimSpace(string(stdout)) + "\n" + strings.TrimSpace(string(stderr)))
		if errText == "" {
			errText = err.Error()
		}
		return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.poetry.run", fmt.Errorf("%s", errText))
	}

	evidence := []core.EvidenceItem{
		{Name: "executor.mode", Value: "poetry_dependency"},
		{Name: "runtime.command", Value: "poetry " + strings.Join(commandArgs, " ")},
		{Name: "runtime.stdout", Value: strings.TrimSpace(string(stdout))},
		{Name: "runtime.stderr", Value: strings.TrimSpace(string(stderr))},
	}
	if !profile.CodeLoaderReady {
		verificationArgs := []string{"-c", "import code_loader"}
		verifyStdout, verifyStderr, verifyErr := e.runCommand(ctx, repoRoot, profile.InterpreterPath, verificationArgs...)
		evidence = append(evidence,
			core.EvidenceItem{Name: "runtime.verification.command", Value: profile.InterpreterPath + " " + strings.Join(verificationArgs, " ")},
			core.EvidenceItem{Name: "runtime.verification.stdout", Value: strings.TrimSpace(string(verifyStdout))},
			core.EvidenceItem{Name: "runtime.verification.stderr", Value: strings.TrimSpace(string(verifyStderr))},
		)
		if verifyErr != nil {
			errText := strings.TrimSpace(strings.TrimSpace(string(verifyStdout)) + "\n" + strings.TrimSpace(string(verifyStderr)))
			if errText == "" {
				errText = verifyErr.Error()
			}
			return core.ExecutionResult{}, core.WrapError(
				core.KindUnknown,
				"execute.poetry.verify_code_loader",
				fmt.Errorf("code_loader import still failed after runtime repair: %s", errText),
			)
		}
		evidence = append(evidence, core.EvidenceItem{Name: "runtime.verification.result", Value: "code_loader import ok"})
		summary += " and verified `code_loader` import"
	}

	return core.ExecutionResult{
		Step:     step,
		Applied:  true,
		Summary:  summary,
		Evidence: evidence,
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

func runtimeSelfServiceSummary(snapshot core.WorkspaceSnapshot) string {
	switch {
	case !snapshot.Runtime.SupportedProject:
		return "Concierge needs a Poetry-managed project before it can run local Tensorleap checks"
	case !snapshot.Runtime.PoetryFound:
		return "Concierge cannot continue until Poetry is installed and available in PATH"
	default:
		return "Concierge cannot continue until this project has an existing Poetry environment"
	}
}

func (e *PoetryDependencyExecutor) runCommandWithProgress(
	ctx context.Context,
	dir string,
	message string,
	name string,
	args ...string,
) ([]byte, []byte, error) {
	if e == nil || e.runCommand == nil {
		return nil, nil, core.NewError(core.KindUnknown, "execute.poetry.run_command", "poetry command runner is not configured")
	}

	e.emit(observe.Event{
		Kind:    observe.EventExecutorProgress,
		Stage:   core.StageExecute,
		StepID:  core.EnsureStepPythonRuntime,
		Message: message,
	})
	if e.observer == nil {
		return e.runCommand(ctx, dir, name, args...)
	}

	type commandResult struct {
		stdout []byte
		stderr []byte
		err    error
	}
	done := make(chan commandResult, 1)
	go func() {
		stdout, stderr, err := e.runCommand(ctx, dir, name, args...)
		done <- commandResult{stdout: stdout, stderr: stderr, err: err}
	}()

	ticker := time.NewTicker(e.heartbeatInterval())
	defer ticker.Stop()

	for {
		select {
		case result := <-done:
			return result.stdout, result.stderr, result.err
		case tickAt := <-ticker.C:
			e.emit(observe.Event{
				Time:    tickAt.UTC(),
				Kind:    observe.EventExecutorHeartbeat,
				Stage:   core.StageExecute,
				StepID:  core.EnsureStepPythonRuntime,
				Message: message,
			})
		case <-ctx.Done():
			result := <-done
			return result.stdout, result.stderr, result.err
		}
	}
}

func (e *PoetryDependencyExecutor) heartbeatInterval() time.Duration {
	if e == nil || e.progressInterval <= 0 {
		return poetryDependencyProgressInterval
	}
	return e.progressInterval
}

func (e *PoetryDependencyExecutor) emit(event observe.Event) {
	if e == nil || e.observer == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	e.observer.Emit(event)
}
