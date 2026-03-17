package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

type stageHarness struct {
	calls []core.Stage
	fail  map[core.Stage]error

	snapshot   core.WorkspaceSnapshot
	status     core.IntegrationStatus
	plan       core.ExecutionPlan
	result     core.ExecutionResult
	validation core.ValidationResult
	reported   core.IterationReport
}

func newStageHarness() *stageHarness {
	step := core.EnsureStep{
		ID:          core.EnsureStepLeapYAML,
		Description: "Ensure leap.yaml exists and satisfies upload boundary contract",
	}

	return &stageHarness{
		fail: make(map[core.Stage]error),
		snapshot: core.WorkspaceSnapshot{
			ID:         "snapshot-1",
			CapturedAt: time.Unix(1700000000, 0).UTC(),
			Repository: core.RepositoryState{
				Root:    "/repo",
				GitRoot: "/repo",
				Branch:  "main",
				Head:    "abc123",
				Dirty:   false,
			},
		},
		status: core.IntegrationStatus{
			Missing: []string{"leap.yaml"},
			Issues: []core.Issue{
				{
					Code:     core.IssueCodeLeapYAMLMissing,
					Message:  "leap.yaml is required",
					Severity: core.SeverityError,
					Scope:    core.IssueScopeLeapYAML,
				},
			},
		},
		plan: core.ExecutionPlan{
			Primary: step,
		},
		result: core.ExecutionResult{
			Step:    step,
			Applied: false,
			Summary: "ok",
		},
		validation: core.ValidationResult{
			Passed: true,
		},
	}
}

type refreshStageHarness struct {
	*stageHarness

	snapshots     []core.WorkspaceSnapshot
	statuses      []core.IntegrationStatus
	snapshotCalls int
	inspectCalls  int
}

func newRefreshStageHarness() *refreshStageHarness {
	step := core.EnsureStep{
		ID:          core.EnsureStepModelContract,
		Description: "Ensure @tensorleap_load_model is wired to a supported model artifact",
	}

	preSnapshot := core.WorkspaceSnapshot{
		ID:                  "snapshot-before-model-loader",
		CapturedAt:          time.Unix(1700000000, 0).UTC(),
		WorktreeFingerprint: "before-load-model",
		Repository: core.RepositoryState{
			Root:    "/repo",
			GitRoot: "/repo",
			Branch:  "feature/step-QA1-qa-text-output",
			Head:    "abc123",
			Dirty:   false,
		},
		FileHashes: map[string]string{
			"leap.yaml":           "leap-yaml-before",
			"leap_integration.py": "integration-before",
		},
	}
	postSnapshot := preSnapshot
	postSnapshot.ID = "snapshot-after-model-loader"
	postSnapshot.WorktreeFingerprint = "after-load-model"
	postSnapshot.FileHashes = map[string]string{
		"leap.yaml":           "leap-yaml-before",
		"leap_integration.py": "integration-after",
	}

	preStatus := core.IntegrationStatus{
		Issues: []core.Issue{
			{
				Code:     core.IssueCodeLoadModelDecoratorMissing,
				Message:  "no @tensorleap_load_model function found in leap_integration.py",
				Severity: core.SeverityError,
				Scope:    core.IssueScopeModel,
			},
		},
		Contracts: &core.IntegrationContracts{
			EntryFile: "leap_integration.py",
		},
	}
	postStatus := core.IntegrationStatus{
		Issues: []core.Issue{
			{
				Code:     core.IssueCodeModelFileMissing,
				Message:  `model file "yolo11s.onnx" was not found`,
				Severity: core.SeverityError,
				Scope:    core.IssueScopeModel,
			},
		},
		Contracts: &core.IntegrationContracts{
			EntryFile:          "leap_integration.py",
			LoadModelFunctions: []string{"load_model"},
		},
	}

	return &refreshStageHarness{
		stageHarness: &stageHarness{
			fail:     make(map[core.Stage]error),
			snapshot: preSnapshot,
			status:   preStatus,
			plan: core.ExecutionPlan{
				Primary: step,
			},
			result: core.ExecutionResult{
				Step:    step,
				Applied: true,
				Summary: "agent task completed",
			},
			validation: core.ValidationResult{
				Passed: true,
			},
		},
		snapshots: []core.WorkspaceSnapshot{preSnapshot, postSnapshot},
		statuses:  []core.IntegrationStatus{preStatus, postStatus},
	}
}

func (h *refreshStageHarness) Snapshot(ctx context.Context, request core.SnapshotRequest) (core.WorkspaceSnapshot, error) {
	_ = ctx
	_ = request
	h.calls = append(h.calls, core.StageSnapshot)
	if err := h.fail[core.StageSnapshot]; err != nil {
		return core.WorkspaceSnapshot{}, err
	}
	index := h.snapshotCalls
	if index >= len(h.snapshots) {
		index = len(h.snapshots) - 1
	}
	h.snapshotCalls++
	return h.snapshots[index], nil
}

