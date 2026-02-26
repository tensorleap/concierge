package cli

import (
	"context"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/tensorleap/concierge/internal/buildinfo"
)

var leapCLIDiagnosticsProvider = detectLeapCLIDiagnostics

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Print baseline runtime diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			diagnostics := leapCLIDiagnosticsProvider(cmd.Context())
			info := buildinfo.Current()
			_, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"version: %s\ncommit: %s\ndate: %s\ngo: %s\nos: %s\narch: %s\nleap_cli_installed: %s\nleap_cli_current_version: %s\nleap_cli_latest_version: %s\nleap_cli_status: %s\nleap_cli_note: %s\nleap_cli_action: %s\n",
				info.Version,
				info.Commit,
				info.Date,
				runtime.Version(),
				runtime.GOOS,
				runtime.GOARCH,
				boolLabel(diagnostics.Installed),
				diagnostics.CurrentVersion,
				diagnostics.LatestVersion,
				diagnostics.Status,
				diagnostics.Note,
				diagnostics.Action,
			)
			return err
		},
	}
}

func boolLabel(v bool) string {
	if v {
		return "yes"
	}
	return "no"
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
