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

	expected := "snapshot=snapshot-123 step=ensure.leap_yaml validation=passed\n"
	if sink.String() != expected {
		t.Fatalf("expected output %q, got %q", expected, sink.String())
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
