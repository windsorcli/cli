// The BlueprintHandler is the entry point for blueprint loading, composition, and writing.
// It provides LoadBlueprint, which runs the load, process, and compose stages in order.
// It provides source retargeting and upgrades, and access to the composed and resolved blueprint.
// The loading, deferred-value, and repository steps are in the handler_* companion files.

package blueprint

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime"
)

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
	composedScope          map[string]any
	deferredPathsMu        sync.Mutex
	deferredPaths          map[string]bool
	traceCollector         TraceCollector
	initBlueprintURLs      []string
	skipValidation         bool
}

// SourceUpgrade records one source moved to a newer version by UpgradeSourcesToLatest.
type SourceUpgrade struct {
	Name string
	From string
	To   string
}

// =============================================================================
// Interfaces
// =============================================================================

// BlueprintHandler manages the lifecycle of infrastructure blueprints.
// It orchestrates loading, processing, composing, and writing blueprints.
type BlueprintHandler interface {
	LoadBlueprint(blueprintURL ...string) error
	SetSkipValidation(skip bool)
	Write(overwrite ...bool) error
	RetargetSource(name, url string) (string, error)
	UpgradeSourcesToLatest() ([]SourceUpgrade, error)
	GetDeclaredSources() ([]blueprintv1alpha1.Source, error)
	GetTerraformComponents() []blueprintv1alpha1.TerraformComponent
	GetLocalTemplateData() (map[string][]byte, error)
	Generate() *blueprintv1alpha1.Blueprint
	GenerateResolved() (*blueprintv1alpha1.Blueprint, error)
	Explain(path string) (*ExplainTrace, error)
	GetDeferredPaths() map[string]bool
	DeferredSubstitutionsInScope(scope map[string]bool) bool
	GetConfigScope() map[string]any
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

// SetTraceCollector enables opt-in trace collection for the explain command. When set, the
// collector is propagated to the processor at the start of composition so expression scope
// references and nested paths are recorded during facet processing. Pass nil to disable.
func (h *BaseBlueprintHandler) SetTraceCollector(tc TraceCollector) {
	h.traceCollector = tc
}

// LoadBlueprint runs the full pipeline: it loads the init blueprints from blueprintURL, the user
// blueprint, and all sources, nested sources included. It then processes the facets of each source
// and composes one blueprint, with the user blueprint applied last. Each call clears the loaded
// sources first, so a call after RetargetSource pulls every source again.
func (h *BaseBlueprintHandler) LoadBlueprint(blueprintURL ...string) error {
	h.initBlueprintURLs = blueprintURL
	h.deferredPathsMu.Lock()
	h.deferredPaths = make(map[string]bool)
	h.deferredPathsMu.Unlock()
	h.sourceBlueprintLoaders = make(map[string]BlueprintLoader)

	if err := h.loadInitBlueprints(); err != nil {
		return werror.Wrap(err, "failed to load init blueprints")
	}

	if err := h.loadUser(); err != nil {
		return werror.Wrap(err, "failed to load user blueprint")
	}

	if err := h.loadSources(); err != nil {
		return werror.Wrap(err, "failed to load sources")
	}

	if h.runtime != nil && h.runtime.Evaluator != nil {
		if merged := h.getMergedTemplateData(); len(merged) > 0 {
			h.runtime.Evaluator.SetTemplateData(merged)
		}
	}

	if err := h.processAndCompose(); err != nil {
		var reqErr *RequirementsError
		if errors.As(err, &reqErr) {
			return err
		}
		return werror.Wrap(err, "failed to compose blueprint")
	}

	if !h.skipValidation {
		if err := ValidateComposedBlueprint(h.composedBlueprint); err != nil {
			return err
		}
	}

	return nil
}

// SetSkipValidation turns off structural validation in LoadBlueprint when skip is true. The default
// is false. Teardown commands (destroy, down, env) set it, so an operator can still tear down or
// inspect a blueprint that fails validation. Deploy commands keep validation, so a structural mistake
// shows when it is made. Destroy order does not depend on validation, because the destroy paths do
// not use blueprint position.
func (h *BaseBlueprintHandler) SetSkipValidation(skip bool) {
	h.skipValidation = skip
}

// Write persists the blueprint to blueprint.yaml in the config root directory. It always writes
// the referential form (metadata, repository, sources only). If overwrite is true, an existing
// file is replaced; if false or omitted, the file is only written if it does not already exist,
// preserving user modifications.
func (h *BaseBlueprintHandler) Write(overwrite ...bool) error {
	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}

	h.setRepositoryDefaults()

	return h.writer.Write(h.composedBlueprint, shouldOverwrite, h.initBlueprintURLs...)
}

