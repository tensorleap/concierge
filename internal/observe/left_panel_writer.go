package observe

import (
	"bytes"
	"io"
	"unicode/utf8"
)

// LeftPanelWriter wraps an io.Writer and truncates each line to maxWidth visible
// characters. After each newline it calls the onNewline callback, which the
// split-screen renderer uses to redraw the right panel.
type LeftPanelWriter struct {
	inner     io.Writer
	maxWidth  func() int // dynamic so it adapts to terminal resize
	onNewline func()     // called after each \n is flushed
	buf       bytes.Buffer
}

// NewLeftPanelWriter creates a writer that constrains output to the left panel.
func NewLeftPanelWriter(inner io.Writer, maxWidth func() int, onNewline func()) *LeftPanelWriter {
	return &LeftPanelWriter{
		inner:     inner,
		maxWidth:  maxWidth,
		onNewline: onNewline,
	}
}

func (w *LeftPanelWriter) Write(p []byte) (int, error) {
	total := len(p)
	for len(p) > 0 {
		idx := bytes.IndexByte(p, '\n')
		if idx < 0 {
			w.buf.Write(p)
			p = nil
			continue
		}
		w.buf.Write(p[:idx])
		p = p[idx+1:]

		line := w.buf.String()
		w.buf.Reset()

		truncated := truncateVisibleWidth(line, w.maxWidth())
		if _, err := io.WriteString(w.inner, truncated+"\n"); err != nil {
			return total, err
		}
		if w.onNewline != nil {
			w.onNewline()
		}
	}
	return total, nil
}

// Flush writes any remaining buffered content as a partial line.
func (w *LeftPanelWriter) Flush() error {
	if w.buf.Len() == 0 {
		return nil
	}
	line := w.buf.String()
	w.buf.Reset()
	truncated := truncateVisibleWidth(line, w.maxWidth())
	_, err := io.WriteString(w.inner, truncated)
	return err
}

// truncateVisibleWidth cuts a string at maxWidth visible characters, stripping
// ANSI escape sequences from the width count but keeping them in the output.
func truncateVisibleWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	var out bytes.Buffer
	visible := 0
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			// ANSI CSI sequence: copy through to the terminator
			j := i + 2
			for j < len(s) && !isCSITerminator(s[j]) {
				j++
			}
			if j < len(s) {
				j++ // include the terminator byte
			}
			out.WriteString(s[i:j])
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if visible >= maxWidth {
			break
		}
		out.WriteRune(r)
		visible++
		i += size
	}
	return out.String()
}

func isCSITerminator(b byte) bool {
	return b >= 0x40 && b <= 0x7E
}
