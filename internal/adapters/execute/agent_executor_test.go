package execute

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/agent"
	"github.com/tensorleap/concierge/internal/core"
)

func TestAgentExecutorDispatchesSupportedSteps(t *testing.T) {
	repoRoot := t.TempDir()
	runner := &fakeAgentRunner{
		result: agent.AgentResult{
			Applied: true,
			Summary: "agent applied preprocess fixes",
		},
	}

	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepPreprocessContract)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepPreprocessContract)
	}

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID: "snapshot-1",
		Repository: core.RepositoryState{
			Root: repoRoot,
		},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected Applied=true from agent result")
	}
	if result.Summary != "agent applied preprocess fixes" {
		t.Fatalf("unexpected summary %q", result.Summary)
	}

	if runner.lastTask.Objective == "" {
		t.Fatal("expected non-empty task objective")
	}
	if runner.lastTask.RepoRoot != repoRoot {
		t.Fatalf("expected task repo root %q, got %q", repoRoot, runner.lastTask.RepoRoot)
	}
	if runner.lastTask.ScopePolicy == nil {
		t.Fatal("expected scope policy to be attached to agent task")
	}
	if len(runner.lastTask.ScopePolicy.DomainSections) == 0 {
		t.Fatalf("expected scope-policy domain sections, got %+v", runner.lastTask.ScopePolicy)
	}
	if runner.lastTask.RepoContext == nil {
		t.Fatal("expected repo context to be attached to agent task")
	}
	if runner.lastTask.DomainKnowledge == nil {
		t.Fatal("expected domain knowledge payload to be attached to agent task")
	}
	if runner.lastTask.DomainKnowledge.Version == "" {
		t.Fatalf("expected non-empty domain knowledge version, got %+v", runner.lastTask.DomainKnowledge)
	}
	if len(runner.lastTask.DomainKnowledge.SectionIDs) == 0 {
		t.Fatalf("expected scoped domain knowledge section IDs, got %+v", runner.lastTask.DomainKnowledge)
	}

	assertEvidence(t, result.Evidence, "executor.mode", "agent")
	assertEvidencePresent(t, result.Evidence, "agent.transcript_path")
	assertEvidence(t, result.Evidence, "agent.knowledge_pack.version", "tlkp-v1")
	assertEvidencePresent(t, result.Evidence, "agent.knowledge_pack.section_ids")
	assertEvidencePresent(t, result.Evidence, "agent.repo_context.path")
	assertEvidencePresent(t, result.Evidence, "agent.repo_context.entry_file")
	assertEvidencePresent(t, result.Evidence, "agent.scope_policy.allowed_files")
	assertEvidencePresent(t, result.Evidence, "agent.scope_policy.forbidden_areas")
	assertEvidencePresent(t, result.Evidence, "agent.scope_policy.required_outcomes")
	assertEvidencePresent(t, result.Evidence, "agent.scope_policy.stop_and_ask_triggers")
	assertEvidencePresent(t, result.Evidence, "agent.scope_policy.domain_sections")

	repoContextPath := evidenceValue(result.Evidence, "agent.repo_context.path")
	if repoContextPath == "" {
		t.Fatalf("expected repo-context evidence path in %+v", result.Evidence)
	}
	if _, err := os.Stat(repoContextPath); err != nil {
		t.Fatalf("expected repo-context evidence file %q to exist: %v", repoContextPath, err)
	}
}

func TestAgentExecutorReturnsDeterministicErrorWhenUnavailable(t *testing.T) {
	repoRoot := t.TempDir()
	runner := &fakeAgentRunner{err: core.NewError(core.KindMissingDependency, "agent.runner.command_lookup", "Claude CLI is unavailable")}
	executor := NewAgentExecutor(runner)

	step, ok := core.EnsureStepByID(core.EnsureStepInputEncoders)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepInputEncoders)
	}

	_, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:         "snapshot-2",
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err == nil {
		t.Fatal("expected missing dependency error")
	}
	if got := core.KindOf(err); got != core.KindMissingDependency {
		t.Fatalf("expected error kind %q, got %q (err=%v)", core.KindMissingDependency, got, err)
	}
}

