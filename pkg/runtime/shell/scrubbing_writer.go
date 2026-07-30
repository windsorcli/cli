package shell

import (
	"io"
)

// =============================================================================
// Types
// =============================================================================

// scrubbingWriter wraps an io.Writer and masks values registered via RegisterSecret before
// writing streamed output to the terminal. Rather than buffering by line (which would let a
// secret containing a literal newline, e.g. a PEM key, be split across separate emitted lines
// and never matched whole), every Write re-scrubs the *entire* pending buffer up front — so a
// complete secret occurrence is masked regardless of where it falls, not just at the tail — and
// only then withholds the trailing holdBack() bytes of the (already-scrubbed) result, enough to
// guarantee a secret still arriving across a future Write can't have a partial prefix released
// early. Flush must be called once the source command completes to emit the final withheld bytes.
type scrubbingWriter struct {
	writer    io.Writer
	scrubFunc func(string) string
	holdBack  func() int
	pending   []byte
}

// =============================================================================
// Methods
// =============================================================================

// Write appends incoming bytes to the pending buffer, scrubs the buffer as a whole, and emits
// everything except the trailing holdBack() bytes of the scrubbed result — keeping those held-back
// bytes (mostly raw, since only already-complete matches were replaced) as the new pending, so a
// secret split across Write calls still gets matched once fully arrived. The full length of p is
// always reported consumed so the writer composes cleanly under io.MultiWriter.
func (sw *scrubbingWriter) Write(p []byte) (n int, err error) {
	sw.pending = append(sw.pending, p...)

	hold := 0
	if sw.holdBack != nil {
		hold = sw.holdBack()
	}
	if hold < 0 {
		hold = 0
	}

	scrubbed := sw.scrubFunc(string(sw.pending))
	if emitLen := len(scrubbed) - hold; emitLen > 0 {
		if _, err := sw.writer.Write([]byte(scrubbed[:emitLen])); err != nil {
			return 0, err
		}
		sw.pending = []byte(scrubbed[emitLen:])
	} else {
		sw.pending = []byte(scrubbed)
	}

	return len(p), nil
}

// Flush emits any remaining withheld bytes, scrubbing them once more first since Write only
// scrubs on arrival of new data. It is called after the source command finishes so trailing
// output that never exceeded the hold-back window is not dropped.
func (sw *scrubbingWriter) Flush() error {
	if len(sw.pending) == 0 {
		return nil
	}
	out := sw.scrubFunc(string(sw.pending))
	sw.pending = nil
	_, err := sw.writer.Write([]byte(out))
	return err
}
