package observe

import "github.com/charmbracelet/lipgloss"

// tuiStyles holds all lipgloss styles for the full-screen TUI.
type tuiStyles struct {
	// Checklist panel
	checklistTitle  lipgloss.Style
	groupHeader     lipgloss.Style
	stepPending     lipgloss.Style
	stepInProgress  lipgloss.Style
	stepPass        lipgloss.Style
	stepWarn        lipgloss.Style
	stepFail        lipgloss.Style
	panelBorder     lipgloss.Style

	// Activity log
	iterationHeader lipgloss.Style
	stageLine       lipgloss.Style
	stepSelected    lipgloss.Style
	agentStarted    lipgloss.Style
	agentTool       lipgloss.Style
	agentMessage    lipgloss.Style
	agentFinished   lipgloss.Style
	agentInterrupt  lipgloss.Style
	heartbeat       lipgloss.Style
	warnLine        lipgloss.Style
	errorLine       lipgloss.Style
	checkPass       lipgloss.Style
	checkFail       lipgloss.Style
	checkWarn       lipgloss.Style
	reportSummary   lipgloss.Style
	dimRule         lipgloss.Style
	filePath        lipgloss.Style

	// Status bar
	statusBar       lipgloss.Style
	statusKey       lipgloss.Style
	statusValue     lipgloss.Style
}

func newTUIStyles() tuiStyles {
	cyan := lipgloss.Color("14")
	green := lipgloss.Color("2")
	yellow := lipgloss.Color("3")
	red := lipgloss.Color("1")
	white := lipgloss.Color("15")
	dim := lipgloss.Color("8")

	return tuiStyles{
		// Checklist panel
		checklistTitle:  lipgloss.NewStyle().Bold(true).Foreground(cyan),
		groupHeader:     lipgloss.NewStyle().Bold(true).Foreground(cyan),
		stepPending:     lipgloss.NewStyle().Faint(true),
		stepInProgress:  lipgloss.NewStyle().Bold(true).Foreground(cyan),
		stepPass:        lipgloss.NewStyle().Foreground(green),
		stepWarn:        lipgloss.NewStyle().Foreground(yellow),
		stepFail:        lipgloss.NewStyle().Foreground(yellow),
		panelBorder:     lipgloss.NewStyle().Foreground(dim),

		// Activity log
		iterationHeader: lipgloss.NewStyle().Bold(true).Foreground(cyan),
		stageLine:       lipgloss.NewStyle().Foreground(cyan),
		stepSelected:    lipgloss.NewStyle().Bold(true).Foreground(white),
		agentStarted:    lipgloss.NewStyle().Foreground(green),
		agentTool:       lipgloss.NewStyle().Faint(true),
		agentMessage:    lipgloss.NewStyle().Faint(true).Italic(true),
		agentFinished:   lipgloss.NewStyle().Foreground(green),
		agentInterrupt:  lipgloss.NewStyle().Foreground(yellow),
		heartbeat:       lipgloss.NewStyle().Faint(true),
		warnLine:        lipgloss.NewStyle().Foreground(yellow),
		errorLine:       lipgloss.NewStyle().Foreground(red),
		checkPass:       lipgloss.NewStyle().Foreground(green),
		checkFail:       lipgloss.NewStyle().Foreground(red),
		checkWarn:       lipgloss.NewStyle().Foreground(yellow),
		reportSummary:   lipgloss.NewStyle(),
		dimRule:         lipgloss.NewStyle().Faint(true),
		filePath:        lipgloss.NewStyle().Underline(true),

		// Status bar
		statusBar:       lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(white),
		statusKey:       lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("236")).Foreground(cyan),
		statusValue:     lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(white),
	}
}
