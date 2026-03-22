package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tensorleap/concierge/internal/adapters/planner"
	"github.com/tensorleap/concierge/internal/core"
)

type runHarness struct {
	stepSequence       []core.EnsureStepID
	failIteration      int
	iteration          int
	reportCount        int
	cancelAfterReports int
	cancel             context.CancelFunc
	executionEvidence  []core.EvidenceItem
	validation         core.ValidationResult
}

func newRunHarness(stepSequence []core.EnsureStepID) *runHarness {
	return &runHarness{stepSequence: stepSequence}
}

func (h *runHarness) Snapshot(ctx context.Context, request core.SnapshotRequest) (core.WorkspaceSnapshot, error) {
	_ = ctx
	_ = request
	h.iteration++
	return core.WorkspaceSnapshot{
		ID:         fmt.Sprintf("snapshot-%d", h.iteration),
		CapturedAt: time.Unix(1700000000, 0).UTC(),
		Repository: core.RepositoryState{
			Root:    "/repo",
			GitRoot: "/repo",
			Branch:  "feature/test",
			Head:    fmt.Sprintf("head-%d", h.iteration),
			Dirty:   false,
		},
	}, nil
}

func (h *runHarness) Inspect(ctx context.Context, snapshot core.WorkspaceSnapshot) (core.IntegrationStatus, error) {
	_ = ctx
	_ = snapshot
	return core.IntegrationStatus{}, nil
}

func (h *runHarness) Plan(ctx context.Context, snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) (core.ExecutionPlan, error) {
	_ = ctx
	_ = snapshot
	_ = status

	stepID := h.currentStepID()
	step, ok := core.EnsureStepByID(stepID)
	if !ok {
		return core.ExecutionPlan{}, core.NewError(core.KindStepNotApplicable, "run_test.plan", "unknown step "+string(stepID))
	}

	return core.ExecutionPlan{Primary: step}, nil
}

func (h *runHarness) Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	_ = ctx
	_ = snapshot
	if h.failIteration > 0 && h.iteration == h.failIteration {
		return core.ExecutionResult{}, errors.New("execute failed")
	}

	return core.ExecutionResult{
		Step:     step,
		Applied:  false,
		Summary:  "ok",
		Evidence: append([]core.EvidenceItem(nil), h.executionEvidence...),
	}, nil
}

func (h *runHarness) Validate(ctx context.Context, snapshot core.WorkspaceSnapshot, result core.ExecutionResult) (core.ValidationResult, error) {
	_ = ctx
	_ = snapshot
	_ = result
	if len(h.validation.Issues) == 0 {
		if !h.validation.Passed {
			return core.ValidationResult{Passed: true}, nil
		}
		return h.validation, nil
	}
	return h.validation, nil
}

func (h *runHarness) Report(ctx context.Context, report core.IterationReport) error {
	_ = ctx
	_ = report
	h.reportCount++
	if h.cancel != nil && h.cancelAfterReports > 0 && h.reportCount >= h.cancelAfterReports {
		h.cancel()
	}
	return nil
}

func (h *runHarness) currentStepID() core.EnsureStepID {
	if len(h.stepSequence) == 0 {
		return core.EnsureStepLeapYAML
	}

	index := h.iteration - 1
	if index < 0 {
		index = 0
	}
	if index >= len(h.stepSequence) {
		return h.stepSequence[len(h.stepSequence)-1]
	}

	return h.stepSequence[index]
}

func TestEngineRunTreatsMaxIterationsZeroAsUnlimited(t *testing.T) {
	harness := newRunHarness([]core.EnsureStepID{core.EnsureStepLeapYAML, core.EnsureStepComplete})
	engine := newRunTestEngine(t, harness)

	result, err := engine.Run(context.Background(), core.SnapshotRequest{RepoRoot: "/repo"}, RunOptions{MaxIterations: 0})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result.StopReason != RunStopReasonSuccess {
		t.Fatalf("expected stop reason %q, got %q", RunStopReasonSuccess, result.StopReason)
	}
	if len(result.Reports) != 2 {
		t.Fatalf("expected two reports, got %d", len(result.Reports))
	}
	if harness.iteration != 2 {
		t.Fatalf("expected two iterations, got %d", harness.iteration)
	}
}

