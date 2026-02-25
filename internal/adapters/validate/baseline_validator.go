package validate

import (
	"context"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

// BaselineValidator performs deterministic execution-level checks.
type BaselineValidator struct{}

// NewBaselineValidator creates a baseline validator adapter.
func NewBaselineValidator() *BaselineValidator {
	return &BaselineValidator{}
}

// Validate evaluates baseline execution consistency before harness integration exists.
func (v *BaselineValidator) Validate(ctx context.Context, snapshot core.WorkspaceSnapshot, result core.ExecutionResult) (core.ValidationResult, error) {
	_ = ctx
	_ = snapshot

	if result.Step.ID == "" {
		return core.ValidationResult{
			Passed: false,
			Issues: []core.Issue{
				{
					Code:     core.IssueCodeHarnessValidationFailed,
					Message:  "execution result is missing step ID",
					Severity: core.SeverityError,
					Scope:    core.IssueScopeValidation,
				},
			},
		}, nil
	}

	if !result.Applied && strings.Contains(strings.ToLower(result.Summary), "not implemented") {
		return core.ValidationResult{
			Passed: true,
			Issues: []core.Issue{
				{
					Code:     core.IssueCodeUnknown,
					Message:  "execution step is a stub and was not applied",
					Severity: core.SeverityInfo,
					Scope:    core.IssueScopeValidation,
				},
			},
		}, nil
	}

	return core.ValidationResult{Passed: true}, nil
}
