package inspect

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func normalizeInputGTFindingsPayload(payload []byte) (core.InputGTNormalizedFindings, error) {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		return core.InputGTNormalizedFindings{}, core.NewError(
			core.KindUnknown,
			"inspect.input_gt_normalizer.empty_payload",
			"input/GT discovery returned an empty payload",
		)
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return core.InputGTNormalizedFindings{}, core.WrapError(
			core.KindUnknown,
			"inspect.input_gt_normalizer.unmarshal_payload",
			err,
		)
	}

	inputItems, inputSourceKey, err := normalizeInputGTCandidatesField(raw,
		[]string{"inputs", "model_inputs", "candidate_inputs"},
	)
	if err != nil {
		return core.InputGTNormalizedFindings{}, err
	}
	groundTruthItems, gtSourceKey, err := normalizeInputGTCandidatesField(raw,
		[]string{"ground_truths", "targets", "candidate_ground_truths"},
	)
	if err != nil {
		return core.InputGTNormalizedFindings{}, err
	}

	proposedMapping, err := normalizeInputGTProposedMapping(raw)
	if err != nil {
		return core.InputGTNormalizedFindings{}, err
	}
	unknowns := normalizeInputGTUnknowns(raw["unknowns"])
	comments := normalizeInputGTComments(raw["comments"])

	schemaVersion := strings.TrimSpace(asString(raw["schema_version"]))
	if schemaVersion == "" {
		schemaVersion = strings.TrimSpace(asString(raw["schemaVersion"]))
	}
	if schemaVersion == "" {
		schemaVersion = inputGTFindingsSchemaVersion
	}

	methodVersion := strings.TrimSpace(asString(raw["method_version"]))
	if methodVersion == "" {
		methodVersion = strings.TrimSpace(asString(raw["methodVersion"]))
	}
	if methodVersion == "" {
		methodVersion = inputGTFindingsMethodVersion
	}

	findings := core.InputGTNormalizedFindings{
		SchemaVersion:   schemaVersion,
		MethodVersion:   methodVersion,
		Inputs:          inputItems,
		GroundTruths:    groundTruthItems,
		ProposedMapping: proposedMapping,
		Unknowns:        unknowns,
		Comments:        comments,
	}
	if len(findings.Inputs) == 0 && len(findings.GroundTruths) == 0 {
		sources := make([]string, 0, 2)
		if strings.TrimSpace(inputSourceKey) != "" {
			sources = append(sources, "inputs="+inputSourceKey)
		}
		if strings.TrimSpace(gtSourceKey) != "" {
			sources = append(sources, "ground_truths="+gtSourceKey)
		}
		notice := "discovery produced no input/ground-truth candidates"
		if len(sources) > 0 {
			notice += " (source fields: " + strings.Join(sources, ", ") + ")"
		}
		findings.Unknowns = append(findings.Unknowns, notice)
	}
	return findings, nil
}

func normalizeInputGTCandidatesField(
	raw map[string]any,
	keys []string,
) ([]core.InputGTCandidate, string, error) {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if value == nil {
			continue
		}
		candidates, err := parseInputGTCandidates(value)
		if err != nil {
			return nil, key, core.WrapError(
				core.KindUnknown,
				"inspect.input_gt_normalizer.parse_candidates."+key,
				err,
			)
		}
		return candidates, key, nil
	}
	return nil, "", nil
}

func parseInputGTCandidates(value any) ([]core.InputGTCandidate, error) {
	if value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case []any:
		return parseInputGTCandidateList(typed)
	case map[string]any:
		return parseInputGTCandidateMap(typed)
	default:
		return nil, fmt.Errorf("expected candidates list/object, got %T", value)
	}
}

func parseInputGTCandidateList(items []any) ([]core.InputGTCandidate, error) {
	candidates := make([]core.InputGTCandidate, 0, len(items))
	for _, rawItem := range items {
		parsed, ok, err := parseInputGTCandidate(rawItem, "")
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		candidates = append(candidates, parsed)
	}
	return uniqueSortedInputGTCandidates(candidates), nil
}

func parseInputGTCandidateMap(items map[string]any) ([]core.InputGTCandidate, error) {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	candidates := make([]core.InputGTCandidate, 0, len(keys))
	for _, key := range keys {
		parsed, ok, err := parseInputGTCandidate(items[key], key)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		candidates = append(candidates, parsed)
	}
	return uniqueSortedInputGTCandidates(candidates), nil
}