func TestAgentExecutorFailsWhenKnowledgePackLoadFails(t *testing.T) {
	repoRoot := t.TempDir()
	runner := &fakeAgentRunner{
		result: agent.AgentResult{Applied: true, Summary: "unexpected"},
	}
	executor := NewAgentExecutor(runner)
	executor.loadKnowledgePack = func() (agent.DomainKnowledgePack, error) {
		return agent.DomainKnowledgePack{}, core.NewError(core.KindUnknown, "agent.context.load", "invalid knowledge pack")
	}

	step, ok := core.EnsureStepByID(core.EnsureStepInputEncoders)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepInputEncoders)
	}

	_, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:         "snapshot-knowledge",
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err == nil {
		t.Fatal("expected knowledge-pack load error")
	}
	if got := core.KindOf(err); got != core.KindUnknown {
		t.Fatalf("expected error kind %q, got %q (err=%v)", core.KindUnknown, got, err)
	}
	if runner.runCount != 0 {
		t.Fatalf("expected runner not to be invoked when knowledge load fails, got %d runs", runner.runCount)
	}
}

func TestAgentTranscriptPersistedAsEvidence(t *testing.T) {
	repoRoot := t.TempDir()
	runner := &fakeAgentRunner{
		onRun: func(task agent.AgentTask) {
			if err := os.MkdirAll(filepath.Dir(task.TranscriptPath), 0o755); err != nil {
				t.Fatalf("MkdirAll failed: %v", err)
			}
			if err := os.WriteFile(task.TranscriptPath, []byte("agent transcript"), 0o644); err != nil {
				t.Fatalf("WriteFile failed: %v", err)
			}
		},
		result: agent.AgentResult{
			Applied: true,
			Summary: "agent task completed",
		},
	}

	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepGroundTruthEncoders)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepGroundTruthEncoders)
	}

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:         "snapshot-3",
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	transcriptPath := runner.lastTask.TranscriptPath
	if _, err := os.Stat(transcriptPath); err != nil {
		t.Fatalf("expected transcript file %q to exist: %v", transcriptPath, err)
	}
	assertEvidence(t, result.Evidence, "agent.transcript_path", transcriptPath)
}

func TestAgentExecutorSupportsIntegrationTestContractStep(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, filepath.Join(repoRoot, "leap.yaml"))
	writeTestFile(t, filepath.Join(repoRoot, "model", "model.h5"))
	if err := os.WriteFile(filepath.Join(repoRoot, "leap.yaml"), []byte("entryFile: leap_integration.py\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "leap_integration.py"), []byte(strings.Join([]string{
		"@tensorleap_input_encoder(name='image')",
		"def image_input(sample_id, preprocess_response):",
		"    return sample_id",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return None",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess_response):",
		"    return None",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	runner := &fakeAgentRunner{
		result: agent.AgentResult{Applied: true, Summary: "integration-test repaired"},
	}
	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepIntegrationTestContract)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepIntegrationTestContract)
	}

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:         "snapshot-integration-test",
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if runner.lastTask.Objective == "" {
		t.Fatal("expected non-empty task objective")
	}
	assertEvidencePresent(t, result.Evidence, "authoring.recommendation.integration_test.rationale")
	assertEvidencePresent(t, result.Evidence, "agent.scope_policy.domain_sections")
}

func TestAgentExecutorRejectsRemovedOptionalStepsInV1(t *testing.T) {
	executor := NewAgentExecutor(&fakeAgentRunner{})

	removedSteps := []core.EnsureStepID{
		core.EnsureStepID("ensure.optional_hooks"),
		core.EnsureStepID("ensure.metadata_functions"),
		core.EnsureStepID("ensure.visualizers"),
		core.EnsureStepID("ensure.metrics"),
		core.EnsureStepID("ensure.loss"),
	}

	for _, stepID := range removedSteps {
		_, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
			ID:         "snapshot-legacy",
			Repository: core.RepositoryState{Root: t.TempDir()},
		}, core.EnsureStep{ID: stepID})
		if err == nil {
			t.Fatalf("expected removed step %q to be rejected", stepID)
		}
		if got := core.KindOf(err); got != core.KindStepNotApplicable {
			t.Fatalf("expected error kind %q for step %q, got %q (err=%v)", core.KindStepNotApplicable, stepID, got, err)
		}
	}
}

