package execute

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/persistence"
)

const (
	maxRepoContextModelCandidates    = 8
	maxRepoContextDecoratorInventory = 16
	maxRepoContextIntegrationCalls   = 16
	maxRepoContextBlockingIssues     = 12
	maxRepoContextValidationFindings = 12
	maxRepoContextBoundaryFiles      = 8
)

// BuildAgentRepoContext assembles deterministic, step-scoped repository facts for one agent task.
func BuildAgentRepoContext(
	step core.EnsureStepID,
	snapshot core.WorkspaceSnapshot,
	status core.IntegrationStatus,
	validation core.ValidationResult,
) (core.AgentRepoContext, error) {
	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return core.AgentRepoContext{}, core.NewError(
			core.KindUnknown,
			"execute.agent.repo_context.repo_root",
			"snapshot repository root is empty",
		)
	}

	context := core.AgentRepoContext{
		RepoRoot:                   repoRoot,
		EntryFile:                  normalizeRepoContextPath(resolveEntryFile(snapshot, status)),
		LeapYAMLBoundary:           leapYAMLBoundarySummary(snapshot),
		RuntimeKind:                runtimeKindForContext(snapshot.RuntimeProfile),
		RuntimeInterpreter:         normalizeRepoContextPath(runtimeInterpreterForContext(snapshot.RuntimeProfile)),
		RuntimeStatus:              runtimeStatusForContext(snapshot.RuntimeProfile),
		SelectedModelPath:          normalizeRepoContextPath(resolveSelectedModelPath(snapshot, status)),
		ModelAcquisitionPlan:       selectedModelAcquisitionPlan(snapshot, status),
		RequiredInputSymbols:       requiredInputSymbolsForContext(status.Contracts),
		RequiredGroundTruthSymbols: requiredGroundTruthSymbolsForContext(status.Contracts),
		ModelCandidates:            truncateRepoContextValues(modelCandidatesForContext(snapshot, status), maxRepoContextModelCandidates),
		ReadyModelArtifacts:        truncateRepoContextValues(readyModelArtifactsForContext(status), maxRepoContextModelCandidates),
		ModelAcquisitionLeads: truncateRepoContextValues(
			modelAcquisitionLeadsForContext(status),
			maxRepoContextModelCandidates,
		),
		DecoratorInventory: truncateRepoContextValues(decoratorInventoryForContext(status.Contracts), maxRepoContextDecoratorInventory),
		IntegrationTestCalls: truncateRepoContextValues(
			uniqueSortedRepoContextValues(statusIntegrationTestCalls(status.Contracts)),
			maxRepoContextIntegrationCalls,
		),
		BlockingIssues: truncateRepoContextValues(
			issueSummariesForStep(step, mergeIssues(status.Issues, validation.Issues), true),
			maxRepoContextBlockingIssues,
		),
		ValidationFindings: truncateRepoContextValues(
			issueSummariesForStep(step, validation.Issues, false),
			maxRepoContextValidationFindings,
		),
	}

	applyRepoContextStepSlice(step, &context)

	if err := validateRequiredRepoContext(step, context); err != nil {
		return core.AgentRepoContext{}, err
	}

	return context, nil
}

func persistAgentRepoContext(repoRoot, snapshotID string, context core.AgentRepoContext) (string, error) {
	paths, err := persistence.NewPaths(repoRoot)
	if err != nil {
		return "", core.WrapError(core.KindUnknown, "execute.agent.repo_context.paths", err)
	}

	id := strings.TrimSpace(snapshotID)
	if id == "" {
		id = "unknown"
	}

	targetPath := filepath.Join(paths.EvidenceDir(id), "agent.repo_context.json")
	if err := persistence.WriteJSONAtomic(targetPath, context); err != nil {
		return "", core.WrapError(core.KindUnknown, "execute.agent.repo_context.persist", err)
	}

	return targetPath, nil
}

func resolveEntryFile(snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) string {
	if status.Contracts != nil {
		if entry := strings.TrimSpace(status.Contracts.EntryFile); entry != "" {
			return entry
		}
	}
	return core.CanonicalIntegrationEntryFile
}

