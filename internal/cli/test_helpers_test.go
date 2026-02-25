package cli

import (
	"bytes"
	"testing"
)

func executeCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := NewRootCommand()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), err
}