func TestAgentExecutorModelAcquisitionObjectiveIncludesSelectedModelPath(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, filepath.Join(repoRoot, "model", "demo.pt"))
	if err := os.WriteFile(filepath.Join(repoRoot, "leap.yaml"), []byte("entryFile: leap_integration.py\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "leap_integration.py"), []byte(strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_preprocess, tensorleap_integration_test",
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
		"    return None",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	runner := &fakeAgentRunner{
		result: agent.AgentResult{Applied: true, Summary: "ok"},
	}
	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepModelAcquisition)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepModelAcquisition)
	}

	_, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:                "snapshot-model-hint",
		SelectedModelPath: "model/demo.onnx",
		Repository:        core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	found := false
	for _, constraint := range runner.lastTask.Constraints {
		if constraint == `Materialize the supported artifact at "model/demo.onnx" unless repository evidence proves a different repo-local output path is required` {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected model-acquisition target constraint in task constraints, got %+v", runner.lastTask.Constraints)
	}
}

func TestModelAuthoringAgentTaskIncludesCandidateContext(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, filepath.Join(repoRoot, "models", "z.h5"))
	writeTestFile(t, filepath.Join(repoRoot, "models", "a.onnx"))
	if err := os.WriteFile(filepath.Join(repoRoot, "leap.yaml"), []byte("entryFile: leap_integration.py\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "leap_integration.py"), []byte(strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_preprocess, tensorleap_integration_test",
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
		"    return None",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	runner := &fakeAgentRunner{
		result: agent.AgentResult{Applied: true, Summary: "model contract fixed"},
	}
	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepModelContract)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepModelContract)
	}

	_, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:         "snapshot-model-context",
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if runner.lastTask.RepoContext == nil {
		t.Fatal("expected repo context for model authoring task")
	}
	if runner.lastTask.RepoContext.SelectedModelPath != "models/a.onnx" {
		t.Fatalf("expected selected model path %q, got %q", "models/a.onnx", runner.lastTask.RepoContext.SelectedModelPath)
	}
	if len(runner.lastTask.RepoContext.ModelCandidates) == 0 {
		t.Fatalf("expected model candidates in repo context, got %+v", runner.lastTask.RepoContext)
	}

	foundCandidatesConstraint := false
	for _, constraint := range runner.lastTask.Constraints {
		if strings.Contains(constraint, "Candidate model paths:") {
			foundCandidatesConstraint = true
			break
		}
	}
	if !foundCandidatesConstraint {
		t.Fatalf("expected candidate-context constraint in task constraints, got %+v", runner.lastTask.Constraints)
	}
}

func TestModelAuthoringEvidenceContainsSelectedModelPath(t *testing.T) {
	repoRoot := t.TempDir()
	runner := &fakeAgentRunner{
		result: agent.AgentResult{Applied: true, Summary: "model contract fixed"},
	}
	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepModelContract)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepModelContract)
	}

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:                "snapshot-model-evidence",
		SelectedModelPath: "model/selected.h5",
		Repository:        core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	assertEvidence(t, result.Evidence, "authoring.recommendation.model.target", "model/selected.h5")
	assertEvidence(t, result.Evidence, "authoring.recommendation.model.rationale", "selected_model_path_override")
	if len(result.Recommendations) == 0 {
		t.Fatalf("expected execution result recommendations, got %+v", result)
	}
	if result.Recommendations[0].Target != "model/selected.h5" {
		t.Fatalf("expected recommendation target %q, got %+v", "model/selected.h5", result.Recommendations)
	}
}

