package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tensorleap/concierge/internal/core"
)

const leapLoaderCheckDatasetScript = `
import json
import sys

from code_loader.leaploader import LeapLoader


def _as_int_list(values):
    if values is None:
        return None
    return [int(value) for value in values]


def _shape_entry(item, include_channel_dim=False):
    entry = {"name": str(item.name)}
    shape = getattr(item, "shape", None)
    if shape is not None:
        entry["shape"] = _as_int_list(shape)
    if include_channel_dim:
        channel_dim = getattr(item, "channel_dim", None)
        if channel_dim is not None:
            entry["channelDim"] = int(channel_dim)
    return entry


def _named_type_entry(item):
    entry = {"name": str(item.name)}
    item_type = getattr(item, "type", None)
    if item_type is not None:
        entry["type"] = getattr(item_type, "name", str(item_type))
    return entry


def _named_args_entry(item):
    entry = {"name": str(item.name)}
    arg_names = getattr(item, "arg_names", None)
    if arg_names:
        entry["argNames"] = [str(value) for value in arg_names]
    item_type = getattr(item, "type", None)
    if item_type is not None:
        entry["type"] = getattr(item_type, "name", str(item_type))
    return entry


def _prediction_type_entry(item):
    entry = {"name": str(item.name)}
    if getattr(item, "labels", None):
        entry["labels"] = [str(value) for value in item.labels]
    channel_dim = getattr(item, "channel_dim", None)
    if channel_dim is not None:
        entry["channelDim"] = int(channel_dim)
    return entry


def _custom_layer_entry(item):
    entry = {"name": str(item.name)}
    if getattr(item, "init_arg_names", None):
        entry["initArgNames"] = [str(value) for value in item.init_arg_names]
    if getattr(item, "call_arg_names", None):
        entry["callArgNames"] = [str(value) for value in item.call_arg_names]
    if getattr(item, "use_custom_latent_space", False):
        entry["useCustomLatentSpace"] = True
    return entry


def _payload_entry(item):
    entry = {
        "name": str(item.name),
        "passed": bool(item.is_passed),
    }
    if getattr(item, "handler_type", None) is not None:
        entry["handlerType"] = str(item.handler_type)
    if getattr(item, "shape", None) is not None:
        entry["shape"] = _as_int_list(item.shape)
    if getattr(item, "display", None):
        entry["display"] = {str(key): str(value) for key, value in item.display.items()}
    return entry


repo_root = sys.argv[1]
entry_name = sys.argv[2]
loader = LeapLoader(repo_root, entry_name)
result = loader.check_dataset()

payload = {
    "available": True,
    "isValid": bool(result.is_valid),
    "isValidForModel": bool(result.is_valid_for_model),
    "generalError": result.general_error or "",
    "printLog": result.print_log or "",
    "payloads": [_payload_entry(item) for item in (result.payloads or [])],
}

if result.setup is not None:
    payload["setup"] = {
        "preprocess": {
            "trainingLength": int(result.setup.preprocess.training_length),
            "validationLength": int(result.setup.preprocess.validation_length),
            "testLength": int(result.setup.preprocess.test_length or 0),
            "unlabeledLength": int(result.setup.preprocess.unlabeled_length or 0),
            "additionalLength": int(result.setup.preprocess.additional_length or 0),
        },
        "inputs": [_shape_entry(item, include_channel_dim=True) for item in (result.setup.inputs or [])],
        "metadata": [_named_type_entry(item) for item in (result.setup.metadata or [])],
        "outputs": [_shape_entry(item) for item in (result.setup.outputs or [])],
        "visualizers": [_named_args_entry(item) for item in (result.setup.visualizers or [])],
        "predictionTypes": [_prediction_type_entry(item) for item in (result.setup.prediction_types or [])],
        "customLosses": [_named_args_entry(item) for item in (result.setup.custom_losses or [])],
        "metrics": [_named_args_entry(item) for item in (result.setup.metrics or [])],
    }

if result.model_setup is not None:
    payload["modelSetup"] = {
        "customLayers": [_custom_layer_entry(item) for item in (result.model_setup.custom_layers or [])],
    }

if result.engine_file_contract is not None:
    engine = {}
    if getattr(result.engine_file_contract, "node_connections", None) is not None:
        engine["nodeConnectionCount"] = len(result.engine_file_contract.node_connections)
    config = getattr(result.engine_file_contract, "leap_analysis_configuration", None)
    if config is not None:
        if getattr(config, "feature_flags", None):
            engine["featureFlags"] = [str(value) for value in config.feature_flags]
        if getattr(config, "domain_gap_metadata", None):
            engine["domainGapMetadata"] = [str(value) for value in config.domain_gap_metadata]
    payload["engineFileContract"] = engine

print(json.dumps(payload, sort_keys=True))
`

var (
	guideCrashPattern        = regexp.MustCompile(`Script crashed before completing all steps\. crashed at function '([^']+)'`)
	guideStatusRowPattern    = regexp.MustCompile(`^(tensorleap_[^|]+?)\s+\|\s+([✅❌❔])\s*$`)
	sharedLibraryNamePattern = regexp.MustCompile(`([A-Za-z0-9._+-]+\.(?:so(?:\.[0-9]+)*|dylib|dll))`)
)

