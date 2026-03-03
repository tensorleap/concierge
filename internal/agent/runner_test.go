package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestRunnerCheckAvailabilityReturnsMissingDependencyWhenCommandUnset(t *testing.T) {
	t.Setenv(CommandEnvVar, "")
	t.Setenv(ArgsEnvVar, "")

	runner := NewRunner(RunnerOptions{})
	err := runner.CheckAvailability()
	if err == nil {
		t.Fatal("expected missing dependency error when no command is configured")
	}
	if got := core.KindOf(err); got != core.KindMissingDependency {
		t.Fatalf("expected error kind %q, got %q (err=%v)", core.KindMissingDependency, got, err)
	}
}

func TestRunnerRunWritesTranscript(t *testing.T) {
	repoRoot := t.TempDir()
	scriptPath := filepath.Join(t.TempDir(), "agent.sh")
	script := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"echo \"agent objective: ${CONCIERGE_AGENT_OBJECTIVE}\"\n" +
		"echo \"stderr line\" >&2\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile failed for script: %v", err)
	}

	transcriptPath := filepath.Join(repoRoot, ".concierge", "evidence", "snapshot-1", "agent.transcript.log")
	runner := NewRunner(RunnerOptions{Command: scriptPath})
	result, err := runner.Run(context.Background(), AgentTask{
		Objective:      "Implement preprocess contract",
		Constraints:    []string{"Keep existing APIs stable"},
		RepoRoot:       repoRoot,
		TranscriptPath: transcriptPath,
	})
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
	if !strings.Contains(contents, "Objective:\nImplement preprocess contract") {
		t.Fatalf("expected objective in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "agent objective: Implement preprocess contract") {
		t.Fatalf("expected stdout in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "stderr line") {
		t.Fatalf("expected stderr in transcript, got: %q", contents)
	}
}
