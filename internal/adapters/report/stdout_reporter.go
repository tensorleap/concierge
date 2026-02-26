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
	if !report.Validation.Passed {
		headerIcon = "⚠"
		headerColor = ansiYellow
	}

	if _, err := fmt.Fprintf(writer, "%s %s\n", paint(headerIcon, headerColor, colorEnabled), paint("Iteration Update", ansiBold+ansiCyan, colorEnabled)); err != nil {
		return err
	}

	stepDescription := humanStepDescription(report.Step)
	if _, err := fmt.Fprintf(writer, "Step: %s\n", stepDescription); err != nil {
		return err
	}
	if options.Debug && strings.TrimSpace(report.Step.Description) != "" {
		if _, err := fmt.Fprintf(writer, "Debug step detail: %s\n", strings.TrimSpace(report.Step.Description)); err != nil {
			return err
		}
	}

	changeStatus := "No repository changes applied"
	if report.Applied {
		changeStatus = "Changes were applied"
	}
	if _, err := fmt.Fprintf(writer, "Changes: %s\n", changeStatus); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(writer, "Validation: %s\n", validationState(report.Validation)); err != nil {
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

	if !report.Validation.Passed && len(report.Validation.Issues) > 0 {
		if _, err := fmt.Fprintln(writer, "What needs attention:"); err != nil {
			return err
		}
		for i, issue := range report.Validation.Issues {
			if i >= 3 {
				if _, err := fmt.Fprintln(writer, "- Additional issues were omitted for brevity."); err != nil {
					return err
				}
				break
			}
			message := strings.TrimSpace(issue.Message)
			if message == "" {
				if options.Debug {
					message = string(issue.Code)
				} else {
					message = "A validation issue was detected."
				}
			}
			if _, err := fmt.Fprintf(writer, "- %s\n", message); err != nil {
				return err
			}
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

func validationState(result core.ValidationResult) string {
	if result.Passed {
		return "Passed"
	}
	return "Needs attention"
}

func humanStepDescription(step core.EnsureStep) string {
	switch step.ID {
	case core.EnsureStepComplete:
		return "Integration is currently complete."
	case core.EnsureStepRepositoryContext:
		return "Check repository setup"
	case core.EnsureStepPythonRuntime:
		return "Check Python setup"
	case core.EnsureStepLeapCLIAuth:
		return "Check Leap CLI installation and login"
	case core.EnsureStepServerConnectivity:
		return "Check Tensorleap server connection"
	case core.EnsureStepSecretsContext:
		return "Check required secrets"
	case core.EnsureStepLeapYAML:
		return "Check leap.yaml setup"
	case core.EnsureStepModelContract:
		return "Check model compatibility"
	case core.EnsureStepIntegrationScript:
		return "Check integration script setup"
	case core.EnsureStepPreprocessContract:
		return "Check preprocess setup"
	case core.EnsureStepInputEncoders:
		return "Check input encoders"
	case core.EnsureStepGroundTruthEncoders:
		return "Check ground-truth encoders"
	case core.EnsureStepIntegrationTestContract:
		return "Check integration test wiring"
	case core.EnsureStepOptionalHooks:
		return "Check optional integration hooks"
	case core.EnsureStepHarnessValidation:
		return "Run runtime validation checks"
	case core.EnsureStepUploadReadiness:
		return "Check upload readiness"
	case core.EnsureStepUploadPush:
		return "Upload integration to Tensorleap"
	case core.EnsureStepInvestigate:
		return "Investigate remaining issues"
	default:
		label := strings.TrimPrefix(string(step.ID), "ensure.")
		label = strings.ReplaceAll(label, "_", " ")
		label = strings.TrimSpace(label)
		if label == "" {
			return "Run the next planned step"
		}
		return strings.ToUpper(label[:1]) + label[1:]
	}
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
