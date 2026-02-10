package blueprint

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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

	sourceBlueprintLoaders map[string]BlueprintLoader
	userBlueprintLoader    BlueprintLoader
	composedBlueprint      *blueprintv1alpha1.Blueprint
	initBlueprintURLs      []string
}

// =============================================================================
// Constructor
// =============================================================================

// NewBlueprintHandler creates a new BlueprintHandler with the provided runtime and artifact builder.
// It initializes the internal processor, composer, and writer components with sensible defaults.
// Optional overrides can be passed to replace any of the internal components for testing or
// custom behavior. Panics if runtime or artifactBuilder are nil.
func NewBlueprintHandler(rt *runtime.Runtime, artifactBuilder artifact.Artifact, opts ...*BaseBlueprintHandler) *BaseBlueprintHandler {
	if rt == nil {
		panic("runtime is required")
	}
	if artifactBuilder == nil {
		panic("artifact builder is required")
	}
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

	return handler
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadBlueprint orchestrates the complete blueprint loading pipeline. It loads the user blueprint
// first, then loads all sources from the user's sources array (including "name: template" for local
// _template). Sources are loaded recursively to discover nested sources. Finally, it processes facets
// for all source blueprints and composes them into a single unified blueprint, applying the user
// blueprint as the final override layer. The blueprintURL parameter stores URLs that should be added
// to sources during initialization. These URLs are loaded first so their metadata names can be used.
func (h *BaseBlueprintHandler) LoadBlueprint(blueprintURL ...string) error {
	h.initBlueprintURLs = blueprintURL

	if err := h.loadInitBlueprints(); err != nil {
		return fmt.Errorf("failed to load init blueprints: %w", err)
	}

	if err := h.loadUser(); err != nil {
		return fmt.Errorf("failed to load user blueprint: %w", err)
	}

	if err := h.loadSources(); err != nil {
		return fmt.Errorf("failed to load sources: %w", err)
	}

	if h.runtime != nil && h.runtime.Evaluator != nil {
		if merged := h.getMergedTemplateData(); len(merged) > 0 {
			h.runtime.Evaluator.SetTemplateData(merged)
		}
	}

	if err := h.processAndCompose(); err != nil {
		return fmt.Errorf("failed to compose blueprint: %w", err)
	}

	return nil
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

	h.setRepositoryDefaults()

	return h.writer.Write(h.composedBlueprint, shouldOverwrite, h.initBlueprintURLs...)
}

// GetTerraformComponents returns a copy of the composed blueprint's terraform components with
// Source and FullPath resolved for each component. Source names are expanded to full OCI or Git
// URLs based on the Sources array. Components with a Name or Source are placed in the Windsor
// scratch path (contexts/<context>/terraform/), while local components without a source are
// placed in the project's terraform directory.
func (h *BaseBlueprintHandler) GetTerraformComponents() []blueprintv1alpha1.TerraformComponent {
	if h.composedBlueprint == nil {
		return nil
	}

	components := make([]blueprintv1alpha1.TerraformComponent, len(h.composedBlueprint.TerraformComponents))
	copy(components, h.composedBlueprint.TerraformComponents)

	for i := range components {
		h.resolveComponentSource(&components[i])
		h.resolveComponentFullPath(&components[i])
	}

	return components
}

// GetLocalTemplateData returns all files collected from the template blueprint's directory.
// This includes blueprint.yaml, schema.yaml, features, and any other template files. The data is
// used by the artifact builder when pushing local templates to an OCI registry. Returns nil if
// no template loader exists (e.g., when loading from OCI without a local _template).
func (h *BaseBlueprintHandler) GetLocalTemplateData() (map[string][]byte, error) {
	templateLoader, exists := h.sourceBlueprintLoaders["template"]
	if !exists || templateLoader == nil {
		return nil, nil
	}
	return templateLoader.GetTemplateData(), nil
}

// Generate returns the fully composed blueprint after all sources and user blueprint
// have been merged. This is a simple accessor method that returns the composedBlueprint field.
// The blueprint is already fully processed and composed by LoadBlueprint(). Input expressions
// and substitutions remain in their raw form and are evaluated later by their respective consumers.
func (h *BaseBlueprintHandler) Generate() *blueprintv1alpha1.Blueprint {
	if h.composedBlueprint != nil {
		h.setRepositoryDefaults()
	}
	return h.composedBlueprint
}

// =============================================================================
// Private Methods
// =============================================================================

// getMergedTemplateData returns template file contents from all loaded blueprint sources merged
// into a single map. Used by the evaluator when resolving yaml() and similar file-based
// expressions so OCI-loaded blueprints resolve from in-memory template data. When no loaders
// have template data, falls back to loading from the runtime template root.
func (h *BaseBlueprintHandler) getMergedTemplateData() map[string][]byte {
	merged := make(map[string][]byte)
	for _, loader := range h.sourceBlueprintLoaders {
		data := loader.GetTemplateData()
		for k, v := range data {
			merged[k] = v
		}
	}
	if len(merged) == 0 && h.runtime != nil && h.runtime.TemplateRoot != "" {
		if _, err := h.shims.Stat(h.runtime.TemplateRoot); err == nil {
			loader := NewBlueprintLoader(h.runtime, h.artifactBuilder)
			if loadErr := loader.Load("template", ""); loadErr == nil {
				for k, v := range loader.GetTemplateData() {
					merged[k] = v
				}
			}
		}
	}
	return merged
}

// loadInitBlueprints loads blueprints specified via --blueprint flag before the user blueprint
// is loaded. This ensures the blueprint metadata names are available when creating the minimal
// blueprint. Each blueprint is loaded and stored with a temporary name derived from the URL,
// which will be replaced with the actual metadata name during composition.
func (h *BaseBlueprintHandler) loadInitBlueprints() error {
	if len(h.initBlueprintURLs) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, url := range h.initBlueprintURLs {
		if url == "" {
			continue
		}
		if !strings.HasPrefix(url, "oci://") {
			continue
		}

		wg.Add(1)
		go func(blueprintURL string) {
			defer wg.Done()

			tempName := h.getTempSourceName(blueprintURL)
			loader := NewBlueprintLoader(h.runtime, h.artifactBuilder)
			if err := loader.Load(tempName, blueprintURL); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("failed to load init blueprint '%s': %w", blueprintURL, err))
				mu.Unlock()
				return
			}

			bp := loader.GetBlueprint()
			if bp != nil && bp.Metadata.Name != "" {
				for i := range bp.Sources {
					if bp.Sources[i].Name == tempName {
						bp.Sources[i].Name = bp.Metadata.Name
						break
					}
				}
				for i := range bp.TerraformComponents {
					if bp.TerraformComponents[i].Source == tempName {
						bp.TerraformComponents[i].Source = bp.Metadata.Name
					}
				}
				for i := range bp.Kustomizations {
					if bp.Kustomizations[i].Source == tempName {
						bp.Kustomizations[i].Source = bp.Metadata.Name
					}
				}
				mu.Lock()
				if _, exists := h.sourceBlueprintLoaders[bp.Metadata.Name]; !exists {
					h.sourceBlueprintLoaders[bp.Metadata.Name] = loader
				}
				mu.Unlock()
			}
		}(url)
	}

	wg.Wait()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// getTempSourceName generates a temporary source name from a URL for loading purposes.
