package ports

import (
	"context"

	"github.com/tensorleap/concierge/internal/core"
)

// Snapshotter captures a deterministic workspace snapshot.
type Snapshotter interface {
	Snapshot(ctx context.Context, request core.SnapshotRequest) (core.WorkspaceSnapshot, error)
}

// Inspector converts a snapshot into integration findings.
type Inspector interface {
	Inspect(ctx context.Context, snapshot core.WorkspaceSnapshot) (core.IntegrationStatus, error)
}

// Planner selects the next deterministic ensure-step.
type Planner interface {
	Plan(ctx context.Context, snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) (core.ExecutionPlan, error)
}

// Executor applies one ensure-step and emits evidence.
type Executor interface {
	Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error)
}

// Validator verifies post-execution acceptance checks.
type Validator interface {
	Validate(ctx context.Context, snapshot core.WorkspaceSnapshot, result core.ExecutionResult) (core.ValidationResult, error)
}

// Reporter publishes the final iteration output.
type Reporter interface {
	Report(ctx context.Context, report core.IterationReport) error
}
