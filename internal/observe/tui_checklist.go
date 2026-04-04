package observe

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/tensorleap/concierge/internal/core"
)

// checklistModel renders the right-panel step checklist.
type checklistModel struct {
	width      int
	height     int
	stepStates map[core.EnsureStepID]StepDisplayStatus
	activeStep core.EnsureStepID
	styles     tuiStyles
}

func newChecklistModel(styles tuiStyles) checklistModel {
	return checklistModel{
		stepStates: makeInitialStepStates(),
		styles:     styles,
	}
}

func (m checklistModel) View() string {
	if m.width <= 0 {
		return ""
	}

	var b strings.Builder
	innerWidth := m.width - 2 // 1 for border char, 1 for padding

	// Title
	title := m.styles.checklistTitle.Render("Concierge Steps")
	b.WriteString(title)
	b.WriteByte('\n')
	b.WriteByte('\n')

	for gi, g := range ChecklistGroups {
		header := m.styles.groupHeader.Render(g.Label)
		b.WriteString(header)
		b.WriteByte('\n')

		for _, stepID := range g.Steps {
			status := m.stepStates[stepID]
			label := ShortStepLabel(stepID)
			icon, style := m.iconAndStyle(status)

			line := fmt.Sprintf("  %s %s", icon, style.Render(label))
			// Truncate if needed
			if visibleLen(line) > innerWidth && innerWidth > 6 {
				line = truncateVisibleWidth(line, innerWidth)
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if gi < len(ChecklistGroups)-1 {
			b.WriteByte('\n')
		}
	}

	// Pad to full height
	content := b.String()
	lines := strings.Count(content, "\n")
	for lines < m.height-1 {
		content += "\n"
		lines++
	}

	return content
}

func (m checklistModel) iconAndStyle(status StepDisplayStatus) (string, lipgloss.Style) {
	switch status {
	case StepInProgress:
		return "▸", m.styles.stepInProgress
	case StepPass:
		return "✅", m.styles.stepPass
	case StepWarning:
		return "⚠", m.styles.stepWarn
	case StepFail:
		return "☐", m.styles.stepFail
	default:
		return "·", m.styles.stepPending
	}
}
