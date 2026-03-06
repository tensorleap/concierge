package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLiveClaudeSmokePreprocessStep(t *testing.T) {
	if os.Getenv("CONCIERGE_LIVE_CLAUDE") != "1" {
		t.Skip("set CONCIERGE_LIVE_CLAUDE=1 to run live Claude smoke tests")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skipf("claude not available on PATH: %v", err)
	}

	t.Setenv("CONCIERGE_ENABLE_HARNESS", "0")
	mockLeapCLIInstalled(t)

	repo := initRunTestRepo(t, true)
	writeFile(t, filepath.Join(repo, "leap_binder.py"), strings.Join([]string{
		"def helper():",
		"    return []",
		"",
	}, "\n"))
	withWorkingDir(t, repo)

	output, err := executeCLI(t, "run", "--yes", "--persist", "--max-iterations=1")
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "pending requirements") {
		t.Fatalf("expected success or max-iterations stop, got err=%v output=%q", err, output)
	}
	if strings.Contains(output, "requires --verbose") {
		t.Fatalf("expected live run not to fail on missing --verbose, got output=%q", output)
	}
	if !strings.Contains(output, "Claude started") {
		t.Fatalf("expected live highlights to show Claude start, got output=%q", output)
	}

	matches, globErr := filepath.Glob(filepath.Join(repo, ".concierge", "evidence", "*", "agent.stream.jsonl"))
	if globErr != nil {
		t.Fatalf("Glob failed: %v", globErr)
	}
	if len(matches) == 0 {
		t.Fatalf("expected agent.stream.jsonl to be persisted, got none in %q", filepath.Join(repo, ".concierge", "evidence"))
	}

	raw, readErr := os.ReadFile(matches[0])
	if readErr != nil {
		t.Fatalf("ReadFile failed for %q: %v", matches[0], readErr)
	}
	if !strings.Contains(string(raw), `"type":"result"`) {
		t.Fatalf("expected result event in persisted agent stream, got %q", string(raw))
	}
}