// This is only used during initial loading; the actual metadata name will be used after loading.
// Uses artifact.ParseOCIReference for OCI URLs; returns "init-blueprint" when parsing fails or URL is not OCI.
func (h *BaseBlueprintHandler) getTempSourceName(url string) string {
	info, _ := artifact.ParseOCIReference(url)
	if info != nil {
		return info.Name
	}
	return "init-blueprint"
}

// loadUser initializes and loads the user blueprint from the config root directory. The user
// blueprint (blueprint.yaml in the context folder) acts as the final override layer in composition,
// allowing users to select specific components, override values, and add custom configurations.
func (h *BaseBlueprintHandler) loadUser() error {
	h.userBlueprintLoader = NewBlueprintLoader(h.runtime, h.artifactBuilder)
	return h.userBlueprintLoader.Load("user", "")
}

// loadSources iterates through the user blueprint's sources array and loads each source.
// Sources with name "template" are loaded from the local _template directory.
// Sources with OCI URLs are loaded from the registry. Sources are loaded in parallel.
// After loading direct sources, it recursively loads any sources referenced by those blueprints.
// When the user blueprint is nil (e.g. no blueprint.yaml yet), still loads local template if
// present so composition produces kustomizations to apply and existing Flux Kustomizations can be updated.
func (h *BaseBlueprintHandler) loadSources() error {
	userBp := h.userBlueprintLoader.GetBlueprint()
	if userBp == nil {
		if h.runtime != nil && h.runtime.TemplateRoot != "" {
			if _, err := h.shims.Stat(h.runtime.TemplateRoot); err == nil {
				loader := NewBlueprintLoader(h.runtime, h.artifactBuilder)
				if loadErr := loader.Load("template", ""); loadErr == nil && loader.GetBlueprint() != nil {
					h.sourceBlueprintLoaders["template"] = loader
				}
			}
		}
		return nil
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, source := range userBp.Sources {
		if source.Name == "" {
			continue
		}
		if _, exists := h.sourceBlueprintLoaders[source.Name]; exists {
			continue
		}

		wg.Add(1)
		go func(src blueprintv1alpha1.Source) {
			defer wg.Done()

			loader := NewBlueprintLoader(h.runtime, h.artifactBuilder)
			var sourceURL string
			if src.Name == "template" {
				sourceURL = ""
			} else if strings.HasPrefix(src.Url, "oci://") {
				sourceURL = src.Url
			} else {
				return
			}

			if err := loader.Load(src.Name, sourceURL); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("failed to load source '%s': %w", src.Name, err))
				mu.Unlock()
				return
			}

			mu.Lock()
			if _, exists := h.sourceBlueprintLoaders[src.Name]; !exists {
				h.sourceBlueprintLoaders[src.Name] = loader
			}
			mu.Unlock()
		}(source)
	}

	wg.Wait()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	if _, exists := h.sourceBlueprintLoaders["template"]; !exists && h.runtime != nil && h.runtime.TemplateRoot != "" {
		if _, err := h.shims.Stat(h.runtime.TemplateRoot); err == nil {
			loader := NewBlueprintLoader(h.runtime, h.artifactBuilder)
			if loadErr := loader.Load("template", ""); loadErr == nil && loader.GetBlueprint() != nil {
				h.sourceBlueprintLoaders["template"] = loader
			}
		}
	}

	return h.loadNestedSources()
}

