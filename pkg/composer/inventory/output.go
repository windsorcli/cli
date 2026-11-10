// The output encoders turn an Inventory into the formats that relocation tools read.
// It provides the Flux Mirror config and the destination<TAB>upstream line format.
// The Flux Mirror config selects exact tags, so a semver selector never picks a sibling tag.
// An artifact with no destination, or one that is already in the chart registry, stays out of the config.

package inventory

import (
	"fmt"
	"regexp"
	"strings"
)

// =============================================================================
// Constants
// =============================================================================

// FluxMirrorAPIVersion is the apiVersion of the Flux Mirror config that FluxMirror writes.
const FluxMirrorAPIVersion = "mirror.plugin.fluxcd.io/v1beta2"

// =============================================================================
// Types
// =============================================================================

// FluxMirrorConfig is a Flux Mirror config.
type FluxMirrorConfig struct {
	APIVersion string               `yaml:"apiVersion"`
	Kind       string               `yaml:"kind"`
	Charts     []FluxMirrorChart    `yaml:"charts,omitempty"`
	Artifacts  []FluxMirrorArtifact `yaml:"artifacts,omitempty"`
}

// FluxMirrorChart copies one chart version from an HTTP Helm repository to an OCI registry.
type FluxMirrorChart struct {
	Name        string `yaml:"name"`
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
	Version     string `yaml:"version"`
}

// FluxMirrorArtifact copies the tags that its selector matches from one OCI repository to another.
type FluxMirrorArtifact struct {
	Source           string             `yaml:"source"`
	Destination      string             `yaml:"destination"`
	Selector         FluxMirrorSelector `yaml:"selector"`
	IncludeReferrers bool               `yaml:"includeReferrers"`
}

// FluxMirrorSelector selects tags by a regular expression.
type FluxMirrorSelector struct {
	Regex FluxMirrorRegex `yaml:"regex"`
}

// FluxMirrorRegex holds the tag pattern of a selector.
type FluxMirrorRegex struct {
	Pattern string `yaml:"pattern"`
}

// =============================================================================
// Public Methods
// =============================================================================

// Upstream returns the pull reference of e. An OCI chart has the oci:// scheme, an HTTP or Git chart
// is its repository URL followed by /<name>:<version>, and an image or blueprint has no scheme.
func (e Entry) Upstream() string {
	ref := e.Reference
	if e.Kind == KindChart && e.Transport != TransportOCI {
		ref += "/" + e.Name
	} else if e.Kind == KindChart {
		ref = "oci://" + ref
	}
	if e.Version != "" {
		ref += ":" + e.Version
	}
	if e.Digest != "" {
		ref += "@" + e.Digest
	}
	return ref
}

// =============================================================================
// Helpers
// =============================================================================

// FluxMirror returns the Flux Mirror config of inv and a note for each artifact it leaves out.
// It writes an HTTP chart as a charts entry with an exact version, and an OCI chart or image as an
// artifacts entry that selects ^<tag>$. The entry includes referrers unless the artifact has
// NoReferrers. It leaves out an artifact with no destination or no tag, and a chart whose upstream
// is already in chartsTo. It needs chartsTo when inv has a chart.
func FluxMirror(inv Inventory, chartsTo string) (FluxMirrorConfig, []string, error) {
	registry := normalizeRegistry(chartsTo)
	cfg := FluxMirrorConfig{APIVersion: FluxMirrorAPIVersion, Kind: "Config"}
	var notes []string
	for _, e := range inv.Artifacts {
		if e.Kind == KindChart && registry == "" {
			return FluxMirrorConfig{}, nil, fmt.Errorf("-o flux-mirror needs --charts-to, because the context pulls chart %s", e.Upstream())
		}
		if e.Kind == KindBlueprint {
			continue
		}
		if isUnderRegistry(e, registry) {
			notes = append(notes, fmt.Sprintf("Chart %s is already in %s, so the config leaves it out.", e.Upstream(), registry))
			continue
		}
		if e.Destination == "" {
			notes = append(notes, fmt.Sprintf("The %s %s has no destination, so the config leaves it out.", e.Kind, e.Upstream()))
			continue
		}
		if e.Kind == KindChart && e.Transport == TransportHTTP {
			cfg.Charts = append(cfg.Charts, FluxMirrorChart{Name: e.Name, Source: e.Reference, Destination: "oci://" + registry, Version: e.Version})
			continue
		}
		if e.Version == "" {
			notes = append(notes, fmt.Sprintf("The %s %s has no tag, so the config leaves it out.", e.Kind, e.Upstream()))
			continue
		}
		destination, _, _ := strings.Cut(e.Destination, "@")
		destination = destination[:strings.LastIndex(destination, ":")]
		cfg.Artifacts = append(cfg.Artifacts, FluxMirrorArtifact{
			Source:           e.Reference,
			Destination:      destination,
			Selector:         FluxMirrorSelector{Regex: FluxMirrorRegex{Pattern: "^" + regexp.QuoteMeta(e.Version) + "$"}},
			IncludeReferrers: !e.NoReferrers,
		})
	}
	return cfg, notes, nil
}

// ListLines returns one destination<TAB>upstream line for each artifact in inv.
// An artifact with no destination has an empty first field.
func ListLines(inv Inventory) string {
	var sb strings.Builder
	for _, e := range inv.Artifacts {
		sb.WriteString(e.Destination)
		sb.WriteString("\t")
		sb.WriteString(e.Upstream())
		sb.WriteString("\n")
	}
	return sb.String()
}