func (h *refreshStageHarness) Inspect(ctx context.Context, snapshot core.WorkspaceSnapshot) (core.IntegrationStatus, error) {
	_ = ctx
	_ = snapshot
	h.calls = append(h.calls, core.StageInspect)
	if err := h.fail[core.StageInspect]; err != nil {
		return core.IntegrationStatus{}, err
	}
	index := h.inspectCalls
	if index >= len(h.statuses) {
		index = len(h.statuses) - 1
	}
	h.inspectCalls++
	return h.statuses[index], nil
}

func (h *stageHarness) Snapshot(ctx context.Context, request core.SnapshotRequest) (core.WorkspaceSnapshot, error) {
	_ = ctx
	_ = request
	h.calls = append(h.calls, core.StageSnapshot)
	if err := h.fail[core.StageSnapshot]; err != nil {
		return core.WorkspaceSnapshot{}, err
	}
	return h.snapshot, nil
}

func (h *stageHarness) Inspect(ctx context.Context, snapshot core.WorkspaceSnapshot) (core.IntegrationStatus, error) {
	_ = ctx
	_ = snapshot
	h.calls = append(h.calls, core.StageInspect)
	if err := h.fail[core.StageInspect]; err != nil {
		return core.IntegrationStatus{}, err
	}
	return h.status, nil
}

func (h *stageHarness) Plan(ctx context.Context, snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) (core.ExecutionPlan, error) {
	_ = ctx
	_ = snapshot
	_ = status
	h.calls = append(h.calls, core.StagePlan)
	if err := h.fail[core.StagePlan]; err != nil {
		return core.ExecutionPlan{}, err
	}
	return h.plan, nil
}

func (h *stageHarness) Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	_ = ctx
	_ = snapshot
	_ = step
	h.calls = append(h.calls, core.StageExecute)
	if err := h.fail[core.StageExecute]; err != nil {
		return core.ExecutionResult{}, err
	}
	return h.result, nil
}

func (h *stageHarness) Validate(ctx context.Context, snapshot core.WorkspaceSnapshot, result core.ExecutionResult) (core.ValidationResult, error) {
	_ = ctx
	_ = snapshot
	_ = result
	h.calls = append(h.calls, core.StageValidate)
	if err := h.fail[core.StageValidate]; err != nil {
		return core.ValidationResult{}, err
	}
	return h.validation, nil
}

func (h *stageHarness) Report(ctx context.Context, report core.IterationReport) error {
	_ = ctx
	h.calls = append(h.calls, core.StageReport)
	if err := h.fail[core.StageReport]; err != nil {
		return err
	}
	h.reported = report
	return nil
}

