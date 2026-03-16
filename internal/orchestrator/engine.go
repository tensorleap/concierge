package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/core/ports"
	"github.com/tensorleap/concierge/internal/observe"
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
	Observer    observe.Sink
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
	observer    observe.Sink
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
		observer:    deps.Observer,
		clock:       clock,
	}, nil
}

// RunIteration executes the canonical stage sequence for one orchestration loop.
func (e *Engine) RunIteration(ctx context.Context, req core.SnapshotRequest) (core.IterationReport, error) {
	report, _, err := e.runIteration(ctx, req, 1, nil)
	return report, err
}

func (e *Engine) runIteration(
	ctx context.Context,
	req core.SnapshotRequest,
	iteration int,
	beforeReport func(snapshot core.WorkspaceSnapshot, report *core.IterationReport) error,
) (core.IterationReport, core.WorkspaceSnapshot, error) {
	e.emit(observe.Event{Kind: observe.EventIterationStarted, Iteration: iteration, Message: "Starting a new guided round"})
	e.emit(observe.Event{Kind: observe.EventStageStarted, Iteration: iteration, Stage: core.StageSnapshot, Message: "Capturing workspace snapshot"})
	snapshot, err := e.snapshotter.Snapshot(ctx, req)
	if err != nil {
		e.emit(observe.Event{Kind: observe.EventError, Iteration: iteration, Stage: core.StageSnapshot, Message: err.Error()})
		return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageSnapshot, Err: err}
	}
	e.emit(observe.Event{Kind: observe.EventStageFinished, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StageSnapshot, Message: "Workspace snapshot captured"})

	e.emit(observe.Event{Kind: observe.EventStageStarted, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StageInspect, Message: "Inspecting Tensorleap artifacts"})
	status, err := e.inspector.Inspect(ctx, snapshot)
	if err != nil {
		e.emit(observe.Event{Kind: observe.EventError, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StageInspect, Message: err.Error()})
		return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageInspect, Err: err}
	}
	e.emit(observe.Event{Kind: observe.EventStageFinished, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StageInspect, Message: "Inspection finished"})

	e.emit(observe.Event{Kind: observe.EventStageStarted, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StagePlan, Message: "Choosing the next step"})
	plan, err := e.planner.Plan(ctx, snapshot, status)
	if err != nil {
		e.emit(observe.Event{Kind: observe.EventError, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StagePlan, Message: err.Error()})
		return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StagePlan, Err: err}
	}
	e.emit(observe.Event{Kind: observe.EventStageFinished, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StagePlan, Message: "Selected the next step"})
	e.emit(observe.Event{Kind: observe.EventStepSelected, Iteration: iteration, SnapshotID: snapshot.ID, StepID: plan.Primary.ID, Message: "Working on: " + core.HumanEnsureStepLabel(plan.Primary.ID)})

	e.emit(observe.Event{Kind: observe.EventStageStarted, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StageExecute, StepID: plan.Primary.ID, Message: "Working through the selected step"})
	result, err := e.executor.Execute(ctx, snapshot, plan.Primary)
	if err != nil {
		e.emit(observe.Event{Kind: observe.EventError, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StageExecute, StepID: plan.Primary.ID, Message: err.Error()})
		return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageExecute, Err: err}
	}

	decision := core.GitDecision{FinalResult: result}
	if e.gitManager != nil {
		decision, err = e.gitManager.Handle(ctx, snapshot, result)
		if err != nil {
			e.emit(observe.Event{Kind: observe.EventError, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StageExecute, StepID: plan.Primary.ID, Message: err.Error()})
			return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageExecute, Err: err}
		}
		if decision.FinalResult.Step.ID == "" {
			decision.FinalResult = result
		}
	}
	e.emit(observe.Event{Kind: observe.EventStageFinished, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StageExecute, StepID: plan.Primary.ID, Message: "Execution finished"})

	finalResult := decision.FinalResult
	postSnapshot := snapshot
	postStatus := status
	if shouldRefreshPostExecutionState(finalResult, decision) {
		postSnapshot, postStatus, err = e.refreshPostExecutionState(ctx, req, iteration, finalResult.Step.ID)
		if err != nil {
			return core.IterationReport{}, core.WorkspaceSnapshot{}, err
		}
	}
	validationStartedMessage := "Validating runtime behavior"
	validationFinishedMessage := "Validation finished"
	reportStartedMessage := "Writing the run report"
	if executionRequiresUserAction(finalResult) {
		validationStartedMessage = "Confirming the manual next step"
		validationFinishedMessage = "Manual next step confirmed"
		reportStartedMessage = "Writing the blocker summary"
	}

	e.emit(observe.Event{Kind: observe.EventValidationStarted, Iteration: iteration, SnapshotID: postSnapshot.ID, Stage: core.StageValidate, StepID: finalResult.Step.ID, Message: validationStartedMessage})
	validation, err := e.validator.Validate(ctx, postSnapshot, finalResult)
	if err != nil {
		e.emit(observe.Event{Kind: observe.EventError, Iteration: iteration, SnapshotID: postSnapshot.ID, Stage: core.StageValidate, StepID: finalResult.Step.ID, Message: err.Error()})
		return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageValidate, Err: err}
	}
	validation = mergeBlockingInspectIssues(validation, postStatus)
	e.emit(observe.Event{Kind: observe.EventValidationFinished, Iteration: iteration, SnapshotID: postSnapshot.ID, Stage: core.StageValidate, StepID: finalResult.Step.ID, Message: validationFinishedMessage})

	evidence := append([]core.EvidenceItem(nil), finalResult.Evidence...)
	evidence = append(evidence, decision.Evidence...)
	evidence = append(evidence, validation.Evidence...)

	report := core.IterationReport{
		GeneratedAt:     e.clock(),
		SnapshotID:      postSnapshot.ID,
		Step:            finalResult.Step,
		Applied:         finalResult.Applied,
		Evidence:        evidence,
		Recommendations: append([]core.AuthoringRecommendation(nil), finalResult.Recommendations...),
		Checks:          core.BuildVerifiedChecks(postSnapshot, postStatus.Issues, validation.Issues, finalResult.Step.ID),
		Validation:      validation,
		Commit:          decision.Commit,
		Notes:           append([]string(nil), decision.Notes...),
	}

	if beforeReport != nil {
		if err := beforeReport(postSnapshot, &report); err != nil {
			e.emit(observe.Event{Kind: observe.EventError, Iteration: iteration, SnapshotID: postSnapshot.ID, Stage: core.StageReport, StepID: finalResult.Step.ID, Message: err.Error()})
			return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageReport, Err: err}
		}
	}

	e.emit(observe.Event{Kind: observe.EventStageStarted, Iteration: iteration, SnapshotID: postSnapshot.ID, Stage: core.StageReport, StepID: finalResult.Step.ID, Message: reportStartedMessage})
	if err := e.reporter.Report(ctx, report); err != nil {
		e.emit(observe.Event{Kind: observe.EventError, Iteration: iteration, SnapshotID: postSnapshot.ID, Stage: core.StageReport, StepID: finalResult.Step.ID, Message: err.Error()})
		return core.IterationReport{}, core.WorkspaceSnapshot{}, &StageError{Stage: core.StageReport, Err: err}
	}
	e.emit(observe.Event{Kind: observe.EventStageFinished, Iteration: iteration, SnapshotID: postSnapshot.ID, Stage: core.StageReport, StepID: finalResult.Step.ID, Message: "Run report written"})
	e.emit(observe.Event{Kind: observe.EventIterationFinished, Iteration: iteration, SnapshotID: postSnapshot.ID, StepID: finalResult.Step.ID, Message: "Guided round finished"})

	return report, postSnapshot, nil
}

