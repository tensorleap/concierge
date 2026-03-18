package execute

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
	"gopkg.in/yaml.v3"
)

var (
	preprocessAuthoringDecoratorPattern = regexp.MustCompile(`^\s*@\s*([A-Za-z_][A-Za-z0-9_\.]+(?:\.[A-Za-z_][A-Za-z0-9_]*)*)`)
	preprocessAuthoringFunctionPattern  = regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(.*\)\s*:`)
	preprocessDatasetResolverPattern    = regexp.MustCompile(`(?m)^\s*def\s+((?:check|load|resolve|download)_[A-Za-z0-9_]*dataset)\s*\(`)
)

type preprocessDatasetManifestLead struct {
	Path        string
	DatasetPath string
	Train       string
	Val         string
	Download    string
}

type preprocessDatasetResolverLead struct {
	Path   string
	Symbol string
}

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
			"Implement a preprocess function that returns both train and validation subsets.",
			"When feasible, each requested subset must produce non-empty output values.",
			"Prefer existing repository dataset configuration, helper code, and project metadata before inventing new dataset roots or sample conventions.",
			"Prefer repo-local dataset and Tensorleap integration configuration over hand-coded installed-package defaults or home-directory settings.",
			"If the repository already declares train/validation subsets in a dataset manifest, reuse those declared subsets instead of inventing a new split from arbitrary images.",
			"If the repository includes explicit dataset manifests, loader code, or Tensorleap integration examples, treat those as stronger evidence than arbitrary image files.",
			"If the repository exposes a supported dataset resolver or downloader, prefer that helper over hard-coded cache roots or generic image scans.",
			"Smoke-test any repository dataset resolver before wiring it into preprocess; if the helper import fails in the current repo state, fall back to manifest-driven resolution/download instead of keeping a broken import.",
			"If a repo helper import fails because project dependencies are missing, do not reverse-engineer internal cache constants or framework settings paths; use explicit manifest train/val/download evidence or stop with the blocker.",
			"If a prepared repository runtime interpreter is available, use that interpreter for Python repo checks instead of bare python/python3, and treat failures under the wrong interpreter as environment mismatch evidence rather than dataset-path evidence.",
			"Do not run pip install, poetry add, or other environment mutation commands while discovering dataset paths for preprocess; if discovery depends on missing packages, stop and surface that blocker.",
			"Do not set deprecated `PreprocessResponse.length`; provide real `sample_ids` for each subset and let Tensorleap derive lengths from them.",
			"Do not hard-code home-directory dataset defaults, installed-package cache roots, or new environment-variable paths unless repository evidence requires them and the repository itself uses them.",
			"Do not fabricate placeholder sample IDs, dummy image paths, or guessed absolute dataset locations just to satisfy subset requirements.",
			"Do not repurpose generic repository assets, screenshots, docs media, or example images as train/validation data unless repository evidence explicitly identifies them as the real dataset.",
			"If repository evidence does not expose real train/validation identifiers and no repo-supported acquisition path exists, stop and surface the missing data requirement instead of guessing.",
			"Do not refactor unrelated training or business logic.",
		},
		Candidates: targetSymbols,
	}
	if evidencePaths := discoverPreprocessRepositoryEvidence(snapshot); len(evidencePaths) > 0 {
		recommendation.Constraints = append(
			recommendation.Constraints,
			fmt.Sprintf(
				"Inspect and reuse existing repository Tensorleap or dataset evidence before inventing new preprocess structure: %s",
				strings.Join(evidencePaths, ", "),
			),
		)
	}
	if manifestLeads := discoverPreprocessDatasetManifestLeads(snapshot); len(manifestLeads) > 0 {
		formatted := make([]string, 0, len(manifestLeads))
		for _, lead := range manifestLeads {
			formatted = append(formatted, formatPreprocessDatasetManifestLead(lead))
		}
		recommendation.Constraints = append(
			recommendation.Constraints,
			fmt.Sprintf(
				"Repository dataset manifests with explicit train/validation structure are available and should be preferred over ad hoc image scans: %s",
				strings.Join(formatted, ", "),
			),
		)
	}
	if resolverLeads := discoverPreprocessDatasetResolverLeads(snapshot); len(resolverLeads) > 0 {
		formatted := make([]string, 0, len(resolverLeads))
		for _, lead := range resolverLeads {
			formatted = append(formatted, fmt.Sprintf("%s:%s", lead.Path, lead.Symbol))
		}
		recommendation.Constraints = append(
			recommendation.Constraints,
			fmt.Sprintf(
				"Repository dataset resolver helpers are available and should be preferred over hand-coded cache paths or generic asset scans: %s",
				strings.Join(formatted, ", "),
			),
		)
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

func discoverPreprocessRepositoryEvidence(snapshot core.WorkspaceSnapshot) []string {
	integrationDocs := make([]string, 0, 4)
	primaryIntegrationCode := make([]string, 0, 4)
	secondaryIntegrationCode := make([]string, 0, 4)
	configFiles := make([]string, 0, 4)
	datasetLoaderCode := make([]string, 0, 4)
	preferredDatasetFiles := make([]string, 0, 4)
	otherDatasetFiles := make([]string, 0, 4)

	appendEvidencePath := func(path string) {
		normalized := normalizeRepoContextPath(path)
		if normalized == "" {
			return
		}
		lower := strings.ToLower(normalized)
		base := strings.ToLower(filepath.Base(normalized))

		switch {
		case strings.Contains(lower, "/tensorleap_folder/"),
			strings.Contains(lower, "/tensorleap/"),
			strings.Contains(lower, "/.tensorleap/"):
			switch base {
			case "readme.md":
				integrationDocs = append(integrationDocs, normalized)
			case "leap_integration.py", "leap_binder.py", "leap_custom_test.py", "leap.yaml":
				primaryIntegrationCode = append(primaryIntegrationCode, normalized)
			default:
				if strings.HasSuffix(lower, ".py") {
					secondaryIntegrationCode = append(secondaryIntegrationCode, normalized)
				}
			}
		case strings.HasSuffix(lower, "/cfg/default.yaml"),
			base == "project_config.yaml":
			configFiles = append(configFiles, normalized)
		case isPreprocessDatasetLoaderPath(lower):
			datasetLoaderCode = append(datasetLoaderCode, normalized)
		case strings.Contains(lower, "/cfg/datasets/") && strings.HasSuffix(lower, ".yaml"),
			strings.Contains(lower, "/datasets/") && strings.HasSuffix(lower, ".yaml"),
			strings.Contains(base, "dataset") && strings.HasSuffix(lower, ".yaml"):
			if isPreferredDatasetEvidence(base) {
				preferredDatasetFiles = append(preferredDatasetFiles, normalized)
			} else {
				otherDatasetFiles = append(otherDatasetFiles, normalized)
			}
		}
	}

	for path := range snapshot.FileHashes {
		appendEvidencePath(path)
	}

	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot != "" {
		visitPreprocessRepositoryFiles(repoRoot, func(relPath, _ string) error {
			appendEvidencePath(relPath)
			return nil
		})
	}

	evidence := make([]string, 0, 6)
	evidence = append(evidence, uniqueSortedStrings(integrationDocs)...)
	evidence = append(evidence, uniqueSortedStrings(primaryIntegrationCode)...)
	evidence = append(evidence, uniqueSortedStrings(configFiles)...)
	evidence = append(evidence, uniqueSortedStrings(datasetLoaderCode)...)
	evidence = append(evidence, uniqueSortedStrings(preferredDatasetFiles)...)
	evidence = append(evidence, uniqueSortedStrings(secondaryIntegrationCode)...)
	evidence = append(evidence, uniqueSortedStrings(otherDatasetFiles)...)
	return truncateRepoContextValues(uniqueOrderedStrings(evidence), 6)
}

func discoverPreprocessDatasetManifestLeads(snapshot core.WorkspaceSnapshot) []preprocessDatasetManifestLead {
	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return nil
	}

	projectTask := discoverPreprocessProjectTask(repoRoot)
	leads := make([]preprocessDatasetManifestLead, 0, 4)
	_ = visitPreprocessRepositoryFiles(repoRoot, func(relPath, absPath string) error {
		lead, ok := parsePreprocessDatasetManifestLead(relPath, absPath)
		if !ok {
			return nil
		}
		leads = append(leads, lead)
		return nil
	})
	if len(leads) == 0 {
		return nil
	}

	sort.Slice(leads, func(i, j int) bool {
		leftRank := rankPreprocessDatasetManifestLead(leads[i], projectTask)
		rightRank := rankPreprocessDatasetManifestLead(leads[j], projectTask)
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		leftPath := strings.ToLower(leads[i].Path)
		rightPath := strings.ToLower(leads[j].Path)
		if leftPath != rightPath {
			return leftPath < rightPath
		}
		return leads[i].Path < leads[j].Path
	})
	return truncatePreprocessDatasetManifestLeads(leads, 3)
}

func discoverPreprocessDatasetResolverLeads(snapshot core.WorkspaceSnapshot) []preprocessDatasetResolverLead {
	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return nil
	}

	leads := make([]preprocessDatasetResolverLead, 0, 4)
	_ = visitPreprocessRepositoryFiles(repoRoot, func(relPath, absPath string) error {
		lower := strings.ToLower(relPath)
		if !isPreprocessDatasetResolverPath(lower) {
			return nil
		}

		source, err := os.ReadFile(absPath)
		if err != nil || len(source) == 0 || len(source) > 512*1024 {
			return nil
		}
		matches := preprocessDatasetResolverPattern.FindAllStringSubmatch(string(source), -1)
		for _, match := range matches {
			symbol := strings.TrimSpace(match[1])
			if symbol == "" {
				continue
			}
			leads = append(leads, preprocessDatasetResolverLead{
				Path:   normalizeRepoContextPath(relPath),
				Symbol: symbol,
			})
		}
		return nil
	})
	if len(leads) == 0 {
		return nil
	}

	sort.Slice(leads, func(i, j int) bool {
		leftRank := rankPreprocessDatasetResolverLead(leads[i])
		rightRank := rankPreprocessDatasetResolverLead(leads[j])
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		leftKey := strings.ToLower(leads[i].Path + ":" + leads[i].Symbol)
		rightKey := strings.ToLower(leads[j].Path + ":" + leads[j].Symbol)
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		return leads[i].Path+":"+leads[i].Symbol < leads[j].Path+":"+leads[j].Symbol
	})
	return truncatePreprocessDatasetResolverLeads(leads, 3)
}

func isPreferredDatasetEvidence(base string) bool {
	base = strings.ToLower(strings.TrimSpace(base))
	switch {
	case strings.Contains(base, "coco8"),
		strings.Contains(base, "coco128"),
		strings.Contains(base, "example"),
		strings.Contains(base, "sample"),
		strings.Contains(base, "demo"),
		strings.Contains(base, "mini"),
		strings.Contains(base, "tiny"):
		return true
	default:
		return false
	}
}

func isPreprocessDatasetLoaderPath(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	if !strings.HasSuffix(path, ".py") {
		return false
	}
	switch {
	case strings.HasSuffix(path, "/data/utils.py"),
		strings.HasSuffix(path, "/data/build.py"),
		strings.HasSuffix(path, "/data/dataset.py"),
		strings.HasSuffix(path, "/data/loaders.py"),
		strings.Contains(path, "/datasets/"),
		strings.Contains(path, "loader"):
		return true
	default:
		return false
	}
}

func isPreprocessDatasetResolverPath(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	if !strings.HasSuffix(path, ".py") {
		return false
	}
	switch {
	case strings.HasSuffix(path, "/data/utils.py"),
		strings.HasSuffix(path, "/data/build.py"),
		strings.HasSuffix(path, "/data/dataset.py"),
		strings.HasSuffix(path, "/data/loaders.py"),
		strings.Contains(path, "dataset"),
		strings.Contains(path, "loader"):
		return true
	default:
		return false
	}
}

func visitPreprocessRepositoryFiles(repoRoot string, visit func(relPath, absPath string) error) error {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil
	}

	return filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := strings.ToLower(d.Name())
		if d.IsDir() {
			switch name {
			case ".git", ".venv", ".concierge", "node_modules", "__pycache__":
				return filepath.SkipDir
			}
			return nil
		}
		relPath, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		if visitErr := visit(relPath, path); visitErr != nil {
			return visitErr
		}
		return nil
	})
}

func discoverPreprocessProjectTask(repoRoot string) string {
	projectConfigPath := filepath.Join(repoRoot, "project_config.yaml")
	source, err := os.ReadFile(projectConfigPath)
	if err != nil || len(source) == 0 {
		return ""
	}

	var document map[string]any
	if err := yaml.Unmarshal(source, &document); err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(extractPreprocessManifestValue(document["task"])))
}

func parsePreprocessDatasetManifestLead(relPath, absPath string) (preprocessDatasetManifestLead, bool) {
	lower := strings.ToLower(strings.TrimSpace(relPath))
	if !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml") {
		return preprocessDatasetManifestLead{}, false
	}

	source, err := os.ReadFile(absPath)
	if err != nil || len(source) == 0 || len(source) > 512*1024 {
		return preprocessDatasetManifestLead{}, false
	}

	var document map[string]any
	if err := yaml.Unmarshal(source, &document); err != nil {
		return preprocessDatasetManifestLead{}, false
	}

	train := extractPreprocessManifestValue(document["train"])
	val := extractPreprocessManifestValue(document["val"])
	if train == "" || val == "" {
		return preprocessDatasetManifestLead{}, false
	}

	lead := preprocessDatasetManifestLead{
		Path:        normalizeRepoContextPath(relPath),
		DatasetPath: extractPreprocessManifestValue(document["path"]),
		Train:       train,
		Val:         val,
		Download:    extractPreprocessManifestValue(document["download"]),
	}
	if lead.Path == "" {
		return preprocessDatasetManifestLead{}, false
	}
	return lead, true
}

func extractPreprocessManifestValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return summarizePreprocessManifestSnippet(typed)
	case []string:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if normalized := summarizePreprocessManifestSnippet(item); normalized != "" {
				parts = append(parts, normalized)
			}
		}
		if len(parts) == 0 {
			return ""
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if normalized := extractPreprocessManifestValue(item); normalized != "" {
				parts = append(parts, normalized)
			}
		}
		if len(parts) == 0 {
			return ""
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return summarizePreprocessManifestSnippet(fmt.Sprint(typed))
	}
}

func summarizePreprocessManifestSnippet(value string) string {
	joined := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if joined == "" {
		return ""
	}
	const maxLen = 96
	if len(joined) <= maxLen {
		return joined
	}
	return joined[:maxLen-3] + "..."
}

func rankPreprocessDatasetManifestLead(lead preprocessDatasetManifestLead, projectTask string) int {
	score := 0
	lowerPath := strings.ToLower(strings.TrimSpace(lead.Path))
	base := strings.ToLower(filepath.Base(lowerPath))

	if strings.Contains(lowerPath, "/cfg/datasets/") {
		score += 6
	}
	if isPreferredDatasetEvidence(base) {
		score += 5
	}
	if lead.Download != "" {
		score += 3
	}
	if lead.DatasetPath != "" {
		score++
	}
	if strings.EqualFold(strings.TrimSpace(lead.Train), strings.TrimSpace(lead.Val)) {
		score--
	} else if lead.Train != "" && lead.Val != "" {
		score += 2
	}
	switch strings.TrimSpace(projectTask) {
	case "detect":
		if strings.Contains(base, "pose") || strings.Contains(base, "seg") || strings.Contains(base, "obb") || strings.Contains(base, "cls") {
			score -= 2
		} else {
			score++
		}
	case "pose":
		if strings.Contains(base, "pose") {
			score += 3
		}
	case "segment":
		if strings.Contains(base, "seg") {
			score += 3
		}
	case "classify":
		if strings.Contains(base, "cls") || strings.Contains(base, "imagenet") {
			score += 3
		}
	}
	return score
}

func formatPreprocessDatasetManifestLead(lead preprocessDatasetManifestLead) string {
	parts := make([]string, 0, 4)
	if lead.DatasetPath != "" {
		parts = append(parts, "path="+lead.DatasetPath)
	}
	if lead.Train != "" {
		parts = append(parts, "train="+lead.Train)
	}
	if lead.Val != "" {
		parts = append(parts, "val="+lead.Val)
	}
	if lead.Download != "" {
		parts = append(parts, "download="+lead.Download)
	}
	if len(parts) == 0 {
		return lead.Path
	}
	return fmt.Sprintf("%s (%s)", lead.Path, strings.Join(parts, "; "))
}

func truncatePreprocessDatasetManifestLeads(leads []preprocessDatasetManifestLead, limit int) []preprocessDatasetManifestLead {
	if len(leads) == 0 || limit <= 0 {
		return nil
	}

	seen := map[string]struct{}{}
	unique := make([]preprocessDatasetManifestLead, 0, len(leads))
	for _, lead := range leads {
		key := strings.ToLower(strings.TrimSpace(lead.Path))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, lead)
		if len(unique) == limit {
			break
		}
	}
	if len(unique) == 0 {
		return nil
	}
	return unique
}

func rankPreprocessDatasetResolverLead(lead preprocessDatasetResolverLead) int {
	score := 0
	lowerPath := strings.ToLower(strings.TrimSpace(lead.Path))
	lowerSymbol := strings.ToLower(strings.TrimSpace(lead.Symbol))

	switch {
	case strings.HasSuffix(lowerPath, "/data/utils.py"):
		score += 4
	case strings.HasSuffix(lowerPath, "/data/build.py"),
		strings.HasSuffix(lowerPath, "/data/dataset.py"),
		strings.HasSuffix(lowerPath, "/data/loaders.py"):
		score += 3
	}
	switch {
	case strings.HasPrefix(lowerSymbol, "check_"):
		score += 4
	case strings.HasPrefix(lowerSymbol, "load_"):
		score += 3
	case strings.HasPrefix(lowerSymbol, "resolve_"),
		strings.HasPrefix(lowerSymbol, "download_"):
		score += 2
	}
	return score
}

func truncatePreprocessDatasetResolverLeads(leads []preprocessDatasetResolverLead, limit int) []preprocessDatasetResolverLead {
	if len(leads) == 0 || limit <= 0 {
		return nil
	}

	seen := map[string]struct{}{}
	unique := make([]preprocessDatasetResolverLead, 0, len(leads))
	for _, lead := range leads {
		key := strings.ToLower(strings.TrimSpace(lead.Path + ":" + lead.Symbol))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, lead)
		if len(unique) == limit {
			break
		}
	}
	if len(unique) == 0 {
		return nil
	}
	return unique
}

func uniqueOrderedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	ordered := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ordered = append(ordered, trimmed)
	}
	if len(ordered) == 0 {
		return nil
	}
	return ordered
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

	candidatePath := filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile)
	candidatePath = filepath.Clean(candidatePath)
	if isPathWithinRepo(repoRoot, candidatePath) && fileExists(candidatePath) {
		return candidatePath
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
