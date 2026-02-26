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
	headerIcon := "✓"
	headerColor := ansiGreen
	if report.Step.ID != core.EnsureStepComplete || !report.Validation.Passed {
		headerIcon = "⚠"
		headerColor = ansiYellow
	}

	if _, err := fmt.Fprintf(writer, "%s %s\n", paint(headerIcon, headerColor, colorEnabled), paint("Integration Checklist", ansiBold+ansiCyan, colorEnabled)); err != nil {
		return err
	}

	rows, blockerStep, blockerIssues, complete := buildChecklist(report)
	for _, row := range rows {
		checkbox := "☐"
		checkboxColor := ansiDim
		switch row.State {
		case checklistPassed:
			checkbox = "☑"
			checkboxColor = ansiGreen
		case checklistBlocked:
			checkboxColor = ansiYellow
		}

		label := core.HumanEnsureStepLabel(row.Step.ID)
		if row.State == checklistBlocked {
			label += " (blocking)"
		}
		if _, err := fmt.Fprintf(writer, "%s %s\n", paint(checkbox, checkboxColor, colorEnabled), label); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(writer); err != nil {
		return err
	}

	if complete {
		if _, err := fmt.Fprintln(writer, "All required checks passed."); err != nil {
			return err
		}
	} else if blockerStep.ID != "" {
		if _, err := fmt.Fprintf(writer, "Blocked on: %s\n", core.HumanEnsureStepLabel(blockerStep.ID)); err != nil {
			return err
		}
		if len(blockerIssues) > 0 {
			if _, err := fmt.Fprintln(writer, "Missing or failing requirements:"); err != nil {
				return err
			}
			for i, issue := range blockerIssues {
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
						message = "A required check is failing."
					}
				}
				if _, err := fmt.Fprintf(writer, "- %s\n", message); err != nil {
					return err
				}
			}
		}
		if _, err := fmt.Fprintln(writer, "Concierge can help with this step interactively and will ask before making any changes."); err != nil {
			return err
		}
	}

	changeStatus := "No changes were applied."
	if hasEvidenceValue(report.Evidence, "executor.change_approval", "rejected") {
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

type checklistState string

const (
	checklistPending checklistState = "pending"
	checklistPassed  checklistState = "passed"
	checklistBlocked checklistState = "blocked"
)

type checklistRow struct {
	Step  core.EnsureStep
	State checklistState
}

func buildChecklist(report core.IterationReport) ([]checklistRow, core.EnsureStep, []core.Issue, bool) {
	steps := checklistSteps()
	if len(steps) == 0 {
		return nil, core.EnsureStep{}, nil, false
	}

	complete := report.Step.ID == core.EnsureStepComplete && report.Validation.Passed
	if complete {
		rows := make([]checklistRow, 0, len(steps))
		for _, step := range steps {
			rows = append(rows, checklistRow{Step: step, State: checklistPassed})
		}
		return rows, core.EnsureStep{}, nil, true
	}

	blockerStep, hasBlocker := resolveBlockerStep(report, steps)
	if !hasBlocker {
		rows := make([]checklistRow, 0, len(steps))
		for _, step := range steps {
			rows = append(rows, checklistRow{Step: step, State: checklistPending})
		}
		return rows, core.EnsureStep{}, nil, false
	}

	blockerIndex := indexOfStep(steps, blockerStep.ID)
	rows := make([]checklistRow, 0, len(steps))
	for i, step := range steps {
		state := checklistPending
		if i < blockerIndex {
			state = checklistPassed
		}
		if i == blockerIndex {
			state = checklistBlocked
		}
		rows = append(rows, checklistRow{Step: step, State: state})
	}

	return rows, blockerStep, issuesForStep(report.Validation.Issues, blockerStep.ID), false
}

func checklistSteps() []core.EnsureStep {
	known := core.KnownEnsureSteps()
	steps := make([]core.EnsureStep, 0, len(known))
	for _, step := range known {
		if step.ID == core.EnsureStepComplete || step.ID == core.EnsureStepInvestigate {
			continue
		}
		steps = append(steps, step)
	}
	return steps
}

func resolveBlockerStep(report core.IterationReport, steps []core.EnsureStep) (core.EnsureStep, bool) {
	if report.Step.ID != "" && report.Step.ID != core.EnsureStepComplete {
		if _, ok := ensureStepInList(steps, report.Step.ID); ok {
			return report.Step, true
		}
	}

	for _, issue := range report.Validation.Issues {
		if issue.Severity != core.SeverityError {
			continue
		}
		step := core.PreferredEnsureStepForIssue(issue)
		if step.ID == "" || step.ID == core.EnsureStepComplete {
			continue
		}
		if _, ok := ensureStepInList(steps, step.ID); ok {
			return step, true
		}
	}

	return core.EnsureStep{}, false
}

func ensureStepInList(steps []core.EnsureStep, stepID core.EnsureStepID) (core.EnsureStep, bool) {
	for _, step := range steps {
		if step.ID == stepID {
			return step, true
		}
	}
	return core.EnsureStep{}, false
}

func indexOfStep(steps []core.EnsureStep, stepID core.EnsureStepID) int {
	for i, step := range steps {
		if step.ID == stepID {
			return i
		}
	}
	return -1
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

func hasEvidenceValue(evidence []core.EvidenceItem, name, value string) bool {
	for _, item := range evidence {
		if item.Name == name && item.Value == value {
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
