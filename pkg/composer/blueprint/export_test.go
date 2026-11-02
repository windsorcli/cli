// The export_test file gives the black-box tests of package blueprint_test their test seams.
// It exposes the shims for fault injection and the handler state that tests set or read.
// It does not expose private methods. Tests call the public methods to check behavior.

package blueprint

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Seams
// =============================================================================

// Shims returns the shims of the handler.
func (h *BaseBlueprintHandler) Shims() *Shims {
	return h.shims
}

// ComposedBlueprint returns the composed blueprint of the handler.
func (h *BaseBlueprintHandler) ComposedBlueprint() *blueprintv1alpha1.Blueprint {
	return h.composedBlueprint
}

// SetComposedBlueprint sets the composed blueprint of the handler.
func (h *BaseBlueprintHandler) SetComposedBlueprint(bp *blueprintv1alpha1.Blueprint) {
	h.composedBlueprint = bp
}

// SetDeferredPaths sets the deferred paths of the handler.
func (h *BaseBlueprintHandler) SetDeferredPaths(paths map[string]bool) {
	h.deferredPaths = paths
}

// SourceLoaders returns the source loaders of the handler, keyed by source name.
func (h *BaseBlueprintHandler) SourceLoaders() map[string]BlueprintLoader {
	return h.sourceBlueprintLoaders
}

// SetWriter replaces the writer of the handler.
func (h *BaseBlueprintHandler) SetWriter(w BlueprintWriter) {
	h.writer = w
}

// HandlerOverrides returns the override value that NewBlueprintHandler accepts.
func HandlerOverrides(processor BlueprintProcessor, composer BlueprintComposer, writer BlueprintWriter) *BaseBlueprintHandler {
	return &BaseBlueprintHandler{processor: processor, composer: composer, writer: writer}
}

// Shims returns the shims of the composer.
func (c *BaseBlueprintComposer) Shims() *Shims {
	return c.shims
}

// Shims returns the shims of the loader.
func (l *BaseBlueprintLoader) Shims() *Shims {
	return l.shims
}

// SetBlueprint sets the loaded blueprint of the loader.
func (l *BaseBlueprintLoader) SetBlueprint(bp *blueprintv1alpha1.Blueprint) {
	l.blueprint = bp
}

// SetFacets sets the loaded facets of the loader.
func (l *BaseBlueprintLoader) SetFacets(facets []blueprintv1alpha1.Facet) {
	l.facets = facets
}

// Shims returns the shims of the writer.
func (w *BaseBlueprintWriter) Shims() *Shims {
	return w.shims
}

// Contributions returns the contributions that the collector recorded for path.
func (c *DefaultTraceCollector) Contributions(path string) []TraceContribution {
	return c.contributions[path]
}

// ConfigBlocks returns the config block records that the collector recorded for path.
func (c *DefaultTraceCollector) ConfigBlocks(path string) []ConfigBlockRecord {
	return c.configBlocks[path]
}
