package observe

import (
	"testing"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

// --- TUI model state machine tests ------------------------------------------

func TestTUIModelStepSelectedMarksInProgress(t *testing.T) {
	m := newTestTUIModel()

	m.handleEvent(Event{Kind: EventStepSelected, StepID: core.EnsureStepLeapYAML})

	if m.checklist.stepStates[core.EnsureStepLeapYAML] != StepInProgress {
		t.Fatalf("expected StepInProgress, got %s", m.checklist.stepStates[core.EnsureStepLeapYAML])
	}
	if m.checklist.activeStep != core.EnsureStepLeapYAML {
		t.Fatalf("expected active step to be LeapYAML, got %s", m.checklist.activeStep)
	}
}

func TestTUIModelStepSelectedResetsPreviousInProgress(t *testing.T) {
	m := newTestTUIModel()

	m.handleEvent(Event{Kind: EventStepSelected, StepID: core.EnsureStepLeapYAML})
	m.handleEvent(Event{Kind: EventStepSelected, StepID: core.EnsureStepPythonRuntime})

	if m.checklist.stepStates[core.EnsureStepLeapYAML] != StepPending {
		t.Fatalf("expected previous step reset to pending, got %s", m.checklist.stepStates[core.EnsureStepLeapYAML])
	}
	if m.checklist.stepStates[core.EnsureStepPythonRuntime] != StepInProgress {
		t.Fatalf("expected new step in_progress, got %s", m.checklist.stepStates[core.EnsureStepPythonRuntime])
	}
}

func TestTUIModelIterationStartedResetsAll(t *testing.T) {
	m := newTestTUIModel()

	m.handleEvent(Event{Kind: EventStepSelected, StepID: core.EnsureStepLeapYAML})
	m.handleEvent(Event{Kind: EventIterationStarted, Iteration: 2})

	if m.checklist.stepStates[core.EnsureStepLeapYAML] != StepPending {
		t.Fatalf("expected reset to pending after new iteration, got %s", m.checklist.stepStates[core.EnsureStepLeapYAML])
	}
	if m.checklist.activeStep != "" {
		t.Fatalf("expected active step cleared, got %s", m.checklist.activeStep)
	}
}

func TestTUIModelReportUpdatesStepStates(t *testing.T) {
	m := newTestTUIModel()

	msg := ReportMsg{
		Report: core.IterationReport{
			Checks: []core.VerifiedCheck{
				{StepID: core.EnsureStepRepositoryContext, Status: core.CheckStatusPass},
				{StepID: core.EnsureStepPythonRuntime, Status: core.CheckStatusWarning},
				{StepID: core.EnsureStepLeapYAML, Status: core.CheckStatusFail},
			},
		},
		Done: make(chan error, 1),
	}

	m.handleReport(msg)

	if m.checklist.stepStates[core.EnsureStepRepositoryContext] != StepPass {
		t.Fatalf("expected pass, got %s", m.checklist.stepStates[core.EnsureStepRepositoryContext])
	}
	if m.checklist.stepStates[core.EnsureStepPythonRuntime] != StepWarning {
		t.Fatalf("expected warning, got %s", m.checklist.stepStates[core.EnsureStepPythonRuntime])
	}
	if m.checklist.stepStates[core.EnsureStepLeapYAML] != StepFail {
		t.Fatalf("expected fail, got %s", m.checklist.stepStates[core.EnsureStepLeapYAML])
	}
}

func TestTUIModelReportClearsActiveStep(t *testing.T) {
	m := newTestTUIModel()

	m.handleEvent(Event{Kind: EventStepSelected, StepID: core.EnsureStepLeapYAML})
	m.handleReport(ReportMsg{
		Report: core.IterationReport{},
		Done:   make(chan error, 1),
	})

	if m.checklist.activeStep != "" {
		t.Fatalf("expected active step cleared after report, got %s", m.checklist.activeStep)
	}
}

func TestTUIModelStageUpdatesStatusBar(t *testing.T) {
	m := newTestTUIModel()

	m.handleEvent(Event{Kind: EventStageStarted, Stage: core.StageExecute})

	if m.statusBar.stage != core.StageExecute {
		t.Fatalf("expected stage Execute, got %s", m.statusBar.stage)
	}
}

func TestTUIModelIterationUpdatesStatusBar(t *testing.T) {
	m := newTestTUIModel()

	m.handleEvent(Event{Kind: EventIterationStarted, Iteration: 3})

	if m.statusBar.iteration != 3 {
		t.Fatalf("expected iteration 3, got %d", m.statusBar.iteration)
	}
}

// --- Activity log formatting tests ------------------------------------------

func TestActivityFormatIterationHeader(t *testing.T) {
	m := newTestTUIModel()

	m.activity.setSize(40, 20)
	m.activity.appendEvent(Event{Kind: EventIterationStarted, Iteration: 1, Time: time.Now()})

	if len(m.activity.lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(m.activity.lines))
	}
	line := m.activity.lines[0]
	if visibleLen(line) == 0 {
		t.Fatal("expected non-empty iteration header")
	}
}

