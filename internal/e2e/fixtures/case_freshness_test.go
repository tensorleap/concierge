package fixtures

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixtureCaseFreshnessReasonReportsMissingMetadata(t *testing.T) {
	repoRoot := t.TempDir()
	writeFixtureCasePatchForTest(t, repoRoot, "fixtures/cases/patches/sample.patch", "sample patch")
	sourceRef := initFixtureSourceRepoForTest(t, filepath.Join(repoRoot, ".fixtures", "mnist", "post"))

	reason, err := fixtureCaseFreshnessReason(repoRoot, fixtureCaseEntry{
		SourceFixtureID: "mnist",
		SourceVariant:   "post",
		Patch:           "fixtures/cases/patches/sample.patch",
	}, t.TempDir())
	if err != nil {
		t.Fatalf("fixtureCaseFreshnessReason returned error: %v", err)
	}
	if !strings.Contains(reason, "missing freshness metadata") {
		t.Fatalf("expected missing-metadata reason, got %q (sourceRef=%s)", reason, sourceRef)
	}
}

func TestFixtureCaseFreshnessReasonReportsPatchHashMismatch(t *testing.T) {
	repoRoot := t.TempDir()
	patchSHA256 := writeFixtureCasePatchForTest(t, repoRoot, "fixtures/cases/patches/sample.patch", "sample patch")
	sourceRef := initFixtureSourceRepoForTest(t, filepath.Join(repoRoot, ".fixtures", "mnist", "post"))
	caseRoot := t.TempDir()
	writeFixtureCaseStateForTest(t, caseRoot, fixtureCaseState{
		SourceRef:   sourceRef,
		PatchSHA256: strings.Repeat("a", len(patchSHA256)),
	})

	reason, err := fixtureCaseFreshnessReason(repoRoot, fixtureCaseEntry{
		SourceFixtureID: "mnist",
		SourceVariant:   "post",
		Patch:           "fixtures/cases/patches/sample.patch",
	}, caseRoot)
	if err != nil {
		t.Fatalf("fixtureCaseFreshnessReason returned error: %v", err)
	}
	if !strings.Contains(reason, "patch hash mismatch") {
		t.Fatalf("expected patch-mismatch reason, got %q", reason)
	}
}

func TestFixtureCaseFreshnessReasonReportsSourceRefMismatch(t *testing.T) {
	repoRoot := t.TempDir()
	patchSHA256 := writeFixtureCasePatchForTest(t, repoRoot, "fixtures/cases/patches/sample.patch", "sample patch")
	initFixtureSourceRepoForTest(t, filepath.Join(repoRoot, ".fixtures", "mnist", "post"))
	caseRoot := t.TempDir()
	writeFixtureCaseStateForTest(t, caseRoot, fixtureCaseState{
		SourceRef:   strings.Repeat("b", 40),
		PatchSHA256: patchSHA256,
	})

	reason, err := fixtureCaseFreshnessReason(repoRoot, fixtureCaseEntry{
		SourceFixtureID: "mnist",
		SourceVariant:   "post",
		Patch:           "fixtures/cases/patches/sample.patch",
	}, caseRoot)
	if err != nil {
		t.Fatalf("fixtureCaseFreshnessReason returned error: %v", err)
	}
	if !strings.Contains(reason, "source fixture ref changed") {
		t.Fatalf("expected source-ref-mismatch reason, got %q", reason)
	}
}

func TestFixtureCaseFreshnessReasonAcceptsMatchingMetadata(t *testing.T) {
	repoRoot := t.TempDir()
	patchSHA256 := writeFixtureCasePatchForTest(t, repoRoot, "fixtures/cases/patches/sample.patch", "sample patch")
	sourceRef := initFixtureSourceRepoForTest(t, filepath.Join(repoRoot, ".fixtures", "mnist", "post"))
	caseRoot := t.TempDir()
	writeFixtureCaseStateForTest(t, caseRoot, fixtureCaseState{
		SourceRef:   sourceRef,
		PatchSHA256: patchSHA256,
	})

	reason, err := fixtureCaseFreshnessReason(repoRoot, fixtureCaseEntry{
		SourceFixtureID: "mnist",
		SourceVariant:   "post",
		Patch:           "fixtures/cases/patches/sample.patch",
	}, caseRoot)
	if err != nil {
		t.Fatalf("fixtureCaseFreshnessReason returned error: %v", err)
	}
	if reason != "" {
		t.Fatalf("expected fresh case with empty reason, got %q", reason)
	}
}

func writeFixtureCasePatchForTest(t *testing.T, repoRoot, relPath, content string) string {
	t.Helper()
	patchPath := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(patchPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(patchPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	sum, err := fileSHA256(patchPath)
	if err != nil {
		t.Fatalf("fileSHA256 failed: %v", err)
	}
	return sum
}

func initFixtureSourceRepoForTest(t *testing.T, repoRoot string) string {
	t.Helper()
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("fixture source"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	runFixtureGitCommand(t, repoRoot, "init")
	runFixtureGitCommand(t, repoRoot, "config", "user.name", "Fixture Test")
	runFixtureGitCommand(t, repoRoot, "config", "user.email", "fixture@example.com")
	runFixtureGitCommand(t, repoRoot, "add", "README.md")
	runFixtureGitCommand(t, repoRoot, "commit", "-m", "init")

	output := runFixtureGitCommand(t, repoRoot, "rev-parse", "HEAD")
	return strings.TrimSpace(output)
}

func writeFixtureCaseStateForTest(t *testing.T, caseRoot string, state fixtureCaseState) {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(caseRoot, fixtureCaseStateFile), raw, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}

func runFixtureGitCommand(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