func (e *Engine) emit(event observe.Event) {
	if e == nil || e.observer == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = e.clock()
	}
	e.observer.Emit(event)
}

func mergeBlockingInspectIssues(validation core.ValidationResult, status core.IntegrationStatus) core.ValidationResult {
	merged := validation
	if len(status.Issues) == 0 {
		return merged
	}

	seen := make(map[string]struct{}, len(merged.Issues))
	for _, issue := range merged.Issues {
		key := string(issue.Code) + "|" + issue.Message + "|" + string(issue.Scope)
		seen[key] = struct{}{}
	}

	for _, issue := range status.Issues {
		if issue.Severity != core.SeverityError {
			continue
		}
		key := string(issue.Code) + "|" + issue.Message + "|" + string(issue.Scope)
		if _, exists := seen[key]; exists {
			continue
		}
		merged.Issues = append(merged.Issues, issue)
		seen[key] = struct{}{}
	}

	merged.Passed = true
	for _, issue := range merged.Issues {
		if issue.Severity == core.SeverityError {
			merged.Passed = false
			break
		}
	}

	return merged
}

func (e *Engine) refreshPostExecutionState(
	ctx context.Context,
	req core.SnapshotRequest,
	iteration int,
	stepID core.EnsureStepID,
) (core.WorkspaceSnapshot, core.IntegrationStatus, error) {
	e.emit(observe.Event{Kind: observe.EventStageStarted, Iteration: iteration, Stage: core.StageSnapshot, StepID: stepID, Message: "Refreshing workspace snapshot after changes"})
	snapshot, err := e.snapshotter.Snapshot(ctx, req)
	if err != nil {
		e.emit(observe.Event{Kind: observe.EventError, Iteration: iteration, Stage: core.StageSnapshot, StepID: stepID, Message: err.Error()})
		return core.WorkspaceSnapshot{}, core.IntegrationStatus{}, &StageError{Stage: core.StageSnapshot, Err: err}
	}
	e.emit(observe.Event{Kind: observe.EventStageFinished, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StageSnapshot, StepID: stepID, Message: "Post-change workspace snapshot captured"})

	e.emit(observe.Event{Kind: observe.EventStageStarted, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StageInspect, StepID: stepID, Message: "Re-inspecting changed workspace"})
	status, err := e.inspector.Inspect(ctx, snapshot)
	if err != nil {
		e.emit(observe.Event{Kind: observe.EventError, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StageInspect, StepID: stepID, Message: err.Error()})
		return core.WorkspaceSnapshot{}, core.IntegrationStatus{}, &StageError{Stage: core.StageInspect, Err: err}
	}
	e.emit(observe.Event{Kind: observe.EventStageFinished, Iteration: iteration, SnapshotID: snapshot.ID, Stage: core.StageInspect, StepID: stepID, Message: "Post-change inspection finished"})
	return snapshot, status, nil
}

func shouldRefreshPostExecutionState(result core.ExecutionResult, decision core.GitDecision) bool {
	if result.Applied {
		return true
	}
	return decision.Commit != nil
}

func executionRequiresUserAction(result core.ExecutionResult) bool {
	for _, item := range result.Evidence {
		if item.Name == "executor.mode" && item.Value == "self_service" {
			return true
		}
	}
	return false
}

func missingDependencyError(name string) error {
	return core.WrapError(
		core.KindMissingDependency,
		"orchestrator.new."+name,
		errors.New(name+" is required"),
	)
}