type guideRuntimeRunner interface {
	RunPython(ctx context.Context, snapshot core.WorkspaceSnapshot, args ...string) (PythonRuntimeCommandResult, error)
}

type integrationTestASTInvoker interface {
	Analyze(ctx context.Context, snapshot core.WorkspaceSnapshot) (IntegrationTestASTResult, error)
}

type guideHandlerKind string

const (
	guideHandlerInput    guideHandlerKind = "input"
	guideHandlerGT       guideHandlerKind = "ground_truth"
	guideHandlerMetadata guideHandlerKind = "metadata"
)

// GuideValidator runs the guide-native local validator and parser through the resolved Poetry runtime.
type GuideValidator struct {
	runtimeRunner guideRuntimeRunner
	astAnalyzer   integrationTestASTInvoker
}

// GuideValidationResult captures guide-native issues, evidence, and summary data.
type GuideValidationResult struct {
	Issues   []core.Issue
	Evidence []core.EvidenceItem
	Summary  core.GuideValidationSummary
}

// NewGuideValidator creates a guide validator backed by the shared Poetry runtime runner.
func NewGuideValidator() *GuideValidator {
	return &GuideValidator{
		runtimeRunner: NewPythonRuntimeRunner(),
		astAnalyzer:   NewIntegrationTestASTAnalyzer(),
	}
}

// Run executes guide-native validation without mutating repository state.
func (v *GuideValidator) Run(ctx context.Context, snapshot core.WorkspaceSnapshot) (GuideValidationResult, error) {
	if v == nil {
		v = NewGuideValidator()
	}
	if v.runtimeRunner == nil {
		v.runtimeRunner = NewPythonRuntimeRunner()
	}
	if v.astAnalyzer == nil {
		v.astAnalyzer = NewIntegrationTestASTAnalyzer()
	}

	summary := core.GuideValidationSummary{}
	evidence := make([]core.EvidenceItem, 0, 8)
	if snapshot.RuntimeProfile != nil {
		summary.CodeLoaderVersion = strings.TrimSpace(snapshot.RuntimeProfile.CodeLoader.Version)
		summary.LocalStatusTableSupported = snapshot.RuntimeProfile.CodeLoader.SupportsGuideLocalStatusTable
	}

	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	skipReason, entryName, err := guideValidationSkipReason(snapshot)
	if err != nil {
		return GuideValidationResult{}, err
	}
	if skipReason != "" {
		summary.Skipped = true
		summary.SkipReason = skipReason
		evidence = append(evidence, marshalGuideEvidence(core.GuideEvidenceSummary, summary))
		return GuideValidationResult{Evidence: evidence, Summary: summary}, nil
	}

	localSummary, localEvidence, localOutput := v.runLocalGuideValidation(ctx, snapshot, entryName)
	summary.Local = localSummary
	evidence = append(evidence, localEvidence...)

	parserSummary, parserEvidence, parserOutput, err := v.runGuideParser(ctx, snapshot, repoRoot, entryName)
	if err != nil {
		return GuideValidationResult{}, err
	}
	summary.Parser = parserSummary
	evidence = append(evidence, parserEvidence...)

	handlerKinds, kindErr := discoverGuideHandlerKinds(filepath.Join(repoRoot, entryName))
	if kindErr != nil {
		return GuideValidationResult{}, core.WrapError(core.KindUnknown, "validate.guide.handler_kinds", kindErr)
	}
	applyGuideHandlerKinds(&summary.Parser, handlerKinds)

	evidence = append(evidence, marshalGuideEvidence(core.GuideEvidenceLocalRaw, localSummary))
	evidence = append(evidence, marshalGuideEvidence(core.GuideEvidenceParserRaw, summary.Parser))

	astResult, err := v.astAnalyzer.Analyze(ctx, snapshot)
	if err != nil {
		return GuideValidationResult{}, err
	}
	evidence = append(evidence, astResult.Evidence...)

	issues := dedupeIssues(collectGuideIssues(summary, localOutput, parserOutput, handlerKinds, astResult.Issues))
	summary.Recommendation = deriveGuideRecommendation(summary, issues)
	evidence = append(evidence, marshalGuideEvidence(core.GuideEvidenceSummary, summary))
	return GuideValidationResult{
		Issues:   issues,
		Evidence: evidence,
		Summary:  summary,
	}, nil
}

func (v *GuideValidator) runLocalGuideValidation(
	ctx context.Context,
	snapshot core.WorkspaceSnapshot,
	entryName string,
) (core.GuideLocalRunSummary, []core.EvidenceItem, PythonRuntimeCommandResult) {
	result, err := v.runtimeRunner.RunPython(ctx, snapshot, entryName)
	summary := parseGuideLocalRunSummary(result.Stdout, result.Stderr)
	summary.Attempted = true

	if err != nil && !summary.MappingFailure && strings.TrimSpace(summary.CrashFunction) == "" {
		if strings.Contains(strings.ToLower(result.Stderr), "traceback") {
			summary.CrashFunction = "import_or_runtime"
		}
	}

	evidence := []core.EvidenceItem{
		{Name: core.GuideEvidenceLocalCmd, Value: result.Command},
		{Name: core.GuideEvidenceLocalStdout, Value: result.Stdout},
		{Name: core.GuideEvidenceLocalStderr, Value: result.Stderr},
	}
	return summary, evidence, result
}

