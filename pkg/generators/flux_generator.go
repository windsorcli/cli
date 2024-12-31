package generators

import (
	"fmt"
	"os"
	"path/filepath"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"sigs.k8s.io/yaml"
)

// FluxGenerator is a generator that writes Flux Kustomization and GitRepository manifests
type FluxGenerator struct {
	BaseGenerator
}

// NewFluxGenerator creates a new FluxGenerator
func NewFluxGenerator(injector di.Injector) *FluxGenerator {
	return &FluxGenerator{
		BaseGenerator: BaseGenerator{injector: injector},
	}
}

// Write generates the Flux Kustomization and GitRepository manifests
func (g *FluxGenerator) Write() error {
	// Get the configuration root path from the context
	configRoot, err := g.injector.Resolve("contextHandler").(context.ContextHandler).GetConfigRoot()
	if err != nil {
		return fmt.Errorf("failed to get config root: %w", err)
	}

	// Define the path to the flux directory
	fluxPath := filepath.Join(configRoot, "flux")

	// Ensure the parent directories exist
	if err := os.MkdirAll(fluxPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create flux directory: %w", err)
	}

	// Create and write the Kustomization manifest
	if err := g.writeKustomization(fluxPath); err != nil {
		return err
	}

	// Create and write the GitRepository manifest
	if err := g.writeGitRepository(fluxPath); err != nil {
		return err
	}

	return nil
}

// writeKustomization creates and writes a Kustomization manifest
func (g *FluxGenerator) writeKustomization(dirPath string) error {
	kustomization := &kustomizev1.Kustomization{
		// Fill in the Kustomization fields as needed
	}

	data, err := yaml.Marshal(kustomization)
	if err != nil {
		return fmt.Errorf("failed to marshal Kustomization: %w", err)
	}

	filePath := filepath.Join(dirPath, "kustomization.yaml")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write Kustomization file: %w", err)
	}

	return nil
}

// writeGitRepository creates and writes a GitRepository manifest
func (g *FluxGenerator) writeGitRepository(dirPath string) error {
	gitRepository := &sourcev1.GitRepository{
		// Fill in the GitRepository fields as needed
	}

	data, err := yaml.Marshal(gitRepository)
	if err != nil {
		return fmt.Errorf("failed to marshal GitRepository: %w", err)
	}

	filePath := filepath.Join(dirPath, "gitrepository.yaml")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write GitRepository file: %w", err)
	}

	return nil
}
