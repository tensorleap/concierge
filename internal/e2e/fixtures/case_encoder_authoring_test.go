package fixtures

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/execute"
	"github.com/tensorleap/concierge/internal/adapters/planner"
	"github.com/tensorleap/concierge/internal/adapters/snapshot"
	"github.com/tensorleap/concierge/internal/adapters/validate"
	"github.com/tensorleap/concierge/internal/core"
)

func TestFixtureCaseMissingInputEncoders_Recovers(t *testing.T) {
	requireFixtureReposPrepared(t)
	t.Setenv(validate.HarnessEnableEnvVar, "0")

	_, postRoot := resolveFixtureRoots(t, "mnist")
	integrationPath := filepath.Join(postRoot, "leap_integration.py")
	binderPath := filepath.Join(postRoot, "leap_binder.py")

	restoreIntegration := replaceFirstInFile(
		t,
		integrationPath,
		"    image = input_encoder(idx, subset)\n",
		"    image = input_encoder(idx, subset)\n    meta = encode_meta(idx, subset)\n",
	)
	defer restoreIntegration()

	snapshotValue := snapshotWithConfirmedMapping(t, postRoot, []string{"image", "meta"}, []string{"classes"})
	status := inspectWithSnapshot(t, snapshotValue)
	if !hasIssueWithSymbol(status.Issues, "meta", core.IssueCodeInputEncoderCoverageIncomplete, core.IssueCodeInputEncoderMissing) {
		t.Fatalf("expected input-encoder issue for symbol %q, got %+v", "meta", status.Issues)
	}

	planAdapter := planner.NewDeterministicPlanner()
	plan, err := planAdapter.Plan(context.Background(), snapshotValue, status)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if plan.Primary.ID != core.EnsureStepInputEncoders {
		t.Fatalf("expected primary step %q, got %q", core.EnsureStepInputEncoders, plan.Primary.ID)
	}

	recommendation, err := execute.BuildInputEncoderAuthoringRecommendation(snapshotValue, status)
	if err != nil {
		t.Fatalf("BuildInputEncoderAuthoringRecommendation failed: %v", err)
	}
	if recommendation.StepID != core.EnsureStepInputEncoders {
		t.Fatalf("expected recommendation step %q, got %q", core.EnsureStepInputEncoders, recommendation.StepID)
	}
	if !containsString(recommendation.Candidates, "meta") {
		t.Fatalf("expected recommendation candidates to include %q, got %v", "meta", recommendation.Candidates)
	}

	restoreBinder := appendToFile(t, binderPath, strings.Join([]string{
		"",
		"@tensorleap_input_encoder('meta', channel_dim=-1)",
		"def encode_meta(idx: int, preprocess: PreprocessResponse) -> np.ndarray:",
		"    return np.array([float(idx)], dtype='float32')",
		"",
	}, "\n"))
	defer restoreBinder()

	statusAfter := inspectWithSnapshot(t, snapshotWithConfirmedMapping(t, postRoot, []string{"image", "meta"}, []string{"classes"}))
	if hasIssueWithSymbol(statusAfter.Issues, "meta", core.IssueCodeInputEncoderCoverageIncomplete, core.IssueCodeInputEncoderMissing) {
		t.Fatalf("expected input-encoder symbol %q to recover, got %+v", "meta", statusAfter.Issues)
	}
}

