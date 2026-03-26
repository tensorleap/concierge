package gitmanager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

const conciergePathExclude = ":(exclude).concierge/**"

var (
	diffStatFilesPattern      = regexp.MustCompile(`(\d+)\s+files?\s+changed`)
	diffStatInsertionsPattern = regexp.MustCompile(`(\d+)\s+insertions?\(\+\)`)
	diffStatDeletionsPattern  = regexp.MustCompile(`(\d+)\s+deletions?\(-\)`)
)

// ApprovalFunc decides whether to approve committing the current diff.
type ApprovalFunc func(step core.EnsureStep, review ChangeReview) (ReviewDecision, error)

// ReviewDecision captures what to do with an applied working-tree diff after review.
type ReviewDecision struct {
	KeepChanges bool
	Commit      bool
}

// ManagerOptions controls review behavior.
type ManagerOptions struct {
	ColorDiff bool
}

// Manager enforces branch safety and audited commit/reject flow.
type Manager struct {
	approve    ApprovalFunc
	runGit     func(ctx context.Context, dir string, args ...string) (string, error)
	removePath func(path string) error
	options    ManagerOptions
}

// NewManager creates a git manager with approval callback.
func NewManager(approve ApprovalFunc, opts ...ManagerOptions) *Manager {
	if approve == nil {
		approve = func(step core.EnsureStep, review ChangeReview) (ReviewDecision, error) {
			_ = step
			_ = review
			return ReviewDecision{}, nil
		}
	}

	options := ManagerOptions{ColorDiff: true}
	if len(opts) > 0 {
		options = opts[0]
	}

	return &Manager{
		approve:    approve,
		runGit:     runGitCombined,
		removePath: os.RemoveAll,
		options:    options,
	}
}