// loadNestedSources recursively loads sources from already-loaded source blueprints until no new
// sources are discovered. This ensures sources referenced within OCI blueprints are also loaded.
func (h *BaseBlueprintHandler) loadNestedSources() error {
	processed := make(map[string]bool)

	for {
		var newSources []blueprintv1alpha1.Source

		for name, loader := range h.sourceBlueprintLoaders {
			if processed[name] {
				continue
			}
			processed[name] = true

			bp := loader.GetBlueprint()
			if bp == nil {
				continue
			}

			for _, source := range bp.Sources {
				if source.Name == "" || source.Name == "template" {
					continue
				}
				if _, exists := h.sourceBlueprintLoaders[source.Name]; exists {
					continue
				}
				if !strings.HasPrefix(source.Url, "oci://") {
					continue
				}
				newSources = append(newSources, source)
			}
		}

		if len(newSources) == 0 {
			break
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		var errs []error

		for _, source := range newSources {
			wg.Add(1)
			go func(src blueprintv1alpha1.Source) {
				defer wg.Done()

				loader := NewBlueprintLoader(h.runtime, h.artifactBuilder)
				if err := loader.Load(src.Name, src.Url); err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("failed to load source '%s': %w", src.Name, err))
					mu.Unlock()
					return
				}

				mu.Lock()
				if _, exists := h.sourceBlueprintLoaders[src.Name]; !exists {
					h.sourceBlueprintLoaders[src.Name] = loader
				}
				mu.Unlock()
			}(source)
		}

		wg.Wait()

		if len(errs) > 0 {
			return errors.Join(errs...)
		}
	}

	return nil
}

