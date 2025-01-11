package tools

import (
	"path/filepath"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
)

// ToolsManager is responsible for managing the cli toolchain required
// by the project. It leverages existing package ecosystems and modifies
// tools manifests to ensure the appropriate tools are installed and configured.
type ToolsManager interface {
	Initialize() error
	WriteManifest() error
	Install() error
}

// BaseToolsManager is the base implementation of the ToolsManager interface.
type BaseToolsManager struct {
	injector      di.Injector
	configHandler config.ConfigHandler
}

// Creates a new ToolsManager instance with the given injector.
func NewToolsManager(injector di.Injector) *BaseToolsManager {
	return &BaseToolsManager{
		injector: injector,
	}
}

// Initialize initializes the tools manager.
func (t *BaseToolsManager) Initialize() error {
	// Resolve the config handler
	configHandler := t.injector.Resolve("configHandler")
	t.configHandler = configHandler.(config.ConfigHandler)

	return nil
}

// WriteManifest writes the tools manifest to the project root.
// It should not overwrite existing manifest files, but
// update them appropriately.
func (t *BaseToolsManager) WriteManifest() error {
	// Placeholder
	return nil
}

// Install installs the tools required by the project.
func (t *BaseToolsManager) Install() error {
	// Placeholder
	return nil
}

// CheckExistingToolsManager checks if a tools manager is in use
// and returns its name.
func CheckExistingToolsManager(projectRoot string) (string, error) {
	// Check for "aqua.yaml" in the project root
	aquaPath := filepath.Join(projectRoot, "aqua.yaml")
	if _, err := osStat(aquaPath); err == nil {
		return "aqua", nil
	}

	// Check for ".tool-versions" in the project root
	asdfPath := filepath.Join(projectRoot, ".tool-versions")
	if _, err := osStat(asdfPath); err == nil {
		return "asdf", nil
	}

	return "", nil
}
