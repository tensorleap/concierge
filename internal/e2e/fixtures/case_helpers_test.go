package fixtures

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/execute"
	"github.com/tensorleap/concierge/internal/adapters/planner"
	"github.com/tensorleap/concierge/internal/adapters/snapshot"
	"github.com/tensorleap/concierge/internal/agent"
	"github.com/tensorleap/concierge/internal/core"
)

type fixtureCaseManifest struct {
	Cases []fixtureCaseEntry `json:"cases"`
}

type fixtureCaseEntry struct {
	ID                             string              `json:"id"`
	SourceFixtureID                string              `json:"source_fixture_id"`
	SourceVariant                  string              `json:"source_variant"`
	Family                         string              `json:"family"`
	Patch                          string              `json:"patch"`
	ExpectedPrimaryStep            string              `json:"expected_primary_step"`
	ExpectedIssueCodes             []string            `json:"expected_issue_codes"`
	ConfirmedMapping               *fixtureCaseMapping `json:"confirmed_mapping,omitempty"`
	ExpectedMissingModelPath       string              `json:"expected_missing_model_path,omitempty"`
	ExpectedMissingIntegrationCall string              `json:"expected_missing_integration_call,omitempty"`
	Notes                          string              `json:"notes,omitempty"`
}

type fixtureCaseMapping struct {
	InputSymbols       []string `json:"input_symbols,omitempty"`
	GroundTruthSymbols []string `json:"ground_truth_symbols,omitempty"`
}

type fixtureFakeAgentRunner struct {
	result   agent.AgentResult
	err      error
	lastTask agent.AgentTask
	runCount int
}

func (f *fixtureFakeAgentRunner) Run(ctx context.Context, task agent.AgentTask) (agent.AgentResult, error) {
	_ = ctx
	f.runCount++
	f.lastTask = task
	if f.err != nil {
		return agent.AgentResult{}, f.err
	}
	return f.result, nil
}

func requireFixtureCaseReposPrepared(t *testing.T) {
	t.Helper()
	requireFixtureReposPrepared(t)

	repoRoot := repoRootFromRuntime(t)
	for _, fixtureCase := range loadFixtureCases(t) {
		caseRoot := filepath.Join(repoRoot, ".fixtures", "cases", fixtureCase.ID)
		if _, err := os.Stat(filepath.Join(caseRoot, ".git")); err != nil {
			t.Skipf("fixture case repositories are not prepared; run `bash scripts/fixtures_mutate_cases.sh` (missing %q: %v)", caseRoot, err)
			return
		}
	}
}

func loadFixtureCases(t *testing.T) []fixtureCaseEntry {
	t.Helper()
	manifestPath := filepath.Join(repoRootFromRuntime(t), "fixtures", "cases", "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile case manifest failed: %v", err)
	}

	var manifest fixtureCaseManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("Unmarshal case manifest failed: %v", err)
	}
	if len(manifest.Cases) == 0 {
		t.Fatal("fixture case manifest is empty")
	}

	return manifest.Cases
}

func loadFixtureCase(t *testing.T, caseID string) fixtureCaseEntry {
	t.Helper()
	for _, fixtureCase := range loadFixtureCases(t) {
		if fixtureCase.ID == caseID {
			return fixtureCase
		}
	}
	t.Fatalf("fixture case %q not found in manifest", caseID)
	return fixtureCaseEntry{}
}

func resolveCaseRoot(t *testing.T, caseID string) string {
	t.Helper()
	caseRoot := filepath.Join(repoRootFromRuntime(t), ".fixtures", "cases", caseID)
	if _, err := os.Stat(filepath.Join(caseRoot, ".git")); err != nil {
		t.Fatalf("fixture case repo missing for %q at %q: %v (run bash scripts/fixtures_mutate_cases.sh)", caseID, caseRoot, err)
	}
	return caseRoot
}

func cloneCaseRepoForTest(t *testing.T, caseID string) (fixtureCaseEntry, string) {
	t.Helper()
	entry := loadFixtureCase(t, caseID)
	repoRoot := cloneFixtureRepoForTest(t, resolveCaseRoot(t, caseID))
	removeReadyModelArtifactsIfNeeded(t, entry, repoRoot)
	seedModelArtifactIfNeeded(t, entry, repoRoot)
	return entry, repoRoot
}

