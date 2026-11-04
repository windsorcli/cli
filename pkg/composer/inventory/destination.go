// The destination rules give each artifact the place a relocation tool copies it to.
// It provides the chart registry layout and the image mirror layout.
// A chart goes to <charts-to>/<chart name>:<chart version>, the layout that helm push writes.
// An image goes to the docker.registries mirror of its upstream host, under its upstream path and tag.

package inventory

import (
	"fmt"
	"strings"
)

// =============================================================================
// Private Methods
// =============================================================================

// imageMirrors returns the mirror host for each upstream registry host in docker.registries.
// The upstream host is the entry's local value, or its key. The mirror host is the entry's
// hostname value, or its key, with :hostport when set. A value with a URL scheme counts as its host.
// An entry whose mirror is its upstream is left out.
func (l *BaseLister) imageMirrors() map[string]string {
	registries, _ := l.runtime.ConfigHandler.Get("docker.registries").(map[string]any)
	mirrors := make(map[string]string)
	for key, raw := range registries {
		entry, _ := raw.(map[string]any)
		upstream := key
		if local, _ := entry["local"].(string); local != "" {
			upstream = hostOf(local)
		}
		mirror := key
		if hostname, _ := entry["hostname"].(string); hostname != "" {
			mirror = hostOf(hostname)
		}
		switch port := entry["hostport"].(type) {
		case int:
			if port != 0 {
				mirror = fmt.Sprintf("%s:%d", mirror, port)
			}
		case float64:
			if port != 0 {
				mirror = fmt.Sprintf("%s:%d", mirror, int(port))
			}
		}
		if mirror != upstream {
			mirrors[upstream] = mirror
		}
	}
	return mirrors
}

// =============================================================================
// Helpers
// =============================================================================

// destinationFor returns the destination of e. A chart with an exact version goes under chartsTo.
// An image with a mirror for its host goes to that mirror. Every other entry has no destination.
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
	if _, rest, found := strings.Cut(value, "://"); found {
		value = rest
	}
	host, _, _ := strings.Cut(value, "/")
	return host
}

// isUnderRegistry reports whether the OCI repository of e is in registry.
func isUnderRegistry(e Entry, registry string) bool {
	return registry != "" && e.Transport == TransportOCI && strings.HasPrefix(e.Reference, registry+"/")
}
