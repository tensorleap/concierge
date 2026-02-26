package cli

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestDoctorHumanOutput(t *testing.T) {
	restoreDiag := setLeapCLIDiagnosticsProviderForTest(func(ctx context.Context) leapCLIDiagnostics {
		_ = ctx
		return leapCLIDiagnostics{
			Installed:      true,
			CurrentVersion: "v1.2.3",
			LatestVersion:  "v1.2.3",
			Status:         "up_to_date",
			Note:           "all good",
			Action:         "No action needed.",
		}
	})
	defer restoreDiag()

	restoreLogo := setDoctorLogoProviderForTest(func() string { return "TEST LOGO" })
	defer restoreLogo()

	restoreColor := setDoctorColorDepsForTest(
		func(key string) string {
			_ = key
			return ""
		},
		func(writer io.Writer) bool {
			_ = writer
			return false
		},
	)
	defer restoreColor()

	output, err := executeCLI(t, "doctor")
	if err != nil {
		t.Fatalf("doctor command failed: %v", err)
	}

	expected := []string{
		"TEST LOGO",
		"Concierge Doctor",
		"System",
		"Leap CLI",
		"✓ Healthy",
	}

	for _, token := range expected {
		if !strings.Contains(output, token) {
			t.Fatalf("expected output to contain %q, got: %q", token, output)
		}
	}

	if strings.Contains(output, "Quick Terms") {
		t.Fatalf("expected output to omit Quick Terms section, got: %q", output)
	}
	if strings.Contains(output, "Next Step") {
		t.Fatalf("expected output to omit Next Step section, got: %q", output)
	}
}

func TestDoctorJSONOutput(t *testing.T) {
	restoreDiag := setLeapCLIDiagnosticsProviderForTest(func(ctx context.Context) leapCLIDiagnostics {
		_ = ctx
		return leapCLIDiagnostics{
			Installed:      true,
			CurrentVersion: "v1.2.2",
			LatestVersion:  "v1.2.3",
			Status:         "outdated",
			Note:           "update available",
			Action:         "Upgrade Leap CLI.",
		}
	})
	defer restoreDiag()

	restoreLogo := setDoctorLogoProviderForTest(func() string { return "IGNORED LOGO" })
	defer restoreLogo()

	output, err := executeCLI(t, "doctor", "--format", "json")
	if err != nil {
		t.Fatalf("doctor command failed: %v", err)
	}

	var payload doctorJSONPayload
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("expected valid JSON output, got error: %v\noutput: %s", err, output)
	}

	if payload.LeapCLI.Status != "outdated" {
		t.Fatalf("expected leapCli.status %q, got %q", "outdated", payload.LeapCLI.Status)
	}
	if payload.LeapCLI.StatusLabel != "Update available" {
		t.Fatalf("expected leapCli.statusLabel %q, got %q", "Update available", payload.LeapCLI.StatusLabel)
	}
	if payload.Docs.LeapCLI != leapCLIInstallGuideURL {
		t.Fatalf("expected docs.leapCli %q, got %q", leapCLIInstallGuideURL, payload.Docs.LeapCLI)
	}
}

func TestDoctorRejectsInvalidFormat(t *testing.T) {
	restoreDiag := setLeapCLIDiagnosticsProviderForTest(func(ctx context.Context) leapCLIDiagnostics {
		_ = ctx
		return leapCLIDiagnostics{}
	})
	defer restoreDiag()

	_, err := executeCLI(t, "doctor", "--format", "yaml")
	if err == nil {
		t.Fatal("expected doctor command to fail for invalid format")
	}
	if !strings.Contains(err.Error(), "invalid value for --format") {
		t.Fatalf("expected invalid format message, got: %v", err)
	}
}

func TestDoctorNoColorFlagDisablesANSI(t *testing.T) {
	restoreDiag := setLeapCLIDiagnosticsProviderForTest(func(ctx context.Context) leapCLIDiagnostics {
		_ = ctx
		return leapCLIDiagnostics{
			Installed:      true,
			CurrentVersion: "v1.2.3",
			LatestVersion:  "v1.2.3",
			Status:         "up_to_date",
			Action:         "No action needed.",
		}
	})
	defer restoreDiag()

	restoreLogo := setDoctorLogoProviderForTest(func() string { return "" })
	defer restoreLogo()

	restoreColor := setDoctorColorDepsForTest(
		func(key string) string {
			_ = key
			return ""
		},
		func(writer io.Writer) bool {
			_ = writer
			return true
		},
	)
	defer restoreColor()

	output, err := executeCLI(t, "doctor", "--no-color")
	if err != nil {
		t.Fatalf("doctor command failed: %v", err)
	}
	if strings.Contains(output, "\x1b[") {
		t.Fatalf("expected no ANSI colors when --no-color is set, got: %q", output)
	}
}

func TestDoctorColorEnabledOnTerminal(t *testing.T) {
	restoreDiag := setLeapCLIDiagnosticsProviderForTest(func(ctx context.Context) leapCLIDiagnostics {
		_ = ctx
		return leapCLIDiagnostics{
			Installed:      true,
			CurrentVersion: "v1.2.3",
			LatestVersion:  "v1.2.3",
			Status:         "up_to_date",
			Action:         "No action needed.",
		}
	})
	defer restoreDiag()

	restoreLogo := setDoctorLogoProviderForTest(func() string { return "" })
	defer restoreLogo()

	restoreColor := setDoctorColorDepsForTest(
		func(key string) string {
			_ = key
			return ""
		},
		func(writer io.Writer) bool {
			_ = writer
			return true
		},
	)
	defer restoreColor()

	output, err := executeCLI(t, "doctor")
	if err != nil {
		t.Fatalf("doctor command failed: %v", err)
	}
	if !strings.Contains(output, "\x1b[") {
		t.Fatalf("expected ANSI colors on terminal output, got: %q", output)
	}
}

func TestDoctorRespectsNoColorEnvironment(t *testing.T) {
	restoreDiag := setLeapCLIDiagnosticsProviderForTest(func(ctx context.Context) leapCLIDiagnostics {
		_ = ctx
		return leapCLIDiagnostics{
			Installed:      true,
			CurrentVersion: "v1.2.3",
			LatestVersion:  "v1.2.3",
			Status:         "up_to_date",
			Action:         "No action needed.",
		}
	})
	defer restoreDiag()

	restoreLogo := setDoctorLogoProviderForTest(func() string { return "" })
	defer restoreLogo()

	restoreColor := setDoctorColorDepsForTest(
		func(key string) string {
			if key == "NO_COLOR" {
				return "1"
			}
			return ""
		},
		func(writer io.Writer) bool {
			_ = writer
			return true
		},
	)
	defer restoreColor()

	output, err := executeCLI(t, "doctor")
	if err != nil {
		t.Fatalf("doctor command failed: %v", err)
	}
	if strings.Contains(output, "\x1b[") {
		t.Fatalf("expected NO_COLOR to disable ANSI colors, got: %q", output)
	}
}
