package inspect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

var (
	decoratorPattern = regexp.MustCompile(`^\s*@\s*([A-Za-z_][A-Za-z0-9_\.]*)`)
	functionPattern  = regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(.*\)\s*(?:->\s*[^:]+)?\s*:`)
	callPattern      = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_\.]*)\s*\(`)
	pythonImportPattern = regexp.MustCompile(`^\s*import\s+(.+)$`)
	pythonFromImportPattern = regexp.MustCompile(`^\s*from\s+([A-Za-z_][A-Za-z0-9_\.]*)\s+import\s+`)
)

type contractDiscoverySyntaxError struct {
	Line    int
	Column  int
	Message string
}

func (e contractDiscoverySyntaxError) Error() string {
	if e.Line <= 0 {
		return e.Message
	}
	if e.Column > 0 {
		return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("line %d: %s", e.Line, e.Message)
}

func inspectIntegrationContracts(repoRoot string, contract *leapYAMLContract, status *core.IntegrationStatus) error {
	if contract == nil || status == nil {
		return nil
	}

	entryFile := strings.TrimSpace(contract.EntryFile)
	if entryFile == "" {
		return nil
	}

	entryAbsPath := entryFile
	if !filepath.IsAbs(entryAbsPath) {
		entryAbsPath = filepath.Join(repoRoot, filepath.FromSlash(entryFile))
	}
	entryAbsPath = filepath.Clean(entryAbsPath)
	if !isPathWithinRepo(repoRoot, entryAbsPath) {
		return nil
	}

	info, err := os.Stat(entryAbsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return core.WrapError(core.KindUnknown, "inspect.baseline.entry_contract_stat", err)
	}
	if info.IsDir() {
		return nil
	}

	entryFilePath := normalizedEntryFilePath(repoRoot, entryAbsPath, entryFile)
	contents, err := os.ReadFile(entryAbsPath)
	if err != nil {
		status.Issues = append(status.Issues, newContractDiscoveryIssue(entryFilePath, 0, 0,
			fmt.Sprintf("failed to read entry file %q for contract discovery: %v", entryFilePath, err)))
		return nil
	}

	contracts, err := discoverContractsFromPythonSource(entryFilePath, string(contents))
	mergeLegacyBinderContracts(repoRoot, entryAbsPath, string(contents), contracts)
	if contracts != nil {
		status.Contracts = contracts
	}
	if err == nil {
		if contracts != nil && len(contracts.IntegrationTestFunctions) == 0 {
			status.Issues = append(status.Issues, core.Issue{
				Code:     core.IssueCodeIntegrationTestMissing,
				Message:  fmt.Sprintf("no @tensorleap_integration_test function found in %s", entryFilePath),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
				Location: &core.IssueLocation{
					Path:   entryFilePath,
					Symbol: "integration_test",
				},
			})
		}
		inspectPreprocessContract(entryFilePath, string(contents), contracts, status)
		inspectMainBlock(entryFilePath, string(contents), contracts, status)
		return nil
	}

	location := core.IssueLocation{Path: entryFilePath}
	if syntaxErr, ok := err.(contractDiscoverySyntaxError); ok {
		location.Line = syntaxErr.Line
		location.Column = syntaxErr.Column
	}
	status.Issues = append(status.Issues, newContractDiscoveryIssue(
		entryFilePath,
		location.Line,
		location.Column,
		fmt.Sprintf("contract discovery failed for entry file %q: %v", entryFilePath, err),
	))

	return nil
}

func appendUniqueStrings(existing []string, values ...string) []string {
	for _, value := range values {
		existing = appendUnique(existing, value)
	}
	return existing
}

func mergeLegacyBinderContracts(repoRoot, entryAbsPath, source string, contracts *core.IntegrationContracts) {
	if contracts == nil {
		return
	}

	for _, binderPath := range discoverLegacyBinderSourcePaths(repoRoot, entryAbsPath, source) {
		contents, err := os.ReadFile(binderPath)
		if err != nil {
			continue
		}

		logicalPath := normalizedEntryFilePath(repoRoot, binderPath, filepath.ToSlash(binderPath))
		binderContracts, err := discoverContractsFromPythonSource(logicalPath, string(contents))
		if binderContracts == nil {
			continue
		}
		_ = err

		contracts.LoadModelFunctions = appendUniqueStrings(contracts.LoadModelFunctions, binderContracts.LoadModelFunctions...)
		contracts.PreprocessFunctions = appendUniqueStrings(contracts.PreprocessFunctions, binderContracts.PreprocessFunctions...)
		contracts.InputEncoders = appendUniqueStrings(contracts.InputEncoders, binderContracts.InputEncoders...)
		contracts.GroundTruthEncoders = appendUniqueStrings(contracts.GroundTruthEncoders, binderContracts.GroundTruthEncoders...)
		contracts.IntegrationTestFunctions = appendUniqueStrings(contracts.IntegrationTestFunctions, binderContracts.IntegrationTestFunctions...)
		contracts.IntegrationTestCalls = appendUniqueStrings(contracts.IntegrationTestCalls, binderContracts.IntegrationTestCalls...)
	}
}

func discoverLegacyBinderSourcePaths(repoRoot, entryAbsPath, source string) []string {
	entryDir := filepath.Dir(entryAbsPath)
	paths := make(map[string]struct{})

	for _, module := range discoverImportedModules(source) {
		if !strings.HasSuffix(module, "leap_binder") {
			continue
		}

		candidate := filepath.Join(repoRoot, filepath.FromSlash(strings.ReplaceAll(module, ".", "/"))+".py")
		candidate = filepath.Clean(candidate)
		if !isPathWithinRepo(repoRoot, candidate) {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			paths[candidate] = struct{}{}
			continue
		}

		localCandidate := filepath.Join(entryDir, filepath.Base(candidate))
		localCandidate = filepath.Clean(localCandidate)
		if !isPathWithinRepo(repoRoot, localCandidate) {
			continue
		}
		if info, err := os.Stat(localCandidate); err == nil && !info.IsDir() {
			paths[localCandidate] = struct{}{}
		}
	}

	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func discoverImportedModules(source string) []string {
	modules := make([]string, 0, 4)
	seen := make(map[string]struct{})

	for _, rawLine := range strings.Split(source, "\n") {
		line := strings.TrimSpace(stripInlinePythonComment(rawLine))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if matches := pythonFromImportPattern.FindStringSubmatch(line); len(matches) == 2 {
			module := strings.TrimSpace(matches[1])
			if module != "" {
				if _, exists := seen[module]; !exists {
					seen[module] = struct{}{}
					modules = append(modules, module)
				}
			}
			continue
		}

		matches := pythonImportPattern.FindStringSubmatch(line)
		if len(matches) != 2 {
			continue
		}
		for _, part := range strings.Split(matches[1], ",") {
			module := strings.TrimSpace(part)
			if module == "" {
				continue
			}
			if beforeAlias, _, ok := strings.Cut(module, " as "); ok {
				module = strings.TrimSpace(beforeAlias)
			}
			if module == "" {
				continue
			}
			if _, exists := seen[module]; exists {
				continue
			}
			seen[module] = struct{}{}
			modules = append(modules, module)
		}
	}

	return modules
}

func discoverContractsFromPythonSource(entryFilePath string, source string) (*core.IntegrationContracts, error) {
	contracts := &core.IntegrationContracts{
		EntryFile: normalizedSourcePath(entryFilePath),
	}

	lines := strings.Split(source, "\n")
	pendingDecorators := make([]string, 0, 4)
	for index := 0; index < len(lines); index++ {
		rawLine := lines[index]
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "@") {
			decorator, ok := extractDecoratorName(line)
			if !ok {
				return contracts, contractDiscoverySyntaxError{
					Line:    index + 1,
					Column:  1,
					Message: "invalid decorator syntax",
				}
			}
			pendingDecorators = append(pendingDecorators, decorator)
			continue
		}

		if strings.HasPrefix(line, "def ") {
			functionName, defEndIndex, ok := extractFunctionDefinition(lines, index)
			if !ok {
				return contracts, contractDiscoverySyntaxError{
					Line:    index + 1,
					Column:  1,
					Message: "invalid function definition syntax",
				}
			}

			if hasDecorator(pendingDecorators, "tensorleap_load_model") {
				contracts.LoadModelFunctions = appendUnique(contracts.LoadModelFunctions, functionName)
			}
			if hasDecorator(pendingDecorators, "tensorleap_preprocess") {
				contracts.PreprocessFunctions = appendUnique(contracts.PreprocessFunctions, functionName)
			}
			if hasDecorator(pendingDecorators, "tensorleap_input_encoder") {
				contracts.InputEncoders = appendUnique(contracts.InputEncoders, functionName)
			}
			if hasDecorator(pendingDecorators, "tensorleap_gt_encoder") {
				contracts.GroundTruthEncoders = appendUnique(contracts.GroundTruthEncoders, functionName)
			}
			if hasDecorator(pendingDecorators, "tensorleap_integration_test") {
				contracts.IntegrationTestFunctions = appendUnique(contracts.IntegrationTestFunctions, functionName)
				calls := extractFunctionCalls(lines, index, defEndIndex)
				for _, call := range calls {
					contracts.IntegrationTestCalls = appendUnique(contracts.IntegrationTestCalls, call)
				}
			}

			pendingDecorators = pendingDecorators[:0]
			index = defEndIndex
			continue
		}

		pendingDecorators = pendingDecorators[:0]
	}

	if len(pendingDecorators) > 0 {
		return contracts, contractDiscoverySyntaxError{
			Line:    len(lines),
			Column:  1,
			Message: "decorator is not attached to a function definition",
		}
	}

	return contracts, nil
}

func extractDecoratorName(line string) (string, bool) {
	matches := decoratorPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	return strings.ToLower(canonicalSymbol(matches[1])), true
}

func extractFunctionName(line string) (string, bool) {
	matches := functionPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}

func extractFunctionDefinition(lines []string, startIndex int) (string, int, bool) {
	if startIndex < 0 || startIndex >= len(lines) {
		return "", startIndex, false
	}

	headerParts := make([]string, 0, 4)
	parenDepth := 0
	sawOpenParen := false
	for index := startIndex; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		headerParts = append(headerParts, line)
		if strings.Contains(line, "(") {
			sawOpenParen = true
		}
		parenDepth += strings.Count(line, "(") - strings.Count(line, ")")

		header := strings.Join(headerParts, " ")
		if sawOpenParen && parenDepth <= 0 && strings.HasSuffix(line, ":") {
			name, ok := extractFunctionName(header)
			return name, index, ok
		}
	}

	return "", startIndex, false
}