// Handle executes diff review, branch guard, and approved commit/reject restoration.
func (m *Manager) Handle(ctx context.Context, snapshot core.WorkspaceSnapshot, result core.ExecutionResult) (core.GitDecision, error) {
	decision := core.GitDecision{FinalResult: result}
	if !result.Applied {
		return decision, nil
	}

	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return core.GitDecision{}, core.NewError(core.KindUnknown, "gitmanager.handle.repo_root", "snapshot repository root is empty")
	}

	statusPorcelain, err := m.runGit(ctx, repoRoot, "status", "--porcelain", "--", ".", conciergePathExclude)
	if err != nil {
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.status", err)
	}
	if strings.TrimSpace(statusPorcelain) == "" {
		return decision, nil
	}

	branch, err := m.runGit(ctx, repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.branch", err)
	}
	branch = strings.TrimSpace(branch)
	if branch == "main" || branch == "master" {
		return core.GitDecision{}, core.NewError(
			core.KindUnknown,
			"gitmanager.handle.protected_branch",
			"refusing to commit on protected branch main/master",
		)
	}

	review, err := m.buildChangeReview(ctx, repoRoot, result.Step, statusPorcelain)
	if err != nil {
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.review", err)
	}

	if review.Risk.IsRisky() {
		decision.Evidence = append(decision.Evidence,
			core.EvidenceItem{Name: "git.review_risk", Value: strings.TrimSpace(review.Risk.Level)},
			core.EvidenceItem{Name: "git.review_risk_summary", Value: strings.TrimSpace(review.Risk.Summary)},
		)
		if len(review.Risk.Reasons) > 0 {
			decision.Evidence = append(decision.Evidence, core.EvidenceItem{
				Name:  "git.review_risk_reasons",
				Value: strings.Join(review.Risk.Reasons, "\n"),
			})
		}
	}

	if review.Risk.Block {
		if err := m.restoreWorkingTree(ctx, repoRoot); err != nil {
			return core.GitDecision{}, err
		}
		decision.FinalResult = core.ExecutionResult{
			Step:     result.Step,
			Applied:  false,
			Summary:  "risky artifact-only remediation blocked and restored",
			Evidence: append([]core.EvidenceItem(nil), result.Evidence...),
		}
		decision.Notes = append(decision.Notes,
			"Concierge blocked this change because it only vendored dataset/cache artifacts into the repository working tree.",
			"Resolve the dataset or cache requirement as external runtime state, then rerun `concierge run`.",
		)
		decision.Evidence = append(decision.Evidence,
			core.EvidenceItem{Name: "git.review_action", Value: "blocked_risky_artifacts"},
		)
		return decision, nil
	}

	reviewDecision, err := m.approve(result.Step, review)
	if err != nil {
		if restoreErr := m.restoreWorkingTree(ctx, repoRoot); restoreErr != nil {
			return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.approval_restore", restoreErr)
		}
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.approval", err)
	}

	decision.Evidence = append(decision.Evidence,
		core.EvidenceItem{Name: "git.diff_summary", Value: strings.TrimSpace(review.Stat)},
	)
	if len(review.Files) > 0 {
		decision.Evidence = append(decision.Evidence, core.EvidenceItem{
			Name:  "git.changed_files",
			Value: strings.Join(review.Files, "\n"),
		})
	}
	if strings.TrimSpace(review.Patch) != "" {
		decision.Evidence = append(decision.Evidence, core.EvidenceItem{
			Name:  "git.patch_available",
			Value: "true",
		})
	}

	if !reviewDecision.KeepChanges {
		if err := m.restoreWorkingTree(ctx, repoRoot); err != nil {
			return core.GitDecision{}, err
		}
		decision.FinalResult = core.ExecutionResult{
			Step:     result.Step,
			Applied:  false,
			Summary:  "changes rejected and restored",
			Evidence: append([]core.EvidenceItem(nil), result.Evidence...),
		}
		decision.Notes = append(decision.Notes, fmt.Sprintf("changes for %s were rejected and reverted", result.Step.ID))
		decision.Evidence = append(decision.Evidence,
			core.EvidenceItem{Name: "git.approval", Value: "rejected"},
			core.EvidenceItem{Name: "git.review_action", Value: "reverted"},
		)
		return decision, nil
	}

	if !reviewDecision.Commit {
		decision.Notes = append(decision.Notes, "changes kept in your working tree for local review; no commit was created")
		decision.Evidence = append(decision.Evidence,
			core.EvidenceItem{Name: "git.approval", Value: "review_pending"},
			core.EvidenceItem{Name: "git.review_action", Value: "kept_uncommitted"},
			core.EvidenceItem{Name: "git.commit_pending_review", Value: "true"},
		)
		return decision, nil
	}

	message := CommitMessage(result.Step, result.Summary)
	if _, err := m.runGit(ctx, repoRoot, "add", "-A", "--", "."); err != nil {
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.add", err)
	}
	if _, err := m.runGit(ctx, repoRoot, "reset", "--quiet", "--", ".concierge"); err != nil {
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.reset_concierge", err)
	}
	if _, err := m.runGit(ctx, repoRoot, "commit", "-m", message); err != nil {
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.commit", err)
	}

	hash, err := m.runGit(ctx, repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return core.GitDecision{}, core.WrapError(core.KindUnknown, "gitmanager.handle.commit_hash", err)
	}
	hash = strings.TrimSpace(hash)

	decision.Commit = &core.CommitMetadata{Hash: hash, Message: message}
	decision.Notes = append(decision.Notes, commitBranchNote(branch))
	decision.Evidence = append(decision.Evidence,
		core.EvidenceItem{Name: "git.approval", Value: "approved"},
		core.EvidenceItem{Name: "git.review_action", Value: "committed"},
		core.EvidenceItem{Name: "git.commit_hash", Value: hash},
	)

	return decision, nil
}

func (m *Manager) restoreWorkingTree(ctx context.Context, repoRoot string) error {
	if _, err := m.runGit(ctx, repoRoot, "restore", "--staged", "--worktree", "--", "."); err != nil {
		return core.WrapError(core.KindUnknown, "gitmanager.restore.restore", err)
	}

	untracked, err := m.runGit(ctx, repoRoot, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return core.WrapError(core.KindUnknown, "gitmanager.restore.untracked", err)
	}

	for _, relativePath := range strings.Split(strings.TrimSpace(untracked), "\n") {
		rel := strings.TrimSpace(relativePath)
		if rel == "" {
			continue
		}
		if isConciergeInternalPath(rel) {
			continue
		}
		cleanRel := filepath.Clean(rel)
		if cleanRel == "." || strings.HasPrefix(cleanRel, "..") {
			continue
		}
		if err := m.removePath(filepath.Join(repoRoot, cleanRel)); err != nil {
			return core.WrapError(core.KindUnknown, "gitmanager.restore.remove_path", err)
		}
	}

	return nil
}

func commitBranchNote(branch string) string {
	if strings.TrimSpace(branch) == "" || strings.TrimSpace(branch) == "HEAD" {
		return "changes committed in detached HEAD state"
	}
	return fmt.Sprintf("changes committed on branch %s", strings.TrimSpace(branch))
}

