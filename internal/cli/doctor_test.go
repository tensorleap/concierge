package cli

import (
	"context"
	"strings"
	"testing"
)

func TestDoctorPrintsDiagnostics(t *testing.T) {
	restore := setLeapCLIDiagnosticsProviderForTest(func(ctx context.Context) leapCLIDiagnostics {
		_ = ctx
		return leapCLIDiagnostics{
			Installed:      true,
			CurrentVersion: "v1.2.3",
			LatestVersion:  "v1.2.3",
			Status:         "up_to_date",
			Note:           "ok",
			Action:         "none",
		}
	})
	defer restore()

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
		"leap_cli_installed:",
		"leap_cli_current_version:",
		"leap_cli_latest_version:",
		"leap_cli_status:",
		"leap_cli_note:",
		"leap_cli_action:",
	}

	for _, field := range expectedFields {
		if !strings.Contains(output, field) {
			t.Fatalf("expected output to contain %q, got: %q", field, output)
		}
	}
}
