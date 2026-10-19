// The output functions read the Terraform outputs of a component and cache them.
// They run terraform output in a scoped context and restore the environment after the run.
// The terraform_output helper of the evaluator calls them to resolve output expressions.
// ClearCache removes the cached outputs so that the next read runs Terraform again.

package terraform

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Constants
// =============================================================================

// terraformOutputUsage tells the operator how to call the terraform_output helper.
const terraformOutputUsage = `Call terraform_output with a component ID and an output key, such as terraform_output("network", "vpc_id").`

// =============================================================================
// Types
// =============================================================================

// terraformContext provides a scoped environment for Terraform operations with automatic cleanup.
type terraformContext struct {
	ComponentID         string
	AbsModulePath       string
	TerraformArgs       *TerraformArgs
	BackendOverridePath string
	originalEnvVars     map[string]string
	provider            *terraformProvider
}

// =============================================================================
// Public Methods
// =============================================================================

// GetTerraformOutputs retrieves Terraform outputs for the specified component by running 'terraform output -json'.
// It sets up the appropriate environment (TF_DATA_DIR, backend override file) before executing the command.
// If output fails initially, it attempts to initialize the module and retry, allowing outputs to be retrieved
// even if the module hasn't been initialized yet. Returns empty map without error if outputs cannot be retrieved
// (component not ready), enabling graceful fallback via ?? operator. Only returns error for JSON parse failures
// indicating state corruption. Only output 'value' fields are returned. The 'output -json' command runs via the
// non-streaming capture path so sensitive outputs are never echoed under --verbose.
func (p *terraformProvider) GetTerraformOutputs(componentID string) (map[string]any, error) {
	return p.withTerraformContext(componentID, func(ctx *terraformContext) (map[string]any, error) {
		terraformCommand := ctx.provider.toolsManager.GetTerraformCommand()
		if terraformCommand == "" {
			terraformCommand = "terraform"
		}

		outputArgs := []string{fmt.Sprintf("-chdir=%s", ctx.AbsModulePath), "output", "-json"}
		output, err := ctx.provider.shell.ExecCaptureWithEnv(terraformCommand, nil, outputArgs...)
		if err != nil {
			chdirInitArgs := []string{fmt.Sprintf("-chdir=%s", ctx.AbsModulePath), "init", "-reconfigure"}
			chdirInitArgs = append(chdirInitArgs, ctx.TerraformArgs.InitArgs...)
			_, initErr := ctx.provider.shell.ExecSilent(terraformCommand, chdirInitArgs...)
			if initErr != nil {
				return make(map[string]any), nil
			}
			output, err = ctx.provider.shell.ExecCaptureWithEnv(terraformCommand, nil, outputArgs...)
			if err != nil {
				return make(map[string]any), nil
			}
		}

		if strings.TrimSpace(output) == "" || strings.TrimSpace(output) == "{}" {
			return make(map[string]any), nil
		}

		var outputs map[string]any
		if err := ctx.provider.Shims.JsonUnmarshal([]byte(output), &outputs); err != nil {
			return make(map[string]any), werror.Wrap(err, "failed to parse terraform output JSON for component '%s'", componentID)
		}

		result := make(map[string]any)
		for key, value := range outputs {
			if valueMap, ok := value.(map[string]any); ok {
				if outputValue, exists := valueMap["value"]; exists {
					result[key] = outputValue
				}
			}
		}

		return result, nil
	})
}

// CacheOutputs proactively fetches and caches all outputs for a component.
// This is called after terraform apply to warm the cache, so subsequent terraform_output()
// calls for this component will hit the cache instead of running terraform output again.
func (p *terraformProvider) CacheOutputs(componentID string) error {
	outputs, err := p.GetTerraformOutputs(componentID)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cache[componentID] == nil {
		p.cache[componentID] = make(map[string]any)
	}
	maps.Copy(p.cache[componentID], outputs)

	return nil
}

