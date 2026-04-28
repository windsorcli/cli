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
	CheckRequirements(reqs Requirements) error
	CheckAuth() error
	GetTerraformCommand() string
}

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

// BaseToolsManager is the base implementation of the ToolsManager interface.
type BaseToolsManager struct {
	configHandler config.ConfigHandler
	shell         shell.Shell
}

// =============================================================================
// Constants
// =============================================================================

// awsProfile* values classify what the context-scoped .aws/config has for a profile.
// awsProfileNone covers missing file, unreadable file, or profile not present.
const (
	awsProfileNone awsProfileState = iota
	awsProfileSSO
	awsProfileKeys
)

// =============================================================================
// Types
// =============================================================================

// awsProfileState classifies an .aws/config profile entry so CheckAuth can tailor its hint.
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

// Check verifies every tool that may be required for any windsor codepath. Equivalent to
// CheckRequirements(AllRequirements()) and exists for backward compatibility with the
// explicit "check everything" command (`windsor check`) and any caller that has not been
// migrated to declare per-command requirements.
func (t *BaseToolsManager) Check() error {
	return t.CheckRequirements(AllRequirements())
}

// CheckRequirements runs only the checks the caller has opted into via reqs, AND that the
// configHandler still gates on. The two-layer test (command-level need × project-level
// config) means an over-broad request from the command side is harmless — `windsor up` can
// safely request Terraform=true even on a context where terraform.enabled is false, because
// the config gate skips the actual binary check. Under-requesting is the failure mode worth
// guarding against: a missed Requirements field lets a stale tool reach the actual run.
//
// AWS tool verification lives on CheckAuth, not here. CheckRequirements fires from many
// command paths including `windsor init` / `windsor env` — at those points the operator has
// no obligation to have the aws CLI installed OR to be authed. Both cloud-CLI presence and
// credential resolution belong to CheckAuth, which runs from bootstrap and from
// `windsor check`.
func (t *BaseToolsManager) CheckRequirements(reqs Requirements) error {
	rt := t.configHandler.GetString("workstation.runtime")
	dockerEnabled := t.configHandler.GetBool("docker.enabled", false)
	needsDocker := dockerEnabled || rt == "colima" || rt == "docker-desktop" || rt == "docker"
	if reqs.Docker && needsDocker {
		if err := t.checkDocker(); err != nil {
			return err
		}
	}

	if reqs.Terraform && t.configHandler.GetBool("terraform.enabled") {
		if err := t.checkTerraform(); err != nil {
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

	if reqs.Kubelogin && t.configHandler.GetBool("azure.enabled") {
		if err := t.checkKubelogin(); err != nil {
			return err
		}
	}
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// checkDocker ensures Docker is available in the system's PATH. When not in Colima mode it also
// verifies the docker CLI version meets the minimum. Docker Compose is not required.
func (t *BaseToolsManager) checkDocker() error {
	if _, err := execLookPath("docker"); err != nil {
		return missingToolError("docker")
	}

	workstationRuntime := t.configHandler.GetString("workstation.runtime")
	isColimaMode := workstationRuntime == "colima"

	if !isColimaMode {
		output, err := t.shell.ExecSilentWithTimeout("docker", []string{"--version"}, 5*time.Second)
		if err != nil {
			return fmt.Errorf("docker --version failed: %v", err)
		}
		dockerVersion := extractVersion(output)
		if dockerVersion != "" && compareVersion(dockerVersion, constants.MinimumVersionDocker) < 0 {
			return outdatedToolError("docker", dockerVersion)
		}
	}

	return nil
}

// checkColima ensures Colima and Limactl are available in the system's PATH using execLookPath.
// It checks for 'colima' and 'limactl' in the system's PATH and verifies their versions.
// Returns nil if both are found and meet the minimum version requirements, else an error indicating either is not available or outdated.
func (t *BaseToolsManager) checkColima() error {
	if _, err := execLookPath("colima"); err != nil {
		return missingToolError("colima")
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
		return outdatedToolError("colima", colimaVersion)
	}

	if _, err := execLookPath("limactl"); err != nil {
		return missingToolError("limactl")
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
		return outdatedToolError("limactl", limactlVersion)
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
		return missingToolError(command)
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
		return outdatedToolError(command, terraformVersion)
	}

	return nil
}

// checkOnePassword ensures 1Password CLI is available in the system's PATH using execLookPath.
// It checks for 'op' in the system's PATH and verifies its version.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkOnePassword() error {
	if _, err := execLookPath("op"); err != nil {
		return missingToolError("op")
	}

	out, err := t.shell.ExecSilentWithTimeout("op", []string{"--version"}, 5*time.Second)
	if err != nil {
		return fmt.Errorf("1Password CLI --version failed: %v", err)
	}

	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract 1Password CLI version")
	}

	if compareVersion(version, constants.MinimumVersion1Password) < 0 {
		return outdatedToolError("op", version)
	}

	return nil
}

// checkSops ensures SOPS CLI is available in the system's PATH using execLookPath.
// It checks for 'sops' in the system's PATH and verifies its version.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkSops() error {
	if _, err := execLookPath("sops"); err != nil {
		return missingToolError("sops")
	}

	out, err := t.shell.ExecSilentWithTimeout("sops", []string{"--version"}, 5*time.Second)
	if err != nil {
		return fmt.Errorf("sops --version failed: %v", err)
	}

	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract SOPS CLI version")
	}

	if compareVersion(version, constants.MinimumVersionSOPS) < 0 {
		return outdatedToolError("sops", version)
	}

	return nil
}

