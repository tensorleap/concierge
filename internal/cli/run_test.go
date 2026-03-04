package cli

import (
	"bufio"
	"encoding/json"
	"io"
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
	if !strings.Contains(output, "Verified checks") {
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

func TestRunFailsWhenClaudeUnavailableWhenAgentStepIsNeeded(t *testing.T) {
	repo := initRunTestRepo(t, true)
	writeFile(t, filepath.Join(repo, "leap_binder.py"), "def helper():\n    return []\n")
	disableHarnessWithoutClaude(t)
	withWorkingDir(t, repo)

	_, err := executeCLI(t, "run", "--yes", "--max-iterations=1")
	if err == nil {
		t.Fatal("expected run to fail when claude is unavailable")
	}
	if got := core.KindOf(err); got != core.KindMissingDependency {
		t.Fatalf("expected missing dependency error kind, got %q (err=%v)", got, err)
	}
}

func TestRunDoesNotRequireClaudeAtStartupWhenNotNeeded(t *testing.T) {
	repo := initRunTestRepo(t, true)
	disableHarnessWithoutClaude(t)
	withWorkingDir(t, repo)

	output, err := executeCLI(t, "run", "--max-iterations=1")
	if err != nil {
		t.Fatalf("expected run to succeed when no agent-backed step is needed, got: %v\noutput=%q", err, output)
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
	if strings.Contains(output, "[y/N]:") || strings.Contains(output, "[Y/n]:") {
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
	if !strings.Contains(output, "Integration Checklist") {
		t.Fatalf("expected checklist in output, got output: %q", output)
	}
	if !strings.Contains(output, "Apply this fix now?") {
		t.Fatalf("expected pre-change approval prompt, got output: %q", output)
	}
	if !strings.Contains(output, "Current blocker: leap.yaml should be present and valid") {
		t.Fatalf("expected blocker heading in pre-change prompt, got output: %q", output)
	}
	if !strings.Contains(output, "Proposed Changes") {
		t.Fatalf("expected styled change review heading, got output: %q", output)
	}
	if !strings.Contains(output, "Fixing: leap.yaml should be present and valid") {
		t.Fatalf("expected user-facing fixing line, got output: %q", output)
	}
	if !strings.Contains(output, "Files changed:") {
		t.Fatalf("expected changed files section, got output: %q", output)
	}
	if !strings.Contains(output, "Patch:") {
		t.Fatalf("expected patch section, got output: %q", output)
	}
	if !strings.Contains(output, "Apply and commit these changes? [Y/n]:") {
		t.Fatalf("expected single final approval prompt, got output: %q", output)
	}
	if strings.Count(output, "[y/N]:") != 1 {
		t.Fatalf("expected exactly one [y/N] prompt before edits, got output: %q", output)
	}
	if strings.Count(output, "[Y/n]:") != 1 {
		t.Fatalf("expected exactly one [Y/n] prompt, got output: %q", output)
	}
	if strings.Contains(output, "Step:") {
		t.Fatalf("expected internal step label to be omitted, got output: %q", output)
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

	snapshot := core.WorkspaceSnapshot{}
	message := stepApprovalMessage(step, snapshot, true, status, true, false)
	if !strings.Contains(message, "☐ leap.yaml should be present and valid (blocking)") {
		t.Fatalf("expected blocking check row, got message: %q", message)
	}
	if strings.Contains(message, "Required secrets are configured") {
		t.Fatalf("expected unverified checks to be hidden, got message: %q", message)
	}
	if strings.Contains(message, "Model path for @tensorleap_load_model is resolved and supported") {
		t.Fatalf("expected model row to stay hidden until leap.yaml is present, got message: %q", message)
	}
	if strings.Contains(message, "Upload prerequisites are satisfied") {
		t.Fatalf("expected upload rows to stay hidden before upload checks are implemented, got message: %q", message)
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

	snapshot := core.WorkspaceSnapshot{}
	message := stepApprovalMessage(step, snapshot, true, status, true, false)
	if !strings.Contains(message, "Current blocker: leap.yaml should be present and valid") {
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

func TestModelAuthoringRecommendationRenderedInApprovalPrompt(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "model", "b.h5"), "binary")
	writeFile(t, filepath.Join(repoRoot, "model", "a.onnx"), "binary")

	step, ok := core.EnsureStepByID(core.EnsureStepModelContract)
	if !ok {
		t.Fatal("expected model ensure-step in catalog")
	}

	snapshot := core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: repoRoot},
	}
	status := core.IntegrationStatus{
		Contracts: &core.IntegrationContracts{
			ModelCandidates: []core.ModelCandidate{
				{Path: "model/b.h5"},
				{Path: "model/a.onnx"},
			},
		},
		Issues: []core.Issue{
			{
				Code:     core.IssueCodeModelCandidatesAmbiguous,
				Message:  "multiple model candidates found",
				Severity: core.SeverityError,
				Scope:    core.IssueScopeModel,
			},
		},
	}

	message := stepApprovalMessage(step, snapshot, true, status, true, false)
	if !strings.Contains(message, "Model recommendation:") {
		t.Fatalf("expected model recommendation section, got message: %q", message)
	}
	if !strings.Contains(message, "- Recommended target: model/a.onnx") {
		t.Fatalf("expected recommended target in prompt, got message: %q", message)
	}
	if !strings.Contains(message, "- Rationale: ambiguous_supported_candidates_lexical_fallback") {
		t.Fatalf("expected rationale in prompt, got message: %q", message)
	}
	if !strings.Contains(message, "- Candidates: model/a.onnx, model/b.h5") {
		t.Fatalf("expected candidate list in prompt, got message: %q", message)
	}
}

func TestPreprocessAuthoringRecommendationRenderedInApprovalPrompt(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "leap_binder.py"), "from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess\n\n@tensorleap_preprocess()\ndef preprocess_data():\n    return []\n")

	step, ok := core.EnsureStepByID(core.EnsureStepPreprocessContract)
	if !ok {
		t.Fatal("expected preprocess ensure-step in catalog")
	}

	snapshot := core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: repoRoot},
	}
	status := core.IntegrationStatus{}
	message := stepApprovalMessage(step, snapshot, true, status, true, false)
	if !strings.Contains(message, "Preprocess recommendation:") {
		t.Fatalf("expected preprocess recommendation section, got message: %q", message)
	}
	if !strings.Contains(message, "- Recommended target: preprocess_data") {
		t.Fatalf("expected recommended target in prompt, got message: %q", message)
	}
	if !strings.Contains(message, "- Target symbols: preprocess_data") {
		t.Fatalf("expected target symbol list in prompt, got message: %q", message)
	}
	if !strings.Contains(message, "Implement a preprocess function that returns both train and validation subsets.") {
		t.Fatalf("expected preprocess constraint in prompt, got message: %q", message)
	}
	if !strings.Contains(message, "non-empty output values") {
		t.Fatalf("expected non-empty subset guidance in prompt, got message: %q", message)
	}
}

