package execute

import (
	"context"
	"fmt"

	"github.com/tensorleap/concierge/internal/core"
)

// StubExecutor is a non-mutating executor used in baseline pipeline wiring.
type StubExecutor struct{}

// NewStubExecutor creates a non-mutating executor adapter.
func NewStubExecutor() *StubExecutor {
	return &StubExecutor{}
}

// Execute returns a deterministic non-applied result for known ensure-steps.
func (e *StubExecutor) Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	_ = ctx
	_ = snapshot

	canonicalStep, ok := core.EnsureStepByID(step.ID)
	if !ok {
		return core.ExecutionResult{}, core.WrapError(
			core.KindStepNotApplicable,
			"execute.stub.step",
			fmt.Errorf("unknown ensure-step ID %q", step.ID),
		)
	}

	return core.ExecutionResult{
		Step:    canonicalStep,
		Applied: false,
		Summary: "not implemented",
		Evidence: []core.EvidenceItem{
			{Name: "executor.mode", Value: "stub"},
		},
	}, nil
}