func TestEngineRunStopsOnEnsureStepComplete(t *testing.T) {
	harness := newRunHarness([]core.EnsureStepID{
		core.EnsureStepLeapYAML,
		core.EnsureStepComplete,
		core.EnsureStepLeapYAML,
	})
	engine := newRunTestEngine(t, harness)

	result, err := engine.Run(context.Background(), core.SnapshotRequest{}, RunOptions{MaxIterations: 5})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result.StopReason != RunStopReasonSuccess {
		t.Fatalf("expected stop reason %q, got %q", RunStopReasonSuccess, result.StopReason)
	}
	if len(result.Reports) != 2 {
		t.Fatalf("expected two reports, got %d", len(result.Reports))
	}
	if result.Reports[1].Step.ID != core.EnsureStepComplete {
		t.Fatalf("expected second report step %q, got %q", core.EnsureStepComplete, result.Reports[1].Step.ID)
	}
}

func TestEngineRunStopsAtMaxIterations(t *testing.T) {
	harness := newRunHarness([]core.EnsureStepID{core.EnsureStepLeapYAML})
	engine := newRunTestEngine(t, harness)

	result, err := engine.Run(context.Background(), core.SnapshotRequest{}, RunOptions{MaxIterations: 2})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result.StopReason != RunStopReasonMaxIterations {
		t.Fatalf("expected stop reason %q, got %q", RunStopReasonMaxIterations, result.StopReason)
	}
	if len(result.Reports) != 2 {
		t.Fatalf("expected two reports, got %d", len(result.Reports))
	}
	if harness.iteration != 2 {
		t.Fatalf("expected two iterations, got %d", harness.iteration)
	}
}

func TestEngineRunPropagatesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	harness := newRunHarness([]core.EnsureStepID{core.EnsureStepLeapYAML})
	harness.cancel = cancel
	harness.cancelAfterReports = 1

	engine := newRunTestEngine(t, harness)

	result, err := engine.Run(ctx, core.SnapshotRequest{}, RunOptions{MaxIterations: 5})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
	if result.StopReason != RunStopReasonCancelled {
		t.Fatalf("expected stop reason %q, got %q", RunStopReasonCancelled, result.StopReason)
	}
	if len(result.Reports) != 1 {
		t.Fatalf("expected one completed report before cancellation, got %d", len(result.Reports))
	}
}

func TestEngineRunReturnsErrorWhenRunIterationErrors(t *testing.T) {
	harness := newRunHarness([]core.EnsureStepID{core.EnsureStepLeapYAML})
	harness.failIteration = 2
	engine := newRunTestEngine(t, harness)

	result, err := engine.Run(context.Background(), core.SnapshotRequest{}, RunOptions{MaxIterations: 5})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var stageErr *StageError
	if !errors.As(err, &stageErr) {
		t.Fatalf("expected stage error, got %T (%v)", err, err)
	}
	if stageErr.Stage != core.StageExecute {
		t.Fatalf("expected stage %q, got %q", core.StageExecute, stageErr.Stage)
	}
	if len(result.Reports) != 1 {
		t.Fatalf("expected one successful report before error, got %d", len(result.Reports))
	}
}

func TestEngineRunStopsWhenManualUserActionIsRequired(t *testing.T) {
	harness := newRunHarness([]core.EnsureStepID{core.EnsureStepPythonRuntime, core.EnsureStepComplete})
	harness.executionEvidence = []core.EvidenceItem{
		{Name: "executor.mode", Value: "self_service"},
	}
	engine := newRunTestEngine(t, harness)

	result, err := engine.Run(context.Background(), core.SnapshotRequest{}, RunOptions{MaxIterations: 5})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result.StopReason != RunStopReasonNeedsUserAction {
		t.Fatalf("expected stop reason %q, got %q", RunStopReasonNeedsUserAction, result.StopReason)
	}
	if len(result.Reports) != 1 {
		t.Fatalf("expected one report, got %d", len(result.Reports))
	}
}

func TestEngineRunStopsWhenStepApprovalIsRejected(t *testing.T) {
	harness := newRunHarness([]core.EnsureStepID{core.EnsureStepPreprocessContract, core.EnsureStepComplete})
	harness.executionEvidence = []core.EvidenceItem{
		{Name: "executor.mode", Value: "approval_gate"},
		{Name: "executor.change_approval", Value: "rejected"},
	}
	engine := newRunTestEngine(t, harness)

	result, err := engine.Run(context.Background(), core.SnapshotRequest{}, RunOptions{MaxIterations: 5})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result.StopReason != RunStopReasonNeedsUserAction {
		t.Fatalf("expected stop reason %q, got %q", RunStopReasonNeedsUserAction, result.StopReason)
	}
	if len(result.Reports) != 1 {
		t.Fatalf("expected one report, got %d", len(result.Reports))
	}
}

