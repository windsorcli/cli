package blueprint

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime"
)

// BlueprintHandler manages the lifecycle of infrastructure blueprints.
// It orchestrates loading, processing, composing, and writing blueprints.
type BlueprintHandler interface {
	LoadBlueprint(blueprintURL ...string) error
	Write(overwrite ...bool) error
	GetTerraformComponents() []blueprintv1alpha1.TerraformComponent
	GetLocalTemplateData() (map[string][]byte, error)
	Generate() *blueprintv1alpha1.Blueprint
}

// =============================================================================
// Types
// =============================================================================

// BaseBlueprintHandler provides the default implementation of the BlueprintHandler interface.
// It orchestrates the pipeline: Load → Process → Compose → Write.
type BaseBlueprintHandler struct {
	runtime         *runtime.Runtime
	artifactBuilder artifact.Artifact
	processor       BlueprintProcessor
	composer        BlueprintComposer
	writer          BlueprintWriter
	shims           *Shims

	primaryBlueprintLoader BlueprintLoader
	sourceBlueprintLoaders map[string]BlueprintLoader
	userBlueprintLoader    BlueprintLoader

	composedBlueprint *blueprintv1alpha1.Blueprint
}

// =============================================================================
// Constructor
// =============================================================================

// NewBlueprintHandler creates a new BlueprintHandler with the provided runtime and artifact builder.
// It initializes the internal processor, composer, and writer components with sensible defaults.
// Optional overrides can be passed to replace any of the internal components for testing or
// custom behavior. The error return is maintained for API compatibility but always returns nil.
func NewBlueprintHandler(rt *runtime.Runtime, artifactBuilder artifact.Artifact, opts ...*BaseBlueprintHandler) (*BaseBlueprintHandler, error) {
	handler := &BaseBlueprintHandler{
		runtime:                rt,
		artifactBuilder:        artifactBuilder,
		processor:              NewBlueprintProcessor(rt),
		composer:               NewBlueprintComposer(rt),
		writer:                 NewBlueprintWriter(rt),
		shims:                  NewShims(),
		sourceBlueprintLoaders: make(map[string]BlueprintLoader),
	}

	if len(opts) > 0 && opts[0] != nil {
		overrides := opts[0]
		if overrides.processor != nil {
			handler.processor = overrides.processor
		}
		if overrides.composer != nil {
			handler.composer = overrides.composer
		}
		if overrides.writer != nil {
			handler.writer = overrides.writer
		}
		if overrides.shims != nil {
			handler.shims = overrides.shims
		}
	}

	return handler, nil
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadBlueprint orchestrates the complete blueprint loading pipeline. It first loads the primary
// blueprint from a local template directory or OCI artifact, then loads any sources referenced
// by the primary blueprint in parallel. Next, it loads the user's blueprint.yaml from the config
// root (if present) and any additional sources it references. Finally, it processes features for
// all blueprints in parallel and composes them into a single unified blueprint. The optional
// blueprintURL parameter specifies an OCI artifact URL for the primary blueprint; if omitted,
// the handler falls back to the local _template directory.
func (h *BaseBlueprintHandler) LoadBlueprint(blueprintURL ...string) error {
	if err := h.loadPrimary(blueprintURL...); err != nil {
		return fmt.Errorf("failed to load primary blueprint: %w", err)
	}

	if err := h.loadSourcesFromBlueprint(h.primaryBlueprintLoader); err != nil {
		return fmt.Errorf("failed to load sources from primary: %w", err)
	}

	if err := h.loadUser(); err != nil {
		return fmt.Errorf("failed to load user blueprint: %w", err)
	}

	if err := h.loadSourcesFromBlueprint(h.userBlueprintLoader); err != nil {
		return fmt.Errorf("failed to load sources from user: %w", err)
	}

	if err := h.processAndCompose(); err != nil {
		return fmt.Errorf("failed to compose blueprint: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// loadPrimary initializes and loads the primary blueprint loader. If a blueprintURL is provided,
// it loads from the OCI artifact at that URL. Otherwise, it attempts to load from the local
// _template directory. The primary blueprint serves as the base layer in the composition hierarchy,
// providing default terraform components, kustomizations, and feature definitions.
func (h *BaseBlueprintHandler) loadPrimary(blueprintURL ...string) error {
	var sourceURL string
	if len(blueprintURL) > 0 && blueprintURL[0] != "" {
		sourceURL = blueprintURL[0]
	}

	h.primaryBlueprintLoader = NewBlueprintLoader(h.runtime, h.artifactBuilder, "primary", sourceURL)
	return h.primaryBlueprintLoader.Load()
}

// loadUser initializes and loads the user blueprint from the config root directory. The user
// blueprint (blueprint.yaml in the context folder) acts as the final overlay in composition,
// allowing users to select specific components, override values, and add custom configurations.
// Unlike primary and source blueprints, the user blueprint does not contain features.
func (h *BaseBlueprintHandler) loadUser() error {
	h.userBlueprintLoader = NewBlueprintLoader(h.runtime, h.artifactBuilder, "user", "")
	return h.userBlueprintLoader.Load()
}

// loadSourcesFromBlueprint iterates through the Sources array of a loaded blueprint and creates
// a loader for each referenced source. Sources are loaded in parallel using goroutines to improve
// performance when multiple OCI artifacts need to be pulled. Each source is identified by name
// and URL; sources with missing name or URL are skipped, and duplicate sources (already loaded)
// are ignored. Errors from individual source loads are collected and returned as a joined error.
func (h *BaseBlueprintHandler) loadSourcesFromBlueprint(loader BlueprintLoader) error {
	if loader == nil {
		return nil
	}

	bp := loader.GetBlueprint()
	if bp == nil {
		return nil
	}

	var sourcesToLoad []blueprintv1alpha1.Source
	for _, source := range bp.Sources {
		if source.Name == "" || source.Url == "" {
			continue
		}
		if _, exists := h.sourceBlueprintLoaders[source.Name]; exists {
			continue
		}
		sourcesToLoad = append(sourcesToLoad, source)
	}

	if len(sourcesToLoad) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, source := range sourcesToLoad {
		wg.Add(1)
		go func(src blueprintv1alpha1.Source) {
			defer wg.Done()

			sourceBlueprintLoader := NewBlueprintLoader(h.runtime, h.artifactBuilder, src.Name, src.Url)
			if err := sourceBlueprintLoader.Load(); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("failed to load source '%s': %w", src.Name, err))
				mu.Unlock()
				return
			}

			mu.Lock()
			h.sourceBlueprintLoaders[src.Name] = sourceBlueprintLoader
			mu.Unlock()
		}(source)
	}

	wg.Wait()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// processAndCompose evaluates features from all loaded blueprints and merges them into a single
// composed blueprint. Feature processing for each blueprint runs in parallel, with each blueprint's
// features evaluated sequentially (sorted by name) against the current configuration values.
// After all features are processed, the composer merges blueprints in order: sources first, then
// primary, then user overlay. The user blueprint filters the final result to only include
// components explicitly selected by the user.
func (h *BaseBlueprintHandler) processAndCompose() error {
	config := h.getConfigValues()

	var loadersToProcess []BlueprintLoader
	loaderNames := make(map[BlueprintLoader]string)

	if h.primaryBlueprintLoader != nil && h.primaryBlueprintLoader.GetBlueprint() != nil {
		loadersToProcess = append(loadersToProcess, h.primaryBlueprintLoader)
		loaderNames[h.primaryBlueprintLoader] = "primary"
	}

	for name, sourceBlueprintLoader := range h.sourceBlueprintLoaders {
		if sourceBlueprintLoader.GetBlueprint() != nil {
			loadersToProcess = append(loadersToProcess, sourceBlueprintLoader)
			loaderNames[sourceBlueprintLoader] = name
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, loader := range loadersToProcess {
		wg.Add(1)
		go func(l BlueprintLoader, name string) {
			defer wg.Done()
			if err := h.processFeaturesForBlueprintLoader(l, config); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("failed to process features for '%s': %w", name, err))
				mu.Unlock()
			}
		}(loader, loaderNames[loader])
	}

	wg.Wait()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	var loaders []BlueprintLoader
	loaders = append(loaders, loadersToProcess...)

	if h.userBlueprintLoader != nil && h.userBlueprintLoader.GetBlueprint() != nil {
		loaders = append(loaders, h.userBlueprintLoader)
	}

	composedBp, err := h.composer.Compose(loaders)
	if err != nil {
		return err
	}

	h.composedBlueprint = composedBp
	return nil
}

// processFeaturesForBlueprintLoader evaluates all features from a single blueprint loader using
// the provided configuration values. Features with 'when' conditions are evaluated against the
// config, and only matching features contribute their terraform components and kustomizations.
// The resulting components are appended directly to the loader's blueprint, modifying it in place.
func (h *BaseBlueprintHandler) processFeaturesForBlueprintLoader(loader BlueprintLoader, config map[string]any) error {
	features := loader.GetFeatures()
	if len(features) == 0 {
		return nil
	}

	processedBp, err := h.processor.ProcessFeatures(features, config)
	if err != nil {
		return err
	}

	bp := loader.GetBlueprint()
	bp.TerraformComponents = append(bp.TerraformComponents, processedBp.TerraformComponents...)
	bp.Kustomizations = append(bp.Kustomizations, processedBp.Kustomizations...)
	return nil
}

// getConfigValues retrieves the current context's configuration values from the ConfigHandler.
// These values are used during feature evaluation to determine which features should be included
// based on their 'when' conditions. Returns nil if ConfigHandler is unavailable or if values
// cannot be retrieved, allowing feature processing to continue with empty configuration.
func (h *BaseBlueprintHandler) getConfigValues() map[string]any {
	if h.runtime.ConfigHandler == nil {
		return nil
	}
	values, err := h.runtime.ConfigHandler.GetContextValues()
	if err != nil {
		return nil
	}
	return values
}

// Write persists the composed blueprint to blueprint.yaml in the config root directory. Before
// writing, transient fields (inputs, substitutions, parallelism, etc.) are stripped since these
// are used at runtime but should not be stored in the user's blueprint. If overwrite is true,
// an existing file is replaced; if false or omitted, the file is only written if it does not
// already exist, preserving user modifications.
func (h *BaseBlueprintHandler) Write(overwrite ...bool) error {
	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}
	return h.writer.Write(h.composedBlueprint, shouldOverwrite)
}

// GetTerraformComponents returns a copy of the composed blueprint's terraform components with
// FullPath resolved for each component. Components with a Name or Source are placed in the
// Windsor scratch path (contexts/<context>/terraform/), while local components without a source
// are placed in the project's terraform directory. Input expressions remain unevaluated and
// are resolved later by consumers like the terraform env printer.
func (h *BaseBlueprintHandler) GetTerraformComponents() []blueprintv1alpha1.TerraformComponent {
	if h.composedBlueprint == nil {
		return nil
	}

	components := make([]blueprintv1alpha1.TerraformComponent, len(h.composedBlueprint.TerraformComponents))
	copy(components, h.composedBlueprint.TerraformComponents)

	for i := range components {
		h.resolveComponentFullPath(&components[i])
	}

	return components
}

// resolveComponentFullPath computes and sets the absolute filesystem path where a terraform
// component's module will be located. Named components or those with a Source are placed in
// the Windsor scratch directory under contexts/<context>/terraform/<name|path>. Local components
// without a source are resolved to the project's terraform/<path> directory. This path is used
// by module resolvers and the terraform provisioner to locate module files.
func (h *BaseBlueprintHandler) resolveComponentFullPath(component *blueprintv1alpha1.TerraformComponent) {
	var dirName string
	if component.Name != "" {
		dirName = component.Name
	} else {
		dirName = component.Path
	}

	if component.Name != "" || component.Source != "" {
		component.FullPath = filepath.Join(h.runtime.WindsorScratchPath, "terraform", dirName)
	} else {
		component.FullPath = filepath.Join(h.runtime.ProjectRoot, "terraform", dirName)
	}
}

// GetLocalTemplateData returns all files collected from the primary blueprint's template directory.
// This includes blueprint.yaml, schema.yaml, features, and any other template files. The data is
// used by the artifact builder when pushing local templates to an OCI registry. Returns nil if
// no primary loader exists (e.g., when loading from OCI without a local template).
func (h *BaseBlueprintHandler) GetLocalTemplateData() (map[string][]byte, error) {
	if h.primaryBlueprintLoader == nil {
		return nil, nil
	}
	return h.primaryBlueprintLoader.GetTemplateData(), nil
}

// Generate returns the fully composed blueprint after all sources, primary, and user blueprints
// have been merged. The returned blueprint contains the complete set of terraform components and
// kustomizations with all feature processing complete. Input expressions and substitutions remain
// in their raw form and are evaluated later by their respective consumers.
func (h *BaseBlueprintHandler) Generate() *blueprintv1alpha1.Blueprint {
	return h.composedBlueprint
}

var _ BlueprintHandler = (*BaseBlueprintHandler)(nil)
