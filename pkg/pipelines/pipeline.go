package pipelines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The BasePipeline is a foundational component that provides common pipeline functionality for command execution.
// It provides a unified interface for pipeline execution with dependency injection support,
// serving as the base implementation for all command-specific pipelines in the Windsor CLI system.
// The BasePipeline facilitates standardized command execution patterns with constructor-based dependency injection.

// =============================================================================
// Types
// =============================================================================

// Pipeline defines the interface for all command pipelines
type Pipeline interface {
	Initialize(injector di.Injector, ctx context.Context) error
	Execute(ctx context.Context) error
}

// BasePipeline provides common pipeline functionality including config loading
// Specific pipelines should embed this and add their own constructor dependencies
type BasePipeline struct {
	shell         shell.Shell
	configHandler config.ConfigHandler
	shims         *Shims
	injector      di.Injector
}

// =============================================================================
// Constructor
// =============================================================================

// NewBasePipeline creates a new BasePipeline instance
func NewBasePipeline() *BasePipeline {
	return &BasePipeline{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize provides a default implementation that can be overridden by specific pipelines
func (p *BasePipeline) Initialize(injector di.Injector, ctx context.Context) error {
	p.injector = injector

	return nil
}

// Execute provides a default implementation that can be overridden by specific pipelines
func (p *BasePipeline) Execute(ctx context.Context) error {
	return nil
}

// =============================================================================
// Protected Methods
// =============================================================================

// handleSessionReset checks session state and performs reset if needed.
// This is a common pattern used by multiple commands (env, exec, context, init).
func (p *BasePipeline) handleSessionReset() error {
	if p.shell == nil {
		return nil
	}

	hasSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN") != ""

	shouldReset, err := p.shell.CheckResetFlags()
	if err != nil {
		return err
	}

	if !hasSessionToken {
		shouldReset = true
	}

	if shouldReset {
		p.shell.Reset()

		if err := os.Setenv("NO_CACHE", "true"); err != nil {
			return err
		}
	}

	return nil
}

// loadConfig loads the windsor.yaml config file from the project root into the config handler.
// This is a common operation that most pipelines will need, so it's provided in the base pipeline.
func (p *BasePipeline) loadConfig() error {
	if p.shell == nil {
		return fmt.Errorf("shell not initialized")
	}
	if p.configHandler == nil {
		return fmt.Errorf("config handler not initialized")
	}
	if p.shims == nil {
		return fmt.Errorf("shims not initialized")
	}

	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}

	yamlPath := filepath.Join(projectRoot, "windsor.yaml")
	ymlPath := filepath.Join(projectRoot, "windsor.yml")

	var cliConfigPath string
	if _, err := p.shims.Stat(yamlPath); err == nil {
		cliConfigPath = yamlPath
	} else if _, err := p.shims.Stat(ymlPath); err == nil {
		cliConfigPath = ymlPath
	}

	if cliConfigPath != "" {
		if err := p.configHandler.LoadConfig(cliConfigPath); err != nil {
			return fmt.Errorf("error loading config file: %w", err)
		}
	}

	return nil
}

// resolveOrCreateDependency is a generic helper that resolves or creates a dependency
// using the provided constructor function, eliminating repetitive dependency resolution code.
func resolveOrCreateDependency[T any](
	injector di.Injector,
	name string,
	constructor func(di.Injector) T,
) T {
	if existing := injector.Resolve(name); existing != nil {
		if dep, ok := existing.(T); ok {
			return dep
		}
	}

	dep := constructor(injector)
	injector.Register(name, dep)
	return dep
}
