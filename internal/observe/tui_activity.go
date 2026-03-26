package observe

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"

	"github.com/tensorleap/concierge/internal/core"
)

const activityHeartbeatInterval = 15 * time.Second

// activityModel renders the left-panel scrollable activity log.
type activityModel struct {
	viewport       viewport.Model
	lines          []string
	styles         tuiStyles
	width          int
	height         int
	autoScroll     bool
	lastProgressAt time.Time
	lastHeartbeat  time.Time
	seenCurrent    map[string]struct{}
}

func newActivityModel(styles tuiStyles) activityModel {
	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = true
	return activityModel{
		viewport:    vp,
		styles:      styles,
		autoScroll:  true,
		seenCurrent: make(map[string]struct{}),
	}
}

func (m *activityModel) setSize(width, height int) {
	m.width = width
	m.height = height
	m.viewport.Width = width
	m.viewport.Height = height
	m.refreshContent()
}

func (m activityModel) View() string {
	return m.viewport.View()
}

// appendEvent formats and appends a log line for the given event.
func (m *activityModel) appendEvent(event Event) {
	now := event.Time
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if event.Kind == EventIterationStarted {
		m.seenCurrent = make(map[string]struct{})
		m.lastProgressAt = time.Time{}
		m.lastHeartbeat = time.Time{}
	}

	line := m.formatEvent(event, now)
	if line == "" {
		if event.Kind != EventAgentHeartbeat && event.Kind != EventExecutorHeartbeat {
			m.lastProgressAt = now
		}
		return
	}

	// Deduplicate within the current iteration
	if _, seen := m.seenCurrent[line]; seen {
		if event.Kind != EventAgentHeartbeat && event.Kind != EventExecutorHeartbeat {
			m.lastProgressAt = now
		}
		return
	}
	m.seenCurrent[line] = struct{}{}

	if event.Kind == EventAgentHeartbeat || event.Kind == EventExecutorHeartbeat {
		m.lastHeartbeat = now
	} else {
		m.lastProgressAt = now
	}

	m.lines = append(m.lines, line)
	m.refreshContent()
}

// appendReportLines adds formatted report output to the log.
func (m *activityModel) appendReportLines(report core.IterationReport) {
	// Separator
	rule := m.styles.dimRule.Render(strings.Repeat("┄", min(m.width, 40)))
	m.lines = append(m.lines, rule)

	// Checks
	for _, check := range report.Checks {
		label := checkLabel(check)
		switch check.Status {
		case core.CheckStatusPass:
			m.lines = append(m.lines, m.styles.checkPass.Render("  ☑ "+label))
		case core.CheckStatusFail:
			m.lines = append(m.lines, m.styles.checkFail.Render("  ☒ "+label))
		case core.CheckStatusWarning:
			m.lines = append(m.lines, m.styles.checkWarn.Render("  ⚠ "+label))
		}
	}

	// Notes from the report
	for _, note := range report.Notes {
		m.lines = append(m.lines, m.styles.reportSummary.Render("  "+note))
	}

	m.lines = append(m.lines, "")
	m.refreshContent()
}

func (m *activityModel) refreshContent() {
	content := strings.Join(m.lines, "\n")
	m.viewport.SetContent(content)
	if m.autoScroll {
		m.viewport.GotoBottom()
	}
}

// formatEvent returns a styled log line for the event, or "" to skip.
func (m *activityModel) formatEvent(event Event, now time.Time) string {
	switch event.Kind {
	case EventIterationStarted:
		n := maxInt(event.Iteration, 1)
		label := fmt.Sprintf("━━ Iteration %d ", n)
		remaining := m.width - visibleLen(label)
		if remaining > 0 {
			label += strings.Repeat("━", remaining)
		}
		return m.styles.iterationHeader.Render(label)

	case EventStageStarted:
		return m.styles.stageLine.Render("● " + nonEmpty(event.Message, stageLabel(event.Stage)))

	case EventStepSelected:
		return m.styles.stepSelected.Render("▸ " + nonEmpty(event.Message, "Working on: "+stepLabel(event.StepID)))

	case EventWaitingApproval, EventGitReviewStarted:
		return m.styles.warnLine.Render("● " + nonEmpty(event.Message, "Waiting for your approval"))

	case EventExecutorProgress:
		return m.styles.stageLine.Render("● " + nonEmpty(event.Message, "Working through the selected step"))

	case EventAgentTaskPrepared:
		return m.styles.agentTool.Render("● Preparing Claude task")

	case EventAgentStarted:
		return m.styles.agentStarted.Render("◆ " + nonEmpty(event.Message, "Claude started"))

	case EventAgentTool:
		detail := nonEmpty(event.Message, "Claude is working")
		if event.Data != nil {
			if toolName, ok := event.Data["tool"]; ok {
				headline := toolHeadline(toolName, event.Detail)
				if headline != "" {
					detail = headline
				}
			}
		}
		return m.styles.agentTool.Render("  ↳ " + detail)

	case EventAgentMessage:
		msg := nonEmpty(event.Message, "Reasoning about the fix")
		return m.styles.agentMessage.Render("  ↳ " + msg)

	case EventAgentStderr:
		return m.styles.warnLine.Render("  ↳ stderr: " + truncate(event.Detail, 80))

	case EventAgentFinished:
		return m.styles.agentFinished.Render("◆ Claude finished the step")

	case EventAgentInterrupted:
		return m.styles.agentInterrupt.Render("⚠ Claude was interrupted for this step")

	case EventValidationStarted:
		return m.styles.stageLine.Render("● " + nonEmpty(event.Message, "Validating runtime behavior"))

	case EventValidationFinished:
		return m.styles.stageLine.Render("● " + nonEmpty(event.Message, "Validation finished"))

	case EventAgentHeartbeat:
		return m.heartbeatLine(event, now)

	case EventExecutorHeartbeat:
		return m.executorHeartbeatLine(event, now)

	case EventFallback:
		return m.styles.warnLine.Render("⚠ " + nonEmpty(event.Message, "Falling back to buffered agent execution"))

	case EventError:
		return m.styles.errorLine.Render("✗ " + nonEmpty(event.Message, "Run error"))

	default:
		return ""
	}
}

func (m *activityModel) heartbeatLine(event Event, now time.Time) string {
	if m.lastProgressAt.IsZero() {
		return ""
	}
	if now.Sub(m.lastProgressAt) < activityHeartbeatInterval {
		return ""
	}
	if !m.lastHeartbeat.IsZero() && now.Sub(m.lastHeartbeat) < activityHeartbeatInterval {
		return ""
	}
	idle := int(now.Sub(m.lastProgressAt).Seconds())
	if idle < 1 {
		idle = 1
	}
	return m.styles.heartbeat.Render(fmt.Sprintf("  ⋯ still working (%ds)", idle))
}

func (m *activityModel) executorHeartbeatLine(event Event, now time.Time) string {
	if m.lastProgressAt.IsZero() {
		return ""
	}
	if now.Sub(m.lastProgressAt) < activityHeartbeatInterval {
		return ""
	}
	if !m.lastHeartbeat.IsZero() && now.Sub(m.lastHeartbeat) < activityHeartbeatInterval {
		return ""
	}
	idle := int(now.Sub(m.lastProgressAt).Seconds())
	if idle < 1 {
		idle = 1
	}
	return m.styles.heartbeat.Render(fmt.Sprintf("  ⋯ still working (%ds)", idle))
}

func checkLabel(check core.VerifiedCheck) string {
	if check.Label != "" {
		return check.Label
	}
	return ShortStepLabel(check.StepID)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
