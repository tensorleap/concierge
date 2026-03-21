package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/tensorleap/concierge/internal/adapters/execute"
	"github.com/tensorleap/concierge/internal/adapters/inspect"
	"github.com/tensorleap/concierge/internal/adapters/planner"
	"github.com/tensorleap/concierge/internal/adapters/report"
	"github.com/tensorleap/concierge/internal/adapters/snapshot"
	"github.com/tensorleap/concierge/internal/adapters/validate"
	"github.com/tensorleap/concierge/internal/agent"
	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/core/ports"
	"github.com/tensorleap/concierge/internal/gitmanager"
	"github.com/tensorleap/concierge/internal/observe"
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
	var modelPathFlag string
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

			recorder, err := observe.NewRecorder(repoRoot)
			if err != nil {
				return err
			}
			liveRenderer := observe.NewHighlightsRenderer(
				writer,
				observe.RenderOptions{NoColor: noColor},
			)
			liveEvents := observe.NewSafeSink(observe.NewMultiSink(recorder, liveRenderer))

			loadedState, err := state.LoadState(repoRoot)
			if err != nil {
				return err
			}
			selectedModelPath := strings.TrimSpace(loadedState.SelectedModelPath)
			if override := strings.TrimSpace(modelPathFlag); override != "" {
				selectedModelPath = override
			}
			selectedModelPath, err = normalizeModelPathOption(repoRoot, selectedModelPath)
			if err != nil {
				return err
			}
			var selectedModelPathMu sync.RWMutex
			getSelectedModelPath := func() string {
				selectedModelPathMu.RLock()
				defer selectedModelPathMu.RUnlock()
				return selectedModelPath
			}
			setSelectedModelPath := func(path string) {
				selectedModelPathMu.Lock()
				selectedModelPath = normalizeModelPathValue(path)
				selectedModelPathMu.Unlock()
			}
			modelAcquisitionClarification := cloneModelAcquisitionClarification(loadedState.ModelAcquisitionClarification)
			var modelAcquisitionClarificationMu sync.RWMutex
			getModelAcquisitionClarification := func() *state.ModelAcquisitionClarification {
				modelAcquisitionClarificationMu.RLock()
				defer modelAcquisitionClarificationMu.RUnlock()
				return cloneModelAcquisitionClarification(modelAcquisitionClarification)
			}
			setModelAcquisitionClarification := func(clarification *state.ModelAcquisitionClarification) {
				modelAcquisitionClarificationMu.Lock()
				modelAcquisitionClarification = cloneModelAcquisitionClarification(clarification)
				modelAcquisitionClarificationMu.Unlock()
			}
			buildModelAcquisitionPlan := func(snapshotValue core.WorkspaceSnapshot) *core.ModelAcquisitionPlan {
				clarification := getModelAcquisitionClarification()
				if clarification != nil && !state.ClarificationStillValid(clarification, snapshotValue) {
					return nil
				}
				return cloneModelAcquisitionPlan(modelAcquisitionPlanFromSelection(getSelectedModelPath(), clarification))
			}
			getModelAcquisitionPlan := func() *core.ModelAcquisitionPlan {
				return cloneModelAcquisitionPlan(modelAcquisitionPlanFromSelection(getSelectedModelPath(), getModelAcquisitionClarification()))
			}
			selectedEncoderMapping := cloneEncoderMappingContract(loadedState.ConfirmedEncoderMapping)
			var selectedEncoderMappingMu sync.RWMutex
			getSelectedEncoderMapping := func() *core.EncoderMappingContract {
				selectedEncoderMappingMu.RLock()
				defer selectedEncoderMappingMu.RUnlock()
				return cloneEncoderMappingContract(selectedEncoderMapping)
			}
			setSelectedEncoderMapping := func(mapping *core.EncoderMappingContract) {
				selectedEncoderMappingMu.Lock()
				selectedEncoderMapping = cloneEncoderMappingContract(mapping)
				selectedEncoderMappingMu.Unlock()
			}
			resolvedRuntimeProfile := cloneLocalRuntimeProfile(loadedState.RuntimeProfile)
			var resolvedRuntimeProfileMu sync.RWMutex
			getRuntimeProfile := func() *core.LocalRuntimeProfile {
				resolvedRuntimeProfileMu.RLock()
				defer resolvedRuntimeProfileMu.RUnlock()
				return cloneLocalRuntimeProfile(resolvedRuntimeProfile)
			}
			setRuntimeProfile := func(profile *core.LocalRuntimeProfile) {
				resolvedRuntimeProfileMu.Lock()
				resolvedRuntimeProfile = cloneLocalRuntimeProfile(profile)
				resolvedRuntimeProfileMu.Unlock()
			}
			runtimeResolver := inspect.NewPoetryRuntimeResolver()
			resolveRuntimeProfile := func(ctx context.Context, snapshotValue core.WorkspaceSnapshot) (*core.LocalRuntimeProfile, error) {
				resolution, err := runtimeResolver.Resolve(ctx, repoRoot, snapshotValue, getRuntimeProfile())
				if err != nil {
					return nil, err
				}
				if resolution.Profile == nil {
					setRuntimeProfile(nil)
					return nil, nil
				}
				if len(resolution.SuspiciousReasons) > 0 {
					if !yes {
						if nonInteractive {
							return nil, core.NewError(
								core.KindUnknown,
								"cli.run.runtime_confirmation_required",
								"Poetry resolved an unexpected runtime and this run is non-interactive; rerun interactively or pass --yes to accept it",
							)
						}
						approved, err := confirmRuntimeProfile(promptInput, cmd.OutOrStdout(), getRuntimeProfile(), resolution)
						if err != nil {
							return nil, err
						}
						if !approved {
							return nil, core.NewError(
								core.KindUnknown,
								"cli.run.runtime_confirmation_rejected",
								"runtime confirmation was rejected",
							)
						}
					}
					resolution.Profile.ConfirmationMode = "user_confirmed"
				}
				setRuntimeProfile(resolution.Profile)
				return cloneLocalRuntimeProfile(resolution.Profile), nil
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
			agentRunner := agent.NewRunner()
			agentRunner.SetObserver(liveEvents)
			agentExecutor := execute.NewAgentExecutor(agentRunner)
			agentExecutor.SetObserver(liveEvents)
			baseExecutor := execute.NewDispatcherExecutorWithAgent(agentExecutor)
			baseExecutor.SetObserver(liveEvents)

			stepApproval := func(step core.EnsureStep) (bool, error) {
				snapshotValue, hasSnapshot := plannerAdapter.LastSnapshot()
				status, hasStatus := plannerAdapter.LastStatus()

				if err := ensureModelPathSelectionForStep(
					step,
					snapshotValue,
					hasSnapshot,
					status,
					hasStatus,
					getSelectedModelPath,
					setSelectedModelPath,
					getModelAcquisitionClarification,
					setModelAcquisitionClarification,
					repoRoot,
					nonInteractive,
					promptInput,
					cmd.OutOrStdout(),
				); err != nil {
					return false, err
				}
				if err := ensureEncoderMappingForStep(
					step,
					snapshotValue,
					hasSnapshot,
					status,
					hasStatus,
					getSelectedEncoderMapping,
					setSelectedEncoderMapping,
					nonInteractive,
					yes,
					promptInput,
					cmd.OutOrStdout(),
				); err != nil {
					return false, err
				}

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
						"this run requires approval before I apply and commit changes; rerun with --yes to auto-approve in non-interactive mode",
					)
				}
				liveEvents.Emit(observe.Event{
					Kind:    observe.EventWaitingApproval,
					StepID:  step.ID,
					Message: "Waiting for your approval before making changes",
				})
				return promptApproval(
					promptInput,
					cmd.OutOrStdout(),
					stepApprovalMessage(step, snapshotValue, hasSnapshot, status, hasStatus, renderOptions.EnableColor),
					renderOptions.EnableColor,
				)
			}

			gitApproval := func(step core.EnsureStep, review gitmanager.ChangeReview) (gitmanager.ReviewDecision, error) {
				if yes {
					return gitmanager.ReviewDecision{KeepChanges: true, Commit: true}, nil
				}
				if nonInteractive {
					return gitmanager.ReviewDecision{}, core.NewError(
						core.KindUnknown,
						"cli.run.non_interactive.approval_required",
						"this run requires approval to keep or commit changes; rerun with --yes to auto-approve in non-interactive mode",
					)
				}
				liveEvents.Emit(observe.Event{
					Kind:    observe.EventGitReviewStarted,
					StepID:  step.ID,
					Message: "Waiting for your approval to review changes",
				})
				return promptChangeReviewApproval(
					promptInput,
					cmd.OutOrStdout(),
					step,
					review,
					changeReviewRenderOptions{EnableColor: renderOptions.EnableColor},
				)
			}

			engine, err := orchestrator.NewEngine(orchestrator.Dependencies{
				Snapshotter: modelPathHintSnapshotter{
					base:                   snapshot.NewGitSnapshotter(),
					selectedModelFn:        getSelectedModelPath,
					modelAcquisitionPlanFn: buildModelAcquisitionPlan,
					selectedMappingFn:      getSelectedEncoderMapping,
					runtimeProfileFn:       getRuntimeProfile,
					resolveRuntimeFn:       resolveRuntimeProfile,
				},
				Inspector: inspect.NewBaselineInspector(),
				Planner:   plannerAdapter,
				Executor: execute.NewApprovalExecutor(
					modelPathHintExecutor{
						base:                   baseExecutor,
						selectedModelFn:        getSelectedModelPath,
						modelAcquisitionPlanFn: getModelAcquisitionPlan,
						runtimeProfileFn:       getRuntimeProfile,
					},
					stepApproval,
				),
				GitManager: gitmanager.NewManager(gitApproval, gitmanager.ManagerOptions{ColorDiff: renderOptions.EnableColor}),
				Validator:  validate.NewBaselineValidator(),
				Reporter:   iterationReporter,
				Observer:   liveEvents,
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
					InitialBlockingIssues: func(snapshotValue core.WorkspaceSnapshot) []core.Issue {
						return state.FreshBlockingValidationIssues(initialState, snapshotValue, repoRoot)
					},
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

						nextState := state.UpdateForIteration(
							currentState,
							snapshotValue,
							*report,
							repoRoot,
							getSelectedModelPath(),
							getModelAcquisitionClarification(),
							getSelectedEncoderMapping(),
							getRuntimeProfile(),
							invalidationReasons,
						)
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
			case orchestrator.RunStopReasonInterrupted:
				return fmt.Errorf("the current Claude step was interrupted. review the latest output and rerun `concierge run` when you're ready to continue")
			case orchestrator.RunStopReasonNeedsUserAction:
				if lastReportHasEvidence(runResult.Reports, "git.commit_pending_review", "true") {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Changes are in your working tree for local review. After reviewing or committing them, rerun `concierge run`."); err != nil {
						return err
					}
					return nil
				}
				if lastReportHasEvidence(runResult.Reports, "executor.change_approval", "rejected") ||
					lastReportHasEvidence(runResult.Reports, "git.approval", "rejected") {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), "No changes were made because approval was not granted. Rerun `concierge run` when you're ready to continue."); err != nil {
						return err
					}
					return nil
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Manual step required outside Concierge. After completing the step above, rerun `concierge run`."); err != nil {
					return err
				}
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
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 0, "Maximum guided rounds before stopping (0 means unlimited)")
	cmd.Flags().BoolVar(&persist, "persist", false, "Persist reports and evidence under .concierge")
	cmd.Flags().StringVar(&projectRootFlag, "project-root", "", "Project root to operate on")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Fail instead of prompting for interactive decisions")
	cmd.Flags().BoolVar(&yes, "yes", false, "Auto-approve mutation/push prompts")
	cmd.Flags().StringVar(&modelPathFlag, "model-path", "", "Preferred model path for @tensorleap_load_model when multiple candidates exist")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colorized output")
	cmd.Flags().BoolVar(&debugOutput, "debug-output", false, "Show internal debug details in run output")
	return cmd
}

