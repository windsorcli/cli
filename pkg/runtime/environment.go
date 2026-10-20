// The environment functions of the Runtime collect the environment variables and aliases of a context.
// It provides the session reset check and the env printers for the configured platform and cluster.
// LoadEnvironment runs the printers in a fixed order, so a later printer overrides an earlier one.
// It sets the variables in the process and then runs the post-env hook of each printer.

package runtime

import (
	"maps"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
)

// =============================================================================
// Public Methods
// =============================================================================

// HandleSessionReset resets the managed env vars when the shell sets a reset flag or WINDSOR_SESSION_TOKEN is not set.
// A reset sets NO_CACHE to "true" only if NO_CACHE is not set, so an explicit NO_CACHE=false stays.
// This is necessary because commands such as `windsor exec` have no session token, so each run resets.
// A reset also clears the Terraform output cache.
func (rt *Runtime) HandleSessionReset() error {
	hasSessionToken := rt.Shims.Getenv("WINDSOR_SESSION_TOKEN") != ""
	shouldReset, err := rt.Shell.CheckResetFlags()
	if err != nil {
		return werror.Wrap(err, "failed to check reset flags")
	}
	if !hasSessionToken {
		shouldReset = true
	}
	if shouldReset {
		rt.Shell.Reset()
		if _, noCacheSet := rt.Shims.LookupEnv("NO_CACHE"); !noCacheSet {
			if err := rt.Shims.Setenv("NO_CACHE", "true"); err != nil {
				return werror.Wrap(err, "failed to set NO_CACHE")
			}
		}
		if rt.TerraformProvider != nil {
			rt.TerraformProvider.ClearCache()
		}
	}

	return nil
}

// LoadEnvironment collects the env vars and aliases of each configured env printer and sets the env vars in the process.
// If decrypt is true, it loads the secrets before the printers run.
// After it sets the env vars, it runs the post-env hook of each printer and returns the first hook error.
func (rt *Runtime) LoadEnvironment(decrypt bool) error {
	rt.initializeEnvPrinters()
	rt.initializeToolsManager()

	if err := rt.InitializeComponents(); err != nil {
		return werror.Wrap(err, "failed to initialize environment components")
	}

	if decrypt {
		if err := rt.loadSecrets(); err != nil {
			return werror.Wrap(err, "failed to load secrets")
		}
	}

	allEnvVars := make(map[string]string)
	allAliases := make(map[string]string)

	for _, printer := range rt.getAllEnvPrinters() {
		if printer != nil {
			envVars, err := printer.GetEnvVars()
			if err != nil {
				return werror.Wrap(err, "error getting environment variables")
			}
			maps.Copy(allEnvVars, envVars)

			aliases, err := printer.GetAlias()
			if err != nil {
				return werror.Wrap(err, "error getting aliases")
			}
			maps.Copy(allAliases, aliases)
		}
	}

	rt.envVars = allEnvVars
	rt.aliases = allAliases

	for key, value := range allEnvVars {
		if err := rt.Shims.Setenv(key, value); err != nil {
			return werror.Wrap(err, "error setting environment variable %s", key)
		}
	}

	var firstError error
	for _, printer := range rt.getAllEnvPrinters() {
		if printer != nil {
			if err := printer.PostEnvHook(); err != nil && firstError == nil {
				firstError = err
			}
		}
	}

	if firstError != nil {
		return werror.Wrap(firstError, "failed to execute post env hooks")
	}

	return nil
}

// GetEnvVars returns a copy of the collected environment variables.
func (rt *Runtime) GetEnvVars() map[string]string {
	result := make(map[string]string)
	maps.Copy(result, rt.envVars)
	return result
}

// GetAliases returns a copy of the collected aliases.
func (rt *Runtime) GetAliases() map[string]string {
	result := make(map[string]string)
	maps.Copy(result, rt.aliases)
	return result
}

// =============================================================================
// Private Methods
// =============================================================================

