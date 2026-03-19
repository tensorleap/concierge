package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/state"
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
	if !strings.Contains(output, "Verified checks") &&
		!strings.Contains(output, "Warning: Leap CLI should be installed and authenticated") {
		t.Fatalf("expected completed checklist or advisory leap-cli warning in output, got: %q", output)
	}
	if strings.Contains(output, "Warning: Leap CLI should be installed and authenticated") {
		if !strings.Contains(output, "Next step: run `leap --version`;") {
			t.Fatalf("expected leap-cli warning next-step guidance in output, got: %q", output)
		}
	} else {
		if !strings.Contains(output, "Next steps:") {
			t.Fatalf("expected next-steps guidance in output, got: %q", output)
		}
		if !strings.Contains(output, "run `leap push` from the repository root.") {
			t.Fatalf("expected leap push guidance in output, got: %q", output)
		}
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

	_, err := executeCLI(t, "run", "--yes", "--max-iterations=3")
	if err == nil {
		t.Fatal("expected run to fail on max-iterations stop")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "pending requirements") {
		t.Fatalf("expected user-facing max-iterations message, got: %v", err)
	}
}

func TestRunMaxIterationsDefaultsToUnlimited(t *testing.T) {
	cmd := newRunCommand()
	flag := cmd.Flags().Lookup("max-iterations")
	if flag == nil {
		t.Fatal("expected max-iterations flag to be registered")
	}
	if flag.DefValue != "0" {
		t.Fatalf("expected max-iterations default to be 0 (unlimited), got %q", flag.DefValue)
	}
}

