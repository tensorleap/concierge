package inspect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

const (
	inputGTFindingsSchemaVersion = "1.0.0"
	inputGTFindingsMethodVersion = "go-semantic-findings-v1"
)

var (
	inputGTIdentifierPattern   = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\b`)
	inputGTTokenizerKeyPattern = regexp.MustCompile(`['"]((?:input_ids|attention_mask|attention_masks|token_type_ids|token_type_id|pixel_values))['"]`)
)

func inspectInputGTDiscovery(ctx context.Context, snapshot core.WorkspaceSnapshot, status *core.IntegrationStatus) error {
	_ = ctx

	if status == nil || status.Contracts == nil {
		return nil
	}

	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return nil
	}

	artifacts := &core.InputGTDiscoveryArtifacts{
		FixtureState: &core.InputGTFixtureState{
			RepoRoot:            repoRoot,
			SnapshotID:          strings.TrimSpace(snapshot.ID),
			WorktreeFingerprint: strings.TrimSpace(snapshot.WorktreeFingerprint),
		},
	}

	leadExtractor := newFrameworkLeadExtractor()
	leadPack, leadSummary, err := leadExtractor.Extract(repoRoot)
	if err != nil {
		return err
	}
	artifacts.LeadPack = &leadPack
	artifacts.LeadSummary = leadSummary

	promptBundle := buildInputGTAgentPromptBundle(repoRoot, leadSummary, leadPack)
	artifacts.AgentPromptBundle = &promptBundle

	rawOutput, rawPayload, err := runInputGTInvestigator(repoRoot, leadPack)
	if err != nil {
		return err
	}
	artifacts.AgentRawOutput = &rawOutput

	findings, normalizeErr := normalizeInputGTFindingsPayload(rawPayload)
	if normalizeErr != nil {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeInputEncoderCoverageIncomplete,
			Message:  fmt.Sprintf("input/GT discovery normalization failed: %v", normalizeErr),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeInputEncoder,
		})
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeGTEncoderCoverageIncomplete,
			Message:  fmt.Sprintf("input/GT discovery normalization failed: %v", normalizeErr),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeGroundTruthEncoder,
		})
		status.Contracts.InputGTDiscovery = artifacts
		return nil
	}

	findings = postProcessInputGTFindings(findings, leadPack.FrameworkDetection.Candidate)
	artifacts.NormalizedFindings = &findings

	runtimeInputs, runtimeNotes := detectRuntimeModelInputs(repoRoot, status.Contracts)
	comparison := buildInputGTComparisonReport(findings, runtimeInputs, runtimeNotes)
	artifacts.ComparisonReport = &comparison
	status.Contracts.InputGTDiscovery = artifacts

	applyInputGTContractSymbols(status.Contracts, snapshot)
	return nil
}

func buildInputGTAgentPromptBundle(
	repoRoot string,
	leadSummary string,
	leadPack core.InputGTLeadPack,
) core.InputGTAgentPromptBundle {
	systemPrompt := strings.TrimSpace(strings.Join([]string{
		"You are a read-only semantic investigator for Tensorleap input/ground-truth discovery.",
		"Trace repository evidence from the provided lead summary.",
		"Return evidence-backed candidates and uncertainty notes.",
		"Do not edit files.",
	}, "\n"))

	userPrompt := strings.TrimSpace(strings.Join([]string{
		fmt.Sprintf("Repository: %s", repoRoot),
		fmt.Sprintf("Method: %s", leadPack.MethodVersion),
		"Task: identify candidate model inputs, candidate ground truths, and encoder mapping suggestions.",
		"Lead summary:",
		leadSummary,
	}, "\n"))

	return core.InputGTAgentPromptBundle{
		SystemPrompt:              systemPrompt,
		UserPrompt:                userPrompt,
		ReadOnly:                  true,
		LeadPackReadSuccess:       true,
		LeadPackReadInformational: true,
	}
}

