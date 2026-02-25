package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tensorleap/concierge/internal/buildinfo"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print Concierge build version metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := buildinfo.Current()
			_, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"version: %s\ncommit: %s\ndate: %s\n",
				info.Version,
				info.Commit,
				info.Date,
			)
			return err
		},
	}
}
