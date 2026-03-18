package observe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

func TestHighlightsRendererDeduplicatesRepeatedLines(t *testing.T) {
	var sink strings.Builder
	renderer := NewHighlightsRenderer(&sink, RenderOptions{NoColor: true})

	renderer.Emit(Event{Kind: EventIterationStarted, Iteration: 1})
	renderer.Emit(Event{Kind: EventStageStarted, Stage: core.StageInspect})
	renderer.Emit(Event{Kind: EventStageStarted, Stage: core.StageInspect})
	renderer.Emit(Event{Kind: EventStepSelected, StepID: core.EnsureStepLeapYAML})
	renderer.Emit(Event{Kind: EventStepSelected, StepID: core.EnsureStepLeapYAML})

	output := sink.String()
	if strings.Count(output, "Inspecting Tensorleap artifacts") != 1 {
		t.Fatalf("expected deduped inspect line, got %q", output)
	}
	if strings.Count(output, "Working on: leap.yaml is present and valid") != 1 {
		t.Fatalf("expected deduped step line, got %q", output)
	}
}

func TestHighlightsRendererShowsRateLimitedHeartbeatAfterLongSilence(t *testing.T) {
	var sink strings.Builder
	renderer := NewHighlightsRenderer(&sink, RenderOptions{NoColor: true})
	startedAt := time.Date(2026, time.March, 17, 12, 0, 0, 0, time.UTC)

	renderer.Emit(Event{Kind: EventIterationStarted, Iteration: 1, Time: startedAt})
	renderer.Emit(Event{Kind: EventAgentStarted, Message: "Claude started", Time: startedAt})
	renderer.Emit(Event{
		Kind:    EventAgentTool,
		Message: "Editing /workspace/leap_integration.py",
		Time:    startedAt.Add(1 * time.Second),
	})
	renderer.Emit(Event{
		Kind:    EventAgentHeartbeat,
		Message: "Editing /workspace/leap_integration.py",
		Time:    startedAt.Add(10 * time.Second),
	})
	renderer.Emit(Event{
		Kind:    EventAgentHeartbeat,
		Message: "Editing /workspace/leap_integration.py",
		Time:    startedAt.Add(18 * time.Second),
	})

	output := sink.String()
	if !strings.Contains(output, "Claude still working: Editing /workspace/leap_integration.py") {
		t.Fatalf("expected heartbeat line in output, got %q", output)
	}
	if strings.Count(output, "Claude still working: Editing /workspace/leap_integration.py") != 1 {
		t.Fatalf("expected a single rate-limited heartbeat line, got %q", output)
	}
}

func TestHighlightsRendererShowsRateLimitedExecutorHeartbeatAfterLongSilence(t *testing.T) {
	var sink strings.Builder
	renderer := NewHighlightsRenderer(&sink, RenderOptions{NoColor: true})
	startedAt := time.Date(2026, time.March, 17, 12, 0, 0, 0, time.UTC)

	renderer.Emit(Event{Kind: EventIterationStarted, Iteration: 1, Time: startedAt})
	renderer.Emit(Event{
		Kind:    EventExecutorProgress,
		Message: "Running poetry install to repair the Poetry environment",
		Time:    startedAt.Add(1 * time.Second),
	})
	renderer.Emit(Event{
		Kind:    EventExecutorHeartbeat,
		Message: "Running poetry install to repair the Poetry environment",
		Time:    startedAt.Add(10 * time.Second),
	})
	renderer.Emit(Event{
		Kind:    EventExecutorHeartbeat,
		Message: "Running poetry install to repair the Poetry environment",
		Time:    startedAt.Add(18 * time.Second),
	})

	output := sink.String()
	if !strings.Contains(output, "Still working: Running poetry install to repair the Poetry environment") {
		t.Fatalf("expected executor heartbeat line in output, got %q", output)
	}
	if strings.Count(output, "Still working: Running poetry install to repair the Poetry environment") != 1 {
		t.Fatalf("expected a single rate-limited executor heartbeat line, got %q", output)
	}
}

func TestRecorderWritesEventsJSONLWhenSnapshotBecomesAvailable(t *testing.T) {
	projectRoot := t.TempDir()
	recorder, err := NewRecorder(projectRoot)
	if err != nil {
		t.Fatalf("NewRecorder failed: %v", err)
	}

	recorder.Emit(Event{Kind: EventIterationStarted, Iteration: 1})
	recorder.Emit(Event{Kind: EventStageFinished, SnapshotID: "snapshot-1", Stage: core.StageSnapshot})

	raw, err := os.ReadFile(filepath.Join(projectRoot, ".concierge", "evidence", "snapshot-1", "events.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 event lines, got %d (%q)", len(lines), string(raw))
	}
	if !strings.Contains(lines[0], `"kind":"iteration_started"`) {
		t.Fatalf("expected buffered event to flush first, got %q", lines[0])
	}
	if !strings.Contains(lines[1], `"snapshotId":"snapshot-1"`) {
		t.Fatalf("expected snapshot event to include snapshot id, got %q", lines[1])
	}
}
