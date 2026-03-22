package observe

import (
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

// EventMsg wraps an observe.Event for delivery into the Bubble Tea update loop.
type EventMsg struct {
	Event Event
}

// ReportMsg delivers an IterationReport for rendering in the TUI.
type ReportMsg struct {
	Report core.IterationReport
	Done   chan<- error
}

// TickMsg fires periodically to update the elapsed-time display.
type TickMsg struct {
	Time time.Time
}

// suspendMsg requests the TUI to release the terminal for prompt I/O.
type suspendMsg struct{}

// restoreMsg requests the TUI to restore the terminal after prompt I/O.
type restoreMsg struct{}
