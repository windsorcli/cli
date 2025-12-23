package v1alpha2

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed providers/schema.yaml
//go:embed secrets/schema.yaml
//go:embed terraform/schema.yaml
//go:embed workstation/schema.yaml
var schemasFS embed.FS

// LoadSchemas loads all embedded schema.yaml files into the provided schema loader function.
// The loader function should accept schema content as bytes and return an error if loading fails.
// This allows the v1alpha2 package to be self-contained and responsible for loading its own schemas.
func LoadSchemas(loadSchema func([]byte) error) error {
	err := fs.WalkDir(schemasFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if d.Name() == "schema.yaml" {
			schemaContent, err := schemasFS.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read schema file %s: %w", path, err)
			}

			if err := loadSchema(schemaContent); err != nil {
				return fmt.Errorf("failed to load schema from %s: %w", path, err)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking API schemas: %w", err)
	}

	return nil
}
