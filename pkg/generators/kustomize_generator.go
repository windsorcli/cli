package generators

import (
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
)

// KustomizeGenerator is a generator that writes Kustomize files
type KustomizeGenerator struct {
	BaseGenerator
}

// NewKustomizeGenerator creates a new KustomizeGenerator
func NewKustomizeGenerator(injector di.Injector) *KustomizeGenerator {
	return &KustomizeGenerator{
		BaseGenerator: BaseGenerator{injector: injector},
	}
}

// Write method creates a "kustomize" directory in the project root if it does not exist.
// It then generates a "kustomization.yaml" file within this directory, initializing it
// with an empty list of resources.
func (g *KustomizeGenerator) Write() error {
	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return err
	}

	kustomizeFolderPath := filepath.Join(projectRoot, "kustomize")
	if err := osMkdirAll(kustomizeFolderPath, os.ModePerm); err != nil {
		return err
	}

	kustomizationFilePath := filepath.Join(kustomizeFolderPath, "kustomization.yaml")

	// Check if the file already exists
	if _, err := osStat(kustomizationFilePath); err == nil {
		// File exists, do not overwrite
		return nil
	}

	// Write the file with resources: [] by default
	kustomizationContent := []byte("resources: []\n")

	if err := osWriteFile(kustomizationFilePath, kustomizationContent, 0644); err != nil {
		return err
	}

	return nil
}

// Ensure KustomizeGenerator implements Generator
var _ Generator = (*KustomizeGenerator)(nil)
