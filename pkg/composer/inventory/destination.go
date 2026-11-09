// The destination rules give each artifact the place a relocation tool copies it to.
// It provides the chart registry layout and the image mirror layout.
// A chart goes to <charts-to>/<chart name>:<chart version>, the layout that helm push writes.
// An image goes to the docker.registries mirror of its upstream host, under its upstream path and tag.
// Two different images must not have the same destination.

package inventory

import (
	"fmt"
	"sort"
	"strings"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// allRegistriesKey is the docker.registries key for each registry with no entry.
const allRegistriesKey = "*"

// =============================================================================
// Private Methods
// =============================================================================

// imageMirrors returns the mirror location for each upstream registry host in docker.registries.
// The upstream host is the entry's local value, or its key. The key * applies to each host with no entry.
// An entry with no mirror location gives its host an empty location, so the key * does not apply to that host.
// It returns an error when two entries give different mirror locations to one upstream host.
func (l *BaseLister) imageMirrors() (map[string]string, error) {
	registries, _ := l.runtime.ConfigHandler.Get("docker.registries").(map[string]any)
	keys := make([]string, 0, len(registries))
	for key := range registries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	mirrors := make(map[string]string)
	for _, key := range keys {
		entry, _ := registries[key].(map[string]any)
		mirror := mirrorLocation(entry)
		upstream := key
		if local, _ := entry["local"].(string); local != "" {
			upstream = hostOf(local)
		}
		other, ok := mirrors[upstream]
		if ok && other != "" && mirror != "" && other != mirror {
			return nil, werror.New(
				fmt.Sprintf("The docker.registries entries give host %s two mirrors: %s and %s.", upstream, other, mirror),
				"Set one mirror for each upstream registry host.",
				nil,
			)
		}
		if mirror != "" || !ok {
			mirrors[upstream] = mirror
		}
	}
	return mirrors, nil
}

// =============================================================================
// Helpers
// =============================================================================

// mirrorLocation returns the host, port and path that receive the images of entry.
// The location is the endpoint value without its scheme. An entry with no endpoint has no mirror location.
func mirrorLocation(entry map[string]any) string {
	endpoint, _ := entry["endpoint"].(string)
	return strings.TrimSuffix(stripScheme(endpoint), "/")
}

// checkDestinations returns an error when two different images have the same destination.
// It ignores the digest of a destination, because a relocation tool writes by tag.
// Two images with the same digest are the same image.
func checkDestinations(entries []Entry) error {
	images := make(map[string]Entry)
	for _, e := range entries {
		if e.Kind != KindImage || e.Destination == "" {
			continue
		}
		destination, _, _ := strings.Cut(e.Destination, "@")
		other, ok := images[destination]
		if ok && other.Reference != e.Reference && (e.Digest == "" || other.Digest != e.Digest) {
			return werror.New(
				fmt.Sprintf("Images %s and %s have the same destination %s.", other.Reference, e.Reference, destination),
				"Give one upstream registry a docker.registries entry with a different path.",
				nil,
			)
		}
		images[destination] = e
	}
	return nil
}

// destinationFor returns the destination of e. A chart with an exact version goes under chartsTo.
// An image with a mirror for its host goes to that mirror. An image already on its mirror host has no destination.
// Every other entry has no destination.
func destinationFor(e Entry, chartsTo string, mirrors map[string]string) string {
	switch e.Kind {
	case KindChart:
		registry := normalizeRegistry(chartsTo)
		if registry == "" || e.Transport == TransportGit || !isExactVersion(e.Version) {
			return ""
		}
		return registry + "/" + e.Name + ":" + e.Version
	case KindImage:
		host, path, _ := strings.Cut(e.Reference, "/")
		mirror, ok := mirrors[host]
		if !ok {
			mirror = mirrors[allRegistriesKey]
		}
		if mirror == "" || hostOf(mirror) == host {
			return ""
		}
		dest := mirror + "/" + path
		if e.Version != "" {
			dest += ":" + e.Version
		}
		if e.Digest != "" {
			dest += "@" + e.Digest
		}
		return dest
	}
	return ""
}

// normalizeRegistry removes an oci:// prefix and a trailing slash from registry.
func normalizeRegistry(registry string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(registry), "oci://"), "/")
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

// isUnderRegistry reports whether the OCI repository of e is in registry.
func isUnderRegistry(e Entry, registry string) bool {
	return registry != "" && e.Transport == TransportOCI && strings.HasPrefix(e.Reference, registry+"/")
}
