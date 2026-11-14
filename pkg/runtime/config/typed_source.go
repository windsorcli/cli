package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/paths"
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
func (s *typedSource) LoadRoot(resolver *paths.Resolver, contextName string) (map[string]any, bool, error) {
	rootConfigPath := filepath.Join(resolver.ProjectRoot(), paths.RootFileName)
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
		if err := s.validateRoot(rootConfigPath, fileData, rootKeys); err != nil {
			return nil, false, err
		}
		if err := s.rejectContextFiles(rootConfigPath, resolver); err != nil {
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

// LoadRootEnvironment returns the environment map of a v1alpha2 root file as a config layer, in the
// form {"environment": {...}}. It returns nil when the file is missing, is not v1alpha2, or has no
// environment key. A read error fails the call. LoadRoot validates the file first, so this method does not validate it again.
func (s *typedSource) LoadRootEnvironment(projectRoot string) (map[string]any, error) {
	rootConfigPath := filepath.Join(projectRoot, "windsor.yaml")
	if _, err := s.shims.Stat(rootConfigPath); err != nil {
		return nil, nil
	}
	fileData, err := s.shims.ReadFile(rootConfigPath)
	if err != nil {
		return nil, werror.Wrap(err, "error reading root config file")
	}
	var root v1alpha2.Root
	if err := s.shims.YamlUnmarshal(fileData, &root); err != nil {
		return nil, werror.Wrap(err, "error unmarshalling root config")
	}
	if root.Version != v1alpha2.Version || len(root.Environment) == 0 {
		return nil, nil
	}
	environment := make(map[string]any, len(root.Environment))
	for name, value := range root.Environment {
		environment[name] = value
	}
	return map[string]any{"environment": environment}, nil
}

// LoadContext loads legacy context-level windsor.yaml or windsor.yml for a context.
func (s *typedSource) LoadContext(resolver *paths.Resolver, contextName string) (map[string]any, bool, error) {
	contextConfigDir := resolver.ContextDir(contextName)
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

// validateRoot decodes a v1alpha2 root file into the Root type. It fails on a contexts key, on any
// key outside the root schema, and on a wrong type in environment or paths.
func (s *typedSource) validateRoot(rootPath string, fileData []byte, rootKeys map[string]any) error {
	if _, ok := rootKeys["contexts"]; ok {
		return werror.New(
			fmt.Sprintf("The v1alpha2 root file %s has the key %q. The root file cannot hold per-context values.", rootPath, "contexts"),
			"Run 'windsor migrate' to move the values into contexts/<name>/values.yaml.", nil)
	}
	if err := rejectEmptyPaths(rootPath, rootKeys["paths"]); err != nil {
		return err
	}
	var root v1alpha2.Root
	if err := s.shims.YamlUnmarshalStrict(fileData, &root); err != nil {
		return werror.New(
			fmt.Sprintf("The v1alpha2 root file %s is invalid. The root file allows only version, environment, and paths.\n%s", rootPath, err),
			"Fix the key or value. Run 'windsor migrate' if the key comes from a v1alpha1 project.", err)
	}
	return nil
}

// rejectContextFiles fails when a contexts/<name>/windsor.yaml or windsor.yml exists. A v1alpha2
// root file does not read these files, so their settings would stop applying without notice.
func (s *typedSource) rejectContextFiles(rootPath string, resolver *paths.Resolver) error {
	contextsDir := resolver.Contexts()
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

// rejectEmptyPaths fails when the paths block of the root file sets a key to an empty string. The
// typed decode cannot tell an empty value from a missing key, so this check reads the raw map.
func rejectEmptyPaths(rootPath string, raw any) error {
	block, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	for key, value := range block {
		if text, isText := value.(string); isText && text == "" {
			return werror.New(
				fmt.Sprintf("The v1alpha2 root file %s sets paths.%s to an empty value.", rootPath, key),
				"Set a relative directory, or remove the key.", nil)
		}
	}
	return nil
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
