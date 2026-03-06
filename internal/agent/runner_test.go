package agent

import (
	"context"
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
	binDir := t.TempDir()
	transcriptPath := filepath.Join(repoRoot, ".concierge", "evidence", "snapshot-system", "agent.transcript.log")
	task := validAgentTask(repoRoot, transcriptPath)

	installMockClaude(t, binDir, "#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"${1-}\" == \"--help\" ]]; then\ncat <<'EOF'\n--output-format stream-json\n--include-partial-messages\nEOF\nexit 0\nfi\necho '{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"ok\"}]}}'\necho '{\"type\":\"result\",\"result\":\"done\"}'\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	runner := NewRunner()
	_, err := runner.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	raw, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("ReadFile failed for transcript: %v", err)
	}
	contents := string(raw)
	if !strings.Contains(contents, "--output-format stream-json") {
		t.Fatalf("expected stream-json invocation in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "--include-partial-messages") {
		t.Fatalf("expected partial-message flag in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "--verbose") {
		t.Fatalf("expected verbose flag in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, BuildClaudeSystemPrompt()) {
		t.Fatalf("expected system prompt in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, BuildClaudeTaskPrompt(task)) {
		t.Fatalf("expected task prompt in transcript, got: %q", contents)
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
	installMockClaude(t, binDir, "#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"${1-}\" == \"--help\" ]]; then\ncat <<'EOF'\n--output-format stream-json\n--include-partial-messages\nEOF\nexit 0\nfi\necho '{\"type\":\"stream_event\",\"event\":{\"type\":\"content_block_start\",\"content_block\":{\"type\":\"tool_use\",\"name\":\"Read\",\"input\":{\"file_path\":\"leap_binder.py\"}}}}'\necho '{\"type\":\"stream_event\",\"event\":{\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"planning fix\"}}}'\necho '{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"final answer\"}]}}'\necho '{\"type\":\"result\",\"result\":\"done\"}'\necho 'stderr line' >&2\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

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
	if !strings.Contains(contents, "--output-format stream-json") {
		t.Fatalf("expected stream-json command in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "--verbose") {
		t.Fatalf("expected verbose flag in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "[tool] Scanning repository code: leap_binder.py") {
		t.Fatalf("expected tool transcript line, got: %q", contents)
	}
	if !strings.Contains(contents, "planning fix") {
		t.Fatalf("expected message text in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "final answer") {
		t.Fatalf("expected assistant text in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "stderr line") {
		t.Fatalf("expected stderr in transcript, got: %q", contents)
	}
	if result.RawStreamPath == "" {
		t.Fatal("expected raw stream path on successful streaming run")
	}
	rawStream, err := os.ReadFile(result.RawStreamPath)
	if err != nil {
		t.Fatalf("ReadFile failed for raw stream: %v", err)
	}
	if !strings.Contains(string(rawStream), "\"type\":\"assistant\"") {
		t.Fatalf("expected assistant event in raw stream, got: %q", string(rawStream))
	}
}

func installMockClaude(t *testing.T, binDir, body string) {
	t.Helper()
	scriptPath := filepath.Join(binDir, "claude")
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock claude command: %v", err)
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