func lastReportHasEvidence(reports []core.IterationReport, name, value string) bool {
	if len(reports) == 0 {
		return false
	}
	for _, item := range reports[len(reports)-1].Evidence {
		if item.Name == name && item.Value == value {
			return true
		}
	}
	return false
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
	checkHeading := "Current step"
	if len(blockers) > 0 {
		checkLabel = core.HumanEnsureStepRequirementLabel(step.ID)
		checkHeading = "Missing integration step"
	}
	headingColor := ansiBold + ansiCyan
	if len(blockers) > 0 {
		headingColor = ansiBold + ansiYellow
	}
	checklist = append(checklist, "", paint(fmt.Sprintf("%s: %s", checkHeading, checkLabel), headingColor, enableColor))

	guidance := approvalGuidanceForStep(step.ID)
	if guidance.Explanation != "" {
		checklist = append(checklist, "Why this step matters: "+guidance.Explanation)
	}
	if guidance.DocsURL != "" {
		checklist = append(checklist, "Docs: "+guidance.DocsURL)
	}

	if len(blockers) > 0 {
		checklist = append(checklist, "What I'm seeing:")
		for i, issue := range blockers {
			if i >= 3 {
				checklist = append(checklist, "- Additional missing-step details were omitted for brevity.")
				break
			}
			message := strings.TrimSpace(issue.Message)
			if message == "" {
				message = "A required integration detail is still missing."
			}
			checklist = append(checklist, "- "+message)
		}
	}

	checklist = append(
		checklist,
		"",
	)
	if len(blockers) > 0 {
		checklist = append(checklist, "I can continue by addressing this missing step now.")
	} else {
		checklist = append(checklist, "I can continue with this step now.")
	}
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
		case state.InvalidationReasonRuntimePyProjectChanged:
			labels = append(labels, "pyproject.toml changed")
		case state.InvalidationReasonRuntimePoetryLockChanged:
			labels = append(labels, "poetry.lock changed")
		case state.InvalidationReasonRuntimeInterpreterChanged:
			labels = append(labels, "Poetry interpreter changed")
		case state.InvalidationReasonRuntimePythonVersionChanged:
			labels = append(labels, "Poetry Python version changed")
		default:
			labels = append(labels, "workspace changed")
		}
	}

	return fmt.Sprintf("Your workspace changed since the previous run (%s), so I re-checked everything.", strings.Join(labels, ", "))
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
	if snapshot.ConfirmedEncoderMapping != nil {
		cloned.ConfirmedEncoderMapping = cloneEncoderMappingContract(snapshot.ConfirmedEncoderMapping)
	}
	if snapshot.ModelAcquisitionPlan != nil {
		cloned.ModelAcquisitionPlan = cloneModelAcquisitionPlan(snapshot.ModelAcquisitionPlan)
	}
	if len(snapshot.FileHashes) > 0 {
		cloned.FileHashes = make(map[string]string, len(snapshot.FileHashes))
		for key, value := range snapshot.FileHashes {
			cloned.FileHashes[key] = value
		}
	}
	if len(snapshot.Runtime.RequirementsFiles) > 0 {
		cloned.Runtime.RequirementsFiles = append([]string(nil), snapshot.Runtime.RequirementsFiles...)
	}
	if snapshot.RuntimeProfile != nil {
		cloned.RuntimeProfile = cloneLocalRuntimeProfile(snapshot.RuntimeProfile)
	}
	return cloned
}

