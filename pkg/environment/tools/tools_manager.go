package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	sh "github.com/windsorcli/cli/pkg/shell"
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
	Initialize() error
	WriteManifest() error
	Install() error
	Check() error
}

// BaseToolsManager is the base implementation of the ToolsManager interface.
type BaseToolsManager struct {
	injector      di.Injector
	configHandler config.ConfigHandler
	shell         shell.Shell
}

// =============================================================================
// Constructor
// =============================================================================

// Creates a new ToolsManager instance with the given injector.
func NewToolsManager(injector di.Injector) *BaseToolsManager {
	return &BaseToolsManager{
		injector: injector,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize the tools manager by resolving the config handler and shell.
func (t *BaseToolsManager) Initialize() error {
	configHandler := t.injector.Resolve("configHandler")
	t.configHandler = configHandler.(config.ConfigHandler)

	shell := t.injector.Resolve("shell")
	t.shell = shell.(sh.Shell)

	return nil
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

	if t.configHandler.GetBool("aws.enabled") {
		if err := t.checkAwsCli(); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
			return fmt.Errorf("aws cli check failed: %v", err)
		}
	}

	if t.configHandler.GetBool("cluster.enabled") {
		if err := t.checkKubectl(); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
			return fmt.Errorf("kubectl check failed: %v", err)
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

	spin.Stop()
	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
	return nil
}

// CheckExistingToolsManager identifies the active tools manager, prioritizing aqua.
func CheckExistingToolsManager(projectRoot string) (string, error) {
	aquaPath := filepath.Join(projectRoot, "aqua.yaml")
	if _, err := osStat(aquaPath); err == nil {
		return "aqua", nil
	}

	asdfPath := filepath.Join(projectRoot, ".tool-versions")
	if _, err := osStat(asdfPath); err == nil {
		return "asdf", nil
	}

	if _, err := execLookPath("aqua"); err == nil {
		return "aqua", nil
	}

	if _, err := execLookPath("asdf"); err == nil {
		return "asdf", nil
	}

	return "", nil
}

// =============================================================================
// Private Methods
// =============================================================================

// checkDocker ensures Docker and Docker Compose are available in the system's PATH using execLookPath and shell.ExecSilent.
// It checks for 'docker', 'docker-compose', 'docker-cli-plugin-docker-compose', or 'docker compose'.
// Returns nil if any are found, else an error indicating Docker Compose is not available in the PATH.
func (t *BaseToolsManager) checkDocker() error {
	if _, err := execLookPath("docker"); err != nil {
		return fmt.Errorf("docker is not available in the PATH")
	}

	output, _ := t.shell.ExecSilent("docker", "version", "--format", "{{.Client.Version}}")
	dockerVersion := extractVersion(output)
	if dockerVersion != "" && compareVersion(dockerVersion, constants.MINIMUM_VERSION_DOCKER) < 0 {
		return fmt.Errorf("docker version %s is below the minimum required version %s", dockerVersion, constants.MINIMUM_VERSION_DOCKER)
	}

	var dockerComposeVersion string

	// Try to get docker-compose version using different methods
	output, _ = t.shell.ExecSilent("docker", "compose", "version", "--short")
	dockerComposeVersion = extractVersion(output)

	if dockerComposeVersion == "" {
		if _, err := execLookPath("docker-compose"); err == nil {
			output, _ = t.shell.ExecSilent("docker-compose", "version", "--short")
			dockerComposeVersion = extractVersion(output)
		}
	}

	// Validate the collected docker-compose version
	if dockerComposeVersion != "" {
		if compareVersion(dockerComposeVersion, constants.MINIMUM_VERSION_DOCKER_COMPOSE) >= 0 {
			return nil
		}
		return fmt.Errorf("docker-compose version %s is below the minimum required version %s", dockerComposeVersion, constants.MINIMUM_VERSION_DOCKER_COMPOSE)
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
	output, _ := t.shell.ExecSilent("colima", "version")
	colimaVersion := extractVersion(output)
	if colimaVersion == "" {
		return fmt.Errorf("failed to extract colima version")
	}
	if compareVersion(colimaVersion, constants.MINIMUM_VERSION_COLIMA) < 0 {
		return fmt.Errorf("colima version %s is below the minimum required version %s", colimaVersion, constants.MINIMUM_VERSION_COLIMA)
	}

	if _, err := execLookPath("limactl"); err != nil {
		return fmt.Errorf("limactl is not available in the PATH")
	}
	output, _ = t.shell.ExecSilent("limactl", "--version")
	limactlVersion := extractVersion(output)
	if limactlVersion == "" {
		return fmt.Errorf("failed to extract limactl version")
	}
	if compareVersion(limactlVersion, constants.MINIMUM_VERSION_LIMA) < 0 {
		return fmt.Errorf("limactl version %s is below the minimum required version %s", limactlVersion, constants.MINIMUM_VERSION_LIMA)
	}

	return nil
}

// checkKubectl ensures Kubectl is available in the system's PATH using execLookPath.
// It checks for 'kubectl' in the system's PATH and verifies its version.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkKubectl() error {
	if _, err := execLookPath("kubectl"); err != nil {
		return fmt.Errorf("kubectl is not available in the PATH")
	}
	output, _ := t.shell.ExecSilent("kubectl", "version", "--client")
	kubectlVersion := extractVersion(output)
	if kubectlVersion == "" {
		return fmt.Errorf("failed to extract kubectl version")
	}
	if compareVersion(kubectlVersion, constants.MINIMUM_VERSION_KUBECTL) < 0 {
		return fmt.Errorf("kubectl version %s is below the minimum required version %s", kubectlVersion, constants.MINIMUM_VERSION_KUBECTL)
	}

	return nil
}

// checkTerraform ensures Terraform is available in the system's PATH using execLookPath.
// It checks for 'terraform' in the system's PATH and verifies its version.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkTerraform() error {
	if _, err := execLookPath("terraform"); err != nil {
		return fmt.Errorf("terraform is not available in the PATH")
	}
	output, _ := t.shell.ExecSilent("terraform", "version")
	terraformVersion := extractVersion(output)
	if terraformVersion == "" {
		return fmt.Errorf("failed to extract terraform version")
	}
	if compareVersion(terraformVersion, constants.MINIMUM_VERSION_TERRAFORM) < 0 {
		return fmt.Errorf("terraform version %s is below the minimum required version %s", terraformVersion, constants.MINIMUM_VERSION_TERRAFORM)
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

	out, err := t.shell.ExecSilent("op", "--version")
	if err != nil {
		return fmt.Errorf("1Password CLI is not available in the PATH")
	}

	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract 1Password CLI version")
	}

	if compareVersion(version, constants.MINIMUM_VERSION_1PASSWORD) < 0 {
		return fmt.Errorf("1Password CLI version %s is below the minimum required version %s", version, constants.MINIMUM_VERSION_1PASSWORD)
	}

	return nil
}

// checkAwsCli ensures AWS CLI is available in the system's PATH using execLookPath.
// It checks for 'aws' in the system's PATH and verifies its version.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkAwsCli() error {
	if _, err := execLookPath("aws"); err != nil {
		return fmt.Errorf("aws cli is not available in the PATH")
	}
	output, _ := t.shell.ExecSilent("aws", "--version")
	re := regexp.MustCompile(`aws-cli/(\d+\.\d+\.\d+)`)
	match := re.FindStringSubmatch(output)
	var awsVersion string
	if len(match) > 1 {
		awsVersion = match[1]
	} else {
		awsVersion = extractVersion(output)
	}
	if awsVersion == "" {
		return fmt.Errorf("failed to extract aws cli version")
	}
	if compareVersion(awsVersion, constants.MINIMUM_VERSION_AWS_CLI) < 0 {
		return fmt.Errorf("aws cli version %s is below the minimum required version %s", awsVersion, constants.MINIMUM_VERSION_AWS_CLI)
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
