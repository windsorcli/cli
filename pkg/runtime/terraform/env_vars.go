// The environment functions generate the Terraform arguments and variables of a component.
// They set the TF_DATA_DIR, TF_CLI_ARGS, and TF_VAR variables for each component.
// They load the Terraform-scoped values from the .env file of the context.
// They format arguments so that Terraform reads them correctly from the environment.

package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/dotenv"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	secretsRuntime "github.com/windsorcli/cli/pkg/runtime/secrets"
)

// =============================================================================
// Public Methods
// =============================================================================

// GenerateTerraformArgs returns the Terraform arguments of a component for the configured backend.
// It finds the var files, adds the -backend-config arguments, and builds the arguments of each subcommand.
// It adds -lock-timeout to init and to each subcommand that changes state. The default is 5m.
// It checks terraform.lock.timeout first, so a typo gets a Windsor error, not a Terraform parse failure.
func (p *terraformProvider) GenerateTerraformArgs(componentID string, interactive bool) (*TerraformArgs, error) {
	_, configRoot, windsorScratchPath, err := p.providerScope()
	if err != nil {
		return nil, werror.Wrap(err, "error resolving provider scope")
	}

	component := p.GetTerraformComponent(componentID)
	actualComponentID := componentID
	if component != nil {
		actualComponentID = component.GetID()
	}

	patterns := []string{
		filepath.Join(windsorScratchPath, "terraform", actualComponentID, "terraform.tfvars"),
		filepath.Join(configRoot, "terraform", actualComponentID+".tfvars"),
		filepath.Join(configRoot, "terraform", actualComponentID+".tfvars.json"),
	}
	if component != nil && component.Path != actualComponentID {
		patterns = append(patterns,
			filepath.Join(configRoot, "terraform", component.Path+".tfvars"),
			filepath.Join(configRoot, "terraform", component.Path+".tfvars.json"),
		)
	}

	var varFileArgs []string
	for _, pattern := range patterns {
		if _, err := p.Shims.Stat(filepath.FromSlash(pattern)); err != nil {
			if !os.IsNotExist(err) {
				return nil, werror.Wrap(err, "error checking file")
			}
		} else {
			slashPath := filepath.ToSlash(pattern)
			varFileArgs = append(varFileArgs, fmt.Sprintf("-var-file=%s", slashPath))
		}
	}
	tfDataDir, err := p.GetTFDataDir(actualComponentID)
	if err != nil {
		return nil, werror.Wrap(err, "error getting TF_DATA_DIR")
	}
	tfPlanPath := filepath.ToSlash(filepath.Join(tfDataDir, "terraform.tfplan"))

	backendConfigArgs, err := p.generateBackendConfigArgs(actualComponentID, configRoot)
	if err != nil {
		return nil, werror.Wrap(err, "error generating backend config args")
	}

	// InitArgs carries only configuration (backend selection + backend-config) — execution
	// policy flags like -upgrade, -force-copy, -migrate-state, or -reconfigure are added by
	// each caller of runTerraformInit according to the operation it's performing.
	initArgs := []string{"-backend=true"}
	initArgs = append(initArgs, backendConfigArgs...)

	planArgs := []string{fmt.Sprintf("-out=%s", tfPlanPath)}
	planArgs = append(planArgs, varFileArgs...)

	applyArgs := []string{}
	refreshArgs := []string{}
	refreshArgs = append(refreshArgs, varFileArgs...)

	planDestroyArgs := []string{"-destroy"}
	planDestroyArgs = append(planDestroyArgs, varFileArgs...)

	destroyArgs := []string{}
	if !interactive {
		destroyArgs = append(destroyArgs, "-auto-approve")
	}
	destroyArgs = append(destroyArgs, varFileArgs...)

	if component != nil && component.Parallelism != nil {
		parallelism := component.Parallelism.ToInt()
		if parallelism != nil {
			parallelismArg := fmt.Sprintf("-parallelism=%d", *parallelism)
			applyArgs = append(applyArgs, parallelismArg)
			destroyArgs = append(destroyArgs, parallelismArg)
		}
	}

	lockTimeout := p.configHandler.GetString("terraform.lock.timeout", "5m")
	if _, err := time.ParseDuration(lockTimeout); err != nil {
		return nil, werror.New(fmt.Sprintf("invalid terraform.lock.timeout %q", lockTimeout),
			"Set terraform.lock.timeout to a duration, such as 5m or 30s.", err)
	}
	lockArg := fmt.Sprintf("-lock-timeout=%s", lockTimeout)
	initArgs = append(initArgs, lockArg)
	planArgs = append(planArgs, lockArg)
	applyArgs = append(applyArgs, lockArg)
	refreshArgs = append(refreshArgs, lockArg)
	planDestroyArgs = append(planDestroyArgs, lockArg)
	destroyArgs = append(destroyArgs, lockArg)
	importArgs := append([]string{}, varFileArgs...)
	importArgs = append(importArgs, lockArg)

	applyArgs = append(applyArgs, tfPlanPath)

	return &TerraformArgs{
		TFDataDir:       strings.TrimSpace(tfDataDir),
		InitArgs:        initArgs,
		PlanArgs:        planArgs,
		ApplyArgs:       applyArgs,
		RefreshArgs:     refreshArgs,
		ImportArgs:      importArgs,
		DestroyArgs:     destroyArgs,
		PlanDestroyArgs: planDestroyArgs,
		BackendConfig:   strings.Join(backendConfigArgs, " "),
	}, nil
}