func parseInputGTCandidate(value any, mapKeyName string) (core.InputGTCandidate, bool, error) {
	switch typed := value.(type) {
	case string:
		name := normalizeInputGTCandidateName(typed)
		if name == "" {
			name = normalizeInputGTCandidateName(mapKeyName)
		}
		if name == "" {
			return core.InputGTCandidate{}, false, nil
		}
		return core.InputGTCandidate{
			Name:       name,
			Confidence: "medium",
		}, true, nil
	case map[string]any:
		name := normalizeInputGTCandidateName(asString(typed["name"]))
		if name == "" {
			name = normalizeInputGTCandidateName(asString(typed["source_function"]))
		}
		if name == "" {
			name = normalizeInputGTCandidateName(asString(typed["source_candidate"]))
		}
		if name == "" {
			name = normalizeInputGTCandidateName(mapKeyName)
		}
		if name == "" {
			return core.InputGTCandidate{}, false, nil
		}

		evidence := normalizeInputGTEvidence(typed)
		candidate := core.InputGTCandidate{
			Name: name,
			SemanticHint: strings.TrimSpace(firstNonEmptyString(
				asString(typed["semantic_hint"]),
				asString(typed["semanticHint"]),
				asString(typed["description"]),
			)),
			ShapeHint: strings.TrimSpace(firstNonEmptyString(
				asString(typed["shape_hint"]),
				asString(typed["shapeHint"]),
				asString(typed["shape"]),
			)),
			DTypeHint: strings.TrimSpace(firstNonEmptyString(
				asString(typed["dtype_hint"]),
				asString(typed["dtypeHint"]),
				asString(typed["dtype"]),
			)),
			Confidence: normalizeConfidence(asString(typed["confidence"])),
			Evidence:   evidence,
			Condition: strings.TrimSpace(firstNonEmptyString(
				asString(typed["condition"]),
				asString(typed["branch_condition"]),
				asString(typed["when"]),
			)),
		}
		return candidate, true, nil
	default:
		return core.InputGTCandidate{}, false, nil
	}
}

func normalizeInputGTEvidence(raw map[string]any) []core.InputGTEvidence {
	evidence := make([]core.InputGTEvidence, 0, 4)

	if list, ok := raw["evidence"].([]any); ok {
		for _, item := range list {
			entry, include := parseInputGTEvidenceEntry(item)
			if !include {
				continue
			}
			evidence = append(evidence, entry)
		}
	}

	if len(evidence) == 0 {
		file := strings.TrimSpace(firstNonEmptyString(
			asString(raw["file"]),
			asString(raw["source_module"]),
		))
		line := asPositiveInt(raw["line"], 1)
		snippet := strings.TrimSpace(firstNonEmptyString(
			asString(raw["snippet"]),
			asString(raw["description"]),
		))
		if file != "" {
			evidence = append(evidence, core.InputGTEvidence{
				File:    file,
				Line:    line,
				Snippet: snippet,
			})
		}
	}

	sort.SliceStable(evidence, func(i, j int) bool {
		if evidence[i].File != evidence[j].File {
			return evidence[i].File < evidence[j].File
		}
		if evidence[i].Line != evidence[j].Line {
			return evidence[i].Line < evidence[j].Line
		}
		return evidence[i].Snippet < evidence[j].Snippet
	})
	return dedupeInputGTEvidence(evidence)
}

func parseInputGTEvidenceEntry(raw any) (core.InputGTEvidence, bool) {
	item, ok := raw.(map[string]any)
	if !ok {
		return core.InputGTEvidence{}, false
	}

	file := strings.TrimSpace(asString(item["file"]))
	if file == "" {
		return core.InputGTEvidence{}, false
	}
	line := asPositiveInt(item["line"], 1)
	snippet := strings.TrimSpace(firstNonEmptyString(
		asString(item["snippet"]),
		asString(item["description"]),
	))
	return core.InputGTEvidence{
		File:    file,
		Line:    line,
		Snippet: snippet,
	}, true
}

func normalizeInputGTProposedMapping(raw map[string]any) ([]core.InputGTProposedMapping, error) {
	switch {
	case raw["proposed_mapping"] != nil:
		return parseInputGTMappingList(raw["proposed_mapping"])
	case raw["proposed_encoder_mapping"] != nil:
		return parseInputGTMappingObject(raw["proposed_encoder_mapping"])
	case raw["encoder_mapping"] != nil:
		return parseInputGTMappingList(raw["encoder_mapping"])
	default:
		return nil, nil
	}
}

func parseInputGTMappingList(value any) ([]core.InputGTProposedMapping, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("mapping list has invalid type %T", value)
	}

	mapping := make([]core.InputGTProposedMapping, 0, len(items))
	for _, rawItem := range items {
		entry, include := parseInputGTMappingEntry(rawItem, "")
		if !include {
			continue
		}
		mapping = append(mapping, entry)
	}
	sort.SliceStable(mapping, func(i, j int) bool {
		if mapping[i].EncoderType != mapping[j].EncoderType {
			return mapping[i].EncoderType < mapping[j].EncoderType
		}
		if mapping[i].Name != mapping[j].Name {
			return mapping[i].Name < mapping[j].Name
		}
		return mapping[i].SourceCandidate < mapping[j].SourceCandidate
	})
	return dedupeInputGTMapping(mapping), nil
}

func parseInputGTMappingObject(value any) ([]core.InputGTProposedMapping, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mapping object has invalid type %T", value)
	}

	mapping := make([]core.InputGTProposedMapping, 0, 2)
	if rawInput, ok := object["input_encoder"]; ok {
		entry, include := parseInputGTMappingEntry(rawInput, "input")
		if include {
			mapping = append(mapping, entry)
		}
	}
	if rawGT, ok := object["ground_truth_encoder"]; ok {
		entry, include := parseInputGTMappingEntry(rawGT, "ground_truth")
		if include {
			mapping = append(mapping, entry)
		}
	}
	return dedupeInputGTMapping(mapping), nil
}

