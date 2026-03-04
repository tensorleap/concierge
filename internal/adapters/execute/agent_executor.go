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

	taskSnapshot := snapshot
	taskStatus := core.IntegrationStatus{}
	recommendations := make([]core.AuthoringRecommendation, 0, 1)
	if canonicalStep.ID == core.EnsureStepModelContract {
		recommendation, err := BuildModelAuthoringRecommendation(snapshot, taskStatus)
		if err != nil {
			return core.ExecutionResult{}, err
		}
		recommendations = append(recommendations, recommendation)
		if strings.TrimSpace(taskSnapshot.SelectedModelPath) == "" {
			taskSnapshot.SelectedModelPath = strings.TrimSpace(recommendation.Target)
		}
		taskStatus.Contracts = &core.IntegrationContracts{
			ModelCandidates:   recommendationCandidatesAsModelCandidates(recommendation.Candidates),
			ResolvedModelPath: strings.TrimSpace(recommendation.Target),
		}
	}

	scopePolicy, err := PolicyForStep(canonicalStep.ID, taskSnapshot, taskStatus)
	if err != nil {
		return core.ExecutionResult{}, err
	}

	repoContext, err := BuildAgentRepoContext(canonicalStep.ID, taskSnapshot, taskStatus, core.ValidationResult{})
	if err != nil {
		return core.ExecutionResult{}, err
	}
	repoContextPath, err := persistAgentRepoContext(snapshot.Repository.Root, snapshot.ID, repoContext)
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

	task, err := agentTaskForStep(taskSnapshot, canonicalStep, scopePolicy, repoContext, knowledgePack, recommendations)
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
		{Name: "agent.knowledge_pack.version", Value: knowledgePack.Version},
		{Name: "agent.knowledge_pack.section_ids", Value: strings.Join(knowledgeSectionIDs(knowledgePack), ",")},
		{Name: "agent.repo_context.path", Value: repoContextPath},
	}
	evidence = append(evidence, scopePolicyEvidence(scopePolicy)...)
	evidence = append(evidence, repoContextEvidence(repoContext)...)
	evidence = append(evidence, recommendationEvidence(recommendations)...)
	evidence = append(evidence, runnerResult.Evidence...)

	return core.ExecutionResult{
		Step:            canonicalStep,
		Applied:         runnerResult.Applied,
		Summary:         summary,
		Evidence:        evidence,
		Recommendations: append([]core.AuthoringRecommendation(nil), recommendations...),
	}, nil
}

func recommendationCandidatesAsModelCandidates(values []string) []core.ModelCandidate {
	if len(values) == 0 {
		return nil
	}
	candidates := make([]core.ModelCandidate, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		candidates = append(candidates, core.ModelCandidate{Path: trimmed, Source: "authoring_recommendation"})
	}
	return candidates
}

func recommendationEvidence(recommendations []core.AuthoringRecommendation) []core.EvidenceItem {
	if len(recommendations) == 0 {
		return nil
	}
	evidence := make([]core.EvidenceItem, 0, 3)
	for _, recommendation := range recommendations {
		if recommendation.StepID != core.EnsureStepModelContract {
			continue
		}
		evidence = append(evidence,
			core.EvidenceItem{Name: "authoring.recommendation.model.target", Value: strings.TrimSpace(recommendation.Target)},
			core.EvidenceItem{Name: "authoring.recommendation.model.rationale", Value: strings.TrimSpace(recommendation.Rationale)},
			core.EvidenceItem{Name: "authoring.recommendation.model.candidates", Value: strings.Join(recommendation.Candidates, ",")},
		)
	}
	return evidence
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

func repoContextEvidence(context core.AgentRepoContext) []core.EvidenceItem {
	return []core.EvidenceItem{
		{Name: "agent.repo_context.entry_file", Value: context.EntryFile},
		{Name: "agent.repo_context.binder_file", Value: context.BinderFile},
		{Name: "agent.repo_context.leap_yaml_boundary", Value: context.LeapYAMLBoundary},
		{Name: "agent.repo_context.selected_model_path", Value: context.SelectedModelPath},
		{Name: "agent.repo_context.model_candidates", Value: strings.Join(context.ModelCandidates, ",")},
		{Name: "agent.repo_context.decorator_inventory", Value: strings.Join(context.DecoratorInventory, ",")},
		{Name: "agent.repo_context.integration_test_calls", Value: strings.Join(context.IntegrationTestCalls, ",")},
		{Name: "agent.repo_context.blocking_issues", Value: strings.Join(context.BlockingIssues, " | ")},
		{Name: "agent.repo_context.validation_findings", Value: strings.Join(context.ValidationFindings, " | ")},
	}
}

func agentTaskForStep(
	snapshot core.WorkspaceSnapshot,
	step core.EnsureStep,
	policy agent.AgentScopePolicy,
	repoContext core.AgentRepoContext,
	knowledgePack agent.DomainKnowledgePack,
	recommendations []core.AuthoringRecommendation,
) (agent.AgentTask, error) {
	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return agent.AgentTask{}, core.NewError(core.KindUnknown, "execute.agent.repo_root", "snapshot repository root is empty")
	}

	objective, acceptanceChecks, supported := objectiveForStep(snapshot, step.ID, recommendations)
	if !supported {
		return agent.AgentTask{}, core.WrapError(
			core.KindStepNotApplicable,
			"execute.agent.unsupported_step",
			fmt.Errorf("ensure-step %q is not supported by agent executor", step.ID),
		)
	}
	acceptanceChecks = mergeAcceptanceChecks(acceptanceChecks, policy.RequiredOutcomes)

	transcriptPath, err := transcriptPathForSnapshot(repoRoot, snapshot.ID)
	if err != nil {
		return agent.AgentTask{}, err
	}

	knowledgeSlice, err := domainKnowledgeSliceForPolicy(policy, knowledgePack)
	if err != nil {
		return agent.AgentTask{}, err
	}

	return agent.AgentTask{
		Objective:        objective,
		Constraints:      acceptanceChecks,
		AcceptanceChecks: acceptanceChecks,
		ScopePolicy:      &policy,
		RepoContext:      &repoContext,
		DomainKnowledge:  &knowledgeSlice,
		RepoRoot:         repoRoot,
		TranscriptPath:   transcriptPath,
	}, nil
}

