package tools

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// The ToolsManager is a core component that manages development tools and dependencies
// required for infrastructure and application development. It handles the lifecycle of
// development tools through a manifest-based approach, ensuring consistent tooling
// across development environments. The manager facilitates tool version management,
// installation verification, and dependency resolution. It integrates with the project's
// configuration system to determine required tools and their versions, enabling
// reproducible development environments. The manager supports both local and remote
// tool installations, with built-in version checking and compatibility validation.

type ToolsManager interface {
	WriteManifest() error
	Install() error
	Check() error
	GetTerraformCommand() string
}

// BaseToolsManager is the base implementation of the ToolsManager interface.
type BaseToolsManager struct {
	configHandler config.ConfigHandler
	shell         shell.Shell
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
	}
}

// WriteManifest writes the tools manifest to the project root.
// It should not overwrite existing manifest files, but
// update them appropriately.
func (t *BaseToolsManager) WriteManifest() error {
	// Placeholder
	return nil
}

// Install installs the tools required by the project.
func (t *BaseToolsManager) Install() error {
	// Placeholder
	return nil
}

// Check checks that appropriate tools are installed and configured.
func (t *BaseToolsManager) Check() error {
	message := "🛠️ Checking tool versions"
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()
	defer spin.Stop()

	if t.configHandler.GetBool("docker.enabled") {
		if err := t.checkDocker(); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
			return fmt.Errorf("docker check failed: %v", err)
		}
	}

	if t.configHandler.GetBool("terraform.enabled") {
		if err := t.checkTerraform(); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
			return fmt.Errorf("terraform check failed: %v", err)
		}
	}

	if t.configHandler.GetString("vm.driver") == "colima" {
		if err := t.checkColima(); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
			return fmt.Errorf("colima check failed: %v", err)
		}
	}

	if vaults := t.configHandler.Get("secrets.onepassword.vaults"); vaults != nil {
		if err := t.checkOnePassword(); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
			return fmt.Errorf("1password check failed: %v", err)
		}
	}

	if t.configHandler.GetBool("secrets.sops.enabled", false) {
		if err := t.checkSops(); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
			return fmt.Errorf("sops check failed: %v", err)
		}
	}

	if t.configHandler.GetBool("azure.enabled") {
		if err := t.checkKubelogin(); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
			return fmt.Errorf("kubelogin check failed: %v", err)
		}
	}
	spin.Stop()
	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// checkDocker ensures Docker and Docker Compose are available in the system's PATH using execLookPath and shell.ExecSilent.
// It checks for 'docker', 'docker-compose', 'docker-cli-plugin-docker-compose', or 'docker compose'.
// When in Colima mode, skips daemon-dependent checks since the Docker daemon may not be running yet.
// Returns nil if any are found, else an error indicating Docker Compose is not available in the PATH.
func (t *BaseToolsManager) checkDocker() error {
	if _, err := execLookPath("docker"); err != nil {
		return fmt.Errorf("docker is not available in the PATH")
	}

	isColimaMode := t.configHandler.GetString("vm.driver") == "colima"

	if !isColimaMode {
		output, err := t.shell.ExecSilentWithTimeout("docker", []string{"--version"}, 5*time.Second)
		if err != nil {
			return fmt.Errorf("docker version check failed: %v", err)
		}
		dockerVersion := extractVersion(output)
		if dockerVersion != "" && compareVersion(dockerVersion, constants.MinimumVersionDocker) < 0 {
			return fmt.Errorf("docker version %s is below the minimum required version %s", dockerVersion, constants.MinimumVersionDocker)
		}
	}

	var dockerComposeVersion string

	if !isColimaMode {
		output, err := t.shell.ExecSilentWithTimeout("docker", []string{"compose", "version", "--short"}, 5*time.Second)
		if err == nil {
			dockerComposeVersion = extractVersion(output)
		}
		if dockerComposeVersion == "" {
			// Docker 27+ removed --short; fall back to full version output.
			output, err := t.shell.ExecSilentWithTimeout("docker", []string{"compose", "version"}, 5*time.Second)
			if err == nil {
				dockerComposeVersion = extractVersion(output)
			}
		}
	}

	if dockerComposeVersion == "" {
		if _, err := execLookPath("docker-compose"); err == nil {
			output, err := t.shell.ExecSilentWithTimeout("docker-compose", []string{"version", "--short"}, 5*time.Second)
			if err == nil {
				dockerComposeVersion = extractVersion(output)
			}
			if dockerComposeVersion == "" {
				output, err := t.shell.ExecSilentWithTimeout("docker-compose", []string{"version"}, 5*time.Second)
				if err == nil {
					dockerComposeVersion = extractVersion(output)
				}
			}
		}
	}

	if dockerComposeVersion != "" {
		if compareVersion(dockerComposeVersion, constants.MinimumVersionDockerCompose) >= 0 {
			return nil
		}
		return fmt.Errorf("docker-compose version %s is below the minimum required version %s", dockerComposeVersion, constants.MinimumVersionDockerCompose)
	}

	if _, err := execLookPath("docker-cli-plugin-docker-compose"); err == nil {
		return nil
	}

	return fmt.Errorf("docker-compose is not available in the PATH")
}