func TestInputEncoderAuthoringRecommendationRenderedInApprovalPrompt(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "leap_binder.py"), strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_input_encoder, tensorleap_integration_test",
		"",
		"@tensorleap_input_encoder('image')",
		"def encode_image():",
		"    return 1",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    encode_image()",
		"    encode_meta()",
		"",
	}, "\n"))

	step, ok := core.EnsureStepByID(core.EnsureStepInputEncoders)
	if !ok {
		t.Fatal("expected input-encoder ensure-step in catalog")
	}

	snapshot := core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: repoRoot},
	}
	status := core.IntegrationStatus{}
	message := stepApprovalMessage(step, snapshot, true, status, true, false)
	if !strings.Contains(message, "Input-encoder recommendation:") {
		t.Fatalf("expected input-encoder recommendation section, got message: %q", message)
	}
	if !strings.Contains(message, "- Recommended target: meta") {
		t.Fatalf("expected recommended target in prompt, got message: %q", message)
	}
	if !strings.Contains(message, "- Missing symbols: meta") {
		t.Fatalf("expected missing-symbol list in prompt, got message: %q", message)
	}
	if !strings.Contains(message, "@tensorleap_input_encoder") {
		t.Fatalf("expected input-encoder constraint context in prompt, got message: %q", message)
	}
}

