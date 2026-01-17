package blueprint

import (
	"fmt"
	"os"
	"path/filepath"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
)

// BlueprintWriter writes the final composed blueprint to contexts/<context>/blueprint.yaml.
type BlueprintWriter interface {
	Write(blueprint *blueprintv1alpha1.Blueprint, overwrite bool) error
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
func (w *BaseBlueprintWriter) Write(blueprint *blueprintv1alpha1.Blueprint, overwrite bool) error {
	if blueprint == nil {
		return fmt.Errorf("cannot write nil blueprint")
	}

	configRoot := w.runtime.ConfigRoot
	if configRoot == "" {
		return fmt.Errorf("config root is empty")
	}

	yamlPath := filepath.Join(configRoot, "blueprint.yaml")

	if !overwrite {
		if _, err := w.shims.Stat(yamlPath); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("error checking file existence: %w", err)
		}
	}

	if err := w.shims.MkdirAll(filepath.Dir(yamlPath), 0755); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}

	cleanedBlueprint := w.cleanTransientFields(blueprint)

	data, err := w.shims.YamlMarshal(cleanedBlueprint)
	if err != nil {
		return fmt.Errorf("error marshalling blueprint: %w", err)
	}

	header := []byte(`# This file selects and overrides components from the underlying blueprint sources
# (_template directory and/or OCI artifacts in 'sources'). To see the fully
# rendered blueprint with all computed fields, run: windsor show blueprint

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

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintWriter = (*BaseBlueprintWriter)(nil)