// checkKubelogin ensures kubelogin is available in the system's PATH using execLookPath.
// It checks for 'kubelogin' in the system's PATH, verifies its version, and validates
// required environment variables for SPN authentication if AZURE_CLIENT_SECRET is set.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkKubelogin() error {
	if _, err := execLookPath("kubelogin"); err != nil {
		return missingToolError("kubelogin")
	}

	out, err := t.shell.ExecSilentWithTimeout("kubelogin", []string{"--version"}, 5*time.Second)
	if err != nil {
		return fmt.Errorf("kubelogin --version failed: %v", err)
	}

	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract kubelogin version")
	}

	if compareVersion(version, constants.MinimumVersionKubelogin) < 0 {
		return outdatedToolError("kubelogin", version)
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
	return nil
}

// awsEnabled reports whether the current context exercises AWS — platform/provider is "aws"
// or an aws config block is present.
func (t *BaseToolsManager) awsEnabled() bool {
	platform := t.configHandler.GetString("platform")
	if platform == "" {
		platform = t.configHandler.GetString("provider")
	}
	if platform == "aws" {
		return true
	}
	cfg := t.configHandler.GetConfig()
	return cfg != nil && cfg.AWS != nil
}

// checkAWSAuth verifies the AWS CLI is present at the minimum version and that
// `aws sts get-caller-identity` resolves credentials end-to-end. When ambient SDK
// credentials are present and the aws CLI is absent (lean CI images), defers to terraform's
// own SDK rather than failing preflight.
//
// The underlying STS error is intentionally discarded in favour of awsAuthHint(): the
// raw output stacks "command execution failed" + the aws CLI's own stderr + our hint,
// which buries the one piece of information the operator needs (the next command to run).
// Trade-off: a network outage or IAM denial gets reported as a generic "run aws sso login"
// hint rather than the true cause. If a debug path is needed later, route the err to a
// verbose-only log rather than re-introducing it into the user-facing message.
func (t *BaseToolsManager) checkAWSAuth() error {
	if hasAmbientAWSCredentials() {
		if _, err := execLookPath("aws"); err != nil {
			return nil
		}
	}
	if err := t.checkAWSBinary(); err != nil {
		return err
	}
	env, err := t.awsContextEnv()
	if err != nil {
		return fmt.Errorf("cannot resolve context-scoped AWS env for credential check: %w", err)
	}
	if _, err := t.shell.ExecSilentWithEnvAndTimeout("aws", env, []string{"sts", "get-caller-identity"}, 10*time.Second); err != nil {
		return fmt.Errorf("%s", t.awsAuthHint())
	}
	return nil
}

// awsContextEnv returns env vars pointing the AWS CLI/SDK at the context-scoped .aws/ dir
// and selecting the right profile. Returns (nil, nil) when ambient SDK credentials are
// present — overriding AWS_PROFILE there would mask the native credential chain.
func (t *BaseToolsManager) awsContextEnv() (map[string]string, error) {
	if hasAmbientAWSCredentials() {
		return nil, nil
	}
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

// hasAmbientAWSCredentials reports whether the parent env already carries AWS credentials
// via a native SDK mechanism (IRSA web identity, ECS container creds, or static keys) that
// must not be overridden by context-scoped profile/config vars. IMDS is not covered — there
// is no env var to detect it.
func hasAmbientAWSCredentials() bool {
	if os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != "" {
		return true
	}
	if os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" {
		return true
	}
	if os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "" {
		return true
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
		return true
	}
	return false
}

