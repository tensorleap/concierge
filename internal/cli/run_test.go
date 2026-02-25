package cli

import (
	"strings"
	"testing"
)

func TestRunDryRunPrintsExecutionStages(t *testing.T) {
	output, err := executeCLI(t, "run", "--dry-run")
	if err != nil {
		t.Fatalf("run --dry-run failed: %v", err)
	}

	expected := "snapshot -> inspect -> plan -> execute -> validate -> report"
	if !strings.Contains(output, expected) {
		t.Fatalf("expected dry-run stages in output, got: %q", output)
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
