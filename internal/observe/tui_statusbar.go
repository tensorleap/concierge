package observe

import (
	"fmt"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

// statusBarModel renders the bottom status bar.
type statusBarModel struct {
	width         int
	stage         core.Stage
	stepID        core.EnsureStepID
	iteration     int
	maxIterations int
	startedAt     time.Time
	styles        tuiStyles
}

func newStatusBarModel(styles tuiStyles) statusBarModel {
	return statusBarModel{
		styles: styles,
	}
}

func (m statusBarModel) View() string {
	if m.width <= 0 {
		return ""
	}

	var parts []string

	// Stage
	if m.stage != "" {
		parts = append(parts, m.styles.statusKey.Render(stageLabel(m.stage)))
	}

	// Step
	if m.stepID != "" {
		parts = append(parts, m.styles.statusValue.Render(ShortStepLabel(m.stepID)))
	}

	// Iteration
	if m.iteration > 0 {
		iterStr := fmt.Sprintf("Iter %d", m.iteration)
		if m.maxIterations > 0 {
			iterStr = fmt.Sprintf("Iter %d/%d", m.iteration, m.maxIterations)
		}
		parts = append(parts, m.styles.statusValue.Render(iterStr))
	}

	// Elapsed time
	if !m.startedAt.IsZero() {
		elapsed := time.Since(m.startedAt).Truncate(time.Second)
		parts = append(parts, m.styles.statusValue.Render(formatDuration(elapsed)))
	}

	left := strings.Join(parts, m.styles.statusValue.Render(" │ "))

	right := m.styles.statusValue.Render("q quit  ↑↓ scroll")

	// Pad between left and right
	gap := m.width - visibleLen(left) - visibleLen(right)
	if gap < 1 {
		gap = 1
	}
	padding := m.styles.statusBar.Render(strings.Repeat(" ", gap))

	return m.styles.statusBar.Render(left + padding + right)
}

func formatDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}
