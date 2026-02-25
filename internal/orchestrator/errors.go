package orchestrator

import (
	"fmt"

	"github.com/tensorleap/concierge/internal/core"
)

// StageError wraps a stage failure with deterministic stage context.
type StageError struct {
	Stage core.Stage
	Err   error
}

func (e *StageError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Stage == "" {
		return fmt.Sprintf("orchestration stage failed: %v", e.Err)
	}
	return fmt.Sprintf("orchestration %s stage failed: %v", e.Stage, e.Err)
}

func (e *StageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
