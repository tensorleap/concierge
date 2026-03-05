package inspect

import (
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func postProcessInputGTFindings(
	findings core.InputGTNormalizedFindings,
	detectedFramework string,
) core.InputGTNormalizedFindings {
	postProcessed := findings
	postProcessed.Inputs = postProcessInputCandidates(findings.Inputs, detectedFramework)
	postProcessed.GroundTruths = postProcessGroundTruthCandidates(findings.GroundTruths)
	postProcessed.ProposedMapping = postProcessProposedMapping(
		findings.ProposedMapping,
		postProcessed.Inputs,
		postProcessed.GroundTruths,
	)
	return postProcessed
}

func postProcessInputCandidates(
	inputs []core.InputGTCandidate,
	detectedFramework string,
) []core.InputGTCandidate {
	if len(inputs) == 0 {
		return nil
	}

	normalized := append([]core.InputGTCandidate(nil), inputs...)
	index := make(map[string]core.InputGTCandidate, len(normalized))
	for _, candidate := range normalized {
		index[candidate.Name] = candidate
	}

	shouldSplitTokenizerDict := strings.EqualFold(strings.TrimSpace(detectedFramework), "tensorflow") ||
		strings.EqualFold(strings.TrimSpace(detectedFramework), "pytorch") ||
		strings.EqualFold(strings.TrimSpace(detectedFramework), "mixed")

	if shouldSplitTokenizerDict {
		tokenizerDerived := tokenizerDerivedInputs(normalized)
		for _, candidate := range tokenizerDerived {
			existing, exists := index[candidate.Name]
			if !exists {
				index[candidate.Name] = candidate
				continue
			}
			existing.Evidence = dedupeInputGTEvidence(append(existing.Evidence, candidate.Evidence...))
			if existing.ShapeHint == "" {
				existing.ShapeHint = candidate.ShapeHint
			}
			if existing.DTypeHint == "" {
				existing.DTypeHint = candidate.DTypeHint
			}
			if existing.SemanticHint == "" {
				existing.SemanticHint = candidate.SemanticHint
			}
			index[candidate.Name] = existing
		}

		if tokens, exists := index["input_tokens"]; exists {
			tokens.Condition = "when tokenizer returns packed tokens instead of explicit key tensors"
			tokens.Confidence = "low"
			index["input_tokens"] = tokens
		}
	}

	ordered := make([]core.InputGTCandidate, 0, len(index))
	for _, candidate := range index {
		ordered = append(ordered, candidate)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		leftConditional := strings.TrimSpace(ordered[i].Condition) != ""
		rightConditional := strings.TrimSpace(ordered[j].Condition) != ""
		if leftConditional != rightConditional {
			return !leftConditional
		}
		if ordered[i].Name != ordered[j].Name {
			return ordered[i].Name < ordered[j].Name
		}
		return ordered[i].Confidence > ordered[j].Confidence
	})
	return ordered
}

func tokenizerDerivedInputs(inputs []core.InputGTCandidate) []core.InputGTCandidate {
	derived := make([]core.InputGTCandidate, 0, 3)
	byName := make(map[string]struct{}, len(inputs))
	for _, candidate := range inputs {
		byName[candidate.Name] = struct{}{}
	}

	for _, required := range []string{"input_ids", "attention_masks", "token_type_ids"} {
		if _, exists := byName[required]; exists {
			continue
		}
		if !hasTokenizerEvidence(inputs, required) {
			continue
		}
		derived = append(derived, core.InputGTCandidate{
			Name:         required,
			SemanticHint: "derived tokenizer dictionary key candidate",
			Confidence:   "medium",
			Evidence:     tokenizerEvidence(inputs, required),
		})
	}

	return derived
}

func hasTokenizerEvidence(inputs []core.InputGTCandidate, key string) bool {
	return len(tokenizerEvidence(inputs, key)) > 0
}

func tokenizerEvidence(inputs []core.InputGTCandidate, key string) []core.InputGTEvidence {
	evidence := make([]core.InputGTEvidence, 0, 4)
	aliases := tokenizerKeyAliases(key)
	for _, candidate := range inputs {
		for _, item := range candidate.Evidence {
			snippet := strings.ToLower(item.Snippet)
			for _, alias := range aliases {
				if strings.Contains(snippet, alias) {
					evidence = append(evidence, item)
					break
				}
			}
		}
	}
	return dedupeInputGTEvidence(evidence)
}

func tokenizerKeyAliases(key string) []string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "attention_masks":
		return []string{"attention_masks", "attention_mask"}
	case "token_type_ids":
		return []string{"token_type_ids", "token_type_id"}
	default:
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "" {
			return nil
		}
		return []string{normalized}
	}
}

func postProcessGroundTruthCandidates(groundTruths []core.InputGTCandidate) []core.InputGTCandidate {
	if len(groundTruths) == 0 {
		return nil
	}
	normalized := uniqueSortedInputGTCandidates(groundTruths)
	sort.SliceStable(normalized, func(i, j int) bool {
		leftConditional := strings.TrimSpace(normalized[i].Condition) != ""
		rightConditional := strings.TrimSpace(normalized[j].Condition) != ""
		if leftConditional != rightConditional {
			return !leftConditional
		}
		return normalized[i].Name < normalized[j].Name
	})
	return normalized
}

func postProcessProposedMapping(
	mapping []core.InputGTProposedMapping,
	inputs []core.InputGTCandidate,
	groundTruths []core.InputGTCandidate,
) []core.InputGTProposedMapping {
	index := make(map[string]core.InputGTProposedMapping, len(mapping)+len(inputs)+len(groundTruths))
	for _, entry := range mapping {
		key := strings.ToLower(strings.Join([]string{entry.EncoderType, entry.Name, entry.SourceCandidate, entry.Condition}, "|"))
		index[key] = entry
	}

	for _, candidate := range inputs {
		entry := core.InputGTProposedMapping{
			EncoderType:     "input",
			Name:            candidate.Name,
			SourceCandidate: candidate.Name,
			Confidence:      candidate.Confidence,
			Condition:       candidate.Condition,
		}
		key := strings.ToLower(strings.Join([]string{entry.EncoderType, entry.Name, entry.SourceCandidate, entry.Condition}, "|"))
		if _, exists := index[key]; exists {
			continue
		}
		index[key] = entry
	}
	for _, candidate := range groundTruths {
		entry := core.InputGTProposedMapping{
			EncoderType:     "ground_truth",
			Name:            candidate.Name,
			SourceCandidate: candidate.Name,
			Confidence:      candidate.Confidence,
			Condition:       candidate.Condition,
		}
		key := strings.ToLower(strings.Join([]string{entry.EncoderType, entry.Name, entry.SourceCandidate, entry.Condition}, "|"))
		if _, exists := index[key]; exists {
			continue
		}
		index[key] = entry
	}

	ordered := make([]core.InputGTProposedMapping, 0, len(index))
	for _, entry := range index {
		ordered = append(ordered, entry)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].EncoderType != ordered[j].EncoderType {
			return ordered[i].EncoderType < ordered[j].EncoderType
		}
		leftConditional := strings.TrimSpace(ordered[i].Condition) != ""
		rightConditional := strings.TrimSpace(ordered[j].Condition) != ""
		if leftConditional != rightConditional {
			return !leftConditional
		}
		if ordered[i].Name != ordered[j].Name {
			return ordered[i].Name < ordered[j].Name
		}
		return ordered[i].SourceCandidate < ordered[j].SourceCandidate
	})
	return ordered
}
