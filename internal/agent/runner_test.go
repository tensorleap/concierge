package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestRunnerCheckAvailabilityReturnsMissingDependencyWhenClaudeMissing(t *testing.T) {
	runner := NewRunner()
	runner.lookPath = func(file string) (string, error) {
		return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
	}
	err := runner.CheckAvailability()
	if err == nil {
		t.Fatal("expected missing dependency error when Claude is unavailable")
	}
	if got := core.KindOf(err); got != core.KindMissingDependency {
		t.Fatalf("expected error kind %q, got %q (err=%v)", core.KindMissingDependency, got, err)
	}
}

func TestRunnerInvokesClaudeWithSystemPrompt(t *testing.T) {
	repoRoot := t.TempDir()
	transcriptPath := filepath.Join(repoRoot, ".concierge", "evidence", "snapshot-system", "agent.transcript.log")
	task := validAgentTask(repoRoot, transcriptPath)

	runner := NewRunner()
	runner.lookPath = func(file string) (string, error) {
		return "/usr/local/bin/claude", nil
	}

	var capturedArgs []string
	runner.runCommand = func(ctx context.Context, dir, command string, args []string) ([]byte, []byte, error) {
		_ = ctx
		if dir != repoRoot {
			t.Fatalf("expected command dir %q, got %q", repoRoot, dir)
		}
		if command != "/usr/local/bin/claude" {
			t.Fatalf("expected command path %q, got %q", "/usr/local/bin/claude", command)
		}
		capturedArgs = append([]string(nil), args...)
		return []byte("ok"), nil, nil
	}

	_, err := runner.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	index := -1
	for i, arg := range capturedArgs {
		if arg == "--system-prompt" {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatalf("expected --system-prompt flag in args, got: %+v", capturedArgs)
	}
	if len(capturedArgs) <= index+1 {
		t.Fatalf("expected system prompt value after --system-prompt, got: %+v", capturedArgs)
	}
	if capturedArgs[index+1] != BuildClaudeSystemPrompt() {
		t.Fatalf("unexpected system prompt value: %q", capturedArgs[index+1])
	}
	if gotPrompt := capturedArgs[len(capturedArgs)-1]; gotPrompt != BuildClaudeTaskPrompt(task) {
		t.Fatalf("expected final arg to be task prompt, got: %q", gotPrompt)
	}
}

func TestRunnerFailsFastWhenRequiredContextPayloadIsMissing(t *testing.T) {
	repoRoot := t.TempDir()
	transcriptPath := filepath.Join(repoRoot, ".concierge", "evidence", "snapshot-missing", "agent.transcript.log")
	task := AgentTask{
		Objective:      "Implement preprocess",
		RepoRoot:       repoRoot,
		TranscriptPath: transcriptPath,
	}

	runner := NewRunner()
	lookPathCalled := false
	runner.lookPath = func(file string) (string, error) {
		lookPathCalled = true
		return "/usr/local/bin/claude", nil
	}

	_, err := runner.Run(context.Background(), task)
	if err == nil {
		t.Fatal("expected context-payload validation error")
	}
	if got := core.KindOf(err); got != core.KindUnknown {
		t.Fatalf("expected error kind %q, got %q (err=%v)", core.KindUnknown, got, err)
	}
	if !strings.Contains(err.Error(), "scope policy") {
		t.Fatalf("expected missing-scope-policy error, got: %v", err)
	}
	if lookPathCalled {
		t.Fatalf("expected runner to fail before command lookup when context payload is missing")
	}
}

func TestRunnerRunWritesTranscript(t *testing.T) {
	repoRoot := t.TempDir()
	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "claude")
	script := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"echo \"argv: $*\"\n" +
		"echo \"prompt: ${@: -1}\"\n" +
		"echo \"stderr line\" >&2\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile failed for script: %v", err)
	}
	t.Setenv("PATH", fmt.Sprintf("%s%c%s", binDir, os.PathListSeparator, os.Getenv("PATH")))

	transcriptPath := filepath.Join(repoRoot, ".concierge", "evidence", "snapshot-1", "agent.transcript.log")
	runner := NewRunner()
	task := validAgentTask(repoRoot, transcriptPath)
	task.Objective = "Implement preprocess contract"
	task.AcceptanceChecks = []string{"Keep existing APIs stable"}

	result, err := runner.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected Applied=true on successful agent run")
	}
	if result.TranscriptPath != transcriptPath {
		t.Fatalf("expected transcript path %q, got %q", transcriptPath, result.TranscriptPath)
	}

	raw, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("ReadFile failed for transcript: %v", err)
	}
	contents := string(raw)
	if !strings.Contains(contents, "System prompt:\nYou are a task-scoped coding collaborator running under Concierge.") {
		t.Fatalf("expected system prompt in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "Task prompt:\nObjective:\nImplement preprocess contract") {
		t.Fatalf("expected structured task prompt in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "argv: --print --output-format text --permission-mode bypassPermissions --system-prompt") {
		t.Fatalf("expected claude arguments in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "prompt: Objective:") {
		t.Fatalf("expected stdout in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "stderr line") {
		t.Fatalf("expected stderr in transcript, got: %q", contents)
	}
}

func validAgentTask(repoRoot, transcriptPath string) AgentTask {
	return AgentTask{
		Objective: "Implement preprocess contract",
		ScopePolicy: &AgentScopePolicy{
			AllowedFiles:       []string{"leap_binder.py"},
			ForbiddenAreas:     []string{"Do not touch training loop"},
			StopAndAskTriggers: []string{"Missing model context"},
			DomainSections:     []string{"preprocess_contract"},
		},
		RepoContext: &core.AgentRepoContext{
			RepoRoot:           repoRoot,
			EntryFile:          "leap_binder.py",
			BinderFile:         "leap_binder.py",
			LeapYAMLBoundary:   "leap.yaml present",
			SelectedModelPath:  "models/model.onnx",
			ModelCandidates:    []string{"models/model.onnx"},
			DecoratorInventory: []string{"preprocess:build_preprocess"},
		},
		DomainKnowledge: &AgentDomainKnowledgePack{
			Version:    "tlkp-v1",
			SectionIDs: []string{"preprocess_contract"},
			Sections: map[string]string{
				"preprocess_contract": "Preprocess must produce train and validation subsets.",
			},
		},
		AcceptanceChecks: []string{"Implement preprocess contract"},
		RepoRoot:         repoRoot,
		TranscriptPath:   transcriptPath,
	}
}
