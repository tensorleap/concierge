package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tensorleap/concierge/internal/buildinfo"
)

var leapCLIDiagnosticsProvider = detectLeapCLIDiagnostics
var doctorLogoProvider = defaultDoctorLogo
var doctorGetenv = os.Getenv
var doctorIsTerminalWriter = isTerminalWriter

func newDoctorCommand() *cobra.Command {
	var format string
	var noColor bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check your Concierge and Leap CLI setup",
		RunE: func(cmd *cobra.Command, args []string) error {
			diagnostics := leapCLIDiagnosticsProvider(cmd.Context())
			info := buildinfo.Current()
			output := doctorOutput{
				Version:          info.Version,
				Commit:           info.Commit,
				Date:             info.Date,
				GoVersion:        runtime.Version(),
				OS:               runtime.GOOS,
				Arch:             runtime.GOARCH,
				LeapCLIDiagnosis: diagnostics,
				Logo:             doctorLogoProvider(),
			}

			normalizedFormat := strings.ToLower(strings.TrimSpace(format))
			switch normalizedFormat {
			case doctorFormatHuman:
				return renderDoctorHuman(
					cmd.OutOrStdout(),
					output,
					doctorRenderOptions{EnableColor: doctorColorEnabled(cmd.OutOrStdout(), noColor)},
				)
			case doctorFormatJSON:
				return renderDoctorJSON(cmd.OutOrStdout(), output)
			default:
				return fmt.Errorf("invalid value for --format %q (allowed: human, json)", format)
			}
		},
	}

	cmd.Flags().StringVar(&format, "format", doctorFormatHuman, "Output format: human|json")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colorized output")
	return cmd
}

func doctorColorEnabled(writer io.Writer, noColor bool) bool {
	if noColor {
		return false
	}
	if strings.TrimSpace(doctorGetenv("NO_COLOR")) != "" {
		return false
	}
	return doctorIsTerminalWriter(writer)
}

func init() {
	// Keep provider initialized for production path and easy test stubbing.
	leapCLIDiagnosticsProvider = detectLeapCLIDiagnostics
}

func setLeapCLIDiagnosticsProviderForTest(provider func(context.Context) leapCLIDiagnostics) func() {
	previous := leapCLIDiagnosticsProvider
	leapCLIDiagnosticsProvider = provider
	return func() {
		leapCLIDiagnosticsProvider = previous
	}
}

func setDoctorLogoProviderForTest(provider func() string) func() {
	previous := doctorLogoProvider
	doctorLogoProvider = provider
	return func() {
		doctorLogoProvider = previous
	}
}

func setDoctorColorDepsForTest(getenv func(string) string, isTerminal func(io.Writer) bool) func() {
	previousGetenv := doctorGetenv
	previousIsTerminal := doctorIsTerminalWriter
	doctorGetenv = getenv
	doctorIsTerminalWriter = isTerminal
	return func() {
		doctorGetenv = previousGetenv
		doctorIsTerminalWriter = previousIsTerminal
	}
}
