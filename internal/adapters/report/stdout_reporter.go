package report

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"

	tensorleapUploadGuideURL = "https://docs.tensorleap.ai/tensorleap-integration/uploading-with-cli/cli-assets-upload"
)

// OutputOptions controls how reporter output is rendered for humans.
type OutputOptions struct {
	NoColor bool
	Debug   bool
}

// StdoutReporter writes a one-line summary for each iteration report.
type StdoutReporter struct {
	writer  io.Writer
	options OutputOptions
}

// NewStdoutReporter creates a reporter that writes to the provided writer.
func NewStdoutReporter(writer io.Writer) *StdoutReporter {
	return NewStdoutReporterWithOptions(writer, OutputOptions{})
}

// NewStdoutReporterWithOptions creates a reporter with rendering options.
func NewStdoutReporterWithOptions(writer io.Writer, options OutputOptions) *StdoutReporter {
	if writer == nil {
		writer = os.Stdout
	}
	return &StdoutReporter{
		writer:  writer,
		options: options,
	}
}

// Report writes a one-line summary of the iteration report.
func (r *StdoutReporter) Report(ctx context.Context, report core.IterationReport) error {
	_ = ctx

	writer := r.writer
	if writer == nil {
		writer = os.Stdout
	}

	if err := writeSummaryLine(writer, report, r.options); err != nil {
		return core.WrapError(core.KindUnknown, "report.stdout.write", err)
	}

	return nil
}

func writeSummaryLine(writer io.Writer, report core.IterationReport, options OutputOptions) error {
	colorEnabled := reportColorEnabled(writer, options)
	if _, err := fmt.Fprintf(writer, "%s\n", paint("Integration Checklist", ansiBold+ansiCyan, colorEnabled)); err != nil {
		return err
	}

	visibleChecks := core.VisibleChecksForFlow(report.Checks)
	for _, check := range visibleChecks {
		checkbox := "☐"
		checkboxColor := ansiDim
		switch check.Status {
		case core.CheckStatusPass:
			checkbox = "☑"
			checkboxColor = ansiGreen
		case core.CheckStatusWarning:
			checkbox = "⚠"
			checkboxColor = ansiYellow
		case core.CheckStatusFail:
			checkboxColor = ansiYellow
		}

		label := renderedCheckLabel(check)
		if check.Blocking {
			label += " (blocking)"
		}
		if _, err := fmt.Fprintf(writer, "%s %s\n", paint(checkbox, checkboxColor, colorEnabled), label); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(writer); err != nil {
		return err
	}

	attentionCheck, hasAttention := core.FirstAttentionCheck(visibleChecks)
	if len(visibleChecks) == 0 {
		if _, err := fmt.Fprintln(writer, "No checks were verified in this iteration."); err != nil {
			return err
		}
	} else if !hasAttention {
		if _, err := fmt.Fprintln(writer, "Verified checks passed."); err != nil {
			return err
		}
		if report.Step.ID == core.EnsureStepComplete && report.Validation.Passed {
			if _, err := fmt.Fprintln(writer, "Next steps:"); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(writer, "- If you have not uploaded this integration yet, run `leap push` from the repository root."); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(writer, "- Upload guide: %s\n", tensorleapUploadGuideURL); err != nil {
				return err
			}
		}
	} else if attentionCheck.StepID != "" {
		attentionLabel := renderedCheckLabel(attentionCheck)

		heading := "Warning:"
		defaultMessage := "A warning was reported for this check."
		if attentionCheck.Status == core.CheckStatusFail {
			heading = "Blocked on:"
			defaultMessage = "A required verification is failing."
		}

		if _, err := fmt.Fprintf(writer, "%s %s\n", heading, attentionLabel); err != nil {
			return err
		}

		details := attentionIssues(attentionCheck, report.Validation.Issues)
		if len(details) > 0 {
			if _, err := fmt.Fprintln(writer, "Details:"); err != nil {
				return err
			}
			for i, issue := range details {
				if i >= 3 {
					if _, err := fmt.Fprintln(writer, "- Additional blocking details were omitted for brevity."); err != nil {
						return err
					}
					break
				}
				message := strings.TrimSpace(issue.Message)
				if message == "" {
					if options.Debug {
						message = string(issue.Code)
					} else {
						message = defaultMessage
					}
				}
				if _, err := fmt.Fprintf(writer, "- %s\n", message); err != nil {
					return err
				}
			}
		}
		if shouldOfferInteractiveHelp(report, attentionCheck) {
			if _, err := fmt.Fprintln(writer, "Concierge can help with this step interactively and will ask before making any changes."); err != nil {
				return err
			}
		} else {
			for _, line := range selfServiceGuidanceLines(report, attentionCheck.StepID, details) {
				if _, err := fmt.Fprintln(writer, line); err != nil {
					return err
				}
			}
		}
	}

	changeStatus := "No changes were applied."
	if hasEvidenceValue(report.Evidence, "executor.change_approval", "rejected") ||
		hasEvidenceValue(report.Evidence, "git.approval", "rejected") {
		changeStatus = "No changes were made because approval was not granted."
	} else if report.Applied {
		changeStatus = "Changes were applied."
	}
	if _, err := fmt.Fprintf(writer, "Changes: %s\n", changeStatus); err != nil {
		return err
	}

	if report.Commit != nil {
		commitSummary := strings.TrimSpace(report.Commit.Hash)
		if message := strings.TrimSpace(report.Commit.Message); message != "" {
			if commitSummary != "" {
				commitSummary = fmt.Sprintf("%s (%s)", commitSummary, message)
			} else {
				commitSummary = message
			}
		}
		if commitSummary != "" {
			if _, err := fmt.Fprintf(writer, "Commit: %s\n", commitSummary); err != nil {
				return err
			}
		}
	}

	if options.Debug && strings.TrimSpace(report.Step.Description) != "" {
		if _, err := fmt.Fprintf(writer, "Debug step detail: %s\n", strings.TrimSpace(report.Step.Description)); err != nil {
			return err
		}
	}

	if len(report.Notes) > 0 {
		if _, err := fmt.Fprintln(writer, "Notes:"); err != nil {
			return err
		}
		for _, note := range report.Notes {
			trimmed := strings.TrimSpace(note)
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "Debug details:") && !options.Debug {
				continue
			}
			if _, err := fmt.Fprintf(writer, "- %s\n", trimmed); err != nil {
				return err
			}
		}
	}

	_, err := fmt.Fprintln(writer)
	return err
}

