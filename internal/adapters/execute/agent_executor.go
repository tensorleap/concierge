package execute

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/agent"
	agentcontext "github.com/tensorleap/concierge/internal/agent/context"
	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/persistence"
)

type agentTaskRunner interface {
	Run(ctx context.Context, task agent.AgentTask) (agent.AgentResult, error)
}

// AgentExecutor delegates complex integration objectives to an external coding agent.
type AgentExecutor struct {
	runner            agentTaskRunner
	loadKnowledgePack func() (agent.DomainKnowledgePack, error)
}

// NewAgentExecutor creates an agent-backed executor.
func NewAgentExecutor(runner agentTaskRunner) *AgentExecutor {
	if runner == nil {
		runner = agent.NewRunner()
	}
	return &AgentExecutor{
		runner:            runner,
		loadKnowledgePack: agentcontext.LoadDomainKnowledgePack,
	}
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

	scopePolicy, err := PolicyForStep(canonicalStep.ID, snapshot, core.IntegrationStatus{})
	if err != nil {
		return core.ExecutionResult{}, err
	}

	task, err := agentTaskForStep(snapshot, canonicalStep, scopePolicy)
	if err != nil {
		return core.ExecutionResult{}, err
	}

	knowledgePack, err := e.loadPack()
	if err != nil {
		return core.ExecutionResult{}, err
	}
	if err := validatePolicyDomainSections(scopePolicy, knowledgePack); err != nil {
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
		{Name: "agent.knowledge_pack.version", Value: knowledgePack.Version},
		{Name: "agent.knowledge_pack.section_ids", Value: strings.Join(knowledgeSectionIDs(knowledgePack), ",")},
	}
	evidence = append(evidence, scopePolicyEvidence(scopePolicy)...)
	evidence = append(evidence, runnerResult.Evidence...)

	return core.ExecutionResult{
		Step:     canonicalStep,
		Applied:  runnerResult.Applied,
		Summary:  summary,
		Evidence: evidence,
	}, nil
}

func (e *AgentExecutor) loadPack() (agent.DomainKnowledgePack, error) {
	if e.loadKnowledgePack == nil {
		e.loadKnowledgePack = agentcontext.LoadDomainKnowledgePack
	}

	pack, err := e.loadKnowledgePack()
	if err != nil {
		return agent.DomainKnowledgePack{}, core.WrapError(core.KindUnknown, "execute.agent.knowledge_pack", err)
	}
	return pack, nil
}

func knowledgeSectionIDs(pack agent.DomainKnowledgePack) []string {
	ids := make([]string, 0, len(pack.Sections))
	for sectionID := range pack.Sections {
		ids = append(ids, sectionID)
	}
	sort.Strings(ids)
	return ids
}

func validatePolicyDomainSections(policy agent.AgentScopePolicy, pack agent.DomainKnowledgePack) error {
	if len(policy.DomainSections) == 0 {
		return core.NewError(core.KindUnknown, "execute.agent.scope_policy.domain_sections", "scope policy does not define domain sections")
	}

	missing := make([]string, 0)
	for _, sectionID := range policy.DomainSections {
		if _, ok := pack.Sections[sectionID]; ok {
			continue
		}
		missing = append(missing, sectionID)
	}
	if len(missing) == 0 {
		return nil
	}

	sort.Strings(missing)
	return core.WrapError(
		core.KindUnknown,
		"execute.agent.scope_policy.domain_sections",
		fmt.Errorf("scope policy references unknown knowledge section(s): %s", strings.Join(missing, ", ")),
	)
}

func scopePolicyEvidence(policy agent.AgentScopePolicy) []core.EvidenceItem {
	return []core.EvidenceItem{
		{Name: "agent.scope_policy.allowed_files", Value: strings.Join(policy.AllowedFiles, ",")},
		{Name: "agent.scope_policy.forbidden_areas", Value: strings.Join(policy.ForbiddenAreas, " | ")},
		{Name: "agent.scope_policy.required_outcomes", Value: strings.Join(policy.RequiredOutcomes, " | ")},
		{Name: "agent.scope_policy.stop_and_ask_triggers", Value: strings.Join(policy.StopAndAskTriggers, " | ")},
		{Name: "agent.scope_policy.domain_sections", Value: strings.Join(policy.DomainSections, ",")},
	}
}

func agentTaskForStep(snapshot core.WorkspaceSnapshot, step core.EnsureStep, policy agent.AgentScopePolicy) (agent.AgentTask, error) {
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

	constraints = append(constraints, constraintsForScopePolicy(policy)...)

	return agent.AgentTask{
		Objective:      objective,
		Constraints:    constraints,
		ScopePolicy:    &policy,
		RepoRoot:       repoRoot,
		TranscriptPath: transcriptPath,
	}, nil
}

func constraintsForScopePolicy(policy agent.AgentScopePolicy) []string {
	constraints := make([]string, 0, len(policy.AllowedFiles)+len(policy.ForbiddenAreas)+len(policy.RequiredOutcomes)+len(policy.StopAndAskTriggers)+1)

	if len(policy.DomainSections) > 0 {
		constraints = append(constraints, fmt.Sprintf("Use Tensorleap rule sections only: %s", strings.Join(policy.DomainSections, ", ")))
	}
	if len(policy.AllowedFiles) > 0 {
		constraints = append(constraints, fmt.Sprintf("Edit only these integration files unless a stop-and-ask trigger is hit: %s", strings.Join(policy.AllowedFiles, ", ")))
	}
	for _, forbidden := range policy.ForbiddenAreas {
		trimmed := strings.TrimSpace(forbidden)
		if trimmed == "" {
			continue
		}
		constraints = append(constraints, fmt.Sprintf("Forbidden edit area: %s", trimmed))
	}
	for _, outcome := range policy.RequiredOutcomes {
		trimmed := strings.TrimSpace(outcome)
		if trimmed == "" {
			continue
		}
		constraints = append(constraints, fmt.Sprintf("Required outcome: %s", trimmed))
	}
	for _, trigger := range policy.StopAndAskTriggers {
		trimmed := strings.TrimSpace(trigger)
		if trimmed == "" {
			continue
		}
		constraints = append(constraints, fmt.Sprintf("Stop and ask before editing when: %s", trimmed))
	}
	return constraints
}

func objectiveForStep(snapshot core.WorkspaceSnapshot, stepID core.EnsureStepID) (string, []string, bool) {
	switch stepID {
	case core.EnsureStepPreprocessContract:
		constraints := []string{
			"Author preprocess in one pass: include @tensorleap_preprocess and required train/validation subset handling",
			"Ensure @tensorleap_load_model exists and preprocess wiring references the resolved model path",
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
