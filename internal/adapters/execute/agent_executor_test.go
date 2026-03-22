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
	step, ok := core.EnsureStepByID(core.EnsureStepIntegrationTestWiring)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepIntegrationTestWiring)
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

func TestAgentExecutorModelAcquisitionUsesSelectedPlanAndRecordsVerificationEvidence(t *testing.T) {
	repoRoot := t.TempDir()
	runner := &fakeAgentRunner{
		result: agent.AgentResult{Applied: true, Summary: "materialized model artifact"},
	}
	executor := NewAgentExecutor(runner)

	inspectCalls := 0
	executor.inspectStatus = func(ctx context.Context, snapshot core.WorkspaceSnapshot) (core.IntegrationStatus, error) {
		_ = ctx
		inspectCalls++
		if inspectCalls == 1 {
			return core.IntegrationStatus{
				Contracts: &core.IntegrationContracts{
					EntryFile: "leap_integration.py",
					ModelAcquisition: &core.ModelAcquisitionArtifacts{
						AcquisitionLeads: []string{"tools/export_model.py"},
					},
				},
			}, nil
		}
		if snapshot.SelectedModelPath != ".concierge/materialized_models/model.onnx" {
			t.Fatalf("expected verification pass to inspect expected output path, got %q", snapshot.SelectedModelPath)
		}
		return core.IntegrationStatus{
			Contracts: &core.IntegrationContracts{
				ResolvedModelPath: ".concierge/materialized_models/model.onnx",
			},
		}, nil
	}

	step, ok := core.EnsureStepByID(core.EnsureStepModelAcquisition)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepModelAcquisition)
	}

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		ID: "snapshot-model-plan",
		Repository: core.RepositoryState{Root: repoRoot},
		Runtime: core.RuntimeState{
			ProbeRan: true,
		},
		RuntimeProfile: &core.LocalRuntimeProfile{
			InterpreterPath:   "/tmp/repo/.venv/bin/python",
			DependenciesReady: true,
			CodeLoaderReady:   true,
		},
		ModelAcquisitionPlan: &core.ModelAcquisitionPlan{
			Strategy:           "repo_helper_export",
			DefaultChoice:      "export from weights/best.pt",
			RuntimeInvocation:  []string{"poetry", "run", "python", "tools/export_model.py"},
			WorkingDir:         "tools",
			ExpectedOutputPath: ".concierge/materialized_models/model.onnx",
			HelperPath:         ".concierge/materializers/materialize_model.py",
			Evidence: []core.ModelAcquisitionPlanEvidence{
				{Path: "README.md", Line: 12, Detail: "documents export helper", Snippet: "python tools/export_model.py"},
			},
		},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(runner.lastTask.Objective, `Execute the selected model acquisition strategy "repo_helper_export"`) {
		t.Fatalf("expected strategy-aware objective, got %q", runner.lastTask.Objective)
	}
	if strings.Contains(runner.lastTask.Objective, "Investigate repository model acquisition logic") {
		t.Fatalf("expected objective to stop rediscovering strategy, got %q", runner.lastTask.Objective)
	}
	if runner.lastTask.RepoContext == nil || runner.lastTask.RepoContext.ModelAcquisitionPlan == nil {
		t.Fatalf("expected repo context to carry the selected plan, got %+v", runner.lastTask.RepoContext)
	}
	if runner.lastTask.RepoContext.ModelAcquisitionPlan.ExpectedOutputPath != ".concierge/materialized_models/model.onnx" {
		t.Fatalf("expected repo context to preserve expected output path, got %+v", runner.lastTask.RepoContext.ModelAcquisitionPlan)
	}

	assertEvidence(t, result.Evidence, "model_acquisition.plan.strategy", "repo_helper_export")
	assertEvidence(t, result.Evidence, "model_acquisition.plan.command", "poetry run python tools/export_model.py")
	assertEvidence(t, result.Evidence, "model_acquisition.plan.expected_output_path", ".concierge/materialized_models/model.onnx")
	assertEvidence(t, result.Evidence, "model_acquisition.materialization.expected_output_path", ".concierge/materialized_models/model.onnx")
	assertEvidence(t, result.Evidence, "model_acquisition.materialization.runtime_verification", "passed")
	assertEvidence(t, result.Evidence, "model_acquisition.materialization.output_path", ".concierge/materialized_models/model.onnx")
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

