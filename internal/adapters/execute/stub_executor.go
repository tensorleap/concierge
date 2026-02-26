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

// DispatcherExecutor routes supported deterministic steps to filesystem executor and
// falls back to the stub executor for unsupported ensure-steps.
type DispatcherExecutor struct {
	filesystem *FilesystemExecutor
	fallback   *StubExecutor
}

// NewDispatcherExecutor creates an executor that applies deterministic mutations where available.
func NewDispatcherExecutor() *DispatcherExecutor {
	return &DispatcherExecutor{
		filesystem: NewFilesystemExecutor(),
		fallback:   NewStubExecutor(),
	}
}

// Execute dispatches supported ensure-steps to filesystem mode and uses stub mode for the rest.
func (d *DispatcherExecutor) Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	if isFilesystemStep(step.ID) {
		return d.filesystem.Execute(ctx, snapshot, step)
	}
	return d.fallback.Execute(ctx, snapshot, step)
}

func isFilesystemStep(stepID core.EnsureStepID) bool {
	switch stepID {
	case core.EnsureStepLeapYAML, core.EnsureStepIntegrationScript, core.EnsureStepIntegrationTestContract:
		return true
	default:
		return false
	}
}