func TestRunIterationSuccessCallsStagesInOrder(t *testing.T) {
	fixedNow := time.Date(2026, 2, 25, 14, 30, 0, 0, time.UTC)
	harness := newStageHarness()
	engine := mustNewEngine(t, Dependencies{
		Snapshotter: harness,
		Inspector:   harness,
		Planner:     harness,
		Executor:    harness,
		Validator:   harness,
		Reporter:    harness,
		Clock: func() time.Time {
			return fixedNow
		},
	})

	report, err := engine.RunIteration(context.Background(), core.SnapshotRequest{RepoRoot: "/repo"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	expectedStages := core.DefaultStages()
	if !reflect.DeepEqual(expectedStages, harness.calls) {
		t.Fatalf("expected stage calls %v, got %v", expectedStages, harness.calls)
	}

	if !report.GeneratedAt.Equal(fixedNow) {
		t.Fatalf("expected generatedAt %s, got %s", fixedNow, report.GeneratedAt)
	}
	if report.SnapshotID != harness.snapshot.ID {
		t.Fatalf("expected snapshot ID %q, got %q", harness.snapshot.ID, report.SnapshotID)
	}
	if report.Step.ID != harness.result.Step.ID {
		t.Fatalf("expected report step %q, got %q", harness.result.Step.ID, report.Step.ID)
	}
	if len(report.Checks) == 0 {
		t.Fatal("expected verified checks to be populated")
	}
	leapCheck := core.VerifiedCheck{}
	foundLeapCheck := false
	for _, check := range report.Checks {
		if check.StepID == core.EnsureStepLeapYAML {
			leapCheck = check
			foundLeapCheck = true
			break
		}
	}
	if !foundLeapCheck {
		t.Fatalf("expected %q check row in report", core.EnsureStepLeapYAML)
	}
	if leapCheck.Status != core.CheckStatusFail {
		t.Fatalf("expected leap.yaml check to fail, got %q", leapCheck.Status)
	}
	if !leapCheck.Blocking {
		t.Fatal("expected leap.yaml check to be marked blocking")
	}
	expectedValidation := mergeBlockingInspectIssues(harness.validation, harness.status)
	if !reflect.DeepEqual(report.Validation, expectedValidation) {
		t.Fatalf("expected validation %+v, got %+v", expectedValidation, report.Validation)
	}
	if !reflect.DeepEqual(report, harness.reported) {
		t.Fatalf("expected reporter to receive returned report, got %+v want %+v", harness.reported, report)
	}
}

func TestRunIterationSnapshotFailureStopsPipeline(t *testing.T) {
	harness := newStageHarness()
	harness.fail[core.StageSnapshot] = errors.New("snapshot failed")
	engine := mustNewEngine(t, testDependencies(harness, nil))

	_, err := engine.RunIteration(context.Background(), core.SnapshotRequest{})
	requireStageError(t, err, core.StageSnapshot)
	requireCallOrder(t, harness.calls, []core.Stage{core.StageSnapshot})
}

func TestRunIterationInspectFailureStopsPipeline(t *testing.T) {
	harness := newStageHarness()
	harness.fail[core.StageInspect] = errors.New("inspect failed")
	engine := mustNewEngine(t, testDependencies(harness, nil))

	_, err := engine.RunIteration(context.Background(), core.SnapshotRequest{})
	requireStageError(t, err, core.StageInspect)
	requireCallOrder(t, harness.calls, []core.Stage{core.StageSnapshot, core.StageInspect})
}

func TestRunIterationPlanFailureStopsPipeline(t *testing.T) {
	harness := newStageHarness()
	harness.fail[core.StagePlan] = errors.New("plan failed")
	engine := mustNewEngine(t, testDependencies(harness, nil))

	_, err := engine.RunIteration(context.Background(), core.SnapshotRequest{})
	requireStageError(t, err, core.StagePlan)
	requireCallOrder(t, harness.calls, []core.Stage{core.StageSnapshot, core.StageInspect, core.StagePlan})
}

func TestRunIterationExecuteFailureStopsPipeline(t *testing.T) {
	harness := newStageHarness()
	harness.fail[core.StageExecute] = errors.New("execute failed")
	engine := mustNewEngine(t, testDependencies(harness, nil))

	_, err := engine.RunIteration(context.Background(), core.SnapshotRequest{})
	requireStageError(t, err, core.StageExecute)
	requireCallOrder(t, harness.calls, []core.Stage{
		core.StageSnapshot,
		core.StageInspect,
		core.StagePlan,
		core.StageExecute,
	})
}

func TestRunIterationValidateFailureStopsPipeline(t *testing.T) {
	harness := newStageHarness()
	harness.fail[core.StageValidate] = errors.New("validate failed")
	engine := mustNewEngine(t, testDependencies(harness, nil))

	_, err := engine.RunIteration(context.Background(), core.SnapshotRequest{})
	requireStageError(t, err, core.StageValidate)
	requireCallOrder(t, harness.calls, []core.Stage{
		core.StageSnapshot,
		core.StageInspect,
		core.StagePlan,
		core.StageExecute,
		core.StageValidate,
	})
}

func TestRunIterationReportFailureReturnsStageError(t *testing.T) {
	harness := newStageHarness()
	harness.fail[core.StageReport] = errors.New("report failed")
	engine := mustNewEngine(t, testDependencies(harness, nil))

	_, err := engine.RunIteration(context.Background(), core.SnapshotRequest{})
	requireStageError(t, err, core.StageReport)
	requireCallOrder(t, harness.calls, core.DefaultStages())
}

func TestNewEngineRejectsMissingDependencies(t *testing.T) {
	harness := newStageHarness()
	base := testDependencies(harness, func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	})

	tests := []struct {
		name      string
		mutate    func(*Dependencies)
		errorHint string
	}{
		{
			name: "snapshotter",
			mutate: func(deps *Dependencies) {
				deps.Snapshotter = nil
			},
			errorHint: "snapshotter",
		},
		{
			name: "inspector",
			mutate: func(deps *Dependencies) {
				deps.Inspector = nil
			},
			errorHint: "inspector",
		},
		{
			name: "planner",
			mutate: func(deps *Dependencies) {
				deps.Planner = nil
			},
			errorHint: "planner",
		},
		{
			name: "executor",
			mutate: func(deps *Dependencies) {
				deps.Executor = nil
			},
			errorHint: "executor",
		},
		{
			name: "validator",
			mutate: func(deps *Dependencies) {
				deps.Validator = nil
			},
			errorHint: "validator",
		},
		{
			name: "reporter",
			mutate: func(deps *Dependencies) {
				deps.Reporter = nil
			},
			errorHint: "reporter",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			deps := base
			tt.mutate(&deps)

			engine, err := NewEngine(deps)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if engine != nil {
				t.Fatalf("expected nil engine on error, got: %+v", engine)
			}
			if core.KindOf(err) != core.KindMissingDependency {
				t.Fatalf("expected missing dependency kind, got %q (%v)", core.KindOf(err), err)
			}
			if !strings.Contains(err.Error(), tt.errorHint) {
				t.Fatalf("expected error %q to contain %q", err, tt.errorHint)
			}
		})
	}
}

