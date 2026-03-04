package execute

import (
	"context"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestExecutorReturnsStubResultForKnownStep(t *testing.T) {
	executor := NewStubExecutor()
	step, ok := core.EnsureStepByID(core.EnsureStepLeapYAML)
	if !ok {
		t.Fatalf("expected step %q to be registered", core.EnsureStepLeapYAML)
	}

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Step.ID != core.EnsureStepLeapYAML {
		t.Fatalf("expected result step %q, got %q", core.EnsureStepLeapYAML, result.Step.ID)
	}
	if result.Applied {
		t.Fatal("expected Applied=false for stub executor")
	}
	if result.Summary != "not implemented" {
		t.Fatalf("expected summary %q, got %q", "not implemented", result.Summary)
	}
}

func TestExecutorRejectsUnknownStep(t *testing.T) {
	executor := NewStubExecutor()

	_, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{}, core.EnsureStep{ID: core.EnsureStepID("ensure.unknown")})
	if err == nil {
		t.Fatal("expected error for unknown step")
	}
	if got := core.KindOf(err); got != core.KindStepNotApplicable {
		t.Fatalf("expected error kind %q, got %q (err=%v)", core.KindStepNotApplicable, got, err)
	}
}

func TestExecutorReturnsEvidenceStub(t *testing.T) {
	executor := NewStubExecutor()
	step, ok := core.EnsureStepByID(core.EnsureStepIntegrationScript)
	if !ok {
		t.Fatalf("expected step %q to be registered", core.EnsureStepIntegrationScript)
	}

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{}, step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(result.Evidence) != 1 {
		t.Fatalf("expected one evidence item, got %+v", result.Evidence)
	}
	if result.Evidence[0].Name != "executor.mode" {
		t.Fatalf("expected evidence name %q, got %q", "executor.mode", result.Evidence[0].Name)
	}
	if result.Evidence[0].Value != "stub" {
		t.Fatalf("expected evidence value %q, got %q", "stub", result.Evidence[0].Value)
	}
}

func TestDispatcherRequiresAgentForPreprocessContract(t *testing.T) {
	executor := NewDispatcherExecutor()
	step, ok := core.EnsureStepByID(core.EnsureStepPreprocessContract)
	if !ok {
		t.Fatalf("expected step %q to be registered", core.EnsureStepPreprocessContract)
	}

	_, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{}, step)
	if err == nil {
		t.Fatal("expected missing dependency error for preprocess contract without agent")
	}
	if got := core.KindOf(err); got != core.KindMissingDependency {
		t.Fatalf("expected error kind %q, got %q (err=%v)", core.KindMissingDependency, got, err)
	}
}

func TestDispatcherRequiresAgentForModelContract(t *testing.T) {
	executor := NewDispatcherExecutor()
	step, ok := core.EnsureStepByID(core.EnsureStepModelContract)
	if !ok {
		t.Fatalf("expected step %q to be registered", core.EnsureStepModelContract)
	}

	_, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		SelectedModelPath: "model/demo.h5",
		Repository:        core.RepositoryState{Root: t.TempDir()},
	}, step)
	if err == nil {
		t.Fatal("expected missing dependency error for model contract without agent")
	}
	if got := core.KindOf(err); got != core.KindMissingDependency {
		t.Fatalf("expected error kind %q, got %q (err=%v)", core.KindMissingDependency, got, err)
	}
}
