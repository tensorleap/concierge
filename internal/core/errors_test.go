package core

import (
	"errors"
	"testing"
)

func TestWrapErrorSupportsTypedChecks(t *testing.T) {
	cause := errors.New("leap.yaml missing")
	err := WrapError(KindMissingLeapYAML, "inspect.leapConfig", cause)

	if !errors.Is(err, ErrMissingLeapYAML) {
		t.Fatalf("expected errors.Is with ErrMissingLeapYAML to match, got: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("expected wrapped cause to remain discoverable, got: %v", err)
	}

	var typed *Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected errors.As to extract *Error, got: %v", err)
	}
	if typed.Kind != KindMissingLeapYAML {
		t.Fatalf("expected kind %q, got %q", KindMissingLeapYAML, typed.Kind)
	}
	if typed.Op != "inspect.leapConfig" {
		t.Fatalf("expected operation to be preserved, got %q", typed.Op)
	}
}

func TestKindOfReturnsUnknownForUntypedErrors(t *testing.T) {
	if kind := KindOf(errors.New("plain error")); kind != KindUnknown {
		t.Fatalf("expected KindUnknown, got %q", kind)
	}
}
