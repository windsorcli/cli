package config

import (
	"fmt"
	"path/filepath"
)

// The TypedSource is a configuration source for the v1alpha1 windsor.yaml files.
// It provides loading for the root contexts map and context windsor.yaml files, root file
// initialization, and the conversion of v1alpha1 values into the v1alpha2 header.
// The CLI reads these files only to convert a v1alpha1 project, and never writes them.

// =============================================================================
// Types
// =============================================================================

// typedSource handles typed root/context windsor.yaml loading and root file creation.
type typedSource struct {
	shims *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// newTypedSource creates a typedSource that reads files through shims.
func newTypedSource(shims *Shims) *typedSource {
	if shims == nil {
		shims = NewShims()
	}

	return &typedSource{
		shims: shims,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadRoot loads root windsor.yaml and returns the selected context map when present. It keeps
// every key of the context map.
func (s *typedSource) LoadRoot(projectRoot string, contextName string) (map[string]any, bool, error) {
	rootConfigPath := filepath.Join(projectRoot, "windsor.yaml")
	if _, err := s.shims.Stat(rootConfigPath); err != nil {
		return nil, false, nil
	}

	fileData, err := s.shims.ReadFile(rootConfigPath)
	if err != nil {
		return nil, false, fmt.Errorf("error reading root config file: %w", err)
	}

	var rootConfig struct {
		Contexts map[string]map[string]any `yaml:"contexts"`
	}
	if err := s.shims.YamlUnmarshal(fileData, &rootConfig); err != nil {
		return nil, false, fmt.Errorf("error unmarshalling root config: %w", err)
	}

	return rootConfig.Contexts[contextName], true, nil
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
// Helpers
// =============================================================================

// convertV1alpha1 converts merged v1alpha1 values into the v1alpha2 header in place. It moves
// provider to platform when platform is empty, and removes provider.
func convertV1alpha1(values map[string]any) {
	provider, ok := values["provider"]
	if !ok {
		return
	}
	delete(values, "provider")
	if platform, _ := values["platform"].(string); platform == "" {
		values["platform"] = provider
	}
}
