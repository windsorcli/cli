package template

import (
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Interfaces
// =============================================================================

// Template defines the interface for template processors
type Template interface {
	Initialize() error
	Process(templateData map[string][]byte, renderedData map[string]any) error
}

// =============================================================================
// Processing Rules
// =============================================================================

// ProcessingRule defines how to match and process template files
type ProcessingRule struct {
	// PathMatcher determines if a file path should be processed by this rule
	PathMatcher func(string) bool
	// KeyGenerator generates the output key from the input path
	KeyGenerator func(string) string
}

// =============================================================================
// Types
// =============================================================================

// BaseTemplate provides common functionality for template implementations
type BaseTemplate struct {
	injector      di.Injector
	configHandler config.ConfigHandler
	shell         shell.Shell
	rules         []ProcessingRule
	shims         *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseTemplate creates a new BaseTemplate instance
func NewBaseTemplate(injector di.Injector) *BaseTemplate {
	return &BaseTemplate{
		injector: injector,
		shims:    NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the BaseTemplate dependencies
func (t *BaseTemplate) Initialize() error {
	if t.injector != nil {
		if configHandler := t.injector.Resolve("configHandler"); configHandler != nil {
			t.configHandler = configHandler.(config.ConfigHandler)
		}
		if shellService := t.injector.Resolve("shell"); shellService != nil {
			t.shell = shellService.(shell.Shell)
		}
	}
	return nil
}
