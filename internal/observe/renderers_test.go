package observe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