// ClearCache clears the session cache for all components and the config scope.
// This invalidates cached Terraform components, output values, and scope used for input evaluation,
// forcing them to be reloaded from the blueprint and re-set by the next composition.
// This is useful when the blueprint has been modified or when outputs need to be refreshed.
func (p *terraformProvider) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]map[string]any)
	p.components = nil
	p.configScope = nil
}

// =============================================================================
// Private Methods
// =============================================================================

// registerTerraformOutputHelper adds the terraform_output(component, key) helper to the evaluator.
// Blueprint expressions use the helper to read the outputs of other components through getOutput.
// Arguments other than two strings return an error with the usage as the hint.
func (p *terraformProvider) registerTerraformOutputHelper(evaluator evaluator.ExpressionEvaluator) {
	evaluator.Register("terraform_output", func(params []any, deferred bool) (any, error) {
		if len(params) != 2 {
			return nil, werror.New(fmt.Sprintf("terraform_output() requires exactly 2 arguments (component, key), got %d", len(params)), terraformOutputUsage, nil)
		}
		component, ok := params[0].(string)
		if !ok {
			return nil, werror.New(fmt.Sprintf("terraform_output() component must be a string, got %T", params[0]), terraformOutputUsage, nil)
		}
		key, ok := params[1].(string)
		if !ok {
			return nil, werror.New(fmt.Sprintf("terraform_output() key must be a string, got %T", params[1]), terraformOutputUsage, nil)
		}
		expr := fmt.Sprintf(`terraform_output("%s", "%s")`, component, key)
		value, err := p.getOutput(component, key, expr, deferred)
		if err != nil {
			return nil, err
		}
		return value, nil
	}, new(func(string, string) any))
}

