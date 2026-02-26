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
	GitManager  ports.GitManager
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
	gitManager  ports.GitManager
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
		gitManager:  deps.GitManager,
		validator:   deps.Validator,
		reporter:    deps.Reporter,
		clock:       clock,
	}, nil
}

// RunIteration executes the canonical stage sequence for one orchestration loop.
func (e *Engine) RunIteration(ctx context.Context, req core.SnapshotRequest) (core.IterationReport, error) {
	report, _, err := e.runIteration(ctx, req, nil)
	return report, err
}

func (e *Engine) runIteration(
	ctx context.Context,
	req core.SnapshotRequest,
	beforeReport func(snapshot core.WorkspaceSnapshot, report *core.IterationReport) error,
) (core.IterationReport, core.WorkspaceSnapshot, error) {
	snapshot, err := e.snapshotter.Snapshot(ctx, req)
	if err != nil {
		return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageSnapshot, Err: err}
	}

	status, err := e.inspector.Inspect(ctx, snapshot)
	if err != nil {
		return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageInspect, Err: err}
	}

	plan, err := e.planner.Plan(ctx, snapshot, status)
	if err != nil {
		return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StagePlan, Err: err}
	}

	result, err := e.executor.Execute(ctx, snapshot, plan.Primary)
	if err != nil {
		return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageExecute, Err: err}
	}

	decision := core.GitDecision{FinalResult: result}
	if e.gitManager != nil {
		decision, err = e.gitManager.Handle(ctx, snapshot, result)
		if err != nil {
			return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageExecute, Err: err}
		}
		if decision.FinalResult.Step.ID == "" {
			decision.FinalResult = result
		}
	}

	finalResult := decision.FinalResult

	validation, err := e.validator.Validate(ctx, snapshot, finalResult)
	if err != nil {
		return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageValidate, Err: err}
	}

	evidence := append([]core.EvidenceItem(nil), finalResult.Evidence...)
	evidence = append(evidence, decision.Evidence...)

	report := core.IterationReport{
		GeneratedAt: e.clock(),
		SnapshotID:  snapshot.ID,
		Step:        finalResult.Step,
		Applied:     finalResult.Applied,
		Evidence:    evidence,
		Validation:  validation,
		Commit:      decision.Commit,
		Notes:       append([]string(nil), decision.Notes...),
	}

	if beforeReport != nil {
		if err := beforeReport(snapshot, &report); err != nil {
			return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageReport, Err: err}
		}
	}

	if err := e.reporter.Report(ctx, report); err != nil {
		return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageReport, Err: err}
	}

	return report, snapshot, nil
}

func missingDependencyError(name string) error {
	return core.WrapError(
		core.KindMissingDependency,
		"orchestrator.new."+name,
		errors.New(name+" is required"),
	)
}