// checkAWSBinary verifies the AWS CLI is available in PATH and meets the minimum version.
func (t *BaseToolsManager) checkAWSBinary() error {
	if _, err := execLookPath("aws"); err != nil {
		return missingToolError("aws")
	}

	out, err := t.shell.ExecSilentWithTimeout("aws", []string{"--version"}, 10*time.Second)
	if err != nil {
		return fmt.Errorf("aws --version failed: %v", err)
	}
	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract aws CLI version")
	}
	if compareVersion(version, constants.MinimumVersionAWS) < 0 {
		return outdatedToolError("aws", version)
	}

	return nil
}

// awsAuthHint returns an actionable next-step message tailored to the context's AWS config
// state (expired SSO, rejected keys, or no profile yet). When the process env doesn't already
// point at the context's .aws/, the suggested command is prefixed with that env so credentials
// land in the right place. The first-time-setup branch surfaces both SSO and access-key paths
// because we can't tell which kind of operator reached it.
func (t *BaseToolsManager) awsAuthHint() string {
	ctx := t.configHandler.GetContext()
	profile := ctx
	cfg := t.configHandler.GetConfig()
	if cfg != nil && cfg.AWS != nil && cfg.AWS.AWSProfile != nil && *cfg.AWS.AWSProfile != "" {
		profile = *cfg.AWS.AWSProfile
	}
	configRoot, err := t.configHandler.GetConfigRoot()
	if err != nil || configRoot == "" {
		return fmt.Sprintf("No AWS credentials configured for context %q yet. Run one of:\n  aws configure sso --profile %s   (SSO)\n  aws configure --profile %s       (access keys)", ctx, profile, profile)
	}
	awsConfigPath := filepath.Join(configRoot, ".aws", "config")
	awsCredentialsPath := filepath.Join(configRoot, ".aws", "credentials")
	prefix := ""
	if !awsEnvPointsAtContext(awsConfigPath, awsCredentialsPath) {
		prefix = awsEnvPrefix(configRoot)
	}
	state := detectAWSProfileState(awsConfigPath, profile)
	switch state {
	case awsProfileSSO:
		return fmt.Sprintf("AWS SSO session for %q has likely expired. Run:\n  %saws sso login --profile %s", profile, prefix, profile)
	case awsProfileKeys:
		return fmt.Sprintf("AWS access keys for %q were rejected by STS. Verify or rotate with:\n  %saws configure --profile %s", profile, prefix, profile)
	default:
		return fmt.Sprintf("No AWS credentials configured for context %q yet. Run one of:\n  %saws configure sso --profile %s   (SSO)\n  %saws configure --profile %s       (access keys)", ctx, prefix, profile, prefix, profile)
	}
}

// awsEnvPointsAtContext reports whether AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE both
// resolve to the given context paths. A partial or mismatched set returns false so callers
// don't suggest bare `aws ...` commands that would write credentials to the wrong place.
func awsEnvPointsAtContext(configPath, credentialsPath string) bool {
	cf := os.Getenv("AWS_CONFIG_FILE")
	sf := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if cf == "" || sf == "" {
		return false
	}
	return filepath.ToSlash(cf) == filepath.ToSlash(configPath) &&
		filepath.ToSlash(sf) == filepath.ToSlash(credentialsPath)
}

// awsEnvPrefix returns the `KEY="VALUE" KEY="VALUE" ` prefix that points an aws CLI
// invocation at the context-scoped .aws/ directory. Paths are quoted to survive spaces.
func awsEnvPrefix(configRoot string) string {
	awsConfigDir := filepath.Join(configRoot, ".aws")
	return fmt.Sprintf("AWS_CONFIG_FILE=%q AWS_SHARED_CREDENTIALS_FILE=%q ",
		filepath.ToSlash(filepath.Join(awsConfigDir, "config")),
		filepath.ToSlash(filepath.Join(awsConfigDir, "credentials")),
	)
}

// azureEnabled mirrors awsEnabled — true when platform=azure or an azure block is present.
// Note: azure.enabled is not consulted here; that flag gates kubelogin, not auth preflight.
func (t *BaseToolsManager) azureEnabled() bool {
	platform := t.configHandler.GetString("platform")
	if platform == "" {
		platform = t.configHandler.GetString("provider")
	}
	if platform == "azure" {
		return true
	}
	cfg := t.configHandler.GetConfig()
	return cfg != nil && cfg.Azure != nil
}

