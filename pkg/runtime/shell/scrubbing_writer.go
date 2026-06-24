package shell

import (
	"bytes"
	"io"
	"regexp"
	"strings"
)

// =============================================================================
// Types
// =============================================================================

// scrubbingWriter wraps an io.Writer and removes sensitive material from streamed
// output before it reaches the terminal. It applies two layers: scrubFunc masks
// values registered via RegisterSecret (a denylist of known secrets), and redactSecrets
// masks structurally-recognised material — PEM blocks and base64 key/cert/token fields —
// that terraform and other tools emit without windsor ever registering them. Output is
// buffered by line so multi-write secrets are masked as a unit; Flush must be called once
// the source command completes to emit any trailing partial line.
type scrubbingWriter struct {
	writer    io.Writer
	scrubFunc func(string) string
	pending   []byte
	inPEM     bool
}

// =============================================================================
// Patterns
// =============================================================================

// pemInlinePattern matches a complete PEM block rendered on a single physical line.
// Terraform prints embedded keys and certificates with escaped "\n", so the whole block
// lands on one line; the surrounding real newlines are handled by the multi-line path.
var pemInlinePattern = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]+-----.*?-----END [A-Z0-9 ]+-----`)

// secretFieldPattern matches a secret-bearing field name followed by a long base64 value,
// masking the value while leaving the field name and separator visible. The field name matches
// either an identifier carrying a secret word (covering terraform attributes like ca_certificate
// and client_key as well as YAML keys like secret/token) or the bare ca key from Talos machine
// config. The bare id key is deliberately excluded: id is a ubiquitous terraform output field
// and masking a long opaque id is not worth the false positives. The 40-character floor keeps
// ordinary identifiers intact while still catching machine-secret blobs, CA bundles, and private keys.
var secretFieldPattern = regexp.MustCompile(`(?i)([A-Za-z0-9_.-]*(?:token|key|crt|cert|secret|serviceaccount)[A-Za-z0-9_.-]*|\bca\b)(\s*[:=]\s*["']?)([A-Za-z0-9+/]{40,}={0,2})(["']?)`)

// talosTokenPattern matches the Talos bootstrap-token shape (six base32 chars, a dot, then
// sixteen) following a token field, masking the value.
var talosTokenPattern = regexp.MustCompile(`(?i)(\btoken\b\s*[:=]\s*["']?)([a-z0-9]{6}\.[a-z0-9]{16})(["']?)`)

// =============================================================================
// Methods
// =============================================================================

// Write buffers incoming bytes and emits complete lines once their newline arrives, holding
// any trailing partial line until the next Write or Flush. Buffering by line lets redaction
// reason about whole values rather than arbitrary chunk boundaries and lets a PEM block that
// spans several writes be masked as a single unit. The full length of p is always reported
// consumed so the writer composes cleanly under io.MultiWriter.
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
// final line without a trailing newline is not dropped. An unterminated PEM block in progress
// is masked rather than passed through.
func (sw *scrubbingWriter) Flush() error {
	if len(sw.pending) == 0 {
		return nil
	}
	line := string(sw.pending)
	sw.pending = nil
	return sw.emit(line)
}

// emit writes a single line after redaction. While inside a multi-line PEM block it suppresses
// the body and emits a single masked placeholder at the END marker. Otherwise it masks any
// structurally-recognised secret, then applies the registered-secret denylist, before writing.
func (sw *scrubbingWriter) emit(line string) error {
	if sw.inPEM {
		if strings.Contains(line, "-----END ") {
			sw.inPEM = false
			_, err := sw.writer.Write([]byte(sw.scrubFunc("********\n")))
			return err
		}
		return nil
	}
	if strings.Contains(line, "-----BEGIN ") && !strings.Contains(line, "-----END ") {
		sw.inPEM = true
		return nil
	}
	_, err := sw.writer.Write([]byte(sw.scrubFunc(redactSecrets(line))))
	return err
}

// =============================================================================
// Functions
// =============================================================================

// redactSecrets masks structurally-recognised sensitive material in a line: inline PEM blocks,
// base64 values bound to secret-bearing field names, and Talos bootstrap tokens. It is a
// best-effort backstop for secrets that were generated downstream (e.g. by terraform providers)
// and therefore never passed through RegisterSecret.
func redactSecrets(line string) string {
	line = pemInlinePattern.ReplaceAllString(line, "********")
	line = talosTokenPattern.ReplaceAllString(line, "${1}********${3}")
	line = secretFieldPattern.ReplaceAllString(line, "${1}${2}********${4}")
	return line
}
