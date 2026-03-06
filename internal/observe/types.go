package observe

import (
	"sync"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

// Mode controls how live run progress is rendered.
type Mode string

const (
	ModeSpinner     Mode = "spinner"
	ModeHighlights  Mode = "highlights"
	ModePassthrough Mode = "passthrough"
)

// EventKind is the machine-readable type for one live run event.
type EventKind string

const (
	EventIterationStarted      EventKind = "iteration_started"
	EventIterationFinished     EventKind = "iteration_finished"
	EventStageStarted          EventKind = "stage_started"
	EventStageFinished         EventKind = "stage_finished"
	EventStepSelected          EventKind = "step_selected"
	EventWaitingApproval       EventKind = "waiting_approval"
	EventApprovalResolved      EventKind = "approval_resolved"
	EventAgentTaskPrepared     EventKind = "agent_task_prepared"
	EventAgentStarted          EventKind = "agent_started"
	EventAgentHeartbeat        EventKind = "agent_heartbeat"
	EventAgentTool             EventKind = "agent_tool"
	EventAgentMessage          EventKind = "agent_message"
	EventAgentStderr           EventKind = "agent_stderr"
	EventAgentFinished         EventKind = "agent_finished"
	EventAgentInterrupted      EventKind = "agent_interrupted"
	EventGitReviewStarted      EventKind = "git_review_started"
	EventGitReviewFinished     EventKind = "git_review_finished"
	EventValidationStarted     EventKind = "validation_started"
	EventValidationFinished    EventKind = "validation_finished"
	EventFallback              EventKind = "fallback"
	EventError                 EventKind = "error"
)

// Event captures one live progress update emitted during a run.
type Event struct {
	Time       time.Time         `json:"time"`
	Iteration  int               `json:"iteration,omitempty"`
	SnapshotID string            `json:"snapshotId,omitempty"`
	Stage      core.Stage        `json:"stage,omitempty"`
	StepID     core.EnsureStepID `json:"stepId,omitempty"`
	Kind       EventKind         `json:"kind"`
	Message    string            `json:"message,omitempty"`
	Detail     string            `json:"detail,omitempty"`
	Data       map[string]string `json:"data,omitempty"`
}

// Sink consumes live progress events.
type Sink interface {
	Emit(Event)
}

// SinkFunc adapts a function into a Sink.
type SinkFunc func(Event)

// Emit implements Sink.
func (f SinkFunc) Emit(event Event) {
	if f == nil {
		return
	}
	f(event)
}

// NopSink ignores all events.
type NopSink struct{}

// Emit implements Sink.
func (NopSink) Emit(Event) {}

// MultiSink fans events out to multiple sinks.
type MultiSink struct {
	sinks []Sink
}

// NewMultiSink creates a sink that broadcasts events to all non-nil sinks.
func NewMultiSink(sinks ...Sink) *MultiSink {
	filtered := make([]Sink, 0, len(sinks))
	for _, sink := range sinks {
		if sink == nil {
			continue
		}
		filtered = append(filtered, sink)
	}
	return &MultiSink{sinks: filtered}
}

// Emit implements Sink.
func (m *MultiSink) Emit(event Event) {
	if m == nil {
		return
	}
	for _, sink := range m.sinks {
		sink.Emit(event)
	}
}

// SafeSink serializes event delivery into an underlying sink.
type SafeSink struct {
	sink Sink
	mu   sync.Mutex
}

// NewSafeSink wraps a sink with a mutex for concurrent emitters.
func NewSafeSink(sink Sink) *SafeSink {
	if sink == nil {
		sink = NopSink{}
	}
	return &SafeSink{sink: sink}
}

// Emit implements Sink.
func (s *SafeSink) Emit(event Event) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sink.Emit(event)
}
