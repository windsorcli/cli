package blueprint

import (
	"path/filepath"
	"strings"
)

// OrdinalFromFacetPath returns the default ordinal for a facet based on its file path.
// The basename of the path is used to match prefix rules. When the facet does not set
// ordinal explicitly, the loader uses this to assign a default so that facet processing
// order is deterministic (config first, then provider/platform base, then provider/platform,
// then options, then addons). Higher ordinal means higher precedence when merging.
// Rules: config-* 100; provider-* or platform-* with "-base" in name 199; provider-* or platform-* 200;
// option-* or options-* 300; addon-* or addons-* 400; no match 0.
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
