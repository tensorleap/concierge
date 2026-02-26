package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tensorleap/concierge/internal/buildinfo"
)

func newVersionCommand() *cobra.Command {
	var format string
	var noColor bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show Concierge build version details",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := buildinfo.Current()
			output := versionOutput{
				Version: info.Version,
				Commit:  info.Commit,
				Date:    info.Date,
			}

			switch strings.ToLower(strings.TrimSpace(format)) {
			case doctorFormatHuman:
				return renderVersionHuman(cmd.OutOrStdout(), output, cliColorEnabled(cmd.OutOrStdout(), noColor))
			case doctorFormatJSON:
				return renderVersionJSON(cmd.OutOrStdout(), output)
			default:
				return fmt.Errorf("invalid value for --format %q (allowed: human, json)", format)
			}
		},
	}

	cmd.Flags().StringVar(&format, "format", doctorFormatHuman, "Output format: human|json")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colorized output")
	return cmd
}
