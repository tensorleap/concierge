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
	"github.com/tensorleap/concierge/internal/gitmanager"
	"github.com/tensorleap/concierge/internal/orchestrator"
	"github.com/tensorleap/concierge/internal/state"
)

func newRunCommand() *cobra.Command {
	var dryRun bool
	var maxIterations int
	var persist bool
	var projectRootFlag string
	var nonInteractive bool
	var yes bool

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

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			repoRoot, projectRootNote, err := resolveProjectRoot(projectRootFlag, cwd, nonInteractive, cmd.InOrStdin(), cmd.OutOrStdout())
			if err != nil {
				return err
			}

			loadedState, err := state.LoadState(repoRoot)
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

			gitApproval := func(step core.EnsureStep, diffSummary string) (bool, error) {
				if yes {
					return true, nil
				}
				if nonInteractive {
					return false, core.NewError(
						core.KindUnknown,
						"cli.run.non_interactive.approval_required",
						"non-interactive mode requires --yes when mutation approval is needed",
					)
				}
				return promptApproval(cmd.InOrStdin(), cmd.OutOrStdout(), gitmanager.ApprovalMessage(step, diffSummary))
			}

			engine, err := orchestrator.NewEngine(orchestrator.Dependencies{
				Snapshotter: snapshot.NewGitSnapshotter(),
				Inspector:   inspect.NewBaselineInspector(),
				Planner:     planner.NewDeterministicPlanner(),
				Executor:    execute.NewDispatcherExecutor(),
				GitManager:  gitmanager.NewManager(gitApproval),
				Validator:   validate.NewBaselineValidator(),
				Reporter:    iterationReporter,
			})
			if err != nil {
				return err
			}

			initialState := loadedState
			currentState := loadedState
			sessionNotes := []string{projectRootNote}
			initializedStateNotes := false
			invalidationReasons := []string(nil)

			runResult, err := engine.Run(
				cmd.Context(),
				core.SnapshotRequest{RepoRoot: repoRoot},
				orchestrator.RunOptions{
					MaxIterations: maxIterations,
					BeforeReport: func(snapshotValue core.WorkspaceSnapshot, report *core.IterationReport) error {
						if !initializedStateNotes {
							invalidationReasons = state.ComputeInvalidationReasons(initialState, snapshotValue, repoRoot)
							report.Notes = append(report.Notes, sessionNotes...)
							if len(invalidationReasons) > 0 {
								report.Notes = append(
									report.Notes,
									fmt.Sprintf("state invalidation: %s", strings.Join(invalidationReasons, ", ")),
								)
								for i, reason := range invalidationReasons {
									report.Evidence = append(report.Evidence, core.EvidenceItem{
										Name:  fmt.Sprintf("state.invalidation_reason.%d", i+1),
										Value: reason,
									})
								}
							}
							initializedStateNotes = true
						}

						nextState := state.UpdateForIteration(currentState, snapshotValue, *report, repoRoot, invalidationReasons)
						if err := state.SaveState(repoRoot, nextState); err != nil {
							return err
						}
						currentState = nextState
						return nil
					},
				},
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
	cmd.Flags().StringVar(&projectRootFlag, "project-root", "", "Project root to operate on")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Fail instead of prompting for interactive decisions")
	cmd.Flags().BoolVar(&yes, "yes", false, "Auto-approve mutation/push prompts")
	return cmd
}