// GetTFDataDir calculates the TF_DATA_DIR path for a given component ID.
// It resolves the component ID to the actual component ID (using GetID if component exists),
// then constructs the path as ${windsorScratchPath}/.terraform/${componentID}.
// This path is used by Terraform to store its working directory data and state.
// Returns the path with forward slashes or an error if scratch path lookup fails.
func (p *terraformProvider) GetTFDataDir(componentID string) (string, error) {
	_, _, windsorScratchPath, err := p.providerScope()
	if err != nil {
		return "", werror.Wrap(err, "error resolving provider scope")
	}

	component := p.GetTerraformComponent(componentID)
	actualComponentID := componentID
	if component != nil {
		actualComponentID = component.GetID()
	}

	tfDataDir := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", actualComponentID))
	return tfDataDir, nil
}

// GetEnvVars constructs the environment variables required for Terraform execution for the specified
// component ID: base vars (TF_DATA_DIR, TF_CLI_ARGS_*, TF_VAR_context_*), contexts/<context>/terraform/.env
// content, and TF_VAR_* from the component's own inputs (evaluated via terraform_output(); outputs from
// other components are used only to evaluate inputs, never emitted as separate TF_VAR_* variables).
// Returns the environment variables map, the terraform/.env key names (for callers that must pass them
// through a narrower allowlist or track them for later cleanup), the TerraformArgs struct, and any error.
func (p *terraformProvider) GetEnvVars(componentID string, interactive bool) (map[string]string, []string, *TerraformArgs, error) {
	terraformArgs, err := p.GenerateTerraformArgs(componentID, interactive)
	if err != nil {
		return nil, nil, nil, werror.Wrap(err, "error generating terraform args")
	}

	envVars, err := p.getBaseEnvVarsForComponent(terraformArgs)
	if err != nil {
		return nil, nil, nil, err
	}

	scopedEnvVars, err := p.loadTerraformScopedEnv()
	if err != nil {
		return nil, nil, nil, err
	}
	scopedKeys := make([]string, 0, len(scopedEnvVars))
	for k, v := range scopedEnvVars {
		scopedKeys = append(scopedKeys, k)
		if _, exists := envVars[k]; !exists {
			envVars[k] = v
		}
	}

	if componentID != "" && p.evaluator != nil {
		component := p.GetTerraformComponent(componentID)
		if component != nil && component.Inputs != nil && len(component.Inputs) > 0 {
			for key, value := range component.Inputs {
				envKey := fmt.Sprintf("TF_VAR_%s", key)
				if deferredExpr, ok := evaluator.DeferredExpression(value); ok {
					value = deferredExpr
				}
				if evaluator.ContainsExpression(value) {
					p.mu.RLock()
					evalScope := p.configScope
					p.mu.RUnlock()
					nonDeferred, err := p.evaluator.EvaluateMap(map[string]any{key: value}, "", evalScope, false)
					if err != nil {
						return nil, nil, nil, werror.Wrap(err, "error evaluating input '%s' for component %s", key, componentID)
					}
					nonDeferredValue, exists := nonDeferred[key]
					if !exists {
						continue
					}
					if !evaluator.ContainsExpression(nonDeferredValue) {
						continue
					}
					if shouldUseTFVarCache(nonDeferredValue) {
						noCache := p.Shims.Getenv("NO_CACHE")
						useCache := noCache == "" || noCache == "0" || noCache == "false" || noCache == "False"
						if useCache {
							if cachedValue, exists := p.Shims.LookupEnv(envKey); exists && cachedValue != "" && !strings.Contains(cachedValue, "<ERROR") {
								envVars[envKey] = cachedValue
								continue
							}
						}
					}
					evaluated, err := p.evaluator.EvaluateMap(map[string]any{key: value}, "", evalScope, true)
					if err != nil {
						return nil, nil, nil, werror.Wrap(err, "error evaluating input '%s' for component %s", key, componentID)
					}
					evaluatedValue, exists := evaluated[key]
					if !exists {
						continue
					}
					// When a deferred terraform_output resolves to nil the dependency
					// has not been applied yet. Skip the variable so Terraform uses its
					// declared default rather than receiving the string "null".
					if evaluatedValue == nil {
						continue
					}
					var envValue string
					if valueStr, ok := evaluatedValue.(string); ok {
						envValue = valueStr
					} else {
						valueBytes, err := p.Shims.JsonMarshal(evaluatedValue)
						if err != nil {
							continue
						}
						envValue = string(valueBytes)
					}
					envVars[envKey] = envValue
				}
			}
		}
	}

	return envVars, scopedKeys, terraformArgs, nil
}