func TestEngineRunStopsWhenValidationRequiresManualUserAction(t *testing.T) {
	harness := newRunHarness([]core.EnsureStepID{core.EnsureStepPreprocessContract, core.EnsureStepComplete})
	harness.validation = core.ValidationResult{
		Passed: false,
		Issues: []core.Issue{
			{
				Code:     core.IssueCodeNativeSystemDependencyMissing,
				Message:  "the current Python environment is missing native system library `libGL.so.1`, so importing integration dependencies failed during Tensorleap parser validation",
				Severity: core.SeverityError,
				Scope:    core.IssueScopeEnvironment,
			},
		},
	}
	engine := newRunTestEngine(t, harness)

	result, err := engine.Run(context.Background(), core.SnapshotRequest{}, RunOptions{MaxIterations: 5})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result.StopReason != RunStopReasonNeedsUserAction {
		t.Fatalf("expected stop reason %q, got %q", RunStopReasonNeedsUserAction, result.StopReason)
	}
	if len(result.Reports) != 1 {
		t.Fatalf("expected one report, got %d", len(result.Reports))
	}
}

type carryoverValidationHarness struct {
	iteration   int
	statuses    []core.IntegrationStatus
	validations []core.ValidationResult
	reports     []core.IterationReport
}

func (h *carryoverValidationHarness) Snapshot(ctx context.Context, request core.SnapshotRequest) (core.WorkspaceSnapshot, error) {
	_ = ctx
	_ = request
	h.iteration++
	return core.WorkspaceSnapshot{
		ID: fmt.Sprintf("snapshot-%d", h.iteration),
		Repository: core.RepositoryState{
			Root:    "/repo",
			GitRoot: "/repo",
			Branch:  "feature/test",
			Head:    fmt.Sprintf("head-%d", h.iteration),
		},
	}, nil
}

func (h *carryoverValidationHarness) Inspect(ctx context.Context, snapshot core.WorkspaceSnapshot) (core.IntegrationStatus, error) {
	_ = ctx
	_ = snapshot
	index := h.iteration - 1
	if index < 0 {
		index = 0
	}
	if index >= len(h.statuses) {
		index = len(h.statuses) - 1
	}
	return h.statuses[index], nil
}

func (h *carryoverValidationHarness) Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	_ = ctx
	_ = snapshot
	return core.ExecutionResult{
		Step:    step,
		Applied: false,
		Summary: "ok",
	}, nil
}

func (h *carryoverValidationHarness) Validate(ctx context.Context, snapshot core.WorkspaceSnapshot, result core.ExecutionResult) (core.ValidationResult, error) {
	_ = ctx
	_ = snapshot
	_ = result
	index := h.iteration - 1
	if index < 0 {
		index = 0
	}
	if index >= len(h.validations) {
		index = len(h.validations) - 1
	}
	return h.validations[index], nil
}

func (h *carryoverValidationHarness) Report(ctx context.Context, report core.IterationReport) error {
	_ = ctx
	h.reports = append(h.reports, report)
	return nil
}

func TestEngineRunCarriesBlockingValidationIssuesIntoNextPlanningRound(t *testing.T) {
	harness := &carryoverValidationHarness{
		statuses: []core.IntegrationStatus{
			{
				Issues: []core.Issue{{
					Code:     core.IssueCodePreprocessFunctionMissing,
					Message:  "no @tensorleap_preprocess function found in leap_integration.py",
					Severity: core.SeverityError,
					Scope:    core.IssueScopePreprocess,
				}},
			},
			{
				Issues: []core.Issue{{
					Code:     core.IssueCodeLoadModelDecoratorMissing,
					Message:  "no @tensorleap_load_model function found in leap_integration.py",
					Severity: core.SeverityError,
					Scope:    core.IssueScopeModel,
				}},
			},
		},
		validations: []core.ValidationResult{
			{
				Passed: false,
				Issues: []core.Issue{{
					Code:     core.IssueCodePreprocessExecutionFailed,
					Message:  "preprocess failed during Tensorleap parser validation: Invalid dataset length",
					Severity: core.SeverityError,
					Scope:    core.IssueScopePreprocess,
				}},
			},
			{Passed: true},
		},
	}

	engine, err := NewEngine(Dependencies{
		Snapshotter: harness,
		Inspector:   harness,
		Planner:     planner.NewDeterministicPlanner(),
		Executor:    harness,
		Validator:   harness,
		Reporter:    harness,
	})
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	result, err := engine.Run(context.Background(), core.SnapshotRequest{RepoRoot: "/repo"}, RunOptions{MaxIterations: 2})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(result.Reports) != 2 {
		t.Fatalf("expected two reports, got %d", len(result.Reports))
	}
	if got := result.Reports[0].Step.ID; got != core.EnsureStepPreprocessContract {
		t.Fatalf("expected first step %q, got %q", core.EnsureStepPreprocessContract, got)
	}
	if got := result.Reports[1].Step.ID; got != core.EnsureStepPreprocessContract {
		t.Fatalf("expected second step to stay on %q after blocking validation failure, got %q", core.EnsureStepPreprocessContract, got)
	}
}

