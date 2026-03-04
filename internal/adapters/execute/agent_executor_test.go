package execute

import (
	"context"
	"os"
	"path/filepath"
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

func TestAgentExecutorPreprocessObjectiveIncludesSelectedModelPath(t *testing.T) {
	repoRoot := t.TempDir()
	runner := &fakeAgentRunner{
		result: agent.AgentResult{Applied: true, Summary: "ok"},
	}
	executor := NewAgentExecutor(runner)
	step, ok := core.EnsureStepByID(core.EnsureStepPreprocessContract)
	if !ok {
		t.Fatalf("expected step %q to exist", core.EnsureStepPreprocessContract)
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
		if constraint == `Use model path "model/demo.onnx" for @tensorleap_load_model unless repository code proves this path is invalid` {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected model-path constraint in task constraints, got %+v", runner.lastTask.Constraints)
	}
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
