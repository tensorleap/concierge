package inspect

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

var (
	encoderDecoratorInvocationPattern = regexp.MustCompile(`^\s*@\s*([A-Za-z_][A-Za-z0-9_\.]*)\s*(?:\((.*)\))?\s*$`)
	encoderFunctionDefinitionPattern  = regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(.*\)\s*(?:->\s*[^:]+)?\s*:`)
	encoderKeywordSymbolPattern       = regexp.MustCompile(`(?i)\b(?:input|feature|target|name)\s*=\s*['"]([^'"]+)['"]`)
	encoderQuotedSymbolPattern        = regexp.MustCompile(`['"]([^'"]+)['"]`)
)

type encoderDecoratorInvocation struct {
	Name      string
	Arguments string
	Line      int
}

type encoderRegistration struct {
	Function           string
	Symbol             string
	Line               int
	RawArguments       string
	HasExplicitSymbol  bool
	AppliesToUnlabeled bool
}

func inspectInputEncoderContract(repoRoot string, status *core.IntegrationStatus) {
	if status == nil || status.Contracts == nil {
		return
	}

	entryFilePath, source, ok := loadContractEntrySource(repoRoot, status.Contracts)
	if !ok {
		return
	}

	registrations := discoverEncoderRegistrations(source, "tensorleap_input_encoder")
	expected := expectedInputEncoderSymbols(status.Contracts, registrations)
	actual := encoderRegistrationSymbols(registrations)
	missing := missingContractSymbols(expected, actual)

	if len(missing) > 0 {
		issueCode := core.IssueCodeInputEncoderCoverageIncomplete
		template := "input encoder coverage is incomplete: missing required symbol %q"
		if len(actual) == 0 {
			issueCode = core.IssueCodeInputEncoderMissing
			template = "missing @tensorleap_input_encoder for required input symbol %q"
		}

		for _, symbol := range missing {
			status.Issues = append(status.Issues, core.Issue{
				Code:     issueCode,
				Message:  fmt.Sprintf(template, symbol),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeInputEncoder,
				Location: &core.IssueLocation{
					Path:   entryFilePath,
					Symbol: symbol,
				},
			})
		}
	}

	if len(expected) == 0 && len(registrations) > 0 && hasUnresolvedModelContractIssue(status.Issues) {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeInputEncoderCoverageIncomplete,
			Message:  "cannot derive required input-encoder symbol mapping because model contract is unresolved",
			Severity: core.SeverityError,
			Scope:    core.IssueScopeInputEncoder,
			Location: &core.IssueLocation{
				Path:   entryFilePath,
				Symbol: "input_encoder",
			},
		})
	}
}

func loadContractEntrySource(repoRoot string, contracts *core.IntegrationContracts) (string, string, bool) {
	if contracts == nil {
		return "", "", false
	}

	entry := strings.TrimSpace(contracts.EntryFile)
	if entry == "" {
		return "", "", false
	}

	entryAbsPath := filepath.FromSlash(entry)
	if !filepath.IsAbs(entryAbsPath) {
		entryAbsPath = filepath.Join(repoRoot, entryAbsPath)
	}
	entryAbsPath = filepath.Clean(entryAbsPath)
	if !isPathWithinRepo(repoRoot, entryAbsPath) {
		return "", "", false
	}

	contents, err := os.ReadFile(entryAbsPath)
	if err != nil {
		return "", "", false
	}

	entryFilePath := normalizedEntryFilePath(repoRoot, entryAbsPath, entry)
	return entryFilePath, string(contents), true
}

func discoverEncoderRegistrations(source string, decoratorName string) []encoderRegistration {
	lines := strings.Split(source, "\n")
	pending := make([]encoderDecoratorInvocation, 0, 4)
	registrations := make([]encoderRegistration, 0, 8)

	targetDecorator := strings.ToLower(strings.TrimSpace(decoratorName))
	for index, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "@") {
			invocation, ok := extractEncoderDecoratorInvocation(line)
			if !ok {
				pending = pending[:0]
				continue
			}
			invocation.Line = index + 1
			pending = append(pending, invocation)
			continue
		}

		if strings.HasPrefix(line, "def ") {
			functionName, ok := extractEncoderFunctionName(line)
			if !ok {
				pending = pending[:0]
				continue
			}

			for _, invocation := range pending {
				if invocation.Name != targetDecorator {
					continue
				}
				symbol, explicit := extractEncoderSymbol(invocation.Arguments, functionName)
				registrations = append(registrations, encoderRegistration{
					Function:           functionName,
					Symbol:             symbol,
					Line:               index + 1,
					RawArguments:       strings.TrimSpace(invocation.Arguments),
					HasExplicitSymbol:  explicit,
					AppliesToUnlabeled: strings.Contains(strings.ToLower(invocation.Arguments), "unlabeled"),
				})
			}

			pending = pending[:0]
			continue
		}

		pending = pending[:0]
	}

	return dedupeEncoderRegistrations(registrations)
}

func extractEncoderDecoratorInvocation(line string) (encoderDecoratorInvocation, bool) {
	matches := encoderDecoratorInvocationPattern.FindStringSubmatch(line)
	if len(matches) != 3 {
		return encoderDecoratorInvocation{}, false
	}

	return encoderDecoratorInvocation{
		Name:      strings.ToLower(canonicalSymbol(matches[1])),
		Arguments: strings.TrimSpace(matches[2]),
	}, true
}

func extractEncoderFunctionName(line string) (string, bool) {
	matches := encoderFunctionDefinitionPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	return strings.TrimSpace(matches[1]), true
}

func extractEncoderSymbol(arguments string, functionName string) (string, bool) {
	args := strings.TrimSpace(arguments)
	if args == "" {
		return inferEncoderSymbol(functionName), false
	}

	if matches := encoderKeywordSymbolPattern.FindStringSubmatch(args); len(matches) == 2 {
		return normalizeEncoderSymbol(matches[1]), true
	}
	if matches := encoderQuotedSymbolPattern.FindStringSubmatch(args); len(matches) == 2 {
		return normalizeEncoderSymbol(matches[1]), true
	}

	return inferEncoderSymbol(functionName), false
}

func inferEncoderSymbol(value string) string {
	symbol := strings.TrimSpace(value)
	if symbol == "" {
		return ""
	}

	lower := strings.ToLower(symbol)
	for _, prefix := range []string{"encode_", "input_", "gt_", "label_", "target_"} {
		if strings.HasPrefix(lower, prefix) {
			return normalizeEncoderSymbol(symbol[len(prefix):])
		}
	}
	return normalizeEncoderSymbol(symbol)
}

func normalizeEncoderSymbol(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(trimmed)
}

func dedupeEncoderRegistrations(registrations []encoderRegistration) []encoderRegistration {
	if len(registrations) == 0 {
		return nil
	}

	type dedupeKey struct {
		Function string
		Symbol   string
	}

	seen := make(map[dedupeKey]struct{}, len(registrations))
	unique := make([]encoderRegistration, 0, len(registrations))
	for _, registration := range registrations {
		function := strings.TrimSpace(registration.Function)
		symbol := strings.TrimSpace(registration.Symbol)
		if function == "" || symbol == "" {
			continue
		}

		key := dedupeKey{
			Function: strings.ToLower(function),
			Symbol:   strings.ToLower(symbol),
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, registration)
	}

	sort.Slice(unique, func(i, j int) bool {
		if !strings.EqualFold(unique[i].Function, unique[j].Function) {
			return strings.ToLower(unique[i].Function) < strings.ToLower(unique[j].Function)
		}
		if !strings.EqualFold(unique[i].Symbol, unique[j].Symbol) {
			return strings.ToLower(unique[i].Symbol) < strings.ToLower(unique[j].Symbol)
		}
		return unique[i].Line < unique[j].Line
	})

	return unique
}

func expectedInputEncoderSymbols(contracts *core.IntegrationContracts, registrations []encoderRegistration) []string {
	expected := make([]string, 0, len(registrations))
	for _, registration := range registrations {
		expected = append(expected, registration.Symbol)
	}

	if contracts == nil {
		return uniqueSortedContractSymbols(expected)
	}

	registrationsByFunction := mapEncoderRegistrationsByFunction(registrations)
	excludedCalls := contractSymbolSet(
		contracts.LoadModelFunctions,
		contracts.PreprocessFunctions,
		contracts.GroundTruthEncoders,
		contracts.IntegrationTestFunctions,
	)

	for _, rawCall := range contracts.IntegrationTestCalls {
		call := strings.TrimSpace(canonicalSymbol(rawCall))
		if call == "" {
			continue
		}
		key := strings.ToLower(call)
		if _, excluded := excludedCalls[key]; excluded {
			continue
		}

		if registration, ok := registrationsByFunction[key]; ok {
			expected = append(expected, registration.Symbol)
			continue
		}

		if looksLikeInputEncoderCall(call) {
			expected = append(expected, inferEncoderSymbol(call))
		}
	}

	return uniqueSortedContractSymbols(expected)
}

func mapEncoderRegistrationsByFunction(registrations []encoderRegistration) map[string]encoderRegistration {
	index := make(map[string]encoderRegistration, len(registrations))
	for _, registration := range registrations {
		function := strings.ToLower(strings.TrimSpace(registration.Function))
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

func contractSymbolSet(groups ...[]string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, group := range groups {
		for _, value := range group {
			key := strings.ToLower(strings.TrimSpace(value))
			if key == "" {
				continue
			}
			set[key] = struct{}{}
		}
	}
	return set
}

func encoderRegistrationSymbols(registrations []encoderRegistration) []string {
	symbols := make([]string, 0, len(registrations))
	for _, registration := range registrations {
		symbols = append(symbols, registration.Symbol)
	}
	return uniqueSortedContractSymbols(symbols)
}

func missingContractSymbols(expected []string, actual []string) []string {
	if len(expected) == 0 {
		return nil
	}

	actualSet := contractSymbolSet(actual)
	missing := make([]string, 0, len(expected))
	for _, symbol := range expected {
		key := strings.ToLower(strings.TrimSpace(symbol))
		if key == "" {
			continue
		}
		if _, exists := actualSet[key]; exists {
			continue
		}
		missing = append(missing, normalizeEncoderSymbol(symbol))
	}
	return uniqueSortedContractSymbols(missing)
}

func uniqueSortedContractSymbols(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	unique := make(map[string]string, len(values))
	for _, value := range values {
		normalized := normalizeEncoderSymbol(value)
		if normalized == "" {
			continue
		}
		if _, exists := unique[normalized]; exists {
			continue
		}
		unique[normalized] = normalized
	}
	if len(unique) == 0 {
		return nil
	}

	symbols := make([]string, 0, len(unique))
	for _, value := range unique {
		symbols = append(symbols, value)
	}
	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i] < symbols[j]
	})
	return symbols
}

func looksLikeInputEncoderCall(call string) bool {
	lower := strings.ToLower(strings.TrimSpace(call))
	if lower == "" {
		return false
	}
	if looksLikeGroundTruthEncoderCall(lower) {
		return false
	}
	return strings.HasPrefix(lower, "encode_") || strings.HasPrefix(lower, "input_")
}

func looksLikeGroundTruthEncoderCall(call string) bool {
	lower := strings.ToLower(strings.TrimSpace(call))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "label") ||
		strings.Contains(lower, "target") ||
		strings.HasPrefix(lower, "gt_") ||
		strings.Contains(lower, "ground_truth")
}

func hasUnresolvedModelContractIssue(issues []core.Issue) bool {
	for _, issue := range issues {
		switch issue.Code {
		case core.IssueCodeModelFileMissing,
			core.IssueCodeModelCandidatesAmbiguous,
			core.IssueCodeModelFormatUnsupported,
			core.IssueCodeModelLoadFailed,
			core.IssueCodeModelInputBatchDimensionMissing,
			core.IssueCodeModelInputShapeMismatch:
			return true
		}
	}
	return false
}
