package inspect

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestDetectRuntimeModelInputsUsesResolvedModelPath(t *testing.T) {
	repoRoot := t.TempDir()
	modelPath := filepath.Join(repoRoot, "model", "demo.onnx")
	writeFixtureFile(t, repoRoot, "model/demo.onnx", "binary")

	previousRunner := runtimeSignatureProbeRunner
	runtimeSignatureProbeRunner = func(path string, modelType string) ([]string, error) {
		if path != modelPath {
			t.Fatalf("expected probe path %q, got %q", modelPath, path)
		}
		if modelType != "onnx" {
			t.Fatalf("expected model type %q, got %q", "onnx", modelType)
		}
		return []string{"input_ids", "attention_mask"}, nil
	}
	t.Cleanup(func() {
		runtimeSignatureProbeRunner = previousRunner
	})

	inputs, notes := detectRuntimeModelInputs(repoRoot, &core.IntegrationContracts{
		ResolvedModelPath: "model/demo.onnx",
	})
	if !reflect.DeepEqual(inputs, []string{"attention_masks", "input_ids"}) {
		t.Fatalf("expected canonicalized runtime inputs, got %+v", inputs)
	}
	if len(notes) == 0 || !strings.HasPrefix(notes[0], "runtime_signature:ok:") {
		t.Fatalf("expected runtime success note, got %+v", notes)
	}
}

func TestDetectRuntimeModelInputsSkipsUnsupportedOrMissingModel(t *testing.T) {
	repoRoot := t.TempDir()
	inputs, notes := detectRuntimeModelInputs(repoRoot, &core.IntegrationContracts{
		ModelCandidates: []core.ModelCandidate{
			{Path: "model/demo.pt"},
		},
	})
	if len(inputs) != 0 {
		t.Fatalf("expected no runtime inputs, got %+v", inputs)
	}
	if len(notes) == 0 || notes[0] != "runtime_signature:skip:no_supported_model_candidate" {
		t.Fatalf("expected skip note, got %+v", notes)
	}
}

func TestBuildInputGTComparisonReportIncludesRuntimeDifferences(t *testing.T) {
	findings := core.InputGTNormalizedFindings{
		Inputs: []core.InputGTCandidate{
			{Name: "image"},
			{Name: "token_type_ids"},
		},
		GroundTruths: []core.InputGTCandidate{
			{Name: "classes"},
		},
	}

	report := buildInputGTComparisonReport(findings, []string{"image", "attention_masks"}, []string{"runtime_signature:ok:onnx:model/demo.onnx"})
	if !reflect.DeepEqual(report.RuntimeInputSymbols, []string{"attention_masks", "image"}) {
		t.Fatalf("unexpected runtime symbols: %+v", report.RuntimeInputSymbols)
	}
	if !reflect.DeepEqual(report.RuntimeOnlyInputSymbols, []string{"attention_masks"}) {
		t.Fatalf("unexpected runtime-only symbols: %+v", report.RuntimeOnlyInputSymbols)
	}
	if !reflect.DeepEqual(report.DiscoveryOnlyInputSymbols, []string{"token_type_ids"}) {
		t.Fatalf("unexpected discovery-only symbols: %+v", report.DiscoveryOnlyInputSymbols)
	}
	if len(report.Notes) == 0 || report.Notes[0] != "runtime_signature:ok:onnx:model/demo.onnx" {
		t.Fatalf("expected runtime note, got %+v", report.Notes)
	}
}
