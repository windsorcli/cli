package config

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/api/v1alpha1"
	v1alpha2config "github.com/windsorcli/cli/api/v1alpha2/config"
)

// The TypedSource is a configuration source for typed Windsor configuration files.
// It provides loading for root and context typed YAML files and root file initialization,
// The TypedSource isolates typed windsor.yaml concerns from dynamic/context values behavior,
// and centralizes version-aware typed config parsing for context-level map extraction.

// =============================================================================
// Types
// =============================================================================

// typedSource handles typed root/context windsor.yaml loading and root file creation.
type typedSource struct {
	shims           *Shims
	schemaValidator *SchemaValidator
}

// =============================================================================
// Constructor
// =============================================================================

// newTypedSource creates a typedSource with shims and schema validator dependencies.
func newTypedSource(shims *Shims, schemaValidator *SchemaValidator) *typedSource {
	if shims == nil {
		shims = NewShims()
	}

	return &typedSource{
		shims:           shims,
		schemaValidator: schemaValidator,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadRoot loads root windsor.yaml and returns the selected context map when present.
func (s *typedSource) LoadRoot(
	projectRoot string,
	contextName string,
	loadSchemaFromBytes func([]byte) error,
) (map[string]any, bool, error) {
	rootConfigPath := filepath.Join(projectRoot, "windsor.yaml")
	if _, err := s.shims.Stat(rootConfigPath); err != nil {
		return nil, false, nil
	}

	fileData, err := s.shims.ReadFile(rootConfigPath)
	if err != nil {
		return nil, false, fmt.Errorf("error reading root config file: %w", err)
	}

	var rootConfigMap map[string]any
	if err := s.shims.YamlUnmarshal(fileData, &rootConfigMap); err != nil {
		return nil, false, fmt.Errorf("error unmarshalling root config: %w", err)
	}

	configVersion, _ := rootConfigMap["version"].(string)
	if configVersion != "" && configVersion != "v1alpha1" {
		if s.schemaValidator != nil {
			if err := v1alpha2config.LoadSchemas(loadSchemaFromBytes); err != nil {
				return nil, false, fmt.Errorf("error loading API schemas: %w", err)
			}
		}
		return s.loadV1Alpha2Root(rootConfigMap), true, nil
	}

	var rootConfig v1alpha1.Config
	if err := s.shims.YamlUnmarshal(fileData, &rootConfig); err != nil {
		return nil, false, fmt.Errorf("error unmarshalling root config: %w", err)
	}

	if rootConfig.Contexts != nil && rootConfig.Contexts[contextName] != nil {
		contextData, err := s.shims.YamlMarshal(rootConfig.Contexts[contextName])
		if err != nil {
			return nil, true, fmt.Errorf("error marshalling context config: %w", err)
		}

		var contextMap map[string]any
		if err := s.shims.YamlUnmarshal(contextData, &contextMap); err != nil {
			return nil, true, fmt.Errorf("error unmarshalling context config to map: %w", err)
		}

		return contextMap, true, nil
	}

	return nil, true, nil
}

// LoadContext loads legacy context-level windsor.yaml or windsor.yml for a context.
func (s *typedSource) LoadContext(projectRoot, contextName string) (map[string]any, bool, error) {
	contextConfigDir := filepath.Join(projectRoot, "contexts", contextName)
	yamlPath := filepath.Join(contextConfigDir, "windsor.yaml")
	ymlPath := filepath.Join(contextConfigDir, "windsor.yml")

	var contextConfigPath string
	if _, err := s.shims.Stat(yamlPath); err == nil {
		contextConfigPath = yamlPath
	} else if _, err := s.shims.Stat(ymlPath); err == nil {
		contextConfigPath = ymlPath
	}

	if contextConfigPath == "" {
		return nil, false, nil
	}

	fileData, err := s.shims.ReadFile(contextConfigPath)
	if err != nil {
		return nil, true, fmt.Errorf("error reading context config file: %w", err)
	}

	var contextMap map[string]any
	if err := s.shims.YamlUnmarshal(fileData, &contextMap); err != nil {
		return nil, true, fmt.Errorf("error unmarshalling context yaml: %w", err)
	}

	return contextMap, true, nil
}

// EnsureRoot creates a minimal root windsor.yaml when missing.
func (s *typedSource) EnsureRoot(projectRoot string) error {
	rootConfigPath := filepath.Join(projectRoot, "windsor.yaml")
	if _, err := s.shims.Stat(rootConfigPath); err == nil {
		return nil
	}

	rootConfig := map[string]any{
		"version": "v1alpha1",
	}
	rootData, err := s.shims.YamlMarshal(rootConfig)
	if err != nil {
		return fmt.Errorf("error marshalling root config: %w", err)
	}
	if err := s.shims.WriteFile(rootConfigPath, rootData, 0644); err != nil {
		return fmt.Errorf("error writing root config: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// loadV1Alpha2Root resolves root-level v1alpha2 configuration without context overlays.
func (s *typedSource) loadV1Alpha2Root(rootConfigMap map[string]any) map[string]any {
	out := make(map[string]any)
	for key, value := range rootConfigMap {
		if key == "version" || key == "contexts" {
			continue
		}
		out[key] = value
	}
	return out
}
