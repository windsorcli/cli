// The handler_persistence file holds the load and save steps of the ConfigHandler.
// It provides the merge of the root, context, workstation, and values files into one map.
// It writes values.yaml and workstation.yaml for the current context.
// A load converts v1alpha1 values into the v1alpha2 header.

package config

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/paths"
)

// =============================================================================
// Public Methods
// =============================================================================

// LoadConfigString loads YAML configuration directly into the internal data map for testing purposes.
// It unmarshals the provided YAML string and, if a "contexts" key exists, extracts and merges only
// the configuration for the current context. If no "contexts" structure is present, it merges the entire
// map. It then converts v1alpha1 values into the v1alpha2 header. This method is primarily intended for
// test helpers and not for production use; load configuration in production with LoadConfig instead.
// Returns an error if YAML unmarshalling fails.
func (c *configHandler) LoadConfigString(content string) error {
	if content == "" {
		return nil
	}

	var dataMap map[string]any
	if err := c.shims.YamlUnmarshal([]byte(content), &dataMap); err != nil {
		return werror.Wrap(err, "error unmarshalling yaml")
	}

	contextName := c.GetContext()

	if contexts, ok := dataMap["contexts"]; ok {
		var contextsMap map[string]any
		switch v := contexts.(type) {
		case map[string]any:
			contextsMap = v
		case map[any]any:
			contextsMap = make(map[string]any)
			for k, val := range v {
				if strKey, ok := k.(string); ok {
					contextsMap[strKey] = val
				}
			}
		}

		if contextsMap != nil {
			if contextData, ok := contextsMap[contextName]; ok {
				var contextMap map[string]any
				switch v := contextData.(type) {
				case map[string]any:
					contextMap = v
				case map[any]any:
					contextMap = c.convertInterfaceMap(v)
				}

				if contextMap != nil {
					c.data = c.deepMerge(c.data, contextMap)
					convertV1alpha1(c.data)
					c.loaded = true
					return nil
				}
			}
		}
	}

	c.data = c.deepMerge(c.data, dataMap)
	convertV1alpha1(c.data)
	c.loaded = true

	return nil
}

// LoadConfig loads and merges all configuration sources for the current context into the internal data map.
// It performs the following actions, in order: loads schema defaults (if available); merges the v1alpha2 root
// environment map; merges root windsor.yaml
// context section (if it exists); merges any context-specific windsor.yaml/yml file; then merges values.yaml for
// dynamic fields, validating values.yaml against the loaded schema (if one is present). All configuration is
// accumulated into one map structure, and v1alpha1 values convert into the v1alpha2 header. If any file is loaded,
// config state is marked as loaded. Returns an error for any I/O or validation failure. Must call Initialize() first.
func (c *configHandler) LoadConfig() error {
	if c.shell == nil {
		return fmt.Errorf("shell not initialized")
	}
	return c.LoadConfigForContext(c.GetContext())
}

// LoadConfigForContext loads and merges all configuration sources for the specified context into the internal data map
// without modifying the current context state. This method performs the same actions as LoadConfig but uses the
// provided contextName parameter directly instead of reading from the current context. It does not write to the
// .windsor/context file or set the WINDSOR_CONTEXT environment variable, making it safe for read-only operations
// like listing contexts. Returns an error for any I/O or validation failure.
func (c *configHandler) LoadConfigForContext(contextName string) error {
	if c.shell == nil {
		return fmt.Errorf("shell not initialized")
	}
	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return werror.Wrap(err, "error retrieving project root")
	}

	hasLoadedFiles := false

	resolver, err := c.loadResolver(projectRoot)
	if err != nil {
		return err
	}

	if c.schemaValidator != nil && c.schemaValidator.Schema == nil {
		schemaPath := filepath.Join(resolver.TemplateDir(), "schema.yaml")
		if _, err := c.shims.Stat(schemaPath); err == nil {
			if err := c.LoadSchema(schemaPath); err != nil {
				return werror.Wrap(err, "error loading schema")
			}
		}
	}

	rootValues, rootLoaded, err := c.typed.LoadRoot(resolver, contextName)
	if err != nil {
		return err
	}
	if rootLoaded {
		hasLoadedFiles = true
	}
	rootEnvironment, err := c.typed.LoadRootEnvironment(projectRoot)
	if err != nil {
		return err
	}
	c.rootEnvironment = nil
	if rootEnvironment != nil {
		c.rootEnvironment, _ = rootEnvironment["environment"].(map[string]any)
		c.data = c.deepMerge(c.data, rootEnvironment)
	}
	if rootValues != nil {
		c.data = c.deepMerge(c.data, rootValues)
	}

	contextValues, contextLoaded, err := c.typed.LoadContext(resolver, contextName)
	if err != nil {
		return err
	}
	if contextLoaded {
		hasLoadedFiles = true
	}
	if contextValues != nil {
		c.data = c.deepMerge(c.data, contextValues)
	}

	workstationValues, _, err := c.workstation.Load(projectRoot, contextName)
	if err != nil {
		return err
	}
	if workstationValues != nil {
		c.data = c.deepMerge(c.data, workstationValues)
	}

	valuesData, valuesLoaded, err := c.values.Load(resolver, contextName)
	if err != nil {
		return err
	}
	if valuesLoaded {
		hasLoadedFiles = true
	}
	if valuesData != nil {
		c.data = c.deepMerge(c.data, valuesData)
	}
	convertV1alpha1(c.data)

	if hasLoadedFiles {
		c.loaded = true
	}

	return nil
}

