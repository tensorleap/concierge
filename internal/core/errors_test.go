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

func TestWrapErrorSupportsNotGitRepoKind(t *testing.T) {
	cause := errors.New("fatal: not a git repository")
	err := WrapError(KindNotGitRepo, "snapshot.git_root", cause)

	if !errors.Is(err, ErrNotGitRepo) {
		t.Fatalf("expected errors.Is with ErrNotGitRepo to match, got: %v", err)
	}

	var typed *Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected errors.As to extract *Error, got: %v", err)
	}
	if typed.Kind != KindNotGitRepo {
		t.Fatalf("expected kind %q, got %q", KindNotGitRepo, typed.Kind)
	}
	if typed.Op != "snapshot.git_root" {
		t.Fatalf("expected operation to be preserved, got %q", typed.Op)
	}
}
