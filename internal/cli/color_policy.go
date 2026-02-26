package cli

import (
	"io"
	"os"
	"strings"
)

var cliGetenv = os.Getenv
var cliIsTerminalWriter = isTerminalWriter

func cliColorEnabled(writer io.Writer, noColor bool) bool {
	if noColor {
		return false
	}
	if strings.TrimSpace(cliGetenv("NO_COLOR")) != "" {
		return false
	}
	return cliIsTerminalWriter(writer)
}

func setCLIColorDepsForTest(getenv func(string) string, isTerminal func(io.Writer) bool) func() {
	previousGetenv := cliGetenv
	previousIsTerminal := cliIsTerminalWriter
	cliGetenv = getenv
	cliIsTerminalWriter = isTerminal
	return func() {
		cliGetenv = previousGetenv
		cliIsTerminalWriter = previousIsTerminal
	}
}
