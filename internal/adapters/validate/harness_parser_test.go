package validate

import (
	"strings"
	"testing"
)

func TestHarnessParserDecodesRichEvents(t *testing.T) {
	raw := []byte(`{"event":"handler_result","status":"shape_invalid","handler_kind":"input","symbol":"image","subset":"train","sample_id":"0","sample_offset":0,"shape":[128,128,3],"dtype":"float32","expected_shape":[224,224,3],"expected_dtype":"float32","finite":true,"fingerprint":"abc"}
{"event":"subset_count","status":"ok","subset":"validation","count":2}
`)

	result, err := ParseHarnessEvents(raw)
	if err != nil {
		t.Fatalf("ParseHarnessEvents returned error: %v", err)
	}
	events := result.Events
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].HandlerKind != "input" || events[0].Symbol != "image" {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[0].Finite == nil || !*events[0].Finite {
		t.Fatalf("expected finite=true, got %+v", events[0].Finite)
	}
	if len(events[0].ExpectedShape) != 3 || events[0].ExpectedShape[0] != 224 {
		t.Fatalf("expected expected_shape to be decoded, got %+v", events[0].ExpectedShape)
	}
	if events[0].ExpectedDType != "float32" {
		t.Fatalf("expected expected_dtype to be decoded, got %q", events[0].ExpectedDType)
	}
	if events[1].Event != "subset_count" || events[1].Count != 2 {
		t.Fatalf("unexpected second event: %+v", events[1])
	}
	if len(result.Noise) != 0 {
		t.Fatalf("expected no noise, got %v", result.Noise)
	}
}

func TestHarnessParserAllowsUnknownEventTypes(t *testing.T) {
	raw := []byte(`{"event":"new_event_type","message":"future payload"}
`)

	result, err := ParseHarnessEvents(raw)
	if err != nil {
		t.Fatalf("ParseHarnessEvents returned error: %v", err)
	}
	if len(result.Events) != 1 || result.Events[0].Event != "new_event_type" {
		t.Fatalf("unexpected events: %+v", result.Events)
	}
}

func TestHarnessParserSkipsNonJSONLines(t *testing.T) {
	raw := []byte(`WARNING: Deprecation warning from numpy
{"event":"subset_count","status":"ok","subset":"train","count":5}
Some other log output
{"event":"summary","status":"ok","message":"done"}
`)

	result, err := ParseHarnessEvents(raw)
	if err != nil {
		t.Fatalf("ParseHarnessEvents returned error: %v", err)
	}
	if len(result.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(result.Events))
	}
	if len(result.Noise) != 2 {
		t.Fatalf("expected 2 noise lines, got %d: %v", len(result.Noise), result.Noise)
	}
	if result.Noise[0] != "WARNING: Deprecation warning from numpy" {
		t.Fatalf("unexpected first noise line: %q", result.Noise[0])
	}
}

func TestHarnessParserSkipsInvalidJSONLines(t *testing.T) {
	raw := []byte(`{this is not json
{"event":"summary","status":"ok","message":"done"}
`)

	result, err := ParseHarnessEvents(raw)
	if err != nil {
		t.Fatalf("ParseHarnessEvents returned error: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}
	if len(result.Noise) != 1 {
		t.Fatalf("expected 1 noise line, got %d", len(result.Noise))
	}
}

func TestHarnessParserFailsOnCompletelyBrokenOutput(t *testing.T) {
	raw := []byte(`Traceback (most recent call last):
  File "harness.py", line 10
ModuleNotFoundError: No module named 'code_loader'
`)

	result, err := ParseHarnessEvents(raw)
	if err == nil {
		t.Fatal("expected error for completely broken output")
	}
	if !strings.Contains(err.Error(), "no valid NDJSON events") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if len(result.Events) != 0 {
		t.Fatalf("expected no events, got %d", len(result.Events))
	}
	if len(result.Noise) != 3 {
		t.Fatalf("expected 3 noise lines, got %d", len(result.Noise))
	}
}

func TestHarnessParserReturnsEmptyOnEmptyInput(t *testing.T) {
	result, err := ParseHarnessEvents([]byte{})
	if err != nil {
		t.Fatalf("ParseHarnessEvents returned error: %v", err)
	}
	if len(result.Events) != 0 {
		t.Fatalf("expected no events, got %d", len(result.Events))
	}
	if len(result.Noise) != 0 {
		t.Fatalf("expected no noise, got %d", len(result.Noise))
	}
}
