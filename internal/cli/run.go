package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func newRunCommand() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run Concierge orchestration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun {
				return errors.New("run is not implemented yet; use --dry-run")
			}

			_, err := fmt.Fprintln(cmd.OutOrStdout(), "dry-run plan: snapshot -> inspect -> plan -> execute -> validate -> report")
			return err
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview orchestration stages without making changes")
	return cmd
}