// TerraformScopedEnvKeys returns the key names declared in contexts/<context>/terraform/.env,
// without resolving any values or paying secret-resolution cost. Used to identify which
// environment variables to unset when leaving a Terraform directory.
func (p *terraformProvider) TerraformScopedEnvKeys() ([]string, error) {
	_, configRoot, _, err := p.providerScope()
	if err != nil {
		return nil, werror.Wrap(err, "error resolving provider scope")
	}

	dotEnvPath := filepath.Join(configRoot, "terraform", ".env")

	data, err := p.Shims.ReadFile(dotEnvPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, werror.Wrap(err, "error reading %s", dotEnvPath)
	}

	parsed := dotenv.Parse(string(data))
	keys := make([]string, 0, len(parsed))
	for k := range parsed {
		keys = append(keys, k)
	}

	return keys, nil
}

// FormatArgsForEnv formats CLI arguments for use in environment variables.
// It adds quotes around file paths and values that need them for shell compatibility, ensuring that
// paths with spaces or special characters are properly escaped. Handles Unix absolute paths (/path),
// relative paths (./path), and Windows drive letter paths (both D:\path and D:/path formats).
// This is necessary because Terraform CLI arguments passed via TF_CLI_ARGS_* environment variables
// must be properly quoted to work correctly across different shell environments.
func (p *terraformProvider) FormatArgsForEnv(args []string) string {
	formatted := make([]string, len(args))
	for i, arg := range args {
		if strings.HasPrefix(arg, "-var-file=") {
			value := strings.TrimPrefix(arg, "-var-file=")
			formatted[i] = fmt.Sprintf("-var-file=\"%s\"", value)
		} else if strings.HasPrefix(arg, "-out=") {
			value := strings.TrimPrefix(arg, "-out=")
			formatted[i] = fmt.Sprintf("-out=\"%s\"", value)
		} else if strings.HasPrefix(arg, "-backend-config=") {
			if strings.Contains(arg, "=") && !strings.HasPrefix(strings.SplitN(arg, "=", 2)[1], "\"") {
				parts := strings.SplitN(arg, "=", 2)
				formatted[i] = fmt.Sprintf("%s=\"%s\"", parts[0], parts[1])
			} else {
				formatted[i] = arg
			}
		} else if !strings.HasPrefix(arg, "-") && (strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, ".") || (len(arg) >= 2 && arg[1] == ':' && ((arg[0] >= 'A' && arg[0] <= 'Z') || (arg[0] >= 'a' && arg[0] <= 'z')))) {
			formatted[i] = fmt.Sprintf("\"%s\"", arg)
		} else {
			formatted[i] = arg
		}
	}
	return strings.Join(formatted, " ")
}

// =============================================================================
// Private Methods
// =============================================================================

