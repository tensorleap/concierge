package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestRunDryRunPrintsExecutionStages(t *testing.T) {
	output, err := executeCLI(t, "run", "--dry-run")
	if err != nil {
		t.Fatalf("run --dry-run failed: %v", err)
	}

	if !strings.Contains(output, "dry-run plan:") {
		t.Fatalf("expected dry-run plan prefix in output, got: %q", output)
	}

	expected := expectedStageChainFromCore()
	if !strings.Contains(output, expected) {
		t.Fatalf("expected dry-run stages in output, got: %q", output)
	}
}

func TestRunDryRunUsesCoreDefaultStages(t *testing.T) {
	output, err := executeCLI(t, "run", "--dry-run")
	if err != nil {
		t.Fatalf("run --dry-run failed: %v", err)
	}

	expected := expectedStageChainFromCore()
	if !strings.Contains(output, expected) {
		t.Fatalf("expected output to contain core stage chain %q, got: %q", expected, output)
	}
}

func TestRunNonDryRunExecutesSingleIterationByDefault(t *testing.T) {
	disableHarness(t)
	repo := initRunTestRepo(t, true)
	withWorkingDir(t, repo)

	output, err := executeCLI(t, "run")
	if err != nil {
		t.Fatalf("expected run to succeed in complete repo, got: %v\noutput=%q", err, output)
	}
	if strings.Count(output, "snapshot=") != 1 {
		t.Fatalf("expected one reporter line, got output: %q", output)
	}
	if !strings.Contains(output, "step=ensure.complete") {
		t.Fatalf("expected complete step in output, got: %q", output)
	}
}

func TestRunNonDryRunHonorsMaxIterationsFlag(t *testing.T) {
	disableHarness(t)
	repo := initRunTestRepo(t, false)
	withWorkingDir(t, repo)

	output, err := executeCLI(t, "run", "--max-iterations=2")
	if err == nil {
		t.Fatal("expected max-iterations stop to return error")
	}
	if strings.Count(output, "snapshot=") != 2 {
		t.Fatalf("expected two reporter lines, got output: %q", output)
	}
	if !strings.Contains(err.Error(), "max_iterations") {
		t.Fatalf("expected max_iterations error, got: %v", err)
	}
}

func TestRunNonDryRunReturnsErrorOnMaxIterationsStop(t *testing.T) {
	disableHarness(t)
	repo := initRunTestRepo(t, false)
	withWorkingDir(t, repo)

	_, err := executeCLI(t, "run")
	if err == nil {
		t.Fatal("expected run to fail on max-iterations stop")
	}
	if !strings.Contains(err.Error(), "max_iterations") {
		t.Fatalf("expected max_iterations stop reason, got: %v", err)
	}
}

func TestRunWithPersistWritesConciergeArtifacts(t *testing.T) {
	disableHarness(t)
	repo := initRunTestRepo(t, true)
	withWorkingDir(t, repo)

	output, err := executeCLI(t, "run", "--persist")
	if err != nil {
		t.Fatalf("run --persist failed: %v\noutput=%q", err, output)
	}
	if !strings.Contains(output, "snapshot=") {
		t.Fatalf("expected reporter summary in output, got: %q", output)
	}

	reportFiles, err := filepath.Glob(filepath.Join(repo, ".concierge", "reports", "*.json"))
	if err != nil {
		t.Fatalf("Glob report files failed: %v", err)
	}
	if len(reportFiles) != 1 {
		t.Fatalf("expected one report file, got %d: %v", len(reportFiles), reportFiles)
	}

	rawReport, err := os.ReadFile(reportFiles[0])
	if err != nil {
		t.Fatalf("ReadFile failed for report file: %v", err)
	}
	var report core.IterationReport
	if err := json.Unmarshal(rawReport, &report); err != nil {
		t.Fatalf("Unmarshal report failed: %v", err)
	}
	if report.SnapshotID == "" {
		t.Fatal("expected snapshot ID in persisted report")
	}

	evidenceFiles, err := filepath.Glob(filepath.Join(repo, ".concierge", "evidence", "*", "executor.mode.log"))
	if err != nil {
		t.Fatalf("Glob evidence files failed: %v", err)
	}
	if len(evidenceFiles) != 1 {
		t.Fatalf("expected one evidence file, got %d: %v", len(evidenceFiles), evidenceFiles)
	}

	output, err = executeCLI(t, "run", "--persist")
	if err != nil {
		t.Fatalf("second run --persist failed: %v\noutput=%q", err, output)
	}
	reportFiles, err = filepath.Glob(filepath.Join(repo, ".concierge", "reports", "*.json"))
	if err != nil {
		t.Fatalf("Glob report files failed after second run: %v", err)
	}
	if len(reportFiles) != 1 {
		t.Fatalf("expected one report file after overwrite, got %d: %v", len(reportFiles), reportFiles)
	}
}

func expectedStageChainFromCore() string {
	stages := core.DefaultStages()
	names := make([]string, 0, len(stages))
	for _, stage := range stages {
		names = append(names, string(stage))
	}
	return strings.Join(names, " -> ")
}

func initRunTestRepo(t *testing.T, complete bool) string {
	t.Helper()

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.name", "Concierge CLI Test")
	runGit(t, repo, "config", "user.email", "concierge-cli@example.com")
	runGit(t, repo, "config", "commit.gpgsign", "false")

	writeFile(t, filepath.Join(repo, "README.md"), "test repo\n")
	writeFile(t, filepath.Join(repo, ".gitignore"), ".concierge/\n")
	if complete {
		writeFile(t, filepath.Join(repo, "leap.yaml"), "entryFile: leap_binder.py\n")
		writeFile(t, filepath.Join(repo, "leap_binder.py"), "def noop():\n    return None\n")
		writeFile(t, filepath.Join(repo, "leap_custom_test.py"), "def test_noop():\n    return None\n")
	}

	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial commit")

	return repo
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir to %q failed: %v", dir, err)
	}

	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("failed to restore cwd %q: %v", original, err)
		}
	})
}

func runGit(t *testing.T, repo string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed in %s: %v\n%s", strings.Join(args, " "), repo, err, output)
	}
	return strings.TrimSpace(string(output))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", path, err)
	}
}

func disableHarness(t *testing.T) {
	t.Helper()
	t.Setenv("CONCIERGE_ENABLE_HARNESS", "0")
}