func runInputGTInvestigator(repoRoot string, leadPack core.InputGTLeadPack) (core.InputGTAgentRawOutput, []byte, error) {
	inputCandidates := make([]core.InputGTCandidate, 0, 8)
	groundTruthCandidates := make([]core.InputGTCandidate, 0, 8)

	inputIndex := make(map[string]core.InputGTCandidate)
	gtIndex := make(map[string]core.InputGTCandidate)

	for _, leadFile := range leadPack.Files {
		absolutePath := filepath.Join(repoRoot, filepath.FromSlash(leadFile.Path))
		raw, err := os.ReadFile(absolutePath)
		if err != nil {
			continue
		}
		lines := strings.Split(string(raw), "\n")

		for lineNumber, line := range lines {
			candidateInputs, candidateGTs := extractInputGTCandidatesFromLine(line)
			if len(candidateInputs) == 0 && len(candidateGTs) == 0 {
				continue
			}
			snippet := strings.TrimSpace(line)
			evidence := core.InputGTEvidence{
				File:    leadFile.Path,
				Line:    lineNumber + 1,
				Snippet: snippet,
			}

			for _, symbol := range candidateInputs {
				upsertInputGTCandidate(inputIndex, symbol, evidence, true)
			}
			for _, symbol := range candidateGTs {
				upsertInputGTCandidate(gtIndex, symbol, evidence, false)
			}
		}
	}

	inputCandidates = mapInputGTCandidates(inputIndex)
	groundTruthCandidates = mapInputGTCandidates(gtIndex)

	proposedMapping := make([]core.InputGTProposedMapping, 0, len(inputCandidates)+len(groundTruthCandidates))
	for _, candidate := range inputCandidates {
		proposedMapping = append(proposedMapping, core.InputGTProposedMapping{
			EncoderType:     "input",
			Name:            candidate.Name,
			SourceCandidate: candidate.Name,
			Confidence:      candidate.Confidence,
		})
	}
	for _, candidate := range groundTruthCandidates {
		proposedMapping = append(proposedMapping, core.InputGTProposedMapping{
			EncoderType:     "ground_truth",
			Name:            candidate.Name,
			SourceCandidate: candidate.Name,
			Confidence:      candidate.Confidence,
		})
	}
	sort.SliceStable(proposedMapping, func(i, j int) bool {
		if proposedMapping[i].EncoderType != proposedMapping[j].EncoderType {
			return proposedMapping[i].EncoderType < proposedMapping[j].EncoderType
		}
		return proposedMapping[i].Name < proposedMapping[j].Name
	})

	payload := map[string]any{
		"schema_version":          inputGTFindingsSchemaVersion,
		"method_version":          inputGTFindingsMethodVersion,
		"candidate_inputs":        inputCandidates,
		"candidate_ground_truths": groundTruthCandidates,
		"proposed_mapping":        proposedMapping,
		"unknowns":                []string{},
		"comments":                "deterministic semantic extraction from framework lead files",
	}
	serialized, err := json.Marshal(payload)
	if err != nil {
		return core.InputGTAgentRawOutput{}, nil, core.WrapError(core.KindUnknown, "inspect.input_gt_discovery.marshal_raw_payload", err)
	}

	rawOutput := core.InputGTAgentRawOutput{
		Provider: "deterministic_local",
		Model:    "regex-semantic-v1",
		Payload:  string(serialized),
		Metadata: map[string]string{
			"read_only": "true",
		},
	}
	return rawOutput, serialized, nil
}

