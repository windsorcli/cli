// The GitRemoteURLNormalizer is a URL normalization utility for git remotes.
// It provides shared normalization behavior for scp-style SSH remotes and schemed URLs,
// The normalizer ensures consistent remote URL handling across runtime consumers,
// preserving valid explicit schemes while defaulting bare remotes to HTTPS.

package git

import (
	"net/url"
	"strings"
)

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

// NormalizeRepositoryURL normalizes a git remote URL for blueprint repository metadata.
// It returns host/path form (for example github.com/org/repo) when host and path can
// be derived from the input. It strips protocol, user info, leading/trailing slashes,
// and a trailing .git suffix from the path. If host/path cannot be derived, it falls
// back to NormalizeRemoteURL.
func NormalizeRepositoryURL(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return ""
	}

	if strings.HasPrefix(normalized, "git@") && strings.Contains(normalized, ":") {
		parts := strings.SplitN(strings.TrimPrefix(normalized, "git@"), ":", 2)
		if len(parts) == 2 {
			host := strings.TrimSpace(parts[0])
			repoPath := normalizeRepositoryPath(parts[1])
			if host != "" && repoPath != "" {
				return host + "/" + repoPath
			}
		}
		return NormalizeRemoteURL(normalized)
	}

	if strings.Contains(normalized, "://") {
		if parsed, err := url.Parse(normalized); err == nil {
			host := strings.TrimSpace(parsed.Host)
			repoPath := normalizeRepositoryPath(parsed.Path)
			if host != "" && repoPath != "" {
				return host + "/" + repoPath
			}
		}
		parts := strings.SplitN(normalized, "://", 2)
		if len(parts) == 2 {
			tail := strings.TrimSpace(parts[1])
			hostPathParts := strings.SplitN(tail, "/", 2)
			if len(hostPathParts) == 2 {
				host := strings.TrimSpace(hostPathParts[0])
				repoPath := normalizeRepositoryPath(hostPathParts[1])
				if host != "" && repoPath != "" {
					return host + "/" + repoPath
				}
			}
		}
		return NormalizeRemoteURL(normalized)
	}

	repoPath := normalizeRepositoryPath(normalized)
	if repoPath == "" {
		return ""
	}

	return repoPath
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

// normalizeRepositoryPath trims separators and strips a trailing .git suffix.
func normalizeRepositoryPath(value string) string {
	trimmed := strings.Trim(strings.TrimSpace(value), "/")
	trimmed = strings.TrimSuffix(trimmed, ".git")
	return strings.Trim(trimmed, "/")
}
