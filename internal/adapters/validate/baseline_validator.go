package validate

import (
	"context"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

type harnessInvoker interface {
	Run(ctx context.Context, snapshot core.WorkspaceSnapshot) (HarnessRunResult, error)
}

type guideInvoker interface {
	Run(ctx context.Context, snapshot core.WorkspaceSnapshot) (GuideValidationResult, error)
}

// BaselineValidator performs deterministic execution-level checks.
type BaselineValidator struct {
	guideRunner   guideInvoker
	harnessRunner harnessInvoker
}

// NewBaselineValidator creates a baseline validator adapter.
func NewBaselineValidator() *BaselineValidator {
	return &BaselineValidator{
		guideRunner:   NewGuideValidator(),
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

	if v != nil && v.guideRunner != nil {
		guideResult, err := v.guideRunner.Run(ctx, snapshot)
		if err != nil {
			return core.ValidationResult{}, core.WrapError(core.KindUnknown, "validate.baseline.guide", err)
		}

		validation.Issues = append(validation.Issues, guideResult.Issues...)
		validation.Evidence = append(validation.Evidence, guideResult.Evidence...)
		if v.harnessRunner != nil {
			if runHarness, reason := shouldRunHarnessAfterGuide(guideResult.Summary); runHarness {
				harnessResult, err := v.harnessRunner.Run(ctx, snapshot)
				if err != nil {
					return core.ValidationResult{}, core.WrapError(core.KindUnknown, "validate.baseline.harness", err)
				}

				if harnessResult.Enabled {
					validation.Issues = append(validation.Issues, harnessResult.Issues...)
					validation.Evidence = append(validation.Evidence, harnessResult.Evidence...)
					validation.Issues = append(validation.Issues, HeuristicIssuesFromHarnessEvents(harnessResult.Events)...)
				}
			} else if reason != "" {
				validation.Evidence = append(validation.Evidence, core.EvidenceItem{
					Name:  "harness.skip_reason",
					Value: reason,
				})
			}
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
		guideRunner:   readyGuideInvoker{},
		harnessRunner: runner,
	}
}

func newBaselineValidatorWithGuideHarness(guide guideInvoker, harness harnessInvoker) *BaselineValidator {
	return &BaselineValidator{
		guideRunner:   guide,
		harnessRunner: harness,
	}
}

func shouldRunHarnessAfterGuide(summary core.GuideValidationSummary) (bool, string) {
	if summary.Skipped {
		return false, messageOrDefault(strings.TrimSpace(summary.SkipReason), "guide validation skipped")
	}
	if summary.Local.Successful {
		return true, ""
	}
	if summary.Parser.Attempted && summary.Parser.Available && summary.Parser.IsValid {
		return true, ""
	}
	return false, "guide first-sample validation is not ready for multi-sample runtime checks"
}

type readyGuideInvoker struct{}

func (readyGuideInvoker) Run(ctx context.Context, snapshot core.WorkspaceSnapshot) (GuideValidationResult, error) {
	_ = ctx
	_ = snapshot
	return GuideValidationResult{
		Summary: core.GuideValidationSummary{
			Local: core.GuideLocalRunSummary{Successful: true},
		},
	}, nil
}