func resolveSelectedModelPath(snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) string {
	if selected := strings.TrimSpace(snapshot.SelectedModelPath); selected != "" {
		return selected
	}
	if status.Contracts != nil {
		return strings.TrimSpace(status.Contracts.ResolvedModelPath)
	}
	return ""
}

func modelCandidatesForContext(snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) []string {
	candidates := make([]string, 0, 8)
	if status.Contracts != nil {
		for _, candidate := range status.Contracts.ModelCandidates {
			candidates = append(candidates, candidate.Path)
		}
	}
	if selected := strings.TrimSpace(snapshot.SelectedModelPath); selected != "" {
		candidates = append(candidates, selected)
	}
	return uniqueSortedRepoContextValues(candidates)
}

func requiredInputSymbolsForContext(contracts *core.IntegrationContracts) []string {
	return requiredSymbolsForContext(
		contracts,
		func(current *core.IntegrationContracts) []string {
			if current.ConfirmedMapping == nil {
				return nil
			}
			return current.ConfirmedMapping.InputSymbols
		},
		func(current *core.IntegrationContracts) []string {
			if current.InputGTDiscovery == nil || current.InputGTDiscovery.ComparisonReport == nil {
				return nil
			}
			return current.InputGTDiscovery.ComparisonReport.PrimaryInputSymbols
		},
		func(current *core.IntegrationContracts) []string {
			return current.DiscoveredInputSymbols
		},
	)
}

func requiredGroundTruthSymbolsForContext(contracts *core.IntegrationContracts) []string {
	return requiredSymbolsForContext(
		contracts,
		func(current *core.IntegrationContracts) []string {
			if current.ConfirmedMapping == nil {
				return nil
			}
			return current.ConfirmedMapping.GroundTruthSymbols
		},
		func(current *core.IntegrationContracts) []string {
			if current.InputGTDiscovery == nil || current.InputGTDiscovery.ComparisonReport == nil {
				return nil
			}
			return current.InputGTDiscovery.ComparisonReport.PrimaryGroundTruthSymbols
		},
		func(current *core.IntegrationContracts) []string {
			return current.DiscoveredGroundTruthSymbols
		},
	)
}

func requiredSymbolsForContext(
	contracts *core.IntegrationContracts,
	sources ...func(*core.IntegrationContracts) []string,
) []string {
	if contracts == nil {
		return nil
	}

	for _, source := range sources {
		if source == nil {
			continue
		}
		if values := uniqueSortedRepoContextValues(source(contracts)); len(values) > 0 {
			return values
		}
	}

	return nil
}

func readyModelArtifactsForContext(status core.IntegrationStatus) []string {
	if status.Contracts == nil || status.Contracts.ModelAcquisition == nil {
		return nil
	}
	values := make([]string, 0, len(status.Contracts.ModelAcquisition.ReadyArtifacts))
	for _, candidate := range status.Contracts.ModelAcquisition.ReadyArtifacts {
		values = append(values, candidate.Path)
	}
	return uniqueSortedRepoContextValues(values)
}

func modelAcquisitionLeadsForContext(status core.IntegrationStatus) []string {
	if status.Contracts == nil || status.Contracts.ModelAcquisition == nil {
		return nil
	}
	values := make([]string, 0, len(status.Contracts.ModelAcquisition.PassiveLeads)+len(status.Contracts.ModelAcquisition.AcquisitionLeads))
	for _, candidate := range status.Contracts.ModelAcquisition.PassiveLeads {
		values = append(values, candidate.Path)
	}
	values = append(values, status.Contracts.ModelAcquisition.AcquisitionLeads...)
	return uniqueSortedRepoContextValues(values)
}