func cloneEncoderMappingContract(mapping *core.EncoderMappingContract) *core.EncoderMappingContract {
	if mapping == nil {
		return nil
	}
	cloned := *mapping
	if len(mapping.InputSymbols) > 0 {
		cloned.InputSymbols = append([]string(nil), mapping.InputSymbols...)
	}
	if len(mapping.GroundTruthSymbols) > 0 {
		cloned.GroundTruthSymbols = append([]string(nil), mapping.GroundTruthSymbols...)
	}
	if len(mapping.Notes) > 0 {
		cloned.Notes = append([]string(nil), mapping.Notes...)
	}
	return &cloned
}

func cloneLocalRuntimeProfile(profile *core.LocalRuntimeProfile) *core.LocalRuntimeProfile {
	if profile == nil {
		return nil
	}
	cloned := *profile
	cloned.Fingerprint = profile.Fingerprint
	return &cloned
}

func cloneIntegrationStatus(status core.IntegrationStatus) core.IntegrationStatus {
	cloned := core.IntegrationStatus{}
	if len(status.Missing) > 0 {
		cloned.Missing = append([]string(nil), status.Missing...)
	}
	if len(status.Issues) > 0 {
		cloned.Issues = append([]core.Issue(nil), status.Issues...)
	}
	if status.Contracts != nil {
		contracts := *status.Contracts
		if len(status.Contracts.LoadModelFunctions) > 0 {
			contracts.LoadModelFunctions = append([]string(nil), status.Contracts.LoadModelFunctions...)
		}
		if len(status.Contracts.PreprocessFunctions) > 0 {
			contracts.PreprocessFunctions = append([]string(nil), status.Contracts.PreprocessFunctions...)
		}
		if len(status.Contracts.InputEncoders) > 0 {
			contracts.InputEncoders = append([]string(nil), status.Contracts.InputEncoders...)
		}
		if len(status.Contracts.GroundTruthEncoders) > 0 {
			contracts.GroundTruthEncoders = append([]string(nil), status.Contracts.GroundTruthEncoders...)
		}
		if len(status.Contracts.IntegrationTestFunctions) > 0 {
			contracts.IntegrationTestFunctions = append([]string(nil), status.Contracts.IntegrationTestFunctions...)
		}
		if len(status.Contracts.IntegrationTestCalls) > 0 {
			contracts.IntegrationTestCalls = append([]string(nil), status.Contracts.IntegrationTestCalls...)
		}
		if len(status.Contracts.ModelCandidates) > 0 {
			contracts.ModelCandidates = append([]core.ModelCandidate(nil), status.Contracts.ModelCandidates...)
		}
		if status.Contracts.ModelAcquisition != nil {
			acquisition := *status.Contracts.ModelAcquisition
			if len(status.Contracts.ModelAcquisition.ReadyArtifacts) > 0 {
				acquisition.ReadyArtifacts = append([]core.ModelCandidate(nil), status.Contracts.ModelAcquisition.ReadyArtifacts...)
			}
			if len(status.Contracts.ModelAcquisition.PassiveLeads) > 0 {
				acquisition.PassiveLeads = append([]core.ModelCandidate(nil), status.Contracts.ModelAcquisition.PassiveLeads...)
			}
			if len(status.Contracts.ModelAcquisition.AcquisitionLeads) > 0 {
				acquisition.AcquisitionLeads = append([]string(nil), status.Contracts.ModelAcquisition.AcquisitionLeads...)
			}
			if status.Contracts.ModelAcquisition.AgentPromptBundle != nil {
				promptBundle := *status.Contracts.ModelAcquisition.AgentPromptBundle
				acquisition.AgentPromptBundle = &promptBundle
			}
			if status.Contracts.ModelAcquisition.AgentRawOutput != nil {
				rawOutput := *status.Contracts.ModelAcquisition.AgentRawOutput
				if len(status.Contracts.ModelAcquisition.AgentRawOutput.Metadata) > 0 {
					rawOutput.Metadata = make(map[string]string, len(status.Contracts.ModelAcquisition.AgentRawOutput.Metadata))
					for key, value := range status.Contracts.ModelAcquisition.AgentRawOutput.Metadata {
						rawOutput.Metadata[key] = value
					}
				}
				acquisition.AgentRawOutput = &rawOutput
			}
			if status.Contracts.ModelAcquisition.NormalizedPlan != nil {
				plan := *status.Contracts.ModelAcquisition.NormalizedPlan
				if len(status.Contracts.ModelAcquisition.NormalizedPlan.RuntimeInvocation) > 0 {
					plan.RuntimeInvocation = append([]string(nil), status.Contracts.ModelAcquisition.NormalizedPlan.RuntimeInvocation...)
				}
				if len(status.Contracts.ModelAcquisition.NormalizedPlan.Evidence) > 0 {
					plan.Evidence = append([]core.ModelAcquisitionPlanEvidence(nil), status.Contracts.ModelAcquisition.NormalizedPlan.Evidence...)
				}
				acquisition.NormalizedPlan = &plan
			}
			if status.Contracts.ModelAcquisition.Materialization != nil {
				materialization := *status.Contracts.ModelAcquisition.Materialization
				if len(status.Contracts.ModelAcquisition.Materialization.Command) > 0 {
					materialization.Command = append([]string(nil), status.Contracts.ModelAcquisition.Materialization.Command...)
				}
				if len(status.Contracts.ModelAcquisition.Materialization.Notes) > 0 {
					materialization.Notes = append([]string(nil), status.Contracts.ModelAcquisition.Materialization.Notes...)
				}
				acquisition.Materialization = &materialization
			}
			contracts.ModelAcquisition = &acquisition
		}
		if len(status.Contracts.DiscoveredInputSymbols) > 0 {
			contracts.DiscoveredInputSymbols = append([]string(nil), status.Contracts.DiscoveredInputSymbols...)
		}
		if len(status.Contracts.DiscoveredGroundTruthSymbols) > 0 {
			contracts.DiscoveredGroundTruthSymbols = append([]string(nil), status.Contracts.DiscoveredGroundTruthSymbols...)
		}
		if status.Contracts.ConfirmedMapping != nil {
			confirmed := *status.Contracts.ConfirmedMapping
			if len(status.Contracts.ConfirmedMapping.InputSymbols) > 0 {
				confirmed.InputSymbols = append([]string(nil), status.Contracts.ConfirmedMapping.InputSymbols...)
			}
			if len(status.Contracts.ConfirmedMapping.GroundTruthSymbols) > 0 {
				confirmed.GroundTruthSymbols = append([]string(nil), status.Contracts.ConfirmedMapping.GroundTruthSymbols...)
			}
			if len(status.Contracts.ConfirmedMapping.Notes) > 0 {
				confirmed.Notes = append([]string(nil), status.Contracts.ConfirmedMapping.Notes...)
			}
			contracts.ConfirmedMapping = &confirmed
		}
		if status.Contracts.InputGTDiscovery != nil {
			discovery := *status.Contracts.InputGTDiscovery
			if status.Contracts.InputGTDiscovery.FixtureState != nil {
				fixtureState := *status.Contracts.InputGTDiscovery.FixtureState
				discovery.FixtureState = &fixtureState
			}
			if status.Contracts.InputGTDiscovery.LeadPack != nil {
				leadPack := *status.Contracts.InputGTDiscovery.LeadPack
				if len(status.Contracts.InputGTDiscovery.LeadPack.Signals) > 0 {
					leadPack.Signals = append([]core.InputGTLeadSignal(nil), status.Contracts.InputGTDiscovery.LeadPack.Signals...)
				}
				if len(status.Contracts.InputGTDiscovery.LeadPack.Files) > 0 {
					leadPack.Files = append([]core.InputGTLeadFile(nil), status.Contracts.InputGTDiscovery.LeadPack.Files...)
				}
				if len(status.Contracts.InputGTDiscovery.LeadPack.FrameworkDetection.Evidence) > 0 {
					leadPack.FrameworkDetection.Evidence = append([]core.InputGTFrameworkEvidence(nil), status.Contracts.InputGTDiscovery.LeadPack.FrameworkDetection.Evidence...)
				}
				discovery.LeadPack = &leadPack
			}
			if status.Contracts.InputGTDiscovery.AgentPromptBundle != nil {
				promptBundle := *status.Contracts.InputGTDiscovery.AgentPromptBundle
				discovery.AgentPromptBundle = &promptBundle
			}
			if status.Contracts.InputGTDiscovery.AgentRawOutput != nil {
				rawOutput := *status.Contracts.InputGTDiscovery.AgentRawOutput
				if len(status.Contracts.InputGTDiscovery.AgentRawOutput.Metadata) > 0 {
					rawOutput.Metadata = make(map[string]string, len(status.Contracts.InputGTDiscovery.AgentRawOutput.Metadata))
					for key, value := range status.Contracts.InputGTDiscovery.AgentRawOutput.Metadata {
						rawOutput.Metadata[key] = value
					}
				}
				discovery.AgentRawOutput = &rawOutput
			}
			if status.Contracts.InputGTDiscovery.NormalizedFindings != nil {
				findings := *status.Contracts.InputGTDiscovery.NormalizedFindings
				if len(status.Contracts.InputGTDiscovery.NormalizedFindings.Inputs) > 0 {
					findings.Inputs = append([]core.InputGTCandidate(nil), status.Contracts.InputGTDiscovery.NormalizedFindings.Inputs...)
				}
				if len(status.Contracts.InputGTDiscovery.NormalizedFindings.GroundTruths) > 0 {
					findings.GroundTruths = append([]core.InputGTCandidate(nil), status.Contracts.InputGTDiscovery.NormalizedFindings.GroundTruths...)
				}
				if len(status.Contracts.InputGTDiscovery.NormalizedFindings.ProposedMapping) > 0 {
					findings.ProposedMapping = append([]core.InputGTProposedMapping(nil), status.Contracts.InputGTDiscovery.NormalizedFindings.ProposedMapping...)
				}
				if len(status.Contracts.InputGTDiscovery.NormalizedFindings.Unknowns) > 0 {
					findings.Unknowns = append([]string(nil), status.Contracts.InputGTDiscovery.NormalizedFindings.Unknowns...)
				}
				discovery.NormalizedFindings = &findings
			}
			if status.Contracts.InputGTDiscovery.ComparisonReport != nil {
				comparison := *status.Contracts.InputGTDiscovery.ComparisonReport
				if len(status.Contracts.InputGTDiscovery.ComparisonReport.PrimaryInputSymbols) > 0 {
					comparison.PrimaryInputSymbols = append([]string(nil), status.Contracts.InputGTDiscovery.ComparisonReport.PrimaryInputSymbols...)
				}
				if len(status.Contracts.InputGTDiscovery.ComparisonReport.PrimaryGroundTruthSymbols) > 0 {
					comparison.PrimaryGroundTruthSymbols = append([]string(nil), status.Contracts.InputGTDiscovery.ComparisonReport.PrimaryGroundTruthSymbols...)
				}
				if len(status.Contracts.InputGTDiscovery.ComparisonReport.ConditionalInputSymbols) > 0 {
					comparison.ConditionalInputSymbols = append([]string(nil), status.Contracts.InputGTDiscovery.ComparisonReport.ConditionalInputSymbols...)
				}
				if len(status.Contracts.InputGTDiscovery.ComparisonReport.ConditionalGroundTruthSymbols) > 0 {
					comparison.ConditionalGroundTruthSymbols = append([]string(nil), status.Contracts.InputGTDiscovery.ComparisonReport.ConditionalGroundTruthSymbols...)
				}
				if len(status.Contracts.InputGTDiscovery.ComparisonReport.RuntimeInputSymbols) > 0 {
					comparison.RuntimeInputSymbols = append([]string(nil), status.Contracts.InputGTDiscovery.ComparisonReport.RuntimeInputSymbols...)
				}
				if len(status.Contracts.InputGTDiscovery.ComparisonReport.RuntimeOnlyInputSymbols) > 0 {
					comparison.RuntimeOnlyInputSymbols = append([]string(nil), status.Contracts.InputGTDiscovery.ComparisonReport.RuntimeOnlyInputSymbols...)
				}
				if len(status.Contracts.InputGTDiscovery.ComparisonReport.DiscoveryOnlyInputSymbols) > 0 {
					comparison.DiscoveryOnlyInputSymbols = append([]string(nil), status.Contracts.InputGTDiscovery.ComparisonReport.DiscoveryOnlyInputSymbols...)
				}
				if len(status.Contracts.InputGTDiscovery.ComparisonReport.Notes) > 0 {
					comparison.Notes = append([]string(nil), status.Contracts.InputGTDiscovery.ComparisonReport.Notes...)
				}
				discovery.ComparisonReport = &comparison
			}
			contracts.InputGTDiscovery = &discovery
		}
		cloned.Contracts = &contracts
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
	checklist := []string{paint("I'm checking the Tensorleap integration's progress:", ansiCyan, enableColor)}
	if !hasSnapshot || !hasStatus {
		return checklist
	}

	checks := core.BuildVerifiedChecks(snapshot, status.Issues, nil, step.ID)
	for _, check := range core.VisibleChecksForFlow(checks) {
		icon := promptCheckIcon(check.Status, enableColor)
		label := promptCheckLabel(check)
		if check.Blocking {
			label += " (missing step)"
		}
		label = paint(label, promptCheckLabelColor(check.Status), enableColor)
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

func promptCheckLabelColor(status core.CheckStatus) string {
	switch status {
	case core.CheckStatusPass:
		return ansiGreen
	case core.CheckStatusWarning:
		return ansiYellow
	case core.CheckStatusFail:
		return ansiBold + ansiYellow
	default:
		return ansiDim
	}
}

type stepApprovalGuidance struct {
	Explanation string
	DocsURL     string
}

func approvalGuidanceForStep(stepID core.EnsureStepID) stepApprovalGuidance {
	switch stepID {
	case core.EnsureStepRepositoryContext:
		return stepApprovalGuidance{
			Explanation: "I need a valid Git project root and a safe working branch before applying fixes.",
		}
	case core.EnsureStepPythonRuntime:
		return stepApprovalGuidance{
			Explanation: "Concierge needs a Poetry environment for this project and the packages required to run local Tensorleap checks.",
			DocsURL:     stepGuideWritingIntegrationURL,
		}
	case core.EnsureStepLeapCLIAuth:
		return stepApprovalGuidance{
			Explanation: "I need a working and authenticated leap CLI to validate and upload integrations.",
			DocsURL:     leapCLIInstallGuideURL,
		}
	case core.EnsureStepServerConnectivity:
		return stepApprovalGuidance{
			Explanation: "The Tensorleap server must be reachable so I can verify mounts and run upload readiness checks.",
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
			Explanation: "@tensorleap_load_model now needs to be wired to the materialized supported artifact so later validation can execute against a stable path.",
			DocsURL:     stepGuideModelIntegrationURL,
		}
	case core.EnsureStepModelAcquisition:
		return stepApprovalGuidance{
			Explanation: "I need to find the repository’s model download/export path and materialize one supported .onnx/.h5 artifact locally before model wiring can be completed.",
			DocsURL:     stepGuideModelIntegrationURL,
		}
	case core.EnsureStepIntegrationScript:
		return stepApprovalGuidance{
			Explanation: "Tensorleap integration code must live in the root leap_integration.py entrypoint that Concierge manages.",
			DocsURL:     stepGuideWritingIntegrationURL,
		}
	case core.EnsureStepPreprocessContract:
		return stepApprovalGuidance{
			Explanation: "Tensorleap needs preprocessing that produces both train and validation subsets so integration checks can run end-to-end.",
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
	case core.EnsureStepIntegrationTestContract, core.EnsureStepIntegrationTestWiring:
		return stepApprovalGuidance{
			Explanation: "The integration test defines which interfaces Tensorleap actually executes during analysis, so it must stay thin and only wire decorated calls plus model inference.",
			DocsURL:     stepGuideIntegrationTestURL,
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
			Explanation: "I found an unmapped missing step and need to inspect it before suggesting the next deterministic fix.",
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

type modelPathHintSnapshotter struct {
	base                   ports.Snapshotter
	selectedModelFn        func() string
	modelAcquisitionPlanFn func(core.WorkspaceSnapshot) *core.ModelAcquisitionPlan
	selectedMappingFn      func() *core.EncoderMappingContract
	runtimeProfileFn       func() *core.LocalRuntimeProfile
	resolveRuntimeFn       func(context.Context, core.WorkspaceSnapshot) (*core.LocalRuntimeProfile, error)
}

func (s modelPathHintSnapshotter) Snapshot(ctx context.Context, request core.SnapshotRequest) (core.WorkspaceSnapshot, error) {
	if s.base == nil {
		return core.WorkspaceSnapshot{}, core.NewError(core.KindMissingDependency, "cli.run.snapshotter", "snapshotter is required")
	}
	snapshotValue, err := s.base.Snapshot(ctx, request)
	if err != nil {
		return core.WorkspaceSnapshot{}, err
	}
	if s.selectedModelFn != nil {
		snapshotValue.SelectedModelPath = normalizeModelPathValue(s.selectedModelFn())
	}
	if s.modelAcquisitionPlanFn != nil {
		snapshotValue.ModelAcquisitionPlan = cloneModelAcquisitionPlan(s.modelAcquisitionPlanFn(snapshotValue))
	}
	if s.selectedMappingFn != nil {
		snapshotValue.ConfirmedEncoderMapping = cloneEncoderMappingContract(s.selectedMappingFn())
	}
	if s.resolveRuntimeFn != nil {
		profile, err := s.resolveRuntimeFn(ctx, snapshotValue)
		if err != nil {
			return core.WorkspaceSnapshot{}, err
		}
		snapshotValue.RuntimeProfile = cloneLocalRuntimeProfile(profile)
		if profile != nil {
			snapshotValue.Runtime.ResolvedInterpreter = strings.TrimSpace(profile.InterpreterPath)
			snapshotValue.Runtime.ResolvedPythonVersion = strings.TrimSpace(profile.PythonVersion)
		}
	} else if s.runtimeProfileFn != nil {
		snapshotValue.RuntimeProfile = cloneLocalRuntimeProfile(s.runtimeProfileFn())
	}
	return snapshotValue, nil
}

type modelPathHintExecutor struct {
	base                   ports.Executor
	selectedModelFn        func() string
	modelAcquisitionPlanFn func() *core.ModelAcquisitionPlan
	runtimeProfileFn       func() *core.LocalRuntimeProfile
}

func (e modelPathHintExecutor) Execute(ctx context.Context, snapshotValue core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	if e.base == nil {
		return core.ExecutionResult{}, core.NewError(core.KindMissingDependency, "cli.run.executor", "executor is required")
	}
	if e.selectedModelFn != nil {
		selectedPath := normalizeModelPathValue(e.selectedModelFn())
		if selectedPath != "" {
			snapshotValue.SelectedModelPath = selectedPath
		}
	}
	if e.modelAcquisitionPlanFn != nil {
		snapshotValue.ModelAcquisitionPlan = cloneModelAcquisitionPlan(e.modelAcquisitionPlanFn())
	}
	if e.runtimeProfileFn != nil {
		snapshotValue.RuntimeProfile = cloneLocalRuntimeProfile(e.runtimeProfileFn())
		if snapshotValue.RuntimeProfile != nil {
			snapshotValue.Runtime.ResolvedInterpreter = strings.TrimSpace(snapshotValue.RuntimeProfile.InterpreterPath)
			snapshotValue.Runtime.ResolvedPythonVersion = strings.TrimSpace(snapshotValue.RuntimeProfile.PythonVersion)
		}
	}
	return e.base.Execute(ctx, snapshotValue, step)
}

func ensureModelPathSelectionForStep(
	step core.EnsureStep,
	snapshotValue core.WorkspaceSnapshot,
	hasSnapshot bool,
	status core.IntegrationStatus,
	hasStatus bool,
	getSelected func() string,
	setSelected func(string),
	getClarification func() *state.ModelAcquisitionClarification,
	setClarification func(*state.ModelAcquisitionClarification),
	repoRoot string,
	requireNonInteractive bool,
	input *bufio.Reader,
	out io.Writer,
) error {
	_ = repoRoot
	if step.ID != core.EnsureStepModelAcquisition && step.ID != core.EnsureStepModelContract {
		return nil
	}
	if !hasStatus || status.Contracts == nil {
		return nil
	}

	current := ""
	if getSelected != nil {
		current = normalizeModelPathValue(getSelected())
	}
	clarification := (*state.ModelAcquisitionClarification)(nil)
	if getClarification != nil {
		clarification = cloneModelAcquisitionClarification(getClarification())
	}
	if hasSnapshot && clarification != nil && !state.ClarificationStillValid(clarification, snapshotValue) {
		if current != "" && current == normalizeModelPathValue(clarification.SelectedVerifiedModelPath) {
			if setSelected != nil {
				setSelected("")
			}
			current = ""
		}
		if setClarification != nil {
			setClarification(nil)
		}
		clarification = nil
	}

	if verifiedCandidates := ambiguousVerifiedModelCandidates(status); len(verifiedCandidates) > 0 {
		if clarification != nil {
			selected := normalizeModelPathValue(clarification.SelectedVerifiedModelPath)
			if selected != "" && containsModelPath(verifiedCandidates, selected) {
				if setSelected != nil {
					setSelected(selected)
				}
				return nil
			}
		}
		if requireNonInteractive {
			return core.NewError(
				core.KindUnknown,
				"cli.run.model_source_selection_required",
				"model source clarification is required; rerun interactively and choose which verified model file Concierge should follow",
			)
		}
		selected, err := promptModelSourceSelection(input, out, verifiedCandidates)
		if err != nil {
			return err
		}
		if setSelected != nil {
			setSelected(selected)
		}
		if setClarification != nil {
			setClarification(
				newModelAcquisitionClarification(
					snapshotValue,
					hasSnapshot,
					selected,
					"",
					"",
				),
			)
		}
		return nil
	}

	if needsModelSourceClarification(status) {
		if clarification != nil &&
			strings.TrimSpace(clarification.ModelSourceNote) != "" &&
			clarification.RuntimeChangePolicy != "" {
			return nil
		}
		if requireNonInteractive {
			return core.NewError(
				core.KindUnknown,
				"cli.run.model_source_note_required",
				"model acquisition clarification is required; rerun interactively and describe where Concierge should obtain the model",
			)
		}
		note, err := promptModelSourceNote(input, out)
		if err != nil {
			return err
		}
		allowRuntimeChanges, err := promptYesNo(
			input,
			out,
			"If the current runtime cannot load the intended model, may Concierge change runtime or dependency versions? [y/N]:",
			false,
		)
		if err != nil {
			return err
		}
		policy := state.ModelRuntimeChangePolicyStayInCurrentRuntime
		if allowRuntimeChanges {
			policy = state.ModelRuntimeChangePolicyAllowRuntimeChanges
		}
		if setClarification != nil {
			setClarification(
				newModelAcquisitionClarification(
					snapshotValue,
					hasSnapshot,
					"",
					note,
					policy,
				),
			)
		}
		return nil
	}

	if current != "" {
		return nil
	}
	return nil
}

func ensureEncoderMappingForStep(
	step core.EnsureStep,
	snapshotValue core.WorkspaceSnapshot,
	hasSnapshot bool,
	status core.IntegrationStatus,
	hasStatus bool,
	getSelected func() *core.EncoderMappingContract,
	setSelected func(*core.EncoderMappingContract),
	nonInteractive bool,
	yes bool,
	input *bufio.Reader,
	out io.Writer,
) error {
	switch step.ID {
	case core.EnsureStepInputEncoders, core.EnsureStepGroundTruthEncoders, core.EnsureStepIntegrationTestContract, core.EnsureStepIntegrationTestWiring:
	default:
		return nil
	}
	if !hasStatus || status.Contracts == nil {
		return nil
	}

	proposed, ok := proposedEncoderMappingFromContracts(status.Contracts)
	if !ok {
		return nil
	}

	current := (*core.EncoderMappingContract)(nil)
	if getSelected != nil {
		current = getSelected()
	}
	if mappingIsValidForSnapshot(current, snapshotValue, hasSnapshot) {
		return nil
	}

	if nonInteractive || yes {
		accepted := finalizeEncoderMappingProposal(proposed, snapshotValue, hasSnapshot, []string{"auto_accepted"})
		if setSelected != nil {
			setSelected(accepted)
		}
		return nil
	}

	if out == nil {
		out = io.Discard
	}
	if _, err := fmt.Fprintln(out, "Encoder Mapping Confirmation"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Review required input and ground-truth names before encoder authoring continues:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "  inputs: %s\n", renderSymbolList(proposed.InputSymbols)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "  ground truths: %s\n", renderSymbolList(proposed.GroundTruthSymbols)); err != nil {
		return err
	}

	approve, err := promptYesNo(input, out, "Accept this mapping? [Y/n]:", true)
	if err != nil {
		return err
	}
	if approve {
		accepted := finalizeEncoderMappingProposal(proposed, snapshotValue, hasSnapshot, []string{"user_accepted"})
		if setSelected != nil {
			setSelected(accepted)
		}
		return nil
	}

	if _, err := fmt.Fprintln(out, "Enter adjusted symbols as comma-separated lists. Leave blank to keep the suggestion."); err != nil {
		return err
	}
	if _, err := fmt.Fprint(out, "Input symbols: "); err != nil {
		return err
	}
	inputLine, err := readPromptLine(input)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(out, "Ground-truth symbols: "); err != nil {
		return err
	}
	gtLine, err := readPromptLine(input)
	if err != nil {
		return err
	}

	adjustedInputs := proposed.InputSymbols
	if strings.TrimSpace(inputLine) != "" {
		adjustedInputs = parseSymbolCSV(inputLine)
	}
	adjustedGroundTruths := proposed.GroundTruthSymbols
	if strings.TrimSpace(gtLine) != "" {
		adjustedGroundTruths = parseSymbolCSV(gtLine)
	}
	if len(adjustedInputs) == 0 && len(adjustedGroundTruths) == 0 {
		return core.NewError(
			core.KindUnknown,
			"cli.run.encoder_mapping.empty",
			"encoder mapping must include at least one input or one ground-truth symbol",
		)
	}

	accepted := finalizeEncoderMappingProposal(&core.EncoderMappingContract{
		InputSymbols:       adjustedInputs,
		GroundTruthSymbols: adjustedGroundTruths,
	}, snapshotValue, hasSnapshot, []string{"user_adjusted"})
	if setSelected != nil {
		setSelected(accepted)
	}
	return nil
}

func proposedEncoderMappingFromContracts(contracts *core.IntegrationContracts) (*core.EncoderMappingContract, bool) {
	if contracts == nil {
		return nil, false
	}
	if contracts.ConfirmedMapping != nil && mappingHasSymbols(contracts.ConfirmedMapping) {
		return cloneEncoderMappingContract(contracts.ConfirmedMapping), true
	}

	inputSymbols := uniqueSortedLowerSymbols(contracts.DiscoveredInputSymbols)
	groundTruthSymbols := uniqueSortedLowerSymbols(contracts.DiscoveredGroundTruthSymbols)
	if contracts.InputGTDiscovery != nil && contracts.InputGTDiscovery.ComparisonReport != nil {
		if len(contracts.InputGTDiscovery.ComparisonReport.PrimaryInputSymbols) > 0 {
			inputSymbols = uniqueSortedLowerSymbols(contracts.InputGTDiscovery.ComparisonReport.PrimaryInputSymbols)
		}
		if len(contracts.InputGTDiscovery.ComparisonReport.PrimaryGroundTruthSymbols) > 0 {
			groundTruthSymbols = uniqueSortedLowerSymbols(contracts.InputGTDiscovery.ComparisonReport.PrimaryGroundTruthSymbols)
		}
	}
	if len(inputSymbols) == 0 && len(groundTruthSymbols) == 0 {
		return nil, false
	}
	return &core.EncoderMappingContract{
		InputSymbols:       inputSymbols,
		GroundTruthSymbols: groundTruthSymbols,
	}, true
}

func mappingHasSymbols(mapping *core.EncoderMappingContract) bool {
	if mapping == nil {
		return false
	}
	return len(mapping.InputSymbols) > 0 || len(mapping.GroundTruthSymbols) > 0
}

func mappingIsValidForSnapshot(mapping *core.EncoderMappingContract, snapshotValue core.WorkspaceSnapshot, hasSnapshot bool) bool {
	if !mappingHasSymbols(mapping) {
		return false
	}
	if !hasSnapshot {
		return true
	}
	fingerprint := strings.TrimSpace(snapshotValue.WorktreeFingerprint)
	if fingerprint == "" {
		return true
	}
	return strings.TrimSpace(mapping.SourceFingerprint) == fingerprint
}

func finalizeEncoderMappingProposal(
	mapping *core.EncoderMappingContract,
	snapshotValue core.WorkspaceSnapshot,
	hasSnapshot bool,
	notes []string,
) *core.EncoderMappingContract {
	if mapping == nil {
		return nil
	}
	final := cloneEncoderMappingContract(mapping)
	final.InputSymbols = uniqueSortedLowerSymbols(final.InputSymbols)
	final.GroundTruthSymbols = uniqueSortedLowerSymbols(final.GroundTruthSymbols)
	if hasSnapshot {
		final.SourceFingerprint = strings.TrimSpace(snapshotValue.WorktreeFingerprint)
	}
	final.AcceptedAt = time.Now().UTC()
	if len(notes) > 0 {
		final.Notes = append(final.Notes, notes...)
	}
	final.Notes = uniqueSortedLowerSymbols(final.Notes)
	return final
}

func uniqueSortedLowerSymbols(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	sort.Strings(normalized)
	return normalized
}

func parseSymbolCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	return uniqueSortedLowerSymbols(parts)
}

func renderSymbolList(values []string) string {
	symbols := uniqueSortedLowerSymbols(values)
	if len(symbols) == 0 {
		return "(none)"
	}
	return strings.Join(symbols, ", ")
}

func selectableModelCandidates(candidates []core.ModelCandidate) []string {
	if len(candidates) == 0 {
		return nil
	}
	values := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		path := normalizeModelPathValue(candidate.Path)
		if path == "" {
			continue
		}
		if strings.HasPrefix(path, "../") || strings.HasPrefix(path, "/") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".onnx" && ext != ".h5" {
			continue
		}
		key := strings.ToLower(path)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, path)
	}
	sort.Strings(values)
	return values
}

func normalizeModelPathOption(repoRoot string, rawPath string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", nil
	}
	path := filepath.FromSlash(trimmed)
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	path = filepath.Clean(path)
	relPath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return "", core.WrapError(core.KindUnknown, "cli.run.model_path_rel", err)
	}
	if strings.HasPrefix(relPath, "..") || strings.HasPrefix(filepath.ToSlash(relPath), "../") {
		return "", core.NewError(core.KindUnknown, "cli.run.model_path_outside_repo", "model path must stay inside the project repository")
	}
	normalized := normalizeModelPathValue(relPath)
	if normalized == "" {
		return "", core.NewError(core.KindUnknown, "cli.run.model_path_invalid", "model path must not be empty")
	}
	ext := strings.ToLower(filepath.Ext(normalized))
	if ext != ".onnx" && ext != ".h5" {
		return "", core.NewError(core.KindUnknown, "cli.run.model_path_extension", "model path must end with .onnx or .h5")
	}
	return normalized, nil
}

func normalizeModelPathValue(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))
}

