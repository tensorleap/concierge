package cli

import (
	"strings"
	"testing"
)

func TestDoctorPrintsDiagnostics(t *testing.T) {
	output, err := executeCLI(t, "doctor")
	if err != nil {
		t.Fatalf("doctor command failed: %v", err)
	}

	expectedFields := []string{
		"version:",
		"commit:",
		"date:",
		"go:",
		"os:",
		"arch:",
	}

	for _, field := range expectedFields {
		if !strings.Contains(output, field) {
			t.Fatalf("expected output to contain %q, got: %q", field, output)
		}
	}
}