func (v *GuideValidator) runGuideParser(
	ctx context.Context,
	snapshot core.WorkspaceSnapshot,
	repoRoot string,
	entryName string,
) (core.GuideParserRunSummary, []core.EvidenceItem, PythonRuntimeCommandResult, error) {
	result, runErr := v.runtimeRunner.RunPython(ctx, snapshot, "-c", leapLoaderCheckDatasetScript, repoRoot, entryName)
	summary := core.GuideParserRunSummary{Attempted: true}

	evidence := []core.EvidenceItem{
		{Name: core.GuideEvidenceParserCmd, Value: result.Command},
		{Name: core.GuideEvidenceParserStdout, Value: result.Stdout},
		{Name: core.GuideEvidenceParserStderr, Value: result.Stderr},
	}

	if runErr != nil {
		if reason, ok := classifyGuideParserUnavailable(result, runErr); ok {
			summary.Available = false
			summary.UnavailableReason = reason
			return summary, evidence, result, nil
		}
		return core.GuideParserRunSummary{}, nil, PythonRuntimeCommandResult{}, core.WrapError(core.KindUnknown, "validate.guide.parser_run", runErr)
	}

	if err := json.Unmarshal([]byte(result.Stdout), &summary); err != nil {
		return core.GuideParserRunSummary{}, nil, PythonRuntimeCommandResult{}, core.WrapError(core.KindUnknown, "validate.guide.parser_unmarshal", err)
	}
	summary.Attempted = true
	summary.Available = true
	for index := range summary.Payloads {
		summary.Payloads[index].Error = firstGuidePayloadError(summary.Payloads[index])
	}

	return summary, evidence, result, nil
}

func guideValidationSkipReason(snapshot core.WorkspaceSnapshot) (string, string, error) {
	if snapshot.RuntimeProfile == nil || strings.TrimSpace(snapshot.RuntimeProfile.InterpreterPath) == "" {
		return "Poetry runtime profile is missing", "", nil
	}

	interpreterPath := strings.TrimSpace(snapshot.RuntimeProfile.InterpreterPath)
	if _, err := os.Stat(interpreterPath); err != nil {
		if os.IsNotExist(err) {
			return "stored Poetry interpreter does not exist", "", nil
		}
		return "", "", core.WrapError(core.KindUnknown, "validate.guide.interpreter_stat", err)
	}

	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return "repository root is missing from the snapshot", "", nil
	}

	leapYAMLPath := filepath.Join(repoRoot, "leap.yaml")
	raw, err := os.ReadFile(leapYAMLPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "leap.yaml is missing", "", nil
		}
		return "", "", core.WrapError(core.KindUnknown, "validate.guide.leap_yaml_read", err)
	}

	var contract struct {
		EntryFile string `yaml:"entryFile"`
	}
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		return "leap.yaml is not parseable", "", nil
	}

	entryName := filepath.ToSlash(filepath.Clean(strings.TrimSpace(contract.EntryFile)))
	entryName = strings.TrimPrefix(entryName, "./")
	if entryName != core.CanonicalIntegrationEntryFile {
		return fmt.Sprintf("leap.yaml entryFile is not %s", core.CanonicalIntegrationEntryFile), "", nil
	}

	if _, err := os.Stat(filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("%s is missing", core.CanonicalIntegrationEntryFile), "", nil
		}
		return "", "", core.WrapError(core.KindUnknown, "validate.guide.entry_stat", err)
	}

	return "", entryName, nil
}

func parseGuideLocalRunSummary(stdout string, stderr string) core.GuideLocalRunSummary {
	summary := core.GuideLocalRunSummary{}
	lines := strings.Split(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n")

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		if strings.Contains(line, "Successful!") {
			summary.Successful = true
			continue
		}
		if strings.Contains(line, "Tensorleap_integration_test code flow failed") {
			summary.MappingFailure = true
			continue
		}
		if matches := guideCrashPattern.FindStringSubmatch(line); len(matches) == 2 {
			summary.CrashFunction = strings.TrimSpace(matches[1])
			continue
		}
		if row, ok := parseGuideStatusRow(line); ok {
			summary.StatusRows = append(summary.StatusRows, row)
			continue
		}
		if warning, ok := parseGuideDefaultWarning(line); ok {
			summary.DefaultWarnings = append(summary.DefaultWarnings, warning)
			continue
		}
		if strings.HasPrefix(line, "Some mandatory components have not yet been added to the Integration test. Recommended next interface to add is:") {
			summary.RecommendedInterface = strings.TrimSpace(strings.TrimPrefix(line, "Some mandatory components have not yet been added to the Integration test. Recommended next interface to add is:"))
			continue
		}
		if strings.HasPrefix(line, "All mandatory parts have been successfully set.") {
			summary.MandatoryReady = true
			if index := strings.LastIndex(line, "adding:"); index >= 0 {
				summary.RecommendedInterface = strings.TrimSpace(line[index+len("adding:"):])
			}
			continue
		}
		if strings.HasPrefix(line, "All parts have been successfully set.") {
			summary.AllPartsReady = true
			summary.MandatoryReady = true
			continue
		}
	}

	if !summary.MappingFailure && strings.Contains(strings.ToLower(stderr), "tensorleap_integration_test code flow failed") {
		summary.MappingFailure = true
	}

	sort.SliceStable(summary.StatusRows, func(i, j int) bool {
		return guideStatusSortOrder(summary.StatusRows[i].Name) < guideStatusSortOrder(summary.StatusRows[j].Name)
	})
	return summary
}

