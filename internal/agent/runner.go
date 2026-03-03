package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

const (
	claudeCommand = "claude"
)

const defaultTimeout = 15 * time.Minute

var defaultClaudeArgs = []string{"--print", "--output-format", "text", "--permission-mode", "bypassPermissions"}

const conciergeOperatingPolicy = `
You are a task-scoped coding collaborator running under Concierge.
Concierge is the deterministic orchestrator; you are not the orchestrator.

Operating responsibilities:
- Complete only the specific objective provided for this task.
- Keep edits minimal, local, and reviewable.
- Prioritize Tensorleap integration files and avoid unrelated repository changes.
- Do not refactor or modify unrelated user/training/business logic.
- Preserve existing behavior outside the requested scope.
- If repository state is ambiguous or objective conflicts appear, stop and state the blocker clearly.
- Never run git commit/push/rebase/reset operations.
- Never access files outside the repository root unless explicitly instructed.
`

type commandRunner func(ctx context.Context, dir, command string, args []string) ([]byte, []byte, error)

// Runner executes one task-scoped command invocation and writes a transcript.
type Runner struct {
	timeout    time.Duration
	lookPath   func(string) (string, error)
	runCommand commandRunner
}

// NewRunner creates an agent runner that invokes Claude Code.
func NewRunner() *Runner {
	return &Runner{
		timeout:    defaultTimeout,
		lookPath:   exec.LookPath,
		runCommand: runAgentCommand,
	}
}

// CheckAvailability reports whether Claude Code can be resolved from PATH.
func (r *Runner) CheckAvailability() error {
	r.ensureDefaults()
	_, err := r.resolveCommand()
	return err
}

// Run executes the task via Claude Code and writes a transcript file.
func (r *Runner) Run(ctx context.Context, task AgentTask) (AgentResult, error) {
	r.ensureDefaults()

	if strings.TrimSpace(task.Objective) == "" {
		return AgentResult{}, core.NewError(core.KindUnknown, "agent.runner.task_objective", "agent task objective is required")
	}
	repoRoot := strings.TrimSpace(task.RepoRoot)
	if repoRoot == "" {
		return AgentResult{}, core.NewError(core.KindUnknown, "agent.runner.task_repo_root", "agent task repo root is required")
	}
	transcriptPath := strings.TrimSpace(task.TranscriptPath)
	if transcriptPath == "" {
		return AgentResult{}, core.NewError(core.KindUnknown, "agent.runner.task_transcript_path", "agent task transcript path is required")
	}

	commandPath, err := r.resolveCommand()
	if err != nil {
		return AgentResult{}, err
	}

	runCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok && r.timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, r.timeout)
	}
	defer cancel()

	prompt := renderPrompt(task)
	args := append(append([]string(nil), defaultClaudeArgs...), prompt)

	stdout, stderr, runErr := r.runCommand(runCtx, repoRoot, commandPath, args)
	if writeErr := writeTranscript(transcriptPath, commandPath, args, task, stdout, stderr, runErr); writeErr != nil {
		return AgentResult{}, core.WrapError(core.KindUnknown, "agent.runner.transcript_write", writeErr)
	}
	if runErr != nil {
		return AgentResult{}, core.WrapError(core.KindUnknown, "agent.runner.run", runErr)
	}

	return AgentResult{
		Applied:        true,
		TranscriptPath: transcriptPath,
		Summary:        "agent task completed",
		Evidence: []core.EvidenceItem{
			{Name: "agent.command", Value: commandPath},
		},
	}, nil
}

func (r *Runner) ensureDefaults() {
	if r.lookPath == nil {
		r.lookPath = exec.LookPath
	}
	if r.runCommand == nil {
		r.runCommand = runAgentCommand
	}
	if r.timeout <= 0 {
		r.timeout = defaultTimeout
	}
}

func (r *Runner) resolveCommand() (string, error) {
	path, err := r.lookPath(claudeCommand)
	if err != nil {
		return "", core.WrapError(
			core.KindMissingDependency,
			"agent.runner.command_lookup",
			fmt.Errorf("Claude CLI is not available on PATH (expected %q): %w", claudeCommand, err),
		)
	}
	return path, nil
}

func renderPrompt(task AgentTask) string {
	var b strings.Builder
	b.WriteString("System context (must follow):\n")
	b.WriteString(strings.TrimSpace(conciergeOperatingPolicy))
	b.WriteString("\n\n")

	b.WriteString("Repository root: ")
	b.WriteString(task.RepoRoot)
	b.WriteString("\n\n")
	b.WriteString("Objective:\n")
	b.WriteString(strings.TrimSpace(task.Objective))
	b.WriteString("\n\n")
	if len(task.Constraints) > 0 {
		b.WriteString("Constraints:\n")
		for _, constraint := range task.Constraints {
			trimmed := strings.TrimSpace(constraint)
			if trimmed == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(trimmed)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Apply the required code changes directly in this repository. Keep edits focused on the objective and constraints.")
	return b.String()
}

func runAgentCommand(ctx context.Context, dir, command string, args []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if stderrText := strings.TrimSpace(stderr.String()); stderrText != "" {
			err = fmt.Errorf("%w (stderr: %s)", err, stderrText)
		}
	}

	return stdout.Bytes(), stderr.Bytes(), err
}

func writeTranscript(path, command string, args []string, task AgentTask, stdout, stderr []byte, runErr error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("System context:\n")
	b.WriteString(strings.TrimSpace(conciergeOperatingPolicy))
	b.WriteString("\n\n")

	b.WriteString("Objective:\n")
	b.WriteString(task.Objective)
	b.WriteString("\n\n")

	if len(task.Constraints) > 0 {
		b.WriteString("Constraints:\n")
		for _, constraint := range task.Constraints {
			trimmed := strings.TrimSpace(constraint)
			if trimmed == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(trimmed)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("Command:\n")
	b.WriteString(command)
	if len(args) > 0 {
		b.WriteString(" ")
		b.WriteString(strings.Join(args, " "))
	}
	b.WriteString("\n\n")

	if runErr != nil {
		b.WriteString("Run error:\n")
		b.WriteString(runErr.Error())
		b.WriteString("\n\n")
	}

	b.WriteString("STDOUT:\n")
	stdoutText := strings.TrimSpace(string(stdout))
	if stdoutText == "" {
		stdoutText = "<empty>"
	}
	b.WriteString(stdoutText)
	b.WriteString("\n\n")

	b.WriteString("STDERR:\n")
	stderrText := strings.TrimSpace(string(stderr))
	if stderrText == "" {
		stderrText = "<empty>"
	}
	b.WriteString(stderrText)
	b.WriteString("\n")

	return os.WriteFile(path, []byte(b.String()), 0o644)
}
