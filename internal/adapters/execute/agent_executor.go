package execute

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/adapters/inspect"
	"github.com/tensorleap/concierge/internal/agent"
	agentcontext "github.com/tensorleap/concierge/internal/agent/context"
	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/observe"
	"github.com/tensorleap/concierge/internal/persistence"
)

type agentTaskRunner interface {
	Run(ctx context.Context, task agent.AgentTask) (agent.AgentResult, error)
}

// AgentExecutor delegates complex integration objectives to an external coding agent.
type AgentExecutor struct {
	runner            agentTaskRunner
	loadKnowledgePack func() (agent.DomainKnowledgePack, error)
	observer          observe.Sink
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

// SetObserver configures the live event sink used for agent task preparation events.
func (e *AgentExecutor) SetObserver(sink observe.Sink) {
	e.observer = sink
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
	if stepRequiresInspectStatus(canonicalStep.ID) {
		inspector := inspect.NewBaselineInspector()
		status, err := inspector.Inspect(ctx, taskSnapshot)
		if err != nil {
			return core.ExecutionResult{}, err
		}
		taskStatus = status
	}
	if canonicalStep.ID == core.EnsureStepIntegrationTestContract {
		recommendation, err := BuildIntegrationTestAuthoringRecommendation(taskSnapshot, taskStatus)
		if err != nil {
			return core.ExecutionResult{}, err
		}
		recommendations = append(recommendations, recommendation)
	}
	if canonicalStep.ID == core.EnsureStepModelAcquisition {
		recommendation, err := BuildModelAcquisitionRecommendation(taskSnapshot, taskStatus)
		if err != nil {
			return core.ExecutionResult{}, err
		}
		recommendations = append(recommendations, recommendation)
		if strings.TrimSpace(taskSnapshot.SelectedModelPath) == "" {
			taskSnapshot.SelectedModelPath = strings.TrimSpace(recommendation.Target)
		}
		if taskStatus.Contracts == nil {
			taskStatus.Contracts = &core.IntegrationContracts{}
		}
		taskStatus.Contracts.ResolvedModelPath = strings.TrimSpace(taskSnapshot.SelectedModelPath)
	}
	if canonicalStep.ID == core.EnsureStepModelContract {
		recommendation, err := BuildModelAuthoringRecommendation(taskSnapshot, taskStatus)
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
	if canonicalStep.ID == core.EnsureStepPreprocessContract {
		recommendation, err := BuildPreprocessAuthoringRecommendation(taskSnapshot, taskStatus)
		if err != nil {
			return core.ExecutionResult{}, err
		}
		recommendations = append(recommendations, recommendation)
	}
	if canonicalStep.ID == core.EnsureStepInputEncoders {
		recommendation, err := BuildInputEncoderAuthoringRecommendation(taskSnapshot, taskStatus)
		if err != nil {
			return core.ExecutionResult{}, err
		}
		recommendations = append(recommendations, recommendation)
	}
	if canonicalStep.ID == core.EnsureStepGroundTruthEncoders {
		recommendation, err := BuildGTEncoderAuthoringRecommendation(taskSnapshot, taskStatus)
		if err != nil {
			return core.ExecutionResult{}, err
		}
		recommendations = append(recommendations, recommendation)
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
	e.emit(observe.Event{
		SnapshotID: snapshot.ID,
		StepID:     canonicalStep.ID,
		Kind:       observe.EventAgentTaskPrepared,
		Message:    "Preparing Claude task",
		Detail:     task.Objective,
	})

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
		{Name: "agent.stream_path", Value: strings.TrimSpace(runnerResult.RawStreamPath)},
		{Name: "agent.knowledge_pack.version", Value: knowledgePack.Version},
		{Name: "agent.knowledge_pack.section_ids", Value: strings.Join(knowledgeSectionIDs(knowledgePack), ",")},
		{Name: "agent.repo_context.path", Value: repoContextPath},
	}
	if runnerResult.Interrupted {
		evidence = append(evidence, core.EvidenceItem{Name: "agent.interrupted", Value: "true"})
	}
	if !runnerResult.LastActivityAt.IsZero() {
		evidence = append(evidence, core.EvidenceItem{Name: "agent.last_activity_at", Value: runnerResult.LastActivityAt.UTC().Format(time.RFC3339)})
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

func stepRequiresInspectStatus(stepID core.EnsureStepID) bool {
	switch stepID {
	case core.EnsureStepPreprocessContract,
		core.EnsureStepInputEncoders,
		core.EnsureStepGroundTruthEncoders,
		core.EnsureStepIntegrationTestContract,
		core.EnsureStepModelAcquisition,
		core.EnsureStepModelContract:
		return true
	default:
		return false
	}
}

func (e *AgentExecutor) emit(event observe.Event) {
	if e == nil || e.observer == nil {
		return
	}
	e.observer.Emit(event)
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
		if recommendation.StepID != core.EnsureStepModelAcquisition &&
			recommendation.StepID != core.EnsureStepModelContract &&
			recommendation.StepID != core.EnsureStepPreprocessContract &&
			recommendation.StepID != core.EnsureStepInputEncoders &&
			recommendation.StepID != core.EnsureStepGroundTruthEncoders &&
			recommendation.StepID != core.EnsureStepIntegrationTestContract {
			continue
		}
		switch recommendation.StepID {
		case core.EnsureStepModelAcquisition:
			evidence = append(evidence,
				core.EvidenceItem{Name: "authoring.recommendation.model_acquisition.target", Value: strings.TrimSpace(recommendation.Target)},
				core.EvidenceItem{Name: "authoring.recommendation.model_acquisition.rationale", Value: strings.TrimSpace(recommendation.Rationale)},
				core.EvidenceItem{Name: "authoring.recommendation.model_acquisition.candidates", Value: strings.Join(recommendation.Candidates, ",")},
				core.EvidenceItem{Name: "authoring.recommendation.model_acquisition.constraints", Value: strings.Join(recommendation.Constraints, " | ")},
			)
		case core.EnsureStepModelContract:
			evidence = append(evidence,
				core.EvidenceItem{Name: "authoring.recommendation.model.target", Value: strings.TrimSpace(recommendation.Target)},
				core.EvidenceItem{Name: "authoring.recommendation.model.rationale", Value: strings.TrimSpace(recommendation.Rationale)},
				core.EvidenceItem{Name: "authoring.recommendation.model.candidates", Value: strings.Join(recommendation.Candidates, ",")},
			)
		case core.EnsureStepPreprocessContract:
			evidence = append(evidence,
				core.EvidenceItem{Name: "authoring.recommendation.preprocess.target", Value: strings.TrimSpace(recommendation.Target)},
				core.EvidenceItem{Name: "authoring.recommendation.preprocess.rationale", Value: strings.TrimSpace(recommendation.Rationale)},
				core.EvidenceItem{Name: "authoring.recommendation.preprocess.target_symbols", Value: strings.Join(recommendation.Candidates, ",")},
				core.EvidenceItem{Name: "authoring.recommendation.preprocess.constraints", Value: strings.Join(recommendation.Constraints, " | ")},
			)
		case core.EnsureStepInputEncoders:
			evidence = append(evidence,
				core.EvidenceItem{Name: "authoring.recommendation.input_encoder.target", Value: strings.TrimSpace(recommendation.Target)},
				core.EvidenceItem{Name: "authoring.recommendation.input_encoder.rationale", Value: strings.TrimSpace(recommendation.Rationale)},
				core.EvidenceItem{Name: "authoring.recommendation.input_encoder.target_symbols", Value: strings.Join(recommendation.Candidates, ",")},
				core.EvidenceItem{Name: "authoring.recommendation.input_encoder.constraints", Value: strings.Join(recommendation.Constraints, " | ")},
			)
		case core.EnsureStepGroundTruthEncoders:
			evidence = append(evidence,
				core.EvidenceItem{Name: "authoring.recommendation.gt_encoder.target", Value: strings.TrimSpace(recommendation.Target)},
				core.EvidenceItem{Name: "authoring.recommendation.gt_encoder.rationale", Value: strings.TrimSpace(recommendation.Rationale)},
				core.EvidenceItem{Name: "authoring.recommendation.gt_encoder.target_symbols", Value: strings.Join(recommendation.Candidates, ",")},
				core.EvidenceItem{Name: "authoring.recommendation.gt_encoder.constraints", Value: strings.Join(recommendation.Constraints, " | ")},
			)
		case core.EnsureStepIntegrationTestContract:
			evidence = append(evidence,
				core.EvidenceItem{Name: "authoring.recommendation.integration_test.target", Value: strings.TrimSpace(recommendation.Target)},
				core.EvidenceItem{Name: "authoring.recommendation.integration_test.rationale", Value: strings.TrimSpace(recommendation.Rationale)},
				core.EvidenceItem{Name: "authoring.recommendation.integration_test.candidates", Value: strings.Join(recommendation.Candidates, ",")},
				core.EvidenceItem{Name: "authoring.recommendation.integration_test.constraints", Value: strings.Join(recommendation.Constraints, " | ")},
			)
		}
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
		{Name: "agent.repo_context.leap_yaml_boundary", Value: context.LeapYAMLBoundary},
		{Name: "agent.repo_context.runtime_kind", Value: context.RuntimeKind},
		{Name: "agent.repo_context.runtime_interpreter", Value: context.RuntimeInterpreter},
		{Name: "agent.repo_context.runtime_status", Value: context.RuntimeStatus},
		{Name: "agent.repo_context.selected_model_path", Value: context.SelectedModelPath},
		{Name: "agent.repo_context.model_candidates", Value: strings.Join(context.ModelCandidates, ",")},
		{Name: "agent.repo_context.ready_model_artifacts", Value: strings.Join(context.ReadyModelArtifacts, ",")},
		{Name: "agent.repo_context.model_acquisition_leads", Value: strings.Join(context.ModelAcquisitionLeads, ",")},
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
	case core.EnsureStepModelAcquisition:
		constraints := []string{
			"Analyze existing repository code and documentation to find how the project obtains its model.",
			"Materialize exactly one supported .onnx or .h5 artifact locally before editing @tensorleap_load_model.",
			"Prefer existing repository commands or Python entrypoints before creating a temporary helper under .concierge/materializers.",
			"Do not modify unrelated training/business logic or rely on Tensorleap rerunning model acquisition on the server.",
		}
		if selectedModelPath := strings.TrimSpace(snapshot.SelectedModelPath); selectedModelPath != "" {
			constraints = append(constraints, fmt.Sprintf("Materialize the supported artifact at %q unless repository evidence proves a different repo-local output path is required", selectedModelPath))
		}
		for _, recommendation := range recommendations {
			if recommendation.StepID != core.EnsureStepModelAcquisition {
				continue
			}
			if target := strings.TrimSpace(recommendation.Target); target != "" {
				constraints = append(constraints, fmt.Sprintf("Recommended materialized artifact target: %q (%s)", target, recommendation.Rationale))
			}
			if len(recommendation.Candidates) > 0 {
				constraints = append(constraints, fmt.Sprintf("Relevant model leads: %s", strings.Join(recommendation.Candidates, ", ")))
			}
			constraints = append(constraints, recommendation.Constraints...)
			break
		}
		return "Investigate repository model acquisition logic and materialize one Tensorleap-compatible model artifact", constraints, true
	case core.EnsureStepModelContract:
		constraints := []string{
			"Bind @tensorleap_load_model to exactly one concrete supported .onnx/.h5 artifact path",
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
			"Implement a preprocess function that returns both train and validation subsets.",
			"Keep preprocess outputs deterministic and non-empty for each feasible subset.",
			"Avoid changing unrelated project behavior",
		}
		for _, recommendation := range recommendations {
			if recommendation.StepID != core.EnsureStepPreprocessContract {
				continue
			}
			if target := strings.TrimSpace(recommendation.Target); target != "" {
				constraints = append(constraints, fmt.Sprintf("Recommended preprocess target: %q (%s)", target, recommendation.Rationale))
			}
			if len(recommendation.Candidates) > 0 {
				constraints = append(constraints, fmt.Sprintf("Suggested preprocess symbols: %s", strings.Join(recommendation.Candidates, ", ")))
			}
			constraints = append(constraints, recommendation.Constraints...)
			break
		}
		return "Implement preprocess contract with required train/validation subset handling and deterministic outputs", constraints, true
	case core.EnsureStepInputEncoders:
		constraints := []string{
			"Implement missing @tensorleap_input_encoder functions for each required input symbol.",
			"Register each @tensorleap_input_encoder with the exact required Tensorleap symbol name; do not substitute aliases like raw model tensor names (`images` vs `image`).",
			"The first encoder argument is the Tensorleap sample_id from preprocess.sample_ids, not a positional dataset index; handle it according to PreprocessResponse.sample_id_type.",
			"Keep tensor shapes and dtypes stable for model inference.",
			"Do not modify @tensorleap_gt_encoder definitions or integration-test wiring in this step.",
		}
		for _, recommendation := range recommendations {
			if recommendation.StepID != core.EnsureStepInputEncoders {
				continue
			}
			if target := strings.TrimSpace(recommendation.Target); target != "" {
				constraints = append(constraints, fmt.Sprintf("Primary missing input symbol: %q (%s)", target, recommendation.Rationale))
			}
			if len(recommendation.Candidates) > 0 {
				constraints = append(constraints, fmt.Sprintf("Required input symbols: %s", strings.Join(recommendation.Candidates, ", ")))
			}
			break
		}
		if selectedModelPath := strings.TrimSpace(snapshot.SelectedModelPath); selectedModelPath != "" {
			constraints = append(constraints, fmt.Sprintf("Use model path %q as input-shape contract source unless repository code proves this path is invalid", selectedModelPath))
		}
		return "Implement and repair Tensorleap input encoders with symbol-level contract coverage", constraints, true
	case core.EnsureStepGroundTruthEncoders:
		constraints := []string{
			"Implement missing @tensorleap_gt_encoder functions for each required target symbol.",
			"The first GT encoder argument is the Tensorleap sample_id from preprocess.sample_ids, not a positional dataset index; handle it according to PreprocessResponse.sample_id_type.",
			"Ground-truth encoders should execute on labeled subsets only (never unlabeled subsets).",
			"Do not modify @tensorleap_input_encoder definitions or integration-test wiring in this step.",
		}
		for _, recommendation := range recommendations {
			if recommendation.StepID != core.EnsureStepGroundTruthEncoders {
				continue
			}
			if target := strings.TrimSpace(recommendation.Target); target != "" {
				constraints = append(constraints, fmt.Sprintf("Primary missing ground-truth symbol: %q (%s)", target, recommendation.Rationale))
			}
			if len(recommendation.Candidates) > 0 {
				constraints = append(constraints, fmt.Sprintf("Required ground-truth symbols: %s", strings.Join(recommendation.Candidates, ", ")))
			}
			break
		}
		if selectedModelPath := strings.TrimSpace(snapshot.SelectedModelPath); selectedModelPath != "" {
			constraints = append(constraints, fmt.Sprintf("Use model path %q as output/label alignment contract unless repository code proves this path is invalid", selectedModelPath))
		}
		return "Implement and repair Tensorleap ground-truth encoders with labeled-subset constraints", constraints, true
	case core.EnsureStepIntegrationTestContract:
		constraints := []string{
			"Repair only @tensorleap_integration_test wiring and body shape.",
			"Keep integration_test thin and declarative so mapping-mode re-execution succeeds.",
			"Do not modify preprocess subset semantics, encoder implementations, or unrelated project logic.",
		}
		for _, recommendation := range recommendations {
			if recommendation.StepID != core.EnsureStepIntegrationTestContract {
				continue
			}
			if target := strings.TrimSpace(recommendation.Target); target != "" {
				constraints = append(constraints, fmt.Sprintf("Primary repair target: %q (%s)", target, recommendation.Rationale))
			}
			if len(recommendation.Candidates) > 0 {
				constraints = append(constraints, fmt.Sprintf("Repair focus areas: %s", strings.Join(recommendation.Candidates, ", ")))
			}
			constraints = append(constraints, recommendation.Constraints...)
			break
		}
		return "Repair Tensorleap integration-test wiring and remove illegal integration-test body logic", constraints, true
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
