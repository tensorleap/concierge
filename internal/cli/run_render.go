package cli

import (
	"fmt"
	"io"

	"github.com/tensorleap/concierge/internal/core"
)

type runRenderOptions struct {
	EnableColor bool
	Logo        string
}

func renderRunDryPlan(writer io.Writer, stages []core.Stage, options runRenderOptions) error {
	if logo := options.Logo; logo != "" {
		if _, err := fmt.Fprintf(writer, "%s\n\n", paint(logo, ansiCyan, options.EnableColor)); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(writer, "%s\n", paint("Concierge Run (Dry Run)", ansiBold+ansiCyan, options.EnableColor)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "%s\n\n", paint("Previewing the workflow only. No files will be changed.", ansiDim, options.EnableColor)); err != nil {
		return err
	}

	if err := writeDoctorSectionTitle(writer, "Planned Workflow", options.EnableColor); err != nil {
		return err
	}
	for index, stage := range stages {
		label := runStageLabel(stage)
		if _, err := fmt.Fprintf(writer, "%d. %s\n", index+1, label); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintln(writer)
	return err
}

func renderRunSessionStart(
	writer io.Writer,
	projectRoot string,
	persist bool,
	nonInteractive bool,
	debugOutput bool,
	options runRenderOptions,
) error {
	if logo := options.Logo; logo != "" {
		if _, err := fmt.Fprintf(writer, "%s\n\n", paint(logo, ansiCyan, options.EnableColor)); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(writer, "%s\n", paint("Concierge Run", ansiBold+ansiCyan, options.EnableColor)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "%s\n\n", paint("Guiding your integration checks and fixes.", ansiDim, options.EnableColor)); err != nil {
		return err
	}

	if err := writeDoctorSectionTitle(writer, "Session", options.EnableColor); err != nil {
		return err
	}
	rows := make([][2]string, 0, 4)
	rows = append(rows, [2]string{"Project root", projectRoot})
	if nonInteractive {
		rows = append(rows, [2]string{"Interaction", "Non-interactive"})
	}
	if persist {
		rows = append(rows, [2]string{"Saved reports", ".concierge"})
	}
	if debugOutput {
		rows = append(rows, [2]string{"Debug output", "Enabled"})
	}
	return writeDoctorRows(writer, rows)
}

func runStageLabel(stage core.Stage) string {
	switch stage {
	case core.StageSnapshot:
		return "Capture workspace snapshot"
	case core.StageInspect:
		return "Inspect project readiness"
	case core.StagePlan:
		return "Choose the next action"
	case core.StageExecute:
		return "Apply the selected action"
	case core.StageValidate:
		return "Run runtime checks"
	case core.StageReport:
		return "Show the run report"
	default:
		return string(stage)
	}
}