// RetargetSource repoints an already-declared source to a new tagged OCI URL and returns the
// source's previous URL for diff reporting. The new URL must include a version tag, validated via
// OCI parsing; the tag is carried in the URL (the canonical source form), so any prior ref fields
// are cleared. It errors when the source name is not declared, since adding or removing a source is
// a structural edit to blueprint.yaml rather than a retarget. RetargetSource mutates the composed
// blueprint in memory only; call Write to persist.
func (h *BaseBlueprintHandler) RetargetSource(name, url string) (string, error) {
	if h.composedBlueprint == nil {
		return "", fmt.Errorf("blueprint not loaded")
	}
	if _, err := artifact.ParseOCIReference(url); err != nil {
		return "", werror.Wrap(err, "invalid source url %q", url)
	}
	for i := range h.composedBlueprint.Sources {
		if h.composedBlueprint.Sources[i].Name == name {
			previous := h.composedBlueprint.Sources[i].Url
			h.composedBlueprint.Sources[i].Url = url
			h.composedBlueprint.Sources[i].Ref = blueprintv1alpha1.Reference{}
			return previous, nil
		}
	}
	return "", werror.New(fmt.Sprintf("Blueprint source %q is not declared.", name),
		"Add the source to blueprint.yaml, then run the command again.", nil)
}

// UpgradeSourcesToLatest moves each remote OCI source pinned to a semver to the highest stable tag
// this CLI is compatible with (see artifact.ResolveCompatibleTag), mutating the composed blueprint
// in memory (call Write to persist) and returning the changes for reporting. Sources that are not
// OCI, not semver-pinned, or already at the latest compatible tag are left untouched. All tags are
// resolved before any source is mutated, so a registry failure mid-resolution leaves the in-memory
// blueprint untouched.
func (h *BaseBlueprintHandler) UpgradeSourcesToLatest() ([]SourceUpgrade, error) {
	if h.composedBlueprint == nil {
		return nil, fmt.Errorf("blueprint not loaded")
	}

	type plannedUpgrade struct {
		index  int
		newURL string
		change SourceUpgrade
	}

	var planned []plannedUpgrade
	for i := range h.composedBlueprint.Sources {
		src := h.composedBlueprint.Sources[i]
		if !strings.HasPrefix(src.Url, "oci://") {
			continue
		}
		info, err := artifact.ParseOCIReference(src.Url)
		if err != nil || info == nil {
			continue
		}
		current, err := semver.NewVersion(info.Tag)
		if err != nil {
			continue
		}

		tags, err := h.artifactBuilder.ListTags(src.Url)
		if err != nil {
			return nil, werror.Wrap(err, "failed to list tags for source %q", src.Name)
		}
		urlPrefix := strings.TrimSuffix(src.Url, ":"+info.Tag)
		latestTag, latest, ok, err := artifact.ResolveCompatibleTag(h.artifactBuilder, urlPrefix, tags)
		if err != nil {
			return nil, werror.Wrap(err, "failed to check CLI compatibility for source %q", src.Name)
		}
		if !ok || !latest.GreaterThan(current) {
			continue
		}

		newURL := urlPrefix + ":" + latestTag
		planned = append(planned, plannedUpgrade{
			index:  i,
			newURL: newURL,
			change: SourceUpgrade{Name: src.Name, From: src.Url, To: newURL},
		})
	}

	upgrades := make([]SourceUpgrade, 0, len(planned))
	for _, p := range planned {
		h.composedBlueprint.Sources[p.index].Url = p.newURL
		h.composedBlueprint.Sources[p.index].Ref = blueprintv1alpha1.Reference{}
		upgrades = append(upgrades, p.change)
	}
	return upgrades, nil
}

