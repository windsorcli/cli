// The ToolsManager is the preflight check for the external tools that Windsor commands run.
// It provides the tool checks that a command requests, the cloud credential checks, and the terraform driver.
// Each check runs only when the context config uses the tool, so a broad request is safe.
// A failed check returns an error that names the tool, its minimum version, and its install page.

package tools

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Types
// =============================================================================

// Requirements is the set of tool families a single command's codepath will actually exercise.
// Each field is opt-in: a command requests only what it will run, and the corresponding check
// still respects the configHandler gates (e.g. Docker=true only triggers a docker check when
// docker.enabled is set or workstation.runtime is a docker-family driver). Over-requesting is
// safe — checks no-op when the underlying config gate is off; under-requesting is the bug to
// avoid because it lets a stale tool slip through to the actual command.
type Requirements struct {
	Docker    bool
	Colima    bool
	Terraform bool
	Secrets   bool
	Kubelogin bool
}

// BaseToolsManager is the base implementation of the ToolsManager interface.
type BaseToolsManager struct {
	configHandler config.ConfigHandler
	shell         shell.Shell
	shims         *Shims
}

// =============================================================================
// Interfaces
// =============================================================================

// ToolsManager checks that the tools and the cloud credentials of a Windsor command are ready.
type ToolsManager interface {
	Install() error
	Check() error
	CheckRequirements(reqs Requirements) error
	CheckAuth() error
	CheckTerraform() error
	CheckFlux() error
	CheckFluxOperator() error
	GetTerraformCommand() string
}

// =============================================================================
// Constructor
// =============================================================================

// NewToolsManager creates a new ToolsManager instance with the given config handler and shell.
func NewToolsManager(configHandler config.ConfigHandler, shell shell.Shell) *BaseToolsManager {
	if configHandler == nil {
		panic("config handler is required")
	}
	if shell == nil {
		panic("shell is required")
	}
	return &BaseToolsManager{
		configHandler: configHandler,
		shell:         shell,
		shims:         NewShims(),
	}
}