func cloneModelAcquisitionClarification(clarification *state.ModelAcquisitionClarification) *state.ModelAcquisitionClarification {
	if clarification == nil {
		return nil
	}
	cloned := *clarification
	cloned.SelectedVerifiedModelPath = normalizeModelPathValue(clarification.SelectedVerifiedModelPath)
	cloned.ModelSourceNote = strings.TrimSpace(clarification.ModelSourceNote)
	cloned.SnapshotHead = strings.TrimSpace(clarification.SnapshotHead)
	cloned.WorktreeFingerprint = strings.TrimSpace(clarification.WorktreeFingerprint)
	return &cloned
}

func newModelAcquisitionClarification(
	snapshotValue core.WorkspaceSnapshot,
	hasSnapshot bool,
	selectedVerifiedModelPath string,
	modelSourceNote string,
	runtimeChangePolicy state.ModelRuntimeChangePolicy,
) *state.ModelAcquisitionClarification {
	clarification := &state.ModelAcquisitionClarification{
		SelectedVerifiedModelPath: normalizeModelPathValue(selectedVerifiedModelPath),
		ModelSourceNote:           strings.TrimSpace(modelSourceNote),
		RuntimeChangePolicy:       runtimeChangePolicy,
	}
	if !hasSnapshot {
		return clarification
	}
	clarification.SnapshotHead = strings.TrimSpace(snapshotValue.Repository.Head)
	clarification.WorktreeFingerprint = strings.TrimSpace(snapshotValue.WorktreeFingerprint)
	if snapshotValue.RuntimeProfile != nil {
		clarification.RuntimeFingerprint = snapshotValue.RuntimeProfile.Fingerprint
	}
	return clarification
}

