package inspect

import (
	"encoding/json"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestNormalizeInputGTFindingsPayloadAcceptsSchemaVariants(t *testing.T) {
	payload := map[string]any{
		"schemaVersion": "1.0.0",
		"methodVersion": "test",
		"model_inputs": []any{
			map[string]any{"name": "input_ids"},
			map[string]any{"name": "attention_mask"},
		},
		"targets": map[string]any{
			"classes": map[string]any{"name": "classes"},
		},
		"proposed_encoder_mapping": map[string]any{
			"input_encoder": map[string]any{
				"name":             "input_ids",
				"source_candidate": "input_ids",
			},
			"ground_truth_encoder": map[string]any{
				"name":             "classes",
				"source_candidate": "classes",
			},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	findings, err := normalizeInputGTFindingsPayload(raw)
	if err != nil {
		t.Fatalf("normalizeInputGTFindingsPayload returned error: %v", err)
	}

	if len(findings.Inputs) != 2 {
		t.Fatalf("expected 2 input candidates, got %+v", findings.Inputs)
	}
	if findings.Inputs[0].Name != "attention_masks" || findings.Inputs[1].Name != "input_ids" {
		t.Fatalf("expected canonicalized/sorted input candidates, got %+v", findings.Inputs)
	}
	if len(findings.GroundTruths) != 1 || findings.GroundTruths[0].Name != "classes" {
		t.Fatalf("expected classes target candidate, got %+v", findings.GroundTruths)
	}
	if len(findings.ProposedMapping) != 2 {
		t.Fatalf("expected 2 mapping entries, got %+v", findings.ProposedMapping)
	}
}

func TestNormalizeInputGTFindingsPayloadAllowsEmptyCandidates(t *testing.T) {
	payload := map[string]any{
		"schema_version":          "1.0.0",
		"method_version":          "test-empty",
		"candidate_inputs":        nil,
		"candidate_ground_truths": nil,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	findings, err := normalizeInputGTFindingsPayload(raw)
	if err != nil {
		t.Fatalf("normalizeInputGTFindingsPayload returned error: %v", err)
	}
	if len(findings.Inputs) != 0 || len(findings.GroundTruths) != 0 {
		t.Fatalf("expected empty candidates, got inputs=%+v groundTruths=%+v", findings.Inputs, findings.GroundTruths)
	}
	if len(findings.Unknowns) == 0 {
		t.Fatalf("expected informational unknown notice for empty candidates, got %+v", findings.Unknowns)
	}
}

func TestPostProcessInputCandidatesDerivesTokenizerKeys(t *testing.T) {
	inputs := []inputCandidateForTest{
		{
			Name:    "input_tokens",
			Snippet: "encoded = tokenizer(text)['input_ids']; mask = tokenizer(text)['attention_mask']; typ = tokenizer(text)['token_type_ids']",
		},
	}

	processed := postProcessInputCandidates(buildInputCandidates(inputs), "pytorch")
	names := extractCandidateNames(processed)
	expected := []string{"attention_masks", "input_ids", "token_type_ids", "input_tokens"}
	assertEqualStringSlice(t, names, expected)

	inputTokens := findCandidateByName(processed, "input_tokens")
	if inputTokens == nil || inputTokens.Condition == "" || inputTokens.Confidence != "low" {
		t.Fatalf("expected input_tokens to be retained as conditional low-confidence alternate, got %+v", inputTokens)
	}
}

type inputCandidateForTest struct {
	Name    string
	Snippet string
}

func buildInputCandidates(values []inputCandidateForTest) []core.InputGTCandidate {
	candidates := make([]core.InputGTCandidate, 0, len(values))
	for _, value := range values {
		candidates = append(candidates, core.InputGTCandidate{
			Name:       value.Name,
			Confidence: "medium",
			Evidence: []core.InputGTEvidence{
				{File: "train.py", Line: 1, Snippet: value.Snippet},
			},
		})
	}
	return candidates
}

func extractCandidateNames(values []core.InputGTCandidate) []string {
	names := make([]string, 0, len(values))
	for _, value := range values {
		names = append(names, value.Name)
	}
	return names
}

func findCandidateByName(values []core.InputGTCandidate, name string) *core.InputGTCandidate {
	for i := range values {
		if values[i].Name == name {
			return &values[i]
		}
	}
	return nil
}

func assertEqualStringSlice(t *testing.T, got, expected []string) {
	t.Helper()
	if len(got) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("expected %v, got %v", expected, got)
		}
	}
}