func hasDecorator(decorators []string, name string) bool {
	target := strings.ToLower(strings.TrimSpace(name))
	for _, decorator := range decorators {
		if strings.EqualFold(decorator, target) {
			return true
		}
	}
	return false
}

func extractFunctionCalls(lines []string, defLineIndex int, defEndIndex int) []string {
	defIndent := indentationLevel(lines[defLineIndex])
	calls := make([]string, 0, 8)

	for i := defEndIndex + 1; i < len(lines); i++ {
		rawLine := lines[i]
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if indentationLevel(rawLine) <= defIndent {
			break
		}

		matches := callPattern.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			call := canonicalSymbol(match[1])
			if call == "" || shouldIgnoreCallSymbol(call) {
				continue
			}
			calls = appendUnique(calls, call)
		}
	}

	return calls
}

func shouldIgnoreCallSymbol(symbol string) bool {
	switch symbol {
	case "if", "for", "while", "with", "except", "elif", "assert", "return", "def", "class", "lambda":
		return true
	default:
		return false
	}
}

func canonicalSymbol(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lastDot := strings.LastIndex(trimmed, ".")
	if lastDot < 0 {
		return trimmed
	}
	return trimmed[lastDot+1:]
}

func indentationLevel(line string) int {
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

func normalizedEntryFilePath(repoRoot, entryAbsPath, fallback string) string {
	relPath, err := filepath.Rel(repoRoot, entryAbsPath)
	if err == nil && relPath != "" && !strings.HasPrefix(relPath, "..") {
		return normalizedSourcePath(relPath)
	}
	return normalizedSourcePath(fallback)
}

func normalizedSourcePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(trimmed))
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func inspectMainBlock(entryFilePath string, source string, contracts *core.IntegrationContracts, status *core.IntegrationStatus) {
	if contracts == nil || status == nil {
		return
	}
	if len(contracts.PreprocessFunctions) == 0 || len(contracts.IntegrationTestFunctions) == 0 {
		return
	}
	if strings.Contains(source, "if __name__") {
		return
	}
	status.Issues = append(status.Issues, core.Issue{
		Code:     core.IssueCodeIntegrationTestMainBlockMissing,
		Message:  fmt.Sprintf("%s has @tensorleap_preprocess and @tensorleap_integration_test but no if __name__ == \"__main__\" entry-point", entryFilePath),
		Severity: core.SeverityError,
		Scope:    core.IssueScopeIntegrationTest,
		Location: &core.IssueLocation{
			Path: entryFilePath,
		},
	})
}

func newContractDiscoveryIssue(path string, line int, column int, message string) core.Issue {
	location := &core.IssueLocation{Path: path}
	if line > 0 {
		location.Line = line
	}
	if column > 0 {
		location.Column = column
	}

	return core.Issue{
		Code:     core.IssueCodeIntegrationScriptImportFailed,
		Message:  message,
		Severity: core.SeverityError,
		Scope:    core.IssueScopeIntegrationScript,
		Location: location,
	}
}