func parseGuideStatusRow(line string) (core.GuideStatusRow, bool) {
	matches := guideStatusRowPattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(matches) != 3 {
		return core.GuideStatusRow{}, false
	}

	status := "unknown"
	switch matches[2] {
	case "✅":
		status = "pass"
	case "❌":
		status = "fail"
	case "❔":
		status = "unknown"
	}

	return core.GuideStatusRow{
		Name:   strings.TrimSpace(matches[1]),
		Status: status,
	}, true
}

func parseGuideDefaultWarning(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, "Parameter '") || !strings.Contains(trimmed, "defaults to") {
		return "", false
	}
	return trimmed, true
}

func guideStatusSortOrder(name string) int {
	switch strings.TrimSpace(name) {
	case "tensorleap_preprocess":
		return 0
	case "tensorleap_input_encoder":
		return 1
	case "tensorleap_load_model":
		return 2
	case "tensorleap_integration_test":
		return 3
	case "tensorleap_gt_encoder":
		return 4
	default:
		return 100
	}
}

func classifyGuideParserUnavailable(result PythonRuntimeCommandResult, err error) (string, bool) {
	combined := strings.ToLower(strings.TrimSpace(result.Stdout + "\n" + result.Stderr + "\n" + err.Error()))
	switch {
	case strings.Contains(combined, "no module named 'code_loader'"),
		strings.Contains(combined, "modulenotfounderror"),
		strings.Contains(combined, "cannot import name 'leaploader'"),
		strings.Contains(combined, "cannot import name \"leaploader\""):
		return "code-loader parser is unavailable in the resolved Poetry environment", true
	default:
		return "", false
	}
}

func firstGuidePayloadError(payload core.GuidePayloadSummary) string {
	if payload.Passed {
		return ""
	}
	if len(payload.Display) == 0 {
		return ""
	}

	keys := make([]string, 0, len(payload.Display))
	for key := range payload.Display {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := strings.TrimSpace(payload.Display[key])
		if value == "" {
			continue
		}
		return value
	}
	return ""
}

func deriveGuideRecommendation(summary core.GuideValidationSummary, issues []core.Issue) core.GuideRecommendation {
	if summary.Skipped {
		return core.GuideRecommendation{}
	}
	if library := missingNativeSystemLibrary(summary.Parser.GeneralError); library != "" {
		return core.GuideRecommendation{
			Stage:   "runtime_native_dependency",
			Message: runtimeNativeDependencyRecommendationMessage(library),
		}
	}
	if isNativeSystemDependencyError(summary.Parser.GeneralError) {
		return core.GuideRecommendation{
			Stage:   "runtime_native_dependency",
			Message: runtimeNativeDependencyRecommendationMessage(""),
		}
	}

	hasLocalStatusTable := len(summary.Local.StatusRows) > 0 || summary.LocalStatusTableSupported

	primary, ok := selectPrimaryBlockingGuideStep(issues)
	if ok {
		switch primary.ID {
		case core.EnsureStepIntegrationScript:
			return core.GuideRecommendation{
				Stage:   "integration_script",
				Message: "Next prerequisite: make `leap_integration.py` import cleanly from the repository root, then rerun `concierge run`.",
			}
		case core.EnsureStepPreprocessContract:
			return core.GuideRecommendation{
				Stage:   "preprocess",
				Message: "Next recommended interface: make preprocess run directly and return training and validation subsets.",
			}
		case core.EnsureStepInputEncoders:
			if hasLocalStatusTable && guideStatus(summary.Local, "tensorleap_load_model") != "pass" {
				return core.GuideRecommendation{
					Stage:   "minimum_inputs",
					Message: "Next recommended interface: add the minimum required input encoders so a real sample can reach the model.",
				}
			}
			return core.GuideRecommendation{
				Stage:   "remaining_inputs",
				Message: "Next recommended interface: add the remaining required input encoders and rerun the thin integration test.",
			}
		case core.EnsureStepModelAcquisition:
			return core.GuideRecommendation{
				Stage:   "model_artifact",
				Message: "Next recommended milestone: materialize one supported `.onnx` or `.h5` model artifact locally so `@tensorleap_load_model` can be wired.",
			}
		case core.EnsureStepModelContract:
			return core.GuideRecommendation{
				Stage:   "load_model",
				Message: "Next recommended interface: add @tensorleap_load_model after the minimum input path runs locally.",
			}
		case core.EnsureStepIntegrationTestContract, core.EnsureStepIntegrationTestWiring:
			return core.GuideRecommendation{
				Stage:   "thin_integration_test",
				Message: "Next recommended interface: add or repair a thin @tensorleap_integration_test that only calls Tensorleap decorators.",
			}
		case core.EnsureStepGroundTruthEncoders:
			return core.GuideRecommendation{
				Stage:   "ground_truth",
				Message: "Next recommended interface: add the required GT encoders and rerun the integration test.",
			}
		}
	}

	if summary.Local.Successful || summary.Parser.IsValid {
		return core.GuideRecommendation{
			Stage:   "wider_sample_coverage",
			Message: "Next recommended milestone: expand from first-sample success to a few training and validation samples.",
		}
	}

	return core.GuideRecommendation{}
}