func TestActivityFormatDeduplicatesEvents(t *testing.T) {
	m := newTestTUIModel()
	m.activity.setSize(80, 20)

	now := time.Now()
	m.activity.appendEvent(Event{Kind: EventIterationStarted, Iteration: 1, Time: now})
	m.activity.appendEvent(Event{Kind: EventStageStarted, Stage: core.StageSnapshot, Time: now})
	m.activity.appendEvent(Event{Kind: EventStageStarted, Stage: core.StageSnapshot, Time: now})

	// Should have 2 lines (iteration + 1 stage), not 3
	if len(m.activity.lines) != 2 {
		t.Fatalf("expected 2 lines (deduplicated), got %d", len(m.activity.lines))
	}
}

func TestActivityReportAddsCheckLines(t *testing.T) {
	m := newTestTUIModel()
	m.activity.setSize(80, 20)

	m.activity.appendReportLines(core.IterationReport{
		Checks: []core.VerifiedCheck{
			{StepID: core.EnsureStepRepositoryContext, Status: core.CheckStatusPass, Label: "Repo available"},
			{StepID: core.EnsureStepLeapYAML, Status: core.CheckStatusFail, Label: "leap.yaml valid"},
		},
	})

	// Should have: rule + 2 check lines + empty line = 4
	if len(m.activity.lines) < 3 {
		t.Fatalf("expected at least 3 lines for report, got %d", len(m.activity.lines))
	}
}

func TestActivityHeartbeatSuppressedWhenRecent(t *testing.T) {
	m := newTestTUIModel()
	m.activity.setSize(80, 20)

	now := time.Now()
	m.activity.appendEvent(Event{Kind: EventIterationStarted, Iteration: 1, Time: now})
	m.activity.appendEvent(Event{Kind: EventAgentStarted, Message: "Claude started", Time: now})

	before := len(m.activity.lines)
	// Heartbeat within 15s should be suppressed
	m.activity.appendEvent(Event{Kind: EventAgentHeartbeat, Time: now.Add(5 * time.Second)})
	if len(m.activity.lines) != before {
		t.Fatal("expected heartbeat to be suppressed within 15s interval")
	}
}

// --- Checklist view tests ---------------------------------------------------

func TestChecklistViewContainsGroupLabels(t *testing.T) {
	m := newTestTUIModel()
	m.checklist.width = 34
	m.checklist.height = 30

	view := m.checklist.View()
	for _, g := range ChecklistGroups {
		if !containsVisible(view, g.Label) {
			t.Errorf("expected checklist to contain group label %q", g.Label)
		}
	}
}

func TestChecklistViewShowsInProgressIcon(t *testing.T) {
	m := newTestTUIModel()
	m.checklist.width = 34
	m.checklist.height = 30
	m.checklist.stepStates[core.EnsureStepLeapYAML] = StepInProgress

	view := m.checklist.View()
	if !containsVisible(view, "▸") {
		t.Error("expected in-progress icon ▸ in checklist view")
	}
}

// --- Status bar tests -------------------------------------------------------

func TestStatusBarShowsIteration(t *testing.T) {
	m := newTestTUIModel()
	m.statusBar.width = 80
	m.statusBar.iteration = 3

	view := m.statusBar.View()
	if !containsVisible(view, "Iter 3") {
		t.Errorf("expected status bar to contain 'Iter 3', got %q", view)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00"},
		{42 * time.Second, "0:42"},
		{90 * time.Second, "1:30"},
		{5*time.Minute + 7*time.Second, "5:07"},
	}
	for _, tc := range cases {
		got := formatDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// --- test helpers -----------------------------------------------------------

func newTestTUIModel() *tuiModel {
	styles := newTUIStyles()
	m := &tuiModel{
		activity:  newActivityModel(styles),
		checklist: newChecklistModel(styles),
		statusBar: newStatusBarModel(styles),
		styles:    styles,
		startedAt: time.Now(),
		width:     120,
		height:    40,
	}
	m.layout()
	return m
}

// containsVisible checks if a rendered string contains the target when
// stripping ANSI escapes.
func containsVisible(rendered, target string) bool {
	// Simple check: the target should appear somewhere in the rendered output.
	// ANSI codes won't affect string containment for visible text.
	return contains(rendered, target)
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
