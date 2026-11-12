// The destination rules give each artifact the place a relocation tool copies it to.
// It provides the chart registry layout and the image mirror layout.
// A chart goes to <charts-to>/<chart name>:<chart version>, the layout that helm push writes.
// An image or a blueprint goes to the registries mirror of its upstream host, under its upstream path and tag.
// Two different images or blueprints must not have the same destination.

package inventory

import (
	"fmt"
	"strings"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Helpers
// =============================================================================

// checkDestinations returns an error when two different images or blueprints have the same destination.
// It ignores the digest of a destination, because a relocation tool writes by tag.
// Two artifacts with the same digest are the same artifact.
func checkDestinations(entries []Entry) error {
	artifacts := make(map[string]Entry)
	for _, e := range entries {
		if e.Kind == KindChart || e.Destination == "" {
			continue
		}
		destination, _, _ := strings.Cut(e.Destination, "@")
		other, ok := artifacts[destination]
		if ok && other.Reference != e.Reference && (e.Digest == "" || other.Digest != e.Digest) {
			return werror.New(
				fmt.Sprintf("Artifacts %s and %s have the same destination %s.", other.Reference, e.Reference, destination),
				"Give one upstream registry a registries entry with a different path.",
				nil,
			)
		}
		artifacts[destination] = e
	}
	return nil
}

// destinationFor returns the destination of e. A chart with an exact version goes under chartsTo.
// An image or a blueprint with a mirror for its host goes to that mirror.
// An image or a blueprint already on its mirror host has no destination. Every other entry has no destination.
func destinationFor(e Entry, chartsTo string, mirrors config.Mirrors) string {
	switch e.Kind {
	case KindChart:
		registry := normalizeRegistry(chartsTo)
		if registry == "" || e.Transport == TransportGit || !isExactVersion(e.Version) {
			return ""
		}
		return registry + "/" + e.Name + ":" + e.Version
	case KindImage, KindBlueprint:
		host, path, _ := strings.Cut(e.Reference, "/")
		mirror, ok := mirrors.For(host)
		if !ok {
			return ""
		}
		dest := mirror.Location + "/" + path
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

// isUnderRegistry reports whether the OCI repository of e is in registry.
func isUnderRegistry(e Entry, registry string) bool {
	return registry != "" && e.Transport == TransportOCI && strings.HasPrefix(e.Reference, registry+"/")
}
