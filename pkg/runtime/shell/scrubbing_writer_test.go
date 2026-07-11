package shell

import (
	"bytes"
	"strings"
	"testing"
)

func TestScrubbingWriter_Write(t *testing.T) {
	// setup builds a scrubbingWriter over a buffer with the given registered secrets.
	setup := func(secrets ...string) (*scrubbingWriter, *bytes.Buffer) {
		var sink bytes.Buffer
		scrub := func(in string) string {
			out := in
			for _, sec := range secrets {
				out = strings.ReplaceAll(out, sec, "********")
			}
			return out
		}
		return &scrubbingWriter{writer: &sink, scrubFunc: scrub}, &sink
	}

	t.Run("BuffersPartialLineUntilFlush", func(t *testing.T) {
		// Given a line delivered without a trailing newline
		sw, sink := setup()

		// When a partial line is written
		if _, err := sw.Write([]byte("no newline yet")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}

		// Then nothing is emitted until Flush
		if sink.Len() != 0 {
			t.Errorf("Expected no output before flush, got %q", sink.String())
		}
		if err := sw.Flush(); err != nil {
			t.Fatalf("unexpected flush error: %v", err)
		}
		if sink.String() != "no newline yet" {
			t.Errorf("Expected flushed content, got %q", sink.String())
		}
	})

	t.Run("MasksSecretSplitAcrossWrites", func(t *testing.T) {
		// Given a registered secret delivered across two separate Write calls
		sw, sink := setup("s3cr3t")

		// When the line is written in two chunks that split the secret
		if _, err := sw.Write([]byte("value is s3")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if _, err := sw.Write([]byte("cr3t here\n")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}

		// Then the secret is still masked once the full line is assembled
		if strings.Contains(sink.String(), "s3cr3t") {
			t.Errorf("Expected registered secret masked, got %q", sink.String())
		}
	})

	t.Run("AppliesRegisteredSecretDenylist", func(t *testing.T) {
		// Given a writer with a registered secret
		sw, sink := setup("s3cr3t")

		// When a line containing the secret is written
		if _, err := sw.Write([]byte("value is s3cr3t here\n")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}

		// Then the registered secret is masked
		if strings.Contains(sink.String(), "s3cr3t") {
			t.Errorf("Expected registered secret masked, got %q", sink.String())
		}
	})

	t.Run("ReportsFullInputConsumedEvenWhenMaskingChangesLength", func(t *testing.T) {
		// Given a writer with a registered secret shorter than its mask
		sw, sink := setup("hi")
		input := []byte("value is hi here\n")

		// When the input is written
		n, err := sw.Write(input)

		// Then the writer reports the full input length consumed even though the masked
		// output written to the sink is a different length
		if err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if n != len(input) {
			t.Errorf("Expected %d bytes consumed, got %d", len(input), n)
		}
		if sink.Len() == len(input) {
			t.Errorf("expected masked output length to differ from input length, got equal %q", sink.String())
		}
	})
}
