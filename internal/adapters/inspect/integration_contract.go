package inspect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

var (
	decoratorPattern = regexp.MustCompile(`^\s*@\s*([A-Za-z_][A-Za-z0-9_\.]*)`)
	functionPattern  = regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(.*\)\s*:`)
	callPattern      = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_\.]*)\s*\(`)
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
	if contracts != nil {
		status.Contracts = contracts
	}
	if err == nil {
		emitIntegrationContractPresenceIssues(entryFilePath, contracts, status)
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

func emitIntegrationContractPresenceIssues(entryFilePath string, contracts *core.IntegrationContracts, status *core.IntegrationStatus) {
	if contracts == nil || status == nil {
		return
	}

	normalizedEntry := strings.ToLower(strings.TrimSpace(filepath.Base(entryFilePath)))
	if normalizedEntry == "" {
		return
	}
	if normalizedEntry != "leap_binder.py" {
		return
	}

	if len(contracts.PreprocessFunctions) == 0 {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodePreprocessFunctionMissing,
			Message:  "no @tensorleap_preprocess function found in leap_binder.py",
			Severity: core.SeverityError,
			Scope:    core.IssueScopePreprocess,
			Location: &core.IssueLocation{
				Path:   entryFilePath,
				Symbol: "preprocess",
			},
		})
	}
}

func discoverContractsFromPythonSource(entryFilePath string, source string) (*core.IntegrationContracts, error) {
	contracts := &core.IntegrationContracts{
		EntryFile: normalizedSourcePath(entryFilePath),
	}

	lines := strings.Split(source, "\n")
	pendingDecorators := make([]string, 0, 4)
	for index, rawLine := range lines {
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

		if strings.HasPrefix(line, "def") {
			functionName, ok := extractFunctionName(line)
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
				calls := extractFunctionCalls(lines, index)
				for _, call := range calls {
					contracts.IntegrationTestCalls = appendUnique(contracts.IntegrationTestCalls, call)
				}
			}

			pendingDecorators = pendingDecorators[:0]
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

func hasDecorator(decorators []string, name string) bool {
	target := strings.ToLower(strings.TrimSpace(name))
	for _, decorator := range decorators {
		if strings.EqualFold(decorator, target) {
			return true
		}
	}
	return false
}

func extractFunctionCalls(lines []string, defLineIndex int) []string {
	defIndent := indentationLevel(lines[defLineIndex])
	calls := make([]string, 0, 8)

	for i := defLineIndex + 1; i < len(lines); i++ {
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
