// The CompositeModuleResolver is a terraform module resolver that delegates to the OCI and standard resolvers.
// It provides the ModuleResolver that the composer uses for all terraform components.
// Each resolver skips a component whose source it does not handle.
// GenerateTfvars uses the shared BaseModuleResolver implementation.

package terraform

import (
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Types
// =============================================================================

// CompositeModuleResolver handles terraform modules through the OCI and standard resolvers.
type CompositeModuleResolver struct {
	ociResolver      *OCIModuleResolver
	standardResolver *StandardModuleResolver
}

// =============================================================================
// Constructor
// =============================================================================

// NewCompositeModuleResolver creates a composite module resolver with an OCI resolver and a standard resolver.
// It panics if rt, blueprintHandler, or artifactBuilder is nil.
func NewCompositeModuleResolver(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler, artifactBuilder artifact.Artifact) *CompositeModuleResolver {
	if rt == nil {
		panic("runtime is required")
	}
	if blueprintHandler == nil {
		panic("blueprint handler is required")
	}
	if artifactBuilder == nil {
		panic("artifact builder is required")
	}

	return &CompositeModuleResolver{
		ociResolver:      NewOCIModuleResolver(rt, blueprintHandler, artifactBuilder),
		standardResolver: NewStandardModuleResolver(rt, blueprintHandler),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// ProcessModules runs ProcessModules on the OCI resolver, then on the standard resolver.
// It stops at the first error.
func (h *CompositeModuleResolver) ProcessModules() error {
	if err := h.ociResolver.ProcessModules(); err != nil {
		return werror.Wrap(err, "failed to process OCI modules")
	}

	if err := h.standardResolver.ProcessModules(); err != nil {
		return werror.Wrap(err, "failed to process standard modules")
	}

	return nil
}

// GenerateTfvars writes the tfvars files for all terraform components through the standard resolver.
// All resolvers share the same BaseModuleResolver implementation.
func (h *CompositeModuleResolver) GenerateTfvars(overwrite bool) error {
	return h.standardResolver.GenerateTfvars(overwrite)
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure CompositeModuleResolver implements ModuleResolver
var _ ModuleResolver = (*CompositeModuleResolver)(nil)
