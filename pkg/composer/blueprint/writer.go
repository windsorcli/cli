// The BlueprintWriter is the writer of the user blueprint file.
// It provides Write, which writes contexts/<context>/blueprint.yaml.
// The file holds metadata and sources, and selects components from the sources.
// Write keeps an existing file unless the caller asks to overwrite it.

package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Types
// =============================================================================

// BaseBlueprintWriter provides the default implementation of the BlueprintWriter interface.
type BaseBlueprintWriter struct {
	runtime *runtime.Runtime
	shims   *Shims
}

// =============================================================================
// Interfaces
// =============================================================================

// BlueprintWriter writes the final composed blueprint to contexts/<context>/blueprint.yaml.
type BlueprintWriter interface {
	Write(blueprint *blueprintv1alpha1.Blueprint, overwrite bool, initBlueprintURLs ...string) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewBlueprintWriter creates a new BlueprintWriter that persists blueprints to the filesystem.
// The runtime provides the config root path where blueprint.yaml will be written. Optional
// overrides allow replacing the shims for testing file system operations.
func NewBlueprintWriter(rt *runtime.Runtime) *BaseBlueprintWriter {
	if rt == nil {
		panic("runtime is required")
	}

	return &BaseBlueprintWriter{
		runtime: rt,
		shims:   NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Write saves the blueprint to blueprint.yaml in the config root, in its referential form: metadata,
// repository, and sources, with no components. Components come from the sources; "windsor show
// blueprint" shows the full blueprint. When overwrite is false and the file exists, Write does
// nothing. initBlueprintURLs become sources, and only oci:// sources get install: true.
func (w *BaseBlueprintWriter) Write(blueprint *blueprintv1alpha1.Blueprint, overwrite bool, initBlueprintURLs ...string) error {
	if blueprint == nil {
		return fmt.Errorf("cannot write nil blueprint")
	}

	configRoot := w.runtime.ConfigRoot
	if configRoot == "" {
		return fmt.Errorf("config root is empty")
	}

	yamlPath := filepath.Join(configRoot, "blueprint.yaml")

	fileExists := true
	if _, err := w.shims.Stat(yamlPath); os.IsNotExist(err) {
		fileExists = false
	} else if err != nil {
		return werror.Wrap(err, "error checking file existence")
	}

	if !overwrite && fileExists {
		return nil
	}

	if err := w.shims.MkdirAll(filepath.Dir(yamlPath), 0755); err != nil {
		return werror.Wrap(err, "error creating directory")
	}

	referential := w.createMinimalBlueprint(blueprint, initBlueprintURLs...)
	data, err := w.shims.YamlMarshal(referential)
	if err != nil {
		return werror.Wrap(err, "error marshalling blueprint")
	}

	header := []byte(`# This file selects and overrides components from underlying blueprint sources.
# To see the fully rendered blueprint, run: windsor show blueprint
`)
	data = append(header, data...)

	if err := w.shims.WriteFile(yamlPath, data, 0644); err != nil {
		return werror.Wrap(err, "error writing blueprint.yaml")
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// createMinimalBlueprint builds the referential form: metadata, repository, and sources. It leaves
// out Backend, because the composer takes the backend from the facets on every run.
// initBlueprintURLs become install sources, named by the metadata of their loaded blueprints.
// When contexts/_template exists, it adds a "template" source with install: true and no URL.
// It drops the CRDs of each source, because the composer derives them from the source's facets.
func (w *BaseBlueprintWriter) createMinimalBlueprint(blueprint *blueprintv1alpha1.Blueprint, initBlueprintURLs ...string) *blueprintv1alpha1.Blueprint {
	metadata := blueprint.Metadata
	if w.runtime != nil && w.runtime.ContextName != "" {
		metadata.Name = w.runtime.ContextName
		metadata.Description = fmt.Sprintf("Blueprint for the %s context", w.runtime.ContextName)
	}

	minimal := &blueprintv1alpha1.Blueprint{
		Kind:       blueprint.Kind,
		ApiVersion: blueprint.ApiVersion,
		Metadata:   metadata,
		Sources:    make([]blueprintv1alpha1.Source, 0),
	}
	if blueprint.Repository.Url != "" {
		minimal.Repository = blueprint.Repository
	}

	trueVal := true
	existingSourceNames := make(map[string]bool)
	hasTemplateSource := false
	localTemplateExists := false
	if w.runtime != nil && w.runtime.TemplateRoot != "" {
		if _, err := w.shims.Stat(w.runtime.TemplateRoot); err == nil {
			localTemplateExists = true
		}
	}

	for _, source := range blueprint.Sources {
		if source.Name == "template" {
			if !localTemplateExists {
				continue
			}
			hasTemplateSource = true
			templateSource := blueprintv1alpha1.Source{
				Name:    "template",
				Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false},
			}
			minimal.Sources = append(minimal.Sources, templateSource)
			existingSourceNames[source.Name] = true
			continue
		}
		existingSourceNames[source.Name] = true
		cleanedSource := source
		cleanedSource.Crds = nil
		if cleanedSource.Install != nil && !cleanedSource.Install.IsExpr {
			if cleanedSource.Install.Value != nil && !*cleanedSource.Install.Value {
				cleanedSource.Install = nil
			}
		}
		minimal.Sources = append(minimal.Sources, cleanedSource)
	}

	for _, url := range initBlueprintURLs {
		if url == "" {
			continue
		}
		if localTemplateExists && url == constants.GetEffectiveBlueprintURL() {
			continue
		}
		source := w.findSourceByURL(blueprint, url)
		if source != nil && (source.Name == "template" && source.Url != "") {
			source = nil
		}
		if source != nil && !existingSourceNames[source.Name] {
			sourceCopy := *source
			sourceCopy.Crds = nil
			if strings.HasPrefix(sourceCopy.Url, "oci://") {
				sourceCopy.Install = &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}
			}
			minimal.Sources = append(minimal.Sources, sourceCopy)
			existingSourceNames[source.Name] = true
		} else if source == nil {
			sourceName := w.getSourceNameFromURL(url)
			if !existingSourceNames[sourceName] {
				newSource := blueprintv1alpha1.Source{
					Name: sourceName,
					Url:  url,
				}
				if strings.HasPrefix(url, "oci://") {
					newSource.Install = &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}
				}
				minimal.Sources = append(minimal.Sources, newSource)
				existingSourceNames[sourceName] = true
			}
		}
	}

	explicitBlueprint := false
	for _, u := range initBlueprintURLs {
		if u != "" && u != constants.GetEffectiveBlueprintURL() {
			explicitBlueprint = true
			break
		}
	}
	if !hasTemplateSource && localTemplateExists && !explicitBlueprint {
		templateSource := blueprintv1alpha1.Source{
			Name:    "template",
			Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false},
		}
		minimal.Sources = append(minimal.Sources, templateSource)
	}

	return minimal
}

// findSourceByURL finds a source in the composed blueprint that matches the given URL.
// This is used to get the correct source name from the blueprint's metadata rather than
// deriving it from the URL. Returns nil if no matching source is found.
func (w *BaseBlueprintWriter) findSourceByURL(blueprint *blueprintv1alpha1.Blueprint, url string) *blueprintv1alpha1.Source {
	if blueprint == nil {
		return nil
	}
	normalizedURL := w.normalizeURL(url)
	for i := range blueprint.Sources {
		sourceURL := blueprint.Sources[i].Url
		if blueprint.Sources[i].Ref.Tag != "" {
			sourceURL = fmt.Sprintf("%s:%s", sourceURL, blueprint.Sources[i].Ref.Tag)
		}
		if w.normalizeURL(sourceURL) == normalizedURL {
			return &blueprint.Sources[i]
		}
	}
	return nil
}

// normalizeURL normalizes a URL for comparison by ensuring it has the oci:// prefix
// and handles tag variations.
func (w *BaseBlueprintWriter) normalizeURL(url string) string {
	normalized := strings.TrimPrefix(url, "oci://")
	if !strings.HasPrefix(url, "oci://") {
		normalized = "oci://" + normalized
	}
	return normalized
}

// getSourceNameFromURL extracts a source name from a blueprint URL. For OCI URLs, it uses the
// repository name via artifact.ParseOCIReference. This is a fallback when the source cannot be
// found in the composed blueprint. Returns "blueprint" when parsing fails or URL is not OCI.
func (w *BaseBlueprintWriter) getSourceNameFromURL(url string) string {
	info, _ := artifact.ParseOCIReference(url)
	if info != nil {
		return info.Name
	}
	return "blueprint"
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintWriter = (*BaseBlueprintWriter)(nil)