func TestEngineRunSeedsFirstPlanningRoundWithInitialBlockingIssues(t *testing.T) {
	harness := &carryoverValidationHarness{
		statuses: []core.IntegrationStatus{
			{
				Issues: []core.Issue{{
					Code:     core.IssueCodeLoadModelDecoratorMissing,
					Message:  "no @tensorleap_load_model function found in leap_integration.py",
					Severity: core.SeverityError,
					Scope:    core.IssueScopeModel,
				}},
			},
		},
		validations: []core.ValidationResult{{Passed: true}},
	}

	engine, err := NewEngine(Dependencies{
		Snapshotter: harness,
		Inspector:   harness,
		Planner:     planner.NewDeterministicPlanner(),
		Executor:    harness,
		Validator:   harness,
		Reporter:    harness,
	})
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	initialResolved := 0
	result, err := engine.Run(context.Background(), core.SnapshotRequest{RepoRoot: "/repo"}, RunOptions{
		MaxIterations: 1,
		InitialBlockingIssues: func(snapshot core.WorkspaceSnapshot) []core.Issue {
			initialResolved++
			if snapshot.ID != "snapshot-1" {
				t.Fatalf("expected first snapshot to seed initial blocking issues, got %q", snapshot.ID)
			}
			return []core.Issue{{
				Code:     core.IssueCodePreprocessExecutionFailed,
				Message:  "preprocess failed during Tensorleap parser validation: length is deprecated",
				Severity: core.SeverityError,
				Scope:    core.IssueScopePreprocess,
			}}
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if initialResolved != 1 {
		t.Fatalf("expected initial blocking issues resolver to run once, got %d", initialResolved)
	}
	if len(result.Reports) != 1 {
		t.Fatalf("expected one report, got %d", len(result.Reports))
	}
	if got := result.Reports[0].Step.ID; got != core.EnsureStepPreprocessContract {
		t.Fatalf("expected first step %q after seeding initial blockers, got %q", core.EnsureStepPreprocessContract, got)
	}
}

func TestEngineRunStopsOnNoProgress(t *testing.T) {
	// Simulate a step that is always selected but never applies changes.
	harness := newRunHarness([]core.EnsureStepID{core.EnsureStepIntegrationTestContract})
	engine := newRunTestEngine(t, harness)

	result, err := engine.Run(context.Background(), core.SnapshotRequest{}, RunOptions{MaxIterations: 10})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result.StopReason != RunStopReasonNoProgress {
		t.Fatalf("expected stop reason %q, got %q", RunStopReasonNoProgress, result.StopReason)
	}
	// First iteration sets lastStepID; 2 more consecutive no-progress → 3 total.
	if len(result.Reports) != 3 {
		t.Fatalf("expected 3 reports (1 initial + 2 consecutive no-progress), got %d", len(result.Reports))
	}
}

func TestEngineRunResetsNoProgressOnDifferentStep(t *testing.T) {
	// Alternating steps should not trigger no-progress detection.
	harness := newRunHarness([]core.EnsureStepID{
		core.EnsureStepLeapYAML,
		core.EnsureStepIntegrationScript,
		core.EnsureStepLeapYAML,
		core.EnsureStepIntegrationScript,
		core.EnsureStepComplete,
	})
	engine := newRunTestEngine(t, harness)

	result, err := engine.Run(context.Background(), core.SnapshotRequest{}, RunOptions{MaxIterations: 10})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result.StopReason != RunStopReasonSuccess {
		t.Fatalf("expected stop reason %q, got %q", RunStopReasonSuccess, result.StopReason)
	}
	if len(result.Reports) != 5 {
		t.Fatalf("expected 5 reports, got %d", len(result.Reports))
	}
}

func newRunTestEngine(t *testing.T, harness *runHarness) *Engine {
	t.Helper()

	engine, err := NewEngine(Dependencies{
		Snapshotter: harness,
		Inspector:   harness,
		Planner:     harness,
		Executor:    harness,
		Validator:   harness,
		Reporter:    harness,
		Clock: func() time.Time {
			return time.Unix(1700000000, 0).UTC()
		},
	})
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	return engine
}
