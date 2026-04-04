package observe

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/core/ports"
)

const (
	splitPanelWidth    = 42
	splitMinLeftWidth  = 40
	splitMinTotalWidth = splitPanelWidth + splitMinLeftWidth
)

// SplitScreenRenderer renders a persistent right-side step checklist alongside
// the normal left-panel progress output. It implements both observe.Sink (for
// live events) and ports.Reporter (for end-of-iteration checklist data).
type SplitScreenRenderer struct {
	file     *os.File
	fd       int
	options  RenderOptions
	reporter ports.Reporter // wrapped reporter for left-panel summary output

	mu             sync.Mutex
	termWidth      int
	termHeight     int
	panelEnabled   bool
	panelSuspended bool

	stepStates   map[core.EnsureStepID]StepDisplayStatus
	activeStepID core.EnsureStepID

	highlights *HighlightsRenderer
	leftWriter *LeftPanelWriter

	stopResize chan struct{}

	// lipgloss styles (initialised once)
	groupStyle      lipgloss.Style
	pendingStyle    lipgloss.Style
	inProgressStyle lipgloss.Style
	passStyle       lipgloss.Style
	warnStyle       lipgloss.Style
	failStyle       lipgloss.Style
	panelBorderChar string
}

// NewSplitScreenRenderer creates a split-screen renderer for TTY output.
// It wraps the provided reporter for left-panel summary output while overlaying
// the step checklist panel on the right side of the terminal.
func NewSplitScreenRenderer(file *os.File, options RenderOptions) *SplitScreenRenderer {
	fd := int(file.Fd())
	w, h, err := term.GetSize(fd)
	if err != nil || w < splitMinTotalWidth {
		w = 0
		h = 0
	}

	ss := &SplitScreenRenderer{
		file:       file,
		fd:         fd,
		options:    options,
		termWidth:  w,
		termHeight: h,
		stepStates: makeInitialStepStates(),
		stopResize: make(chan struct{}),
	}

	ss.panelEnabled = w >= splitMinTotalWidth && !options.NoColor
	ss.initStyles()

	// The left-panel writer constrains output and triggers panel redraws.
	ss.leftWriter = NewLeftPanelWriter(file, ss.leftWidth, ss.afterLeftLine)
	ss.highlights = NewHighlightsRenderer(ss.leftWriter, options)

	if ss.panelEnabled {
		ss.watchResize()
		ss.drawPanel()
	}

	return ss
}

// SetReporter sets the wrapped reporter whose output is constrained to the left panel.
// Must be called before the first Report() call.
func (ss *SplitScreenRenderer) SetReporter(r ports.Reporter) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.reporter = r
}

// LeftWriter returns the width-constrained writer for the left panel.
// Pass this to reporters so their output stays within the left panel.
func (ss *SplitScreenRenderer) LeftWriter() io.Writer {
	return ss.leftWriter
}

// Emit implements observe.Sink.
func (ss *SplitScreenRenderer) Emit(event Event) {
	if ss == nil {
		return
	}
	ss.mu.Lock()

	switch event.Kind {
	case EventIterationStarted:
		ss.stepStates = makeInitialStepStates()
		ss.activeStepID = ""

	case EventStepSelected:
		if ss.activeStepID != "" && ss.stepStates[ss.activeStepID] == StepInProgress {
			ss.stepStates[ss.activeStepID] = StepPending
		}
		ss.activeStepID = event.StepID
		ss.stepStates[event.StepID] = StepInProgress

	case EventWaitingApproval, EventGitReviewStarted:
		ss.panelSuspended = true
		ss.clearPanelLocked()

	case EventApprovalResolved, EventGitReviewFinished:
		ss.panelSuspended = false
	}

	needsRedraw := event.Kind == EventStepSelected ||
		event.Kind == EventIterationStarted ||
		event.Kind == EventApprovalResolved ||
		event.Kind == EventGitReviewFinished

	ss.mu.Unlock()

	// Forward to highlights renderer (it writes to leftWriter, which triggers
	// afterLeftLine and thus panel redraws on its own). We do this outside
	// the lock because HighlightsRenderer has its own mutex.
	ss.highlights.Emit(event)

	if needsRedraw {
		ss.mu.Lock()
		ss.drawPanelLocked()
		ss.mu.Unlock()
	}
}

// Report implements ports.Reporter.
func (ss *SplitScreenRenderer) Report(ctx context.Context, r core.IterationReport) error {
	ss.mu.Lock()
	for _, check := range r.Checks {
		ss.stepStates[check.StepID] = CheckStatusToDisplay(check.Status)
	}
	ss.activeStepID = ""
	ss.mu.Unlock()

	// Delegate to the wrapped stdout reporter for left-panel output.
	if err := ss.reporter.Report(ctx, r); err != nil {
		return err
	}

	ss.mu.Lock()
	ss.drawPanelLocked()
	ss.mu.Unlock()
	return nil
}

// Close stops the resize watcher.
func (ss *SplitScreenRenderer) Close() {
	if ss == nil {
		return
	}
	select {
	case ss.stopResize <- struct{}{}:
	default:
	}
}

// --- internal helpers -------------------------------------------------------

func (ss *SplitScreenRenderer) initStyles() {
	ss.groupStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	ss.pendingStyle = lipgloss.NewStyle().Faint(true)
	ss.inProgressStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	ss.passStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	ss.warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	ss.failStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	ss.panelBorderChar = "│"
}

