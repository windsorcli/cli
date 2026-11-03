// The export_test file gives the black-box tests of package terraform_test their test seams.
// It exposes private fields for fault injection and state checks. It does not expose private methods.

package terraform

import (
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Seams
// =============================================================================

// BlueprintHandler returns the blueprintHandler field of the BaseModuleResolver.
func (x *BaseModuleResolver) BlueprintHandler() blueprint.BlueprintHandler {
	return x.blueprintHandler
}

// SetBlueprintHandler sets the blueprintHandler field of the BaseModuleResolver.
func (x *BaseModuleResolver) SetBlueprintHandler(v blueprint.BlueprintHandler) {
	x.blueprintHandler = v
}

// SetEvaluator sets the evaluator field of the BaseModuleResolver.
func (x *BaseModuleResolver) SetEvaluator(v evaluator.ExpressionEvaluator) {
	x.evaluator = v
}

// Runtime returns the runtime field of the BaseModuleResolver.
func (x *BaseModuleResolver) Runtime() *runtime.Runtime {
	return x.runtime
}

// Shims returns the shims field of the BaseModuleResolver.
func (x *BaseModuleResolver) Shims() *Shims {
	return x.shims
}

// SetShims sets the shims field of the BaseModuleResolver.
func (x *BaseModuleResolver) SetShims(v *Shims) {
	x.shims = v
}

// OciResolver returns the ociResolver field of the CompositeModuleResolver.
func (x *CompositeModuleResolver) OciResolver() *OCIModuleResolver {
	return x.ociResolver
}

// StandardResolver returns the standardResolver field of the CompositeModuleResolver.
func (x *CompositeModuleResolver) StandardResolver() *StandardModuleResolver {
	return x.standardResolver
}

// ArtifactBuilder returns the artifactBuilder field of the OCIModuleResolver.
func (x *OCIModuleResolver) ArtifactBuilder() artifact.Artifact {
	return x.artifactBuilder
}

// SetArtifactBuilder sets the artifactBuilder field of the OCIModuleResolver.
func (x *OCIModuleResolver) SetArtifactBuilder(v artifact.Artifact) {
	x.artifactBuilder = v
}