// initializeEnvPrinters initializes environment printers based on configuration settings.
// It creates and registers the appropriate environment printers with the dependency injector
// based on the current configuration state.
func (rt *Runtime) initializeEnvPrinters() {
	contextValues := map[string]any{}
	if values, err := rt.ConfigHandler.GetContextValues(); err == nil && values != nil {
		contextValues = values
	}

	clusterDriver := getNestedString(contextValues, "cluster", "driver")
	if clusterDriver == "" {
		clusterDriver = rt.ConfigHandler.GetString("cluster.driver", "")
	}
	clusterEnabled := clusterDriver != ""
	needsDocker := rt.needsDockerEnv()
	configData := rt.ConfigHandler.GetHeader()
	platform := rt.ConfigHandler.GetString("platform", "")
	hasAWSConfig := configData != nil && configData.AWS != nil
	hasAzureConfig := configData != nil && configData.Azure != nil
	hasGCPConfig := configData != nil && configData.GCP != nil
	hasVSphereConfig := configData != nil && configData.VSphere != nil

	if rt.EnvPrinters.DotEnvEnv == nil {
		rt.EnvPrinters.DotEnvEnv = env.NewDotEnvEnvPrinter(rt.Shell, rt.ConfigHandler, rt.Evaluator)
	}
	if rt.EnvPrinters.AwsEnv == nil && (hasAWSConfig || platform == "aws") {
		rt.EnvPrinters.AwsEnv = env.NewAwsEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.AzureEnv == nil && (hasAzureConfig || platform == "azure") {
		rt.EnvPrinters.AzureEnv = env.NewAzureEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.GcpEnv == nil && (hasGCPConfig || platform == "gcp") {
		rt.EnvPrinters.GcpEnv = env.NewGcpEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.VsphereEnv == nil && (hasVSphereConfig || platform == "vsphere") {
		rt.EnvPrinters.VsphereEnv = env.NewVsphereEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.DockerEnv == nil && needsDocker {
		rt.EnvPrinters.DockerEnv = env.NewVirtEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.KubeEnv == nil && clusterEnabled {
		rt.EnvPrinters.KubeEnv = env.NewKubeEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.TalosEnv == nil && clusterDriver == "talos" {
		rt.EnvPrinters.TalosEnv = env.NewTalosEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.TerraformEnv == nil && rt.ConfigHandler.GetBool("terraform.enabled", true) {
		rt.initializeToolsManager()
		if rt.TerraformProvider == nil {
			rt.TerraformProvider = terraform.NewTerraformProvider(rt.ConfigHandler, rt.Shell, rt.ToolsManager, rt.Evaluator)
		}
		rt.EnvPrinters.TerraformEnv = env.NewTerraformEnvPrinter(rt.Shell, rt.ConfigHandler, rt.ToolsManager, rt.TerraformProvider)
	}
	if rt.EnvPrinters.WindsorEnv == nil {
		allEnvPrinters := rt.getAllEnvPrinters()
		rt.EnvPrinters.WindsorEnv = env.NewWindsorEnvPrinter(rt.Shell, rt.ConfigHandler, rt.Evaluator, allEnvPrinters)
	}
}

// needsDockerEnv returns true when VirtEnvPrinter (DOCKER_HOST, DOCKER_CONFIG, etc.) should be used:
// platform must be "docker" and workstation.runtime must be colima, docker-desktop, or docker.
func (rt *Runtime) needsDockerEnv() bool {
	platform := rt.ConfigHandler.GetString("platform")
	if platform != "docker" {
		return false
	}
	wsRuntime := rt.ConfigHandler.GetString("workstation.runtime")
	switch wsRuntime {
	case "colima", "docker-desktop", "docker":
		return true
	default:
		return false
	}
}

// getAllEnvPrinters returns all environment printers in a consistent order.
// This ensures that environment variables are processed in a predictable sequence,
// with DotEnvEnv processed first (lowest precedence, an operator-supplied base layer)
// and WindsorEnv processed last to take precedence.
func (rt *Runtime) getAllEnvPrinters() []env.EnvPrinter {
	return []env.EnvPrinter{
		rt.EnvPrinters.DotEnvEnv,
		rt.EnvPrinters.AwsEnv,
		rt.EnvPrinters.AzureEnv,
		rt.EnvPrinters.GcpEnv,
		rt.EnvPrinters.VsphereEnv,
		rt.EnvPrinters.DockerEnv,
		rt.EnvPrinters.KubeEnv,
		rt.EnvPrinters.TalosEnv,
		rt.EnvPrinters.TerraformEnv,
		rt.EnvPrinters.WindsorEnv,
	}
}

// getNestedString returns the string at path in values, or an empty string if the path does not lead to a string.
func getNestedString(values map[string]any, path ...string) string {
	current := any(values)
	for _, key := range path {
		next, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		val, exists := next[key]
		if !exists {
			return ""
		}
		current = val
	}
	str, _ := current.(string)
	return str
}