// checkAzureAuth runs `az account show` against the context-scoped AZURE_CONFIG_DIR.
// Skips entirely under ambient creds (defers to the azurerm SDK) — unlike `aws sts`,
// `az account show` only reads its local token cache and would false-positive against
// a CI host that has SDK env vars set but never ran `az login`.
// The az error is discarded in favour of azureAuthHint to keep the actionable line visible.
func (t *BaseToolsManager) checkAzureAuth() error {
	if hasAmbientAzureCredentials() {
		return nil
	}
	if err := t.checkAzureBinary(); err != nil {
		return err
	}
	env, err := t.azureContextEnv()
	if err != nil {
		return fmt.Errorf("cannot resolve context-scoped Azure env for credential check: %w", err)
	}
	if _, err := t.shell.ExecSilentWithEnvAndTimeout("az", env, []string{"account", "show"}, 10*time.Second); err != nil {
		return fmt.Errorf("%s", t.azureAuthHint())
	}
	return nil
}

// azureContextEnv returns env pointing az at the context's .azure/ dir.
func (t *BaseToolsManager) azureContextEnv() (map[string]string, error) {
	configRoot, err := t.configHandler.GetConfigRoot()
	if err != nil {
		return nil, err
	}
	azureConfigDir := filepath.Join(configRoot, ".azure")
	return map[string]string{
		"AZURE_CONFIG_DIR":               filepath.ToSlash(azureConfigDir),
		"AZURE_CORE_LOGIN_EXPERIENCE_V2": "false",
	}, nil
}

// hasAmbientAzureCredentials detects Workload Identity or SPN secret env. IMDS
// (Managed Identity) is not covered — no env signal, same blind spot as AWS IMDS.
func hasAmbientAzureCredentials() bool {
	clientID := os.Getenv("AZURE_CLIENT_ID")
	tenantID := os.Getenv("AZURE_TENANT_ID")
	if clientID == "" || tenantID == "" {
		return false
	}
	if os.Getenv("AZURE_FEDERATED_TOKEN_FILE") != "" {
		return true
	}
	if os.Getenv("AZURE_CLIENT_SECRET") != "" {
		return true
	}
	return false
}

// checkAzureBinary verifies the az CLI is available in PATH and meets the minimum version.
func (t *BaseToolsManager) checkAzureBinary() error {
	if _, err := execLookPath("az"); err != nil {
		return missingToolError("az")
	}

	out, err := t.shell.ExecSilentWithTimeout("az", []string{"--version"}, 10*time.Second)
	if err != nil {
		return fmt.Errorf("az --version failed: %v", err)
	}
	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract az CLI version")
	}
	if compareVersion(version, constants.MinimumVersionAzure) < 0 {
		return outdatedToolError("az", version)
	}

	return nil
}

// azureAuthHint returns an actionable `az login` command, pinned to tenant_id when set,
// chained with `az account set` when subscription_id is set, prefixed with AZURE_CONFIG_DIR=
// when the process env doesn't already point at the context.
func (t *BaseToolsManager) azureAuthHint() string {
	ctx := t.configHandler.GetContext()
	tenantID := ""
	subscriptionID := ""
	cfg := t.configHandler.GetConfig()
	if cfg != nil && cfg.Azure != nil {
		if cfg.Azure.TenantID != nil {
			tenantID = *cfg.Azure.TenantID
		}
		if cfg.Azure.SubscriptionID != nil {
			subscriptionID = *cfg.Azure.SubscriptionID
		}
	}

	loginCmd := "az login"
	if tenantID != "" {
		loginCmd = fmt.Sprintf("az login --tenant %s", tenantID)
	}

	prefix := ""
	if configRoot, err := t.configHandler.GetConfigRoot(); err == nil && configRoot != "" {
		if !azureEnvPointsAtContext(filepath.Join(configRoot, ".azure")) {
			prefix = azureEnvPrefix(configRoot)
		}
	}

	suffix := ""
	if subscriptionID != "" {
		suffix = fmt.Sprintf(" && %saz account set --subscription %s", prefix, subscriptionID)
	}

	return fmt.Sprintf("Azure credentials for context %q did not resolve. Run:\n  %s%s%s", ctx, prefix, loginCmd, suffix)
}

// azureEnvPointsAtContext reports whether AZURE_CONFIG_DIR resolves to the given path.
func azureEnvPointsAtContext(azureConfigDir string) bool {
	cd := os.Getenv("AZURE_CONFIG_DIR")
	if cd == "" {
		return false
	}
	return filepath.ToSlash(cd) == filepath.ToSlash(azureConfigDir)
}

// azureEnvPrefix returns the `AZURE_CONFIG_DIR="..." ` prefix for paste-safe hints.
func azureEnvPrefix(configRoot string) string {
	azureConfigDir := filepath.Join(configRoot, ".azure")
	return fmt.Sprintf("AZURE_CONFIG_DIR=%q ", filepath.ToSlash(azureConfigDir))
}

// detectAWSProfileState classifies the named profile in the AWS config INI at path. Missing
// file, parse errors, or an empty matching section resolve to awsProfileNone.
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