func decoratorInventoryForContext(contracts *core.IntegrationContracts) []string {
	if contracts == nil {
		return nil
	}

	values := make([]string, 0,
		len(contracts.LoadModelFunctions)+
			len(contracts.PreprocessFunctions)+
			len(contracts.InputEncoders)+
			len(contracts.GroundTruthEncoders)+
			len(contracts.IntegrationTestFunctions),
	)

	appendDecorators := func(prefix string, symbols []string) {
		for _, symbol := range symbols {
			normalized := normalizeRepoContextPath(symbol)
			if normalized == "" {
				continue
			}
			values = append(values, fmt.Sprintf("%s:%s", prefix, normalized))
		}
	}

	appendDecorators("load_model", contracts.LoadModelFunctions)
	appendDecorators("preprocess", contracts.PreprocessFunctions)
	appendDecorators("input_encoder", contracts.InputEncoders)
	appendDecorators("gt_encoder", contracts.GroundTruthEncoders)
	appendDecorators("integration_test", contracts.IntegrationTestFunctions)

	return uniqueSortedRepoContextValues(values)
}

func statusIntegrationTestCalls(contracts *core.IntegrationContracts) []string {
	if contracts == nil {
		return nil
	}
	return append([]string(nil), contracts.IntegrationTestCalls...)
}

func mergeIssues(groups ...[]core.Issue) []core.Issue {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	if total == 0 {
		return nil
	}

	merged := make([]core.Issue, 0, total)
	for _, group := range groups {
		merged = append(merged, group...)
	}
	return merged
}

func issueSummariesForStep(step core.EnsureStepID, issues []core.Issue, blockingOnly bool) []string {
	if len(issues) == 0 {
		return nil
	}

	summaries := make([]string, 0, len(issues))
	for _, issue := range issues {
		if blockingOnly && issue.Severity != core.SeverityError {
			continue
		}
		if !issueRelevantToStep(step, issue) {
			continue
		}
		summaries = append(summaries, issueSummary(issue))
	}

	return uniqueSortedRepoContextValues(summaries)
}

func issueRelevantToStep(step core.EnsureStepID, issue core.Issue) bool {
	if preferred := core.PreferredEnsureStepForIssue(issue).ID; preferred == step {
		return true
	}

	switch step {
	case core.EnsureStepPreprocessContract:
		return issue.Scope == core.IssueScopePreprocess
	case core.EnsureStepInputEncoders:
		return issue.Scope == core.IssueScopeInputEncoder
	case core.EnsureStepGroundTruthEncoders:
		return issue.Scope == core.IssueScopeGroundTruthEncoder
	case core.EnsureStepIntegrationTestContract:
		return issue.Scope == core.IssueScopeIntegrationTest
	case core.EnsureStepHarnessValidation:
		return issue.Scope == core.IssueScopeValidation
	case core.EnsureStepModelAcquisition:
		return issue.Scope == core.IssueScopeModel
	case core.EnsureStepModelContract:
		return issue.Scope == core.IssueScopeModel
	case core.EnsureStepInvestigate:
		return core.PreferredEnsureStepForIssue(issue).ID == core.EnsureStepInvestigate
	default:
		return false
	}
}

func issueSummary(issue core.Issue) string {
	message := strings.TrimSpace(issue.Message)
	if message == "" {
		message = "no details"
	}

	summary := fmt.Sprintf("%s: %s", issue.Code, message)
	if issue.Location == nil {
		return summary
	}

	location := strings.TrimSpace(issue.Location.Path)
	if location == "" {
		return summary
	}

	location = normalizeRepoContextPath(location)
	if issue.Location.Line > 0 {
		location = fmt.Sprintf("%s:%d", location, issue.Location.Line)
		if issue.Location.Column > 0 {
			location = fmt.Sprintf("%s:%d", location, issue.Location.Column)
		}
	}
	return fmt.Sprintf("%s @ %s", summary, location)
}

func leapYAMLBoundarySummary(snapshot core.WorkspaceSnapshot) string {
	if !hasSnapshotHash(snapshot, "leap.yaml") {
		return "leap.yaml missing"
	}

	boundaryPaths := make([]string, 0, len(snapshot.FileHashes))
	for path := range snapshot.FileHashes {
		normalized := normalizeRepoContextPath(path)
		if normalized == "" {
			continue
		}
		boundaryPaths = append(boundaryPaths, normalized)
	}

	boundaryPaths = truncateRepoContextValues(uniqueSortedRepoContextValues(boundaryPaths), maxRepoContextBoundaryFiles)
	if len(boundaryPaths) == 0 {
		return "leap.yaml present"
	}

	return fmt.Sprintf("leap.yaml present; tracked boundary files: %s", strings.Join(boundaryPaths, ", "))
}