func (m *Manager) buildChangeReview(
	ctx context.Context,
	repoRoot string,
	step core.EnsureStep,
	statusPorcelain string,
) (ChangeReview, error) {
	review := ChangeReview{
		Focus: ReviewFocus(step),
	}

	review.Files = m.collectChangedFiles(ctx, repoRoot, statusPorcelain)
	review.Stat = m.collectDiffStat(ctx, repoRoot, statusPorcelain)

	patch, err := m.collectPatch(ctx, repoRoot)
	if err != nil {
		return ChangeReview{}, err
	}
	review.Patch = strings.TrimSpace(patch)
	review.Risk = classifyReviewRisk(step, review.Files, review.Stat)

	return review, nil
}

func (m *Manager) collectChangedFiles(ctx context.Context, repoRoot string, fallback string) []string {
	merged := make([]string, 0, 8)
	seen := make(map[string]struct{})

	appendUnique := func(line string) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
		merged = append(merged, trimmed)
	}

	nameStatus, err := m.runGit(ctx, repoRoot, "diff", "--name-status", "--", ".", conciergePathExclude)
	if err == nil {
		for _, line := range splitNonEmptyLines(nameStatus) {
			appendUnique(line)
		}
	}

	untracked, err := m.runGit(ctx, repoRoot, "ls-files", "--others", "--exclude-standard")
	if err == nil {
		for _, file := range splitNonEmptyLines(untracked) {
			if isConciergeInternalPath(file) {
				continue
			}
			appendUnique("A\t" + file)
		}
	}

	if len(merged) == 0 {
		for _, line := range splitNonEmptyLines(fallback) {
			appendUnique(line)
		}
	}

	return merged
}

func (m *Manager) collectDiffStat(ctx context.Context, repoRoot string, fallback string) string {
	summary := diffStatSummary{}

	trackedStat, err := m.runGit(ctx, repoRoot, "diff", "--stat", "--", ".", conciergePathExclude)
	if err == nil {
		summary = mergeDiffStatSummary(summary, parseDiffStatSummary(trackedStat))
	}

	untracked, err := m.runGit(ctx, repoRoot, "ls-files", "--others", "--exclude-standard")
	if err == nil {
		for _, relativePath := range splitNonEmptyLines(untracked) {
			if isConciergeInternalPath(relativePath) {
				continue
			}
			stat, statErr := diffUntrackedStat(ctx, repoRoot, relativePath)
			if statErr != nil {
				continue
			}
			summary = mergeDiffStatSummary(summary, parseDiffStatSummary(stat))
		}
	}

	rendered := strings.TrimSpace(renderDiffStatSummary(summary))
	if rendered != "" {
		return rendered
	}
	return strings.TrimSpace(fallback)
}

func (m *Manager) collectPatch(ctx context.Context, repoRoot string) (string, error) {
	args := []string{"diff", "--patch", "--minimal", "--", ".", conciergePathExclude}
	if m.options.ColorDiff {
		args = append([]string{"-c", "color.ui=always"}, args...)
	} else {
		args = append([]string{"-c", "color.ui=never"}, args...)
	}

	sections := make([]string, 0, 4)
	trackedPatch, err := m.runGit(ctx, repoRoot, args...)
	if err == nil && strings.TrimSpace(trackedPatch) != "" {
		sections = append(sections, strings.TrimSpace(trackedPatch))
	}

	untracked, err := m.runGit(ctx, repoRoot, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return strings.Join(sections, "\n\n"), nil
	}

	for _, relativePath := range splitNonEmptyLines(untracked) {
		if isConciergeInternalPath(relativePath) {
			continue
		}
		patch, patchErr := diffUntrackedPath(ctx, repoRoot, relativePath, m.options.ColorDiff)
		if patchErr != nil {
			return "", patchErr
		}
		if strings.TrimSpace(patch) == "" {
			continue
		}
		sections = append(sections, strings.TrimSpace(patch))
	}

	return strings.Join(sections, "\n\n"), nil
}

