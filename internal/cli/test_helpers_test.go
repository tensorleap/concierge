package cli

import (
	"bytes"
	"strings"
	"testing"
)

func executeCLI(t *testing.T, args ...string) (string, error) {
	return executeCLIWithInput(t, "", args...)
}

func executeCLIWithInput(t *testing.T, input string, args ...string) (string, error) {
	t.Helper()

	cmd := NewRootCommand()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), err
}
