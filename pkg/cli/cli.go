package cli

import (
	"github.com/spf13/cobra"
	internalCli "github.com/tensorleap/concierge/internal/cli"
)

// NewRootCommand returns the top-level concierge cobra command.
func NewRootCommand() *cobra.Command {
	return internalCli.NewRootCommand()
}
