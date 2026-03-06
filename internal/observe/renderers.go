package observe

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type RenderOptions struct {
	NoColor bool
}

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiCyan   = "\033[36m"
	ansiYellow = "\033[33m"
)

// SpinnerRenderer renders one live status line for long-running work.
type SpinnerRenderer struct {
	writer         io.Writer
	options        RenderOptions
	mu             sync.Mutex
	status         string
	lastActivityAt time.Time
}

func NewSpinnerRenderer(writer io.Writer, options RenderOptions) *SpinnerRenderer {
	if writer == nil {
		writer = os.Stdout
	}
	return &SpinnerRenderer{writer: writer, options: options}
}

func (r *SpinnerRenderer) Emit(event Event) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := event.Time
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if event.Kind != EventAgentHeartbeat {
		r.lastActivityAt = now
	}

	switch event.Kind {
	case EventStageStarted:
		r.status = stageLabel(event.Stage)
	case EventStepSelected:
		r.status = "Fixing " + strings.ToLower(stepLabel(event.StepID))
	case EventWaitingApproval, EventGitReviewStarted:
		r.status = event.Message
	case EventAgentStarted:
		r.status = event.Message
	case EventAgentHeartbeat:
		idle := "unknown"
		if !r.lastActivityAt.IsZero() {
			idle = fmt.Sprintf("%ds", int(now.Sub(r.lastActivityAt).Seconds()))
		}
		r.status = fmt.Sprintf("%s · still running · last activity %s ago", nonEmpty(event.Message, r.status), idle)
	case EventStageFinished, EventAgentFinished, EventAgentInterrupted, EventIterationFinished:
		if event.Message != "" {
			r.status = event.Message
		}
	}

	if strings.TrimSpace(r.status) == "" {
		return
	}
	_, _ = fmt.Fprintf(r.writer, "\r%s", padStatusLine(paint(r.status, ansiDim, !r.options.NoColor)))
	if event.Kind == EventWaitingApproval || event.Kind == EventGitReviewStarted || event.Kind == EventIterationFinished || event.Kind == EventAgentInterrupted {
		_, _ = fmt.Fprintln(r.writer)
	}
}

// HighlightsRenderer renders concise milestone lines.
type HighlightsRenderer struct {
	writer      io.Writer
	options     RenderOptions
	mu          sync.Mutex
	lastLine    string
	seenCurrent map[string]struct{}
}

func NewHighlightsRenderer(writer io.Writer, options RenderOptions) *HighlightsRenderer {
	if writer == nil {
		writer = os.Stdout
	}
	return &HighlightsRenderer{
		writer:      writer,
		options:     options,
		seenCurrent: make(map[string]struct{}),
	}
}

func (r *HighlightsRenderer) Emit(event Event) {
	if r == nil {
		return
	}
	line := r.lineForEvent(event)
	if strings.TrimSpace(line) == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if event.Kind == EventIterationStarted {
		r.seenCurrent = make(map[string]struct{})
	}
	if _, exists := r.seenCurrent[line]; exists {
		return
	}
	r.seenCurrent[line] = struct{}{}
	r.lastLine = line
	_, _ = fmt.Fprintln(r.writer, line)
}

func (r *HighlightsRenderer) lineForEvent(event Event) string {
	switch event.Kind {
	case EventIterationStarted:
		return paint(fmt.Sprintf("Iteration %d", maxInt(event.Iteration, 1)), ansiBold+ansiCyan, !r.options.NoColor)
	case EventStageStarted:
		return "• " + stageLabel(event.Stage)
	case EventStepSelected:
		return "• Fixing: " + stepLabel(event.StepID)
	case EventWaitingApproval, EventGitReviewStarted:
		return "• " + nonEmpty(event.Message, "Waiting for your approval")
	case EventAgentTaskPrepared:
		return "• Preparing Claude task"
	case EventAgentStarted:
		return "• " + nonEmpty(event.Message, "Claude started")
	case EventAgentTool:
		return "• " + nonEmpty(event.Message, "Claude is working")
	case EventAgentMessage:
		return "• " + nonEmpty(event.Message, "Reasoning about the fix")
	case EventAgentStderr:
		return "• Claude stderr: " + truncate(event.Detail, 100)
	case EventAgentFinished:
		return "• Claude finished the step"
	case EventAgentInterrupted:
		return "• Claude was interrupted for this step"
	case EventValidationStarted:
		return "• Validating runtime behavior"
	case EventValidationFinished:
		return "• Validation finished"
	case EventFallback:
		return paint("• "+nonEmpty(event.Message, "Falling back to buffered agent execution"), ansiYellow, !r.options.NoColor)
	case EventError:
		return paint("• "+nonEmpty(event.Message, "Run error"), ansiYellow, !r.options.NoColor)
	default:
		return ""
	}
}

// PassthroughRenderer prints the live Claude stream with minimal decoration.
type PassthroughRenderer struct {
	writer  io.Writer
	options RenderOptions
	mu      sync.Mutex
}

func NewPassthroughRenderer(writer io.Writer, options RenderOptions) *PassthroughRenderer {
	if writer == nil {
		writer = os.Stdout
	}
	return &PassthroughRenderer{writer: writer, options: options}
}

func (r *PassthroughRenderer) Emit(event Event) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	switch event.Kind {
	case EventIterationStarted:
		_, _ = fmt.Fprintln(r.writer, paint(fmt.Sprintf("Iteration %d", maxInt(event.Iteration, 1)), ansiBold+ansiCyan, !r.options.NoColor))
	case EventAgentStarted:
		_, _ = fmt.Fprintln(r.writer, paint(nonEmpty(event.Message, "Claude started"), ansiCyan, !r.options.NoColor))
		_, _ = fmt.Fprintln(r.writer, paint("Press b + Enter to stop Claude for this step.", ansiDim, !r.options.NoColor))
	case EventAgentTool:
		_, _ = fmt.Fprintf(r.writer, "[tool] %s\n", event.Message)
	case EventAgentMessage:
		_, _ = fmt.Fprintln(r.writer, event.Detail)
	case EventAgentStderr:
		_, _ = fmt.Fprintf(r.writer, "[stderr] %s\n", event.Detail)
	case EventAgentFinished:
		_, _ = fmt.Fprintln(r.writer, paint("Claude finished the step.", ansiCyan, !r.options.NoColor))
	case EventAgentInterrupted:
		_, _ = fmt.Fprintln(r.writer, paint("Claude was interrupted for this step.", ansiYellow, !r.options.NoColor))
	}
}

func paint(text, colorCode string, enabled bool) string {
	if !enabled || strings.TrimSpace(colorCode) == "" {
		return text
	}
	return colorCode + text + ansiReset
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit || limit <= 3 {
		return value
	}
	return value[:limit-3] + "..."
}

func maxInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func padStatusLine(value string) string {
	if len(value) < 96 {
		return value + strings.Repeat(" ", 96-len(value))
	}
	return value
}
