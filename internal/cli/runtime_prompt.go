package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/tensorleap/concierge/internal/adapters/inspect"
	"github.com/tensorleap/concierge/internal/core"
)

func confirmRuntimeProfile(
	in io.Reader,
	out io.Writer,
	previous *core.LocalRuntimeProfile,
	resolution inspect.PoetryRuntimeResolution,
) (bool, error) {
	if resolution.Profile == nil {
		return false, fmt.Errorf("runtime profile is missing")
	}
	if len(resolution.SuspiciousReasons) == 0 {
		return true, nil
	}
	if out == nil {
		out = io.Discard
	}

	if _, err := fmt.Fprintln(out, "Runtime Selection"); err != nil {
		return false, err
	}
	if _, err := fmt.Fprintln(out, "Concierge found a Poetry environment, but it looks unusual:"); err != nil {
		return false, err
	}
	for _, reason := range resolution.SuspiciousReasons {
		if _, err := fmt.Fprintf(out, "- %s\n", strings.TrimSpace(reason)); err != nil {
			return false, err
		}
	}
	if previous != nil && strings.TrimSpace(previous.InterpreterPath) != "" {
		if _, err := fmt.Fprintf(out, "Previous interpreter: %s\n", strings.TrimSpace(previous.InterpreterPath)); err != nil {
			return false, err
		}
	}
	if _, err := fmt.Fprintf(out, "Resolved interpreter: %s\n", strings.TrimSpace(resolution.Profile.InterpreterPath)); err != nil {
		return false, err
	}
	if strings.TrimSpace(resolution.Profile.PythonVersion) != "" {
		if _, err := fmt.Fprintf(out, "Resolved Python: %s\n", strings.TrimSpace(resolution.Profile.PythonVersion)); err != nil {
			return false, err
		}
	}

	return promptYesNo(in, out, "Use this Poetry runtime? [Y/n]:", true)
}