func TestPreprocessAuthoringTaskIncludesTrainValidationConstraint(t *testing.T) {
	repoRoot := t.TempDir()
	runner := &fakeAgentRunner{
		result: agent.AgentResult{
			Applied: true,
			Summary: "preprocess contract fixed",
		},
	}
	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepPreprocessContract)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepPreprocessContract)
	}

	_, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:         "snapshot-preprocess-constraints",
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	foundTrain := false
	foundValidation := false
	for _, constraint := range runner.lastTask.Constraints {
		if strings.Contains(constraint, "train") {
			foundTrain = true
		}
		if strings.Contains(constraint, "validation") {
			foundValidation = true
		}
	}
	if !foundTrain || !foundValidation {
		t.Fatalf("expected constraints to include train/validation requirements, got %+v", runner.lastTask.Constraints)
	}
}

func TestPreprocessAuthoringEvidenceCapturesTargetSymbols(t *testing.T) {
	repoRoot := t.TempDir()
	binderPath := filepath.Join(repoRoot, "leap_integration.py")
	writePreprocessFixtureFile(t, binderPath, "preprocess_data")

	runner := &fakeAgentRunner{
		result: agent.AgentResult{
			Applied: true,
			Summary: "preprocess contract fixed",
		},
	}
	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepPreprocessContract)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepPreprocessContract)
	}

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:         "snapshot-preprocess-evidence",
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	assertEvidence(t, result.Evidence, "authoring.recommendation.preprocess.target_symbols", "preprocess_data")
}

func TestInputEncoderAuthoringTaskCarriesSymbolList(t *testing.T) {
	repoRoot := t.TempDir()
	binderPath := filepath.Join(repoRoot, "leap_integration.py")
	writeInputEncoderFixtureFile(t, binderPath, "image", []string{"encode_image", "encode_meta"})

	runner := &fakeAgentRunner{
		result: agent.AgentResult{
			Applied: true,
			Summary: "input encoders fixed",
		},
	}
	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepInputEncoders)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepInputEncoders)
	}

	_, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:         "snapshot-input-task",
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	foundSymbolConstraint := false
	for _, constraint := range runner.lastTask.Constraints {
		if strings.Contains(constraint, "Required input symbols: meta") {
			foundSymbolConstraint = true
			break
		}
	}
	if !foundSymbolConstraint {
		t.Fatalf("expected symbol-list constraint in task constraints, got %+v", runner.lastTask.Constraints)
	}
}

func TestInputEncoderAuthoringEvidenceIncludesRecommendationAndResult(t *testing.T) {
	repoRoot := t.TempDir()
	binderPath := filepath.Join(repoRoot, "leap_integration.py")
	writeInputEncoderFixtureFile(t, binderPath, "image", []string{"encode_image", "encode_meta"})

	runner := &fakeAgentRunner{
		result: agent.AgentResult{
			Applied: true,
			Summary: "input encoders fixed",
		},
	}
	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepInputEncoders)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepInputEncoders)
	}

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:         "snapshot-input-evidence",
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	assertEvidence(t, result.Evidence, "authoring.recommendation.input_encoder.target_symbols", "meta")
	if len(result.Recommendations) == 0 {
		t.Fatalf("expected recommendations in execution result, got %+v", result)
	}
	if result.Recommendations[0].StepID != core.EnsureStepInputEncoders {
		t.Fatalf("expected recommendation step %q, got %+v", core.EnsureStepInputEncoders, result.Recommendations)
	}
}

func TestGTEncoderAuthoringTaskIncludesLabeledSubsetConstraint(t *testing.T) {
	repoRoot := t.TempDir()
	binderPath := filepath.Join(repoRoot, "leap_integration.py")
	writeGTEncoderFixtureFile(t, binderPath, "label", []string{"encode_label", "encode_mask"})

	runner := &fakeAgentRunner{
		result: agent.AgentResult{
			Applied: true,
			Summary: "gt encoders fixed",
		},
	}
	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepGroundTruthEncoders)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepGroundTruthEncoders)
	}

	_, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:         "snapshot-gt-task",
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	foundLabeledSubsetConstraint := false
	for _, constraint := range runner.lastTask.Constraints {
		if strings.Contains(strings.ToLower(constraint), "labeled subsets only") {
			foundLabeledSubsetConstraint = true
			break
		}
	}
	if !foundLabeledSubsetConstraint {
		t.Fatalf("expected labeled-subset constraint in task constraints, got %+v", runner.lastTask.Constraints)
	}
}

