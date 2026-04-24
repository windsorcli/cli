package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

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
	CheckAuth() error
	GetTerraformCommand() string
}

// BaseToolsManager is the base implementation of the ToolsManager interface.
type BaseToolsManager struct {
	configHandler config.ConfigHandler
	shell         shell.Shell
}

// =============================================================================
// Constants
// =============================================================================

// awsProfileNone, awsProfileSSO, and awsProfileKeys describe what (if anything) the
// context-scoped .aws/config has for a given profile. awsProfileNone covers "file missing,
// unreadable, or profile not present"; callers treat it as first-time setup.
const (
	awsProfileNone awsProfileState = iota
	awsProfileSSO
	awsProfileKeys
)

// =============================================================================
// Types
// =============================================================================

// awsProfileState classifies an entry in a context's .aws/config so CheckAuth can return a
// hint tuned to the operator's actual state rather than generic advice.
type awsProfileState int

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

// Check verifies required tools are installed.
func (t *BaseToolsManager) Check() error {
	rt := t.configHandler.GetString("workstation.runtime")
	dockerEnabled := t.configHandler.GetBool("docker.enabled", false)
	needsDocker := dockerEnabled || rt == "colima" || rt == "docker-desktop" || rt == "docker"
	if needsDocker {
		if err := t.checkDocker(); err != nil {
			return fmt.Errorf("docker check failed: %v", err)
		}
	}

	if t.configHandler.GetBool("terraform.enabled") {
		if err := t.checkTerraform(); err != nil {
			return fmt.Errorf("terraform check failed: %v", err)
		}
	}

	if rt == "colima" {
		if err := t.checkColima(); err != nil {
			return fmt.Errorf("colima check failed: %v", err)
		}
	}

	if vaults := t.configHandler.Get("secrets.onepassword.vaults"); vaults != nil {
		if err := t.checkOnePassword(); err != nil {
			return fmt.Errorf("1password check failed: %v", err)
		}
	}

	if t.configHandler.GetBool("secrets.sops.enabled", false) {
		if err := t.checkSops(); err != nil {
			return fmt.Errorf("sops check failed: %v", err)
		}
	}

	if t.configHandler.GetBool("azure.enabled") {
		if err := t.checkKubelogin(); err != nil {
			return fmt.Errorf("kubelogin check failed: %v", err)
		}
	}
	// AWS tool verification lives on CheckAuth, not here. Check() fires from every command
	// path including `windsor init` / `windsor env` — at those points the operator has no
	// obligation to have the aws CLI installed OR to be authed. Both cloud-CLI presence and
	// credential resolution belong to CheckAuth, which runs only from bootstrap/up/apply and
	// from `windsor check`.
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// checkDocker ensures Docker is available in the system's PATH. When not in Colima mode it also
// verifies the docker CLI version meets the minimum. Docker Compose is not required.
func (t *BaseToolsManager) checkDocker() error {
	if _, err := execLookPath("docker"); err != nil {
		return fmt.Errorf("docker is not available in the PATH")
	}

	workstationRuntime := t.configHandler.GetString("workstation.runtime")
	isColimaMode := workstationRuntime == "colima"

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

	return nil
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
	minLima := constants.MinimumVersionLima
	if t.configHandler.GetString("platform") == "incus" {
		minLima = constants.MinimumVersionLimaIncus
	}
	if compareVersion(limactlVersion, minLima) < 0 {
		return fmt.Errorf("limactl version %s is below the minimum required version %s (use same PATH as in core repo or upgrade limactl)", limactlVersion, minLima)
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

// CheckAuth verifies that the operator has the cloud CLIs for any in-use platforms AND that
// their credentials actually resolve. For AWS it runs `aws --version` (presence + minimum
// version) and then `aws sts get-caller-identity` — a cheap read-only STS API that forces the
// SDK to run its full credential-resolution chain (SSO session, static keys, env vars, IMDS)
// end-to-end. A success proves the operator's AWS setup will work when terraform/SDK calls
// run later; a failure surfaces the vendor's own message (expired SSO, profile not found, no
// credentials) at preflight time rather than minutes into a `terraform apply`. CheckAuth must
// NOT be called from `windsor init` or `windsor env` (which have no obligation to be authed
// yet); it is intended for bootstrap/up/apply paths where credentials are about to be
// exercised, and from `windsor check` where the operator is explicitly asking to verify.
// Error messages are tuned to the operator's current state (missing config, expired SSO
// session, rejected static keys) so the returned hint is a copy-pasteable command rather than
// generic advice.
func (t *BaseToolsManager) CheckAuth() error {
	platform := t.configHandler.GetString("platform")
	if platform == "" {
		platform = t.configHandler.GetString("provider")
	}
	configData := t.configHandler.GetConfig()
	hasAWSConfig := configData != nil && configData.AWS != nil
	if platform == "aws" || hasAWSConfig {
		if err := t.checkAWSBinary(); err != nil {
			return err
		}
		// Inject the context-scoped AWS env (AWS_CONFIG_FILE, AWS_SHARED_CREDENTIALS_FILE,
		// AWS_PROFILE) so the credential check succeeds even when the operator hasn't sourced
		// `windsor env` / installed the windsor shell hook yet — without this, bootstrap on a
		// fresh machine deadlocks: it can't proceed without valid credentials, but the
		// operator can't establish credentials (`aws sso login`, `aws configure`) without the
		// same env pointing aws at the context's .aws/config. When configRoot can't be
		// resolved the error is surfaced rather than swallowed — letting sts fall back to the
		// ambient shell env would silently validate the wrong credentials and produce the
		// exact false-positive CheckAuth exists to prevent.
		env, err := t.awsContextEnv()
		if err != nil {
			return fmt.Errorf("cannot resolve context-scoped AWS env for credential check: %w", err)
		}
		if _, err := t.shell.ExecSilentWithEnvAndTimeout("aws", env, []string{"sts", "get-caller-identity"}, 10*time.Second); err != nil {
			return fmt.Errorf("aws credentials did not resolve for context %q: %v\n%s", t.configHandler.GetContext(), err, t.awsAuthHint())
		}
	}
	return nil
}

// awsContextEnv returns the env vars that point the AWS CLI / SDK at the context-scoped
// .aws/ directory and select the right profile. Mirrors AwsEnvPrinter.GetEnvVars but is
// duplicated here intentionally — pkg/runtime/tools doesn't depend on pkg/runtime/env, and
// the shape is small. Returns (nil, err) if the configRoot can't be resolved; callers may
// pass nil straight through to the shell, which then falls back to the inherited env.
func (t *BaseToolsManager) awsContextEnv() (map[string]string, error) {
	configRoot, err := t.configHandler.GetConfigRoot()
	if err != nil {
		return nil, err
	}
	awsConfigDir := filepath.Join(configRoot, ".aws")
	env := map[string]string{
		"AWS_CONFIG_FILE":             filepath.ToSlash(filepath.Join(awsConfigDir, "config")),
		"AWS_SHARED_CREDENTIALS_FILE": filepath.ToSlash(filepath.Join(awsConfigDir, "credentials")),
	}
	cfg := t.configHandler.GetConfig()
	if cfg != nil && cfg.AWS != nil && cfg.AWS.AWSProfile != nil && *cfg.AWS.AWSProfile != "" {
		env["AWS_PROFILE"] = *cfg.AWS.AWSProfile
	} else if ctx := t.configHandler.GetContext(); ctx != "" {
		env["AWS_PROFILE"] = ctx
	}
	return env, nil
}

// checkAWSBinary verifies the AWS CLI is available in PATH and meets the minimum version.
// Pulled out of CheckAuth so the binary vs. credential failure modes each have a clean
// single-error exit point.
func (t *BaseToolsManager) checkAWSBinary() error {
	if _, err := execLookPath("aws"); err != nil {
		return fmt.Errorf("aws CLI is not available in the PATH; install it from https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html")
	}

	out, err := t.shell.ExecSilentWithTimeout("aws", []string{"--version"}, 5*time.Second)
	if err != nil {
		return fmt.Errorf("aws --version failed: %v", err)
	}
	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract aws CLI version")
	}
	if compareVersion(version, constants.MinimumVersionAWS) < 0 {
		return fmt.Errorf("aws CLI version %s is below the minimum required version %s", version, constants.MinimumVersionAWS)
	}

	return nil
}

// awsAuthHint inspects the context-scoped AWS config file and returns an actionable next-step
// message tuned to what it finds there. The three useful states are: no profile configured yet
// (first-time setup — offer both SSO and access-key commands), an SSO profile whose token has
// expired (point at `aws sso login`), and a static-keys profile that was rejected (point at
// rotation). The lookup uses the operator's effective profile name — the value of aws.profile
// when set, falling back to the context name — so a context like `prod` configured with
// `aws.profile: company-prod` searches for `[profile company-prod]` and emits commands with
// `--profile company-prod` rather than misleadingly suggesting `--profile prod`. Each
// suggestion is prefixed with the context's AWS_CONFIG_FILE / AWS_SHARED_CREDENTIALS_FILE so
// the command works in any shell — including one where `windsor env` hasn't been sourced —
// and the resulting profile/keys land in the context folder rather than ~/.aws. Anything
// unparseable or missing falls back to a generic hint so the error is still useful even when
// the file is in an unexpected shape.
func (t *BaseToolsManager) awsAuthHint() string {
	ctx := t.configHandler.GetContext()
	profile := ctx
	cfg := t.configHandler.GetConfig()
	if cfg != nil && cfg.AWS != nil && cfg.AWS.AWSProfile != nil && *cfg.AWS.AWSProfile != "" {
		profile = *cfg.AWS.AWSProfile
	}
	configRoot, err := t.configHandler.GetConfigRoot()
	if err != nil || configRoot == "" {
		return fmt.Sprintf("Run 'aws configure sso --profile %s' (SSO) or 'aws configure --profile %s' (access keys) to set up credentials.", profile, profile)
	}
	awsConfigPath := filepath.Join(configRoot, ".aws", "config")
	envPrefix := awsEnvPrefix(configRoot)
	state := detectAWSProfileState(awsConfigPath, profile)
	switch state {
	case awsProfileSSO:
		return fmt.Sprintf("AWS SSO session for %q has likely expired. Run:\n  %saws sso login --profile %s", profile, envPrefix, profile)
	case awsProfileKeys:
		return fmt.Sprintf("AWS access keys for %q were rejected by STS. Verify or rotate with:\n  %saws configure --profile %s", profile, envPrefix, profile)
	default:
		return fmt.Sprintf("No AWS credentials configured for context %q yet. Run one of:\n  %saws configure sso --profile %s   (SSO — recommended for teams)\n  %saws configure --profile %s       (access keys)", ctx, envPrefix, profile, envPrefix, profile)
	}
}

// awsEnvPrefix returns the `KEY=VALUE KEY=VALUE ` prefix that, when prepended to an aws CLI
// invocation, makes that invocation read from and write to the context-scoped .aws/ directory.
// Pulled out of awsAuthHint so all three hint branches (sso, keys, none) share one source of
// truth for the env-prefix shape.
func awsEnvPrefix(configRoot string) string {
	awsConfigDir := filepath.Join(configRoot, ".aws")
	return fmt.Sprintf("AWS_CONFIG_FILE=%s AWS_SHARED_CREDENTIALS_FILE=%s ",
		filepath.ToSlash(filepath.Join(awsConfigDir, "config")),
		filepath.ToSlash(filepath.Join(awsConfigDir, "credentials")),
	)
}

// detectAWSProfileState parses the AWS config INI at path and classifies the profile named
// profileName. It looks for either [profile <name>] or [default] (when profileName is
// "default"), then scans subsequent lines until the next section header for sso_session= or
// aws_access_key_id=, returning the appropriate state. Absent file, parse errors, or an empty
// matching section all resolve to awsProfileNone so the caller gets the first-time-setup hint
// rather than a misleading refresh hint.
func detectAWSProfileState(path, profileName string) awsProfileState {
	data, err := osReadFile(path)
	if err != nil {
		return awsProfileNone
	}
	var wantedHeader string
	if profileName == "default" {
		wantedHeader = "[default]"
	} else {
		wantedHeader = "[profile " + profileName + "]"
	}
	inSection := false
	for rawLine := range strings.SplitSeq(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = line == wantedHeader
			continue
		}
		if !inSection {
			continue
		}
		key := strings.TrimSpace(strings.SplitN(line, "=", 2)[0])
		switch key {
		case "sso_session", "sso_start_url", "sso_account_id":
			return awsProfileSSO
		case "aws_access_key_id":
			return awsProfileKeys
		}
	}
	return awsProfileNone
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
