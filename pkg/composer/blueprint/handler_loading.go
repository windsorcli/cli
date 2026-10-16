package blueprint

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

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
				useMetadataName := bp.Metadata.Name != "template" || h.hasLocalTemplate()
				storageKey := tempName
				if useMetadataName {
					storageKey = bp.Metadata.Name
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
				}
				mu.Lock()
				if _, exists := h.sourceBlueprintLoaders[storageKey]; !exists {
					h.sourceBlueprintLoaders[storageKey] = loader
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

// hasLocalTemplate returns true when the runtime has a TemplateRoot and that directory exists.
// When false, init OCI blueprints with metadata.name "template" are stored under URL-derived name
// so the name "template" is not introduced without a local _template.
func (h *BaseBlueprintHandler) hasLocalTemplate() bool {
	if h.runtime == nil || h.runtime.TemplateRoot == "" {
		return false
	}
	_, err := h.shims.Stat(h.runtime.TemplateRoot)
	return err == nil
}

// loadUser initializes and loads the user blueprint from the config root directory. The user
// blueprint (blueprint.yaml in the context folder) acts as the final override layer in composition,
// allowing users to select specific components, override values, and add custom configurations.
func (h *BaseBlueprintHandler) loadUser() error {
	h.userBlueprintLoader = NewBlueprintLoader(h.runtime, h.artifactBuilder)
	return h.userBlueprintLoader.Load("user", "")
}

// loadSources iterates through the user blueprint's sources array and loads each source.
// A blueprint's own facets under contexts/_template/ are the thing being authored, not a source, so the
// local _template is loaded whenever it exists and the load is not an init-with-URLs flow — independent
// of whether the sources list is empty. Listing sources (e.g. a downstream blueprint layering on core)
// therefore no longer silently drops the blueprint's own facets. Sources with name "template" are loaded
// from the local _template directory; sources with OCI URLs are loaded from the registry. The explicit
// load is a no-op when the local template is already loaded, so listing "template" alongside other
// sources still works. Sources are loaded in parallel. After loading direct sources, it recursively loads
// any sources referenced by those blueprints. When the user blueprint is nil (e.g. no blueprint.yaml
// yet), the local template is loaded if present so initial composition produces kustomizations to apply.
// During an init-with-URLs flow the local template is left to loadInitBlueprints, which owns the template
// name in that path.
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

	if len(h.initBlueprintURLs) == 0 && h.runtime != nil && h.runtime.TemplateRoot != "" {
		if _, err := h.shims.Stat(h.runtime.TemplateRoot); err == nil {
			if _, exists := h.sourceBlueprintLoaders["template"]; !exists {
				loader := NewBlueprintLoader(h.runtime, h.artifactBuilder)
				if loadErr := loader.Load("template", ""); loadErr == nil && loader.GetBlueprint() != nil {
					h.sourceBlueprintLoaders["template"] = loader
				}
			}
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	claimed := make(map[string]bool)

	for _, source := range userBp.Sources {
		if source.Name == "" {
			continue
		}
		mu.Lock()
		_, exists := h.sourceBlueprintLoaders[source.Name]
		if exists || claimed[source.Name] {
			mu.Unlock()
			continue
		}
		claimed[source.Name] = true
		mu.Unlock()

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
				if source.Name == "" {
					continue
				}
				if source.Name == "template" && !strings.HasPrefix(source.Url, "oci://") {
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

// orderLoadersByDependency returns loaders sorted so that a source is processed before any source
// that lists it in its blueprint's Sources, letting a downstream source's facets read config an
// upstream source derived. It is a Kahn topological sort keyed by loader name; ready nodes are
// drained alphabetically, with userLoaderName forced last (the final override layer), for a
// deterministic order. Edges to names not present among the loaders are ignored, and a dependency
// cycle falls back to appending the unresolved loaders in stable name order rather than dropping them.
func orderLoadersByDependency(loaders []BlueprintLoader, names map[BlueprintLoader]string, userLoaderName string) []BlueprintLoader {
	byName := make(map[string]BlueprintLoader, len(loaders))
	for _, l := range loaders {
		byName[names[l]] = l
	}

	indegree := make(map[string]int, len(loaders))
	for _, l := range loaders {
		indegree[names[l]] = 0
	}
	dependents := make(map[string][]string)
	for _, l := range loaders {
		name := names[l]
		bp := l.GetBlueprint()
		if bp == nil {
			continue
		}
		seen := make(map[string]bool)
		for _, s := range bp.Sources {
			dep := s.Name
			if dep == "" || dep == name || seen[dep] {
				continue
			}
			if _, ok := byName[dep]; !ok {
				continue
			}
			seen[dep] = true
			dependents[dep] = append(dependents[dep], name)
			indegree[name]++
		}
	}

	sortReady := func(s []string) {
		sort.Slice(s, func(i, j int) bool {
			if (s[i] == userLoaderName) != (s[j] == userLoaderName) {
				return s[j] == userLoaderName
			}
			return s[i] < s[j]
		})
	}

	var ready []string
	for name, d := range indegree {
		if d == 0 {
			ready = append(ready, name)
		}
	}
	sortReady(ready)

	order := make([]BlueprintLoader, 0, len(loaders))
	processed := make(map[string]bool, len(loaders))
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		if processed[name] {
			continue
		}
		processed[name] = true
		order = append(order, byName[name])
		for _, dependent := range dependents[name] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
		sortReady(ready)
	}

	if len(order) < len(loaders) {
		remaining := make([]string, 0, len(loaders)-len(order))
		for _, l := range loaders {
			if name := names[l]; !processed[name] {
				remaining = append(remaining, name)
			}
		}
		sortReady(remaining)
		for _, name := range remaining {
			order = append(order, byName[name])
			processed[name] = true
		}
	}

	return order
}