func runtimeKindForContext(profile *core.LocalRuntimeProfile) string {
	if profile == nil {
		return ""
	}
	return strings.TrimSpace(profile.Kind)
}

func runtimeInterpreterForContext(profile *core.LocalRuntimeProfile) string {
	if profile == nil {
		return ""
	}
	return strings.TrimSpace(profile.InterpreterPath)
}

func runtimeStatusForContext(profile *core.LocalRuntimeProfile) string {
	if profile == nil {
		return ""
	}

	parts := make([]string, 0, 3)
	if profile.DependenciesReady {
		parts = append(parts, "dependencies ready")
	} else {
		parts = append(parts, "dependencies not ready")
	}

	switch {
	case profile.CodeLoaderReady || profile.CodeLoader.ProbeSucceeded:
		status := "code_loader import succeeded"
		if version := strings.TrimSpace(profile.CodeLoader.Version); version != "" {
			status = fmt.Sprintf("%s (%s)", status, version)
		}
		parts = append(parts, status)
	case profile.CodeLoaderDeclaredInProject:
		parts = append(parts, "code_loader import unavailable")
	}

	return strings.Join(parts, "; ")
}

func validateRequiredRepoContext(step core.EnsureStepID, context core.AgentRepoContext) error {
	_ = step
	_ = context
	return nil
}

func applyRepoContextStepSlice(step core.EnsureStepID, context *core.AgentRepoContext) {
	if context == nil {
		return
	}

	switch step {
	case core.EnsureStepModelAcquisition:
		context.RequiredInputSymbols = nil
		context.RequiredGroundTruthSymbols = nil
		context.DecoratorInventory = nil
		context.IntegrationTestCalls = nil
	case core.EnsureStepModelContract:
		context.RequiredInputSymbols = nil
		context.RequiredGroundTruthSymbols = nil
		context.DecoratorInventory = nil
		context.IntegrationTestCalls = nil
	case core.EnsureStepPreprocessContract:
		context.IntegrationTestCalls = nil
	case core.EnsureStepInputEncoders, core.EnsureStepGroundTruthEncoders:
		context.ModelCandidates = nil
		context.SelectedModelPath = ""
		context.IntegrationTestCalls = nil
		if step == core.EnsureStepInputEncoders {
			context.RequiredGroundTruthSymbols = nil
		} else {
			context.RequiredInputSymbols = nil
		}
	case core.EnsureStepIntegrationTestContract:
		context.ModelCandidates = nil
		context.SelectedModelPath = ""
	default:
		// Keep full context for investigate/harness and unknown fallbacks.
	}
}

func hasSnapshotHash(snapshot core.WorkspaceSnapshot, relativePath string) bool {
	if len(snapshot.FileHashes) == 0 {
		return false
	}
	_, ok := snapshot.FileHashes[relativePath]
	return ok
}

func uniqueSortedRepoContextValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	byKey := make(map[string]string, len(values))
	for _, value := range values {
		normalized := normalizeRepoContextPath(value)
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, exists := byKey[key]; exists {
			continue
		}
		byKey[key] = normalized
	}
	if len(byKey) == 0 {
		return nil
	}

	unique := make([]string, 0, len(byKey))
	for _, value := range byKey {
		unique = append(unique, value)
	}

	sort.Slice(unique, func(i, j int) bool {
		left := strings.ToLower(unique[i])
		right := strings.ToLower(unique[j])
		if left != right {
			return left < right
		}
		return unique[i] < unique[j]
	})
	return unique
}

func truncateRepoContextValues(values []string, limit int) []string {
	if len(values) == 0 || limit <= 0 || len(values) <= limit {
		return values
	}
	return append([]string(nil), values[:limit]...)
}

func normalizeRepoContextPath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		return trimmed
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))
	if cleaned == "." {
		return ""
	}
	return cleaned
}
