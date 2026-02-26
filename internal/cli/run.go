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

var runLogoProvider = defaultCLILogo

func newRunCommand() *cobra.Command {
	var dryRun bool
	var maxIterations int
	var persist bool
	var projectRootFlag string
	var nonInteractive bool
	var yes bool
	var noColor bool
	var debugOutput bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run Concierge orchestration",
		RunE: func(cmd *cobra.Command, args []string) error {
			writer := cmd.OutOrStdout()
			renderOptions := runRenderOptions{
				EnableColor: cliColorEnabled(writer, noColor),
				Logo:        runLogoProvider(),
			}

			if dryRun {
				stages := core.DefaultStages()
				return renderRunDryPlan(writer, stages, renderOptions)
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			repoRoot, _, err := resolveProjectRoot(projectRootFlag, cwd, nonInteractive, cmd.InOrStdin(), cmd.OutOrStdout())
			if err != nil {
				return err
			}

			if err := renderRunSessionStart(writer, repoRoot, persist, nonInteractive, debugOutput, renderOptions); err != nil {
				return err
			}

			loadedState, err := state.LoadState(repoRoot)
			if err != nil {
				return err
			}

			var iterationReporter ports.Reporter = report.NewStdoutReporterWithOptions(
				writer,
				report.OutputOptions{
					NoColor: noColor,
					Debug:   debugOutput,
				},
			)
			if persist {
				iterationReporter, err = report.NewFileReporterWithOptions(
					repoRoot,
					writer,
					report.OutputOptions{
						NoColor: noColor,
						Debug:   debugOutput,
					},
				)
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
						"this run requires approval to commit changes; rerun with --yes to auto-approve in non-interactive mode",
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
							if len(invalidationReasons) > 0 {
								report.Notes = append(report.Notes, humanInvalidationSummary(invalidationReasons))
								if debugOutput {
									report.Notes = append(
										report.Notes,
										fmt.Sprintf("Debug details: invalidation reasons = %s", strings.Join(invalidationReasons, ", ")),
									)
								}
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
				return fmt.Errorf("more work is needed. run `concierge run` again to continue.\ntip: use `--max-iterations 3` to run multiple passes in one command")
			case orchestrator.RunStopReasonCancelled:
				if ctxErr := cmd.Context().Err(); ctxErr != nil {
					return ctxErr
				}
				return context.Canceled
			default:
				return errors.New("run stopped unexpectedly; please rerun and review the latest output")
			}
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview orchestration stages without making changes")
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 1, "Maximum orchestration iterations before stopping")
	cmd.Flags().BoolVar(&persist, "persist", false, "Persist reports and evidence under .concierge")
	cmd.Flags().StringVar(&projectRootFlag, "project-root", "", "Project root to operate on")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Fail instead of prompting for interactive decisions")
	cmd.Flags().BoolVar(&yes, "yes", false, "Auto-approve mutation/push prompts")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colorized output")
	cmd.Flags().BoolVar(&debugOutput, "debug-output", false, "Show internal debug details in run output")
	return cmd
}

func humanInvalidationSummary(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}

	labels := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		switch reason {
		case state.InvalidationReasonProjectRootChanged:
			labels = append(labels, "project folder changed")
		case state.InvalidationReasonGitHeadChanged:
			labels = append(labels, "Git commit changed")
		case state.InvalidationReasonWorktreeFingerprintDiff:
			labels = append(labels, "files changed")
		default:
			labels = append(labels, "workspace changed")
		}
	}

	return fmt.Sprintf("Your workspace changed since the previous run (%s), so Concierge re-checked everything.", strings.Join(labels, ", "))
}