func diffUntrackedStat(ctx context.Context, repoRoot string, relativePath string) (string, error) {
	rel := filepath.Clean(strings.TrimSpace(relativePath))
	if rel == "" || rel == "." || strings.HasPrefix(rel, "..") {
		return "", nil
	}
	if isConciergeInternalPath(rel) {
		return "", nil
	}
	target := filepath.Join(repoRoot, rel)
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", core.WrapError(core.KindUnknown, "gitmanager.diff_untracked_stat.stat", err)
	}
	if info.IsDir() {
		return "", nil
	}

	cmd := exec.CommandContext(ctx, "git", "diff", "--stat", "--no-index", "--", "/dev/null", rel)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err == nil {
		return trimmed, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return trimmed, nil
	}
	if trimmed == "" {
		return "", core.WrapError(core.KindUnknown, "gitmanager.diff_untracked_stat.run", err)
	}
	return "", core.NewError(core.KindUnknown, "gitmanager.diff_untracked_stat.run", fmt.Sprintf("git diff --stat --no-index failed: %s", trimmed))
}

func diffUntrackedPath(ctx context.Context, repoRoot string, relativePath string, colorDiff bool) (string, error) {
	rel := filepath.Clean(strings.TrimSpace(relativePath))
	if rel == "" || rel == "." || strings.HasPrefix(rel, "..") {
		return "", nil
	}
	if isConciergeInternalPath(rel) {
		return "", nil
	}
	target := filepath.Join(repoRoot, rel)
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", core.WrapError(core.KindUnknown, "gitmanager.diff_untracked.stat", err)
	}
	if info.IsDir() {
		return "", nil
	}

	args := make([]string, 0, 9)
	if colorDiff {
		args = append(args, "-c", "color.ui=always")
	} else {
		args = append(args, "-c", "color.ui=never")
	}
	args = append(args, "diff", "--patch", "--no-index", "--", "/dev/null", rel)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err == nil {
		return trimmed, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return trimmed, nil
	}
	if trimmed == "" {
		return "", core.WrapError(core.KindUnknown, "gitmanager.diff_untracked.run", err)
	}
	return "", core.NewError(core.KindUnknown, "gitmanager.diff_untracked.run", fmt.Sprintf("git %s failed: %s", strings.Join(args, " "), trimmed))
}

func splitNonEmptyLines(raw string) []string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func isConciergeInternalPath(relativePath string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(relativePath)))
	if cleaned == "" || cleaned == "." {
		return false
	}
	return cleaned == ".concierge" || strings.HasPrefix(cleaned, ".concierge/")
}

type diffStatSummary struct {
	fileLines  []string
	files      int
	insertions int
	deletions  int
}

func parseDiffStatSummary(raw string) diffStatSummary {
	summary := diffStatSummary{}
	for _, line := range splitNonEmptyLines(raw) {
		switch {
		case strings.Contains(line, "file changed"), strings.Contains(line, "files changed"):
			summary.files += parseFirstDiffStatCount(diffStatFilesPattern, line)
			summary.insertions += parseFirstDiffStatCount(diffStatInsertionsPattern, line)
			summary.deletions += parseFirstDiffStatCount(diffStatDeletionsPattern, line)
		default:
			summary.fileLines = append(summary.fileLines, line)
		}
	}
	return summary
}

func mergeDiffStatSummary(left, right diffStatSummary) diffStatSummary {
	merged := diffStatSummary{
		fileLines:  append(append([]string{}, left.fileLines...), right.fileLines...),
		files:      left.files + right.files,
		insertions: left.insertions + right.insertions,
		deletions:  left.deletions + right.deletions,
	}
	if merged.files == 0 {
		merged.files = len(merged.fileLines)
	}
	return merged
}

func renderDiffStatSummary(summary diffStatSummary) string {
	if len(summary.fileLines) == 0 {
		return ""
	}

	files := summary.files
	if files == 0 {
		files = len(summary.fileLines)
	}

	parts := []string{fmt.Sprintf("%d file%s changed", files, pluralizeCount(files))}
	if summary.insertions > 0 {
		parts = append(parts, fmt.Sprintf("%d insertion%s(+)", summary.insertions, pluralizeCount(summary.insertions)))
	}
	if summary.deletions > 0 {
		parts = append(parts, fmt.Sprintf("%d deletion%s(-)", summary.deletions, pluralizeCount(summary.deletions)))
	}

	lines := append([]string{}, summary.fileLines...)
	lines = append(lines, strings.Join(parts, ", "))
	return strings.Join(lines, "\n")
}

func parseFirstDiffStatCount(pattern *regexp.Regexp, line string) int {
	matches := pattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return 0
	}
	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return value
}

func pluralizeCount(value int) string {
	if value == 1 {
		return ""
	}
	return "s"
}

func runGitCombined(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), trimmed)
	}
	return trimmed, nil
}