func TestFixtureCaseMissingGTEncoders_Recovers(t *testing.T) {
	requireFixtureReposPrepared(t)
	t.Setenv(validate.HarnessEnableEnvVar, "0")

	_, postRoot := resolveFixtureRoots(t, "mnist")
	integrationPath := filepath.Join(postRoot, "leap_integration.py")
	binderPath := filepath.Join(postRoot, "leap_binder.py")

	restoreIntegration := replaceFirstInFile(
		t,
		integrationPath,
		"    gt = gt_encoder(idx, subset)\n",
		"    gt = gt_encoder(idx, subset)\n    aux_gt = encode_label(idx, subset)\n",
	)
	defer restoreIntegration()

	snapshotValue := snapshotWithConfirmedMapping(t, postRoot, []string{"image"}, []string{"classes", "label"})
	status := inspectWithSnapshot(t, snapshotValue)
	if !hasIssueWithSymbol(status.Issues, "label", core.IssueCodeGTEncoderCoverageIncomplete, core.IssueCodeGTEncoderMissing) {
		t.Fatalf("expected GT-encoder issue for symbol %q, got %+v", "label", status.Issues)
	}

	planAdapter := planner.NewDeterministicPlanner()
	plan, err := planAdapter.Plan(context.Background(), snapshotValue, status)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if plan.Primary.ID != core.EnsureStepGroundTruthEncoders {
		t.Fatalf("expected primary step %q, got %q", core.EnsureStepGroundTruthEncoders, plan.Primary.ID)
	}

	recommendation, err := execute.BuildGTEncoderAuthoringRecommendation(snapshotValue, status)
	if err != nil {
		t.Fatalf("BuildGTEncoderAuthoringRecommendation failed: %v", err)
	}
	if recommendation.StepID != core.EnsureStepGroundTruthEncoders {
		t.Fatalf("expected recommendation step %q, got %q", core.EnsureStepGroundTruthEncoders, recommendation.StepID)
	}
	if !containsString(recommendation.Candidates, "label") {
		t.Fatalf("expected recommendation candidates to include %q, got %v", "label", recommendation.Candidates)
	}

	restoreBinder := appendToFile(t, binderPath, strings.Join([]string{
		"",
		"@tensorleap_gt_encoder('label')",
		"def encode_label(idx: int, preprocessing: PreprocessResponse) -> np.ndarray:",
		"    return preprocessing.data['labels'][idx].astype('float32')",
		"",
	}, "\n"))
	defer restoreBinder()

	statusAfter := inspectWithSnapshot(t, snapshotWithConfirmedMapping(t, postRoot, []string{"image"}, []string{"classes", "label"}))
	if hasIssueWithSymbol(statusAfter.Issues, "label", core.IssueCodeGTEncoderCoverageIncomplete, core.IssueCodeGTEncoderMissing) {
		t.Fatalf("expected GT-encoder symbol %q to recover, got %+v", "label", statusAfter.Issues)
	}
}

func snapshotWithConfirmedMapping(
	t *testing.T,
	repoRoot string,
	inputSymbols []string,
	groundTruthSymbols []string,
) core.WorkspaceSnapshot {
	t.Helper()
	snapshotter := snapshot.NewGitSnapshotter()
	snapshotValue, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	snapshotValue.ConfirmedEncoderMapping = &core.EncoderMappingContract{
		InputSymbols:       append([]string(nil), inputSymbols...),
		GroundTruthSymbols: append([]string(nil), groundTruthSymbols...),
	}
	attachReadyRuntimeProfile(&snapshotValue)
	return snapshotValue
}

func attachReadyRuntimeProfile(snapshotValue *core.WorkspaceSnapshot) {
	if snapshotValue == nil {
		return
	}

	pyprojectHash := strings.TrimSpace(snapshotValue.FileHashes["pyproject.toml"])
	poetryLockHash := strings.TrimSpace(snapshotValue.FileHashes["poetry.lock"])
	interpreterPath := "/tmp/fixture-poetry/bin/python"
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

func replaceFirstInFile(t *testing.T, filePath string, old, updated string) func() {
	t.Helper()
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile failed for %q: %v", filePath, err)
	}
	original := string(raw)
	if !strings.Contains(original, old) {
		t.Fatalf("expected content not found in %q: %q", filePath, old)
	}

	replaced := strings.Replace(original, old, updated, 1)
	if err := os.WriteFile(filePath, []byte(replaced), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", filePath, err)
	}

	return func() {
		if writeErr := os.WriteFile(filePath, []byte(original), 0o644); writeErr != nil {
			t.Fatalf("restore WriteFile failed for %q: %v", filePath, writeErr)
		}
	}
}

func appendToFile(t *testing.T, filePath string, suffix string) func() {
	t.Helper()
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile failed for %q: %v", filePath, err)
	}
	original := string(raw)
	if strings.TrimSpace(suffix) == "" {
		t.Fatalf("suffix must not be empty for %q", filePath)
	}

	if err := os.WriteFile(filePath, []byte(original+suffix), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", filePath, err)
	}

	return func() {
		if writeErr := os.WriteFile(filePath, []byte(original), 0o644); writeErr != nil {
			t.Fatalf("restore WriteFile failed for %q: %v", filePath, writeErr)
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
