package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/core/ports"
)

// Dependencies contains the stage adapters required to run one iteration.
type Dependencies struct {
	Snapshotter ports.Snapshotter
	Inspector   ports.Inspector
	Planner     ports.Planner
	Executor    ports.Executor
	Validator   ports.Validator
	Reporter    ports.Reporter
	Clock       func() time.Time
}

// Engine executes one deterministic orchestration iteration.
type Engine struct {
	snapshotter ports.Snapshotter
	inspector   ports.Inspector
	planner     ports.Planner
	executor    ports.Executor
	validator   ports.Validator
	reporter    ports.Reporter
	clock       func() time.Time
}

// NewEngine validates dependencies and returns a ready-to-run engine.
func NewEngine(deps Dependencies) (*Engine, error) {
	if deps.Snapshotter == nil {
		return nil, missingDependencyError("snapshotter")
	}
	if deps.Inspector == nil {
		return nil, missingDependencyError("inspector")
	}
	if deps.Planner == nil {
		return nil, missingDependencyError("planner")
	}
	if deps.Executor == nil {
		return nil, missingDependencyError("executor")
	}
	if deps.Validator == nil {
		return nil, missingDependencyError("validator")
	}
	if deps.Reporter == nil {
		return nil, missingDependencyError("reporter")
	}

	clock := deps.Clock
	if clock == nil {
		clock = func() time.Time {
			return time.Now().UTC()
		}
	}

	return &Engine{
		snapshotter: deps.Snapshotter,
		inspector:   deps.Inspector,
		planner:     deps.Planner,
		executor:    deps.Executor,
		validator:   deps.Validator,
		reporter:    deps.Reporter,
		clock:       clock,
	}, nil
}

// RunIteration executes the canonical stage sequence for one orchestration loop.
func (e *Engine) RunIteration(ctx context.Context, req core.SnapshotRequest) (core.IterationReport, error) {
	snapshot, err := e.snapshotter.Snapshot(ctx, req)
	if err != nil {
		return core.IterationReport{}, &StageError{Stage: core.StageSnapshot, Err: err}
	}

	status, err := e.inspector.Inspect(ctx, snapshot)
	if err != nil {
		return core.IterationReport{}, &StageError{Stage: core.StageInspect, Err: err}
	}

	plan, err := e.planner.Plan(ctx, snapshot, status)
	if err != nil {
		return core.IterationReport{}, &StageError{Stage: core.StagePlan, Err: err}
	}

	result, err := e.executor.Execute(ctx, snapshot, plan.Primary)
	if err != nil {
		return core.IterationReport{}, &StageError{Stage: core.StageExecute, Err: err}
	}

	validation, err := e.validator.Validate(ctx, snapshot, result)
	if err != nil {
		return core.IterationReport{}, &StageError{Stage: core.StageValidate, Err: err}
	}

	report := core.IterationReport{
		GeneratedAt: e.clock(),
		SnapshotID:  snapshot.ID,
		Step:        result.Step,
		Validation:  validation,
	}

	if err := e.reporter.Report(ctx, report); err != nil {
		return core.IterationReport{}, &StageError{Stage: core.StageReport, Err: err}
	}

	return report, nil
}

func missingDependencyError(name string) error {
	return core.WrapError(
		core.KindMissingDependency,
		"orchestrator.new."+name,
		errors.New(name+" is required"),
	)
}
