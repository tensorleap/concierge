package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/tensorleap/concierge/internal/buildinfo"
)

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Print baseline runtime diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := buildinfo.Current()
			_, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"version: %s\ncommit: %s\ndate: %s\ngo: %s\nos: %s\narch: %s\n",
				info.Version,
				info.Commit,
				info.Date,
				runtime.Version(),
				runtime.GOOS,
				runtime.GOARCH,
			)
			return err
		},
	}
}
