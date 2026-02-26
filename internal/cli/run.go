package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

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

			promptInput := bufio.NewReader(cmd.InOrStdin())

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			repoRoot, _, err := resolveProjectRoot(projectRootFlag, cwd, nonInteractive, promptInput, cmd.OutOrStdout())
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

			plannerAdapter := newPlanCapturePlanner(planner.NewDeterministicPlanner())

			stepApproval := func(step core.EnsureStep) (bool, error) {
				if step.ID == core.EnsureStepComplete {
					return true, nil
				}
				if yes {
					return true, nil
				}
				if nonInteractive {
					return false, core.NewError(
						core.KindUnknown,
						"cli.run.non_interactive.step_approval_required",
						"this run requires approval before Concierge applies changes; rerun with --yes to auto-approve in non-interactive mode",
					)
				}
				status, hasStatus := plannerAdapter.LastStatus()
				return promptApproval(promptInput, cmd.OutOrStdout(), stepApprovalMessage(step, status, hasStatus))
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
				return promptApproval(promptInput, cmd.OutOrStdout(), gitmanager.ApprovalMessage(step, diffSummary))
			}

			engine, err := orchestrator.NewEngine(orchestrator.Dependencies{
				Snapshotter: snapshot.NewGitSnapshotter(),
				Inspector:   inspect.NewBaselineInspector(),
				Planner:     plannerAdapter,
				Executor:    execute.NewApprovalExecutor(execute.NewDispatcherExecutor(), stepApproval),
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
				return fmt.Errorf("integration still has pending requirements. run `concierge run` again to continue guided checks.\ntip: use `--max-iterations 3` to run multiple guided rounds in one command")
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
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 1, "Maximum guided rounds before stopping")
	cmd.Flags().BoolVar(&persist, "persist", false, "Persist reports and evidence under .concierge")
	cmd.Flags().StringVar(&projectRootFlag, "project-root", "", "Project root to operate on")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Fail instead of prompting for interactive decisions")
	cmd.Flags().BoolVar(&yes, "yes", false, "Auto-approve mutation/push prompts")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colorized output")
	cmd.Flags().BoolVar(&debugOutput, "debug-output", false, "Show internal debug details in run output")
	return cmd
}

func stepApprovalMessage(step core.EnsureStep, status core.IntegrationStatus, hasStatus bool) string {
	checklist := make([]string, 0, 32)
	checklist = append(checklist, "Integration checks:")

	steps := checklistStepsForPrompt()
	blockingIndex := -1
	for i, knownStep := range steps {
		if knownStep.ID == step.ID {
			blockingIndex = i
			break
		}
	}

	for i, knownStep := range steps {
		prefix := "☐"
		label := core.HumanEnsureStepLabel(knownStep.ID)
		if blockingIndex >= 0 && i < blockingIndex {
			prefix = "☑"
		}
		if knownStep.ID == step.ID {
			label += " (blocking)"
		}
		checklist = append(checklist, fmt.Sprintf("%s %s", prefix, label))
	}

	if hasStatus {
		blockers := blockingIssuesForStep(status.Issues, step.ID)
		if len(blockers) > 0 {
			checklist = append(checklist, "", "Missing or failing requirements:")
			for i, issue := range blockers {
				if i >= 3 {
					checklist = append(checklist, "- Additional blocking details were omitted for brevity.")
					break
				}
				message := strings.TrimSpace(issue.Message)
				if message == "" {
					message = "A required check is failing."
				}
				checklist = append(checklist, "- "+message)
			}
		}
	}

	checklist = append(
		checklist,
		"",
		fmt.Sprintf("Next required check: %s", core.HumanEnsureStepLabel(step.ID)),
		"Concierge can help with this interactively.",
		"Allow Concierge to make changes for this check now? (No changes will be made before approval.)",
	)
	return strings.Join(checklist, "\n")
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

type planCapturePlanner struct {
	base ports.Planner

	mu         sync.RWMutex
	lastStatus core.IntegrationStatus
	hasStatus  bool
}

func newPlanCapturePlanner(base ports.Planner) *planCapturePlanner {
	return &planCapturePlanner{base: base}
}

func (p *planCapturePlanner) Plan(ctx context.Context, snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) (core.ExecutionPlan, error) {
	plan, err := p.base.Plan(ctx, snapshot, status)
	if err != nil {
		return core.ExecutionPlan{}, err
	}

	p.mu.Lock()
	p.lastStatus = cloneIntegrationStatus(status)
	p.hasStatus = true
	p.mu.Unlock()

	return plan, nil
}

func (p *planCapturePlanner) LastStatus() (core.IntegrationStatus, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.hasStatus {
		return core.IntegrationStatus{}, false
	}
	return cloneIntegrationStatus(p.lastStatus), true
}

func cloneIntegrationStatus(status core.IntegrationStatus) core.IntegrationStatus {
	cloned := core.IntegrationStatus{}
	if len(status.Missing) > 0 {
		cloned.Missing = append([]string(nil), status.Missing...)
	}
	if len(status.Issues) > 0 {
		cloned.Issues = append([]core.Issue(nil), status.Issues...)
	}
	return cloned
}

func checklistStepsForPrompt() []core.EnsureStep {
	known := core.KnownEnsureSteps()
	steps := make([]core.EnsureStep, 0, len(known))
	for _, step := range known {
		if step.ID == core.EnsureStepComplete || step.ID == core.EnsureStepInvestigate {
			continue
		}
		steps = append(steps, step)
	}
	return steps
}

func blockingIssuesForStep(issues []core.Issue, stepID core.EnsureStepID) []core.Issue {
	filtered := make([]core.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity != core.SeverityError {
			continue
		}
		step := core.PreferredEnsureStepForIssue(issue)
		if step.ID != stepID {
			continue
		}
		filtered = append(filtered, issue)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Code != filtered[j].Code {
			return filtered[i].Code < filtered[j].Code
		}
		return filtered[i].Message < filtered[j].Message
	})
	return filtered
}
