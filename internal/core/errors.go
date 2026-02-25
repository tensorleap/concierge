package core

import (
	"errors"
	"fmt"
)

// ErrorKind is a stable machine-readable error classification.
type ErrorKind string

const (
	KindUnknown           ErrorKind = "unknown"
	KindNotGitRepo        ErrorKind = "not_git_repo"
	KindDirtyWorkingTree  ErrorKind = "dirty_working_tree"
	KindMissingLeapYAML   ErrorKind = "missing_leap_yaml"
	KindInvalidEntryFile  ErrorKind = "invalid_entry_file"
	KindStepNotApplicable ErrorKind = "step_not_applicable"
	KindMissingDependency ErrorKind = "missing_dependency"
)

// Error is a typed error used across deterministic orchestration contracts.
type Error struct {
	Kind ErrorKind
	Op   string
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}

	if e.Op == "" {
		return fmt.Sprintf("%s: %v", e.Kind, e.Err)
	}

	return fmt.Sprintf("%s: %s: %v", e.Kind, e.Op, e.Err)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Is allows errors.Is checks by kind (and optionally operation when set on target).
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	if t.Kind != "" && e.Kind != t.Kind {
		return false
	}
	if t.Op != "" && e.Op != t.Op {
		return false
	}
	return true
}

var (
	ErrNotGitRepo        = &Error{Kind: KindNotGitRepo}
	ErrDirtyWorkingTree  = &Error{Kind: KindDirtyWorkingTree}
	ErrMissingLeapYAML   = &Error{Kind: KindMissingLeapYAML}
	ErrInvalidEntryFile  = &Error{Kind: KindInvalidEntryFile}
	ErrStepNotApplicable = &Error{Kind: KindStepNotApplicable}
	ErrMissingDependency = &Error{Kind: KindMissingDependency}
)

// NewError creates a typed error with a message.
func NewError(kind ErrorKind, op, message string) error {
	return &Error{Kind: kind, Op: op, Err: errors.New(message)}
}

// WrapError wraps an underlying error with a typed classification.
func WrapError(kind ErrorKind, op string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Op: op, Err: err}
}

// KindOf returns a typed kind if present, otherwise KindUnknown.
func KindOf(err error) ErrorKind {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Kind
	}
	return KindUnknown
}
