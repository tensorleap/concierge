package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerLiveClaudeStreamJSONContract(t *testing.T) {
	if os.Getenv("CONCIERGE_LIVE_CLAUDE") != "1" {
		t.Skip("set CONCIERGE_LIVE_CLAUDE=1 to run live Claude smoke tests")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skipf("claude not available on PATH: %v", err)
	}

	repoRoot := t.TempDir()
	transcriptPath := filepath.Join(repoRoot, ".concierge", "evidence", "snapshot-live", "agent.transcript.log")
	task := validAgentTask(repoRoot, transcriptPath)
	task.Objective = "Do not edit any files. Reply with exactly OK."
	task.AcceptanceChecks = []string{"Do not edit any files", "Reply with exactly OK"}

	runner := NewRunner()
	runner.timeout = 45 * time.Second

	result, err := runner.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected Applied=true from live runner")
	}
	if result.RawStreamPath == "" {
		t.Fatal("expected raw stream path from live runner")
	}

	rawTranscript, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("ReadFile failed for transcript: %v", err)
	}
	transcript := string(rawTranscript)
	if !strings.Contains(transcript, "--output-format stream-json") {
		t.Fatalf("expected stream-json invocation in transcript, got %q", transcript)
	}
	if !strings.Contains(transcript, "--include-partial-messages") {
		t.Fatalf("expected include-partial-messages flag in transcript, got %q", transcript)
	}
	if !strings.Contains(transcript, "--verbose") {
		t.Fatalf("expected verbose flag in transcript, got %q", transcript)
	}

	rawStream, err := os.ReadFile(result.RawStreamPath)
	if err != nil {
		t.Fatalf("ReadFile failed for raw stream: %v", err)
	}
	stream := string(rawStream)
	if !strings.Contains(stream, `"type":"result"`) {
		t.Fatalf("expected result event in raw stream, got %q", stream)
	}
}
