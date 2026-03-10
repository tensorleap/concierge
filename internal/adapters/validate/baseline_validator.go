package validate

import (
	"context"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

type harnessInvoker interface {
	Run(ctx context.Context, snapshot core.WorkspaceSnapshot) (HarnessRunResult, error)
}

// BaselineValidator performs deterministic execution-level checks.
type BaselineValidator struct {
	harnessRunner harnessInvoker
}

// NewBaselineValidator creates a baseline validator adapter.
func NewBaselineValidator() *BaselineValidator {
	return &BaselineValidator{
		harnessRunner: NewHarnessRunner(),
	}
}

// Validate evaluates baseline execution consistency before harness integration exists.
func (v *BaselineValidator) Validate(ctx context.Context, snapshot core.WorkspaceSnapshot, result core.ExecutionResult) (core.ValidationResult, error) {
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

	validation := core.ValidationResult{Passed: true}

	if !result.Applied && strings.Contains(strings.ToLower(result.Summary), "not implemented") {
		validation.Issues = append(validation.Issues, core.Issue{
			Code:     core.IssueCodeUnknown,
			Message:  "execution step is a stub and was not applied",
			Severity: core.SeverityInfo,
			Scope:    core.IssueScopeValidation,
		})
	}

	if v != nil && v.harnessRunner != nil {
		harnessResult, err := v.harnessRunner.Run(ctx, snapshot)
		if err != nil {
			return core.ValidationResult{}, core.WrapError(core.KindUnknown, "validate.baseline.harness", err)
		}

		if harnessResult.Enabled {
			validation.Issues = append(validation.Issues, harnessResult.Issues...)
			validation.Evidence = append(validation.Evidence, harnessResult.Evidence...)
			validation.Issues = append(validation.Issues, HeuristicIssuesFromHarnessEvents(harnessResult.Events)...)
		}
	}

	validation.Passed = true
	for _, issue := range validation.Issues {
		if issue.Severity == core.SeverityError {
			validation.Passed = false
			break
		}
	}

	return validation, nil
}

func newBaselineValidatorWithHarness(runner harnessInvoker) *BaselineValidator {
	return &BaselineValidator{
		harnessRunner: runner,
	}
}