func objectiveForStep(
	snapshot core.WorkspaceSnapshot,
	stepID core.EnsureStepID,
	recommendations []core.AuthoringRecommendation,
) (string, []string, bool) {
	switch stepID {
	case core.EnsureStepModelContract:
		constraints := []string{
			"Resolve @tensorleap_load_model to exactly one concrete .onnx/.h5 model path",
			"Do not modify unrelated training/business logic",
			"Model binaries are uploaded by leap CLI; leap.yaml include/exclude governs integration code",
		}
		if selectedModelPath := strings.TrimSpace(snapshot.SelectedModelPath); selectedModelPath != "" {
			constraints = append(constraints, fmt.Sprintf("Use model path %q unless repository evidence proves it invalid", selectedModelPath))
		}
		for _, recommendation := range recommendations {
			if recommendation.StepID != core.EnsureStepModelContract {
				continue
			}
			if target := strings.TrimSpace(recommendation.Target); target != "" {
				constraints = append(constraints, fmt.Sprintf("Recommended model target: %q (%s)", target, recommendation.Rationale))
			}
			if len(recommendation.Candidates) > 0 {
				constraints = append(constraints, fmt.Sprintf("Candidate model paths: %s", strings.Join(recommendation.Candidates, ", ")))
			}
			break
		}
		return "Remediate Tensorleap model contract by fixing @tensorleap_load_model path selection", constraints, true
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

func mergeAcceptanceChecks(groups ...[]string) []string {
	merged := make([]string, 0)
	seen := map[string]struct{}{}
	for _, group := range groups {
		for _, value := range group {
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			key := strings.ToLower(trimmed)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, trimmed)
		}
	}
	return merged
}

func domainKnowledgeSliceForPolicy(policy agent.AgentScopePolicy, pack agent.DomainKnowledgePack) (agent.AgentDomainKnowledgePack, error) {
	sectionIDs := mergeAcceptanceChecks(policy.DomainSections)
	if len(sectionIDs) == 0 {
		return agent.AgentDomainKnowledgePack{}, core.NewError(
			core.KindUnknown,
			"execute.agent.domain_knowledge",
			"scope policy does not define domain knowledge section IDs",
		)
	}

	sections := make(map[string]string, len(sectionIDs))
	missing := make([]string, 0)
	for _, sectionID := range sectionIDs {
		body := strings.TrimSpace(pack.Sections[sectionID])
		if body == "" {
			missing = append(missing, sectionID)
			continue
		}
		sections[sectionID] = body
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return agent.AgentDomainKnowledgePack{}, core.WrapError(
			core.KindUnknown,
			"execute.agent.domain_knowledge",
			fmt.Errorf("missing scoped domain knowledge section(s): %s", strings.Join(missing, ", ")),
		)
	}

	return agent.AgentDomainKnowledgePack{
		Version:    strings.TrimSpace(pack.Version),
		SectionIDs: sectionIDs,
		Sections:   sections,
	}, nil
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
