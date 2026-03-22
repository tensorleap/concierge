package cli

import (
	"io"
	"os"

	"golang.org/x/term"
)

func isTerminalWriter(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return (info.Mode() & os.ModeCharDevice) != 0
}

// isSplitScreenCapable returns true if the writer is a TTY wide enough for the
// split-screen step panel.
func isSplitScreenCapable(writer io.Writer, noColor bool) bool {
	if noColor {
		return false
	}
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	fd := int(file.Fd())
	if !term.IsTerminal(fd) {
		return false
	}
	w, _, err := term.GetSize(fd)
	return err == nil && w >= 82
}

// isTUICapable returns true if the writer is a TTY suitable for the full-screen TUI.
func isTUICapable(writer io.Writer, noColor bool) bool {
	if noColor {
		return false
	}
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	fd := int(file.Fd())
	if !term.IsTerminal(fd) {
		return false
	}
	w, _, err := term.GetSize(fd)
	return err == nil && w >= 60
}
