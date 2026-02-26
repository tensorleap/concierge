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

	policy := newPlanningPolicy()
	primary, additional, complete := policy.build(status)

	if primary.ID == "" {
		return core.ExecutionPlan{}, core.NewError(core.KindUnknown, "planner.deterministic.primary", "planner produced an empty primary step")
	}

	plan := core.ExecutionPlan{Primary: primary}
	if !complete && len(additional) > 0 {
		plan.Additional = append([]core.EnsureStep(nil), additional...)
	}

	return plan, nil
}
