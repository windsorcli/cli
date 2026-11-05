// The Lister is the artifact inventory of one context.
// It provides the list of charts, images and blueprints that a composed blueprint pulls.
// It reads each oci:// blueprint source of the blueprint. It does not render manifests.
// It gives each artifact a destination from --charts-to and the docker.registries mirrors.

package inventory

import (
	"fmt"
	"sort"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Constants
// =============================================================================

// InventoryVersion is the schema version of an Inventory document.
const InventoryVersion = "v1alpha1"

// Kind classifies an artifact by the object that declares it.
type Kind string

const (
	// KindChart is a Helm chart that a chart source names.
	KindChart Kind = "chart"

	// KindImage is a container image that a pod template names.
	KindImage Kind = "image"

	// KindBlueprint is a blueprint artifact that an oci:// blueprint source names.
	KindBlueprint Kind = "blueprint"
)

// Transport tells how a client fetches an artifact from its upstream.
type Transport string

const (
	// TransportOCI is an OCI registry.
	TransportOCI Transport = "oci"

	// TransportHTTP is a Helm repository that serves an index.yaml over HTTP.
	TransportHTTP Transport = "http"

	// TransportGit is a Git repository.
	TransportGit Transport = "git"
)

// =============================================================================
// Types
// =============================================================================

// Inventory is the document that windsor show artifacts prints with -o yaml.
type Inventory struct {
	Version   string  `yaml:"version"`
	Context   string  `yaml:"context,omitempty"`
	Artifacts []Entry `yaml:"artifacts"`
}

// Entry is one external artifact.
// Reference is the repository without a scheme or tag for OCI, and the repository URL for HTTP and Git.
type Entry struct {
	Kind        Kind      `yaml:"kind"`
	Transport   Transport `yaml:"transport"`
	Reference   string    `yaml:"reference"`
	Name        string    `yaml:"name,omitempty"`
	Version     string    `yaml:"version,omitempty"`
	Digest      string    `yaml:"digest,omitempty"`
	Destination string    `yaml:"destination,omitempty"`
	Source      Source    `yaml:"source"`
}

// Source records where the context declares an artifact.
type Source struct {
	Kustomization string `yaml:"kustomization,omitempty"`
	Object        string `yaml:"object,omitempty"`
	File          string `yaml:"file,omitempty"`
}

// Options holds the settings of one List call.
type Options struct {
	ChartsTo string
}

// Result is the inventory and the notes that List writes for the operator.
type Result struct {
	Inventory Inventory
	Notes     []string
}

// BaseLister is the default Lister.
type BaseLister struct {
	runtime *runtime.Runtime
}

// imageReference is a container image reference split into its parts.
// repository holds the registry host, and Docker Hub images use docker.io.
type imageReference struct {
	repository string
	tag        string
	digest     string
}

// =============================================================================
// Interfaces
// =============================================================================

// Lister lists the external artifacts of a composed blueprint.
type Lister interface {
	List(blueprint *blueprintv1alpha1.Blueprint, opts Options) (*Result, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewLister creates a BaseLister. It panics if rt is nil.
func NewLister(rt *runtime.Runtime) *BaseLister {
	if rt == nil {
		panic("runtime is required")
	}
	return &BaseLister{runtime: rt}
}

// =============================================================================
// Public Methods
// =============================================================================

// List returns the artifacts of blueprint. It lists each oci:// blueprint source.
// Each entry gets a destination, and the entries are unique and sorted.
func (l *BaseLister) List(blueprint *blueprintv1alpha1.Blueprint, opts Options) (*Result, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	result := &Result{Inventory: Inventory{Version: InventoryVersion, Context: l.runtime.ContextName}}

	entries := blueprintEntries(blueprint)

	mirrors := l.imageMirrors()
	for i := range entries {
		entries[i].Destination = destinationFor(entries[i], opts.ChartsTo, mirrors)
	}

	result.Inventory.Artifacts = sortEntries(dedupeEntries(entries))
	return result, nil
}

// =============================================================================
// Helpers
// =============================================================================

// blueprintEntries returns one blueprint entry for each oci:// source of blueprint.
func blueprintEntries(blueprint *blueprintv1alpha1.Blueprint) []Entry {
	var entries []Entry
	for _, source := range blueprint.Sources {
		if !strings.HasPrefix(source.Url, "oci://") {
			continue
		}
		ref := parseImageReference(strings.TrimPrefix(source.Url, "oci://"))
		entries = append(entries, Entry{
			Kind:      KindBlueprint,
			Transport: TransportOCI,
			Reference: ref.repository,
			Version:   ref.tag,
			Digest:    ref.digest,
			Source:    Source{Object: "source/" + source.Name},
		})
	}
	return entries
}

// dedupeEntries keeps the first entry for each kind, reference, name, version and digest.
func dedupeEntries(entries []Entry) []Entry {
	seen := make(map[string]bool)
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		key := strings.Join([]string{string(e.Kind), e.Reference, e.Name, e.Version, e.Digest}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}

// sortEntries sorts entries by kind, reference, name and version.
func sortEntries(entries []Entry) []Entry {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Reference != b.Reference {
			return a.Reference < b.Reference
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Version < b.Version
	})
	return entries
}

// parseImageReference splits an image reference into repository, tag and digest. A reference
// with no registry host gets docker.io, and a one-segment Docker Hub path gets library/.
func parseImageReference(ref string) imageReference {
	var out imageReference
	if at := strings.Index(ref, "@"); at >= 0 {
		out.digest = ref[at+1:]
		ref = ref[:at]
	}
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		out.tag = ref[colon+1:]
		ref = ref[:colon]
	}
	first, rest, hasSlash := strings.Cut(ref, "/")
	if !hasSlash || (!strings.ContainsAny(first, ".:") && first != "localhost") {
		first, rest = "docker.io", ref
	}
	if first == "docker.io" && !strings.Contains(rest, "/") {
		rest = "library/" + rest
	}
	out.repository = first + "/" + rest
	return out
}

// isExactVersion reports whether version names one chart version and not a semver range.
func isExactVersion(version string) bool {
	if version == "" || strings.ContainsAny(version, "<>=~^*|, ") {
		return false
	}
	for _, part := range strings.Split(version, ".") {
		if part == "x" || part == "X" {
			return false
		}
	}
	return true
}
