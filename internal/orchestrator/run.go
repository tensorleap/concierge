package orchestrator

import (
	"context"

	"github.com/tensorleap/concierge/internal/core"
)

// RunOptions configures the outer orchestration loop.
type RunOptions struct {
	MaxIterations int
	// InitialBlockingIssues resolves persisted blocking validation issues against
	// the first fresh snapshot of a new `concierge run` invocation.
	InitialBlockingIssues func(snapshot core.WorkspaceSnapshot) []core.Issue
	BeforeReport          func(snapshot core.WorkspaceSnapshot, report *core.IterationReport) error
	AfterReport           func(snapshot core.WorkspaceSnapshot, report core.IterationReport) error
}

// RunStopReason captures why the orchestration loop stopped.
type RunStopReason string

const (
	RunStopReasonSuccess         RunStopReason = "success"
	RunStopReasonMaxIterations   RunStopReason = "max_iterations"
	RunStopReasonCancelled       RunStopReason = "cancelled"
	RunStopReasonInterrupted     RunStopReason = "interrupted_step"
	RunStopReasonNeedsUserAction RunStopReason = "needs_user_action"
	RunStopReasonNoProgress      RunStopReason = "no_progress"
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
	carriedValidationIssues := []core.Issue(nil)
	var lastStepID core.EnsureStepID
	consecutiveNoProgress := 0
	for i := 0; maxIterations <= 0 || i < maxIterations; i++ {
		if err := ctx.Err(); err != nil {
			return RunResult{
				Reports:    reports,
				StopReason: RunStopReasonCancelled,
			}, err
		}

		report, snapshot, err := e.runIteration(
			ctx,
			req,
			i+1,
			carriedValidationIssues,
			opts.InitialBlockingIssues,
			opts.BeforeReport,
		)
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
		carriedValidationIssues = blockingIssues(report.Validation.Issues)
		if !report.Applied && report.Step.ID == lastStepID {
			consecutiveNoProgress++
		} else {
			consecutiveNoProgress = 0
		}
		lastStepID = report.Step.ID
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
		if requiresUserAction(report) {
			return RunResult{
				Reports:    reports,
				StopReason: RunStopReasonNeedsUserAction,
			}, nil
		}
		if report.Step.ID == core.EnsureStepComplete && report.Validation.Passed {
			return RunResult{
				Reports:    reports,
				StopReason: RunStopReasonSuccess,
			}, nil
		}
		if consecutiveNoProgress >= 2 {
			return RunResult{
				Reports:    reports,
				StopReason: RunStopReasonNoProgress,
			}, nil
		}
	}

	return RunResult{
		Reports:    reports,
		StopReason: RunStopReasonMaxIterations,
	}, nil
}

func blockingIssues(issues []core.Issue) []core.Issue {
	if len(issues) == 0 {
		return nil
	}

	blocking := make([]core.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity == core.SeverityError {
			blocking = append(blocking, issue)
		}
	}
	return blocking
}

func hasInterruptedAgent(report core.IterationReport) bool {
	for _, item := range report.Evidence {
		if item.Name == "agent.interrupted" && item.Value == "true" {
			return true
		}
	}
	return false
}

func requiresUserAction(report core.IterationReport) bool {
	for _, item := range report.Evidence {
		if item.Name == "executor.mode" && item.Value == "self_service" {
			return true
		}
		if item.Name == "executor.change_approval" && item.Value == "rejected" {
			return true
		}
		if item.Name == "git.approval" && item.Value == "rejected" {
			return true
		}
		if item.Name == "git.review_action" && item.Value == "blocked_risky_artifacts" {
			return true
		}
		if item.Name == "git.commit_pending_review" && item.Value == "true" {
			return true
		}
	}
	if core.IssuesRequireManualAction(report.Validation.Issues) {
		return true
	}
	return false
}
