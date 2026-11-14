package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/internal/werror"
)

// The TypedSource is a configuration source for the root windsor.yaml and the v1alpha1 context files.
// It provides root file validation, loading for the root contexts map and context windsor.yaml files,
// root file initialization, and the conversion of v1alpha1 values into the v1alpha2 header.
// The CLI reads the v1alpha1 forms only to convert a v1alpha1 project, and never writes them.

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
// every key of the context map. A v1alpha1 root file, or a root file with no version, loads as
// before. A v1alpha2 root file MUST hold only version, environment, and paths, and MUST NOT sit
// beside a contexts/<name>/windsor.yaml or windsor.yml. Any other version value fails the load.
func (s *typedSource) LoadRoot(projectRoot string, contextName string) (map[string]any, bool, error) {
	rootConfigPath := filepath.Join(projectRoot, "windsor.yaml")
	if _, err := s.shims.Stat(rootConfigPath); err != nil {
		return nil, false, nil
	}

	fileData, err := s.shims.ReadFile(rootConfigPath)
	if err != nil {
		return nil, false, werror.Wrap(err, "error reading root config file")
	}

	var rootKeys map[string]any
	if err := s.shims.YamlUnmarshal(fileData, &rootKeys); err != nil {
		return nil, false, werror.Wrap(err, "error unmarshalling root config")
	}

	version, hasVersion := rootKeys["version"]
	switch {
	case !hasVersion || version == "v1alpha1":
	case version == v1alpha2.Version:
		if err := s.validateRoot(rootConfigPath, rootKeys); err != nil {
			return nil, false, err
		}
		if err := s.rejectContextFiles(rootConfigPath, projectRoot); err != nil {
			return nil, false, err
		}
		return nil, true, nil
	default:
		return nil, false, werror.New(
			fmt.Sprintf("The root file %s has the version %v. The CLI supports v1alpha1 and v1alpha2.", rootConfigPath, version),
			"Set version to v1alpha2, or run 'windsor migrate' to convert a v1alpha1 project.", nil)
	}

	var rootConfig struct {
		Contexts map[string]map[string]any `yaml:"contexts"`
	}
	if err := s.shims.YamlUnmarshal(fileData, &rootConfig); err != nil {
		return nil, false, werror.Wrap(err, "error unmarshalling root config")
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
		return nil, true, werror.Wrap(err, "error reading context config file")
	}

	var contextMap map[string]any
	if err := s.shims.YamlUnmarshal(fileData, &contextMap); err != nil {
		return nil, true, werror.Wrap(err, "error unmarshalling context yaml")
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
		"version": v1alpha2.Version,
	}
	rootData, err := s.shims.YamlMarshal(rootConfig)
	if err != nil {
		return werror.Wrap(err, "error marshalling root config")
	}
	if err := s.shims.WriteFile(rootConfigPath, rootData, 0644); err != nil {
		return werror.Wrap(err, "error writing root config")
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// validateRoot checks the keys and value types of a v1alpha2 root file. It fails on a contexts key,
// on any key outside the root schema, and on a wrong type in environment or paths.
func (s *typedSource) validateRoot(rootPath string, rootKeys map[string]any) error {
	if _, ok := rootKeys["contexts"]; ok {
		return werror.New(
			fmt.Sprintf("The v1alpha2 root file %s has the key %q. The root file cannot hold per-context values.", rootPath, "contexts"),
			"Run 'windsor migrate' to move the values into contexts/<name>/values.yaml.", nil)
	}
	for _, key := range sortedKeys(rootKeys) {
		if !slices.Contains(v1alpha2.RootKeys, key) {
			return werror.New(
				fmt.Sprintf("The v1alpha2 root file %s has the key %q. The root file allows only %s.", rootPath, key, strings.Join(v1alpha2.RootKeys, ", ")),
				"Remove the key. Run 'windsor migrate' if it comes from a v1alpha1 project.", nil)
		}
	}
	if env, ok := rootKeys["environment"]; ok {
		if err := requireStringMap(rootPath, "environment", env); err != nil {
			return err
		}
	}
	if paths, ok := rootKeys["paths"]; ok {
		pathMap, ok := paths.(map[string]any)
		if !ok {
			return typeError(rootPath, "paths", "an object")
		}
		for _, key := range sortedKeys(pathMap) {
			if !slices.Contains(v1alpha2.PathKeys, key) {
				return werror.New(
					fmt.Sprintf("The root file %s has the key %q under paths. Paths allows only %s.", rootPath, key, strings.Join(v1alpha2.PathKeys, ", ")),
					"Remove the key.", nil)
			}
			if _, ok := pathMap[key].(string); !ok {
				return typeError(rootPath, "paths."+key, "a string")
			}
		}
	}
	return nil
}

// rejectContextFiles fails when a contexts/<name>/windsor.yaml or windsor.yml exists. A v1alpha2
// root file does not read these files, so their settings would stop applying without notice.
func (s *typedSource) rejectContextFiles(rootPath, projectRoot string) error {
	contextsDir := filepath.Join(projectRoot, "contexts")
	entries, err := s.shims.ReadDir(contextsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return werror.Wrap(err, "error reading contexts directory")
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for _, name := range []string{"windsor.yaml", "windsor.yml"} {
			path := filepath.Join(contextsDir, entry.Name(), name)
			if _, err := s.shims.Stat(path); err == nil {
				return werror.New(
					fmt.Sprintf("The context file %s exists beside the v1alpha2 root file %s. The CLI does not read it.", path, rootPath),
					"Run 'windsor migrate' to move its values into values.yaml.", nil)
			}
		}
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// sortedKeys returns the keys of m in ascending order.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// requireStringMap fails unless value is a map whose values are all strings.
func requireStringMap(rootPath, key string, value any) error {
	m, ok := value.(map[string]any)
	if !ok {
		return typeError(rootPath, key, "a map of strings")
	}
	for _, name := range sortedKeys(m) {
		if _, ok := m[name].(string); !ok {
			return typeError(rootPath, key+"."+name, "a string")
		}
	}
	return nil
}

// typeError returns the error for a root key that holds the wrong type.
func typeError(rootPath, key, want string) error {
	return werror.New(fmt.Sprintf("The key %q in the root file %s must be %s.", key, rootPath, want), "Fix the value.", nil)
}

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
