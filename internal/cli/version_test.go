package cli

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/buildinfo"
)

func TestVersionDefaults(t *testing.T) {
	restoreBuildInfo(t, "dev", "none", "unknown")

	output, err := executeCLI(t, "version")
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	expected := []string{
		"version: dev",
		"commit: none",
		"date: unknown",
	}

	for _, value := range expected {
		if !strings.Contains(output, value) {
			t.Fatalf("expected output to contain %q, got: %q", value, output)
		}
	}
}

func TestVersionUsesInjectedBuildMetadata(t *testing.T) {
	restoreBuildInfo(t, "v0.0.1", "abc1234", "2026-02-25T00:00:00Z")

	output, err := executeCLI(t, "version")
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	expected := []string{
		"version: v0.0.1",
		"commit: abc1234",
		"date: 2026-02-25T00:00:00Z",
	}

	for _, value := range expected {
		if !strings.Contains(output, value) {
			t.Fatalf("expected output to contain %q, got: %q", value, output)
		}
	}
}

func restoreBuildInfo(t *testing.T, version, commit, date string) {
	t.Helper()

	originalVersion := buildinfo.Version
	originalCommit := buildinfo.Commit
	originalDate := buildinfo.Date

	buildinfo.Version = version
	buildinfo.Commit = commit
	buildinfo.Date = date

	t.Cleanup(func() {
		buildinfo.Version = originalVersion
		buildinfo.Commit = originalCommit
		buildinfo.Date = originalDate
	})
}
