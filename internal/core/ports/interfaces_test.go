package ports_test

import (
	"context"
	"testing"
	"time"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/core/ports"
)

type ctxKey string

const traceKey ctxKey = "trace-id"

func requireTrace(ctx context.Context) error {
	if ctx.Value(traceKey) == nil {
		return core.NewError(core.KindMissingDependency, "test.requireTrace", "missing trace value in context")
	}
	return nil
}

type fakeSnapshotter struct{}

func (fakeSnapshotter) Snapshot(ctx context.Context, request core.SnapshotRequest) (core.WorkspaceSnapshot, error) {
	if err := requireTrace(ctx); err != nil {
		return core.WorkspaceSnapshot{}, err
	}
	return core.WorkspaceSnapshot{
		ID:         "snap-1",
		CapturedAt: time.Unix(1700000000, 0).UTC(),
		Repository: core.RepositoryState{Root: request.RepoRoot},
	}, nil
}

type fakeInspector struct{}

func (fakeInspector) Inspect(ctx context.Context, snapshot core.WorkspaceSnapshot) (core.IntegrationStatus, error) {
	if err := requireTrace(ctx); err != nil {
		return core.IntegrationStatus{}, err
	}
	if snapshot.ID == "" {
		return core.IntegrationStatus{}, core.NewError(core.KindMissingDependency, "inspect", "snapshot ID is required")
	}
	return core.IntegrationStatus{}, nil
}

type fakePlanner struct{}

func (fakePlanner) Plan(ctx context.Context, snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) (core.ExecutionPlan, error) {
	if err := requireTrace(ctx); err != nil {
		return core.ExecutionPlan{}, err
	}
	_ = snapshot
	_ = status
	return core.ExecutionPlan{Primary: core.EnsureStep{ID: core.EnsureStepLeapYAML, Description: "Ensure leap.yaml exists"}}, nil
}

type fakeExecutor struct{}

func (fakeExecutor) Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	if err := requireTrace(ctx); err != nil {
		return core.ExecutionResult{}, err
	}
	if step.ID == "" {
		return core.ExecutionResult{}, core.NewError(core.KindStepNotApplicable, "execute", "step ID is required")
	}
	_ = snapshot
	return core.ExecutionResult{Step: step, Applied: true, Summary: "ok"}, nil
}

type fakeValidator struct{}

func (fakeValidator) Validate(ctx context.Context, snapshot core.WorkspaceSnapshot, result core.ExecutionResult) (core.ValidationResult, error) {
	if err := requireTrace(ctx); err != nil {
		return core.ValidationResult{}, err
	}
	_ = snapshot
	if !result.Applied {
		return core.ValidationResult{Passed: false}, nil
	}
	return core.ValidationResult{Passed: true}, nil
}

type fakeReporter struct{}

func (fakeReporter) Report(ctx context.Context, report core.IterationReport) error {
	if err := requireTrace(ctx); err != nil {
		return err
	}
	if report.SnapshotID == "" {
		return core.NewError(core.KindMissingDependency, "report", "snapshot ID is required")
	}
	return nil
}

func TestPortsSupportContextFirstPipeline(t *testing.T) {
	ctx := context.WithValue(context.Background(), traceKey, "trace-123")
	var snapshotter ports.Snapshotter = fakeSnapshotter{}
	var inspector ports.Inspector = fakeInspector{}
	var planner ports.Planner = fakePlanner{}
	var executor ports.Executor = fakeExecutor{}
	var validator ports.Validator = fakeValidator{}
	var reporter ports.Reporter = fakeReporter{}

	snapshot, err := snapshotter.Snapshot(ctx, core.SnapshotRequest{RepoRoot: "/repo"})
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	status, err := inspector.Inspect(ctx, snapshot)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	plan, err := planner.Plan(ctx, snapshot, status)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	result, err := executor.Execute(ctx, snapshot, plan.Primary)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	validation, err := validator.Validate(ctx, snapshot, result)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if !validation.Passed {
		t.Fatalf("expected validation to pass, got: %+v", validation)
	}

	err = reporter.Report(ctx, core.IterationReport{
		GeneratedAt: time.Now().UTC(),
		SnapshotID:  snapshot.ID,
		Step:        result.Step,
		Validation:  validation,
	})
	if err != nil {
		t.Fatalf("report failed: %v", err)
	}
}
