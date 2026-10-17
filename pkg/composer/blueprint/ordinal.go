// The ordinal file holds the default facet ordinals.
// It provides OrdinalFromFacetPath and OrdinalFromBasename.
// The default ordinal comes from the prefix of the facet file name.
// The loader uses it when a facet does not set ordinal, so the facet order is deterministic.

package blueprint

import (
	"path/filepath"
	"strings"
)

// =============================================================================
// Helpers
// =============================================================================

// OrdinalFromFacetPath returns the default ordinal for a facet from the basename of its path. A higher
// ordinal wins in a merge. Rules: config-* 100; provider-* or platform-* with "-base" 199; provider-*
// or platform-* 200; option-* or options-* 300; addon-* or addons-* 400; no match 0.
func OrdinalFromFacetPath(path string) int {
	return OrdinalFromBasename(filepath.Base(path))
}

// OrdinalFromBasename returns the default ordinal for a facet file given only its basename
// (e.g. "config-cluster.yaml", "provider-base.yaml"). Used by OrdinalFromFacetPath and by tests.
func OrdinalFromBasename(basename string) int {
	if basename == "" {
		return 0
	}
	if strings.HasPrefix(basename, "config-") {
		return 100
	}
	if strings.HasPrefix(basename, "provider-") || strings.HasPrefix(basename, "platform-") {
		if strings.Contains(basename, "-base") {
			return 199
		}
		return 200
	}
	if strings.HasPrefix(basename, "options-") || strings.HasPrefix(basename, "option-") {
		return 300
	}
	if strings.HasPrefix(basename, "addons-") || strings.HasPrefix(basename, "addon-") {
		return 400
	}
	return 0
}