func (ss *SplitScreenRenderer) leftWidth() int {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if !ss.panelEnabled || ss.panelSuspended {
		return 0 // no truncation
	}
	lw := ss.termWidth - splitPanelWidth - 1 // -1 for border
	if lw < splitMinLeftWidth {
		return 0
	}
	return lw
}

func (ss *SplitScreenRenderer) afterLeftLine() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.drawPanelLocked()
}

func (ss *SplitScreenRenderer) drawPanel() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.drawPanelLocked()
}

// drawPanelLocked renders the right panel. Caller must hold ss.mu.
func (ss *SplitScreenRenderer) drawPanelLocked() {
	if !ss.panelEnabled || ss.panelSuspended {
		return
	}
	w, h, err := term.GetSize(ss.fd)
	if err == nil {
		ss.termWidth = w
		ss.termHeight = h
	}
	if ss.termWidth < splitMinTotalWidth {
		return
	}

	rightCol := ss.termWidth - splitPanelWidth + 1
	lines := ss.buildPanelLines()

	var buf strings.Builder
	buf.WriteString("\033[?25l") // hide cursor
	buf.WriteString("\033[s")    // save cursor position

	for i, line := range lines {
		row := i + 1
		if row > ss.termHeight {
			break
		}
		buf.WriteString(fmt.Sprintf("\033[%d;%dH", row, rightCol))
		buf.WriteString(line)
	}

	// Clear any leftover rows from a previous taller render.
	blank := strings.Repeat(" ", splitPanelWidth)
	for row := len(lines) + 1; row <= ss.termHeight && row <= len(lines)+2; row++ {
		buf.WriteString(fmt.Sprintf("\033[%d;%dH%s", row, rightCol, blank))
	}

	buf.WriteString("\033[u")    // restore cursor position
	buf.WriteString("\033[?25h") // show cursor

	_, _ = io.WriteString(ss.file, buf.String())
}

// clearPanelLocked erases the right panel area. Caller must hold ss.mu.
func (ss *SplitScreenRenderer) clearPanelLocked() {
	if !ss.panelEnabled {
		return
	}
	rightCol := ss.termWidth - splitPanelWidth + 1
	blank := strings.Repeat(" ", splitPanelWidth)

	var buf strings.Builder
	buf.WriteString("\033[?25l")
	buf.WriteString("\033[s")
	panelRows := ss.panelRowCount()
	for row := 1; row <= panelRows && row <= ss.termHeight; row++ {
		buf.WriteString(fmt.Sprintf("\033[%d;%dH%s", row, rightCol, blank))
	}
	buf.WriteString("\033[u")
	buf.WriteString("\033[?25h")
	_, _ = io.WriteString(ss.file, buf.String())
}

func (ss *SplitScreenRenderer) panelRowCount() int {
	count := 0
	for _, g := range ChecklistGroups {
		count++ // group header
		count += len(g.Steps)
	}
	return count + 1 // +1 for title
}

func (ss *SplitScreenRenderer) buildPanelLines() []string {
	lines := make([]string, 0, ss.panelRowCount())

	title := ss.padLine(ss.panelBorderChar + " " + ss.groupStyle.Render("Concierge Steps"))
	lines = append(lines, title)

	for _, g := range ChecklistGroups {
		header := ss.padLine(ss.panelBorderChar + " " + ss.groupStyle.Render(g.Label))
		lines = append(lines, header)

		for _, stepID := range g.Steps {
			status := ss.stepStates[stepID]
			label := ShortStepLabel(stepID)
			icon, style := ss.iconAndStyle(status)
			entry := fmt.Sprintf("%s  %s %s", ss.panelBorderChar, icon, style.Render(label))
			lines = append(lines, ss.padLine(entry))
		}
	}
	return lines
}

func (ss *SplitScreenRenderer) iconAndStyle(status StepDisplayStatus) (string, lipgloss.Style) {
	switch status {
	case StepInProgress:
		return "▸", ss.inProgressStyle
	case StepPass:
		return "✅", ss.passStyle
	case StepWarning:
		return "⚠", ss.warnStyle
	case StepFail:
		return "☐", ss.failStyle
	default:
		return "·", ss.pendingStyle
	}
}

// padLine pads or truncates a panel line to exactly splitPanelWidth visible chars.
func (ss *SplitScreenRenderer) padLine(s string) string {
	vis := visibleLen(s)
	if vis >= splitPanelWidth {
		return truncateVisibleWidth(s, splitPanelWidth)
	}
	return s + strings.Repeat(" ", splitPanelWidth-vis)
}

// visibleLen returns the number of visible (non-ANSI-escape) characters.
func visibleLen(s string) int {
	count := 0
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !isCSITerminator(s[j]) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
			continue
		}
		count++
		i++
	}
	return count
}

func makeInitialStepStates() map[core.EnsureStepID]StepDisplayStatus {
	states := make(map[core.EnsureStepID]StepDisplayStatus)
	for _, g := range ChecklistGroups {
		for _, id := range g.Steps {
			states[id] = StepPending
		}
	}
	return states
}

func (ss *SplitScreenRenderer) watchResize() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for {
			select {
			case <-ch:
				w, h, err := term.GetSize(ss.fd)
				if err != nil {
					continue
				}
				ss.mu.Lock()
				ss.termWidth = w
				ss.termHeight = h
				ss.panelEnabled = w >= splitMinTotalWidth && !ss.options.NoColor
				if ss.panelEnabled && !ss.panelSuspended {
					ss.drawPanelLocked()
				}
				ss.mu.Unlock()
			case <-ss.stopResize:
				signal.Stop(ch)
				return
			}
		}
	}()
}