// IsDowngrade reports whether targetURL pins an older stable semver than previousURL for the same
// OCI repository. It returns false when either URL is not a parseable OCI reference, when the two
// reference different repositories (a re-source is not an ordered version change), or when either
// tag is not a semver (a branch, mutable tag, or commit is not ordered) — in those cases a
// regression cannot be asserted, so the caller must not treat the change as a downgrade.
func IsDowngrade(previousURL, targetURL string) bool {
	prev, err := artifact.ParseOCIReference(previousURL)
	if err != nil || prev == nil {
		return false
	}
	target, err := artifact.ParseOCIReference(targetURL)
	if err != nil || target == nil {
		return false
	}
	if strings.TrimSuffix(prev.URL, ":"+prev.Tag) != strings.TrimSuffix(target.URL, ":"+target.Tag) {
		return false
	}
	from, err := semver.NewVersion(prev.Tag)
	if err != nil {
		return false
	}
	to, err := semver.NewVersion(target.Tag)
	if err != nil {
		return false
	}
	return to.LessThan(from)
}

// GetDeclaredSources loads only the context's blueprint.yaml (the user blueprint) and returns its
// declared sources, without loading remote source content or composing. It is the cheap pre-flight
// read upgrade uses to evaluate a --source change before pulling or composing anything, so a refused
// downgrade never triggers a registry round-trip. Returns an empty slice when no blueprint.yaml or
// no sources are declared.
func (h *BaseBlueprintHandler) GetDeclaredSources() ([]blueprintv1alpha1.Source, error) {
	if err := h.loadUser(); err != nil {
		return nil, err
	}
	userBp := h.userBlueprintLoader.GetBlueprint()
	if userBp == nil {
		return nil, nil
	}
	return userBp.Sources, nil
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

// GenerateResolved returns a deep copy of the composed blueprint with deferred substitutions resolved,
// for consumers that need final values, such as the ConfigMaps the provisioner writes. The composed
// blueprint does not change, so later calls start from the deferred expressions. Display callers use
// Generate. A substitution that does not resolve is an error that names its path, such as
// "kustomize.dns.substitutions.external_dns_tenant_id", so no raw ${...} text reaches a ConfigMap.
func (h *BaseBlueprintHandler) GenerateResolved() (*blueprintv1alpha1.Blueprint, error) {
	bp := h.Generate()
	if bp == nil {
		return nil, nil
	}
	resolved := bp.DeepCopy()
	h.composedBlueprint = resolved
	err := h.resolveDeferredSubstitutions()
	if err == nil {
		err = h.resolveMessages()
	}
	h.composedBlueprint = bp
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// GetDeferredPaths returns composed paths whose values were deferred during expression evaluation.
func (h *BaseBlueprintHandler) GetDeferredPaths() map[string]bool {
	h.deferredPathsMu.Lock()
	defer h.deferredPathsMu.Unlock()
	if len(h.deferredPaths) == 0 {
		return nil
	}
	out := make(map[string]bool, len(h.deferredPaths))
	for k, v := range h.deferredPaths {
		out[k] = v
	}
	return out
}

// DeferredSubstitutionsInScope reports whether a deferred substitution is about to be written by
// `apply kustomize`. A blueprint-level ConfigMap or global substitution is always in scope. A
// per-kustomization substitution is in scope only when its kustomization name is in scope. A plain
// kustomize: entry names that kustomization directly, as "kustomize.<name>.substitutions.<key>". A
// flux: system tier names it indirectly. fluxTierScopeName maps the tier path to its compiled
// Kustomization name first.
func (h *BaseBlueprintHandler) DeferredSubstitutionsInScope(scope map[string]bool) bool {
	for path := range h.GetDeferredPaths() {
		if strings.HasPrefix(path, "configmaps.") || strings.HasPrefix(path, "substitutions.") {
			return true
		}
		if rest, ok := strings.CutPrefix(path, "kustomize."); ok {
			for name := range scope {
				if rest == name || strings.HasPrefix(rest, name+".") {
					return true
				}
			}
			continue
		}
		if rest, ok := strings.CutPrefix(path, "flux."); ok {
			if name, ok := fluxTierScopeName(rest); ok && scope[name] {
				return true
			}
		}
	}
	return false
}

// GetConfigScope returns the composed config scope: facet config: block names mapped to their
// fully evaluated values, merged with context values. Populated by LoadBlueprint(); nil before
// the first successful load.
func (h *BaseBlueprintHandler) GetConfigScope() map[string]any {
	return h.composedScope
}

// =============================================================================
// Private Methods
// =============================================================================

// processAndCompose processes the facets of every loaded source and composes the result. It processes
// the loaders in dependency order, and each source sees the scope of the sources before it. It merges
// the init blueprints, then the other sources, then the user blueprint. In the final scope, the facet
// scope overlays the raw context values, so a derived facet value wins over a default. It evaluates
// the terraform inputs against that scope and stores the composed blueprint on the handler.
func (h *BaseBlueprintHandler) processAndCompose() error {
	repositoryScope := h.getRepositoryScopeValues()

	if h.traceCollector != nil {
		if bp, ok := h.processor.(*BaseBlueprintProcessor); ok {
			bp.SetTraceCollector(h.traceCollector)
		}
	}

	loadersToProcess, loaderNames := h.loadersWithBlueprints()

	collectedScopes, err := h.processSourceFacets(loadersToProcess, loaderNames, repositoryScope)
	if err != nil {
		return err
	}

	loaders, initLoaderNames := h.compositionOrder(loadersToProcess)

	var mergedScope map[string]any
	if contextValues := h.getConfigValues(); contextValues != nil {
		mergedScope = MergeScopeMaps(mergedScope, contextValues)
	}
	for _, loader := range loaders {
		name := loaderNames[loader]
		if name == "" {
			continue
		}
		if scope, ok := collectedScopes[name]; ok && scope != nil {
			mergedScope = MergeScopeMaps(mergedScope, scope)
		}
	}

	userPath := ""
	if h.userBlueprintLoader != nil {
		if ul, ok := h.userBlueprintLoader.(*BaseBlueprintLoader); ok {
			userPath = ul.GetBlueprintPath()
		}
	}
	if concrete, ok := h.processor.(*BaseBlueprintProcessor); ok {
		if comp, ok := h.composer.(*BaseBlueprintComposer); ok {
			comp.SetExcludedFacets(concrete.GetExcludedFacets())
		}
	}

	composedBp, composeErr := h.composer.Compose(loaders, initLoaderNames, userPath, mergedScope)
	h.composedBlueprint = composedBp
	h.composedScope = mergedScope

	if h.traceCollector != nil {
		h.traceCollector.Finalize(h.composedBlueprint, mergedScope, h.runtime.TemplateRoot)
	}

	if composeErr != nil {
		return composeErr
	}

	h.deriveConfigMapDeferredPaths()

	if err := h.evaluateTerraformInputs(mergedScope); err != nil {
		return err
	}

	h.clearLocalTemplateSource(h.composedBlueprint)

	if h.runtime.TerraformProvider != nil {
		components := h.GetTerraformComponents()
		h.runtime.TerraformProvider.SetTerraformComponents(components)
		h.runtime.TerraformProvider.SetConfigScope(mergedScope)
	}

	if h.runtime.Evaluator != nil {
		h.runtime.Evaluator.SetConfigScope(mergedScope)
	}

	return nil
}

// loadersWithBlueprints returns the source loaders that hold a blueprint, then the user loader when it
// holds one, along with each loader's source name.
func (h *BaseBlueprintHandler) loadersWithBlueprints() ([]BlueprintLoader, map[BlueprintLoader]string) {
	var loadersToProcess []BlueprintLoader
	loaderNames := make(map[BlueprintLoader]string)

	for name, loader := range h.sourceBlueprintLoaders {
		if loader.GetBlueprint() != nil {
			loadersToProcess = append(loadersToProcess, loader)
			loaderNames[loader] = name
		}
	}
	if h.userBlueprintLoader != nil && h.userBlueprintLoader.GetBlueprint() != nil {
		loadersToProcess = append(loadersToProcess, h.userBlueprintLoader)
		loaderNames[h.userBlueprintLoader] = h.userBlueprintLoader.GetSourceName()
	}
	return loadersToProcess, loaderNames
}

// processSourceFacets runs global facet processing over the loaders in dependency order, with
// repositoryScope as extra scope. It copies the processor's deferred paths onto the handler and returns
// the resolved scope keyed by each source that contributed facets. A RequirementsError is returned
// unwrapped.
func (h *BaseBlueprintHandler) processSourceFacets(loadersToProcess []BlueprintLoader, loaderNames map[BlueprintLoader]string, repositoryScope map[string]any) (map[string]map[string]any, error) {
	collectedScopes := make(map[string]map[string]any)

	if concrete, ok := h.processor.(*BaseBlueprintProcessor); ok {
		concrete.ResetExcludedFacets()
	}

	userLoaderName := ""
	if h.userBlueprintLoader != nil {
		userLoaderName = loaderNames[h.userBlueprintLoader]
	}
	if concrete, ok := h.processor.(*BaseBlueprintProcessor); ok {
		concrete.SetExtraScope(repositoryScope)
	}
	var sourceSets []SourceFacetSet
	for depth, loader := range orderLoadersByDependency(loadersToProcess, loaderNames, userLoaderName) {
		facets := loader.GetFacets()
		if len(facets) == 0 {
			continue
		}
		sourceSets = append(sourceSets, SourceFacetSet{
			Name:   loaderNames[loader],
			Depth:  depth,
			Target: loader.GetBlueprint(),
			Facets: facets,
		})
	}
	globalScope, err := h.processor.ProcessGlobally(sourceSets)
	if err != nil {
		var reqErr *RequirementsError
		if errors.As(err, &reqErr) {
			return nil, err
		}
		return nil, werror.Wrap(err, "failed to process facets")
	}
	if concrete, ok := h.processor.(*BaseBlueprintProcessor); ok {
		h.deferredPathsMu.Lock()
		for path := range concrete.GetDeferredPaths() {
			h.deferredPaths[path] = true
		}
		h.deferredPathsMu.Unlock()
	}
	for _, src := range sourceSets {
		collectedScopes[src.Name] = globalScope
	}
	return collectedScopes, nil
}

// compositionOrder returns the loaders in composition order: initializer blueprints, the other sources,
// then the user blueprint. It also returns the source names of the OCI initializer blueprints.
func (h *BaseBlueprintHandler) compositionOrder(loadersToProcess []BlueprintLoader) ([]BlueprintLoader, []string) {
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
	return loaders, initLoaderNames
}

// evaluateTerraformInputs evaluates each composed terraform component's inputs against scope and
// replaces them with the results. Deferred values keep their expressions. It does nothing when the
// runtime has no evaluator.
func (h *BaseBlueprintHandler) evaluateTerraformInputs(scope map[string]any) error {
	if h.runtime.Evaluator == nil {
		return nil
	}
	for i := range h.composedBlueprint.TerraformComponents {
		comp := h.composedBlueprint.TerraformComponents[i]
		if comp.Inputs == nil {
			continue
		}
		evaluated := make(map[string]any, len(comp.Inputs))
		for key, value := range comp.Inputs {
			result, err := EvaluateWithOrigins(
				h.runtime.Evaluator, key, value, comp.InputOrigins, scope, false,
			)
			if err != nil {
				return werror.Wrap(err, "evaluate terraform inputs for component %q", comp.GetID())
			}
			evaluated[key] = normalizeDeferredValue(result)
		}
		h.composedBlueprint.TerraformComponents[i].Inputs = evaluated
	}
	return nil
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
	if repositoryScope := h.getRepositoryScopeValues(); repositoryScope != nil {
		values = MergeScopeMaps(values, repositoryScope)
	}
	return values
}

// =============================================================================
// Helpers
// =============================================================================

// latestStableSemver returns the highest non-prerelease semver among tags, alongside its original tag
// string (preserving any "v" prefix) for rebuilding the URL. Non-semver and prerelease tags are
// ignored. Reports false when no stable semver tag is present.
func latestStableSemver(tags []string) (string, *semver.Version, bool) {
	var best *semver.Version
	var bestTag string
	for _, t := range tags {
		v, err := semver.NewVersion(t)
		if err != nil || v.Prerelease() != "" {
			continue
		}
		if best == nil || v.GreaterThan(best) {
			best = v
			bestTag = t
		}
	}
	return bestTag, best, best != nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintHandler = (*BaseBlueprintHandler)(nil)
