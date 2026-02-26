package execute

import (
	"context"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestApprovalExecutorSkipsExecutionWhenRejected(t *testing.T) {
	base := &spyExecutor{
		result: core.ExecutionResult{
			Step:    core.EnsureStep{ID: core.EnsureStepLeapYAML},
			Applied: true,
		},
	}

	approvals := 0
	executor := NewApprovalExecutor(base, func(step core.EnsureStep) (bool, error) {
		approvals++
		if step.ID != core.EnsureStepLeapYAML {
			t.Fatalf("expected approval for %q, got %q", core.EnsureStepLeapYAML, step.ID)
		}
		return false, nil
	})

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{}, core.EnsureStep{ID: core.EnsureStepLeapYAML})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if approvals != 1 {
		t.Fatalf("expected one approval prompt, got %d", approvals)
	}
	if base.calls != 0 {
		t.Fatalf("expected base executor not to run, got %d calls", base.calls)
	}
	if result.Applied {
		t.Fatalf("expected rejected execution to be non-applied, got %+v", result)
	}
	if !strings.Contains(result.Summary, "waiting for approval") {
		t.Fatalf("expected approval-wait summary, got %q", result.Summary)
	}
	if !hasEvidence(result.Evidence, "executor.change_approval", "rejected") {
		t.Fatalf("expected rejected approval evidence, got %+v", result.Evidence)
	}
}

func TestApprovalExecutorDelegatesWhenApproved(t *testing.T) {
	base := &spyExecutor{
		result: core.ExecutionResult{
			Step:    core.EnsureStep{ID: core.EnsureStepLeapYAML},
			Applied: true,
			Summary: "updated leap.yaml",
			Evidence: []core.EvidenceItem{
				{Name: "executor.mode", Value: "filesystem"},
			},
		},
	}

	executor := NewApprovalExecutor(base, func(step core.EnsureStep) (bool, error) {
		if step.ID != core.EnsureStepLeapYAML {
			t.Fatalf("expected approval for %q, got %q", core.EnsureStepLeapYAML, step.ID)
		}
		return true, nil
	})

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{}, core.EnsureStep{ID: core.EnsureStepLeapYAML})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if base.calls != 1 {
		t.Fatalf("expected base executor to run once, got %d", base.calls)
	}
	if !result.Applied {
		t.Fatalf("expected approved execution to keep applied=true, got %+v", result)
	}
	if !hasEvidence(result.Evidence, "executor.change_approval", "approved") {
		t.Fatalf("expected approved evidence, got %+v", result.Evidence)
	}
}

func TestApprovalExecutorSkipsPromptForCompleteStep(t *testing.T) {
	base := &spyExecutor{
		result: core.ExecutionResult{
			Step:    core.EnsureStep{ID: core.EnsureStepComplete},
			Applied: false,
		},
	}

	approvals := 0
	executor := NewApprovalExecutor(base, func(step core.EnsureStep) (bool, error) {
		approvals++
		return true, nil
	})

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{}, core.EnsureStep{ID: core.EnsureStepComplete})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if approvals != 0 {
		t.Fatalf("expected no approval prompt for complete step, got %d", approvals)
	}
	if base.calls != 1 {
		t.Fatalf("expected base executor to run once, got %d", base.calls)
	}
	if !hasEvidence(result.Evidence, "executor.change_approval", "approved") {
		t.Fatalf("expected approved evidence on delegated execution, got %+v", result.Evidence)
	}
}

type spyExecutor struct {
	calls  int
	result core.ExecutionResult
	err    error
}

func (s *spyExecutor) Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	_ = ctx
	_ = snapshot
	_ = step
	s.calls++
	if s.err != nil {
		return core.ExecutionResult{}, s.err
	}
	return s.result, nil
}

func hasEvidence(items []core.EvidenceItem, name, value string) bool {
	for _, item := range items {
		if item.Name == name && item.Value == value {
			return true
		}
	}
	return false
}