func TestPreprocessAuthoringTaskPrefersRepositoryEvidenceOverInventedPaths(t *testing.T) {
	repoRoot := t.TempDir()
	writeTextFixtureFile(t, filepath.Join(repoRoot, "ultralytics", "cfg", "datasets", "coco8.yaml"), strings.Join([]string{
		"path: coco8",
		"train: images/train",
		"val: images/val",
		"download: https://example.invalid/coco8.zip",
		"",
	}, "\n"))
	writeTextFixtureFile(t, filepath.Join(repoRoot, "ultralytics", "data", "utils.py"), strings.Join([]string{
		"def check_det_dataset(dataset, autodownload=True):",
		"    return dataset",
		"",
	}, "\n"))
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
		ID: "snapshot-preprocess-repo-evidence",
		FileHashes: map[string]string{
			"ultralytics/tensorleap_folder/README.md":           "hash-readme",
			"ultralytics/tensorleap_folder/pose/leap_binder.py": "hash-binder",
			"ultralytics/cfg/default.yaml":                      "hash-default",
			"ultralytics/cfg/datasets/coco8.yaml":               "hash-coco8",
		},
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	expected := []string{
		"Prefer existing repository dataset configuration, helper code, and project metadata before inventing new dataset roots or sample conventions.",
		"Prefer repo-local dataset and Tensorleap integration configuration over hand-coded installed-package defaults or home-directory settings.",
		"If repository conventions point to a sibling datasets directory, verify that the resolved sibling path is writable in the current runtime; do not translate a repo mounted at /workspace into a new filesystem-root directory such as /datasets.",
		"If the repository already declares train/validation subsets in a dataset manifest, reuse those declared subsets instead of inventing a new split from arbitrary images.",
		"If the repository includes explicit dataset manifests, loader code, or Tensorleap integration examples, treat those as stronger evidence than arbitrary image files.",
		"If the repository exposes a supported dataset resolver or downloader, prefer that helper over hard-coded cache roots or generic image scans.",
		"Smoke-test any repository dataset resolver before wiring it into preprocess; if the helper import fails in the current repo state, fall back to manifest-driven resolution/download instead of keeping a broken import.",
		"If a repo helper import fails because project dependencies are missing, do not reverse-engineer internal cache constants or framework settings paths; use explicit manifest train/val/download evidence or stop with the blocker.",
		"Do not run pip install, poetry add, or other environment mutation commands while discovering dataset paths for preprocess; if discovery depends on missing packages, stop and surface that blocker.",
		"Do not set deprecated `PreprocessResponse.length`; provide real `sample_ids` for each subset and let Tensorleap derive lengths from them.",
		"Do not create or write to top-level absolute directories outside the repo/workspace just to satisfy preprocess data access; if the repo-supported path is unavailable in the current runtime, stop and surface that blocker or use a repo-local writable fallback supported by repository evidence.",
		"Do not hard-code home-directory dataset defaults, installed-package cache roots, or new environment-variable paths unless repository evidence requires them and the repository itself uses them.",
		"Do not fabricate placeholder sample IDs, dummy image paths, or guessed absolute dataset locations just to satisfy subset requirements.",
		"Do not repurpose generic repository assets, screenshots, docs media, or example images as train/validation data unless repository evidence explicitly identifies them as the real dataset.",
		"If repository evidence does not expose real train/validation identifiers and no repo-supported acquisition path exists, stop and surface the missing data requirement instead of guessing.",
	}
	for _, want := range expected {
		found := false
		for _, constraint := range runner.lastTask.Constraints {
			if constraint == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected constraint %q in %+v", want, runner.lastTask.Constraints)
		}
	}

	foundEvidencePaths := false
	for _, constraint := range runner.lastTask.Constraints {
		if strings.Contains(constraint, "ultralytics/tensorleap_folder/README.md") &&
			strings.Contains(constraint, "ultralytics/tensorleap_folder/pose/leap_binder.py") &&
			strings.Contains(constraint, "ultralytics/cfg/default.yaml") {
			foundEvidencePaths = true
			break
		}
	}
	if !foundEvidencePaths {
		t.Fatalf("expected preprocess task to include repository evidence paths, got %+v", runner.lastTask.Constraints)
	}

	foundManifestHint := false
	foundResolverHint := false
	for _, constraint := range runner.lastTask.Constraints {
		if strings.Contains(constraint, "ultralytics/cfg/datasets/coco8.yaml") &&
			strings.Contains(constraint, "train=images/train") &&
			strings.Contains(constraint, "val=images/val") {
			foundManifestHint = true
		}
		if strings.Contains(constraint, "ultralytics/data/utils.py:check_det_dataset") {
			foundResolverHint = true
		}
	}
	if !foundManifestHint {
		t.Fatalf("expected preprocess task to include dataset manifest hint, got %+v", runner.lastTask.Constraints)
	}
	if !foundResolverHint {
		t.Fatalf("expected preprocess task to include dataset resolver hint, got %+v", runner.lastTask.Constraints)
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

func TestPreprocessAgentRepoContextIncludesBlockingIssues(t *testing.T) {
	repoRoot := t.TempDir()
	writeTextFixtureFile(t, filepath.Join(repoRoot, "leap.yaml"), "entryFile: leap_integration.py\n")
	writeTextFixtureFile(t, filepath.Join(repoRoot, "leap_integration.py"), "\"\"\"baseline\"\"\"\n")

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
		ID:         "snapshot-preprocess-blockers",
		Repository: core.RepositoryState{Root: repoRoot},
	}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	found := false
	for _, item := range result.Evidence {
		if item.Name != "agent.repo_context.blocking_issues" {
			continue
		}
		if strings.Contains(item.Value, "no @tensorleap_preprocess function found") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected preprocess blocking issue in repo context evidence, got %+v", result.Evidence)
	}
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

func writeTextFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll failed for %q: %v", path, err)
	}
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
