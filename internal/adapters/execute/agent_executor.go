package execute

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/agent"
	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/persistence"
)

type agentTaskRunner interface {
	Run(ctx context.Context, task agent.AgentTask) (agent.AgentResult, error)
}

// AgentExecutor delegates complex integration objectives to an external coding agent.
type AgentExecutor struct {
	runner agentTaskRunner
}

// NewAgentExecutor creates an agent-backed executor.
func NewAgentExecutor(runner agentTaskRunner) *AgentExecutor {
	if runner == nil {
		runner = agent.NewRunner()
	}
	return &AgentExecutor{runner: runner}
}

// Execute delegates supported ensure-steps to the configured agent runner.
func (e *AgentExecutor) Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	canonicalStep, ok := core.EnsureStepByID(step.ID)
	if !ok {
		return core.ExecutionResult{}, core.WrapError(
			core.KindStepNotApplicable,
			"execute.agent.step",
			fmt.Errorf("unknown ensure-step ID %q", step.ID),
		)
	}

	task, err := agentTaskForStep(snapshot, canonicalStep)
	if err != nil {
		return core.ExecutionResult{}, err
	}

	runnerResult, err := e.runner.Run(ctx, task)
	if err != nil {
		return core.ExecutionResult{}, err
	}

	transcriptPath := strings.TrimSpace(runnerResult.TranscriptPath)
	if transcriptPath == "" {
		transcriptPath = task.TranscriptPath
	}

	summary := strings.TrimSpace(runnerResult.Summary)
	if summary == "" {
		summary = "agent task completed"
	}

	evidence := []core.EvidenceItem{
		{Name: "executor.mode", Value: "agent"},
		{Name: "agent.objective", Value: task.Objective},
		{Name: "agent.transcript_path", Value: transcriptPath},
	}
	evidence = append(evidence, runnerResult.Evidence...)

	return core.ExecutionResult{
		Step:     canonicalStep,
		Applied:  runnerResult.Applied,
		Summary:  summary,
		Evidence: evidence,
	}, nil
}

func agentTaskForStep(snapshot core.WorkspaceSnapshot, step core.EnsureStep) (agent.AgentTask, error) {
	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return agent.AgentTask{}, core.NewError(core.KindUnknown, "execute.agent.repo_root", "snapshot repository root is empty")
	}

	objective, constraints, supported := objectiveForStep(snapshot, step.ID)
	if !supported {
		return agent.AgentTask{}, core.WrapError(
			core.KindStepNotApplicable,
			"execute.agent.unsupported_step",
			fmt.Errorf("ensure-step %q is not supported by agent executor", step.ID),
		)
	}

	transcriptPath, err := transcriptPathForSnapshot(repoRoot, snapshot.ID)
	if err != nil {
		return agent.AgentTask{}, err
	}

	return agent.AgentTask{
		Objective:      objective,
		Constraints:    constraints,
		RepoRoot:       repoRoot,
		TranscriptPath: transcriptPath,
	}, nil
}

func objectiveForStep(snapshot core.WorkspaceSnapshot, stepID core.EnsureStepID) (string, []string, bool) {
	switch stepID {
	case core.EnsureStepPreprocessContract:
		constraints := []string{
			"Author preprocess in one pass: include @tensorleap_preprocess and required train/validation subset handling",
			"Ensure @tensorleap_load_model exists and preprocess wiring references the resolved model path",
			"Avoid changing input encoders, ground-truth encoders, and integration-test wiring in this step",
			"Avoid changing unrelated project behavior",
		}
		if selectedModelPath := strings.TrimSpace(snapshot.SelectedModelPath); selectedModelPath != "" {
			constraints = append(constraints, fmt.Sprintf("Use model path %q for @tensorleap_load_model unless repository code proves this path is invalid", selectedModelPath))
		}
		return "Implement preprocess contract with decorator-correct model loading in one pass", constraints, true
	case core.EnsureStepInputEncoders:
		return "Implement and repair Tensorleap input encoders", []string{
			"Ensure encoders execute for multiple indices without exceptions",
			"Keep tensor shapes and dtypes stable for model inference",
		}, true
	case core.EnsureStepGroundTruthEncoders:
		return "Implement and repair Tensorleap ground-truth encoders", []string{
			"Ground-truth encoders should execute on labeled subsets only",
			"Preserve existing dataset semantics",
		}, true
	case core.EnsureStepHarnessValidation:
		return "Resolve runtime harness and anti-stub validation findings", []string{
			"Address root-cause failures and keep generated integration artifacts consistent",
		}, true
	case core.EnsureStepInvestigate:
		return "Investigate and resolve remaining integration issues", []string{
			"Prefer minimal, reviewable changes",
		}, true
	default:
		return "", nil, false
	}
}

func transcriptPathForSnapshot(repoRoot, snapshotID string) (string, error) {
	paths, err := persistence.NewPaths(repoRoot)
	if err != nil {
		return "", core.WrapError(core.KindUnknown, "execute.agent.paths", err)
	}
	id := strings.TrimSpace(snapshotID)
	if id == "" {
		id = "unknown"
	}
	return filepath.Join(paths.EvidenceDir(id), "agent.transcript.log"), nil
}
