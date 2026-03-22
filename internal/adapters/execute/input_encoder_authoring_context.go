package execute

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

var (
	authoringDecoratorPattern       = regexp.MustCompile(`^\s*@\s*([A-Za-z_][A-Za-z0-9_\.]*)\s*(?:\((.*)\))?\s*$`)
	authoringFunctionPattern        = regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(.*\)\s*(?:->\s*[^:]+)?\s*:`)
	authoringIntegrationCallPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_\.]*)\s*\(`)
	authoringKeywordSymbolPattern   = regexp.MustCompile(`(?i)\b(?:input|feature|target|name)\s*=\s*['"]([^'"]+)['"]`)
	authoringQuotedSymbolPattern    = regexp.MustCompile(`['"]([^'"]+)['"]`)
)

type authoringDecoratorInvocation struct {
	Name      string
	Arguments string
}

type authoringEncoderRegistration struct {
	Function          string
	Symbol            string
	HasExplicitSymbol bool
	RawArguments      string
}

// BuildInputEncoderAuthoringRecommendation builds deterministic remediation guidance for ensure.input_encoders.
func BuildInputEncoderAuthoringRecommendation(
	snapshot core.WorkspaceSnapshot,
	status core.IntegrationStatus,
) (core.AuthoringRecommendation, error) {
	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return core.AuthoringRecommendation{}, core.NewError(
			core.KindUnknown,
			"execute.input_encoder_authoring.repo_root",
			"snapshot repository root is empty",
		)
	}

	missingSymbols := collectInputEncoderRecommendationSymbols(repoRoot, status)
	constraints := []string{
		"Implement @tensorleap_input_encoder functions for each missing input symbol.",
		"Register each encoder with the exact required Tensorleap symbol name; do not substitute raw model tensor aliases such as \"images\" for required symbols such as \"image\".",
		"The first encoder argument is the Tensorleap sample_id matching PreprocessResponse.sample_id_type; do not treat it as a positional index into preprocess.sample_ids.",
		"Keep encoder output shapes and dtypes compatible with model inference inputs.",
		"Do not modify @tensorleap_gt_encoder definitions in this step.",
		"After adding each encoder, wire it into the @tensorleap_integration_test function body so the code_loader status table sees it exercised.",
	}
	if selectedModelPath := strings.TrimSpace(snapshot.SelectedModelPath); selectedModelPath != "" {
		constraints = append(constraints, fmt.Sprintf("Use model path %q as the shape-contract reference unless repository evidence proves it invalid.", selectedModelPath))
	}

	recommendation := core.AuthoringRecommendation{
		StepID:      core.EnsureStepInputEncoders,
		Candidates:  missingSymbols,
		Constraints: constraints,
	}

	if len(missingSymbols) > 0 {
		recommendation.Target = missingSymbols[0]
		recommendation.Rationale = "missing input-encoder symbols discovered from integration contracts"
		return recommendation, nil
	}

	recommendation.Rationale = "add or repair @tensorleap_input_encoder mappings for required model-input symbols"
	return recommendation, nil
}

func collectInputEncoderRecommendationSymbols(repoRoot string, status core.IntegrationStatus) []string {
	symbolsFromIssues := recommendationSymbolsFromIssues(status.Issues,
		core.IssueCodeInputEncoderMissing,
		core.IssueCodeInputEncoderCoverageIncomplete,
	)
	if len(symbolsFromIssues) > 0 {
		return symbolsFromIssues
	}

	source, ok := resolveAuthoringEntrySource(repoRoot, status.Contracts)
	if !ok {
		return nil
	}

	return deriveMissingInputSymbolsFromSource(source, status.Contracts)
}

func resolveAuthoringEntrySource(repoRoot string, contracts *core.IntegrationContracts) (string, bool) {
	entryFile := resolvePreprocessAuthoringEntryFile(repoRoot, contracts)
	if strings.TrimSpace(entryFile) == "" {
		return "", false
	}
	source, err := os.ReadFile(entryFile)
	if err != nil {
		return "", false
	}
	return string(source), true
}

func recommendationSymbolsFromIssues(issues []core.Issue, codes ...core.IssueCode) []string {
	if len(issues) == 0 || len(codes) == 0 {
		return nil
	}

	codeSet := make(map[core.IssueCode]struct{}, len(codes))
	for _, code := range codes {
		codeSet[code] = struct{}{}
	}

	symbols := make([]string, 0, len(issues))
	for _, issue := range issues {
		if _, ok := codeSet[issue.Code]; !ok {
			continue
		}
		if issue.Location == nil {
			continue
		}
		symbol := normalizeAuthoringSymbol(issue.Location.Symbol)
		if symbol == "" || symbol == "input_encoder" || symbol == "gt_encoder" {
			continue
		}
		symbols = append(symbols, symbol)
	}
	return uniqueSortedStrings(symbols)
}