// processAndCompose merges facets from all loaded source blueprints into a composed blueprint.
// This method processes facets from each loader in parallel. After facet processing, sources are
// merged in the order: initializer blueprints, other sources, then the user blueprint as a final override.
// The composed blueprint is updated on the handler. Terraform input expressions are fully evaluated
// against the merged scope if possible. If a Terraform provider is configured, the composed components
// are registered with the provider. Any errors during facet processing or composition are returned.
func (h *BaseBlueprintHandler) processAndCompose() error {
	var loadersToProcess []BlueprintLoader
	loaderNames := make(map[BlueprintLoader]string)

	for name, loader := range h.sourceBlueprintLoaders {
		if loader.GetBlueprint() != nil {
			loadersToProcess = append(loadersToProcess, loader)
			loaderNames[loader] = name
		}
	}

	var scopeMu sync.Mutex
	collectedScopes := make(map[string]map[string]any)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, loader := range loadersToProcess {
		wg.Add(1)
		go func(l BlueprintLoader, name string) {
			defer wg.Done()
			scope, _, err := h.processLoader(l)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("failed to process facets for '%s': %w", name, err))
				mu.Unlock()
				return
			}
			if scope != nil {
				scopeMu.Lock()
				collectedScopes[name] = scope
				scopeMu.Unlock()
			}
		}(loader, loaderNames[loader])
	}

	wg.Wait()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	initLoaderNames := make([]string, 0, len(h.initBlueprintURLs))
	for _, url := range h.initBlueprintURLs {
		if url != "" && strings.HasPrefix(url, "oci://") {
			initLoaderNames = append(initLoaderNames, h.getTempSourceName(url))
		}
	}
	initNamesSet := make(map[string]bool)
	for _, n := range initLoaderNames {
		initNamesSet[n] = true
	}

	var loaders []BlueprintLoader
	for _, loader := range loadersToProcess {
		if initNamesSet[loader.GetSourceName()] {
			loaders = append(loaders, loader)
		}
	}
	for _, loader := range loadersToProcess {
		if !initNamesSet[loader.GetSourceName()] {
			loaders = append(loaders, loader)
		}
	}
	if h.userBlueprintLoader != nil && h.userBlueprintLoader.GetBlueprint() != nil {
		loaders = append(loaders, h.userBlueprintLoader)
	}

	var mergedScope map[string]any
	scopeMu.Lock()
	for _, loader := range loaders {
		if loader.GetSourceName() == "user" {
			continue
		}
		if scope, ok := collectedScopes[loader.GetSourceName()]; ok && scope != nil {
			mergedScope = MergeConfigMaps(mergedScope, scope)
		}
	}
	scopeMu.Unlock()

	userPath := ""
	if h.userBlueprintLoader != nil {
		if ul, ok := h.userBlueprintLoader.(*BaseBlueprintLoader); ok {
			userPath = ul.GetBlueprintPath()
		}
	}
	composedBp, err := h.composer.Compose(loaders, initLoaderNames, userPath, mergedScope)
	h.composedBlueprint = composedBp
	if err != nil {
		return err
	}

	if h.runtime.Evaluator != nil {
		evalPath := filepath.Join(h.runtime.TemplateRoot, "facets", "_eval.yaml")
		if h.runtime.TemplateRoot == "" {
			evalPath = ""
		}
		for i := range h.composedBlueprint.TerraformComponents {
			if h.composedBlueprint.TerraformComponents[i].Inputs == nil {
				continue
			}
			comp := h.composedBlueprint.TerraformComponents[i]
			evaluated, err := h.runtime.Evaluator.EvaluateMap(comp.Inputs, evalPath, mergedScope, false)
			if err != nil {
				return fmt.Errorf("evaluate terraform inputs for component %q: %w", comp.GetID(), err)
			}
			h.composedBlueprint.TerraformComponents[i].Inputs = evaluated
		}
	}

	h.clearLocalTemplateSource(h.composedBlueprint)

	if h.runtime.TerraformProvider != nil {
		components := h.GetTerraformComponents()
		h.runtime.TerraformProvider.SetTerraformComponents(components)
	}

	return nil
}