func TestRunIterationUsesDefaultClock(t *testing.T) {
	harness := newStageHarness()
	engine := mustNewEngine(t, testDependencies(harness, nil))

	report, err := engine.RunIteration(context.Background(), core.SnapshotRequest{})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if report.GeneratedAt.IsZero() {
		t.Fatal("expected generatedAt to be populated")
	}
	if report.GeneratedAt.Location() != time.UTC {
		t.Fatalf("expected generatedAt timezone UTC, got %v", report.GeneratedAt.Location())
	}
}

func TestRunIterationRefreshesInspectionAfterAppliedModelAuthoring(t *testing.T) {
	// Regression for QA run 20260315T181250Z-78ca4afe:
	// after applying @tensorleap_load_model, the same iteration still reported
	// "no model candidate found" instead of acknowledging the discovered yolo11s.onnx path.
	harness := newRefreshStageHarness()
	engine := mustNewEngine(t, Dependencies{
		Snapshotter: harness,
		Inspector:   harness,
		Planner:     harness,
		Executor:    harness,
		Validator:   harness,
		Reporter:    harness,
		Clock: func() time.Time {
			return time.Date(2026, 3, 15, 18, 18, 7, 0, time.UTC)
		},
	})

	report, err := engine.RunIteration(context.Background(), core.SnapshotRequest{RepoRoot: "/repo"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if report.SnapshotID != "snapshot-after-model-loader" {
		t.Fatalf("expected post-change snapshot ID %q, got %q", "snapshot-after-model-loader", report.SnapshotID)
	}
	if hasIssueMessage(report.Validation.Issues, "no model candidate found") {
		t.Fatalf("expected post-apply validation to drop the stale pre-edit model issue, got %+v", report.Validation.Issues)
	}
	if !hasIssueMessage(report.Validation.Issues, `model file "yolo11s.onnx" was not found`) {
		t.Fatalf("expected post-apply validation to acknowledge the discovered model loader path, got %+v", report.Validation.Issues)
	}

	modelCheck, ok := findCheck(report.Checks, core.EnsureStepModelAcquisition)
	if !ok {
		t.Fatalf("expected %q check in report, got %+v", core.EnsureStepModelAcquisition, report.Checks)
	}
	if hasIssueMessage(modelCheck.Issues, "no model candidate found") {
		t.Fatalf("expected model check to drop the stale pre-edit issue, got %+v", modelCheck.Issues)
	}
	if !hasIssueMessage(modelCheck.Issues, `model file "yolo11s.onnx" was not found`) {
		t.Fatalf("expected model check to acknowledge the discovered loader path, got %+v", modelCheck.Issues)
	}

	if harness.snapshotCalls < 2 || harness.inspectCalls < 2 {
		t.Fatalf(
			"expected engine to re-snapshot and re-inspect after applied changes; snapshotCalls=%d inspectCalls=%d",
			harness.snapshotCalls,
			harness.inspectCalls,
		)
	}
}

func mustNewEngine(t *testing.T, deps Dependencies) *Engine {
	t.Helper()
	engine, err := NewEngine(deps)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	return engine
}

func testDependencies(harness *stageHarness, clock func() time.Time) Dependencies {
	return Dependencies{
		Snapshotter: harness,
		Inspector:   harness,
		Planner:     harness,
		Executor:    harness,
		Validator:   harness,
		Reporter:    harness,
		Clock:       clock,
	}
}

func requireStageError(t *testing.T, err error, expectedStage core.Stage) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var stageErr *StageError
	if !errors.As(err, &stageErr) {
		t.Fatalf("expected StageError, got %T (%v)", err, err)
	}
	if stageErr.Stage != expectedStage {
		t.Fatalf("expected stage %q, got %q", expectedStage, stageErr.Stage)
	}
}

func requireCallOrder(t *testing.T, got, expected []core.Stage) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected calls %v, got %v", expected, got)
	}
}

func hasIssueMessage(issues []core.Issue, substring string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, substring) {
			return true
		}
	}
	return false
}

func findCheck(checks []core.VerifiedCheck, stepID core.EnsureStepID) (core.VerifiedCheck, bool) {
	for _, check := range checks {
		if check.StepID == stepID {
			return check, true
		}
	}
	return core.VerifiedCheck{}, false
}