// getOutput retrieves a single output value for a Terraform component by key.
// If outputs for the component are requested for the first time, all outputs are fetched from Terraform
// and cached for subsequent requests. Cached values are used for later accesses to avoid redundant retrievals.
// When deferred is false, this function returns a DeferredError to signal that the expression should be preserved.
// When deferred is true, it returns the actual output value if available, or nil if the key is absent — whether
// the component has no outputs at all or has outputs but not this key. Returning nil in both cases lets the
// ?? operator fall back uniformly across unapplied and partial-destroy states.
func (p *terraformProvider) getOutput(componentID, key string, expression string, deferred bool) (any, error) {
	if !deferred {
		return nil, &evaluator.DeferredError{
			Expression: expression,
			Message:    fmt.Sprintf("terraform output '%s' for component %s is deferred", key, componentID),
		}
	}

	p.mu.RLock()
	if cached, exists := p.cache[componentID]; exists {
		if value, exists := cached[key]; exists {
			p.mu.RUnlock()
			return value, nil
		}
		// Component outputs are cached as a complete set, so an absent key here
		// is authoritative — refetching cannot reveal it.
		p.mu.RUnlock()
		return nil, nil
	}
	p.mu.RUnlock()

	outputs, err := p.GetTerraformOutputs(componentID)
	if err != nil {
		return nil, werror.Wrap(err, "failed to get terraform outputs for component '%s'", componentID)
	}
	if len(outputs) == 0 {
		return nil, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if cached, exists := p.cache[componentID]; exists {
		if value, exists := cached[key]; exists {
			return value, nil
		}
	}

	if p.cache[componentID] == nil {
		p.cache[componentID] = make(map[string]any)
	}

	maps.Copy(p.cache[componentID], outputs)

	if value, exists := outputs[key]; exists {
		return value, nil
	}

	return nil, nil
}

// prepareTerraformContext sets up the Terraform environment of a component for one run.
// It resolves the module path, generates the arguments, sets the env vars, and writes the backend override file.
// It returns the context and the original env vars, so the caller can restore them after the run.
// An unknown component ID returns an error with a hint.
func (p *terraformProvider) prepareTerraformContext(componentID string) (*terraformContext, map[string]string, error) {
	component := p.GetTerraformComponent(componentID)
	if component == nil {
		return nil, nil, werror.New(fmt.Sprintf("component not found: %s", componentID),
			"Check that the blueprint has a terraform component with this ID.", nil)
	}

	absModulePath, err := p.resolveModulePath(component)
	if err != nil {
		return nil, nil, werror.Wrap(err, "failed to resolve module path")
	}

	terraformArgs, err := p.GenerateTerraformArgs(componentID, false)
	if err != nil {
		return nil, nil, werror.Wrap(err, "failed to generate terraform args")
	}

	envVars, err := p.getBaseEnvVarsForComponent(terraformArgs)
	if err != nil {
		return nil, nil, werror.Wrap(err, "failed to get base env vars")
	}

	originalEnvVars, err := p.applyEnvVars(envVars)
	if err != nil {
		return nil, nil, werror.Wrap(err, "failed to apply env vars")
	}

	if err := p.GenerateBackendOverride(absModulePath); err != nil {
		p.restoreEnvVars(originalEnvVars)
		return nil, nil, werror.Wrap(err, "failed to generate backend override")
	}

	backendOverridePath := filepath.Join(absModulePath, "backend_override.tf")

	ctx := &terraformContext{
		ComponentID:         componentID,
		AbsModulePath:       absModulePath,
		TerraformArgs:       terraformArgs,
		BackendOverridePath: backendOverridePath,
		originalEnvVars:     originalEnvVars,
		provider:            p,
	}

	return ctx, originalEnvVars, nil
}

// withTerraformContext sets up a Terraform environment context for a component, executes the provided function,
// and ensures cleanup. This helper ensures that environment variables are properly set, backend overrides
// are created, and all resources are cleaned up after the function completes, even if it returns an error.
// This pattern provides a safe way to execute Terraform operations in an isolated environment context.
// Returns the result of the function. Returns empty map without error if context cannot be prepared
// (component not found, etc.), enabling graceful fallback via ?? operator in expressions.
func (p *terraformProvider) withTerraformContext(componentID string, fn func(*terraformContext) (map[string]any, error)) (map[string]any, error) {
	ctx, originalEnvVars, err := p.prepareTerraformContext(componentID)
	if err != nil {
		return nil, werror.Wrap(err, "failed to prepare terraform context for component '%s'", componentID)
	}

	cleanup := func() {
		if _, err := p.Shims.Stat(ctx.BackendOverridePath); err == nil {
			_ = p.Shims.Remove(ctx.BackendOverridePath)
		}
		p.restoreEnvVars(originalEnvVars)
	}
	defer cleanup()

	return fn(ctx)
}

// applyEnvVars applies environment variables and returns a map of original values for restoration.
// It stores the current value of each environment variable before setting the new value, allowing
// the caller to restore the original environment state. If setting any variable fails, it attempts
// to restore all previously set variables before returning the error.
func (p *terraformProvider) applyEnvVars(envVars map[string]string) (map[string]string, error) {
	originalEnvVars := make(map[string]string)
	for key, value := range envVars {
		originalValue := p.Shims.Getenv(key)
		originalEnvVars[key] = originalValue
		if err := p.Shims.Setenv(key, value); err != nil {
			p.restoreEnvVars(originalEnvVars)
			return nil, err
		}
	}
	return originalEnvVars, nil
}

// restoreEnvVar restores an environment variable to its original value or unsets it if it was empty.
// This is used to clean up environment variables that were temporarily set for Terraform operations,
// ensuring that the process environment is restored to its original state after operations complete.
func (p *terraformProvider) restoreEnvVar(key, originalValue string) {
	if originalValue != "" {
		_ = p.Shims.Setenv(key, originalValue)
	} else {
		_ = p.Shims.Unsetenv(key)
	}
}

// restoreEnvVars restores environment variables to their original values or unsets them if they were empty.
// This iterates through the provided map of original values and restores each environment variable,
// ensuring that all temporarily set environment variables are cleaned up after Terraform operations complete.
func (p *terraformProvider) restoreEnvVars(originalEnvVars map[string]string) {
	for key, originalValue := range originalEnvVars {
		p.restoreEnvVar(key, originalValue)
	}
}