// checkColima ensures Colima and Limactl are available in the system's PATH using execLookPath.
// It checks for 'colima' and 'limactl' in the system's PATH and verifies their versions.
// Returns nil if both are found and meet the minimum version requirements, else an error indicating either is not available or outdated.
func (t *BaseToolsManager) checkColima() error {
	if _, err := execLookPath("colima"); err != nil {
		return fmt.Errorf("colima is not available in the PATH")
	}
	output, err := t.shell.ExecSilentWithTimeout("colima", []string{"version"}, 5*time.Second)
	if err != nil {
		return fmt.Errorf("colima version check failed: %v", err)
	}
	colimaVersion := extractVersion(output)
	if colimaVersion == "" {
		return fmt.Errorf("failed to extract colima version")
	}
	if compareVersion(colimaVersion, constants.MinimumVersionColima) < 0 {
		return fmt.Errorf("colima version %s is below the minimum required version %s", colimaVersion, constants.MinimumVersionColima)
	}

	if _, err := execLookPath("limactl"); err != nil {
		return fmt.Errorf("limactl is not available in the PATH")
	}
	output, err = t.shell.ExecSilentWithTimeout("limactl", []string{"--version"}, 5*time.Second)
	if err != nil {
		return fmt.Errorf("limactl version check failed: %v", err)
	}
	limactlVersion := extractVersion(output)
	if limactlVersion == "" {
		return fmt.Errorf("failed to extract limactl version")
	}
	if compareVersion(limactlVersion, constants.MinimumVersionLima) < 0 {
		return fmt.Errorf("limactl version %s is below the minimum required version %s", limactlVersion, constants.MinimumVersionLima)
	}

	return nil
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
	if _, err := execLookPath("terraform"); err == nil {
		return "terraform"
	}
	if _, err := execLookPath("tofu"); err == nil {
		return "opentofu"
	}
	return "terraform"
}

// checkTerraform ensures Terraform or OpenTofu is available in the system's PATH using execLookPath.
// It checks for the configured driver command in the system's PATH and verifies its version.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkTerraform() error {
	command := t.GetTerraformCommand()
	if _, err := execLookPath(command); err != nil {
		return fmt.Errorf("%s is not available in the PATH", command)
	}
	output, err := t.shell.ExecSilentWithTimeout(command, []string{"version"}, 5*time.Second)
	if err != nil {
		return fmt.Errorf("%s version check failed: %v", command, err)
	}
	terraformVersion := extractVersion(output)
	if terraformVersion == "" {
		return fmt.Errorf("failed to extract %s version", command)
	}
	if compareVersion(terraformVersion, constants.MinimumVersionTerraform) < 0 {
		return fmt.Errorf("%s version %s is below the minimum required version %s", command, terraformVersion, constants.MinimumVersionTerraform)
	}

	return nil
}

// checkOnePassword ensures 1Password CLI is available in the system's PATH using execLookPath.
// It checks for 'op' in the system's PATH and verifies its version.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkOnePassword() error {
	if _, err := execLookPath("op"); err != nil {
		return fmt.Errorf("1Password CLI is not available in the PATH")
	}

	out, err := t.shell.ExecSilentWithTimeout("op", []string{"--version"}, 5*time.Second)
	if err != nil {
		return fmt.Errorf("1Password CLI is not available in the PATH: %v", err)
	}

	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract 1Password CLI version")
	}

	if compareVersion(version, constants.MinimumVersion1Password) < 0 {
		return fmt.Errorf("1Password CLI version %s is below the minimum required version %s", version, constants.MinimumVersion1Password)
	}

	return nil
}

