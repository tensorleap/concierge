package report

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestReporterWritesSingleLineSummary(t *testing.T) {
	var sink strings.Builder
	reporter := NewStdoutReporter(&sink)

	err := reporter.Report(context.Background(), core.IterationReport{
		SnapshotID: "snapshot-123",
		Step:       core.EnsureStep{ID: core.EnsureStepLeapYAML},
		Validation: core.ValidationResult{Passed: true},
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}

	output := sink.String()
	expectedSnippets := []string{
		"Iteration Update",
		"Step: Check leap.yaml setup",
		"Changes: No repository changes applied",
		"Validation: Passed",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(strings.ToLower(output), strings.ToLower(snippet)) {
			t.Fatalf("expected output to contain %q, got %q", snippet, output)
		}
	}
}

func TestReporterReturnsWriteError(t *testing.T) {
	reporter := NewStdoutReporter(failingWriter{})

	err := reporter.Report(context.Background(), core.IterationReport{SnapshotID: "snapshot-1"})
	if err == nil {
		t.Fatal("expected write error")
	}
	if !errors.Is(err, errWriteFailed) {
		t.Fatalf("expected wrapped write error, got %v", err)
	}
}

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (f failingWriter) Write(p []byte) (int, error) {
	_ = p
	return 0, errWriteFailed
}
