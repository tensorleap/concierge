package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/gitmanager"
)

type changeReviewRenderOptions struct {
	EnableColor bool
}

type reviewDecisionDefault int

const (
	reviewDecisionDefaultReject reviewDecisionDefault = iota
	reviewDecisionDefaultKeep
)

func promptChangeReviewApproval(
	in io.Reader,
	out io.Writer,
	step core.EnsureStep,
	review gitmanager.ChangeReview,
	options changeReviewRenderOptions,
) (gitmanager.ReviewDecision, error) {
	if out == nil {
		out = io.Discard
	}

	if _, err := fmt.Fprintln(out); err != nil {
		return gitmanager.ReviewDecision{}, err
	}
	title := "Proposed Changes"
	if _, err := fmt.Fprintln(out, paint(title, ansiBold+ansiBlue, options.EnableColor)); err != nil {
		return gitmanager.ReviewDecision{}, err
	}
	if _, err := fmt.Fprintln(out, strings.Repeat("-", len(title))); err != nil {
		return gitmanager.ReviewDecision{}, err
	}

	focus := strings.TrimSpace(review.Focus)
	if focus == "" {
		focus = core.HumanEnsureStepRequirementLabel(step.ID)
	}
	if _, err := fmt.Fprintf(out, "Fixing: %s\n", focus); err != nil {
		return gitmanager.ReviewDecision{}, err
	}

	if review.Risk.IsRisky() {
		if _, err := fmt.Fprintln(out, "Risk warning:"); err != nil {
			return gitmanager.ReviewDecision{}, err
		}
		if summary := strings.TrimSpace(review.Risk.Summary); summary != "" {
			if _, err := fmt.Fprintf(out, "- %s\n", summary); err != nil {
				return gitmanager.ReviewDecision{}, err
			}
		}
		for _, reason := range review.Risk.Reasons {
			trimmed := strings.TrimSpace(reason)
			if trimmed == "" {
				continue
			}
			if _, err := fmt.Fprintf(out, "- %s\n", trimmed); err != nil {
				return gitmanager.ReviewDecision{}, err
			}
		}
	}

	if len(review.Files) > 0 {
		if _, err := fmt.Fprintln(out, "Files changed:"); err != nil {
			return gitmanager.ReviewDecision{}, err
		}
		for _, fileLine := range review.Files {
			if _, err := fmt.Fprintf(out, "- %s\n", formatChangedFileLine(fileLine, options.EnableColor)); err != nil {
				return gitmanager.ReviewDecision{}, err
			}
		}
	}

	stat := strings.TrimSpace(review.Stat)
	if stat != "" {
		if _, err := fmt.Fprintln(out, "Diff summary:"); err != nil {
			return gitmanager.ReviewDecision{}, err
		}
		for _, line := range strings.Split(strings.ReplaceAll(stat, "\r\n", "\n"), "\n") {
			trimmed := strings.TrimRight(line, " ")
			if strings.TrimSpace(trimmed) == "" {
				continue
			}
			if _, err := fmt.Fprintf(out, "  %s\n", trimmed); err != nil {
				return gitmanager.ReviewDecision{}, err
			}
		}
	}

	patch := strings.TrimSpace(review.Patch)
	if patch != "" && !review.Risk.HidePatch {
		if _, err := fmt.Fprintln(out, "Patch:"); err != nil {
			return gitmanager.ReviewDecision{}, err
		}
		if _, err := fmt.Fprintln(out, patch); err != nil {
			return gitmanager.ReviewDecision{}, err
		}
	} else if review.Risk.HidePatch {
		if _, err := fmt.Fprintln(out, "Patch omitted because this diff is dominated by risky artifact changes."); err != nil {
			return gitmanager.ReviewDecision{}, err
		}
	}

	reviewPrompt := "What should I do with these reviewed changes? [y] commit / [K] keep for local review / [n] restore:"
	reviewDefault := reviewDecisionDefaultKeep
	if review.Risk.IsRisky() {
		reviewPrompt = "What should I do with these risky reviewed changes? [y] commit / [k] keep / [N] restore:"
		reviewDefault = reviewDecisionDefaultReject
	}
	return promptReviewDecision(in, out, reviewPrompt, reviewDefault)
}

func formatChangedFileLine(line string, enableColor bool) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	status := trimmed
	path := ""
	if tab := strings.Index(trimmed, "\t"); tab >= 0 {
		status = strings.TrimSpace(trimmed[:tab])
		path = strings.TrimSpace(trimmed[tab+1:])
	} else if space := strings.Index(trimmed, " "); space >= 0 {
		status = strings.TrimSpace(trimmed[:space])
		path = strings.TrimSpace(trimmed[space+1:])
	}
	if path == "" {
		path = status
		status = "?"
	}

	code := status
	color := ansiDim
	switch {
	case strings.HasPrefix(status, "A"), strings.HasPrefix(status, "??"):
		code = "A"
		color = ansiGreen
	case strings.HasPrefix(status, "M"):
		code = "M"
		color = ansiYellow
	case strings.HasPrefix(status, "D"):
		code = "D"
		color = ansiRed
	case strings.HasPrefix(status, "R"):
		code = "R"
		color = ansiCyan
	}

	return fmt.Sprintf("%s %s", paint(code, ansiBold+color, enableColor), path)
}

func promptReviewDecision(
	in io.Reader,
	out io.Writer,
	prompt string,
	defaultDecision reviewDecisionDefault,
) (gitmanager.ReviewDecision, error) {
	if out == nil {
		out = io.Discard
	}

	promptText := strings.TrimSpace(prompt)
	if promptText == "" {
		promptText = "What should I do with these reviewed changes? [y] commit / [K] keep / [n] restore:"
	}
	if _, err := fmt.Fprintf(out, "%s ", promptText); err != nil {
		return gitmanager.ReviewDecision{}, err
	}

	line, err := readPromptLine(in)
	if err != nil {
		return gitmanager.ReviewDecision{}, err
	}

	normalized := strings.ToLower(strings.TrimSpace(line))
	if normalized == "" {
		switch defaultDecision {
		case reviewDecisionDefaultReject:
			return gitmanager.ReviewDecision{}, nil
		default:
			return gitmanager.ReviewDecision{KeepChanges: true, Commit: false}, nil
		}
	}

	switch normalized {
	case "y", "yes", "c", "commit":
		return gitmanager.ReviewDecision{KeepChanges: true, Commit: true}, nil
	case "k", "keep", "review":
		return gitmanager.ReviewDecision{KeepChanges: true, Commit: false}, nil
	case "n", "no", "r", "reject", "restore":
		return gitmanager.ReviewDecision{}, nil
	default:
		return gitmanager.ReviewDecision{}, fmt.Errorf("invalid review decision %q", line)
	}
}
