package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

const (
	doctorFormatHuman = "human"
	doctorFormatJSON  = "json"

	tensorleapUploadGuideURL  = "https://docs.tensorleap.ai/tensorleap-integration/uploading-with-cli/cli-assets-upload"
	tensorleapSecretsGuideURL = "https://docs.tensorleap.ai/user-interface/secrets-management"

	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiCyan   = "\033[36m"
)

type doctorOutput struct {
	Version          string
	Commit           string
	Date             string
	GoVersion        string
	OS               string
	Arch             string
	LeapCLIDiagnosis leapCLIDiagnostics
	Logo             string
}

type doctorRenderOptions struct {
	EnableColor bool
}

type doctorStatusView struct {
	Label        string
	Icon         string
	Summary      string
	ColorCode    string
	CurrentValue string
	LatestValue  string
}

type doctorJSONPayload struct {
	Tool     doctorJSONTool     `json:"tool"`
	Platform doctorJSONPlatform `json:"platform"`
	LeapCLI  doctorJSONLeapCLI  `json:"leapCli"`
	Docs     doctorJSONDocs     `json:"docs"`
}

type doctorJSONTool struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"builtAt"`
}

type doctorJSONPlatform struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"goVersion"`
}

type doctorJSONLeapCLI struct {
	Installed      bool   `json:"installed"`
	Status         string `json:"status"`
	StatusLabel    string `json:"statusLabel"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Message        string `json:"message"`
	Action         string `json:"action"`
}

type doctorJSONDocs struct {
	LeapCLI       string `json:"leapCli"`
	UploadGuide   string `json:"uploadGuide"`
	SecretsGuide  string `json:"secretsGuide"`
	TensorleapHub string `json:"tensorleapDocs"`
}

func renderDoctorHuman(writer io.Writer, output doctorOutput, options doctorRenderOptions) error {
	status := mapDoctorStatus(output.LeapCLIDiagnosis)

	if logo := strings.TrimSpace(output.Logo); logo != "" {
		if _, err := fmt.Fprintf(writer, "%s\n\n", paint(logo, ansiCyan, options.EnableColor)); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(writer, "%s\n", paint("Concierge Doctor", ansiBold+ansiCyan, options.EnableColor)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(
		writer,
		"%s\n\n",
		paint("A quick health check for your Tensorleap command-line setup.", ansiDim, options.EnableColor),
	); err != nil {
		return err
	}

	statusLine := fmt.Sprintf("%s %s", status.Icon, paint(status.Label, ansiBold+status.ColorCode, options.EnableColor))
	if _, err := fmt.Fprintf(writer, "%s\n%s\n\n", statusLine, status.Summary); err != nil {
		return err
	}

	if err := writeDoctorSectionTitle(writer, "System", options.EnableColor); err != nil {
		return err
	}
	systemRows := [][2]string{
		{"Concierge version", nonEmptyValue(output.Version, "unknown")},
		{"Build commit", nonEmptyValue(output.Commit, "unknown")},
		{"Build date", nonEmptyValue(output.Date, "unknown")},
		{"Operating system", fmt.Sprintf("%s/%s", nonEmptyValue(output.OS, "unknown"), nonEmptyValue(output.Arch, "unknown"))},
		{"Go runtime", nonEmptyValue(output.GoVersion, "unknown")},
	}
	if err := writeDoctorRows(writer, systemRows); err != nil {
		return err
	}

	if err := writeDoctorSectionTitle(writer, "Leap CLI", options.EnableColor); err != nil {
		return err
	}
	leapRows := [][2]string{
		{"Status", fmt.Sprintf("%s %s", status.Icon, status.Label)},
		{"Installed", yesNoValue(output.LeapCLIDiagnosis.Installed)},
		{"Installed version", status.CurrentValue},
		{"Latest version", status.LatestValue},
	}
	if err := writeDoctorRows(writer, leapRows); err != nil {
		return err
	}

	return nil
}

func renderDoctorJSON(writer io.Writer, output doctorOutput) error {
	status := mapDoctorStatus(output.LeapCLIDiagnosis)
	payload := doctorJSONPayload{
		Tool: doctorJSONTool{
			Version: nonEmptyValue(output.Version, "unknown"),
			Commit:  nonEmptyValue(output.Commit, "unknown"),
			BuiltAt: nonEmptyValue(output.Date, "unknown"),
		},
		Platform: doctorJSONPlatform{
			OS:        nonEmptyValue(output.OS, "unknown"),
			Arch:      nonEmptyValue(output.Arch, "unknown"),
			GoVersion: nonEmptyValue(output.GoVersion, "unknown"),
		},
		LeapCLI: doctorJSONLeapCLI{
			Installed:      output.LeapCLIDiagnosis.Installed,
			Status:         nonEmptyValue(output.LeapCLIDiagnosis.Status, "unknown"),
			StatusLabel:    status.Label,
			CurrentVersion: status.CurrentValue,
			LatestVersion:  status.LatestValue,
			Message:        nonEmptyValue(output.LeapCLIDiagnosis.Note, "No additional details."),
			Action:         nonEmptyValue(output.LeapCLIDiagnosis.Action, "No action needed."),
		},
		Docs: doctorJSONDocs{
			LeapCLI:       leapCLIInstallGuideURL,
			UploadGuide:   tensorleapUploadGuideURL,
			SecretsGuide:  tensorleapSecretsGuideURL,
			TensorleapHub: "https://docs.tensorleap.ai",
		},
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

func mapDoctorStatus(diagnostics leapCLIDiagnostics) doctorStatusView {
	current := nonEmptyValue(diagnostics.CurrentVersion, "Not detected")
	latest := nonEmptyValue(diagnostics.LatestVersion, "Not checked")

	switch diagnostics.Status {
	case "missing":
		return doctorStatusView{
			Label:        "Needs attention",
			Icon:         "✗",
			Summary:      "Leap CLI is not available yet. Install it to connect Concierge to Tensorleap.",
			ColorCode:    ansiRed,
			CurrentValue: "Not installed",
			LatestValue:  latest,
		}
	case "outdated":
		return doctorStatusView{
			Label:        "Update available",
			Icon:         "⚠",
			Summary:      "Your Leap CLI works, but a newer version is available.",
			ColorCode:    ansiYellow,
			CurrentValue: current,
			LatestValue:  latest,
		}
	case "up_to_date":
		return doctorStatusView{
			Label:        "Healthy",
			Icon:         "✓",
			Summary:      "Everything looks good. Concierge can use your Leap CLI right away.",
			ColorCode:    ansiGreen,
			CurrentValue: current,
			LatestValue:  latest,
		}
	default:
		return doctorStatusView{
			Label:        "Check needed",
			Icon:         "⚠",
			Summary:      "Concierge found Leap CLI but could not fully verify its health.",
			ColorCode:    ansiYellow,
			CurrentValue: current,
			LatestValue:  latest,
		}
	}
}

func writeDoctorSectionTitle(writer io.Writer, title string, enableColor bool) error {
	heading := paint(title, ansiBold+ansiBlue, enableColor)
	if _, err := fmt.Fprintf(writer, "%s\n", heading); err != nil {
		return err
	}
	_, err := fmt.Fprintln(writer, strings.Repeat("-", len(title)))
	return err
}

func writeDoctorRows(writer io.Writer, rows [][2]string) error {
	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		if _, err := fmt.Fprintf(table, "%s:\t%s\n", row[0], row[1]); err != nil {
			return err
		}
	}
	if err := table.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintln(writer)
	return err
}

func paint(text, colorCode string, enabled bool) string {
	if !enabled || strings.TrimSpace(text) == "" {
		return text
	}
	return colorCode + text + ansiReset
}

func nonEmptyValue(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "unknown") {
		return fallback
	}
	return trimmed
}

func yesNoValue(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

func defaultDoctorLogo() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		candidate := filepath.Join(cwd, "logo.txt")
		contents, readErr := os.ReadFile(candidate)
		if readErr == nil {
			return strings.TrimRight(string(contents), "\n")
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return ""
		}
		cwd = parent
	}
}