func applyGuideHandlerKinds(summary *core.GuideParserRunSummary, handlerKinds map[string]guideHandlerKind) {
	if summary == nil || len(summary.Payloads) == 0 || len(handlerKinds) == 0 {
		return
	}
	for index := range summary.Payloads {
		if strings.TrimSpace(summary.Payloads[index].HandlerType) != "" {
			continue
		}
		if kind, ok := handlerKinds[normalizeGuideHandlerName(summary.Payloads[index].Name)]; ok {
			summary.Payloads[index].HandlerType = string(kind)
		}
	}
}

func guideStatus(summary core.GuideLocalRunSummary, name string) string {
	for _, row := range summary.StatusRows {
		if strings.EqualFold(strings.TrimSpace(row.Name), strings.TrimSpace(name)) {
			return row.Status
		}
	}
	return "unknown"
}

func payloadFailed(summary core.GuideParserRunSummary, payloadName string) bool {
	for _, payload := range summary.Payloads {
		if strings.EqualFold(strings.TrimSpace(payload.Name), strings.TrimSpace(payloadName)) && !payload.Passed {
			return true
		}
	}
	return false
}

func payloadFailedByKinds(summary core.GuideParserRunSummary, kinds ...guideHandlerKind) bool {
	allowed := make(map[string]struct{}, len(kinds))
	for _, kind := range kinds {
		allowed[string(kind)] = struct{}{}
	}
	for _, payload := range summary.Payloads {
		if payload.Passed {
			continue
		}
		if _, ok := allowed[strings.TrimSpace(payload.HandlerType)]; ok {
			return true
		}
	}
	return false
}

func parserHasGeneralFailure(summary core.GuideParserRunSummary) bool {
	return strings.TrimSpace(summary.GeneralError) != ""
}

func discoverGuideHandlerKinds(entryPath string) (map[string]guideHandlerKind, error) {
	raw, err := os.ReadFile(entryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(string(raw), "\n")
	pendingDecorators := make([]guideDecoratorInvocation, 0, 4)
	kinds := make(map[string]guideHandlerKind)

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "@") {
			invocation, ok := extractGuideDecoratorInvocation(line)
			if !ok {
				pendingDecorators = pendingDecorators[:0]
				continue
			}
			pendingDecorators = append(pendingDecorators, invocation)
			continue
		}
		if strings.HasPrefix(line, "def ") {
			functionName, ok := extractGuideFunctionName(line)
			if !ok {
				pendingDecorators = pendingDecorators[:0]
				continue
			}
			for _, invocation := range pendingDecorators {
				switch invocation.Name {
				case "tensorleap_input_encoder":
					kinds[extractGuideDecoratorSymbol(invocation.Arguments, functionName)] = guideHandlerInput
				case "tensorleap_gt_encoder":
					kinds[extractGuideDecoratorSymbol(invocation.Arguments, functionName)] = guideHandlerGT
				case "tensorleap_metadata":
					kinds[extractGuideDecoratorSymbol(invocation.Arguments, functionName)] = guideHandlerMetadata
				}
			}
			pendingDecorators = pendingDecorators[:0]
			continue
		}
		pendingDecorators = pendingDecorators[:0]
	}

	delete(kinds, "")
	return kinds, nil
}

type guideDecoratorInvocation struct {
	Name      string
	Arguments string
}

var (
	guideDecoratorPattern        = regexp.MustCompile(`^\s*@\s*([A-Za-z_][A-Za-z0-9_\.]*)\s*(?:\((.*)\))?\s*$`)
	guideFunctionPattern         = regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	guideDecoratorKeywordPattern = regexp.MustCompile(`(?i)\b(?:input|feature|target|name)\s*=\s*['"]([^'"]+)['"]`)
	guideQuotedStringPattern     = regexp.MustCompile(`['"]([^'"]+)['"]`)
)

func extractGuideDecoratorInvocation(line string) (guideDecoratorInvocation, bool) {
	matches := guideDecoratorPattern.FindStringSubmatch(line)
	if len(matches) != 3 {
		return guideDecoratorInvocation{}, false
	}
	return guideDecoratorInvocation{
		Name:      strings.ToLower(strings.TrimSpace(matches[1])),
		Arguments: strings.TrimSpace(matches[2]),
	}, true
}

func extractGuideFunctionName(line string) (string, bool) {
	matches := guideFunctionPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	return strings.TrimSpace(matches[1]), true
}

func extractGuideDecoratorSymbol(arguments string, functionName string) string {
	args := strings.TrimSpace(arguments)
	if args == "" {
		return inferGuideHandlerName(functionName)
	}
	if matches := guideDecoratorKeywordPattern.FindStringSubmatch(args); len(matches) == 2 {
		return normalizeGuideHandlerName(matches[1])
	}
	if matches := guideQuotedStringPattern.FindStringSubmatch(args); len(matches) == 2 {
		return normalizeGuideHandlerName(matches[1])
	}
	return inferGuideHandlerName(functionName)
}

