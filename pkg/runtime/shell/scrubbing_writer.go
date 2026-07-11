package shell

import (
	"bytes"
	"io"
)

// =============================================================================
// Types
// =============================================================================

// scrubbingWriter wraps an io.Writer and masks values registered via RegisterSecret before
// writing streamed output to the terminal. Output is buffered by line so a secret split
// across multiple Write calls (e.g. at a buffer boundary) is still matched as a whole
// value; Flush must be called once the source command completes to emit any trailing
// partial line.
type scrubbingWriter struct {
	writer    io.Writer
	scrubFunc func(string) string
	pending   []byte
}

// =============================================================================
// Methods
// =============================================================================

// Write buffers incoming bytes and emits complete lines once their newline arrives, holding
// any trailing partial line until the next Write or Flush. The full length of p is always
// reported consumed so the writer composes cleanly under io.MultiWriter.
func (sw *scrubbingWriter) Write(p []byte) (n int, err error) {
	sw.pending = append(sw.pending, p...)
	for {
		i := bytes.IndexByte(sw.pending, '\n')
		if i < 0 {
			break
		}
		line := string(sw.pending[:i+1])
		sw.pending = sw.pending[i+1:]
		if err := sw.emit(line); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Flush emits any buffered partial line. It is called after the source command finishes so a
// final line without a trailing newline is not dropped.
func (sw *scrubbingWriter) Flush() error {
	if len(sw.pending) == 0 {
		return nil
	}
	line := string(sw.pending)
	sw.pending = nil
	return sw.emit(line)
}

// emit applies the registered-secret denylist to a line and writes the result.
func (sw *scrubbingWriter) emit(line string) error {
	_, err := sw.writer.Write([]byte(sw.scrubFunc(line)))
	return err
}
