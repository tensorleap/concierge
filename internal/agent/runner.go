package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

const (
	// CommandEnvVar points to the agent command Concierge should run when agent delegation is enabled.
	CommandEnvVar = "CONCIERGE_AGENT_COMMAND"
	// ArgsEnvVar optionally provides additional command arguments for the agent command.
	ArgsEnvVar = "CONCIERGE_AGENT_ARGS"
)

const defaultTimeout = 15 * time.Minute

type commandRunner func(ctx context.Context, dir, command string, args, env []string) ([]byte, []byte, error)

// RunnerOptions configures the agent runner.
type RunnerOptions struct {
	Command string
	Args    []string
	Timeout time.Duration
}

// Runner executes one task-scoped command invocation and writes a transcript.
type Runner struct {
	command    string
	args       []string
	timeout    time.Duration
	getEnv     func(string) string
	lookPath   func(string) (string, error)
	runCommand commandRunner
}

// NewRunner creates an agent runner with environment-backed defaults.
func NewRunner(options RunnerOptions) *Runner {
	return &Runner{
		command:    strings.TrimSpace(options.Command),
		args:       append([]string(nil), options.Args...),
		timeout:    options.Timeout,
		getEnv:     os.Getenv,
		lookPath:   exec.LookPath,
		runCommand: runAgentCommand,
	}
}

// CheckAvailability reports whether the configured agent command can be resolved.
func (r *Runner) CheckAvailability() error {
	r.ensureDefaults()
	_, _, err := r.resolveCommand()
	return err
}

// Run executes the task via the configured agent command and writes a transcript file.
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

	commandPath, args, err := r.resolveCommand()
	if err != nil {
		return AgentResult{}, err
	}

	runCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok && r.timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, r.timeout)
	}
	defer cancel()

	payload := map[string]any{
		"objective":   task.Objective,
		"constraints": task.Constraints,
	}
	payloadJSON, _ := json.Marshal(payload)
	env := []string{
		"CONCIERGE_AGENT_OBJECTIVE=" + task.Objective,
		"CONCIERGE_AGENT_CONSTRAINTS=" + strings.Join(task.Constraints, "\n"),
		"CONCIERGE_AGENT_TASK_JSON=" + string(payloadJSON),
	}

	stdout, stderr, runErr := r.runCommand(runCtx, repoRoot, commandPath, args, env)
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
	if r.getEnv == nil {
		r.getEnv = os.Getenv
	}
	if r.lookPath == nil {
		r.lookPath = exec.LookPath
	}
	if r.runCommand == nil {
		r.runCommand = runAgentCommand
	}
	if r.timeout <= 0 {
		r.timeout = defaultTimeout
	}
	if len(r.args) == 0 {
		r.args = strings.Fields(strings.TrimSpace(r.getEnv(ArgsEnvVar)))
	}
	if strings.TrimSpace(r.command) == "" {
		r.command = strings.TrimSpace(r.getEnv(CommandEnvVar))
	}
}

func (r *Runner) resolveCommand() (string, []string, error) {
	command := strings.TrimSpace(r.command)
	if command == "" {
		return "", nil, core.NewError(
			core.KindMissingDependency,
			"agent.runner.command_missing",
			"agent delegation is enabled but no agent command is configured (set --agent-command or CONCIERGE_AGENT_COMMAND)",
		)
	}

	path, err := r.lookPath(command)
	if err != nil {
		return "", nil, core.WrapError(
			core.KindMissingDependency,
			"agent.runner.command_lookup",
			fmt.Errorf("agent command %q is not available: %w", command, err),
		)
	}

	args := append([]string(nil), r.args...)
	return path, args, nil
}

func runAgentCommand(ctx context.Context, dir, command string, args, env []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)

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