func ambiguousVerifiedModelCandidates(status core.IntegrationStatus) []string {
	if !hasModelIssueCode(status.Issues, core.IssueCodeModelCandidatesAmbiguous) || status.Contracts == nil {
		return nil
	}
	values := make([]string, 0, len(status.Contracts.ModelCandidates))
	for _, candidate := range status.Contracts.ModelCandidates {
		if candidate.VerificationState != core.ModelCandidateVerificationStateVerified {
			continue
		}
		path := normalizeModelPathValue(candidate.Path)
		if path == "" {
			continue
		}
		values = append(values, path)
	}
	sort.Strings(values)
	return uniqueStringSlice(values)
}

func needsModelSourceClarification(status core.IntegrationStatus) bool {
	if !hasModelIssueCode(status.Issues, core.IssueCodeModelAcquisitionUnresolved) || status.Contracts == nil {
		return false
	}
	for _, candidate := range status.Contracts.ModelCandidates {
		if candidate.VerificationState == core.ModelCandidateVerificationStateFailed {
			return true
		}
	}
	if status.Contracts.ModelAcquisition == nil {
		return false
	}
	return len(status.Contracts.ModelAcquisition.AcquisitionLeads) > 0 ||
		len(status.Contracts.ModelAcquisition.PassiveLeads) > 0
}

func hasModelIssueCode(issues []core.Issue, code core.IssueCode) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func containsModelPath(values []string, target string) bool {
	normalizedTarget := normalizeModelPathValue(target)
	for _, value := range values {
		if normalizeModelPathValue(value) == normalizedTarget {
			return true
		}
	}
	return false
}

func uniqueStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		unique = append(unique, normalized)
	}
	return unique
}
