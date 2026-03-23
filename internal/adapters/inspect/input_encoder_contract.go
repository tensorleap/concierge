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
	encoderFunctionDefinitionPattern  = regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
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

	sources := loadContractSources(repoRoot, status.Contracts)
	if len(sources) == 0 {
		return
	}

	registrations := make([]encoderRegistration, 0, 8)
	for _, source := range sources {
		registrations = append(registrations, discoverEncoderRegistrations(source.Contents, "tensorleap_input_encoder")...)
	}
	expected := expectedInputEncoderSymbols(status.Contracts, registrations)
	actual := encoderRegistrationSymbols(registrations)
	missing := missingContractSymbols(expected, actual)
	issuePath := primaryContractSourcePath(sources, registrations)

	if len(missing) > 0 {
		issueCode := core.IssueCodeInputEncoderCoverageIncomplete
		template := "input encoder coverage is incomplete: missing required input name %q"
		if len(actual) == 0 {
			issueCode = core.IssueCodeInputEncoderMissing
			template = "missing @tensorleap_input_encoder for required input name %q"
		}

		for _, symbol := range missing {
			status.Issues = append(status.Issues, core.Issue{
				Code:     issueCode,
				Message:  fmt.Sprintf(template, symbol),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeInputEncoder,
				Location: &core.IssueLocation{
					Path:   issuePath,
					Symbol: symbol,
				},
			})
		}
	}

	if len(expected) == 0 && len(registrations) > 0 && hasUnresolvedModelContractIssue(status.Issues) {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeInputEncoderCoverageIncomplete,
			Message:  "cannot derive required input-encoder name mapping because the model contract is unresolved",
			Severity: core.SeverityError,
			Scope:    core.IssueScopeInputEncoder,
			Location: &core.IssueLocation{
				Path:   issuePath,
				Symbol: "input_encoder",
			},
		})
	}
}

type contractSource struct {
	Path     string
	Contents string
}

func loadContractSources(repoRoot string, contracts *core.IntegrationContracts) []contractSource {
	if contracts == nil {
		return nil
	}

	sources := make([]contractSource, 0, 2)
	loaded := make(map[string]struct{}, 2)
	appendSource := func(entry string) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return
		}

		entryAbsPath := filepath.FromSlash(entry)
		if !filepath.IsAbs(entryAbsPath) {
			entryAbsPath = filepath.Join(repoRoot, entryAbsPath)
		}
		entryAbsPath = filepath.Clean(entryAbsPath)
		if !isPathWithinRepo(repoRoot, entryAbsPath) {
			return
		}

		contents, err := os.ReadFile(entryAbsPath)
		if err != nil {
			return
		}
		if _, exists := loaded[entryAbsPath]; exists {
			return
		}
		loaded[entryAbsPath] = struct{}{}

		entryFilePath := normalizedEntryFilePath(repoRoot, entryAbsPath, entry)
		sources = append(sources, contractSource{Path: entryFilePath, Contents: string(contents)})
	}

	entry := strings.TrimSpace(contracts.EntryFile)
	appendSource(entry)
	if entry != "" {
		entryAbsPath := filepath.FromSlash(entry)
		if !filepath.IsAbs(entryAbsPath) {
			entryAbsPath = filepath.Join(repoRoot, entryAbsPath)
		}
		entryAbsPath = filepath.Clean(entryAbsPath)
		if isPathWithinRepo(repoRoot, entryAbsPath) {
			if entryContents, err := os.ReadFile(entryAbsPath); err == nil {
				for _, binderPath := range discoverLegacyBinderSourcePaths(repoRoot, entryAbsPath, string(entryContents)) {
					appendSource(binderPath)
				}
			}
		}
	}
	return sources
}

func primaryContractSourcePath(sources []contractSource, registrations []encoderRegistration) string {
	if len(sources) == 0 {
		return ""
	}
	return sources[0].Path
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
	if contracts.ConfirmedMapping != nil && len(contracts.ConfirmedMapping.InputSymbols) > 0 {
		expected = append(expected, contracts.ConfirmedMapping.InputSymbols...)
		return uniqueSortedContractSymbols(expected)
	}
	// Discovery-derived symbols are intentionally only enforced when no input encoder
	// registrations exist yet; once at least one encoder exists, confirmation is
	// required to avoid noisy false positives from alternate semantic branches.
	if len(expected) == 0 && len(contracts.DiscoveredInputSymbols) > 0 {
		expected = append(expected, contracts.DiscoveredInputSymbols...)
		return uniqueSortedContractSymbols(expected)
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

func looksLikeGroundTruthEncoderCall(call string) bool {
	lower := strings.ToLower(strings.TrimSpace(call))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "ground_truth") ||
		strings.HasPrefix(lower, "encode_label") ||
		strings.HasPrefix(lower, "encode_target")
}

func hasUnresolvedModelContractIssue(issues []core.Issue) bool {
	for _, issue := range issues {
		switch issue.Code {
		case core.IssueCodeModelAcquisitionRequired,
			core.IssueCodeModelAcquisitionUnresolved,
			core.IssueCodeModelMaterializationFailed,
			core.IssueCodeModelMaterializationOutputMissing,
			core.IssueCodeModelFileMissing,
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
