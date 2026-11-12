// The registries rules read the registries map of a context.
// It provides the merge of the deprecated docker.registries key into the top-level registries key.
// It provides the Mirrors, which give each upstream registry host the mirror that its endpoint sets.
// The image destinations, the source pulls, and the blueprint OCIRepository all read the Mirrors.

package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// AllRegistriesKey is the registries key for each registry host with no entry.
const AllRegistriesKey = "*"

// =============================================================================
// Types
// =============================================================================

// Mirror is the mirror of one upstream registry host.
// Location is the host, the optional port, and the optional path of the endpoint, with no scheme.
// Insecure is true when the endpoint uses http.
type Mirror struct {
	Location string
	Insecure bool
}

// Mirrors holds the mirror of each upstream registry host, keyed by host or by AllRegistriesKey.
// An entry with an empty Location turns off the mirror for its host.
type Mirrors map[string]Mirror

// =============================================================================
// Constructor
// =============================================================================

// NewMirrors returns the mirror of each upstream host in registries.
// The upstream host is the host of the entry's local value, or its key.
// An entry with no endpoint gives its host an empty mirror, so the key * does not apply to that host.
// It returns an error when two entries give different mirrors to one upstream host.
func NewMirrors(registries map[string]cliconfig.Registry) (Mirrors, error) {
	mirrors := make(Mirrors)
	for _, key := range slices.Sorted(maps.Keys(registries)) {
		entry := registries[key]
		mirror := mirrorOf(entry)
		upstream := key
		if entry.Local != nil && *entry.Local != "" {
			upstream = hostOf(*entry.Local)
		}
		other, ok := mirrors[upstream]
		if ok && other.Location != "" && mirror.Location != "" && other != mirror {
			return nil, werror.New(
				fmt.Sprintf("The registries entries give host %s two mirrors: %s and %s.", upstream, other.Location, mirror.Location),
				"Set one mirror for each upstream registry host.",
				nil,
			)
		}
		if mirror.Location != "" || !ok {
			mirrors[upstream] = mirror
		}
	}
	return mirrors, nil
}

// =============================================================================
// Public Methods
// =============================================================================

// For returns the mirror of host. A host with no entry uses the entry of the key *.
// It returns false when host has no mirror, or when the mirror is on host itself.
func (m Mirrors) For(host string) (Mirror, bool) {
	mirror, ok := m[host]
	if !ok {
		mirror = m[AllRegistriesKey]
	}
	if mirror.Location == "" || hostOf(mirror.Location) == host {
		return Mirror{}, false
	}
	return mirror, true
}

// OCIURL returns the oci:// URL of the mirror of url, and true when the mirror uses http.
// The mirror URL keeps the path and the tag of url after the mirror location.
// It returns url unchanged when url is not an oci:// URL or its host has no mirror.
func (m Mirrors) OCIURL(url string) (string, bool) {
	ref, ok := strings.CutPrefix(url, "oci://")
	if !ok {
		return url, false
	}
	host, path, ok := strings.Cut(ref, "/")
	if !ok {
		return url, false
	}
	mirror, ok := m.For(host)
	if !ok {
		return url, false
	}
	return "oci://" + mirror.Location + "/" + path, mirror.Insecure
}

// CheckRegistriesKeys reports whether the context sets only the deprecated docker.registries map.
// It reads the values the context sets, without schema defaults.
// It returns an error when the context sets both registries and docker.registries.
func (c *configHandler) CheckRegistriesKeys() (bool, error) {
	if legacyRegistries(c.data) == nil {
		return false, nil
	}
	if current, _ := c.data["registries"].(map[string]any); len(current) > 0 {
		return false, werror.New(
			"The context sets both registries and docker.registries.",
			"Move the docker.registries entries to registries, then remove docker.registries.",
			nil,
		)
	}
	return true, nil
}

// =============================================================================
// Helpers
// =============================================================================

// mergeRegistries sets registries in values to a copy of docker.registries when only docker.registries is set.
// Facets then read one registries map, whichever key the context sets.
func mergeRegistries(values map[string]any) {
	legacy := legacyRegistries(values)
	if legacy == nil {
		return
	}
	if current, _ := values["registries"].(map[string]any); len(current) > 0 {
		return
	}
	values["registries"] = deepCopyValue(legacy)
}

// legacyRegistries returns the docker.registries map of values, or nil when it is not set.
func legacyRegistries(values map[string]any) map[string]any {
	docker, _ := values["docker"].(map[string]any)
	legacy, _ := docker["registries"].(map[string]any)
	if len(legacy) == 0 {
		return nil
	}
	return legacy
}

// mirrorOf returns the mirror that the endpoint of entry sets.
// An entry with no endpoint has an empty mirror.
func mirrorOf(entry cliconfig.Registry) Mirror {
	if entry.Endpoint == nil {
		return Mirror{}
	}
	endpoint := strings.TrimSpace(*entry.Endpoint)
	return Mirror{
		Location: strings.TrimSuffix(stripScheme(endpoint), "/"),
		Insecure: strings.HasPrefix(strings.ToLower(endpoint), "http://"),
	}
}

// hostOf returns the host of value. It removes a URL scheme and any path.
func hostOf(value string) string {
	host, _, _ := strings.Cut(stripScheme(value), "/")
	return host
}

// stripScheme returns value without its URL scheme.
func stripScheme(value string) string {
	if _, rest, found := strings.Cut(value, "://"); found {
		return rest
	}
	return value
}
