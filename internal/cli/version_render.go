package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

type versionOutput struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func renderVersionHuman(writer io.Writer, output versionOutput, enableColor bool) error {
	if logo := defaultCLILogo(); logo != "" {
		if _, err := fmt.Fprintf(writer, "%s\n\n", paint(logo, ansiCyan, enableColor)); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(writer, "%s\n", paint("Concierge Version", ansiBold+ansiCyan, enableColor)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "%s\n\n", paint("Build details for this Concierge binary.", ansiDim, enableColor)); err != nil {
		return err
	}

	if err := writeDoctorSectionTitle(writer, "Build", enableColor); err != nil {
		return err
	}

	rows := [][2]string{
		{"Version", nonEmptyValue(output.Version, "unknown")},
		{"Commit", nonEmptyValue(output.Commit, "unknown")},
		{"Build date", nonEmptyValue(output.Date, "unknown")},
	}
	return writeDoctorRows(writer, rows)
}

func renderVersionJSON(writer io.Writer, output versionOutput) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
