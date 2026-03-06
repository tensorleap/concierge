package inspect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

const runtimeSignatureProbeTimeout = 20 * time.Second

var runtimeSignatureProbeRunner = runRuntimeSignatureProbe

func detectRuntimeModelInputs(repoRoot string, contracts *core.IntegrationContracts) ([]string, []string) {
	notes := make([]string, 0, 4)
	if contracts == nil {
		return nil, append(notes, "runtime_signature:skip:no_contracts")
	}

	modelPath, modelType := runtimeSignatureModelPath(repoRoot, contracts)
	if strings.TrimSpace(modelPath) == "" || strings.TrimSpace(modelType) == "" {
		return nil, append(notes, "runtime_signature:skip:no_supported_model_candidate")
	}

	inputs, err := runtimeSignatureProbeRunner(modelPath, modelType)
	if err != nil {
		return nil, append(notes, fmt.Sprintf("runtime_signature:error:%s", strings.TrimSpace(err.Error())))
	}
	if len(inputs) == 0 {
		return nil, append(notes, fmt.Sprintf("runtime_signature:empty:%s:%s", modelType, filepath.ToSlash(modelPath)))
	}

	normalized := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if symbol := canonicalDiscoveredSymbol(input); symbol != "" {
			normalized = append(normalized, symbol)
		}
	}
	normalized = uniqueSortedContractSymbols(normalized)
	notes = append(notes, fmt.Sprintf("runtime_signature:ok:%s:%s", modelType, filepath.ToSlash(modelPath)))
	return normalized, notes
}

func runtimeSignatureModelPath(repoRoot string, contracts *core.IntegrationContracts) (string, string) {
	if contracts == nil {
		return "", ""
	}
	if path, modelType, ok := resolveRuntimeSignatureModelPath(repoRoot, contracts.ResolvedModelPath); ok {
		return path, modelType
	}

	for _, candidate := range contracts.ModelCandidates {
		if path, modelType, ok := resolveRuntimeSignatureModelPath(repoRoot, candidate.Path); ok {
			return path, modelType
		}
	}

	return "", ""
}

func resolveRuntimeSignatureModelPath(repoRoot string, modelPath string) (string, string, bool) {
	normalized := strings.TrimSpace(modelPath)
	if normalized == "" {
		return "", "", false
	}

	absPath := filepath.FromSlash(normalized)
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(repoRoot, absPath)
	}
	absPath = filepath.Clean(absPath)
	if !isPathWithinRepo(repoRoot, absPath) {
		return "", "", false
	}
	if _, err := os.Stat(absPath); err != nil {
		return "", "", false
	}

	switch strings.ToLower(filepath.Ext(absPath)) {
	case ".onnx":
		return absPath, "onnx", true
	case ".h5", ".keras":
		return absPath, "keras", true
	default:
		return "", "", false
	}
}

func runRuntimeSignatureProbe(modelPath string, modelType string) ([]string, error) {
	probeCode := runtimeProbeScript(modelType)
	if strings.TrimSpace(probeCode) == "" {
		return nil, fmt.Errorf("unsupported runtime signature model type %q", modelType)
	}

	ctx, cancel := context.WithTimeout(context.Background(), runtimeSignatureProbeTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, "python3", "-c", probeCode, modelPath)
	output, err := command.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("python probe timed out")
		}
		return nil, fmt.Errorf("python probe failed: %s", strings.TrimSpace(string(output)))
	}

	var payload struct {
		Inputs []string `json:"inputs"`
		Error  string   `json:"error"`
	}
	if unmarshalErr := json.Unmarshal(output, &payload); unmarshalErr != nil {
		return nil, fmt.Errorf("python probe returned malformed payload: %s", strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(payload.Error) != "" {
		return nil, fmt.Errorf("python probe error: %s", strings.TrimSpace(payload.Error))
	}

	return uniqueSortedContractSymbols(payload.Inputs), nil
}

func runtimeProbeScript(modelType string) string {
	switch strings.ToLower(strings.TrimSpace(modelType)) {
	case "onnx":
		return `
import json
import sys

result = {"inputs": []}
try:
    import onnx
    model = onnx.load(sys.argv[1])
    result["inputs"] = [value.name for value in model.graph.input if getattr(value, "name", "")]
except Exception as exc:
    result["error"] = str(exc)

print(json.dumps(result))
`
	case "keras":
		return `
import json
import sys

result = {"inputs": []}
try:
    from tensorflow import keras
    model = keras.models.load_model(sys.argv[1], compile=False)
    tensors = getattr(model, "inputs", None) or []
    names = []
    for tensor in tensors:
        name = getattr(tensor, "name", "")
        if isinstance(name, str) and name:
            names.append(name.split(":")[0])
    result["inputs"] = names
except Exception as exc:
    result["error"] = str(exc)

print(json.dumps(result))
`
	default:
		return ""
	}
}