func TestRunFailsWhenClaudeUnavailableWhenAgentStepIsNeeded(t *testing.T) {
	repo := initRunTestRepo(t, true)
	writeFile(t, filepath.Join(repo, "leap_integration.py"), "def helper():\n    return []\n")
	disableHarnessWithoutClaude(t)
	withWorkingDir(t, repo)

	_, err := executeCLI(t, "run", "--yes", "--max-iterations=1")
	if err == nil {
		t.Fatal("expected run to fail when claude is unavailable")
	}
	if got := core.KindOf(err); got != core.KindMissingDependency &&
		!strings.Contains(strings.ToLower(err.Error()), "pending requirements") {
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

	output, err := executeCLIWithInput(t, "y\ny\ny\n", "run", "--max-iterations=1")
	if err == nil {
		t.Fatal("expected max-iterations stop to return error")
	}
	if !strings.Contains(output, "Integration Checklist") {
		t.Fatalf("expected checklist in output, got output: %q", output)
	}
	if !strings.Contains(output, "You > Continue now? [y/N]:") {
		t.Fatalf("expected pre-change approval prompt, got output: %q", output)
	}
	if !strings.Contains(output, "Missing integration step: leap.yaml should be present and valid") {
		t.Fatalf("expected missing-step heading in pre-change prompt, got output: %q", output)
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
	if !strings.Contains(output, "Keep these changes in your working tree for local review? [Y/n]:") {
		t.Fatalf("expected keep-changes prompt, got output: %q", output)
	}
	if !strings.Contains(output, "Create a commit for these reviewed changes now? [y/N]:") {
		t.Fatalf("expected explicit commit prompt, got output: %q", output)
	}
	if strings.Count(output, "Continue now? [y/N]:") != 1 {
		t.Fatalf("expected exactly one pre-change approval prompt, got output: %q", output)
	}
	if strings.Count(output, "Keep these changes in your working tree for local review? [Y/n]:") != 1 {
		t.Fatalf("expected exactly one keep-changes prompt, got output: %q", output)
	}
	if strings.Count(output, "Create a commit for these reviewed changes now? [y/N]:") != 1 {
		t.Fatalf("expected exactly one explicit commit prompt, got output: %q", output)
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

func TestRunDeclineCommitApprovalKeepsChangesForLocalReview(t *testing.T) {
	disableHarness(t)
	repo := initRunTestRepo(t, false)
	withWorkingDir(t, repo)

	output, err := executeCLIWithInput(t, "y\ny\nn\n", "run", "--max-iterations=1")
	if err != nil {
		t.Fatalf("expected clean handoff after declining commit, got err=%v output=%q", err, output)
	}
	if !strings.Contains(output, "Keep these changes in your working tree for local review? [Y/n]:") {
		t.Fatalf("expected keep-changes prompt, got output: %q", output)
	}
	if !strings.Contains(output, "Create a commit for these reviewed changes now? [y/N]:") {
		t.Fatalf("expected explicit commit prompt, got output: %q", output)
	}
	if !strings.Contains(output, "Changes are in your working tree for local review. After reviewing or committing them, rerun `concierge run`.") {
		t.Fatalf("expected local-review handoff, got output: %q", output)
	}

	status := runGit(t, repo, "status", "--porcelain")
	if !strings.Contains(status, "leap.yaml") || !strings.Contains(status, "leap_integration.py") {
		t.Fatalf("expected generated files to remain for review, got %q", status)
	}
	latestMessage := runGit(t, repo, "log", "-1", "--pretty=%s")
	if latestMessage != "initial commit" {
		t.Fatalf("expected no new commit after declining commit prompt, got %q", latestMessage)
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
	if !strings.Contains(message, "I'm checking the Tensorleap integration's progress:") {
		t.Fatalf("expected progress heading in prompt, got message: %q", message)
	}
	if !strings.Contains(message, "☐ leap.yaml should be present and valid (missing step)") {
		t.Fatalf("expected missing-step check row, got message: %q", message)
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

func TestStepApprovalMessageUsesMissingStepContext(t *testing.T) {
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
	if !strings.Contains(message, "Missing integration step: leap.yaml should be present and valid") {
		t.Fatalf("expected missing-step heading, got message: %q", message)
	}
	if !strings.Contains(message, "What I'm seeing:\n- leap.yaml is required at repository root") {
		t.Fatalf("expected missing details, got message: %q", message)
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
	if !strings.Contains(message, "I can continue by addressing this missing step now.") {
		t.Fatalf("expected journey-style continuation prompt, got message: %q", message)
	}
}

func TestStepApprovalPromptDoesNotExposeModelRecommendationDetails(t *testing.T) {
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
	if strings.Contains(strings.ToLower(message), "recommendation:") {
		t.Fatalf("did not expect recommendation details in prompt, got message: %q", message)
	}
	if strings.Contains(strings.ToLower(message), "ambiguous_supported_candidates_lexical_fallback") {
		t.Fatalf("did not expect internal recommendation rationale in prompt, got message: %q", message)
	}
}

func TestStepApprovalPromptUsesUserFacingPreprocessLanguage(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "leap_integration.py"), "from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess\n\n@tensorleap_preprocess()\ndef preprocess_data():\n    return []\n")

	step, ok := core.EnsureStepByID(core.EnsureStepPreprocessContract)
	if !ok {
		t.Fatal("expected preprocess ensure-step in catalog")
	}

	snapshot := core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: repoRoot},
	}
	status := core.IntegrationStatus{}
	message := stepApprovalMessage(step, snapshot, true, status, true, false)
	if strings.Contains(strings.ToLower(message), "preprocess recommendation:") {
		t.Fatalf("did not expect preprocess recommendation section, got message: %q", message)
	}
	if strings.Contains(strings.ToLower(message), "target symbols:") {
		t.Fatalf("did not expect internal symbol targeting details in prompt, got message: %q", message)
	}
	if !strings.Contains(message, "Current step: Dataset preprocessing is configured") {
		t.Fatalf("expected user-facing preprocess copy in prompt, got message: %q", message)
	}
	if !strings.Contains(message, "Why this step matters: Tensorleap needs preprocessing that produces both train and validation subsets so integration checks can run end-to-end.") {
		t.Fatalf("expected user-facing preprocess explanation in prompt, got message: %q", message)
	}
}

func TestStepApprovalPromptDoesNotExposeInputEncoderRecommendationDetails(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "leap_integration.py"), strings.Join([]string{
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
	if strings.Contains(strings.ToLower(message), "input-encoder recommendation:") {
		t.Fatalf("did not expect input-encoder recommendation section, got message: %q", message)
	}
	if strings.Contains(strings.ToLower(message), "missing symbols:") {
		t.Fatalf("did not expect internal missing-symbol details in prompt, got message: %q", message)
	}
}

func TestStepApprovalPromptDoesNotExposeGTRecommendationDetails(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "leap_integration.py"), strings.Join([]string{
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
	if strings.Contains(strings.ToLower(message), "ground-truth recommendation:") {
		t.Fatalf("did not expect GT recommendation section, got message: %q", message)
	}
	if strings.Contains(strings.ToLower(message), "target symbols:") {
		t.Fatalf("did not expect internal GT target-symbol details in prompt, got message: %q", message)
	}
}

func TestStepApprovalPromptUsesThinIntegrationTestLanguage(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "leap_integration.py"), strings.Join([]string{
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess_response):",
		"    return None",
	}, "\n"))

	step, ok := core.EnsureStepByID(core.EnsureStepIntegrationTestContract)
	if !ok {
		t.Fatal("expected integration-test ensure-step in catalog")
	}

	snapshot := core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: repoRoot},
	}
	status := core.IntegrationStatus{}
	message := stepApprovalMessage(step, snapshot, true, status, true, false)
	if !strings.Contains(message, "Current step: Integration test wiring is complete") {
		t.Fatalf("expected integration-test heading, got message: %q", message)
	}
	if !strings.Contains(message, "must stay thin and only wire decorated calls plus model inference") {
		t.Fatalf("expected thin-integration-test explanation, got message: %q", message)
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
	if !strings.Contains(output, "You > Continue now? [y/N]:") {
		t.Fatalf("expected pre-change approval prompt, got output: %q", output)
	}
	if strings.Contains(output, "Keep these changes in your working tree for local review? [Y/n]:") {
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

func TestRunStopsAfterManualRuntimeSetupIsRequired(t *testing.T) {
	t.Setenv("CONCIERGE_ENABLE_HARNESS", "0")
	mockLeapCLIInstalled(t)
	mockPoetryEnvInfoFailure(t)

	repo := initRunTestRepo(t, true)
	if err := os.RemoveAll(filepath.Join(repo, ".concierge")); err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}
	withWorkingDir(t, repo)

	output, err := executeCLI(t, "run", "--max-iterations=3", "--no-color")
	if err != nil {
		t.Fatalf("expected run to return success for manual runtime setup handoff, got: %v\noutput=%q", err, output)
	}
	if strings.Count(output, "Integration Checklist") != 1 {
		t.Fatalf("expected one checklist before stopping, got output: %q", output)
	}
	if strings.Contains(output, "You > Continue now? [y/N]:") {
		t.Fatalf("did not expect approval prompt for manual-only runtime blocker, got output: %q", output)
	}
	if !strings.Contains(output, "Poetry environment should be available and have the required packages") {
		t.Fatalf("expected user-facing runtime label, got output: %q", output)
	}
	if !strings.Contains(output, "Concierge could not find a working Poetry environment for this project.") {
		t.Fatalf("expected explicit missing-environment message, got output: %q", output)
	}
	if !strings.Contains(output, "Next step: run `poetry install` in this project.") {
		t.Fatalf("expected concrete self-service guidance, got output: %q", output)
	}
	if !strings.Contains(output, "If `poetry env info --executable` still does not print a Python path, run `poetry env use <python>`, then rerun `concierge run`.") {
		t.Fatalf("expected explicit Poetry fallback guidance, got output: %q", output)
	}
	if !strings.Contains(output, "You do not need to start Concierge with `poetry run`; Concierge will use the Poetry environment automatically.") {
		t.Fatalf("expected explicit guidance about running Concierge directly, got output: %q", output)
	}
	if !strings.Contains(output, "Manual step required outside Concierge. After completing the step above, rerun `concierge run`.") {
		t.Fatalf("expected explicit non-error handoff message, got output: %q", output)
	}
}

func TestRunRepairsDeclaredCodeLoaderMissingWithoutCommitPrompt(t *testing.T) {
	t.Setenv("CONCIERGE_ENABLE_HARNESS", "0")
	mockPoetryDeclaredCodeLoaderMissingUntilInstall(t)

	repo := initRunTestRepo(t, true)
	withWorkingDir(t, repo)

	output, err := executeCLIWithInput(t, "y\n", "run", "--max-iterations=2", "--no-color")
	if err != nil {
		t.Fatalf("expected runtime repair flow to complete, got: %v\noutput=%q", err, output)
	}
	if !strings.Contains(output, "You > Continue now? [y/N]:") {
		t.Fatalf("expected runtime repair approval prompt, got output: %q", output)
	}
	if strings.Contains(output, "Keep these changes in your working tree for local review? [Y/n]:") {
		t.Fatalf("did not expect commit approval prompt for environment-only runtime repair, got output: %q", output)
	}

	status := runGit(t, repo, "status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Fatalf("expected clean worktree after runtime repair, got %q", status)
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

func TestEnsureModelPathSelectionForStepDoesNotPromptOnAmbiguousCandidates(t *testing.T) {
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
		core.EnsureStep{ID: core.EnsureStepModelContract},
		core.WorkspaceSnapshot{},
		false,
		status,
		true,
		func() string { return current },
		func(path string) { current = path },
		func() *state.ModelAcquisitionClarification { return nil },
		func(*state.ModelAcquisitionClarification) {},
		t.TempDir(),
		true,
		bufio.NewReader(strings.NewReader("")),
		io.Discard,
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if current != "" {
		t.Fatalf("expected no automatic model-path selection, got %q", current)
	}
}

func TestEnsureModelPathSelectionForStepPreservesExistingSelection(t *testing.T) {
	status := core.IntegrationStatus{}
	current := "model/a.h5"
	err := ensureModelPathSelectionForStep(
		core.EnsureStep{ID: core.EnsureStepModelAcquisition},
		core.WorkspaceSnapshot{},
		false,
		status,
		true,
		func() string { return current },
		func(path string) { current = path },
		func() *state.ModelAcquisitionClarification { return nil },
		func(*state.ModelAcquisitionClarification) {},
		t.TempDir(),
		false,
		bufio.NewReader(strings.NewReader("")),
		io.Discard,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if current != "model/a.h5" {
		t.Fatalf("expected selected model path to stay %q, got %q", "model/a.h5", current)
	}
}

func TestEnsureModelPathSelectionForStepPromptsForAmbiguousVerifiedModelSources(t *testing.T) {
	snapshot := core.WorkspaceSnapshot{
		Repository:          core.RepositoryState{Head: "head-1"},
		WorktreeFingerprint: "fp-1",
		RuntimeProfile: &core.LocalRuntimeProfile{
			Fingerprint: core.RuntimeProfileFingerprint{
				ProjectRoot:     "/repo",
				PyProjectHash:   "pyproject-hash",
				InterpreterPath: "/tmp/python",
				PythonVersion:   "Python 3.11.8",
			},
		},
	}
	status := core.IntegrationStatus{
		Contracts: &core.IntegrationContracts{
			ModelCandidates: []core.ModelCandidate{
				{Path: "model/a.h5", Exists: true, VerificationState: core.ModelCandidateVerificationStateVerified},
				{Path: "model/b.onnx", Exists: true, VerificationState: core.ModelCandidateVerificationStateVerified},
			},
		},
		Issues: []core.Issue{{
			Code:     core.IssueCodeModelCandidatesAmbiguous,
			Message:  "multiple supported model artifacts were verified",
			Severity: core.SeverityError,
			Scope:    core.IssueScopeModel,
		}},
	}

	current := ""
	var clarification *state.ModelAcquisitionClarification
	output := new(strings.Builder)
	err := ensureModelPathSelectionForStep(
		core.EnsureStep{ID: core.EnsureStepModelAcquisition},
		snapshot,
		true,
		status,
		true,
		func() string { return current },
		func(path string) { current = path },
		func() *state.ModelAcquisitionClarification { return clarification },
		func(next *state.ModelAcquisitionClarification) { clarification = next },
		t.TempDir(),
		false,
		bufio.NewReader(strings.NewReader("2\n")),
		output,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if current != "model/b.onnx" {
		t.Fatalf("expected clarified selected model path %q, got %q", "model/b.onnx", current)
	}
	if clarification == nil {
		t.Fatal("expected clarification to be recorded")
	}
	if clarification.SelectedVerifiedModelPath != "model/b.onnx" {
		t.Fatalf("expected selected verified model path %q, got %+v", "model/b.onnx", clarification)
	}
}

func TestEnsureModelPathSelectionForStepPromptsForUnverifiedModelSourceClarification(t *testing.T) {
	snapshot := core.WorkspaceSnapshot{
		Repository:          core.RepositoryState{Head: "head-1"},
		WorktreeFingerprint: "fp-1",
		RuntimeProfile: &core.LocalRuntimeProfile{
			Fingerprint: core.RuntimeProfileFingerprint{
				ProjectRoot:     "/repo",
				PyProjectHash:   "pyproject-hash",
				InterpreterPath: "/tmp/python",
				PythonVersion:   "Python 3.11.8",
			},
		},
	}
	status := core.IntegrationStatus{
		Contracts: &core.IntegrationContracts{
			ModelCandidates: []core.ModelCandidate{
				{
					Path:              "model/demo.onnx",
					Exists:            true,
					VerificationState: core.ModelCandidateVerificationStateFailed,
					VerificationError: "failed to load in runtime",
				},
			},
			ModelAcquisition: &core.ModelAcquisitionArtifacts{
				AcquisitionLeads: []string{"tools/export_model.py"},
			},
		},
		Issues: []core.Issue{{
			Code:     core.IssueCodeModelAcquisitionUnresolved,
			Message:  "supported model artifacts were found but could not be loaded",
			Severity: core.SeverityError,
			Scope:    core.IssueScopeModel,
		}},
	}

	current := ""
	var clarification *state.ModelAcquisitionClarification
	err := ensureModelPathSelectionForStep(
		core.EnsureStep{ID: core.EnsureStepModelAcquisition},
		snapshot,
		true,
		status,
		true,
		func() string { return current },
		func(path string) { current = path },
		func() *state.ModelAcquisitionClarification { return clarification },
		func(next *state.ModelAcquisitionClarification) { clarification = next },
		t.TempDir(),
		false,
		bufio.NewReader(strings.NewReader("export from weights/best.pt\nn\n")),
		io.Discard,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clarification == nil {
		t.Fatal("expected clarification to be recorded")
	}
	if clarification.ModelSourceNote != "export from weights/best.pt" {
		t.Fatalf("expected model source note to be recorded, got %+v", clarification)
	}
	if clarification.RuntimeChangePolicy != state.ModelRuntimeChangePolicyStayInCurrentRuntime {
		t.Fatalf("expected runtime policy to stay in current runtime, got %+v", clarification)
	}
}

func TestEnsureModelPathSelectionForStepRequiresInteractiveClarificationForAmbiguousModelSources(t *testing.T) {
	status := core.IntegrationStatus{
		Contracts: &core.IntegrationContracts{
			ModelCandidates: []core.ModelCandidate{
				{Path: "model/a.h5", Exists: true, VerificationState: core.ModelCandidateVerificationStateVerified},
				{Path: "model/b.onnx", Exists: true, VerificationState: core.ModelCandidateVerificationStateVerified},
			},
		},
		Issues: []core.Issue{{
			Code:     core.IssueCodeModelCandidatesAmbiguous,
			Message:  "multiple supported model artifacts were verified",
			Severity: core.SeverityError,
			Scope:    core.IssueScopeModel,
		}},
	}

	err := ensureModelPathSelectionForStep(
		core.EnsureStep{ID: core.EnsureStepModelAcquisition},
		core.WorkspaceSnapshot{},
		false,
		status,
		true,
		func() string { return "" },
		func(string) {},
		func() *state.ModelAcquisitionClarification { return nil },
		func(*state.ModelAcquisitionClarification) {},
		t.TempDir(),
		true,
		bufio.NewReader(strings.NewReader("")),
		io.Discard,
	)
	if err == nil {
		t.Fatal("expected clarification-required error in non-interactive mode")
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
	writeFile(t, filepath.Join(repo, "pyproject.toml"), strings.Join([]string{
		"[tool.poetry]",
		"name = \"concierge-test\"",
		"version = \"0.1.0\"",
		"description = \"\"",
		"authors = [\"Concierge Test <concierge@example.com>\"]",
		"",
		"[tool.poetry.dependencies]",
		"python = \">=3.10,<3.13\"",
		"code-loader = \"^1.0\"",
		"",
		"[build-system]",
		"requires = [\"poetry-core\"]",
		"build-backend = \"poetry.core.masonry.api\"",
		"",
	}, "\n"))
	if complete {
		writeFile(t, filepath.Join(repo, "leap.yaml"), strings.Join([]string{
			"entryFile: leap_integration.py",
			"",
		}, "\n"))
		writeFile(t, filepath.Join(repo, "leap_integration.py"), strings.Join([]string{
			"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_integration_test, tensorleap_load_model, tensorleap_preprocess",
			"",
			"@tensorleap_preprocess()",
			"def preprocess():",
			"    return []",
			"",
			"@tensorleap_load_model()",
			"def load_model():",
			"    return None",
			"",
			"@tensorleap_integration_test()",
			"def integration_test(sample_id, preprocess_response):",
			"    load_model()",
			"    return None",
			"",
		}, "\n"))
		writeFile(t, filepath.Join(repo, "model", "model.h5"), "binary\n")
	}

	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial commit")
	runGit(t, repo, "checkout", "-B", "feature/test")

	pyprojectHash := hashFileForTest(t, filepath.Join(repo, "pyproject.toml"))
	interpreterPath := mockRuntimeInterpreterPath()
	runtimeState := state.DefaultRunState(repo)
	runtimeState.RuntimeProfile = &core.LocalRuntimeProfile{
		Kind:              "poetry",
		PoetryExecutable:  "/usr/local/bin/poetry",
		PoetryVersion:     "Poetry 2.0.0",
		InterpreterPath:   interpreterPath,
		PythonVersion:     "Python 3.11.8",
		ConfirmationMode:  "auto",
		DependenciesReady: true,
		CodeLoaderReady:   true,
		Fingerprint: core.RuntimeProfileFingerprint{
			ProjectRoot:     repo,
			PyProjectHash:   pyprojectHash,
			PoetryLockHash:  "",
			InterpreterPath: interpreterPath,
			PythonVersion:   "Python 3.11.8",
		},
	}
	if err := state.SaveState(repo, runtimeState); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}
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

func mockRuntimeInterpreterPath() string {
	if path := strings.TrimSpace(os.Getenv("CONCIERGE_TEST_MOCK_PYTHON_PATH")); path != "" {
		return path
	}
	return "/tmp/concierge-test/.venv/bin/python"
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

func hashFileForTest(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed for %q: %v", path, err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
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
	pathEntries := []string{binDir}
	for _, command := range []string{"git", "bash", "python3"} {
		resolved, err := exec.LookPath(command)
		if err != nil {
			continue
		}
		dir := filepath.Dir(resolved)
		if contains(pathEntries, dir) {
			continue
		}
		pathEntries = append(pathEntries, dir)
	}
	t.Setenv("PATH", strings.Join(pathEntries, string(os.PathListSeparator)))
}

func mockLeapCLIInstalled(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	leapPath := filepath.Join(binDir, "leap")
	poetryPath := filepath.Join(binDir, "poetry")
	mockPythonPath := filepath.Join(binDir, "poetry-python")
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

	mockPythonScript := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

self_path=%q
cmd="${1:-}"
case "$cmd" in
  --version)
    echo "Python 3.11.8"
    ;;
  -c)
    code="${2:-}"
    if [[ "$code" == "import code_loader" ]]; then
      exit 0
    fi
    if [[ "$code" == "import sys; print(sys.executable)" ]]; then
      echo "$self_path"
      exit 0
    fi
    if [[ "$code" == *"probeSucceeded"* && "$code" == *"supportsGuideLocalStatusTable"* ]]; then
      echo '{"probeSucceeded":true,"version":"1.0.165","supportsGuideLocalStatusTable":true,"supportsCheckDataset":true}'
      exit 0
    fi
    if [[ "$code" == *"LeapLoader"* && "$code" == *"check_dataset"* ]]; then
      echo '{"available":true,"isValid":true,"isValidForModel":true,"generalError":"","printLog":"","payloads":[],"setup":{"preprocess":{"trainingLength":1,"validationLength":1,"testLength":0,"unlabeledLength":0,"additionalLength":0},"inputs":[],"metadata":[],"outputs":[],"visualizers":[],"predictionTypes":[],"customLosses":[],"metrics":[]}}'
      exit 0
    fi
    if [[ "$code" == *"import onnx"* || "$code" == *"from tensorflow import keras"* ]]; then
      echo '{"inputs":[]}'
      exit 0
    fi
    python3 "$@"
    ;;
  leap_integration.py|*/leap_integration.py)
    cat <<'EOF'
tensorleap_preprocess        | ✅
tensorleap_input_encoder     | ✅
tensorleap_load_model        | ✅
tensorleap_integration_test  | ✅
tensorleap_gt_encoder        | ✅
All parts have been successfully set.
Successful!
EOF
    ;;
  *)
    python3 "$@"
    ;;
esac
`, mockPythonPath)
	if err := os.WriteFile(mockPythonPath, []byte(mockPythonScript), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock poetry runtime python: %v", err)
	}

	poetryScript := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

mock_python=%q
cmd="${1:-}"
case "$cmd" in
  --version)
    echo "Poetry 2.0.0"
    ;;
  env)
    if [[ "${2:-}" != "info" || "${3:-}" != "--executable" ]]; then
      echo "unsupported poetry env command" >&2
      exit 1
    fi
    echo "$mock_python"
    ;;
  check)
    echo "All set!"
    ;;
  install)
    echo "Installing dependencies"
    ;;
  add)
    echo "Adding dependency ${2:-}"
    ;;
  run)
    if [[ "${2:-}" != "python" ]]; then
      echo "unsupported poetry run command" >&2
      exit 1
    fi
    "$mock_python" "${@:3}"
    ;;
  *)
    echo "unsupported poetry command" >&2
    exit 1
    ;;
esac
`, mockPythonPath)
	if err := os.WriteFile(poetryPath, []byte(poetryScript), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock poetry CLI: %v", err)
	}

	t.Setenv("CONCIERGE_TEST_MOCK_PYTHON_PATH", mockPythonPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return binDir
}

func mockPoetryEnvInfoFailure(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	poetryPath := filepath.Join(binDir, "poetry")
	poetryScript := `#!/usr/bin/env bash
set -euo pipefail

cmd="${1:-}"
case "$cmd" in
  --version)
    echo "Poetry 2.0.0"
    ;;
  env)
    if [[ "${2:-}" != "info" || "${3:-}" != "--executable" ]]; then
      echo "unsupported poetry env command" >&2
      exit 1
    fi
    exit 1
    ;;
  *)
    echo "unsupported poetry command" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(poetryPath, []byte(poetryScript), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock poetry CLI: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return binDir
}

func mockPoetryDeclaredCodeLoaderMissingUntilInstall(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	installMarker := filepath.Join(t.TempDir(), "code_loader_installed")
	leapPath := filepath.Join(binDir, "leap")
	poetryPath := filepath.Join(binDir, "poetry")
	mockPythonPath := filepath.Join(binDir, "mock-python")

	leapScript := `#!/usr/bin/env bash
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
	if err := os.WriteFile(leapPath, []byte(leapScript), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock leap CLI: %v", err)
	}

	mockPythonScript := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

self_path=%q
install_marker=%q
cmd="${1:-}"
case "$cmd" in
  --version)
    echo "Python 3.11.8"
    ;;
  -c)
    code="${2:-}"
    if [[ "$code" == "import code_loader" ]]; then
      if [[ -f "$install_marker" ]]; then
        exit 0
      fi
      echo "ModuleNotFoundError: No module named 'code_loader'" >&2
      exit 1
    fi
    if [[ "$code" == "import sys; print(sys.executable)" ]]; then
      echo "$self_path"
      exit 0
    fi
    if [[ "$code" == *"probeSucceeded"* && "$code" == *"supportsGuideLocalStatusTable"* ]]; then
      echo '{"probeSucceeded":true,"version":"1.0.165","supportsGuideLocalStatusTable":true,"supportsCheckDataset":true}'
      exit 0
    fi
    if [[ "$code" == *"LeapLoader"* && "$code" == *"check_dataset"* ]]; then
      echo '{"available":true,"isValid":true,"isValidForModel":true,"generalError":"","printLog":"","payloads":[],"setup":{"preprocess":{"trainingLength":1,"validationLength":1,"testLength":0,"unlabeledLength":0,"additionalLength":0},"inputs":[],"metadata":[],"outputs":[],"visualizers":[],"predictionTypes":[],"customLosses":[],"metrics":[]}}'
      exit 0
    fi
    if [[ "$code" == *"import onnx"* || "$code" == *"from tensorflow import keras"* ]]; then
      echo '{"inputs":[]}'
      exit 0
    fi
    python3 "$@"
    ;;
  leap_integration.py|*/leap_integration.py)
    cat <<'EOF'
tensorleap_preprocess        | ✅
tensorleap_input_encoder     | ✅
tensorleap_load_model        | ✅
tensorleap_integration_test  | ✅
tensorleap_gt_encoder        | ✅
All parts have been successfully set.
Successful!
EOF
    ;;
  *)
    python3 "$@"
    ;;
esac
`, mockPythonPath, installMarker)
	if err := os.WriteFile(mockPythonPath, []byte(mockPythonScript), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock poetry runtime python: %v", err)
	}

	poetryScript := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

mock_python=%q
install_marker=%q
cmd="${1:-}"
case "$cmd" in
  --version)
    echo "Poetry 2.0.0"
    ;;
  env)
    if [[ "${2:-}" != "info" || "${3:-}" != "--executable" ]]; then
      echo "unsupported poetry env command" >&2
      exit 1
    fi
    echo "$mock_python"
    ;;
  check)
    echo "All set!"
    ;;
  install)
    touch "$install_marker"
    echo "Installing dependencies"
    ;;
  add)
    echo "unexpected poetry add ${2:-}" >&2
    exit 99
    ;;
  run)
    if [[ "${2:-}" != "python" ]]; then
      echo "unsupported poetry run command" >&2
      exit 1
    fi
    "$mock_python" "${@:3}"
    ;;
  *)
    echo "unsupported poetry command" >&2
    exit 1
    ;;
esac
`, mockPythonPath, installMarker)
	if err := os.WriteFile(poetryPath, []byte(poetryScript), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock poetry CLI: %v", err)
	}

	t.Setenv("CONCIERGE_TEST_MOCK_PYTHON_PATH", mockPythonPath)
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

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
