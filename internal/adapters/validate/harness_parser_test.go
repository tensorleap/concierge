package validate

import "testing"

func TestHarnessParserDecodesRichEvents(t *testing.T) {
	raw := []byte(`{"event":"handler_result","status":"shape_invalid","handler_kind":"input","symbol":"image","subset":"train","sample_id":"0","sample_offset":0,"shape":[128,128,3],"dtype":"float32","expected_shape":[224,224,3],"expected_dtype":"float32","finite":true,"fingerprint":"abc"}
{"event":"subset_count","status":"ok","subset":"validation","count":2}
`)

	events, err := ParseHarnessEvents(raw)
	if err != nil {
		t.Fatalf("ParseHarnessEvents returned error: %v", err)
	}
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
}

func TestHarnessParserAllowsUnknownEventTypes(t *testing.T) {
	raw := []byte(`{"event":"new_event_type","message":"future payload"}
`)

	events, err := ParseHarnessEvents(raw)
	if err != nil {
		t.Fatalf("ParseHarnessEvents returned error: %v", err)
	}
	if len(events) != 1 || events[0].Event != "new_event_type" {
		t.Fatalf("unexpected events: %+v", events)
	}
}
