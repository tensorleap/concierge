package observe

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

// --- StepGroup / StepDisplayStatus tests ------------------------------------

func TestChecklistGroupsCoverAllVisibleSteps(t *testing.T) {
	seen := map[core.EnsureStepID]bool{}
	for _, g := range ChecklistGroups {
		for _, id := range g.Steps {
			if seen[id] {
				t.Fatalf("duplicate step %s", id)
			}
			seen[id] = true
		}
	}
	if len(seen) == 0 {
		t.Fatal("no steps in ChecklistGroups")
	}
}

func TestCheckStatusToDisplay(t *testing.T) {
	cases := []struct {
		in  core.CheckStatus
		out StepDisplayStatus
	}{
		{core.CheckStatusPass, StepPass},
		{core.CheckStatusWarning, StepWarning},
		{core.CheckStatusFail, StepFail},
		{"", StepPending},
	}
	for _, tc := range cases {
		got := CheckStatusToDisplay(tc.in)
		if got != tc.out {
			t.Errorf("CheckStatusToDisplay(%q) = %q, want %q", tc.in, got, tc.out)
		}
	}
}

// --- LeftPanelWriter tests --------------------------------------------------

func TestLeftPanelWriterTruncatesLongLines(t *testing.T) {
	var buf bytes.Buffer
	redraws := 0
	w := NewLeftPanelWriter(&buf, func() int { return 10 }, func() { redraws++ })

	_, err := w.Write([]byte("abcdefghijklmnop\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "abcdefghij") {
		t.Fatalf("expected truncated output, got %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatal("expected trailing newline")
	}
	if redraws != 1 {
		t.Fatalf("expected 1 redraw, got %d", redraws)
	}
}

func TestLeftPanelWriterNoTruncationWhenZeroWidth(t *testing.T) {
	var buf bytes.Buffer
	w := NewLeftPanelWriter(&buf, func() int { return 0 }, nil)

	_, err := w.Write([]byte("hello world\n"))
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello world\n" {
		t.Fatalf("expected no truncation, got %q", buf.String())
	}
}

func TestLeftPanelWriterHandlesMultipleLines(t *testing.T) {
	var buf bytes.Buffer
	redraws := 0
	w := NewLeftPanelWriter(&buf, func() int { return 5 }, func() { redraws++ })

	_, err := w.Write([]byte("abcdefg\n1234567\n"))
	if err != nil {
		t.Fatal(err)
	}
	if redraws != 2 {
		t.Fatalf("expected 2 redraws, got %d", redraws)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
}

func TestLeftPanelWriterPreservesANSI(t *testing.T) {
	var buf bytes.Buffer
	w := NewLeftPanelWriter(&buf, func() int { return 5 }, nil)

	// ANSI escape + "hello" (5 visible chars) + extra visible chars
	_, err := w.Write([]byte("\033[1mhelloworld\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "\033[1m") {
		t.Fatalf("expected ANSI code preserved, got %q", got)
	}
	// Should have "hello" (5 visible) but not "world"
	if strings.Contains(got, "world") {
		t.Fatalf("expected truncation to remove 'world', got %q", got)
	}
}

// --- SplitScreenRenderer state machine tests --------------------------------

func TestSplitScreenStepSelectedMarksInProgress(t *testing.T) {
	ss := newTestRenderer(t)

	ss.Emit(Event{Kind: EventStepSelected, StepID: core.EnsureStepLeapYAML})

	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.stepStates[core.EnsureStepLeapYAML] != StepInProgress {
		t.Fatalf("expected StepInProgress, got %s", ss.stepStates[core.EnsureStepLeapYAML])
	}
	if ss.activeStepID != core.EnsureStepLeapYAML {
		t.Fatalf("expected active step to be LeapYAML, got %s", ss.activeStepID)
	}
}

func TestSplitScreenStepSelectedResetsPreviousInProgress(t *testing.T) {
	ss := newTestRenderer(t)

	ss.Emit(Event{Kind: EventStepSelected, StepID: core.EnsureStepLeapYAML})
	ss.Emit(Event{Kind: EventStepSelected, StepID: core.EnsureStepPythonRuntime})

	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.stepStates[core.EnsureStepLeapYAML] != StepPending {
		t.Fatalf("expected previous step reset to pending, got %s", ss.stepStates[core.EnsureStepLeapYAML])
	}
	if ss.stepStates[core.EnsureStepPythonRuntime] != StepInProgress {
		t.Fatalf("expected new step in_progress, got %s", ss.stepStates[core.EnsureStepPythonRuntime])
	}
}

func TestSplitScreenIterationStartedResetsAll(t *testing.T) {
	ss := newTestRenderer(t)

	ss.Emit(Event{Kind: EventStepSelected, StepID: core.EnsureStepLeapYAML})
	ss.Emit(Event{Kind: EventIterationStarted, Iteration: 2})

	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.stepStates[core.EnsureStepLeapYAML] != StepPending {
		t.Fatalf("expected reset to pending after new iteration, got %s", ss.stepStates[core.EnsureStepLeapYAML])
	}
	if ss.activeStepID != "" {
		t.Fatalf("expected active step cleared, got %s", ss.activeStepID)
	}
}

func TestSplitScreenReportUpdatesStepStates(t *testing.T) {
	ss := newTestRenderer(t)

	report := core.IterationReport{
		Checks: []core.VerifiedCheck{
			{StepID: core.EnsureStepRepositoryContext, Status: core.CheckStatusPass},
			{StepID: core.EnsureStepPythonRuntime, Status: core.CheckStatusWarning},
			{StepID: core.EnsureStepLeapYAML, Status: core.CheckStatusFail},
		},
	}

	err := ss.Report(context.Background(), report)
	if err != nil {
		t.Fatal(err)
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.stepStates[core.EnsureStepRepositoryContext] != StepPass {
		t.Fatalf("expected pass, got %s", ss.stepStates[core.EnsureStepRepositoryContext])
	}
	if ss.stepStates[core.EnsureStepPythonRuntime] != StepWarning {
		t.Fatalf("expected warning, got %s", ss.stepStates[core.EnsureStepPythonRuntime])
	}
	if ss.stepStates[core.EnsureStepLeapYAML] != StepFail {
		t.Fatalf("expected fail, got %s", ss.stepStates[core.EnsureStepLeapYAML])
	}
}

func TestSplitScreenPanelSuspendsOnApproval(t *testing.T) {
	ss := newTestRenderer(t)

	ss.Emit(Event{Kind: EventWaitingApproval, StepID: core.EnsureStepLeapYAML})

	ss.mu.Lock()
	suspended := ss.panelSuspended
	ss.mu.Unlock()

	if !suspended {
		t.Fatal("expected panel to be suspended during approval")
	}

	ss.Emit(Event{Kind: EventApprovalResolved})

	ss.mu.Lock()
	suspended = ss.panelSuspended
	ss.mu.Unlock()

	if suspended {
		t.Fatal("expected panel to be restored after approval")
	}
}

func TestSplitScreenBuildPanelLinesHasGroupsAndSteps(t *testing.T) {
	ss := newTestRenderer(t)

	ss.mu.Lock()
	lines := ss.buildPanelLines()
	ss.mu.Unlock()

	if len(lines) == 0 {
		t.Fatal("expected non-empty panel lines")
	}

	// Title + group headers + steps
	expectedGroups := len(ChecklistGroups)
	totalSteps := 0
	for _, g := range ChecklistGroups {
		totalSteps += len(g.Steps)
	}
	expectedLines := 1 + expectedGroups + totalSteps // title + headers + steps
	if len(lines) != expectedLines {
		t.Fatalf("expected %d panel lines (1 title + %d groups + %d steps), got %d",
			expectedLines, expectedGroups, totalSteps, len(lines))
	}
}

// --- test helpers -----------------------------------------------------------

// newTestRenderer creates a SplitScreenRenderer backed by a temp file with
// the panel disabled (since test output isn't a real terminal).
func newTestRenderer(t *testing.T) *SplitScreenRenderer {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "splitscreen-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })

	ss := &SplitScreenRenderer{
		file:       f,
		fd:         int(f.Fd()),
		options:    RenderOptions{NoColor: true},
		termWidth:  120,
		termHeight: 40,
		panelEnabled: false, // don't try to write ANSI to a temp file
		stepStates: makeInitialStepStates(),
		stopResize: make(chan struct{}),
	}
	ss.initStyles()
	ss.leftWriter = NewLeftPanelWriter(f, ss.leftWidth, nil)
	ss.highlights = NewHighlightsRenderer(ss.leftWriter, ss.options)
	// No-op reporter for tests.
	ss.reporter = nopReporter{}
	return ss
}

type nopReporter struct{}

func (nopReporter) Report(_ context.Context, _ core.IterationReport) error { return nil }