// IsNewContext reports whether no config source holds the current context. The sources are the context
// entry in the root windsor.yaml, the context windsor.yaml or windsor.yml, and contexts/<name>/values.yaml.
// It reads the files and does not change the loaded config. Call it before SaveConfig.
func (c *configHandler) IsNewContext() (bool, error) {
	if c.shell == nil {
		return false, fmt.Errorf("shell not initialized")
	}
	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return false, werror.Wrap(err, "error retrieving project root")
	}
	contextName := c.GetContext()
	resolver, err := c.resolver(projectRoot)
	if err != nil {
		return false, err
	}

	rootValues, _, err := c.typed.LoadRoot(resolver, contextName)
	if err != nil {
		return false, err
	}
	if rootValues != nil {
		return false, nil
	}
	_, contextLoaded, err := c.typed.LoadContext(resolver, contextName)
	if err != nil {
		return false, err
	}
	if contextLoaded {
		return false, nil
	}
	_, valuesLoaded, err := c.values.Load(resolver, contextName)
	if err != nil {
		return false, err
	}
	return !valuesLoaded, nil
}

// SaveConfig writes the current configuration to values.yaml for the context, creating the root
// windsor.yaml first if missing. A missing values.yaml gets the full config; an existing one is
// patched at Set-touched paths when overwrite is true, and left alone otherwise. A successful
// write clears the pending Set paths, so a later SaveConfig only patches what changed since.
func (c *configHandler) SaveConfig(overwrite ...bool) error {
	if c.shell == nil {
		return fmt.Errorf("shell not initialized")
	}

	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}

	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return werror.Wrap(err, "error retrieving project root")
	}

	if !c.shell.IsGlobal() {
		if err := c.typed.EnsureRoot(projectRoot); err != nil {
			return err
		}
	}

	resolver, err := c.resolver(projectRoot)
	if err != nil {
		return err
	}

	deletes := make([]string, 0, len(c.pendingDeletes))
	for key := range c.pendingDeletes {
		deletes = append(deletes, key)
	}

	wrote, err := c.values.Save(
		resolver,
		c.GetContext(),
		c.dataWithoutRootEnvironment(),
		c.pendingOverrides,
		deletes,
		shouldOverwrite,
		c.getPersistencePolicyInput(),
	)
	if err != nil {
		return err
	}
	if wrote {
		c.pendingOverrides = nil
		c.pendingDeletes = nil
	}

	return nil
}

// dataWithoutRootEnvironment returns the config data minus the environment entries that come only
// from the root file. An entry stays when its value differs from the root value, or when a Set call
// wrote it. The method returns c.data itself when the root file has no environment.
func (c *configHandler) dataWithoutRootEnvironment() map[string]any {
	if len(c.rootEnvironment) == 0 {
		return c.data
	}
	environment, _ := c.data["environment"].(map[string]any)
	if environment == nil {
		return c.data
	}
	pending, _ := c.pendingOverrides["environment"].(map[string]any)
	kept := make(map[string]any, len(environment))
	for name, value := range environment {
		rootValue, fromRoot := c.rootEnvironment[name]
		_, set := pending[name]
		if fromRoot && rootValue == value && !set {
			continue
		}
		kept[name] = value
	}
	data := make(map[string]any, len(c.data))
	for key, value := range c.data {
		data[key] = value
	}
	if len(kept) == 0 {
		delete(data, "environment")
	} else {
		data["environment"] = kept
	}
	return data
}

// SaveWorkstationState extracts workstation-managed keys from config data and writes them
// to .windsor/contexts/<context>/workstation.yaml. This is the canonical persistence point for system-derived
// workstation configuration (runtime, address, platform, DNS settings, network CIDR block).
func (c *configHandler) SaveWorkstationState() error {
	if c.shell == nil {
		return fmt.Errorf("shell not initialized")
	}
	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return werror.Wrap(err, "error retrieving project root")
	}
	return c.workstation.Save(
		projectRoot,
		c.GetContext(),
		c.data,
		c.getPersistencePolicyInput(),
	)
}

// =============================================================================
// Private Methods
// =============================================================================

// loadResolver reads the root file, validates its paths block, and stores the resolver for later
// calls. It returns the resolver for projectRoot.
func (c *configHandler) loadResolver(projectRoot string) (*paths.Resolver, error) {
	resolver, err := paths.Load(projectRoot, c.pathShims())
	if err != nil {
		return nil, err
	}
	c.storeResolver(projectRoot, resolver)
	return resolver, nil
}

// resolver returns the stored resolver for projectRoot, and loads it when none is stored.
func (c *configHandler) resolver(projectRoot string) (*paths.Resolver, error) {
	if cache := c.projectPaths; cache != nil {
		cache.mu.Lock()
		stored, root := cache.resolver, cache.root
		cache.mu.Unlock()
		if stored != nil && root == projectRoot {
			return stored, nil
		}
	}
	return c.loadResolver(projectRoot)
}

// storeResolver keeps resolver as the resolver of projectRoot.
func (c *configHandler) storeResolver(projectRoot string, resolver *paths.Resolver) {
	if c.projectPaths == nil {
		return
	}
	c.projectPaths.mu.Lock()
	c.projectPaths.root, c.projectPaths.resolver = projectRoot, resolver
	c.projectPaths.mu.Unlock()
}

// pathShims returns the shims of the paths package, built from the shims of the handler.
func (c *configHandler) pathShims() *paths.Shims {
	return &paths.Shims{Shims: c.shims.Shims, EvalSymlinks: filepath.EvalSymlinks}
}

// getPersistencePolicyInput builds policy input for persistence ownership decisions.
func (c *configHandler) getPersistencePolicyInput() persistencePolicyInput {
	contextName := c.GetContext()
	return persistencePolicyInput{
		IsDevMode:          c.IsDevMode(contextName),
		WorkstationRuntime: c.GetString("workstation.runtime"),
	}
}
