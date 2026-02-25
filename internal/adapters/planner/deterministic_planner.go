package planner

import (
	"context"

	"github.com/tensorleap/concierge/internal/core"
)

// DeterministicPlanner converts issues into a stable execution plan.
type DeterministicPlanner struct{}

// NewDeterministicPlanner creates a deterministic planner adapter.
func NewDeterministicPlanner() *DeterministicPlanner {
	return &DeterministicPlanner{}
}

// Plan converts integration issues into primary and additional ensure-steps.
func (p *DeterministicPlanner) Plan(ctx context.Context, snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) (core.ExecutionPlan, error) {
	_ = ctx
	_ = snapshot

	steps := core.PreferredEnsureStepsForIssues(status.Issues)
	if len(steps) == 0 {
		completeStep, ok := core.EnsureStepByID(core.EnsureStepComplete)
		if !ok {
			return core.ExecutionPlan{}, core.NewError(core.KindUnknown, "planner.deterministic.complete_step", "ensure.complete step is not registered")
		}
		return core.ExecutionPlan{Primary: completeStep}, nil
	}

	plan := core.ExecutionPlan{Primary: steps[0]}
	if len(steps) > 1 {
		plan.Additional = append([]core.EnsureStep(nil), steps[1:]...)
	}

	return plan, nil
}