func inferGuideHandlerName(functionName string) string {
	name := strings.TrimSpace(functionName)
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	for _, prefix := range []string{"encode_", "input_", "gt_", "label_", "target_", "metadata_"} {
		if strings.HasPrefix(lower, prefix) {
			return normalizeGuideHandlerName(name[len(prefix):])
		}
	}
	return normalizeGuideHandlerName(name)
}

func normalizeGuideHandlerName(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	normalized = strings.ReplaceAll(normalized, " ", "_")
	return normalized
}

func collectGuideIssues(
	summary core.GuideValidationSummary,
	localResult PythonRuntimeCommandResult,
	parserResult PythonRuntimeCommandResult,
	handlerKinds map[string]guideHandlerKind,
	astIssues []core.Issue,
) []core.Issue {
	issues := make([]core.Issue, 0, 8)
	issues = append(issues, astIssues...)

	if summary.Parser.Available {
		payloadIssues := make([]core.Issue, 0, len(summary.Parser.Payloads))
		for _, payload := range summary.Parser.Payloads {
			if payload.Passed {
				continue
			}
			issue, ok := issueFromGuidePayloadFailure(payload, handlerKinds)
			if ok {
				payloadIssues = append(payloadIssues, issue)
			}
		}
		if issue, ok := issueFromGuideParserGeneralError(summary.Parser.GeneralError); ok &&
			!suppressGuideParserGeneralIssue(issue, payloadIssues) {
			issues = append(issues, issue)
		}
		issues = append(issues, payloadIssues...)
	} else {
		if summary.Local.MappingFailure && !hasSpecificIntegrationTestIssue(astIssues) {
			issues = append(issues, core.Issue{
				Code:     core.IssueCodeIntegrationTestExecutionFailed,
				Message:  "the local validator reported that @tensorleap_integration_test failed in mapping mode",
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
			})
		}
		if strings.TrimSpace(summary.Local.CrashFunction) != "" {
			if issue, ok := issueFromGuideCrashFunction(summary.Local.CrashFunction, localResult, parserResult); ok {
				issues = append(issues, issue)
			}
		}
	}

	statusIssues := issuesFromGuideStatusRows(summary.Local, summary.Parser)
	statusIssues = suppressStaleGuideStatusIssues(statusIssues, summary.Local, issues)
	issues = append(issues, statusIssues...)
	return issues
}

func parserClearsPreprocess(parser core.GuideParserRunSummary) bool {
	if !parser.Available {
		return false
	}
	for _, payload := range parser.Payloads {
		if strings.EqualFold(strings.TrimSpace(payload.Name), "preprocess") {
			return payload.Passed
		}
	}
	return parser.IsValid
}

func parserClearsInputEncoder(parser core.GuideParserRunSummary) bool {
	if !parser.Available || !parser.IsValid || parser.Setup == nil {
		return false
	}
	return len(parser.Setup.Inputs) > 0
}

func selectPrimaryBlockingGuideStep(issues []core.Issue) (core.EnsureStep, bool) {
	blocking := make([]core.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity != core.SeverityError {
			continue
		}
		blocking = append(blocking, issue)
	}
	return core.SelectPrimaryEnsureStep(blocking)
}

var guideStatusRowIssueMap = map[string]core.IssueCode{
	"tensorleap_preprocess":       core.IssueCodePreprocessFunctionMissing,
	"tensorleap_input_encoder":    core.IssueCodeInputEncoderMissing,
	"tensorleap_gt_encoder":       core.IssueCodeGTEncoderMissing,
	"tensorleap_load_model":       core.IssueCodeLoadModelDecoratorMissing,
	"tensorleap_integration_test": core.IssueCodeIntegrationTestMissing,
	"tensorleap_custom_loss":      core.IssueCodeIntegrationTestMissingRequiredCalls,
}

var guideStatusRowScopeMap = map[string]core.IssueScope{
	"tensorleap_preprocess":       core.IssueScopePreprocess,
	"tensorleap_input_encoder":    core.IssueScopeInputEncoder,
	"tensorleap_gt_encoder":       core.IssueScopeGroundTruthEncoder,
	"tensorleap_load_model":       core.IssueScopeModel,
	"tensorleap_integration_test": core.IssueScopeIntegrationTest,
	"tensorleap_custom_loss":      core.IssueScopeIntegrationTest,
}

func issuesFromGuideStatusRows(local core.GuideLocalRunSummary, parser core.GuideParserRunSummary) []core.Issue {
	if local.MandatoryReady || len(local.StatusRows) == 0 {
		return nil
	}

	var issues []core.Issue
	preprocessCleared := parserClearsPreprocess(parser)
	inputEncoderCleared := parserClearsInputEncoder(parser)
	for _, row := range local.StatusRows {
		if row.Status != "fail" {
			continue
		}
		name := strings.TrimSpace(row.Name)
		if strings.Contains(strings.ToLower(name), "(optional)") {
			continue
		}
		normalizedName := strings.ToLower(name)
		if normalizedName == "tensorleap_preprocess" && preprocessCleared {
			continue
		}
		if normalizedName == "tensorleap_input_encoder" && inputEncoderCleared {
			// Work around code-loader#273: the local status table can keep the
			// generic input-encoder row at fail even after check_dataset() proves
			// a real input encoder exists and runs successfully.
			continue
		}
		code, ok := guideStatusRowIssueMap[normalizedName]
		if !ok {
			// Unknown/unmapped decorator — use a generic fallback distinct from
			// the "known required call is missing" code.
			code = core.IssueCodeMandatoryDecoratorFailing
		}
		scope := guideStatusRowScopeMap[normalizedName]
		if scope == "" {
			scope = core.IssueScopeValidation
		}
		issues = append(issues, core.Issue{
			Code:     code,
			Message:  fmt.Sprintf("guide status table reports mandatory decorator @%s is not passing", name),
			Severity: core.SeverityError,
			Scope:    scope,
		})
	}
	return issues
}

