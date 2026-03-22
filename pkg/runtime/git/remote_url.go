// The GitRemoteURLNormalizer is a URL normalization utility for git remotes.
// It provides shared normalization behavior for scp-style SSH remotes and schemed URLs,
// The normalizer ensures consistent remote URL handling across runtime consumers,
// preserving valid explicit schemes while defaulting bare remotes to HTTPS.

package git

import "strings"

// =============================================================================
// Public Methods
// =============================================================================

// NormalizeRemoteURL normalizes a git remote URL into an explicit transport URL.
// It converts scp-style SSH remotes (git@host:path) to ssh://git@host/path. If no valid
// URI scheme is present, it prefixes the URL with https://. URLs with explicit valid schemes
// are preserved unchanged.
func NormalizeRemoteURL(value string) string {
	normalized := strings.TrimSpace(value)
	if strings.HasPrefix(normalized, "git@") && strings.Contains(normalized, ":") {
		return "ssh://" + strings.Replace(normalized, ":", "/", 1)
	}
	if hasExplicitURLScheme(normalized) {
		return normalized
	}
	return "https://" + normalized
}

// =============================================================================
// Helpers
// =============================================================================

// hasExplicitURLScheme returns true when the value starts with a valid URI scheme followed by ://.
func hasExplicitURLScheme(value string) bool {
	schemeEnd := strings.Index(value, "://")
	if schemeEnd <= 0 {
		return false
	}

	scheme := value[:schemeEnd]
	if !isASCIILetter(scheme[0]) {
		return false
	}
	for i := 1; i < len(scheme); i++ {
		ch := scheme[i]
		if !isASCIILetter(ch) && (ch < '0' || ch > '9') && ch != '+' && ch != '-' && ch != '.' {
			return false
		}
	}

	return true
}

// isASCIILetter reports whether a byte is an ASCII letter.
func isASCIILetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}
