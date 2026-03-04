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
	if err := validateTaskContextPayload(task); err != nil {
		return AgentResult{}, err
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

	systemPrompt := BuildClaudeSystemPrompt()
	taskPrompt := BuildClaudeTaskPrompt(task)
	args := append(append([]string(nil), defaultClaudeArgs...), "--system-prompt", systemPrompt, taskPrompt)

	stdout, stderr, runErr := r.runCommand(runCtx, repoRoot, commandPath, args)
	if writeErr := writeTranscript(transcriptPath, commandPath, args, systemPrompt, taskPrompt, stdout, stderr, runErr); writeErr != nil {
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

func validateTaskContextPayload(task AgentTask) error {
	if task.ScopePolicy == nil {
		return core.NewError(core.KindUnknown, "agent.runner.task_scope_policy", "agent task scope policy is required")
	}
	if task.RepoContext == nil {
		return core.NewError(core.KindUnknown, "agent.runner.task_repo_context", "agent task repository context is required")
	}
	if strings.TrimSpace(task.RepoContext.RepoRoot) == "" {
		return core.NewError(core.KindUnknown, "agent.runner.task_repo_context", "agent task repository context repoRoot is required")
	}
	if task.DomainKnowledge == nil {
		return core.NewError(core.KindUnknown, "agent.runner.task_domain_knowledge", "agent task domain knowledge slice is required")
	}
	if strings.TrimSpace(task.DomainKnowledge.Version) == "" {
		return core.NewError(core.KindUnknown, "agent.runner.task_domain_knowledge", "agent task domain knowledge version is required")
	}

	sectionIDs := normalizedUniqueOrdered(task.DomainKnowledge.SectionIDs)
	if len(sectionIDs) == 0 {
		return core.NewError(core.KindUnknown, "agent.runner.task_domain_knowledge", "agent task domain knowledge section IDs are required")
	}

	missingSections := make([]string, 0)
	for _, sectionID := range sectionIDs {
		if strings.TrimSpace(task.DomainKnowledge.Sections[sectionID]) != "" {
			continue
		}
		missingSections = append(missingSections, sectionID)
	}
	if len(missingSections) > 0 {
		return core.NewError(
			core.KindUnknown,
			"agent.runner.task_domain_knowledge",
			fmt.Sprintf("agent task domain knowledge is missing section body for: %s", strings.Join(missingSections, ", ")),
		)
	}

	return nil
}

func writeTranscript(path, command string, args []string, systemPrompt, taskPrompt string, stdout, stderr []byte, runErr error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("System prompt:\n")
	b.WriteString(strings.TrimSpace(systemPrompt))
	b.WriteString("\n\n")

	b.WriteString("Task prompt:\n")
	b.WriteString(strings.TrimSpace(taskPrompt))
	b.WriteString("\n\n")

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