func blockingIssues(issues []core.Issue) []core.Issue {
	filtered := make([]core.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity == core.SeverityError {
			filtered = append(filtered, issue)
		}
	}
	if len(filtered) > 0 {
		return filtered
	}
	return append([]core.Issue(nil), issues...)
}

func renderedCheckLabel(check core.VerifiedCheck) string {
	if check.Status == core.CheckStatusPass {
		label := strings.TrimSpace(check.Label)
		if label != "" {
			return label
		}
		return core.HumanEnsureStepLabel(check.StepID)
	}
	return core.HumanEnsureStepRequirementLabel(check.StepID)
}

func attentionIssues(check core.VerifiedCheck, validationIssues []core.Issue) []core.Issue {
	if len(check.Issues) > 0 {
		if check.Status == core.CheckStatusFail {
			return blockingIssues(check.Issues)
		}

		warnings := make([]core.Issue, 0, len(check.Issues))
		for _, issue := range check.Issues {
			if issue.Severity == core.SeverityWarning {
				warnings = append(warnings, issue)
			}
		}
		if len(warnings) > 0 {
			return warnings
		}
		return append([]core.Issue(nil), check.Issues...)
	}

	if check.Status == core.CheckStatusFail {
		return issuesForStep(validationIssues, check.StepID)
	}
	return issuesForStepAny(validationIssues, check.StepID)
}

func issuesForStep(issues []core.Issue, stepID core.EnsureStepID) []core.Issue {
	matched := make([]core.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity != core.SeverityError {
			continue
		}
		step := core.PreferredEnsureStepForIssue(issue)
		if step.ID == stepID {
			matched = append(matched, issue)
		}
	}
	if len(matched) > 0 {
		return matched
	}

	fallback := make([]core.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity == core.SeverityError {
			fallback = append(fallback, issue)
		}
	}
	return fallback
}

func issuesForStepAny(issues []core.Issue, stepID core.EnsureStepID) []core.Issue {
	matched := make([]core.Issue, 0, len(issues))
	for _, issue := range issues {
		step := core.PreferredEnsureStepForIssue(issue)
		if step.ID == stepID {
			matched = append(matched, issue)
		}
	}
	return matched
}

func hasEvidenceValue(evidence []core.EvidenceItem, name, value string) bool {
	for _, item := range evidence {
		if item.Name == name && item.Value == value {
			return true
		}
	}
	return false
}

func shouldOfferInteractiveHelp(report core.IterationReport, attentionCheck core.VerifiedCheck) bool {
	if attentionCheck.StepID == "" {
		return false
	}
	if report.Step.ID == core.EnsureStepComplete {
		return false
	}
	if hasEvidenceValue(report.Evidence, "executor.mode", "stub") {
		return false
	}
	return true
}

func selfServiceGuidanceLines(report core.IterationReport, stepID core.EnsureStepID, issues []core.Issue) []string {
	if report.Step.ID == core.EnsureStepComplete {
		return warningFollowUpLines(stepID, issues)
	}

	lines := []string{
		"Concierge cannot apply an automated fix for this check in the current run.",
	}
	return append(lines, stepGuidanceLines(stepID, issues)...)
}

func warningFollowUpLines(stepID core.EnsureStepID, issues []core.Issue) []string {
	lines := []string{
		"This warning is advisory in the current run, so Concierge did not apply a fix automatically.",
	}
	return append(lines, stepGuidanceLines(stepID, issues)...)
}

func stepGuidanceLines(stepID core.EnsureStepID, issues []core.Issue) []string {
	switch stepID {
	case core.EnsureStepLeapCLIAuth:
		if hasIssueCode(issues, core.IssueCodeLeapCLINotFound) {
			return []string{
				"Next step: install the Leap CLI, then run `leap --version` and `leap auth login`, and rerun `concierge run`.",
			}
		}
		if hasIssueCode(issues, core.IssueCodeLeapCLIVersionUnavailable) {
			return []string{
				"Next step: run `leap --version`; if it fails, reinstall the Leap CLI, then rerun `concierge run`.",
			}
		}
		if hasIssueCode(issues, core.IssueCodeLeapCLINotAuthenticated) {
			return []string{
				"Next step: run `leap auth login`, then rerun `concierge run`.",
			}
		}
		return []string{
			"Next step: run `leap --version` and `leap auth whoami`, then rerun `concierge run`.",
		}
	case core.EnsureStepServerConnectivity:
		return []string{
			"Next step: run `leap server info` and make sure your Tensorleap server is reachable on port 4589, then rerun `concierge run`.",
		}
	default:
		return []string{
			"Resolve the warning and rerun `concierge run` to verify it is cleared.",
		}
	}
}

func hasIssueCode(issues []core.Issue, code core.IssueCode) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func reportColorEnabled(writer io.Writer, options OutputOptions) bool {
	if options.NoColor {
		return false
	}
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func paint(text, colorCode string, enabled bool) string {
	if !enabled {
		return text
	}
	return colorCode + text + ansiReset
}
