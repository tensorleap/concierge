package observe

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/core/ports"
)

const (
	tuiChecklistWidth = 34
	tuiMinWidth       = 60
	tuiTickInterval   = time.Second
)

// TUIRenderer implements observe.Sink and ports.Reporter using a full-screen
// Bubble Tea TUI. It provides the same interface surface as SplitScreenRenderer.
type TUIRenderer struct {
	program   *tea.Program
	reporter  ports.Reporter
	mu        sync.Mutex
	suspended bool
}

// NewTUIRenderer creates and starts the Bubble Tea program on the alternate screen.
func NewTUIRenderer(file *os.File, options RenderOptions) *TUIRenderer {
	styles := newTUIStyles()
	model := tuiModel{
		activity:  newActivityModel(styles),
		checklist: newChecklistModel(styles),
		statusBar: newStatusBarModel(styles),
		styles:    styles,
		startedAt: time.Now(),
	}

	t := &TUIRenderer{}

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithOutput(file),
		tea.WithInput(os.Stdin),
		tea.WithMouseCellMotion(),
	)
	t.program = p

	go func() {
		_, _ = p.Run()
	}()

	return t
}

// SetReporter sets the wrapped reporter for iteration report output.
func (t *TUIRenderer) SetReporter(r ports.Reporter) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reporter = r
}

// Emit implements observe.Sink. Thread-safe.
//
// For suspension events (EventWaitingApproval, EventGitReviewStarted), the
// terminal is released synchronously so the caller can read stdin immediately
// after Emit returns. For restoration events (EventApprovalResolved,
// EventGitReviewFinished), the terminal is restored synchronously.
func (t *TUIRenderer) Emit(event Event) {
	if t == nil || t.program == nil {
		return
	}

	switch event.Kind {
	case EventWaitingApproval, EventGitReviewStarted:
		t.mu.Lock()
		t.suspended = true
		t.mu.Unlock()
		t.program.ReleaseTerminal()
		return // don't forward; prompt will run on the normal terminal

	case EventApprovalResolved, EventGitReviewFinished:
		t.program.RestoreTerminal()
		t.mu.Lock()
		t.suspended = false
		t.mu.Unlock()
		// Forward the event so the model can update state.
	}

	t.program.Send(EventMsg{Event: event})
}

// Report implements ports.Reporter.
func (t *TUIRenderer) Report(ctx context.Context, r core.IterationReport) error {
	t.mu.Lock()
	reporter := t.reporter
	t.mu.Unlock()

	// Delegate to the wrapped reporter (writes to a buffer or file).
	var reportErr error
	if reporter != nil {
		reportErr = reporter.Report(ctx, r)
	}

	// Send report to TUI for checklist + log updates.
	done := make(chan error, 1)
	t.program.Send(ReportMsg{Report: r, Done: done})
	<-done

	return reportErr
}

// Close shuts down the Bubble Tea program and restores the terminal.
func (t *TUIRenderer) Close() {
	if t == nil || t.program == nil {
		return
	}
	t.program.Quit()
	t.program.Wait()
}

// --- Bubble Tea model -------------------------------------------------------

type tuiModel struct {
	activity  activityModel
	checklist checklistModel
	statusBar statusBarModel
	styles    tuiStyles
	startedAt time.Time
	width     int
	height    int
	quitting  bool
}

func (m tuiModel) Init() tea.Cmd {
	return tickCmd()
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			m.activity.autoScroll = false
			m.activity.viewport.LineUp(1)
		case "down", "j":
			m.activity.viewport.LineDown(1)
			if m.activity.viewport.AtBottom() {
				m.activity.autoScroll = true
			}
		case "pgup":
			m.activity.autoScroll = false
			m.activity.viewport.HalfViewUp()
		case "pgdown":
			m.activity.viewport.HalfViewDown()
			if m.activity.viewport.AtBottom() {
				m.activity.autoScroll = true
			}
		case "G":
			m.activity.autoScroll = true
			m.activity.viewport.GotoBottom()
		case "g":
			m.activity.autoScroll = false
			m.activity.viewport.GotoTop()
		}

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.activity.viewport, cmd = m.activity.viewport.Update(msg)
		if !m.activity.viewport.AtBottom() {
			m.activity.autoScroll = false
		}
		cmds = append(cmds, cmd)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()

	case EventMsg:
		m.handleEvent(msg.Event)

	case ReportMsg:
		m.handleReport(msg)

	case TickMsg:
		cmds = append(cmds, tickCmd())
	}

	return m, tea.Batch(cmds...)
}

func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	leftWidth := m.width - tuiChecklistWidth - 1
	if leftWidth < 20 {
		leftWidth = m.width
	}

	// Left panel: activity log
	leftView := m.activity.View()

	// Status bar
	barView := m.statusBar.View()

	// If terminal is too narrow, skip the checklist
	if m.width < tuiMinWidth {
		return lipgloss.JoinVertical(lipgloss.Left, leftView, barView)
	}

	// Right panel: checklist
	rightView := m.checklist.View()

	// Border column
	borderHeight := m.height - 1 // minus status bar
	borderLines := make([]string, borderHeight)
	for i := range borderLines {
		borderLines[i] = m.styles.panelBorder.Render("│")
	}
	border := strings.Join(borderLines, "\n")

	// Join panels
	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftView, border, rightView)

	return lipgloss.JoinVertical(lipgloss.Left, mainContent, barView)
}

// layout recalculates sub-model dimensions after a resize.
func (m *tuiModel) layout() {
	if m.width < tuiMinWidth {
		m.activity.setSize(m.width, m.height-1)
		m.checklist.width = 0
		m.checklist.height = 0
	} else {
		leftWidth := m.width - tuiChecklistWidth - 1
		m.activity.setSize(leftWidth, m.height-1)
		m.checklist.width = tuiChecklistWidth
		m.checklist.height = m.height - 1
	}
	m.statusBar.width = m.width
}

func (m *tuiModel) handleEvent(event Event) {
	switch event.Kind {
	case EventIterationStarted:
		m.statusBar.iteration = maxInt(event.Iteration, 1)
		m.checklist.stepStates = makeInitialStepStates()
		m.checklist.activeStep = ""
	case EventStageStarted:
		m.statusBar.stage = event.Stage
	case EventStepSelected:
		m.statusBar.stepID = event.StepID
		if m.checklist.activeStep != "" && m.checklist.stepStates[m.checklist.activeStep] == StepInProgress {
			m.checklist.stepStates[m.checklist.activeStep] = StepPending
		}
		m.checklist.activeStep = event.StepID
		m.checklist.stepStates[event.StepID] = StepInProgress
	}

	m.activity.appendEvent(event)
}

func (m *tuiModel) handleReport(msg ReportMsg) {
	for _, check := range msg.Report.Checks {
		m.checklist.stepStates[check.StepID] = CheckStatusToDisplay(check.Status)
	}
	m.checklist.activeStep = ""
	m.activity.appendReportLines(msg.Report)

	if msg.Done != nil {
		msg.Done <- nil
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(tuiTickInterval, func(t time.Time) tea.Msg {
		return TickMsg{Time: t}
	})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