// processLoader evaluates all facets from a single blueprint loader.
// Facets with 'when' conditions are evaluated, and only matching facets contribute their
// terraform components and kustomizations. The loader's source name is passed to the processor
// to set the Source field on facet-derived components. Facets are processed directly against
// the loader's blueprint, modifying it in place. Returns the evaluated config scope and block
// order for this loader so the handler can merge scopes from all loaders.
func (h *BaseBlueprintHandler) processLoader(loader BlueprintLoader) (map[string]any, []string, error) {
	facets := loader.GetFacets()
	if len(facets) == 0 {
		return nil, nil, nil
	}

	sourceName := loader.GetSourceName()
	bp := loader.GetBlueprint()
	scope, order, err := h.processor.ProcessFacets(bp, facets, sourceName)
	if err != nil {
		return nil, nil, err
	}
	return scope, order, nil
}

// getConfigValues retrieves the current context's configuration values from the ConfigHandler.
// These values are used during facet evaluation to determine which facets should be included
// based on their 'when' conditions. Returns nil if ConfigHandler is unavailable or if values
// cannot be retrieved, allowing facet processing to continue with empty configuration.
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

// setRepositoryDefaults sets default repository values on the composed blueprint when
// Repository.Url is empty. In dev mode uses http://git.<dns.domain>/git/<project>;
// otherwise uses git remote origin URL. Also sets Ref.Branch to main and, in dev mode,
// SecretName to flux-system when not already set.
func (h *BaseBlueprintHandler) setRepositoryDefaults() {
	if h.composedBlueprint == nil || h.runtime == nil {
		return
	}
	if h.composedBlueprint.Repository.Url != "" {
		devMode := h.runtime.ConfigHandler != nil && h.runtime.ConfigHandler.GetBool("dev")
		if h.composedBlueprint.Repository.Ref == (blueprintv1alpha1.Reference{}) {
			h.composedBlueprint.Repository.Ref = blueprintv1alpha1.Reference{Branch: "main"}
		}
		if devMode && h.composedBlueprint.Repository.SecretName == nil {
			secretName := "flux-system"
			h.composedBlueprint.Repository.SecretName = &secretName
		}
		return
	}

	devMode := h.runtime.ConfigHandler != nil && h.runtime.ConfigHandler.GetBool("dev")

	if devMode {
		domain := h.runtime.ConfigHandler.GetString("dns.domain", "test")
		folder := h.shims.FilepathBase(h.runtime.ProjectRoot)
		if domain != "" && folder != "" && folder != "." {
			h.composedBlueprint.Repository.Url = fmt.Sprintf("http://git.%s/git/%s", domain, folder)
		}
	}

	if h.composedBlueprint.Repository.Url == "" && h.runtime.Shell != nil {
		if gitURL, err := h.runtime.Shell.ExecSilent("git", "config", "--get", "remote.origin.url"); err == nil && gitURL != "" {
			gitURL = h.shims.TrimSpace(gitURL)
			if h.shims.HasPrefix(gitURL, "git@") && h.shims.Contains(gitURL, ":") {
				gitURL = "ssh://" + h.shims.Replace(gitURL, ":", "/", 1)
			}
			h.composedBlueprint.Repository.Url = gitURL
		}
	}

	if h.composedBlueprint.Repository.Url != "" {
		if h.composedBlueprint.Repository.Ref == (blueprintv1alpha1.Reference{}) {
			h.composedBlueprint.Repository.Ref = blueprintv1alpha1.Reference{Branch: "main"}
		}
		if devMode && h.composedBlueprint.Repository.SecretName == nil {
			secretName := "flux-system"
			h.composedBlueprint.Repository.SecretName = &secretName
		}
	}
}