// checkSops ensures SOPS CLI is available in the system's PATH using execLookPath.
// It checks for 'sops' in the system's PATH and verifies its version.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkSops() error {
	if _, err := execLookPath("sops"); err != nil {
		return fmt.Errorf("SOPS CLI is not available in the PATH")
	}

	out, err := t.shell.ExecSilentWithTimeout("sops", []string{"--version"}, 5*time.Second)
	if err != nil {
		return fmt.Errorf("SOPS CLI is not available in the PATH: %v", err)
	}

	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract SOPS CLI version")
	}

	if compareVersion(version, constants.MinimumVersionSOPS) < 0 {
		return fmt.Errorf("SOPS CLI version %s is below the minimum required version %s", version, constants.MinimumVersionSOPS)
	}

	return nil
}

// checkKubelogin ensures kubelogin is available in the system's PATH using execLookPath.
// It checks for 'kubelogin' in the system's PATH, verifies its version, and validates
// required environment variables for SPN authentication if AZURE_CLIENT_SECRET is set.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkKubelogin() error {
	if _, err := execLookPath("kubelogin"); err != nil {
		return fmt.Errorf("kubelogin is not available in the PATH")
	}

	out, err := t.shell.ExecSilentWithTimeout("kubelogin", []string{"--version"}, 5*time.Second)
	if err != nil {
		return fmt.Errorf("kubelogin is not available in the PATH: %v", err)
	}

	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract kubelogin version")
	}

	if compareVersion(version, constants.MinimumVersionKubelogin) < 0 {
		return fmt.Errorf("kubelogin version %s is below the minimum required version %s", version, constants.MinimumVersionKubelogin)
	}

	validationRules := []struct {
		triggerVar string
		authMethod string
	}{
		{"AZURE_FEDERATED_TOKEN_FILE", "Workload Identity"},
		{"AZURE_CLIENT_SECRET", "SPN"},
	}

	for _, rule := range validationRules {
		if os.Getenv(rule.triggerVar) != "" {
			azureClientID := os.Getenv("AZURE_CLIENT_ID")
			azureTenantID := os.Getenv("AZURE_TENANT_ID")

			if azureClientID == "" {
				return fmt.Errorf("%s is set but AZURE_CLIENT_ID is missing - both are required for %s authentication", rule.triggerVar, rule.authMethod)
			}
			if azureTenantID == "" {
				return fmt.Errorf("%s is set but AZURE_TENANT_ID is missing - both are required for %s authentication", rule.triggerVar, rule.authMethod)
			}
		}
	}

	return nil
}

// compareVersion is a helper function to compare two version strings.
// It returns -1 if version1 < version2, 0 if version1 == version2, and 1 if version1 > version2.
func compareVersion(version1, version2 string) int {
	// Split version into main and pre-release parts
	splitVersion := func(version string) (main, preRelease string) {
		parts := strings.SplitN(version, "-", 2)
		main = parts[0]
		if len(parts) > 1 {
			preRelease = parts[1]
		}
		return
	}

	main1, pre1 := splitVersion(version1)
	main2, pre2 := splitVersion(version2)

	v1 := strings.Split(main1, ".")
	v2 := strings.Split(main2, ".")
	length := len(v1)
	length = max(length, len(v2))

	for i := range make([]int, length) {
		var comp1, comp2 int

		if i < len(v1) {
			comp1, _ = strconv.Atoi(v1[i])
		}
		if i < len(v2) {
			comp2, _ = strconv.Atoi(v2[i])
		}

		if comp1 < comp2 {
			return -1
		}
		if comp1 > comp2 {
			return 1
		}
	}

	// Handle trailing zeros by comparing the length of the version components
	if len(v1) < len(v2) {
		return -1
	}
	if len(v1) > len(v2) {
		return 1
	}

	// Compare pre-release parts
	if pre1 == "" && pre2 == "" {
		return 0
	}
	if pre1 == "" {
		return 1
	}
	if pre2 == "" {
		return -1
	}
	return strings.Compare(pre1, pre2)
}

// extractVersion uses a regex to extract the first version component from a string.
func extractVersion(output string) string {
	re := regexp.MustCompile(`\d+\.\d+\.\d+`)
	match := re.FindString(output)
	return match
}