func suppressGuideParserGeneralIssue(issue core.Issue, payloadIssues []core.Issue) bool {
	if issue.Code != core.IssueCodeIntegrationScriptImportFailed {
		return false
	}
	return len(payloadIssues) > 0
}

func suppressStaleGuideStatusIssues(
	statusIssues []core.Issue,
	local core.GuideLocalRunSummary,
	existing []core.Issue,
) []core.Issue {
	if len(statusIssues) == 0 {
		return nil
	}

	preprocessSuperseded := localStatusShowsDownstreamPreprocessProgress(local)
	integrationTestSuperseded := hasConcreteIntegrationTestIssue(existing)

	filtered := make([]core.Issue, 0, len(statusIssues))
	for _, issue := range statusIssues {
		switch issue.Code {
		case core.IssueCodePreprocessFunctionMissing:
			if preprocessSuperseded {
				continue
			}
		case core.IssueCodeIntegrationTestMissing:
			if integrationTestSuperseded {
				continue
			}
		}
		filtered = append(filtered, issue)
	}
	return filtered
}

func localStatusShowsDownstreamPreprocessProgress(local core.GuideLocalRunSummary) bool {
	return guideStatus(local, "tensorleap_input_encoder") == "pass" ||
		guideStatus(local, "tensorleap_gt_encoder") == "pass"
}

func hasConcreteIntegrationTestIssue(issues []core.Issue) bool {
	for _, issue := range issues {
		if issue.Severity != core.SeverityError || issue.Scope != core.IssueScopeIntegrationTest {
			continue
		}
		if issue.Code == core.IssueCodeIntegrationTestMissing {
			continue
		}
		return true
	}
	return false
}

func hasSpecificIntegrationTestIssue(issues []core.Issue) bool {
	for _, issue := range issues {
		if issue.Severity != core.SeverityError || issue.Scope != core.IssueScopeIntegrationTest {
			continue
		}
		switch issue.Code {
		case core.IssueCodeIntegrationTestMissingRequiredCalls,
			core.IssueCodeIntegrationTestCallsUnknownInterfaces,
			core.IssueCodeIntegrationTestDirectDatasetAccess,
			core.IssueCodeIntegrationTestIllegalBodyLogic,
			core.IssueCodeIntegrationTestManualBatchManipulation:
			return true
		}
	}
	return false
}

func issueFromGuideParserGeneralError(message string) (core.Issue, bool) {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return core.Issue{}, false
	}
	if library := missingNativeSystemLibrary(trimmed); library != "" {
		return core.Issue{
			Code:     core.IssueCodeNativeSystemDependencyMissing,
			Message:  nativeSystemDependencyIssueMessage(library),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeEnvironment,
		}, true
	}
	if isNativeSystemDependencyError(trimmed) {
		return core.Issue{
			Code:     core.IssueCodeNativeSystemDependencyMissing,
			Message:  nativeSystemDependencyIssueMessage(""),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeEnvironment,
		}, true
	}

	lower := strings.ToLower(trimmed)
	code := core.IssueCodeIntegrationTestExecutionFailed
	scope := core.IssueScopeIntegrationTest
	switch {
	case strings.Contains(lower, "modulenotfounderror"),
		strings.Contains(lower, "importerror"),
		strings.Contains(lower, "syntaxerror"):
		code = core.IssueCodeIntegrationScriptImportFailed
		scope = core.IssueScopeIntegrationScript
	case strings.Contains(lower, "load_model"):
		code = core.IssueCodeModelLoadFailed
		scope = core.IssueScopeModel
	case strings.Contains(lower, "preprocess"):
		code = core.IssueCodePreprocessExecutionFailed
		scope = core.IssueScopePreprocess
	}

	return core.Issue{
		Code:     code,
		Message:  fmt.Sprintf("Tensorleap parser reported: %s", firstGuideLine(trimmed)),
		Severity: core.SeverityError,
		Scope:    scope,
	}, true
}

func runtimeNativeDependencyRecommendationMessage(library string) string {
	if strings.TrimSpace(library) == "" {
		return "Next prerequisite: install the missing native system library required by this Python environment, then rerun `concierge run`."
	}
	return fmt.Sprintf("Next prerequisite: install the system package that provides `%s`, then rerun `concierge run`.", library)
}

func nativeSystemDependencyIssueMessage(library string) string {
	if strings.TrimSpace(library) == "" {
		return "importing integration dependencies failed because a required native system library is missing from the current Python environment"
	}
	return fmt.Sprintf("the current Python environment is missing native system library `%s`, so importing integration dependencies failed during Tensorleap parser validation", library)
}

