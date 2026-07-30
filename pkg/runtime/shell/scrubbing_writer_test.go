package shell

import (
	"bytes"
	"strings"
	"testing"
)

func TestScrubbingWriter_Write(t *testing.T) {
	// setup builds a scrubbingWriter over a buffer with the given registered secrets, holding
	// back a window sized to the longest secret — mirroring how DefaultShell.newScrubbingWriter
	// wires holdBack to maxSecretLen in production.
	setup := func(secrets ...string) (*scrubbingWriter, *bytes.Buffer) {
		var sink bytes.Buffer
		scrub := func(in string) string {
			out := in
			for _, sec := range secrets {
				out = strings.ReplaceAll(out, sec, "********")
			}
			return out
		}
		maxLen := 0
		for _, sec := range secrets {
			if len(sec) > maxLen {
				maxLen = len(sec)
			}
		}
		return &scrubbingWriter{writer: &sink, scrubFunc: scrub, holdBack: func() int { return maxLen }}, &sink
	}

	t.Run("StreamsImmediatelyWhenNoSecretsRegistered", func(t *testing.T) {
		// Given no registered secrets, so there's nothing to hold back for
		sw, sink := setup()

		// When a partial line (no trailing newline) is written
		if _, err := sw.Write([]byte("no newline yet")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}

		// Then it streams straight through without waiting for Flush
		if sink.String() != "no newline yet" {
			t.Errorf("Expected immediate passthrough, got %q", sink.String())
		}
	})

	t.Run("HoldsBackTrailingWindowSizedToLongestSecret", func(t *testing.T) {
		// Given a registered secret, so a hold-back window applies
		sw, sink := setup("s3cr3t12")

		// When a write arrives that doesn't yet exceed the hold-back window
		if _, err := sw.Write([]byte("short")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}

		// Then nothing is released yet
		if sink.Len() != 0 {
			t.Errorf("Expected nothing emitted before exceeding hold-back window, got %q", sink.String())
		}
		if err := sw.Flush(); err != nil {
			t.Fatalf("unexpected flush error: %v", err)
		}
		if sink.String() != "short" {
			t.Errorf("Expected flushed content, got %q", sink.String())
		}
	})

	t.Run("MasksSecretSplitAcrossWrites", func(t *testing.T) {
		// Given a registered secret delivered across two separate Write calls
		sw, sink := setup("s3cr3t12")

		// When the line is written in two chunks that split the secret
		if _, err := sw.Write([]byte("value is s3")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if _, err := sw.Write([]byte("cr3t12 here\n")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if err := sw.Flush(); err != nil {
			t.Fatalf("unexpected flush error: %v", err)
		}

		// Then the secret is still masked once the full line is assembled
		if strings.Contains(sink.String(), "s3cr3t12") {
			t.Errorf("Expected registered secret masked, got %q", sink.String())
		}
	})

	t.Run("MasksSecretFollowedByMoreTrailingDataThanHoldBackWindow", func(t *testing.T) {
		// Given a secret that appears mid-buffer, followed by more trailing bytes than the
		// hold-back window alone would protect — regression case for a naive "hold back N
		// trailing bytes without re-scrubbing the whole buffer first" implementation, which
		// would slice straight through the secret instead of masking it whole.
		sw, sink := setup("supersecretvalue")

		if _, err := sw.Write([]byte("password for supersecretvalue accepted\n")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if err := sw.Flush(); err != nil {
			t.Fatalf("unexpected flush error: %v", err)
		}

		if strings.Contains(sink.String(), "supersecretvalue") {
			t.Errorf("Expected registered secret masked, got %q", sink.String())
		}
	})

	t.Run("AppliesRegisteredSecretDenylist", func(t *testing.T) {
		// Given a writer with a registered secret
		sw, sink := setup("s3cr3t12")

		// When a line containing the secret is written
		if _, err := sw.Write([]byte("value is s3cr3t12 here\n")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if err := sw.Flush(); err != nil {
			t.Fatalf("unexpected flush error: %v", err)
		}

		// Then the registered secret is masked
		if strings.Contains(sink.String(), "s3cr3t12") {
			t.Errorf("Expected registered secret masked, got %q", sink.String())
		}
	})

	t.Run("ReportsFullInputConsumedEvenWhenMaskingChangesLength", func(t *testing.T) {
		// Given a writer with a registered secret shorter than its mask
		sw, _ := setup("shorterthanmask")
		input := []byte("value is shorterthanmask here\n")

		// When the input is written
		n, err := sw.Write(input)

		// Then the writer reports the full input length consumed even though the masked
		// output eventually written to the sink is a different length
		if err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if n != len(input) {
			t.Errorf("Expected %d bytes consumed, got %d", len(input), n)
		}
	})
}
