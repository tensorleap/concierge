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

const (
	stepGuideLeapYAMLURL           = "https://docs.tensorleap.ai/tensorleap-integration/leap.yaml"
	stepGuideModelIntegrationURL   = "https://docs.tensorleap.ai/tensorleap-integration/model-integration"
	stepGuideWritingIntegrationURL = "https://docs.tensorleap.ai/tensorleap-integration/writing-integration-code"
	stepGuidePreprocessURL         = "https://docs.tensorleap.ai/tensorleap-integration/writing-integration-code/preprocess-function"
	stepGuideInputEncoderURL       = "https://docs.tensorleap.ai/tensorleap-integration/writing-integration-code/input-encoder"
	stepGuideGroundTruthURL        = "https://docs.tensorleap.ai/tensorleap-integration/writing-integration-code/ground-truth-encoder"
	stepGuideIntegrationTestURL    = "https://docs.tensorleap.ai/tensorleap-integration/integration-test"
)

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
						"this run requires approval before Concierge applies and commits changes; rerun with --yes to auto-approve in non-interactive mode",
					)
				}
				snapshotValue, hasSnapshot := plannerAdapter.LastSnapshot()
				status, hasStatus := plannerAdapter.LastStatus()
				return promptApproval(
					promptInput,
					cmd.OutOrStdout(),
					stepApprovalMessage(step, snapshotValue, hasSnapshot, status, hasStatus, renderOptions.EnableColor),
				)
			}

			gitApproval := func(step core.EnsureStep, review gitmanager.ChangeReview) (bool, error) {
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
				return promptChangeReviewApproval(
					promptInput,
					cmd.OutOrStdout(),
					step,
					review,
					changeReviewRenderOptions{EnableColor: renderOptions.EnableColor},
				)
			}

			engine, err := orchestrator.NewEngine(orchestrator.Dependencies{
				Snapshotter: snapshot.NewGitSnapshotter(),
				Inspector:   inspect.NewBaselineInspector(),
				Planner:     plannerAdapter,
				Executor:    execute.NewApprovalExecutor(execute.NewDispatcherExecutor(), stepApproval),
				GitManager:  gitmanager.NewManager(gitApproval, gitmanager.ManagerOptions{ColorDiff: renderOptions.EnableColor}),
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

func stepApprovalMessage(
	step core.EnsureStep,
	snapshot core.WorkspaceSnapshot,
	hasSnapshot bool,
	status core.IntegrationStatus,
	hasStatus bool,
	enableColor bool,
) string {
	checklist := checklistRowsForPrompt(step, snapshot, hasSnapshot, status, hasStatus, enableColor)

	blockers := []core.Issue(nil)
	if hasStatus {
		blockers = blockingIssuesForStep(status.Issues, step.ID)
	}

	checkLabel := core.HumanEnsureStepLabel(step.ID)
	checkHeading := "Current check"
	if len(blockers) > 0 {
		checkLabel = core.HumanEnsureStepRequirementLabel(step.ID)
		checkHeading = "Current blocker"
	}
	checklist = append(checklist, "", fmt.Sprintf("%s: %s", checkHeading, checkLabel))

	guidance := approvalGuidanceForStep(step.ID)
	if guidance.Explanation != "" {
		checklist = append(checklist, "Why it matters: "+guidance.Explanation)
	}
	if guidance.DocsURL != "" {
		checklist = append(checklist, "Docs: "+guidance.DocsURL)
	}

	if len(blockers) > 0 {
		checklist = append(checklist, "What failed:")
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

	checklist = append(
		checklist,
		"",
		"Concierge can help fix this now.",
		"Allow Concierge to make changes for this check now?",
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
	lastSnap   core.WorkspaceSnapshot
	hasSnap    bool
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
	p.lastSnap = cloneWorkspaceSnapshot(snapshot)
	p.hasSnap = true
	p.lastStatus = cloneIntegrationStatus(status)
	p.hasStatus = true
	p.mu.Unlock()

	return plan, nil
}

func (p *planCapturePlanner) LastSnapshot() (core.WorkspaceSnapshot, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.hasSnap {
		return core.WorkspaceSnapshot{}, false
	}
	return cloneWorkspaceSnapshot(p.lastSnap), true
}

func (p *planCapturePlanner) LastStatus() (core.IntegrationStatus, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.hasStatus {
		return core.IntegrationStatus{}, false
	}
	return cloneIntegrationStatus(p.lastStatus), true
}

func cloneWorkspaceSnapshot(snapshot core.WorkspaceSnapshot) core.WorkspaceSnapshot {
	cloned := snapshot
	if len(snapshot.FileHashes) > 0 {
		cloned.FileHashes = make(map[string]string, len(snapshot.FileHashes))
		for key, value := range snapshot.FileHashes {
			cloned.FileHashes[key] = value
		}
	}
	if len(snapshot.Runtime.RequirementsFiles) > 0 {
		cloned.Runtime.RequirementsFiles = append([]string(nil), snapshot.Runtime.RequirementsFiles...)
	}
	return cloned
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

func checklistRowsForPrompt(
	step core.EnsureStep,
	snapshot core.WorkspaceSnapshot,
	hasSnapshot bool,
	status core.IntegrationStatus,
	hasStatus bool,
	enableColor bool,
) []string {
	checklist := []string{"Integration checks:"}
	if !hasSnapshot || !hasStatus {
		return checklist
	}

	checks := core.BuildVerifiedChecks(snapshot, status.Issues, nil, step.ID)
	for _, check := range core.VisibleChecksForFlow(checks) {
		icon := promptCheckIcon(check.Status, enableColor)
		label := promptCheckLabel(check)
		if check.Blocking {
			label += " (blocking)"
		}
		checklist = append(checklist, fmt.Sprintf("%s %s", icon, label))
	}

	return checklist
}

func promptCheckIcon(status core.CheckStatus, enableColor bool) string {
	icon := "☐"
	color := ansiDim
	switch status {
	case core.CheckStatusPass:
		icon = "☑"
		color = ansiGreen
	case core.CheckStatusWarning:
		icon = "⚠"
		color = ansiYellow
	case core.CheckStatusFail:
		color = ansiYellow
	}
	return paint(icon, color, enableColor)
}

func promptCheckLabel(check core.VerifiedCheck) string {
	if check.Status == core.CheckStatusPass {
		label := strings.TrimSpace(check.Label)
		if label != "" {
			return label
		}
		return core.HumanEnsureStepLabel(check.StepID)
	}
	return core.HumanEnsureStepRequirementLabel(check.StepID)
}

type stepApprovalGuidance struct {
	Explanation string
	DocsURL     string
}

func approvalGuidanceForStep(stepID core.EnsureStepID) stepApprovalGuidance {
	switch stepID {
	case core.EnsureStepRepositoryContext:
		return stepApprovalGuidance{
			Explanation: "Concierge needs a valid Git project root and a safe working branch before applying fixes.",
		}
	case core.EnsureStepPythonRuntime:
		return stepApprovalGuidance{
			Explanation: "Python and dependencies are required to run integration code and validation checks.",
			DocsURL:     stepGuideWritingIntegrationURL,
		}
	case core.EnsureStepLeapCLIAuth:
		return stepApprovalGuidance{
			Explanation: "Concierge needs a working and authenticated leap CLI to validate and upload integrations.",
			DocsURL:     leapCLIInstallGuideURL,
		}
	case core.EnsureStepServerConnectivity:
		return stepApprovalGuidance{
			Explanation: "The Tensorleap server must be reachable so Concierge can verify mounts and run upload readiness checks.",
			DocsURL:     tensorleapUploadGuideURL,
		}
	case core.EnsureStepSecretsContext:
		return stepApprovalGuidance{
			Explanation: "Required secrets must be configured so integration code can access protected assets safely.",
			DocsURL:     tensorleapSecretsGuideURL,
		}
	case core.EnsureStepLeapYAML:
		return stepApprovalGuidance{
			Explanation: "leap.yaml defines the upload boundary and entry point that Tensorleap uses to run your integration.",
			DocsURL:     stepGuideLeapYAMLURL,
		}
	case core.EnsureStepModelContract:
		return stepApprovalGuidance{
			Explanation: "Tensorleap requires model artifacts and tensor shapes to follow supported integration contracts.",
			DocsURL:     stepGuideModelIntegrationURL,
		}
	case core.EnsureStepIntegrationScript:
		return stepApprovalGuidance{
			Explanation: "The integration script is where preprocess and encoder interfaces are defined for Tensorleap.",
			DocsURL:     stepGuideWritingIntegrationURL,
		}
	case core.EnsureStepPreprocessContract:
		return stepApprovalGuidance{
			Explanation: "Preprocess must return valid dataset subsets so Tensorleap can iterate samples correctly.",
			DocsURL:     stepGuidePreprocessURL,
		}
	case core.EnsureStepInputEncoders:
		return stepApprovalGuidance{
			Explanation: "Input encoders provide model-ready tensors for each sample and must run reliably.",
			DocsURL:     stepGuideInputEncoderURL,
		}
	case core.EnsureStepGroundTruthEncoders:
		return stepApprovalGuidance{
			Explanation: "Ground-truth encoders provide labels and are required for labeled-set validation and analysis.",
			DocsURL:     stepGuideGroundTruthURL,
		}
	case core.EnsureStepIntegrationTestContract:
		return stepApprovalGuidance{
			Explanation: "The integration test defines which interfaces Tensorleap actually executes during analysis.",
			DocsURL:     stepGuideIntegrationTestURL,
		}
	case core.EnsureStepOptionalHooks:
		return stepApprovalGuidance{
			Explanation: "Optional hooks like metadata, metrics, and visualizers should execute cleanly when enabled.",
			DocsURL:     stepGuideWritingIntegrationURL,
		}
	case core.EnsureStepHarnessValidation:
		return stepApprovalGuidance{
			Explanation: "Runtime checks confirm that integration behavior is valid across real sample execution, not just static wiring.",
			DocsURL:     stepGuideIntegrationTestURL,
		}
	case core.EnsureStepUploadReadiness, core.EnsureStepUploadPush:
		return stepApprovalGuidance{
			Explanation: "Upload readiness checks prevent failed pushes by verifying required files, mounts, and CLI prerequisites.",
			DocsURL:     tensorleapUploadGuideURL,
		}
	case core.EnsureStepInvestigate:
		return stepApprovalGuidance{
			Explanation: "Concierge found an unmapped blocker and needs to inspect it before suggesting the next deterministic fix.",
		}
	default:
		return stepApprovalGuidance{}
	}
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
