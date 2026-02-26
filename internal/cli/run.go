package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tensorleap/concierge/internal/adapters/execute"
	"github.com/tensorleap/concierge/internal/adapters/inspect"
	"github.com/tensorleap/concierge/internal/adapters/planner"
	"github.com/tensorleap/concierge/internal/adapters/report"
	"github.com/tensorleap/concierge/internal/adapters/snapshot"
	"github.com/tensorleap/concierge/internal/adapters/validate"
	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/core/ports"
	"github.com/tensorleap/concierge/internal/orchestrator"
)

func newRunCommand() *cobra.Command {
	var dryRun bool
	var maxIterations int
	var persist bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run Concierge orchestration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun {
				stages := core.DefaultStages()
				stageNames := make([]string, 0, len(stages))
				for _, stage := range stages {
					stageNames = append(stageNames, string(stage))
				}
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run plan: %s\n", strings.Join(stageNames, " -> "))
				return err
			}

			repoRoot, err := os.Getwd()
			if err != nil {
				return err
			}

			var iterationReporter ports.Reporter = report.NewStdoutReporter(cmd.OutOrStdout())
			if persist {
				iterationReporter, err = report.NewFileReporter(repoRoot, cmd.OutOrStdout())
				if err != nil {
					return err
				}
			}

			engine, err := orchestrator.NewEngine(orchestrator.Dependencies{
				Snapshotter: snapshot.NewGitSnapshotter(),
				Inspector:   inspect.NewBaselineInspector(),
				Planner:     planner.NewDeterministicPlanner(),
				Executor:    execute.NewStubExecutor(),
				Validator:   validate.NewBaselineValidator(),
				Reporter:    iterationReporter,
			})
			if err != nil {
				return err
			}

			runResult, err := engine.Run(
				cmd.Context(),
				core.SnapshotRequest{RepoRoot: repoRoot},
				orchestrator.RunOptions{MaxIterations: maxIterations},
			)
			if err != nil {
				return err
			}

			switch runResult.StopReason {
			case orchestrator.RunStopReasonSuccess:
				return nil
			case orchestrator.RunStopReasonMaxIterations:
				return fmt.Errorf(
					"run stopped with stop reason %q after %d iteration(s)",
					runResult.StopReason,
					len(runResult.Reports),
				)
			case orchestrator.RunStopReasonCancelled:
				if ctxErr := cmd.Context().Err(); ctxErr != nil {
					return ctxErr
				}
				return context.Canceled
			default:
				return errors.New("run stopped with an unknown reason")
			}
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview orchestration stages without making changes")
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 1, "Maximum orchestration iterations before stopping")
	cmd.Flags().BoolVar(&persist, "persist", false, "Persist reports and evidence under .concierge")
	return cmd
}
