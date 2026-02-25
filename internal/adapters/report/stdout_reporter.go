package report

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/tensorleap/concierge/internal/core"
)

// StdoutReporter writes a one-line summary for each iteration report.
type StdoutReporter struct {
	writer io.Writer
}

// NewStdoutReporter creates a reporter that writes to the provided writer.
func NewStdoutReporter(writer io.Writer) *StdoutReporter {
	if writer == nil {
		writer = os.Stdout
	}
	return &StdoutReporter{writer: writer}
}

// Report writes a one-line summary of the iteration report.
func (r *StdoutReporter) Report(ctx context.Context, report core.IterationReport) error {
	_ = ctx

	writer := r.writer
	if writer == nil {
		writer = os.Stdout
	}

	validation := "failed"
	if report.Validation.Passed {
		validation = "passed"
	}

	line := fmt.Sprintf("snapshot=%s step=%s validation=%s\n", report.SnapshotID, report.Step.ID, validation)
	if _, err := io.WriteString(writer, line); err != nil {
		return core.WrapError(core.KindUnknown, "report.stdout.write", err)
	}

	return nil
}
