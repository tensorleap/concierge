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
	if !strings.Contains(contents, "argv: --print --output-format text --permission-mode bypassPermissions") {
		t.Fatalf("expected claude arguments in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "prompt: Repository root:") {
		t.Fatalf("expected stdout in transcript, got: %q", contents)
	}
	if !strings.Contains(contents, "stderr line") {
		t.Fatalf("expected stderr in transcript, got: %q", contents)
	}
}