func TestGTEncoderAuthoringEvidenceContainsTargetSymbols(t *testing.T) {
	repoRoot := t.TempDir()
	binderPath := filepath.Join(repoRoot, "leap_integration.py")
	writeGTEncoderFixtureFile(t, binderPath, "label", []string{"encode_label", "encode_mask"})

	runner := &fakeAgentRunner{
		result: agent.AgentResult{
			Applied: true,
			Summary: "gt encoders fixed",
		},
	}
	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepGroundTruthEncoders)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepGroundTruthEncoders)
	}

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID:         "snapshot-gt-evidence",
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	assertEvidence(t, result.Evidence, "authoring.recommendation.gt_encoder.target_symbols", "mask")
}

type fakeAgentRunner struct {
	result   agent.AgentResult
	err      error
	lastTask agent.AgentTask
	onRun    func(task agent.AgentTask)
	runCount int
}

func (f *fakeAgentRunner) Run(ctx context.Context, task agent.AgentTask) (agent.AgentResult, error) {
	_ = ctx
	f.runCount++
	f.lastTask = task
	if f.onRun != nil {
		f.onRun(task)
	}
	if f.err != nil {
		return agent.AgentResult{}, f.err
	}
	return f.result, nil
}

func assertEvidence(t *testing.T, items []core.EvidenceItem, name, want string) {
	t.Helper()
	for _, item := range items {
		if item.Name == name {
			if item.Value != want {
				t.Fatalf("expected evidence %q to have value %q, got %q", name, want, item.Value)
			}
			return
		}
	}
	t.Fatalf("expected evidence item %q in %+v", name, items)
}

func assertEvidencePresent(t *testing.T, items []core.EvidenceItem, name string) {
	t.Helper()
	for _, item := range items {
		if item.Name == name {
			if item.Value == "" {
				t.Fatalf("expected non-empty evidence value for %q", name)
			}
			return
		}
	}
	t.Fatalf("expected evidence item %q in %+v", name, items)
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll failed for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", path, err)
	}
}

func writePreprocessFixtureFile(t *testing.T, path, functionName string) {
	t.Helper()
	content := `from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess

@tensorleap_preprocess()
def ` + functionName + `():
    return []`
	writeTestFile(t, path)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", path, err)
	}
}

func writeInputEncoderFixtureFile(t *testing.T, path, existingSymbol string, integrationCalls []string) {
	t.Helper()
	callLines := make([]string, 0, len(integrationCalls))
	for _, call := range integrationCalls {
		callLines = append(callLines, "    "+call+"()")
	}
	content := `from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_input_encoder, tensorleap_integration_test

@tensorleap_input_encoder("` + existingSymbol + `")
def encode_` + existingSymbol + `():
    return 1

@tensorleap_integration_test()
def run_flow():
` + strings.Join(callLines, "\n")
	writeTestFile(t, path)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", path, err)
	}
}

func writeGTEncoderFixtureFile(t *testing.T, path, existingSymbol string, integrationCalls []string) {
	t.Helper()
	callLines := make([]string, 0, len(integrationCalls))
	for _, call := range integrationCalls {
		callLines = append(callLines, "    "+call+"()")
	}
	content := `from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_gt_encoder, tensorleap_integration_test

@tensorleap_gt_encoder("` + existingSymbol + `")
def encode_` + existingSymbol + `():
    return 1

@tensorleap_integration_test()
def run_flow():
` + strings.Join(callLines, "\n")
	writeTestFile(t, path)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", path, err)
	}
}
