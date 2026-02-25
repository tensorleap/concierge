package cli

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestRunDryRunPrintsExecutionStages(t *testing.T) {
	output, err := executeCLI(t, "run", "--dry-run")
	if err != nil {
		t.Fatalf("run --dry-run failed: %v", err)
	}

	if !strings.Contains(output, "dry-run plan:") {
		t.Fatalf("expected dry-run plan prefix in output, got: %q", output)
	}

	expected := expectedStageChainFromCore()
	if !strings.Contains(output, expected) {
		t.Fatalf("expected dry-run stages in output, got: %q", output)
	}
}

func TestRunDryRunUsesCoreDefaultStages(t *testing.T) {
	output, err := executeCLI(t, "run", "--dry-run")
	if err != nil {
		t.Fatalf("run --dry-run failed: %v", err)
	}

	expected := expectedStageChainFromCore()
	if !strings.Contains(output, expected) {
		t.Fatalf("expected output to contain core stage chain %q, got: %q", expected, output)
	}
}

func TestRunWithoutDryRunReturnsNotImplementedError(t *testing.T) {
	_, err := executeCLI(t, "run")
	if err == nil {
		t.Fatal("expected run without --dry-run to fail")
	}

	if !strings.Contains(err.Error(), "run is not implemented yet; use --dry-run") {
		t.Fatalf("expected not-implemented message, got: %v", err)
	}
}

func expectedStageChainFromCore() string {
	stages := core.DefaultStages()
	names := make([]string, 0, len(stages))
	for _, stage := range stages {
		names = append(names, string(stage))
	}
	return strings.Join(names, " -> ")
}
