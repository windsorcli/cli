package bundler

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The Bundler provides an interface for adding content to artifacts during the bundling process.
// It provides a unified approach for different content types (templates, kustomize, terraform)
// to contribute their files to the artifact build directory. The Bundler serves as a composable
// component that can validate and bundle specific types of content into distributable artifacts.

// =============================================================================
// Interfaces
// =============================================================================

// Bundler defines the interface for content bundling operations
type Bundler interface {
	Initialize(injector di.Injector) error
	Bundle(artifact Artifact) error
}

// =============================================================================
// Types
// =============================================================================

// BaseBundler provides common functionality for bundler implementations
type BaseBundler struct {
	injector di.Injector
	shims    *Shims
	shell    shell.Shell
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseBundler creates a new BaseBundler instance
func NewBaseBundler() *BaseBundler {
	return &BaseBundler{
		shims: NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize initializes the BaseBundler with dependency injection
func (b *BaseBundler) Initialize(injector di.Injector) error {
	b.injector = injector

	shell, ok := injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("failed to resolve shell from injector")
	}
	b.shell = shell

	return nil
}

// Bundle provides a default implementation that can be overridden by concrete bundlers
func (b *BaseBundler) Bundle(artifact Artifact) error {
	return nil
}

// Ensure BaseBundler implements Bundler interface
var _ Bundler = (*BaseBundler)(nil)
