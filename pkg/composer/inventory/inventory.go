// The Lister is the artifact inventory of one context.
// It provides the list of charts, images and blueprints that a composed blueprint pulls.
// It renders each Flux Kustomization with the flux CLI and reads the source objects and pod templates.
// It gives each artifact a destination from --charts-to and the docker.registries mirrors.

package inventory

import (
	"fmt"
	"sort"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
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
	All      bool
}

// Result is the inventory and the notes that List writes for the operator.
type Result struct {
	Inventory Inventory
	Notes     []string
}

// BaseLister is the default Lister.
type BaseLister struct {
	runtime         *runtime.Runtime
	artifactBuilder artifact.Artifact
	shims           *Shims
	extractors      []Extractor
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

// NewLister creates a BaseLister with the built-in extractors. It panics if rt or artifactBuilder is nil.
// A non-nil field in an opts entry replaces the default.
func NewLister(rt *runtime.Runtime, artifactBuilder artifact.Artifact, opts ...*BaseLister) *BaseLister {
	if rt == nil {
		panic("runtime is required")
	}
	if artifactBuilder == nil {
		panic("artifact builder is required")
	}

	lister := &BaseLister{
		runtime:         rt,
		artifactBuilder: artifactBuilder,
		shims:           NewShims(),
	}

	if len(opts) > 0 && opts[0] != nil {
		if opts[0].shims != nil {
			lister.shims = opts[0].shims
		}
		if opts[0].extractors != nil {
			lister.extractors = opts[0].extractors
		}
	}

	if lister.extractors == nil {
		lister.extractors = []Extractor{NewFluxInstanceExtractor(rt, lister.shims)}
	}

	return lister
}

// =============================================================================
// Public Methods
// =============================================================================

// List returns the artifacts of blueprint. Without opts.All it renders every Kustomization the
// context applies and reads the rendered objects. With opts.All it reads the artifact-manifest.yaml
// of each blueprint source. Each entry gets a destination, and the entries are unique and sorted.
// A deferred value in a chart source or an image reference is an error.
func (l *BaseLister) List(blueprint *blueprintv1alpha1.Blueprint, opts Options) (*Result, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	result := &Result{Inventory: Inventory{Version: InventoryVersion, Context: l.runtime.ContextName}}

	var entries []Entry
	var err error
	if opts.All {
		entries, err = l.listFromManifests(blueprint, result)
	} else {
		entries, err = l.listFromRender(blueprint, result)
	}
	if err != nil {
		return nil, err
	}

	entries = append(entries, blueprintEntries(blueprint)...)

	mirrors := l.imageMirrors()
	for i := range entries {
		entries[i].Destination = destinationFor(entries[i], opts.ChartsTo, mirrors)
	}

	result.Inventory.Artifacts = sortEntries(dedupeEntries(entries))
	return result, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// listFromRender renders each Kustomization that the context applies, runs the extractors on the
// rendered objects, and reads the chart sources and images. It needs the flux CLI only when the
// blueprint has a Kustomization to render.
func (l *BaseLister) listFromRender(blueprint *blueprintv1alpha1.Blueprint, result *Result) ([]Entry, error) {
	var targets []blueprintv1alpha1.Kustomization
	for _, k := range blueprint.AllKustomizations() {
		if k.DestroyOnly != nil && *k.DestroyOnly {
			continue
		}
		targets = append(targets, k)
	}
	if len(targets) == 0 {
		return nil, nil
	}

	if err := l.runtime.ToolsManager.CheckFlux(); err != nil {
		return nil, err
	}

	var objects []renderedObject
	for _, k := range targets {
		rendered, err := l.renderKustomization(blueprint, k)
		if err != nil {
			return nil, err
		}
		objects = append(objects, rendered...)
	}

	extracted, err := l.runExtractors(objects)
	if err != nil {
		return nil, err
	}
	objects = append(objects, extracted...)

	entries, notes := scanObjects(objects)
	result.Notes = append(result.Notes, notes...)
	if err := checkDeferred(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// runExtractors returns the objects that each extractor derives from a rendered object of its kind.
// Each derived object keeps the Kustomization of its parent and names the parent as its object.
func (l *BaseLister) runExtractors(objects []renderedObject) ([]renderedObject, error) {
	var derived []renderedObject
	for _, obj := range objects {
		for _, extractor := range l.extractors {
			if !extractor.Matches(obj.object) {
				continue
			}
			out, err := extractor.Extract(obj.object)
			if err != nil {
				return nil, werror.Wrap(err, "failed to extract the images of %s in kustomization %q", obj.id(), obj.kustomization)
			}
			for _, o := range out {
				derived = append(derived, renderedObject{kustomization: obj.kustomization, object: o, parent: obj.id()})
			}
		}
	}
	return derived, nil
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

// checkDeferred returns an error for the first entry whose upstream holds the deferred marker.
func checkDeferred(entries []Entry) error {
	for _, e := range entries {
		if strings.Contains(e.Reference+e.Name+e.Version+e.Digest, deferredMarker) {
			return werror.New(
				fmt.Sprintf("the %s of %s in kustomization %q uses a value that is not known before apply", e.Kind, e.Source.Object, e.Source.Kustomization),
				"Give the substitution a value that does not come from a Terraform output.", nil)
		}
	}
	return nil
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