func captureCaseSnapshot(t *testing.T, entry fixtureCaseEntry, repoRoot string) core.WorkspaceSnapshot {
	t.Helper()
	snapshotter := snapshot.NewGitSnapshotter()
	snapshotValue, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	if entry.ConfirmedMapping != nil {
		snapshotValue.ConfirmedEncoderMapping = &core.EncoderMappingContract{
			InputSymbols:       append([]string(nil), entry.ConfirmedMapping.InputSymbols...),
			GroundTruthSymbols: append([]string(nil), entry.ConfirmedMapping.GroundTruthSymbols...),
		}
	}
	attachReadyRuntimeProfile(&snapshotValue)
	if !shouldAttachReadyRuntimeProfile(entry) {
		snapshotValue.Runtime.ProbeRan = false
	}
	return snapshotValue
}

func shouldAttachReadyRuntimeProfile(entry fixtureCaseEntry) bool {
	switch core.EnsureStepID(entry.ExpectedPrimaryStep) {
	case core.EnsureStepModelAcquisition, core.EnsureStepModelContract:
		return true
	default:
		return false
	}
}

func inspectPlanForCase(t *testing.T, entry fixtureCaseEntry, repoRoot string) (core.WorkspaceSnapshot, core.IntegrationStatus, core.ExecutionPlan) {
	t.Helper()
	snapshotValue := captureCaseSnapshot(t, entry, repoRoot)
	status := inspectWithSnapshot(t, snapshotValue)
	planAdapter := planner.NewDeterministicPlanner()
	plan, err := planAdapter.Plan(context.Background(), snapshotValue, status)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	return snapshotValue, status, plan
}

