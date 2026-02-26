package execute

import (
	"context"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/core/ports"
)

// StepApprovalFunc asks whether Concierge should attempt changes for a step.
type StepApprovalFunc func(step core.EnsureStep) (bool, error)

// ApprovalExecutor gates step execution on explicit approval before any changes run.
type ApprovalExecutor struct {
	base    ports.Executor
	approve StepApprovalFunc
}

// NewApprovalExecutor wraps an executor with pre-execution approval checks.
func NewApprovalExecutor(base ports.Executor, approve StepApprovalFunc) *ApprovalExecutor {
	if base == nil {
		base = NewStubExecutor()
	}
	if approve == nil {
		approve = func(step core.EnsureStep) (bool, error) {
			_ = step
			return false, nil
		}
	}
	return &ApprovalExecutor{
		base:    base,
		approve: approve,
	}
}

// Execute asks for approval before running actionable ensure-steps.
func (e *ApprovalExecutor) Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	canonicalStep, ok := core.EnsureStepByID(step.ID)
	if ok {
		step = canonicalStep
	}

	if step.ID != core.EnsureStepComplete {
		approved, err := e.approve(step)
		if err != nil {
			return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.approval.prompt", err)
		}
		if !approved {
			return core.ExecutionResult{
				Step:    step,
				Applied: false,
				Summary: "waiting for approval before making changes",
				Evidence: []core.EvidenceItem{
					{Name: "executor.mode", Value: "approval_gate"},
					{Name: "executor.change_approval", Value: "rejected"},
				},
			}, nil
		}
	}

	result, err := e.base.Execute(ctx, snapshot, step)
	if err != nil {
		return core.ExecutionResult{}, err
	}
	result.Evidence = append(result.Evidence, core.EvidenceItem{Name: "executor.change_approval", Value: "approved"})
	return result, nil
}
