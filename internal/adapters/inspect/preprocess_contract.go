package inspect

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

var (
	functionSignaturePattern = regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\((.*)\)\s*(?:->\s*[^:]+)?\s*:`)
	returnStatementPattern   = regexp.MustCompile(`^\s*return(?:\s+(.*))?$`)
	numberLiteralPattern     = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)$`)
)

type pythonFunctionDefinition struct {
	Name       string
	Parameters string
	Line       int
	Body       []pythonFunctionBodyLine
}

type pythonFunctionBodyLine struct {
	Line int
	Text string
}

func inspectPreprocessContract(entryFilePath string, source string, contracts *core.IntegrationContracts, status *core.IntegrationStatus) {
	if contracts == nil || status == nil {
		return
	}

	preprocessFunctions := uniqueSymbols(contracts.PreprocessFunctions)
	if len(preprocessFunctions) == 0 {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodePreprocessFunctionMissing,
			Message:  fmt.Sprintf("no @tensorleap_preprocess function found in %s", entryFilePath),
			Severity: core.SeverityError,
			Scope:    core.IssueScopePreprocess,
			Location: &core.IssueLocation{
				Path:   entryFilePath,
				Symbol: "preprocess",
			},
		})
		return
	}

	definitions := discoverPythonFunctionDefinitions(source)
	for _, functionName := range preprocessFunctions {
		definition, ok := definitions[functionName]
		if !ok {
			continue
		}

		if strings.TrimSpace(definition.Parameters) != "" {
			status.Issues = append(status.Issues, core.Issue{
				Code:     core.IssueCodePreprocessResponseInvalid,
				Message:  fmt.Sprintf("@tensorleap_preprocess function %q must not accept parameters", definition.Name),
				Severity: core.SeverityError,
				Scope:    core.IssueScopePreprocess,
				Location: &core.IssueLocation{
					Path:   entryFilePath,
					Line:   definition.Line,
					Symbol: definition.Name,
				},
			})
			continue
		}

		if issue, ok := invalidPreprocessReturnIssue(entryFilePath, definition); ok {
			status.Issues = append(status.Issues, issue)
		}
	}
}

func uniqueSymbols(symbols []string) []string {
	unique := make([]string, 0, len(symbols))
	seen := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		trimmed := strings.TrimSpace(symbol)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		unique = append(unique, trimmed)
	}
	return unique
}

func discoverPythonFunctionDefinitions(source string) map[string]pythonFunctionDefinition {
	lines := strings.Split(source, "\n")
	definitions := make(map[string]pythonFunctionDefinition)

	for index, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(trimmed, "def ") {
			continue
		}

		name, parameters, ok := extractFunctionSignature(rawLine)
		if !ok {
			continue
		}
		if _, exists := definitions[name]; exists {
			continue
		}

		definitions[name] = pythonFunctionDefinition{
			Name:       name,
			Parameters: parameters,
			Line:       index + 1,
			Body:       extractFunctionBody(lines, index),
		}
	}

	return definitions
}

func extractFunctionSignature(line string) (string, string, bool) {
	matches := functionSignaturePattern.FindStringSubmatch(line)
	if len(matches) != 3 {
		return "", "", false
	}
	return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2]), true
}

func extractFunctionBody(lines []string, defLineIndex int) []pythonFunctionBodyLine {
	defIndent := indentationLevel(lines[defLineIndex])
	body := make([]pythonFunctionBodyLine, 0, 8)

	for i := defLineIndex + 1; i < len(lines); i++ {
		rawLine := lines[i]
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if indentationLevel(rawLine) <= defIndent {
			break
		}

		body = append(body, pythonFunctionBodyLine{
			Line: i + 1,
			Text: line,
		})
	}

	return body
}

func invalidPreprocessReturnIssue(entryFilePath string, definition pythonFunctionDefinition) (core.Issue, bool) {
	for _, line := range definition.Body {
		returnExpression, ok := extractReturnExpression(line.Text)
		if !ok {
			continue
		}

		if issue, invalid := preprocessInvalidReturnExpressionIssue(entryFilePath, definition.Name, line.Line, returnExpression); invalid {
			return issue, true
		}
		return core.Issue{}, false
	}

	return core.Issue{
		Code:     core.IssueCodePreprocessResponseInvalid,
		Message:  fmt.Sprintf("@tensorleap_preprocess function %q must return a list of PreprocessResponse values", definition.Name),
		Severity: core.SeverityError,
		Scope:    core.IssueScopePreprocess,
		Location: &core.IssueLocation{
			Path:   entryFilePath,
			Line:   definition.Line,
			Symbol: definition.Name,
		},
	}, true
}

func extractReturnExpression(line string) (string, bool) {
	matches := returnStatementPattern.FindStringSubmatch(line)
	if len(matches) == 0 {
		return "", false
	}
	if len(matches) < 2 {
		return "", true
	}

	return strings.TrimSpace(stripInlinePythonComment(matches[1])), true
}

func preprocessInvalidReturnExpressionIssue(entryFilePath string, functionName string, line int, expression string) (core.Issue, bool) {
	if !isObviouslyInvalidPreprocessReturnExpression(expression) {
		return core.Issue{}, false
	}

	message := fmt.Sprintf("@tensorleap_preprocess function %q returns %q; expected a list of PreprocessResponse values", functionName, expression)
	if strings.TrimSpace(expression) == "" {
		message = fmt.Sprintf("@tensorleap_preprocess function %q has a bare return; expected a list of PreprocessResponse values", functionName)
	}

	return core.Issue{
		Code:     core.IssueCodePreprocessResponseInvalid,
		Message:  message,
		Severity: core.SeverityError,
		Scope:    core.IssueScopePreprocess,
		Location: &core.IssueLocation{
			Path:   entryFilePath,
			Line:   line,
			Symbol: functionName,
		},
	}, true
}

func isObviouslyInvalidPreprocessReturnExpression(expression string) bool {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return true
	}

	lower := strings.ToLower(trimmed)
	switch lower {
	case "none", "true", "false":
		return true
	}

	if numberLiteralPattern.MatchString(trimmed) {
		return true
	}
	if isQuotedStringLiteral(trimmed) {
		return true
	}
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		return true
	}
	if strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") && strings.Contains(trimmed, ",") {
		return true
	}

	return false
}

func isQuotedStringLiteral(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 2 {
		return false
	}

	first := trimmed[0]
	last := trimmed[len(trimmed)-1]
	if (first == '\'' || first == '"') && first == last {
		return true
	}

	return false
}

func stripInlinePythonComment(value string) string {
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for i, char := range value {
		if escaped {
			escaped = false
			continue
		}

		if char == '\\' {
			escaped = true
			continue
		}

		if char == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if char == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}

		if char == '#' && !inSingleQuote && !inDoubleQuote {
			return value[:i]
		}
	}

	return value
}