func executeAgentStepForCase(
	t *testing.T,
	entry fixtureCaseEntry,
	repoRoot string,
	stepID core.EnsureStepID,
) (*fixtureFakeAgentRunner, core.ExecutionResult) {
	t.Helper()

	runner := &fixtureFakeAgentRunner{
		result: agent.AgentResult{
			Applied: true,
			Summary: "fixture agent task completed",
		},
	}
	executor := execute.NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(stepID)
	if !ok {
		t.Fatalf("expected step %q to exist", stepID)
	}

	snapshotValue := captureCaseSnapshot(t, entry, repoRoot)
	result, err := executor.Execute(context.Background(), snapshotValue, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	return runner, result
}

func attachReadyRuntimeProfile(snapshotValue *core.WorkspaceSnapshot) {
	if snapshotValue == nil {
		return
	}

	pyprojectHash := strings.TrimSpace(snapshotValue.FileHashes["pyproject.toml"])
	poetryLockHash := strings.TrimSpace(snapshotValue.FileHashes["poetry.lock"])
	interpreterPath := "/tmp/fixture-poetry/bin/python"
	if resolvedPath, err := exec.LookPath("python3"); err == nil {
		interpreterPath = resolvedPath
	} else if resolvedPath, err := exec.LookPath("python"); err == nil {
		interpreterPath = resolvedPath
	}
	pythonVersion := "Python 3.11.0"

	snapshotValue.RuntimeProfile = &core.LocalRuntimeProfile{
		Kind:              "poetry",
		PoetryExecutable:  "poetry",
		PoetryVersion:     "Poetry 1.8.3",
		InterpreterPath:   interpreterPath,
		PythonVersion:     pythonVersion,
		ConfirmationMode:  "auto",
		DependenciesReady: true,
		CodeLoaderReady:   true,
		Fingerprint: core.RuntimeProfileFingerprint{
			ProjectRoot:     snapshotValue.Repository.Root,
			PyProjectHash:   pyprojectHash,
			PoetryLockHash:  poetryLockHash,
			InterpreterPath: interpreterPath,
			PythonVersion:   pythonVersion,
		},
	}
	snapshotValue.Runtime.ProbeRan = true
	snapshotValue.Runtime.PyProjectPresent = true
	snapshotValue.Runtime.PoetryLockPresent = poetryLockHash != ""
	snapshotValue.Runtime.SupportedProject = true
	snapshotValue.Runtime.PoetryFound = true
	snapshotValue.Runtime.PoetryExecutable = "poetry"
	snapshotValue.Runtime.PoetryVersion = "Poetry 1.8.3"
	snapshotValue.Runtime.ResolvedInterpreter = interpreterPath
	snapshotValue.Runtime.ResolvedPythonVersion = pythonVersion
}

func assertCasePrimaryStep(t *testing.T, entry fixtureCaseEntry, plan core.ExecutionPlan) {
	t.Helper()
	if plan.Primary.ID != core.EnsureStepID(entry.ExpectedPrimaryStep) {
		t.Fatalf("expected primary step %q for case %q, got %q", entry.ExpectedPrimaryStep, entry.ID, plan.Primary.ID)
	}
}

func assertExpectedIssueCodes(t *testing.T, issues []core.Issue, expectedCodes []string) {
	t.Helper()
	for _, expectedCode := range expectedCodes {
		found := false
		for _, issue := range issues {
			if string(issue.Code) == expectedCode {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected issue code %q, got %+v", expectedCode, issues)
		}
	}
}

func hasIssueWithSymbol(issues []core.Issue, symbol string, codes ...core.IssueCode) bool {
	normalizedSymbol := strings.ToLower(strings.TrimSpace(symbol))
	if normalizedSymbol == "" {
		return false
	}

	codeSet := make(map[core.IssueCode]struct{}, len(codes))
	for _, code := range codes {
		codeSet[code] = struct{}{}
	}

	for _, issue := range issues {
		if len(codeSet) > 0 {
			if _, ok := codeSet[issue.Code]; !ok {
				continue
			}
		}
		if issue.Location == nil {
			continue
		}
		if strings.ToLower(strings.TrimSpace(issue.Location.Symbol)) == normalizedSymbol {
			return true
		}
	}
	return false
}

func containsString(values []string, expected string) bool {
	needle := strings.ToLower(strings.TrimSpace(expected))
	if needle == "" {
		return false
	}
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == needle {
			return true
		}
	}
	return false
}

func evidenceValue(evidence []core.EvidenceItem, name string) string {
	for _, item := range evidence {
		if item.Name == name {
			return item.Value
		}
	}
	return ""
}

func assertConstraintContains(t *testing.T, task agent.AgentTask, expectedSubstring string) {
	t.Helper()
	for _, constraint := range task.Constraints {
		if strings.Contains(constraint, expectedSubstring) {
			return
		}
	}
	t.Fatalf("expected task constraint containing %q, got %+v", expectedSubstring, task.Constraints)
}

func assertPromptSectionPresent(t *testing.T, prompt, sectionID string) {
	t.Helper()
	if !strings.Contains(prompt, "["+sectionID+"]") {
		t.Fatalf("expected prompt section %q in prompt: %s", sectionID, prompt)
	}
}

func assertPromptSectionAbsent(t *testing.T, prompt, sectionID string) {
	t.Helper()
	if strings.Contains(prompt, "["+sectionID+"]") {
		t.Fatalf("did not expect prompt section %q in prompt: %s", sectionID, prompt)
	}
}

func copyRepoFile(t *testing.T, sourceRoot, sourceRelPath, destinationRoot, destinationRelPath string) {
	t.Helper()
	sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(sourceRelPath))
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile failed for %q: %v", sourcePath, err)
	}

	destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(destinationRelPath))
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed for %q: %v", destinationPath, err)
	}
	if err := os.WriteFile(destinationPath, raw, 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", destinationPath, err)
	}
}

func removeReadyModelArtifactsIfNeeded(t *testing.T, entry fixtureCaseEntry, repoRoot string) {
	t.Helper()
	if strings.TrimSpace(entry.ExpectedMissingModelPath) == "" {
		return
	}

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		switch strings.ToLower(filepath.Ext(d.Name())) {
		case ".h5", ".onnx":
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("removeReadyModelArtifactsIfNeeded failed: %v", err)
	}
}

func seedModelArtifactIfNeeded(t *testing.T, entry fixtureCaseEntry, repoRoot string) {
	t.Helper()
	switch core.EnsureStepID(entry.ExpectedPrimaryStep) {
	case core.EnsureStepModelAcquisition, core.EnsureStepModelContract:
		return
	}

	for _, relPath := range []string{"model.h5", filepath.Join("model", "model.h5")} {
		modelPath := filepath.Join(repoRoot, relPath)
		if _, err := os.Stat(modelPath); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
			t.Fatalf("MkdirAll failed for %q: %v", modelPath, err)
		}
		if err := os.WriteFile(modelPath, []byte("fixture placeholder model"), 0o644); err != nil {
			t.Fatalf("WriteFile failed for %q: %v", modelPath, err)
		}
	}
}