// AllRequirements returns a Requirements with every field true. Used by `windsor check` (the
// explicit "verify my whole setup" command) and as the default for code paths that must
// preserve historical Check() behavior.
func AllRequirements() Requirements {
	return Requirements{
		Docker:    true,
		Colima:    true,
		Terraform: true,
		Secrets:   true,
		Kubelogin: true,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Install is a placeholder for the tool installation. It does nothing and returns nil.
func (t *BaseToolsManager) Install() error {
	return nil
}

// Check verifies every tool that may be required for any windsor codepath. Equivalent to
// CheckRequirements(AllRequirements()) and exists for backward compatibility with the
// explicit "check everything" command (`windsor check`) and any caller that has not been
// migrated to declare per-command requirements.
func (t *BaseToolsManager) Check() error {
	return t.CheckRequirements(AllRequirements())
}

// CheckRequirements runs each check that reqs requests and that the config also enables.
// A request that is too broad is harmless, because the config gate skips the binary check.
// A request that is too narrow is the risk, because an outdated tool can then reach the run.
// CheckAuth, not this method, checks the AWS CLI and the credentials.
// Commands such as windsor init and windsor env do not need the AWS CLI.
func (t *BaseToolsManager) CheckRequirements(reqs Requirements) error {
	rt := t.configHandler.GetString("workstation.runtime")
	dockerEnabled := t.configHandler.GetBool("docker.enabled", false)
	needsDocker := dockerEnabled || rt == "colima" || rt == "docker-desktop" || rt == "docker"
	if reqs.Docker && needsDocker {
		if err := t.checkDocker(); err != nil {
			return err
		}
		if t.configHandler.GetString("cluster.driver", "") == "talos" {
			if err := checkCPUMicroarchitecture(); err != nil {
				return err
			}
		}
	}

	if reqs.Terraform && t.configHandler.GetBool("terraform.enabled") {
		if err := t.CheckTerraform(); err != nil {
			return err
		}
	}

	if reqs.Colima && rt == "colima" {
		if err := t.checkColima(); err != nil {
			return err
		}
	}

	if reqs.Secrets {
		if vaults := t.configHandler.Get("secrets.onepassword.vaults"); vaults != nil {
			if err := t.checkOnePassword(); err != nil {
				return err
			}
		}
		if t.configHandler.GetBool("secrets.sops.enabled", false) {
			if err := t.checkSops(); err != nil {
				return err
			}
		}
	}

	if reqs.Kubelogin && t.azureEnabled() {
		if err := t.checkKubelogin(); err != nil {
			return err
		}
	}
	return nil
}

// CheckAuth verifies the operator has the cloud CLIs for in-use platforms and that their
// credentials resolve. Called from terraform-touching command paths (bootstrap/up/apply/
// plan/destroy) and `windsor check` — never from `init` or `env`.
func (t *BaseToolsManager) CheckAuth() error {
	if t.awsEnabled() {
		if err := t.checkAWSAuth(); err != nil {
			return err
		}
	}
	if t.azureEnabled() {
		if err := t.checkAzureAuth(); err != nil {
			return err
		}
	}
	if t.gcpEnabled() {
		if err := t.checkGCPAuth(); err != nil {
			return err
		}
	}
	return nil
}

// CheckTerraform checks that the configured Terraform or OpenTofu command is on PATH.
// It also checks that the version meets the minimum.
// It is exported for callers that need Terraform when terraform.enabled is false,
// such as the module resolver of the composer, which runs terraform init.
func (t *BaseToolsManager) CheckTerraform() error {
	command := t.GetTerraformCommand()
	if _, err := t.shims.LookPath(command); err != nil {
		return missingToolError(command)
	}
	output, err := t.shell.ExecSilentWithTimeout(command, []string{"version"}, 5*time.Second)
	if err != nil {
		return werror.Wrap(err, "%s version check failed", command)
	}
	terraformVersion := extractVersion(output)
	if terraformVersion == "" {
		return fmt.Errorf("failed to extract %s version", command)
	}
	if compareVersion(terraformVersion, constants.MinimumVersionTerraform) < 0 {
		return outdatedToolError(command, terraformVersion)
	}

	return nil
}

// CheckFlux checks that the flux CLI is on PATH and meets the minimum version.
// Commands that render or diff Flux Kustomizations call it before they run flux.
func (t *BaseToolsManager) CheckFlux() error {
	return t.checkToolVersion("flux", "version", "--client")
}

// CheckFluxOperator checks that the flux-operator CLI is on PATH and meets the minimum version.
// The minimum version is the first release with the build instance command.
func (t *BaseToolsManager) CheckFluxOperator() error {
	return t.checkToolVersion("flux-operator", "--version")
}

// GetTerraformCommand returns the terraform command to use (terraform or tofu) based on configuration.
// Defaults to "terraform" if not specified in the root-level terraform config.
// Accepts case-insensitive driver values and "tofu" as an alias for "opentofu".
func (t *BaseToolsManager) GetTerraformCommand() string {
	if t.configHandler == nil {
		return "terraform"
	}
	driver := t.getTerraformDriver()
	driverLower := strings.ToLower(driver)
	if driverLower == "opentofu" || driverLower == "tofu" {
		return "tofu"
	}
	return "terraform"
}

// =============================================================================
// Private Methods
// =============================================================================

// checkToolVersion looks up command on PATH, runs it with versionArgs, and compares the first
// dotted version in the output with the minimum version in the tool registry.
func (t *BaseToolsManager) checkToolVersion(command string, versionArgs ...string) error {
	if _, err := t.shims.LookPath(command); err != nil {
		return missingToolError(command)
	}
	out, err := t.shell.ExecSilentWithTimeout(command, versionArgs, 5*time.Second)
	if err != nil {
		return werror.Wrap(err, "%s version check failed", command)
	}
	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract %s version from output %q", command, out)
	}
	if compareVersion(version, toolRegistry[command].minVersion) < 0 {
		return outdatedToolError(command, version)
	}
	return nil
}

// getTerraformDriver returns the terraform driver from root config, or detects which CLI is available.
// If not specified in config, it checks for "terraform" first, then "tofu", defaulting to "terraform" if neither is found.
func (t *BaseToolsManager) getTerraformDriver() string {
	if t.shell == nil {
		return t.detectTerraformDriver()
	}
	projectRoot, err := t.shell.GetProjectRoot()
	if err != nil {
		return t.detectTerraformDriver()
	}
	root, err := os.OpenRoot(projectRoot)
	if err != nil {
		return t.detectTerraformDriver()
	}
	defer root.Close()
	if _, err := root.Stat("windsor.yaml"); err != nil {
		return t.detectTerraformDriver()
	}
	fileData, err := root.ReadFile("windsor.yaml")
	if err != nil {
		return t.detectTerraformDriver()
	}
	var rootConfig struct {
		Terraform struct {
			Driver string `yaml:"driver,omitempty"`
		} `yaml:"terraform,omitempty"`
	}
	if err := yaml.Unmarshal(fileData, &rootConfig); err != nil {
		return t.detectTerraformDriver()
	}
	if rootConfig.Terraform.Driver != "" {
		return rootConfig.Terraform.Driver
	}
	return t.detectTerraformDriver()
}

// detectTerraformDriver detects which terraform CLI is available in the system PATH.
// Checks for "terraform" first, then "tofu", defaulting to "terraform" if neither is found.
func (t *BaseToolsManager) detectTerraformDriver() string {
	if _, err := t.shims.LookPath("terraform"); err == nil {
		return "terraform"
	}
	if _, err := t.shims.LookPath("tofu"); err == nil {
		return "opentofu"
	}
	return "terraform"
}