func deriveMissingInputSymbolsFromSource(source string, contracts *core.IntegrationContracts) []string {
	inputRegistrations := discoverAuthoringEncoderRegistrations(source, "tensorleap_input_encoder")
	gtRegistrations := discoverAuthoringEncoderRegistrations(source, "tensorleap_gt_encoder")
	integrationCalls := discoverAuthoringIntegrationCalls(source, contracts)

	registrationByFunction := mapAuthoringRegistrationsByFunction(inputRegistrations)
	excludedFunctions := authoringFunctionSet(contracts, nil, gtRegistrations)

	expected := make([]string, 0, len(inputRegistrations))
	for _, registration := range inputRegistrations {
		expected = append(expected, registration.Symbol)
	}

	for _, rawCall := range integrationCalls {
		call := canonicalAuthoringSymbol(rawCall)
		if call == "" {
			continue
		}
		key := strings.ToLower(call)
		if _, excluded := excludedFunctions[key]; excluded {
			continue
		}
		if registration, ok := registrationByFunction[key]; ok {
			expected = append(expected, registration.Symbol)
			continue
		}
		if looksLikeInputAuthoringCall(call) {
			expected = append(expected, inferAuthoringSymbol(call))
		}
	}

	actual := make([]string, 0, len(inputRegistrations))
	for _, registration := range inputRegistrations {
		actual = append(actual, registration.Symbol)
	}

	return missingAuthoringSymbols(expected, actual)
}

func discoverAuthoringEncoderRegistrations(source string, decoratorName string) []authoringEncoderRegistration {
	lines := strings.Split(source, "\n")
	pending := make([]authoringDecoratorInvocation, 0, 4)
	registrations := make([]authoringEncoderRegistration, 0, 8)
	targetDecorator := strings.ToLower(strings.TrimSpace(decoratorName))

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "@") {
			invocation, ok := extractAuthoringDecoratorInvocation(line)
			if !ok {
				pending = pending[:0]
				continue
			}
			pending = append(pending, invocation)
			continue
		}

		if strings.HasPrefix(line, "def ") {
			functionName, ok := extractAuthoringFunctionName(line)
			if !ok {
				pending = pending[:0]
				continue
			}
			for _, invocation := range pending {
				if invocation.Name != targetDecorator {
					continue
				}
				symbol, explicit := extractAuthoringSymbol(invocation.Arguments, functionName)
				registrations = append(registrations, authoringEncoderRegistration{
					Function:          functionName,
					Symbol:            symbol,
					HasExplicitSymbol: explicit,
					RawArguments:      strings.TrimSpace(invocation.Arguments),
				})
			}
			pending = pending[:0]
			continue
		}

		pending = pending[:0]
	}

	return uniqueAuthoringRegistrations(registrations)
}

func extractAuthoringDecoratorInvocation(line string) (authoringDecoratorInvocation, bool) {
	matches := authoringDecoratorPattern.FindStringSubmatch(line)
	if len(matches) != 3 {
		return authoringDecoratorInvocation{}, false
	}
	return authoringDecoratorInvocation{
		Name:      canonicalAuthoringSymbol(matches[1]),
		Arguments: strings.TrimSpace(matches[2]),
	}, true
}

func extractAuthoringFunctionName(line string) (string, bool) {
	matches := authoringFunctionPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	return strings.TrimSpace(matches[1]), true
}

func extractAuthoringSymbol(arguments string, functionName string) (string, bool) {
	args := strings.TrimSpace(arguments)
	if args == "" {
		return inferAuthoringSymbol(functionName), false
	}
	if matches := authoringKeywordSymbolPattern.FindStringSubmatch(args); len(matches) == 2 {
		return normalizeAuthoringSymbol(matches[1]), true
	}
	if matches := authoringQuotedSymbolPattern.FindStringSubmatch(args); len(matches) == 2 {
		return normalizeAuthoringSymbol(matches[1]), true
	}
	return inferAuthoringSymbol(functionName), false
}

func discoverAuthoringIntegrationCalls(source string, contracts *core.IntegrationContracts) []string {
	if contracts != nil && len(contracts.IntegrationTestCalls) > 0 {
		return append([]string(nil), contracts.IntegrationTestCalls...)
	}

	lines := strings.Split(source, "\n")
	calls := make([]string, 0, 8)
	pendingDecorators := make([]string, 0, 4)
	inIntegrationFunction := false
	integrationIndent := 0

	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "@") {
			invocation, ok := extractAuthoringDecoratorInvocation(trimmed)
			if !ok {
				pendingDecorators = pendingDecorators[:0]
				continue
			}
			pendingDecorators = append(pendingDecorators, invocation.Name)
			continue
		}

		if strings.HasPrefix(trimmed, "def ") {
			_, ok := extractAuthoringFunctionName(trimmed)
			if !ok {
				pendingDecorators = pendingDecorators[:0]
				inIntegrationFunction = false
				continue
			}
			inIntegrationFunction = authoringHasDecorator(pendingDecorators, "tensorleap_integration_test")
			integrationIndent = authoringIndentationLevel(rawLine)
			pendingDecorators = pendingDecorators[:0]
			continue
		}

		if !inIntegrationFunction {
			pendingDecorators = pendingDecorators[:0]
			continue
		}
		if authoringIndentationLevel(rawLine) <= integrationIndent {
			inIntegrationFunction = false
			continue
		}

		matches := authoringIntegrationCallPattern.FindAllStringSubmatch(trimmed, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			call := canonicalAuthoringSymbol(match[1])
			if call == "" {
				continue
			}
			calls = append(calls, call)
		}
	}

	return uniqueSortedStrings(calls)
}