// getBaseEnvVarsForComponent returns the base environment variables for a Terraform component
// without TF_VAR outputs from other components. This is the core set of env vars needed
// for any Terraform operation on the component, including TF_DATA_DIR, TF_CLI_ARGS_* for all
// Terraform commands, TF_VAR_context (context name), TF_VAR_project_root (project root path),
// and TF_VAR_context_* variables that provide context information to Terraform.
// These variables ensure Terraform can locate its state, use the correct backend configuration,
// and have access to context-specific information during execution.
func (p *terraformProvider) getBaseEnvVarsForComponent(terraformArgs *TerraformArgs) (map[string]string, error) {
	contextName, configRoot, _, err := p.providerScope()
	if err != nil {
		return nil, werror.Wrap(err, "error resolving provider scope")
	}
	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return nil, werror.Wrap(err, "error getting project root")
	}

	envVars := make(map[string]string)
	envVars["TF_DATA_DIR"] = terraformArgs.TFDataDir
	envVars["TF_CLI_ARGS_init"] = p.FormatArgsForEnv(terraformArgs.InitArgs)
	envVars["TF_CLI_ARGS_plan"] = p.FormatArgsForEnv(terraformArgs.PlanArgs)
	envVars["TF_CLI_ARGS_apply"] = p.FormatArgsForEnv(terraformArgs.ApplyArgs)
	envVars["TF_CLI_ARGS_refresh"] = p.FormatArgsForEnv(terraformArgs.RefreshArgs)
	envVars["TF_CLI_ARGS_import"] = p.FormatArgsForEnv(terraformArgs.ImportArgs)
	envVars["TF_CLI_ARGS_destroy"] = p.FormatArgsForEnv(terraformArgs.DestroyArgs)
	envVars["TF_VAR_context"] = contextName
	envVars["TF_VAR_project_root"] = filepath.ToSlash(projectRoot)
	envVars["TF_VAR_context_path"] = filepath.ToSlash(configRoot)
	envVars["TF_VAR_context_id"] = p.configHandler.GetString("id", "")

	if p.Shims.Goos() == "windows" {
		envVars["TF_VAR_os_type"] = "windows"
	} else {
		envVars["TF_VAR_os_type"] = "unix"
	}

	return envVars, nil
}

// loadTerraformScopedEnv reads contexts/<context>/terraform/.env, resolving secret(...)
// expressions and registering every value with the shell for output scrubbing. A key
// already cached in the shell is omitted rather than re-evaluated. Returns an empty map
// when the file is absent, and warns (without failing) on loose file permissions. Reused
// by both the interactive shell hook and windsor up/apply/plan/destroy, since both paths
// call GetEnvVars per component.
func (p *terraformProvider) loadTerraformScopedEnv() (map[string]string, error) {
	envVars := make(map[string]string)

	_, configRoot, _, err := p.providerScope()
	if err != nil {
		return nil, werror.Wrap(err, "error resolving provider scope")
	}

	dotEnvPath := filepath.Join(configRoot, "terraform", ".env")

	info, err := p.Shims.Stat(dotEnvPath)
	if err != nil {
		if os.IsNotExist(err) {
			return envVars, nil
		}
		return nil, werror.Wrap(err, "error checking %s", dotEnvPath)
	}

	dotenv.WarnOnLoosePermissions(p.warningWriter, p.Shims.Goos(), dotEnvPath, info.Mode())

	data, err := p.Shims.ReadFile(dotEnvPath)
	if err != nil {
		return nil, werror.Wrap(err, "error reading %s", dotEnvPath)
	}

	for k, v := range dotenv.Parse(string(data)) {
		normalizedValue := secretsRuntime.NormalizeLegacyBraces(v)

		if p.evaluator != nil && evaluator.ContainsExpression(normalizedValue) {
			if existingValue, exists := p.Shims.LookupEnv(k); exists &&
				dotenv.ShouldUseCache(p.Shims.LookupEnv) && !strings.Contains(existingValue, "<ERROR") {
				p.shell.RegisterSecret(existingValue)
				continue
			}
			envVars[k] = dotenv.EvaluateExpressionValue(p.evaluator, normalizedValue)
		} else {
			envVars[k] = normalizedValue
		}
		p.shell.RegisterSecret(envVars[k])
	}

	return envVars, nil
}

// =============================================================================
// Helpers
// =============================================================================

// sanitizeForK8s ensures a string is compatible with Kubernetes naming conventions by converting
// to lowercase, replacing invalid characters, and trimming to a maximum length of 63 characters.
func sanitizeForK8s(input string) string {
	sanitized := strings.ToLower(input)
	sanitized = regexp.MustCompile(`[_]+`).ReplaceAllString(sanitized, "-")
	sanitized = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(sanitized, "-")
	sanitized = regexp.MustCompile(`-+`).ReplaceAllString(sanitized, "-")
	sanitized = strings.Trim(sanitized, "-")
	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
	}
	return sanitized
}

// shouldUseTFVarCache reports whether TF_VAR cache reuse should be applied for this deferred expression.
func shouldUseTFVarCache(value any) bool {
	expression, ok := deferredExpressionString(value)
	if !ok {
		return false
	}

	trimmed := strings.TrimSpace(expression)
	if body, wrapped := evaluator.ExpressionBody(trimmed); wrapped {
		trimmed = body
	}
	return secretsRuntime.IsCacheable(trimmed)
}

// deferredExpressionString returns the deferred expression string when available.
func deferredExpressionString(value any) (string, bool) {
	if s, ok := value.(string); ok {
		return s, true
	}
	return evaluator.DeferredExpression(value)
}
