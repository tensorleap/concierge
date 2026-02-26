package report

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/persistence"
)

// FileReporter persists iteration artifacts to .concierge and prints a one-line summary.
type FileReporter struct {
	writer  io.Writer
	paths   *persistence.Paths
	options OutputOptions
}

// NewFileReporter creates a reporter that writes report/evidence artifacts to .concierge.
func NewFileReporter(projectRoot string, writer io.Writer) (*FileReporter, error) {
	return NewFileReporterWithOptions(projectRoot, writer, OutputOptions{})
}

// NewFileReporterWithOptions creates a reporter that writes report/evidence artifacts to .concierge.
func NewFileReporterWithOptions(projectRoot string, writer io.Writer, options OutputOptions) (*FileReporter, error) {
	paths, err := persistence.NewPaths(projectRoot)
	if err != nil {
		return nil, err
	}

	if writer == nil {
		writer = os.Stdout
	}

	return &FileReporter{
		writer:  writer,
		paths:   paths,
		options: options,
	}, nil
}

// Report persists the iteration report and evidence items, then writes a one-line summary.
func (r *FileReporter) Report(ctx context.Context, report core.IterationReport) error {
	_ = ctx

	if r.paths == nil {
		return core.NewError(core.KindUnknown, "report.file.paths", "paths are not configured")
	}

	if err := persistence.WriteJSONAtomic(r.paths.ReportFile(report.SnapshotID), report); err != nil {
		return core.WrapError(core.KindUnknown, "report.file.report_json", err)
	}

	for _, item := range report.Evidence {
		if err := writeEvidenceItem(r.paths, report.SnapshotID, item); err != nil {
			return err
		}
	}

	writer := r.writer
	if writer == nil {
		writer = os.Stdout
	}
	if err := writeSummaryLine(writer, report, r.options); err != nil {
		return core.WrapError(core.KindUnknown, "report.file.summary", err)
	}

	return nil
}

func writeEvidenceItem(paths *persistence.Paths, snapshotID string, item core.EvidenceItem) error {
	evidencePath := paths.EvidenceFile(snapshotID, item.Name)
	evidenceDir := paths.EvidenceDir(snapshotID)
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		return core.WrapError(core.KindUnknown, "report.file.evidence_mkdir", err)
	}

	value := item.Value
	if !strings.HasSuffix(value, "\n") {
		value += "\n"
	}
	if err := os.WriteFile(evidencePath, []byte(value), 0o644); err != nil {
		return core.WrapError(core.KindUnknown, "report.file.evidence_write", err)
	}

	return nil
}
