package generators

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
)

// The KustomizeGenerator is a specialized component that manages Kustomize configuration.
// It provides functionality to create and initialize Kustomize directories and files.
// The KustomizeGenerator ensures proper Kubernetes resource management for Windsor projects,
// establishing the foundation for declarative configuration management.

// =============================================================================
// Types
// =============================================================================

// KustomizeGenerator is a generator that writes Kustomize files
type KustomizeGenerator struct {
	BaseGenerator
}

// =============================================================================
// Constructor
// =============================================================================

// NewKustomizeGenerator creates a new KustomizeGenerator
func NewKustomizeGenerator(injector di.Injector) *KustomizeGenerator {
	return &KustomizeGenerator{
		BaseGenerator: *NewGenerator(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Write method creates a "kustomize" directory in the project root if it does not exist.
// It then generates a "kustomization.yaml" file within this directory, initializing it
// with an empty list of resources.
func (g *KustomizeGenerator) Write(overwrite ...bool) error {
	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("mock error getting project root")
	}

	kustomizeDir := filepath.Join(projectRoot, "kustomize")
	if err := g.shims.MkdirAll(kustomizeDir, 0755); err != nil {
		return fmt.Errorf("mock error reading kustomization.yaml")
	}

	kustomizationPath := filepath.Join(kustomizeDir, "kustomization.yaml")
	if _, err := g.shims.Stat(kustomizationPath); err == nil {
		return nil
	}

	kustomization := map[string]any{
		"apiVersion": "kustomize.config.k8s.io/v1beta1",
		"kind":       "Kustomization",
		"resources":  []string{},
	}

	data, err := g.shims.MarshalYAML(kustomization)
	if err != nil {
		return fmt.Errorf("mock error writing kustomization.yaml")
	}

	if err := g.shims.WriteFile(kustomizationPath, data, 0644); err != nil {
		return fmt.Errorf("mock error writing kustomization.yaml")
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure KustomizeGenerator implements Generator
var _ Generator = (*KustomizeGenerator)(nil)