func parseInputGTMappingEntry(value any, defaultRole string) (core.InputGTProposedMapping, bool) {
	item, ok := value.(map[string]any)
	if !ok {
		return core.InputGTProposedMapping{}, false
	}

	encoderType := strings.TrimSpace(firstNonEmptyString(
		asString(item["encoder_type"]),
		asString(item["encoderType"]),
		asString(item["role"]),
		defaultRole,
	))
	if strings.EqualFold(encoderType, "gt") {
		encoderType = "ground_truth"
	}
	if encoderType != "input" && encoderType != "ground_truth" {
		return core.InputGTProposedMapping{}, false
	}

	name := normalizeInputGTCandidateName(firstNonEmptyString(
		asString(item["name"]),
		asString(item["encoder_name"]),
		asString(item["leap_binder_function"]),
	))
	if name == "" {
		return core.InputGTProposedMapping{}, false
	}

	source := normalizeInputGTCandidateName(firstNonEmptyString(
		asString(item["source_candidate"]),
		asString(item["source"]),
		asString(item["maps_to_candidate"]),
		asString(item["maps_to"]),
		name,
	))
	if source == "" {
		source = name
	}

	return core.InputGTProposedMapping{
		EncoderType:     encoderType,
		Name:            name,
		SourceCandidate: source,
		Confidence:      normalizeConfidence(asString(item["confidence"])),
		Notes: strings.TrimSpace(firstNonEmptyString(
			asString(item["notes"]),
			asString(item["description"]),
			asString(item["rationale"]),
		)),
		Condition: strings.TrimSpace(firstNonEmptyString(
			asString(item["condition"]),
			asString(item["when"]),
		)),
	}, true
}

func normalizeInputGTUnknowns(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	unknowns := make([]string, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed != "" {
				unknowns = append(unknowns, trimmed)
			}
		case map[string]any:
			description := strings.TrimSpace(asString(typed["description"]))
			if description != "" {
				unknowns = append(unknowns, description)
			}
		}
	}
	sort.Strings(unknowns)
	return uniqueStrings(unknowns)
}

func normalizeInputGTComments(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			trimmed := strings.TrimSpace(asString(item))
			if trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]any:
		raw, _ := json.Marshal(typed)
		return strings.TrimSpace(string(raw))
	default:
		return ""
	}
}

func normalizeConfidence(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "high", "medium", "low":
		return normalized
	default:
		return "medium"
	}
}

func normalizeInputGTCandidateName(value string) string {
	return canonicalDiscoveredSymbol(value)
}

func asPositiveInt(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" && trimmed != "<nil>" {
			return trimmed
		}
	}
	return ""
}

func uniqueSortedInputGTCandidates(values []core.InputGTCandidate) []core.InputGTCandidate {
	if len(values) == 0 {
		return nil
	}

	index := make(map[string]core.InputGTCandidate, len(values))
	for _, value := range values {
		name := normalizeInputGTCandidateName(value.Name)
		if name == "" {
			continue
		}
		value.Name = name
		if value.Confidence == "" {
			value.Confidence = "medium"
		}

		existing, exists := index[name]
		if !exists {
			index[name] = value
			continue
		}

		merged := existing
		if merged.SemanticHint == "" {
			merged.SemanticHint = value.SemanticHint
		}
		if merged.ShapeHint == "" {
			merged.ShapeHint = value.ShapeHint
		}
		if merged.DTypeHint == "" {
			merged.DTypeHint = value.DTypeHint
		}
		if merged.Condition == "" {
			merged.Condition = value.Condition
		}
		if merged.Confidence == "low" && value.Confidence != "low" {
			merged.Confidence = value.Confidence
		}
		merged.Evidence = dedupeInputGTEvidence(append(merged.Evidence, value.Evidence...))
		index[name] = merged
	}

	names := make([]string, 0, len(index))
	for name := range index {
		names = append(names, name)
	}
	sort.Strings(names)

	normalized := make([]core.InputGTCandidate, 0, len(names))
	for _, name := range names {
		normalized = append(normalized, index[name])
	}
	return normalized
}

func dedupeInputGTEvidence(values []core.InputGTEvidence) []core.InputGTEvidence {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	unique := make([]core.InputGTEvidence, 0, len(values))
	for _, value := range values {
		file := strings.TrimSpace(value.File)
		if file == "" {
			continue
		}
		key := fmt.Sprintf("%s:%d:%s", file, value.Line, strings.TrimSpace(value.Snippet))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, core.InputGTEvidence{
			File:    file,
			Line:    asPositiveInt(value.Line, 1),
			Snippet: strings.TrimSpace(value.Snippet),
		})
	}
	return unique
}

func dedupeInputGTMapping(values []core.InputGTProposedMapping) []core.InputGTProposedMapping {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	unique := make([]core.InputGTProposedMapping, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.Join([]string{
			value.EncoderType,
			value.Name,
			value.SourceCandidate,
			value.Condition,
		}, "|"))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
