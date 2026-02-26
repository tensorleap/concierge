package cli

import (
	"context"
	"strings"
	"testing"
)

func TestLogLevelValidationAcceptsSupportedValues(t *testing.T) {
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

	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		output, err := executeCLI(t, "--log-level", level, "doctor")
		if err != nil {
			t.Fatalf("expected %q to be accepted, got error: %v", level, err)
		}
		if !strings.Contains(output, "Concierge Doctor") {
			t.Fatalf("expected doctor output for %q, got: %q", level, output)
		}
	}
}

func TestLogLevelValidationRejectsUnsupportedValues(t *testing.T) {
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

	_, err := executeCLI(t, "--log-level", "trace", "doctor")
	if err == nil {
		t.Fatal("expected invalid log level to fail")
	}

	if !strings.Contains(err.Error(), "invalid value for --log-level") {
		t.Fatalf("expected invalid log-level message, got: %v", err)
	}
}
