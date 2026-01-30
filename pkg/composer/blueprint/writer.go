package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime"
)

// BlueprintWriter writes the final composed blueprint to contexts/<context>/blueprint.yaml.
type BlueprintWriter interface {
	Write(blueprint *blueprintv1alpha1.Blueprint, overwrite bool, initBlueprintURLs ...string) error
}

// =============================================================================
// Types
// =============================================================================

// BaseBlueprintWriter provides the default implementation of the BlueprintWriter interface.
type BaseBlueprintWriter struct {
	runtime *runtime.Runtime
	shims   *Shims
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

// Write serializes the blueprint to YAML and saves it to blueprint.yaml in the config root.
// Before writing, transient fields are stripped - these include inputs, substitutions, patches,
// parallelism, and Flux timing settings that are used at runtime but should not be stored in
// the user's blueprint. If overwrite is false and the file exists, the write is skipped to
// preserve user modifications. The directory structure is created if it doesn't exist.
// On first initialization (file doesn't exist), writes a minimal blueprint with only sources,
// since terraform and kustomize components come from referenced blueprints. The initBlueprintURLs
// parameter contains blueprint URLs that should be added as sources during initialization.
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
		return fmt.Errorf("error checking file existence: %w", err)
	}

	if !overwrite && fileExists {
		return nil
	}

	if err := w.shims.MkdirAll(filepath.Dir(yamlPath), 0755); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}

	var data []byte
	var err error

	if !fileExists {
		minimalBlueprint := w.createMinimalBlueprint(blueprint, initBlueprintURLs...)
		data, err = w.shims.YamlMarshal(minimalBlueprint)
		if err != nil {
			return fmt.Errorf("error marshalling blueprint: %w", err)
		}
	} else {
		cleanedBlueprint := w.cleanTransientFields(blueprint)
		data, err = w.shims.YamlMarshal(cleanedBlueprint)
		if err != nil {
			return fmt.Errorf("error marshalling blueprint: %w", err)
		}
	}

	header := []byte(`# This file selects and overrides components from underlying blueprint sources.
# To see the fully rendered blueprint, run: windsor show blueprint
`)
	data = append(header, data...)

	if err := w.shims.WriteFile(yamlPath, data, 0644); err != nil {
		return fmt.Errorf("error writing blueprint.yaml: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// cleanTransientFields creates a deep copy of the blueprint with runtime-only fields removed.
// For terraform components: Inputs (used for tfvars generation) and Parallelism (runtime override).
// For kustomizations: Patches, Interval, RetryInterval, Timeout, Substitutions, Wait, Force, and
// Prune are all stripped as they come from feature composition and should not be written to the
// user's blueprint. ConfigMaps are stripped except for user-defined ones (values-common is
// runtime-generated from legacy variables and should not be persisted). Users can override these
// in their blueprint.yaml if explicitly needed.
func (w *BaseBlueprintWriter) cleanTransientFields(blueprint *blueprintv1alpha1.Blueprint) *blueprintv1alpha1.Blueprint {
	if blueprint == nil {
		return nil
	}

	cleaned := blueprint.DeepCopy()

	for i := range cleaned.TerraformComponents {
		cleaned.TerraformComponents[i].Inputs = nil
		cleaned.TerraformComponents[i].Parallelism = nil
		if cleaned.TerraformComponents[i].Enabled != nil && !cleaned.TerraformComponents[i].Enabled.IsExpr {
			if cleaned.TerraformComponents[i].Enabled.Value != nil && *cleaned.TerraformComponents[i].Enabled.Value {
				cleaned.TerraformComponents[i].Enabled = nil
			}
		}
	}

	for i := range cleaned.Kustomizations {
		cleaned.Kustomizations[i].Patches = nil
		cleaned.Kustomizations[i].Interval = nil
		cleaned.Kustomizations[i].RetryInterval = nil
		cleaned.Kustomizations[i].Timeout = nil
		cleaned.Kustomizations[i].Substitutions = nil
		cleaned.Kustomizations[i].Wait = nil
		cleaned.Kustomizations[i].Force = nil
		cleaned.Kustomizations[i].Prune = nil
		if cleaned.Kustomizations[i].Enabled != nil && !cleaned.Kustomizations[i].Enabled.IsExpr {
			if cleaned.Kustomizations[i].Enabled.Value != nil && *cleaned.Kustomizations[i].Enabled.Value {
				cleaned.Kustomizations[i].Enabled = nil
			}
		}
	}

	for i := range cleaned.Sources {
		if cleaned.Sources[i].Install != nil && !cleaned.Sources[i].Install.IsExpr {
			if cleaned.Sources[i].Install.Value != nil && !*cleaned.Sources[i].Install.Value {
				cleaned.Sources[i].Install = nil
			}
		}
	}

	if cleaned.ConfigMaps != nil {
		if _, hasValuesCommon := cleaned.ConfigMaps["values-common"]; hasValuesCommon {
			if len(cleaned.ConfigMaps) == 1 {
				cleaned.ConfigMaps = nil
			} else {
				userConfigMaps := make(map[string]map[string]string)
				for k, v := range cleaned.ConfigMaps {
					if k != "values-common" {
						userConfigMaps[k] = v
					}
				}
				cleaned.ConfigMaps = userConfigMaps
			}
		}
	}

	return cleaned
}

// createMinimalBlueprint creates a minimal blueprint with only sources, metadata, and API version.
// This is used for first-time initialization when components come from referenced blueprints
// rather than being explicitly listed in the user blueprint. The initBlueprintURLs parameter
// contains blueprint URLs that should be added as sources with install: true, using their metadata
// names from the loaded blueprints. When contexts/_template exists, a "template" source (install: true,
// no URL) is included so the blueprint declares local template; no GitRepository/OCIRepository is
// created for it—components from template reference repository: or another source by name.
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
		source := w.findSourceByURL(blueprint, url)
		if source != nil && !existingSourceNames[source.Name] {
			sourceCopy := *source
			sourceCopy.Install = &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}
			minimal.Sources = append(minimal.Sources, sourceCopy)
			existingSourceNames[source.Name] = true
		} else if source == nil {
			sourceName := w.getSourceNameFromURL(url)
			if !existingSourceNames[sourceName] {
				newSource := blueprintv1alpha1.Source{
					Name:    sourceName,
					Url:     url,
					Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false},
				}
				minimal.Sources = append(minimal.Sources, newSource)
				existingSourceNames[sourceName] = true
			}
		}
	}

	if !hasTemplateSource && localTemplateExists {
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
