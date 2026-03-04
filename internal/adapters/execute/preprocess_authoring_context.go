package execute

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

var (
	preprocessAuthoringDecoratorPattern = regexp.MustCompile(`^\s*@\s*([A-Za-z_][A-Za-z0-9_\.]+(?:\.[A-Za-z_][A-Za-z0-9_]*)*)`)
	preprocessAuthoringFunctionPattern  = regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(.*\)\s*:`)
)

// BuildPreprocessAuthoringRecommendation builds deterministic preprocessing remediation guidance.
func BuildPreprocessAuthoringRecommendation(snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) (core.AuthoringRecommendation, error) {
	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return core.AuthoringRecommendation{}, core.NewError(
			core.KindUnknown,
			"execute.preprocess_authoring.repo_root",
			"snapshot repository root is empty",
		)
	}

	targetSymbols := discoverPreprocessTargetSymbols(repoRoot, status)

	recommendation := core.AuthoringRecommendation{
		StepID: core.EnsureStepPreprocessContract,
		Constraints: []string{
			"Implement a preprocess contract that returns both train and validation subsets.",
			"When feasible, each requested subset must produce non-empty output values.",
			"Do not refactor unrelated training or business logic.",
		},
		Candidates: targetSymbols,
	}

	if len(targetSymbols) > 0 {
		recommendation.Target = targetSymbols[0]
		recommendation.Rationale = "preprocess symbols discovered from integration entry file"
		return recommendation, nil
	}

	recommendation.Rationale = "add or repair a decorated preprocess function and wire required subset outputs"
	return recommendation, nil
}

func discoverPreprocessTargetSymbols(repoRoot string, status core.IntegrationStatus) []string {
	if status.Contracts != nil {
		symbols := make([]string, 0, len(status.Contracts.PreprocessFunctions))
		for _, symbol := range status.Contracts.PreprocessFunctions {
			symbol = strings.TrimSpace(symbol)
			if symbol == "" {
				continue
			}
			symbols = append(symbols, symbol)
		}
		if sorted := uniqueSortedStrings(symbols); len(sorted) > 0 {
			return sorted
		}
	}

	entryFile := resolvePreprocessAuthoringEntryFile(repoRoot, status.Contracts)
	if entryFile == "" {
		return nil
	}

	source, err := os.ReadFile(entryFile)
	if err != nil {
		return nil
	}

	return discoverDecoratedPreprocessSymbols(string(source))
}

func resolvePreprocessAuthoringEntryFile(repoRoot string, contracts *core.IntegrationContracts) string {
	if contracts != nil {
		entry := strings.TrimSpace(contracts.EntryFile)
		if entry != "" {
			entryAbsPath := filepath.FromSlash(entry)
			if !filepath.IsAbs(entryAbsPath) {
				entryAbsPath = filepath.Join(repoRoot, entryAbsPath)
			}
			entryAbsPath = filepath.Clean(entryAbsPath)
			if isPathWithinRepo(repoRoot, entryAbsPath) && fileExists(entryAbsPath) {
				return entryAbsPath
			}
		}
	}

	for _, candidate := range []string{"leap_binder.py", "leap_custom_test.py", "integration_test.py"} {
		candidatePath := filepath.Join(repoRoot, filepath.FromSlash(candidate))
		candidatePath = filepath.Clean(candidatePath)
		if isPathWithinRepo(repoRoot, candidatePath) && fileExists(candidatePath) {
			return candidatePath
		}
	}
	return ""
}

func discoverDecoratedPreprocessSymbols(source string) []string {
	lines := strings.Split(source, "\n")
	pendingDecorators := make([]string, 0, 4)
	symbols := make([]string, 0, 4)

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "@") {
			decorator, ok := extractPreprocessAuthoringDecorator(line)
			if !ok {
				pendingDecorators = pendingDecorators[:0]
				continue
			}
			pendingDecorators = append(pendingDecorators, decorator)
			continue
		}

		if strings.HasPrefix(line, "def ") {
			name, ok := extractPreprocessAuthoringFunctionName(line)
			if !ok {
				pendingDecorators = pendingDecorators[:0]
				continue
			}

			if hasPreprocessAuthoringDecorator(pendingDecorators) {
				symbols = append(symbols, name)
			}
			pendingDecorators = pendingDecorators[:0]
			continue
		}

		pendingDecorators = pendingDecorators[:0]
	}

	return uniqueSortedStrings(symbols)
}

func extractPreprocessAuthoringDecorator(line string) (string, bool) {
	matches := preprocessAuthoringDecoratorPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	return strings.ToLower(canonicalPreprocessSymbol(matches[1])), true
}

func extractPreprocessAuthoringFunctionName(line string) (string, bool) {
	matches := preprocessAuthoringFunctionPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	return strings.TrimSpace(matches[1]), true
}

func hasPreprocessAuthoringDecorator(decorators []string) bool {
	for _, decorator := range decorators {
		if decorator == "tensorleap_preprocess" {
			return true
		}
	}
	return false
}

func canonicalPreprocessSymbol(value string) string {
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

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
