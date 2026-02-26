package report

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestFileReporterWritesReportJSON(t *testing.T) {
	projectRoot := t.TempDir()
	var sink strings.Builder

	reporter, err := NewFileReporter(projectRoot, &sink)
	if err != nil {
		t.Fatalf("NewFileReporter returned error: %v", err)
	}

	report := core.IterationReport{
		SnapshotID: "snapshot-123",
		Step:       core.EnsureStep{ID: core.EnsureStepLeapYAML},
		Validation: core.ValidationResult{Passed: true},
	}
	if err := reporter.Report(context.Background(), report); err != nil {
		t.Fatalf("Report returned error: %v", err)
	}

	reportPath := filepath.Join(projectRoot, ".concierge", "reports", "snapshot-123.json")
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var decoded core.IterationReport
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.SnapshotID != "snapshot-123" {
		t.Fatalf("expected snapshot ID %q, got %q", "snapshot-123", decoded.SnapshotID)
	}
	if decoded.Step.ID != core.EnsureStepLeapYAML {
		t.Fatalf("expected step %q, got %q", core.EnsureStepLeapYAML, decoded.Step.ID)
	}
	if !strings.Contains(sink.String(), "snapshot=snapshot-123") {
		t.Fatalf("expected summary line in output, got %q", sink.String())
	}
}

func TestFileReporterWritesEvidenceFiles(t *testing.T) {
	projectRoot := t.TempDir()
	reporter, err := NewFileReporter(projectRoot, nil)
	if err != nil {
		t.Fatalf("NewFileReporter returned error: %v", err)
	}

	report := core.IterationReport{
		SnapshotID: "snapshot-456",
		Step:       core.EnsureStep{ID: core.EnsureStepIntegrationScript},
		Validation: core.ValidationResult{Passed: true},
		Evidence: []core.EvidenceItem{
			{Name: "executor.mode", Value: "stub"},
			{Name: "encoder/input", Value: "ok"},
		},
	}
	if err := reporter.Report(context.Background(), report); err != nil {
		t.Fatalf("Report returned error: %v", err)
	}

	evidenceRoot := filepath.Join(projectRoot, ".concierge", "evidence", "snapshot-456")
	cases := map[string]string{
		"executor.mode.log": "stub\n",
		"encoder_input.log": "ok\n",
	}
	for fileName, expected := range cases {
		contents, err := os.ReadFile(filepath.Join(evidenceRoot, fileName))
		if err != nil {
			t.Fatalf("ReadFile failed for %s: %v", fileName, err)
		}
		if string(contents) != expected {
			t.Fatalf("expected %q in %s, got %q", expected, fileName, string(contents))
		}
	}
}

func TestFileReporterPreservesExistingEvidenceDirectory(t *testing.T) {
	projectRoot := t.TempDir()
	evidenceRoot := filepath.Join(projectRoot, ".concierge", "evidence", "snapshot-789")
	if err := os.MkdirAll(evidenceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	markerPath := filepath.Join(evidenceRoot, "existing.log")
	if err := os.WriteFile(markerPath, []byte("keep\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	reporter, err := NewFileReporter(projectRoot, nil)
	if err != nil {
		t.Fatalf("NewFileReporter returned error: %v", err)
	}
	if err := reporter.Report(context.Background(), core.IterationReport{
		SnapshotID: "snapshot-789",
		Step:       core.EnsureStep{ID: core.EnsureStepLeapYAML},
		Validation: core.ValidationResult{Passed: true},
		Evidence: []core.EvidenceItem{
			{Name: "executor.mode", Value: "stub"},
		},
	}); err != nil {
		t.Fatalf("Report returned error: %v", err)
	}

	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected marker file to persist, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(evidenceRoot, "executor.mode.log")); err != nil {
		t.Fatalf("expected new evidence file, got error: %v", err)
	}
}