func TestGTEncoderAuthoringRecommendationRenderedInApprovalPrompt(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "leap_binder.py"), strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_gt_encoder, tensorleap_integration_test",
		"",
		"@tensorleap_gt_encoder('label')",
		"def encode_label():",
		"    return 1",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    encode_label()",
		"    encode_mask()",
		"",
	}, "\n"))

	step, ok := core.EnsureStepByID(core.EnsureStepGroundTruthEncoders)
	if !ok {
		t.Fatal("expected GT ensure-step in catalog")
	}

	snapshot := core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: repoRoot},
	}
	status := core.IntegrationStatus{}
	message := stepApprovalMessage(step, snapshot, true, status, true, false)
	if !strings.Contains(message, "Ground-truth recommendation:") {
		t.Fatalf("expected GT recommendation section, got message: %q", message)
	}
	if !strings.Contains(message, "- Recommended target: mask") {
		t.Fatalf("expected recommended target in prompt, got message: %q", message)
	}
	if !strings.Contains(message, "- Target symbols: mask") {
		t.Fatalf("expected target-symbol list in prompt, got message: %q", message)
	}
	if !strings.Contains(strings.ToLower(message), "labeled subsets only") {
		t.Fatalf("expected labeled-subset constraint in prompt, got message: %q", message)
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
	if !strings.Contains(output, "Apply this fix now?") {
		t.Fatalf("expected pre-change approval prompt, got output: %q", output)
	}
	if strings.Contains(output, "Apply and commit these changes? [Y/n]:") {
		t.Fatalf("did not expect commit approval prompt after declining pre-change prompt, got output: %q", output)
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

func TestEnsureModelPathSelectionForStepNonInteractiveFailsOnAmbiguous(t *testing.T) {
	status := core.IntegrationStatus{
		Contracts: &core.IntegrationContracts{
			ModelCandidates: []core.ModelCandidate{
				{Path: "model/a.h5"},
				{Path: "model/b.onnx"},
			},
		},
	}
	current := ""
	err := ensureModelPathSelectionForStep(
		core.EnsureStep{ID: core.EnsureStepPreprocessContract},
		status,
		true,
		func() string { return current },
		func(path string) { current = path },
		t.TempDir(),
		true,
		bufio.NewReader(strings.NewReader("")),
		io.Discard,
	)
	if err == nil {
		t.Fatal("expected model selection error in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "--model-path") {
		t.Fatalf("expected --model-path guidance, got %v", err)
	}
}

func TestEnsureModelPathSelectionForStepSelectsSingleCandidate(t *testing.T) {
	status := core.IntegrationStatus{
		Contracts: &core.IntegrationContracts{
			ModelCandidates: []core.ModelCandidate{
				{Path: "model/a.h5"},
			},
		},
	}
	current := ""
	err := ensureModelPathSelectionForStep(
		core.EnsureStep{ID: core.EnsureStepPreprocessContract},
		status,
		true,
		func() string { return current },
		func(path string) { current = path },
		t.TempDir(),
		false,
		bufio.NewReader(strings.NewReader("")),
		io.Discard,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if current != "model/a.h5" {
		t.Fatalf("expected selected model path %q, got %q", "model/a.h5", current)
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
		writeFile(t, filepath.Join(repo, "leap.yaml"), strings.Join([]string{
			"entryFile: leap_binder.py",
			"",
		}, "\n"))
		writeFile(t, filepath.Join(repo, "leap_binder.py"), strings.Join([]string{
			"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess",
			"",
			"@tensorleap_preprocess()",
			"def preprocess():",
			"    return []",
			"",
		}, "\n"))
		writeFile(t, filepath.Join(repo, "leap_custom_test.py"), "def test_noop():\n    return None\n")
		writeFile(t, filepath.Join(repo, "model", "model.h5"), "binary\n")
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

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll failed for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", path, err)
	}
}

func disableHarness(t *testing.T) {
	t.Helper()
	t.Setenv("CONCIERGE_ENABLE_HARNESS", "0")
	mockLeapCLIInstalled(t)
	mockClaudeCLIInstalled(t)
}

func disableHarnessWithoutClaude(t *testing.T) {
	t.Helper()
	t.Setenv("CONCIERGE_ENABLE_HARNESS", "0")
	binDir := mockLeapCLIInstalled(t)
	t.Setenv("PATH", strings.Join([]string{binDir, "/usr/bin", "/bin"}, string(os.PathListSeparator)))
}

func mockLeapCLIInstalled(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	leapPath := filepath.Join(binDir, "leap")
	script := `#!/usr/bin/env bash
set -euo pipefail

cmd="${1:-}"
case "$cmd" in
  --version)
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
	return binDir
}

func mockClaudeCLIInstalled(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	commandPath := filepath.Join(binDir, "claude")
	script := `#!/usr/bin/env bash
set -euo pipefail
echo "claude mock: $*"
`
	if err := os.WriteFile(commandPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock claude command: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
