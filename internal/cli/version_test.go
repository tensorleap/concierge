package cli

import (
	"encoding/json"
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
		"Concierge Version",
		"Version:     dev",
		"Commit:      none",
		"Build date:  unknown",
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
		"Version:     v0.0.1",
		"Commit:      abc1234",
		"Build date:  2026-02-25T00:00:00Z",
	}

	for _, value := range expected {
		if !strings.Contains(output, value) {
			t.Fatalf("expected output to contain %q, got: %q", value, output)
		}
	}
}

func TestVersionJSONFormat(t *testing.T) {
	restoreBuildInfo(t, "v0.0.1", "abc1234", "2026-02-25T00:00:00Z")

	output, err := executeCLI(t, "version", "--format", "json")
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("expected JSON output, got error: %v\noutput=%q", err, output)
	}

	if payload["version"] != "v0.0.1" {
		t.Fatalf("expected version %q, got %q", "v0.0.1", payload["version"])
	}
	if payload["commit"] != "abc1234" {
		t.Fatalf("expected commit %q, got %q", "abc1234", payload["commit"])
	}
}

func TestVersionRejectsInvalidFormat(t *testing.T) {
	restoreBuildInfo(t, "dev", "none", "unknown")

	_, err := executeCLI(t, "version", "--format", "yaml")
	if err == nil {
		t.Fatal("expected invalid format to fail")
	}
	if !strings.Contains(err.Error(), "invalid value for --format") {
		t.Fatalf("expected invalid format message, got: %v", err)
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
