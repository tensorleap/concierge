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

	if !strings.Contains(output, "Concierge Run (Dry Run)") {
		t.Fatalf("expected dry-run title in output, got: %q", output)
	}
	if !strings.Contains(output, "Planned Workflow") {
		t.Fatalf("expected planned workflow section in output, got: %q", output)
	}
}

func TestRunDryRunUsesCoreDefaultStages(t *testing.T) {
	output, err := executeCLI(t, "run", "--dry-run")
	if err != nil {
		t.Fatalf("run --dry-run failed: %v", err)
	}

	for _, stage := range core.DefaultStages() {
		label := runStageLabel(stage)
		if !strings.Contains(output, label) {
			t.Fatalf("expected output to contain stage label %q, got: %q", label, output)
		}
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
	if strings.Count(output, "Integration Checklist") != 1 {
		t.Fatalf("expected one reporter line, got output: %q", output)
	}
	if !strings.Contains(output, "All required checks passed.") {
		t.Fatalf("expected completed checklist in output, got: %q", output)
	}
	if !strings.Contains(output, "Next steps:") {
		t.Fatalf("expected next-steps guidance in output, got: %q", output)
	}
	if !strings.Contains(output, "run `leap push` from the repository root.") {
		t.Fatalf("expected leap push guidance in output, got: %q", output)
	}
}

func TestRunNonDryRunHonorsMaxIterationsFlag(t *testing.T) {
	disableHarness(t)
	repo := initRunTestRepo(t, false)
	withWorkingDir(t, repo)

	output, err := executeCLI(t, "run", "--max-iterations=2", "--yes")
	if err == nil {
		t.Fatal("expected max-iterations stop to return error")
	}
	if strings.Count(output, "Integration Checklist") != 2 {
		t.Fatalf("expected two reporter lines, got output: %q", output)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "pending requirements") {
		t.Fatalf("expected user-facing max-iterations message, got: %v", err)
	}
}

func TestRunNonDryRunReturnsErrorOnMaxIterationsStop(t *testing.T) {
	disableHarness(t)
	repo := initRunTestRepo(t, false)
	withWorkingDir(t, repo)

	_, err := executeCLI(t, "run", "--yes")
	if err == nil {
		t.Fatal("expected run to fail on max-iterations stop")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "pending requirements") {
		t.Fatalf("expected user-facing max-iterations message, got: %v", err)
	}
}

func TestRunPromptsForProjectRootWhenAmbiguous(t *testing.T) {
	disableHarness(t)

	workspace := t.TempDir()
	initRunTestRepoAtPath(t, filepath.Join(workspace, "repo-a"), true)
	initRunTestRepoAtPath(t, filepath.Join(workspace, "repo-b"), true)
	withWorkingDir(t, workspace)

	output, err := executeCLIWithInput(t, "2\n", "run", "--max-iterations=1")
	if err != nil {
		t.Fatalf("expected run to succeed, got error: %v\noutput=%q", err, output)
	}
	if !strings.Contains(output, "Project Selection") {
		t.Fatalf("expected project root prompt, got output: %q", output)
	}
}

func TestRunNonInteractiveFailsWithoutApprovalOverride(t *testing.T) {
	disableHarness(t)
	repo := initRunTestRepo(t, false)
	withWorkingDir(t, repo)

	_, err := executeCLI(t, "run", "--non-interactive", "--max-iterations=1")
	if err == nil {
		t.Fatal("expected non-interactive run to fail without --yes")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes guidance in error, got: %v", err)
	}

	status := runGit(t, repo, "status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Fatalf("expected clean worktree after failed approval gate, got %q", status)
	}
}

func TestRunYesSkipsApprovalPrompts(t *testing.T) {
	disableHarness(t)
	repo := initRunTestRepo(t, false)
	withWorkingDir(t, repo)

	output, err := executeCLI(t, "run", "--yes", "--max-iterations=1")
	if err == nil {
		t.Fatal("expected max-iterations stop to return error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "pending requirements") {
		t.Fatalf("expected user-facing max-iterations message, got: %v", err)
	}
	if strings.Contains(output, "[y/N]:") {
		t.Fatalf("expected --yes to skip approval prompts, got output: %q", output)
	}
}

func TestRunFlowPromptsBeforeCommit(t *testing.T) {
	disableHarness(t)
	repo := initRunTestRepo(t, false)
	withWorkingDir(t, repo)

	output, err := executeCLIWithInput(t, "y\ny\n", "run", "--max-iterations=1")
	if err == nil {
		t.Fatal("expected max-iterations stop to return error")
	}
	promptOutput := output
	if promptEnd := strings.Index(output, "Review proposed changes"); promptEnd >= 0 {
		promptOutput = output[:promptEnd]
	}
	if !strings.Contains(output, "Integration checks:") {
		t.Fatalf("expected checklist before approval prompt, got output: %q", output)
	}
	if !strings.Contains(promptOutput, "☐ Check leap.yaml setup (blocking)") {
		t.Fatalf("expected blocking checklist row in approval prompt, got output: %q", output)
	}
	if strings.Contains(promptOutput, "☐ Check model compatibility") {
		t.Fatalf("expected checklist to omit future unchecked steps, got output: %q", output)
	}
	if !strings.Contains(promptOutput, "Current blocker: Check leap.yaml setup") {
		t.Fatalf("expected blocker heading in approval prompt, got output: %q", output)
	}
	if !strings.Contains(promptOutput, "Why it matters: leap.yaml defines the upload boundary and entry point that Tensorleap uses to run your integration.") {
		t.Fatalf("expected blocker explanation in approval prompt, got output: %q", output)
	}
	if !strings.Contains(promptOutput, "Docs: "+stepGuideLeapYAMLURL) {
		t.Fatalf("expected leap.yaml docs link in approval prompt, got output: %q", output)
	}
	if strings.Contains(promptOutput, "Next required check:") {
		t.Fatalf("expected prompt to avoid next-check phrasing, got output: %q", output)
	}
	if strings.Contains(promptOutput, "(No changes will be made before approval.)") {
		t.Fatalf("expected prompt to avoid redundant parenthetical approval note, got output: %q", output)
	}
	if !strings.Contains(output, "Allow Concierge to make changes for this check now?") {
		t.Fatalf("expected pre-change approval prompt, got output: %q", output)
	}
	if !strings.Contains(output, "Review proposed changes") {
		t.Fatalf("expected commit approval prompt, got output: %q", output)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "pending requirements") {
		t.Fatalf("expected user-facing max-iterations message, got: %v", err)
	}

	latestMessage := runGit(t, repo, "log", "-1", "--pretty=%s")
	if !strings.HasPrefix(latestMessage, "concierge(ensure.leap_yaml):") {
		t.Fatalf("expected structured commit message, got %q", latestMessage)
	}
}

func TestStepApprovalMessageShowsOnlyChecklistThroughBlockingStep(t *testing.T) {
	step, ok := core.EnsureStepByID(core.EnsureStepLeapYAML)
	if !ok {
		t.Fatal("expected leap.yaml ensure-step in catalog")
	}
	status := core.IntegrationStatus{
		Issues: []core.Issue{
			{
				Code:     core.IssueCodeLeapYAMLMissing,
				Message:  "leap.yaml is required at repository root",
				Severity: core.SeverityError,
			},
		},
	}

	message := stepApprovalMessage(step, status, true)
	if !strings.Contains(message, "☑ Check required secrets") {
		t.Fatalf("expected prior checks to be marked done, got message: %q", message)
	}
	if !strings.Contains(message, "☐ Check leap.yaml setup (blocking)") {
		t.Fatalf("expected blocking check row, got message: %q", message)
	}
	if strings.Contains(message, "☐ Check model compatibility") {
		t.Fatalf("expected future unchecked checks to be omitted, got message: %q", message)
	}
}

func TestStepApprovalMessageIncludesBlockerContext(t *testing.T) {
	step, ok := core.EnsureStepByID(core.EnsureStepLeapYAML)
	if !ok {
		t.Fatal("expected leap.yaml ensure-step in catalog")
	}
	status := core.IntegrationStatus{
		Issues: []core.Issue{
			{
				Code:     core.IssueCodeLeapYAMLMissing,
				Message:  "leap.yaml is required at repository root",
				Severity: core.SeverityError,
			},
		},
	}

	message := stepApprovalMessage(step, status, true)
	if !strings.Contains(message, "Current blocker: Check leap.yaml setup") {
		t.Fatalf("expected blocker heading, got message: %q", message)
	}
	if !strings.Contains(message, "What failed:\n- leap.yaml is required at repository root") {
		t.Fatalf("expected failure details, got message: %q", message)
	}
	if !strings.Contains(message, "Docs: "+stepGuideLeapYAMLURL) {
		t.Fatalf("expected docs link, got message: %q", message)
	}
	if strings.Contains(message, "Next required check:") {
		t.Fatalf("expected next-check wording to be removed, got message: %q", message)
	}
	if strings.Contains(message, "(No changes will be made before approval.)") {
		t.Fatalf("expected redundant approval note to be removed, got message: %q", message)
	}
}

func TestRunDeclineStepApprovalLeavesRepoUnchanged(t *testing.T) {
	disableHarness(t)
	repo := initRunTestRepo(t, false)
	withWorkingDir(t, repo)

	output, err := executeCLIWithInput(t, "n\n", "run", "--max-iterations=1")
	if err == nil {
		t.Fatal("expected max-iterations stop to return error")
	}
	if !strings.Contains(output, "Allow Concierge to make changes for this check now?") {
		t.Fatalf("expected pre-change approval prompt, got output: %q", output)
	}
	if strings.Contains(output, "Review proposed changes") {
		t.Fatalf("did not expect commit prompt when changes were not approved, got output: %q", output)
	}

	status := runGit(t, repo, "status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Fatalf("expected clean worktree after declining approval, got %q", status)
	}
	if _, statErr := os.Stat(filepath.Join(repo, "leap.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("expected leap.yaml to stay absent after declining approval, stat err=%v", statErr)
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
	if !strings.Contains(output, "Integration Checklist") {
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

func initRunTestRepo(t *testing.T, complete bool) string {
	t.Helper()

	repo := filepath.Join(t.TempDir(), "repo")
	initRunTestRepoAtPath(t, repo, complete)
	return repo
}

func initRunTestRepoAtPath(t *testing.T, repo string, complete bool) {
	t.Helper()

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
	runGit(t, repo, "checkout", "-B", "feature/test")
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
	mockLeapCLIInstalled(t)
}

func mockLeapCLIInstalled(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	leapPath := filepath.Join(binDir, "leap")
	script := `#!/usr/bin/env bash
set -euo pipefail

cmd="${1:-}"
case "$cmd" in
  version)
    echo "leap v0.2.0"
    ;;
  auth)
    if [[ "${2:-}" != "whoami" ]]; then
      echo "unsupported auth subcommand" >&2
      exit 1
    fi
    echo "concierge@example.com"
    ;;
  server)
    if [[ "${2:-}" != "info" ]]; then
      echo "unsupported server subcommand" >&2
      exit 1
    fi
    cat <<'EOF'
Installation information:
datasetvolumes: []
EOF
    ;;
  *)
    echo "unsupported leap command" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(leapPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock leap CLI: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
