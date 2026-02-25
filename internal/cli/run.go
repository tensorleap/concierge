package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tensorleap/concierge/internal/core"
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

			stages := core.DefaultStages()
			stageNames := make([]string, 0, len(stages))
			for _, stage := range stages {
				stageNames = append(stageNames, string(stage))
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run plan: %s\n", strings.Join(stageNames, " -> "))
			return err
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview orchestration stages without making changes")
	return cmd
}
