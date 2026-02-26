package validate

import (
	"reflect"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestHarnessParserMapsKnownEvents(t *testing.T) {
	raw := []byte(`{"event":"preprocess","status":"failed","message":"preprocess exploded"}
{"event":"encoder_coverage","status":"incomplete","message":"missing encoder coverage"}
{"event":"validation","status":"failed","message":"validation failed"}
`)

	events, issues, err := ParseHarnessEvents(raw)
	if err != nil {
		t.Fatalf("ParseHarnessEvents returned error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	codes := make([]core.IssueCode, 0, len(issues))
	for _, issue := range issues {
		codes = append(codes, issue.Code)
	}

	expected := []core.IssueCode{
		core.IssueCodeHarnessPreprocessFailed,
		core.IssueCodeHarnessEncoderCoverageIncomplete,
		core.IssueCodeHarnessValidationFailed,
	}
	if !reflect.DeepEqual(codes, expected) {
		t.Fatalf("expected issue codes %v, got %v", expected, codes)
	}
}

func TestHarnessParserUnknownEventFallsBackToUnknownIssue(t *testing.T) {
	raw := []byte(`{"event":"new_event_type","message":"future payload"}
`)

	_, issues, err := ParseHarnessEvents(raw)
	if err != nil {
		t.Fatalf("ParseHarnessEvents returned error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Code != core.IssueCodeUnknown {
		t.Fatalf("expected issue code %q, got %q", core.IssueCodeUnknown, issues[0].Code)
	}
	if issues[0].Severity != core.SeverityInfo {
		t.Fatalf("expected severity %q, got %q", core.SeverityInfo, issues[0].Severity)
	}
}
