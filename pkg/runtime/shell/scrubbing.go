// The secret scrubbing methods are the DefaultShell methods that hide secrets in command output.
// It provides the secret registration and the replacement of each registered value with asterisks.
// The scrub matches exact substrings only, so an encoded form of a secret does not match.
// The scrubbingWriter in scrubbing_writer.go uses these methods to scrub streamed output.

package shell

import (
	"io"
	"slices"
	"strings"
)

// =============================================================================
// Constants
// =============================================================================

// minRegisteredSecretLength is the shortest value RegisterSecret will add to the scrub denylist.
// Short common words/values ("true", "1", "admin", "dev") carry no real confidentiality, and
// scrubbing them would mangle unrelated output every time that substring happens to appear.
const minRegisteredSecretLength = 8

// =============================================================================
// Public Methods
// =============================================================================

// RegisterSecret adds a secret value to the internal list of secrets that will be scrubbed from all
// command output. Values shorter than minRegisteredSecretLength are ignored, since scrubbing a short
// common substring mangles unrelated output while providing no real confidentiality. Duplicate values
// are automatically filtered out to maintain list efficiency. Safe for concurrent use with scrubString.
func (s *DefaultShell) RegisterSecret(value string) {
	if len(value) < minRegisteredSecretLength {
		return
	}
	s.addSecret(value)
}

// RegisterResolvedSecret registers a value that a secret provider resolved. It skips the length floor
// of RegisterSecret, because a resolved value is a deliberate secret. Empty values are ignored.
// A short value hides every occurrence of that text in command output, including inside other words.
func (s *DefaultShell) RegisterResolvedSecret(value string) {
	if value == "" {
		return
	}
	s.addSecret(value)
}

// =============================================================================
// Private Methods
// =============================================================================

// addSecret appends value to the scrub list unless it is already present.
func (s *DefaultShell) addSecret(value string) {
	s.secretsMu.Lock()
	defer s.secretsMu.Unlock()

	if slices.Contains(s.secrets, value) {
		return
	}

	s.secrets = append(s.secrets, value)
}

// scrubString replaces all registered secret values with fixed "********" strings for security.
// It processes the input string and replaces any occurrence of registered secrets with asterisks.
// This method is used internally by all command execution methods to prevent secret leakage in
// output. Safe for concurrent use with RegisterSecret. Best-effort, exact-substring only: a
// base64/URL-encoded/JSON-escaped transform of a registered value will not match.
func (s *DefaultShell) scrubString(input string) string {
	s.secretsMu.Lock()
	secrets := slices.Clone(s.secrets)
	s.secretsMu.Unlock()

	result := input
	for _, secret := range secrets {
		if secret != "" {
			result = strings.ReplaceAll(result, secret, "********")
		}
	}

	return result
}

// scrubStringWithMaxLen scrubs input identically to scrubString and, from that same locked
// snapshot of registered secrets, also returns the longest secret's length. scrubbingWriter uses
// this (rather than separate scrubString/maxSecretLen calls) so the hold-back window it sizes is
// always computed from the exact secret set the scrub itself used — a RegisterSecret call landing
// between two independently-locked calls could otherwise leave the window stale for one Write.
func (s *DefaultShell) scrubStringWithMaxLen(input string) (string, int) {
	s.secretsMu.Lock()
	secrets := slices.Clone(s.secrets)
	s.secretsMu.Unlock()

	maxLen := 0
	result := input
	for _, secret := range secrets {
		if secret != "" {
			result = strings.ReplaceAll(result, secret, "********")
		}
		if len(secret) > maxLen {
			maxLen = len(secret)
		}
	}

	return result, maxLen
}

// newScrubbingWriter builds a scrubbingWriter over w bound to this shell's registered secrets.
func (s *DefaultShell) newScrubbingWriter(w io.Writer) *scrubbingWriter {
	return &scrubbingWriter{writer: w, scrubWithMaxLen: s.scrubStringWithMaxLen}
}