func missingNativeSystemLibrary(message string) string {
	if !isNativeSystemDependencyError(message) {
		return ""
	}
	matches := sharedLibraryNamePattern.FindStringSubmatch(message)
	if len(matches) != 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func isNativeSystemDependencyError(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "cannot open shared object file") ||
		strings.Contains(lower, "library not loaded") ||
		strings.Contains(lower, "image not found") ||
		strings.Contains(lower, "dll load failed")
}

func issueFromGuidePayloadFailure(
	payload core.GuidePayloadSummary,
	handlerKinds map[string]guideHandlerKind,
) (core.Issue, bool) {
	name := normalizeGuideHandlerName(payload.Name)
	message := payload.Error
	if message == "" {
		message = fmt.Sprintf("Tensorleap parser reported a failure for %q", payload.Name)
	}

	if name == "preprocess" {
		return core.Issue{
			Code:     core.IssueCodePreprocessExecutionFailed,
			Message:  fmt.Sprintf("preprocess failed during Tensorleap parser validation: %s", firstGuideLine(message)),
			Severity: core.SeverityError,
			Scope:    core.IssueScopePreprocess,
		}, true
	}

	if payload.HandlerType == string(guideHandlerMetadata) {
		return core.Issue{}, false
	}

	switch handlerKinds[name] {
	case guideHandlerInput:
		return core.Issue{
			Code:     core.IssueCodeInputEncoderExecutionFailed,
			Message:  fmt.Sprintf("input encoder %q failed during Tensorleap parser validation: %s", payload.Name, firstGuideLine(message)),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeInputEncoder,
			Location: &core.IssueLocation{Symbol: payload.Name},
		}, true
	case guideHandlerGT:
		return core.Issue{
			Code:     core.IssueCodeGTEncoderExecutionFailed,
			Message:  fmt.Sprintf("ground-truth encoder %q failed during Tensorleap parser validation: %s", payload.Name, firstGuideLine(message)),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeGroundTruthEncoder,
			Location: &core.IssueLocation{Symbol: payload.Name},
		}, true
	default:
		return core.Issue{}, false
	}
}

func issueFromGuideCrashFunction(
	crashFunction string,
	localResult PythonRuntimeCommandResult,
	parserResult PythonRuntimeCommandResult,
) (core.Issue, bool) {
	message := firstGuideLine(strings.TrimSpace(localResult.Stderr))
	if message == "" {
		message = firstGuideLine(strings.TrimSpace(localResult.Stdout))
	}
	if message == "" {
		message = "the local guide validator crashed before completion"
	}

	switch strings.TrimSpace(crashFunction) {
	case "tensorleap_preprocess":
		return core.Issue{
			Code:     core.IssueCodePreprocessExecutionFailed,
			Message:  fmt.Sprintf("preprocess crashed during the local validator run: %s", message),
			Severity: core.SeverityError,
			Scope:    core.IssueScopePreprocess,
		}, true
	case "tensorleap_input_encoder":
		return core.Issue{
			Code:     core.IssueCodeInputEncoderExecutionFailed,
			Message:  fmt.Sprintf("an input encoder crashed during the local validator run: %s", message),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeInputEncoder,
		}, true
	case "tensorleap_gt_encoder":
		return core.Issue{
			Code:     core.IssueCodeGTEncoderExecutionFailed,
			Message:  fmt.Sprintf("a ground-truth encoder crashed during the local validator run: %s", message),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeGroundTruthEncoder,
		}, true
	case "tensorleap_load_model":
		return core.Issue{
			Code:     core.IssueCodeModelLoadFailed,
			Message:  fmt.Sprintf("load_model crashed during the local validator run: %s", message),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeModel,
		}, true
	case "tensorleap_integration_test":
		return core.Issue{
			Code:     core.IssueCodeIntegrationTestExecutionFailed,
			Message:  fmt.Sprintf("integration_test crashed during the local validator run: %s", message),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeIntegrationTest,
		}, true
	default:
		fallback := firstGuideLine(strings.TrimSpace(parserResult.Stderr))
		if fallback != "" {
			message = fallback
		}
		return core.Issue{
			Code:     core.IssueCodeIntegrationScriptImportFailed,
			Message:  fmt.Sprintf("the integration script failed before the local validator completed: %s", message),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeIntegrationScript,
		}, true
	}
}

func dedupeIssues(issues []core.Issue) []core.Issue {
	if len(issues) == 0 {
		return nil
	}

	unique := make([]core.Issue, 0, len(issues))
	seen := make(map[string]struct{}, len(issues))
	for _, issue := range issues {
		key := string(issue.Code) + "|" + issue.Message + "|" + string(issue.Scope)
		if issue.Location != nil {
			key += "|" + issue.Location.Path + "|" + issue.Location.Symbol
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, issue)
	}
	return unique
}

func firstGuideLine(value string) string {
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return line
	}
	return ""
}

func marshalGuideEvidence(name string, value any) core.EvidenceItem {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return core.EvidenceItem{Name: name, Value: "{}"}
	}
	return core.EvidenceItem{Name: name, Value: string(encoded)}
}