func extractInputGTCandidatesFromLine(line string) ([]string, []string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil, nil
	}

	inputs := make([]string, 0, 6)
	groundTruths := make([]string, 0, 6)

	for _, match := range inputGTTokenizerKeyPattern.FindAllStringSubmatch(trimmed, -1) {
		if len(match) < 2 {
			continue
		}
		symbol := canonicalDiscoveredSymbol(match[1])
		if symbol == "" {
			continue
		}
		inputs = append(inputs, symbol)
	}

	lower := strings.ToLower(trimmed)
	identifiers := make([]string, 0, 10)
	for _, match := range inputGTIdentifierPattern.FindAllStringSubmatch(trimmed, -1) {
		if len(match) < 2 {
			continue
		}
		identifiers = append(identifiers, strings.ToLower(match[1]))
	}

	if strings.Contains(lower, "model(") || strings.Contains(lower, ".predict(") || strings.Contains(lower, "session.run(") {
		for _, identifier := range identifiers {
			if symbol, ok := classifyInputIdentifier(identifier); ok {
				inputs = append(inputs, symbol)
			}
		}
	}

	if strings.Contains(lower, "loss(") || strings.Contains(lower, "criterion(") || strings.Contains(lower, "target") || strings.Contains(lower, "label") {
		for _, identifier := range identifiers {
			if symbol, ok := classifyGroundTruthIdentifier(identifier); ok {
				groundTruths = append(groundTruths, symbol)
			}
		}
	}

	if strings.HasPrefix(lower, "for ") && strings.Contains(lower, " in ") {
		for _, identifier := range identifiers {
			if symbol, ok := classifyInputIdentifier(identifier); ok {
				inputs = append(inputs, symbol)
			}
			if symbol, ok := classifyGroundTruthIdentifier(identifier); ok {
				groundTruths = append(groundTruths, symbol)
			}
		}
	}

	return uniqueSortedContractSymbols(inputs), uniqueSortedContractSymbols(groundTruths)
}

func classifyInputIdentifier(identifier string) (string, bool) {
	switch identifier {
	case "image", "images", "img", "imgs", "pixel_values":
		return "image", true
	case "input", "inputs":
		return "input", true
	case "input_ids", "input_id":
		return "input_ids", true
	case "attention_mask", "attention_masks":
		return "attention_masks", true
	case "token_type_ids", "token_type_id":
		return "token_type_ids", true
	}
	if strings.Contains(identifier, "image") || strings.Contains(identifier, "pixel") {
		return "image", true
	}
	return "", false
}

func classifyGroundTruthIdentifier(identifier string) (string, bool) {
	switch identifier {
	case "label", "labels", "class", "classes", "cls", "targets", "target", "gt", "ground_truth":
		return "classes", true
	case "bbs", "bbox", "bboxes", "boxes":
		return "bbs", true
	case "sentiment":
		return "sentiment", true
	}
	if strings.Contains(identifier, "label") || strings.Contains(identifier, "target") || strings.Contains(identifier, "class") {
		return "classes", true
	}
	return "", false
}

func canonicalDiscoveredSymbol(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "attention_mask":
		return "attention_masks"
	case "token_type_id":
		return "token_type_ids"
	}
	return normalized
}

func upsertInputGTCandidate(
	index map[string]core.InputGTCandidate,
	symbol string,
	evidence core.InputGTEvidence,
	inputCandidate bool,
) {
	name := canonicalDiscoveredSymbol(symbol)
	if name == "" {
		return
	}
	current, ok := index[name]
	if !ok {
		semanticHint := "candidate discovered from semantic lead tracing"
		if inputCandidate {
			semanticHint = "candidate model input discovered from semantic lead tracing"
		} else {
			semanticHint = "candidate ground truth discovered from semantic lead tracing"
		}
		current = core.InputGTCandidate{
			Name:         name,
			SemanticHint: semanticHint,
			Confidence:   "medium",
		}
	}

	current.Evidence = append(current.Evidence, evidence)
	sort.SliceStable(current.Evidence, func(i, j int) bool {
		if current.Evidence[i].File != current.Evidence[j].File {
			return current.Evidence[i].File < current.Evidence[j].File
		}
		if current.Evidence[i].Line != current.Evidence[j].Line {
			return current.Evidence[i].Line < current.Evidence[j].Line
		}
		return current.Evidence[i].Snippet < current.Evidence[j].Snippet
	})

	if len(current.Evidence) > 5 {
		current.Evidence = append([]core.InputGTEvidence(nil), current.Evidence[:5]...)
	}
	index[name] = current
}

