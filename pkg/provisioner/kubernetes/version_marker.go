// Package kubernetes provides Kubernetes resource management functionality.
// This file defines the applied-version marker: the authoritative record of which
// blueprint version a context is currently running, stored as a ConfigMap in the
// gitops namespace. bootstrap and upgrade write it; apply and plan only read it.

package kubernetes

import (
	"encoding/json"
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Constants
// =============================================================================

const (
	// VersionMarkerConfigMapName is the ConfigMap, in the gitops namespace, that holds the marker.
	VersionMarkerConfigMapName = "windsor-version-marker"

	// versionMarkerDataKey is the ConfigMap data key the marker JSON is stored under.
	versionMarkerDataKey = "marker"

	// versionMarkerSchemaVersion is the current marker encoding version.
	versionMarkerSchemaVersion = 1

	// VersionMarkerPhaseIdle marks a context with no upgrade in flight.
	VersionMarkerPhaseIdle = "idle"
)

// =============================================================================
// Types
// =============================================================================

// SourceRef records one blueprint source as applied: its URL and resolved human reference
// (tag/branch/semver/commit). The resolved digest is recorded later, when the version gate
// that consumes it is built.
type SourceRef struct {
	URL string `json:"url,omitempty"`
	Ref string `json:"ref,omitempty"`
}

// VersionMarker is the authoritative record of the blueprint version a context is running.
// Because a context can carry several independently-versioned sources, the "version" is the
// set of source refs, not a single string. Phase tracks an in-flight upgrade (idle for a
// settled context); the target source set is recorded by upgrade and is absent at bootstrap.
type VersionMarker struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	Phase          string               `json:"phase"`
	AppliedSources map[string]SourceRef `json:"appliedSources,omitempty"`
}

// =============================================================================
// Helpers
// =============================================================================

// BuildVersionMarker captures a settled (idle) marker from the applied blueprint: the repository
// source (keyed by the blueprint name) and every declared remote source, each with its resolved
// reference. Local template sources are skipped — they track the working tree and have no version.
// It errors on a source-name collision (two sources sharing a name, or a source matching the
// repository's blueprint name) rather than silently overwriting and misrepresenting what was applied.
func BuildVersionMarker(blueprint *blueprintv1alpha1.Blueprint) (VersionMarker, error) {
	marker := VersionMarker{
		SchemaVersion:  versionMarkerSchemaVersion,
		Phase:          VersionMarkerPhaseIdle,
		AppliedSources: map[string]SourceRef{},
	}
	if blueprint == nil {
		return marker, nil
	}
	addSource := func(name string, ref SourceRef) error {
		if _, exists := marker.AppliedSources[name]; exists {
			return fmt.Errorf("duplicate source name %q; cannot record an unambiguous version marker", name)
		}
		marker.AppliedSources[name] = ref
		return nil
	}
	if blueprint.Repository.Url != "" {
		if err := addSource(blueprint.Metadata.Name, SourceRef{
			URL: blueprint.Repository.Url,
			Ref: effectiveRef(blueprint.Repository.Ref),
		}); err != nil {
			return VersionMarker{}, err
		}
	}
	for _, source := range blueprint.Sources {
		if blueprintv1alpha1.IsLocalTemplateSource(source) {
			continue
		}
		if err := addSource(source.Name, SourceRef{
			URL: source.Url,
			Ref: effectiveRef(source.Ref),
		}); err != nil {
			return VersionMarker{}, err
		}
	}
	return marker, nil
}

// ToConfigMapData encodes the marker as ConfigMap data (a single JSON document).
func (m VersionMarker) ToConfigMapData() (map[string]string, error) {
	encoded, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return map[string]string{versionMarkerDataKey: string(encoded)}, nil
}

// ParseVersionMarker decodes a marker from ConfigMap data, reporting false when no marker is present.
func ParseVersionMarker(data map[string]string) (VersionMarker, bool, error) {
	raw, ok := data[versionMarkerDataKey]
	if !ok {
		return VersionMarker{}, false, nil
	}
	var marker VersionMarker
	if err := json.Unmarshal([]byte(raw), &marker); err != nil {
		return VersionMarker{}, false, err
	}
	if marker.SchemaVersion != versionMarkerSchemaVersion {
		return VersionMarker{}, false, fmt.Errorf("unsupported version marker schema version %d (supported %d)", marker.SchemaVersion, versionMarkerSchemaVersion)
	}
	return marker, true, nil
}

// SourcesEqual reports whether two applied-source sets are identical: the same source names, each
// carrying the same URL and resolved ref. It is the version-equality test the apply gate uses —
// equal sets mean the blueprint matches what is applied, so apply may reconcile in place rather than
// redirect to upgrade. SourceRef is a plain comparable struct, so values compare by field.
func SourcesEqual(a, b map[string]SourceRef) bool {
	if len(a) != len(b) {
		return false
	}
	for name, ref := range a {
		other, ok := b[name]
		if !ok || other != ref {
			return false
		}
	}
	return true
}

// effectiveRef resolves a reference in priority order Commit → SemVer → Tag → Branch, matching the
// composer and terraform provisioner; it returns an empty string when none are set.
func effectiveRef(ref blueprintv1alpha1.Reference) string {
	switch {
	case ref.Commit != "":
		return ref.Commit
	case ref.SemVer != "":
		return ref.SemVer
	case ref.Tag != "":
		return ref.Tag
	case ref.Branch != "":
		return ref.Branch
	}
	return ""
}