// clearLocalTemplateSource clears Source on terraform components and kustomizations when Source
// is "template" and the template source has no URL (local template). Those items then use the
// blueprint repository (default source) instead of a non-existent template GitRepository.
func (h *BaseBlueprintHandler) clearLocalTemplateSource(blueprint *blueprintv1alpha1.Blueprint) {
	if blueprint == nil {
		return
	}
	var templateIsLocal bool
	for _, s := range blueprint.Sources {
		if blueprintv1alpha1.IsLocalTemplateSource(s) {
			templateIsLocal = true
			break
		}
	}
	if !templateIsLocal {
		return
	}
	for i := range blueprint.TerraformComponents {
		if blueprint.TerraformComponents[i].Source == "template" {
			blueprint.TerraformComponents[i].Source = ""
		}
	}
	for i := range blueprint.Kustomizations {
		if blueprint.Kustomizations[i].Source == "template" {
			blueprint.Kustomizations[i].Source = ""
		}
	}
}

// resolveComponentSource transforms a component's Source field from a source name (e.g., "local")
// into a fully qualified URL based on the matching entry in the blueprint's Sources array. For OCI
// sources, the URL is formatted as oci://registry/repo:tag//path/prefix/component.path. For Git
// sources, the URL is formatted as url//path/prefix/component.path?ref=reference. Components
// without a matching source or with an already-resolved URL are left unchanged.
func (h *BaseBlueprintHandler) resolveComponentSource(component *blueprintv1alpha1.TerraformComponent) {
	if h.composedBlueprint == nil || component.Source == "" {
		return
	}

	for _, source := range h.composedBlueprint.Sources {
		if component.Source != source.Name {
			continue
		}

		pathPrefix := source.PathPrefix
		if pathPrefix == "" {
			pathPrefix = "terraform"
		}

		if source.Name == "template" && source.Url == "" {
			component.Source = ""
			return
		}

		ref := h.getSourceRef(source)

		if strings.HasPrefix(source.Url, "oci://") {
			baseURL := source.Url
			if ref != "" {
				ociPrefix := "oci://"
				urlWithoutProtocol := baseURL[len(ociPrefix):]
				if !strings.Contains(urlWithoutProtocol, ":") {
					baseURL = baseURL + ":" + ref
				}
			}
			component.Source = baseURL + "//" + pathPrefix + "/" + component.Path
		} else {
			component.Source = source.Url + "//" + pathPrefix + "/" + component.Path + "?ref=" + ref
		}
		return
	}
}

// getSourceRef returns the reference (commit, semver, tag, or branch) for a source. It checks
// fields in priority order: Commit → SemVer → Tag → Branch, returning an empty string if none
// are specified. This matches the priority used by the terraform provisioner for consistency.
func (h *BaseBlueprintHandler) getSourceRef(source blueprintv1alpha1.Source) string {
	if source.Ref.Commit != "" {
		return source.Ref.Commit
	}
	if source.Ref.SemVer != "" {
		return source.Ref.SemVer
	}
	if source.Ref.Tag != "" {
		return source.Ref.Tag
	}
	if source.Ref.Branch != "" {
		return source.Ref.Branch
	}
	return ""
}

// resolveComponentFullPath computes and sets the absolute filesystem path where a terraform
// component's module will be located. Named components or those with a Source are placed in
// the Windsor scratch directory under contexts/<context>/terraform/<name|path>. Local components
// without a source are resolved to the project's terraform/<path> directory. This path is used
// by module resolvers and the terraform provisioner to locate module files.
func (h *BaseBlueprintHandler) resolveComponentFullPath(component *blueprintv1alpha1.TerraformComponent) {
	componentID := component.GetID()

	useScratchPath := component.Name != "" || component.Source != ""
	if useScratchPath {
		component.FullPath = filepath.Join(h.runtime.WindsorScratchPath, "terraform", componentID)
	} else {
		component.FullPath = filepath.Join(h.runtime.ProjectRoot, "terraform", componentID)
	}
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintHandler = (*BaseBlueprintHandler)(nil)
