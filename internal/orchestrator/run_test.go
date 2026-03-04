package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

type runHarness struct {
	stepSequence       []core.EnsureStepID
	failIteration      int
	iteration          int
	reportCount        int
	cancelAfterReports int
	cancel             context.CancelFunc
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
		Step:    step,
		Applied: true,
		Summary: "ok",
	}, nil
}

func (h *runHarness) Validate(ctx context.Context, snapshot core.WorkspaceSnapshot, result core.ExecutionResult) (core.ValidationResult, error) {
	_ = ctx
	_ = snapshot
	_ = result
	return core.ValidationResult{Passed: true}, nil
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

	result, err := engine.Run(context.Background(), core.SnapshotRequest{}, RunOptions{MaxIterations: 3})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result.StopReason != RunStopReasonMaxIterations {
		t.Fatalf("expected stop reason %q, got %q", RunStopReasonMaxIterations, result.StopReason)
	}
	if len(result.Reports) != 3 {
		t.Fatalf("expected three reports, got %d", len(result.Reports))
	}
	if harness.iteration != 3 {
		t.Fatalf("expected three iterations, got %d", harness.iteration)
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