func mapInputGTCandidates(index map[string]core.InputGTCandidate) []core.InputGTCandidate {
	if len(index) == 0 {
		return nil
	}
	names := make([]string, 0, len(index))
	for name := range index {
		names = append(names, name)
	}
	sort.Strings(names)

	candidates := make([]core.InputGTCandidate, 0, len(names))
	for _, name := range names {
		candidates = append(candidates, index[name])
	}
	return candidates
}

func buildInputGTComparisonReport(
	findings core.InputGTNormalizedFindings,
	runtimeInputs []string,
	runtimeNotes []string,
) core.InputGTComparisonReport {
	primaryInputs := make([]string, 0, len(findings.Inputs))
	conditionalInputs := make([]string, 0, len(findings.Inputs))
	for _, candidate := range findings.Inputs {
		if strings.TrimSpace(candidate.Condition) != "" {
			conditionalInputs = append(conditionalInputs, candidate.Name)
			continue
		}
		primaryInputs = append(primaryInputs, candidate.Name)
	}

	primaryGroundTruths := make([]string, 0, len(findings.GroundTruths))
	conditionalGroundTruths := make([]string, 0, len(findings.GroundTruths))
	for _, candidate := range findings.GroundTruths {
		if strings.TrimSpace(candidate.Condition) != "" {
			conditionalGroundTruths = append(conditionalGroundTruths, candidate.Name)
			continue
		}
		primaryGroundTruths = append(primaryGroundTruths, candidate.Name)
	}

	primaryInputSymbols := uniqueSortedContractSymbols(primaryInputs)
	primaryGroundTruthSymbols := uniqueSortedContractSymbols(primaryGroundTruths)
	conditionalInputSymbols := uniqueSortedContractSymbols(conditionalInputs)
	conditionalGroundTruthSymbols := uniqueSortedContractSymbols(conditionalGroundTruths)

	runtimeSymbols := uniqueSortedContractSymbols(runtimeInputs)
	runtimeOnly := missingContractSymbols(runtimeSymbols, primaryInputSymbols)
	discoveryOnly := missingContractSymbols(primaryInputSymbols, runtimeSymbols)
	notes := uniqueSortedContractSymbols(runtimeNotes)

	return core.InputGTComparisonReport{
		PrimaryInputSymbols:           primaryInputSymbols,
		PrimaryGroundTruthSymbols:     primaryGroundTruthSymbols,
		ConditionalInputSymbols:       conditionalInputSymbols,
		ConditionalGroundTruthSymbols: conditionalGroundTruthSymbols,
		RuntimeInputSymbols:           runtimeSymbols,
		RuntimeOnlyInputSymbols:       runtimeOnly,
		DiscoveryOnlyInputSymbols:     discoveryOnly,
		Notes:                         notes,
	}
}

func applyInputGTContractSymbols(contracts *core.IntegrationContracts, snapshot core.WorkspaceSnapshot) {
	if contracts == nil {
		return
	}

	if contracts.ConfirmedMapping == nil && snapshot.ConfirmedEncoderMapping != nil {
		confirmed := *snapshot.ConfirmedEncoderMapping
		contracts.ConfirmedMapping = &confirmed
	}

	if contracts.ConfirmedMapping != nil {
		contracts.DiscoveredInputSymbols = uniqueSortedContractSymbols(contracts.ConfirmedMapping.InputSymbols)
		contracts.DiscoveredGroundTruthSymbols = uniqueSortedContractSymbols(contracts.ConfirmedMapping.GroundTruthSymbols)
		return
	}

	if contracts.InputGTDiscovery == nil || contracts.InputGTDiscovery.ComparisonReport == nil {
		return
	}
	contracts.DiscoveredInputSymbols = uniqueSortedContractSymbols(
		contracts.InputGTDiscovery.ComparisonReport.PrimaryInputSymbols,
	)
	contracts.DiscoveredGroundTruthSymbols = uniqueSortedContractSymbols(
		contracts.InputGTDiscovery.ComparisonReport.PrimaryGroundTruthSymbols,
	)
}
