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

func promptChangeReviewApproval(
	in io.Reader,
	out io.Writer,
	step core.EnsureStep,
	review gitmanager.ChangeReview,
	options changeReviewRenderOptions,
) (bool, error) {
	if out == nil {
		out = io.Discard
	}

	if _, err := fmt.Fprintln(out); err != nil {
		return false, err
	}
	title := "Proposed Changes"
	if _, err := fmt.Fprintln(out, paint(title, ansiBold+ansiBlue, options.EnableColor)); err != nil {
		return false, err
	}
	if _, err := fmt.Fprintln(out, strings.Repeat("-", len(title))); err != nil {
		return false, err
	}

	focus := strings.TrimSpace(review.Focus)
	if focus == "" {
		focus = core.HumanEnsureStepRequirementLabel(step.ID)
	}
	if _, err := fmt.Fprintf(out, "Fixing: %s\n", focus); err != nil {
		return false, err
	}

	if len(review.Files) > 0 {
		if _, err := fmt.Fprintln(out, "Files changed:"); err != nil {
			return false, err
		}
		for _, fileLine := range review.Files {
			if _, err := fmt.Fprintf(out, "- %s\n", formatChangedFileLine(fileLine, options.EnableColor)); err != nil {
				return false, err
			}
		}
	}

	stat := strings.TrimSpace(review.Stat)
	if stat != "" {
		if _, err := fmt.Fprintln(out, "Diff summary:"); err != nil {
			return false, err
		}
		for _, line := range strings.Split(strings.ReplaceAll(stat, "\r\n", "\n"), "\n") {
			trimmed := strings.TrimRight(line, " ")
			if strings.TrimSpace(trimmed) == "" {
				continue
			}
			if _, err := fmt.Fprintf(out, "  %s\n", trimmed); err != nil {
				return false, err
			}
		}
	}

	patch := strings.TrimSpace(review.Patch)
	if patch != "" {
		if _, err := fmt.Fprintln(out, "Patch:"); err != nil {
			return false, err
		}
		if _, err := fmt.Fprintln(out, patch); err != nil {
			return false, err
		}
	}

	return promptYesNo(in, out, "Apply and commit these changes? [Y/n]:", true)
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
