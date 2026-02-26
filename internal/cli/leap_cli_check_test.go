package cli

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "equal", a: "v1.2.3", b: "v1.2.3", want: 0},
		{name: "older", a: "v1.2.3", b: "v1.2.4", want: -1},
		{name: "newer", a: "v1.3.0", b: "v1.2.9", want: 1},
		{name: "prerelease lower", a: "v1.2.3-rc1", b: "v1.2.3", want: -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compareSemver(tc.a, tc.b)
			if err != nil {
				t.Fatalf("compareSemver returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("compareSemver(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestExtractFirstSemver(t *testing.T) {
	version, ok := extractFirstSemver("leap-cli version: 1.2.3")
	if !ok {
		t.Fatal("expected to extract version token")
	}
	if version != "1.2.3" {
		t.Fatalf("expected extracted version %q, got %q", "1.2.3", version)
	}
}

func TestDetectLeapCLIDiagnosticsMissing(t *testing.T) {
	restore := setLeapCLITestHooks(
		func(file string) (string, error) {
			_ = file
			return "", exec.ErrNotFound
		},
		nil,
		nil,
	)
	defer restore()

	diag := detectLeapCLIDiagnostics(context.Background())
	if diag.Installed {
		t.Fatal("expected leap CLI to be reported as missing")
	}
	if diag.Status != "missing" {
		t.Fatalf("expected status %q, got %q", "missing", diag.Status)
	}
	if !strings.Contains(diag.Action, leapCLIInstallGuideURL) {
		t.Fatalf("expected missing action to include install guide URL, got: %q", diag.Action)
	}
}

func TestDetectLeapCLIDiagnosticsOutdated(t *testing.T) {
	restore := setLeapCLITestHooks(
		func(file string) (string, error) {
			_ = file
			return "/usr/local/bin/leap", nil
		},
		func(ctx context.Context, name string, args ...string) (string, error) {
			_ = ctx
			_ = name
			_ = args
			return "leap version v1.2.2", nil
		},
		func(ctx context.Context) (string, error) {
			_ = ctx
			return "v1.2.3", nil
		},
	)
	defer restore()

	diag := detectLeapCLIDiagnostics(context.Background())
	if !diag.Installed {
		t.Fatal("expected installed leap CLI")
	}
	if diag.Status != "outdated" {
		t.Fatalf("expected status %q, got %q", "outdated", diag.Status)
	}
	if diag.CurrentVersion != "v1.2.2" {
		t.Fatalf("expected current version %q, got %q", "v1.2.2", diag.CurrentVersion)
	}
	if diag.LatestVersion != "v1.2.3" {
		t.Fatalf("expected latest version %q, got %q", "v1.2.3", diag.LatestVersion)
	}
	if !strings.Contains(diag.Action, "Upgrade Leap CLI to v1.2.3") {
		t.Fatalf("expected upgrade action to mention target version, got: %q", diag.Action)
	}
	if !strings.Contains(diag.Action, leapCLIInstallGuideURL) {
		t.Fatalf("expected upgrade action to include install guide URL, got: %q", diag.Action)
	}
}

func TestDetectInstalledLeapCLIVersionFallsBackToDashVersion(t *testing.T) {
	callCount := 0
	restore := setLeapCLITestHooks(
		func(file string) (string, error) {
			_ = file
			return "/usr/local/bin/leap", nil
		},
		func(ctx context.Context, name string, args ...string) (string, error) {
			_ = ctx
			_ = name
			callCount++
			if callCount == 1 {
				return "", errors.New("version subcommand unavailable")
			}
			if len(args) == 1 && args[0] == "--version" {
				return "leap-cli v2.0.1", nil
			}
			return "", errors.New("unexpected command args")
		},
		func(ctx context.Context) (string, error) {
			_ = ctx
			return "v2.0.1", nil
		},
	)
	defer restore()

	version, err := detectInstalledLeapCLIVersion(context.Background())
	if err != nil {
		t.Fatalf("detectInstalledLeapCLIVersion returned error: %v", err)
	}
	if version != "v2.0.1" {
		t.Fatalf("expected version %q, got %q", "v2.0.1", version)
	}
}

func setLeapCLITestHooks(
	pathLookup func(file string) (string, error),
	runCommand func(ctx context.Context, name string, args ...string) (string, error),
	fetchLatest func(ctx context.Context) (string, error),
) func() {
	prevPathLookup := leapCLIPathLookup
	prevRunCommand := leapCLIRunCommand
	prevFetchLatest := leapCLIFetchLatest

	if pathLookup != nil {
		leapCLIPathLookup = pathLookup
	}
	if runCommand != nil {
		leapCLIRunCommand = runCommand
	}
	if fetchLatest != nil {
		leapCLIFetchLatest = fetchLatest
	}

	return func() {
		leapCLIPathLookup = prevPathLookup
		leapCLIRunCommand = prevRunCommand
		leapCLIFetchLatest = prevFetchLatest
	}
}
