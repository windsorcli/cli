package terraform

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
)

// The CompositeModuleResolver is a terraform module resolver that delegates to multiple specialized resolvers.
// It coordinates OCI and standard module resolvers, ensuring each component is processed by the appropriate resolver.
// The CompositeModuleResolver acts as the main orchestrator, routing components to the correct resolver based on source type.

// =============================================================================
// Types
// =============================================================================

// CompositeModuleResolver handles terraform modules by delegating to specialized resolvers
type CompositeModuleResolver struct {
	ociResolver      *OCIModuleResolver
	standardResolver *StandardModuleResolver
}

// =============================================================================
// Constructor
// =============================================================================

// NewCompositeModuleResolver creates a new composite module resolver with all specialized resolvers.
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

// ProcessModules processes all terraform modules by delegating to the appropriate specialized resolvers.
// It calls ProcessModules on each resolver in order: OCI, then Standard.
// Returns an error if any resolver fails.
func (h *CompositeModuleResolver) ProcessModules() error {
	if err := h.ociResolver.ProcessModules(); err != nil {
		return fmt.Errorf("failed to process OCI modules: %w", err)
	}

	if err := h.standardResolver.ProcessModules(); err != nil {
		return fmt.Errorf("failed to process standard modules: %w", err)
	}

	return nil
}

// GenerateTfvars generates tfvars files for all terraform components.
// It uses the standard resolver's GenerateTfvars method since all resolvers share the same base implementation.
func (h *CompositeModuleResolver) GenerateTfvars(overwrite bool) error {
	return h.standardResolver.GenerateTfvars(overwrite)
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure CompositeModuleResolver implements ModuleResolver
var _ ModuleResolver = (*CompositeModuleResolver)(nil)