func authoringHasDecorator(decorators []string, decoratorName string) bool {
	target := strings.ToLower(strings.TrimSpace(decoratorName))
	for _, decorator := range decorators {
		if strings.EqualFold(strings.TrimSpace(decorator), target) {
			return true
		}
	}
	return false
}

func authoringIndentationLevel(line string) int {
	indent := 0
	for _, char := range line {
		switch char {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			return indent
		}
	}
	return indent
}

func canonicalAuthoringSymbol(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lastDot := strings.LastIndex(trimmed, ".")
	if lastDot < 0 {
		return strings.ToLower(trimmed)
	}
	return strings.ToLower(trimmed[lastDot+1:])
}

func inferAuthoringSymbol(value string) string {
	symbol := normalizeAuthoringSymbol(value)
	if symbol == "" {
		return ""
	}

	for _, prefix := range []string{"encode_", "input_", "gt_", "label_", "target_"} {
		if strings.HasPrefix(symbol, prefix) {
			return normalizeAuthoringSymbol(symbol[len(prefix):])
		}
	}
	return symbol
}

func normalizeAuthoringSymbol(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func uniqueAuthoringRegistrations(registrations []authoringEncoderRegistration) []authoringEncoderRegistration {
	if len(registrations) == 0 {
		return nil
	}

	seen := make(map[string]authoringEncoderRegistration, len(registrations))
	for _, registration := range registrations {
		function := normalizeAuthoringSymbol(registration.Function)
		symbol := normalizeAuthoringSymbol(registration.Symbol)
		if function == "" || symbol == "" {
			continue
		}
		key := function + ":" + symbol
		if _, exists := seen[key]; exists {
			continue
		}
		registration.Function = function
		registration.Symbol = symbol
		seen[key] = registration
	}
	if len(seen) == 0 {
		return nil
	}

	unique := make([]authoringEncoderRegistration, 0, len(seen))
	for _, registration := range seen {
		unique = append(unique, registration)
	}
	return unique
}

func mapAuthoringRegistrationsByFunction(registrations []authoringEncoderRegistration) map[string]authoringEncoderRegistration {
	index := make(map[string]authoringEncoderRegistration, len(registrations))
	for _, registration := range registrations {
		function := normalizeAuthoringSymbol(registration.Function)
		if function == "" {
			continue
		}
		if _, exists := index[function]; exists {
			continue
		}
		index[function] = registration
	}
	return index
}

func authoringFunctionSet(
	contracts *core.IntegrationContracts,
	inputRegistrations []authoringEncoderRegistration,
	gtRegistrations []authoringEncoderRegistration,
) map[string]struct{} {
	set := make(map[string]struct{})
	add := func(values ...string) {
		for _, value := range values {
			key := normalizeAuthoringSymbol(value)
			if key == "" {
				continue
			}
			set[key] = struct{}{}
		}
	}

	if contracts != nil {
		add(contracts.LoadModelFunctions...)
		add(contracts.PreprocessFunctions...)
		add(contracts.IntegrationTestFunctions...)
		add(contracts.GroundTruthEncoders...)
	}

	for _, registration := range inputRegistrations {
		add(registration.Function)
	}
	for _, registration := range gtRegistrations {
		add(registration.Function)
	}

	return set
}

func missingAuthoringSymbols(expected []string, actual []string) []string {
	expected = uniqueSortedStrings(expected)
	actual = uniqueSortedStrings(actual)
	if len(expected) == 0 {
		return nil
	}

	actualSet := make(map[string]struct{}, len(actual))
	for _, symbol := range actual {
		key := normalizeAuthoringSymbol(symbol)
		if key == "" {
			continue
		}
		actualSet[key] = struct{}{}
	}

	missing := make([]string, 0, len(expected))
	for _, symbol := range expected {
		key := normalizeAuthoringSymbol(symbol)
		if key == "" {
			continue
		}
		if _, exists := actualSet[key]; exists {
			continue
		}
		missing = append(missing, key)
	}
	return uniqueSortedStrings(missing)
}

func looksLikeInputAuthoringCall(call string) bool {
	lower := normalizeAuthoringSymbol(call)
	if lower == "" {
		return false
	}
	if looksLikeGTAuthoringCall(lower) {
		return false
	}
	return strings.HasPrefix(lower, "encode_") || strings.HasPrefix(lower, "input_")
}

func looksLikeGTAuthoringCall(call string) bool {
	lower := normalizeAuthoringSymbol(call)
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "label") ||
		strings.Contains(lower, "target") ||
		strings.HasPrefix(lower, "gt_") ||
		strings.Contains(lower, "ground_truth")
}
