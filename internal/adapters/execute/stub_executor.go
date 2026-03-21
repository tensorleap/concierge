package execute

import (
	"context"
	"fmt"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/observe"
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
	poetry     *PoetryDependencyExecutor
	filesystem *FilesystemExecutor
	agent      *AgentExecutor
	fallback   *StubExecutor
}

// NewDispatcherExecutor creates an executor that applies deterministic mutations where available.
func NewDispatcherExecutor() *DispatcherExecutor {
	return NewDispatcherExecutorWithAgent(nil)
}

// NewDispatcherExecutorWithAgent creates an executor with optional agent fallback.
func NewDispatcherExecutorWithAgent(agentExecutor *AgentExecutor) *DispatcherExecutor {
	return &DispatcherExecutor{
		poetry:     NewPoetryDependencyExecutor(),
		filesystem: NewFilesystemExecutor(),
		agent:      agentExecutor,
		fallback:   NewStubExecutor(),
	}
}

// SetObserver configures shared live progress reporting for supported executors.
func (d *DispatcherExecutor) SetObserver(sink observe.Sink) {
	if d == nil {
		return
	}
	if d.poetry != nil {
		d.poetry.SetObserver(sink)
	}
	if d.agent != nil {
		d.agent.SetObserver(sink)
	}
}

// Execute dispatches supported ensure-steps to filesystem mode and uses stub mode for the rest.
func (d *DispatcherExecutor) Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	if isFilesystemStep(step.ID) {
		return d.filesystem.Execute(ctx, snapshot, step)
	}
	if step.ID == core.EnsureStepPythonRuntime && d.poetry != nil {
		return d.poetry.Execute(ctx, snapshot, step)
	}
	if (step.ID == core.EnsureStepPreprocessContract ||
		step.ID == core.EnsureStepModelAcquisition ||
		step.ID == core.EnsureStepModelContract ||
		step.ID == core.EnsureStepIntegrationTestWiring) && d.agent == nil {
		return core.ExecutionResult{}, core.NewError(
			core.KindMissingDependency,
			"execute.dispatcher.agent_required",
			"this authoring step requires Claude CLI (`claude`) to be installed and available on PATH",
		)
	}
	if d.agent != nil {
		result, err := d.agent.Execute(ctx, snapshot, step)
		if err == nil {
			return result, nil
		}
		if core.KindOf(err) != core.KindStepNotApplicable {
			return core.ExecutionResult{}, err
		}
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
