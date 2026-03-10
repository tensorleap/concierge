package orchestrator

import (
	"context"

	"github.com/tensorleap/concierge/internal/core"
)

// RunOptions configures the outer orchestration loop.
type RunOptions struct {
	MaxIterations int
	BeforeReport  func(snapshot core.WorkspaceSnapshot, report *core.IterationReport) error
	AfterReport   func(snapshot core.WorkspaceSnapshot, report core.IterationReport) error
}

// RunStopReason captures why the orchestration loop stopped.
type RunStopReason string

const (
	RunStopReasonSuccess       RunStopReason = "success"
	RunStopReasonMaxIterations RunStopReason = "max_iterations"
	RunStopReasonCancelled     RunStopReason = "cancelled"
	RunStopReasonInterrupted   RunStopReason = "interrupted_step"
)

// RunResult aggregates per-iteration reports for one run invocation.
type RunResult struct {
	Reports    []core.IterationReport `json:"reports"`
	StopReason RunStopReason          `json:"stopReason"`
}

// Run executes the outer loop until completion, cancellation, or max-iteration limit.
func (e *Engine) Run(ctx context.Context, req core.SnapshotRequest, opts RunOptions) (RunResult, error) {
	maxIterations := opts.MaxIterations

	reports := make([]core.IterationReport, 0)
	if maxIterations > 0 {
		reports = make([]core.IterationReport, 0, maxIterations)
	}
	for i := 0; maxIterations <= 0 || i < maxIterations; i++ {
		if err := ctx.Err(); err != nil {
			return RunResult{
				Reports:    reports,
				StopReason: RunStopReasonCancelled,
			}, err
		}

		report, snapshot, err := e.runIteration(ctx, req, i+1, opts.BeforeReport)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return RunResult{
					Reports:    reports,
					StopReason: RunStopReasonCancelled,
				}, ctxErr
			}
			return RunResult{Reports: reports}, err
		}

		reports = append(reports, report)
		if opts.AfterReport != nil {
			if err := opts.AfterReport(snapshot, report); err != nil {
				return RunResult{Reports: reports}, err
			}
		}
		if hasInterruptedAgent(report) {
			return RunResult{
				Reports:    reports,
				StopReason: RunStopReasonInterrupted,
			}, nil
		}
		if report.Step.ID == core.EnsureStepComplete && report.Validation.Passed {
			return RunResult{
				Reports:    reports,
				StopReason: RunStopReasonSuccess,
			}, nil
		}
	}

	return RunResult{
		Reports:    reports,
		StopReason: RunStopReasonMaxIterations,
	}, nil
}

func hasInterruptedAgent(report core.IterationReport) bool {
	for _, item := range report.Evidence {
		if item.Name == "agent.interrupted" && item.Value == "true" {
			return true
		}
	}
	return false
}
