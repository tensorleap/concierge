package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var allowedLogLevels = map[string]struct{}{
	"debug": {},
	"info":  {},
	"warn":  {},
	"error": {},
}

func NewRootCommand() *cobra.Command {
	var logLevel string

	cmd := &cobra.Command{
		Use:           "concierge",
		Short:         "Tensorleap integration assistant",
		Long:          "Concierge helps you diagnose and guide Tensorleap integration work from the terminal.",
		Example:       "  concierge doctor\n  concierge run --dry-run\n  concierge version --format json",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			normalized := strings.ToLower(strings.TrimSpace(logLevel))
			if _, ok := allowedLogLevels[normalized]; !ok {
				return fmt.Errorf("invalid value for --log-level %q (allowed: debug, info, warn, error)", logLevel)
			}

			logLevel = normalized
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log verbosity: debug|info|warn|error")
	cmd.AddCommand(
		newDoctorCommand(),
		newRunCommand(),
		newVersionCommand(),
	)

	return cmd
}
