// The AWS auth check is the BaseToolsManager check for the AWS CLI and the AWS credentials.
// It provides the aws binary check, the sts get-caller-identity call, and the login hint.
// It runs the aws CLI with the context AWS config when the env does not already point at it.
// With ambient SDK credentials and no aws CLI, it leaves the credential check to terraform.

package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime/internal/awsprofile"
)

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
// Private Methods
// =============================================================================

// awsEnabled reports whether the current context exercises AWS — platform is "aws"
// or an aws config block is present.
func (t *BaseToolsManager) awsEnabled() bool {
	platform := t.configHandler.GetString("platform")
	if platform == "aws" {
		return true
	}
	cfg := t.configHandler.GetHeader()
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
		if _, err := t.shims.LookPath("aws"); err != nil {
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

// awsContextEnv returns env vars pointing the AWS CLI/SDK at the context's profile.
// In project mode AWS_CONFIG_FILE / AWS_SHARED_CREDENTIALS_FILE always point at the
// context's .aws/ — they keep `aws configure` scoped to the context and are safe to emit
// even when the files are empty (the SDK simply doesn't find a profile and falls through).
// AWS_PROFILE is emitted only when the named profile is actually defined in the AWS config
// the SDK will read: the context's .aws/ in project mode, or the operator's ambient
// ~/.aws/ (or AWS_CONFIG_FILE override) in global mode. When the profile is absent the
// var is omitted so the SDK's credential chain runs naturally (env keys, IMDS, IRSA, ECS,
// SSO) rather than failing with "profile not found" against a file the named profile
// was never in.
func (t *BaseToolsManager) awsContextEnv() (map[string]string, error) {
	env := map[string]string{}
	global := t.shell.IsGlobal()
	var configRoot string
	if !global {
		root, err := t.configHandler.GetConfigRoot()
		if err != nil {
			return nil, err
		}
		configRoot = root
		awsConfigDir := filepath.Join(configRoot, ".aws")
		env["AWS_CONFIG_FILE"] = filepath.ToSlash(filepath.Join(awsConfigDir, "config"))
		env["AWS_SHARED_CREDENTIALS_FILE"] = filepath.ToSlash(filepath.Join(awsConfigDir, "credentials"))
	}

	cfg := t.configHandler.GetHeader()
	profileName := ""
	if cfg != nil && cfg.AWS != nil && cfg.AWS.Profile != nil {
		profileName = *cfg.AWS.Profile
	}
	if profileName == "" {
		profileName = t.configHandler.GetContext()
	}
	if profileName != "" {
		resolver := awsprofile.ForContext(configRoot)
		if global {
			resolver = awsprofile.Ambient()
		}
		if resolver.HasProfile(profileName) {
			env["AWS_PROFILE"] = profileName
		}
	}

	if len(env) == 0 {
		return nil, nil
	}
	return env, nil
}

// checkAWSBinary verifies the AWS CLI is available in PATH and meets the minimum version.
func (t *BaseToolsManager) checkAWSBinary() error {
	if _, err := t.shims.LookPath("aws"); err != nil {
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
// state (expired SSO, rejected keys, or no profile yet). In project mode windsor scopes
// AWS config to the context's .aws/, so when the process env doesn't already point there
// the suggested command is prefixed with that env so credentials land in the right place.
// In global mode windsor defers to the operator's ambient ~/.aws/ (or AWS_CONFIG_FILE
// override), so no env prefix is suggested and profile-state detection reads from the
// ambient config. The first-time-setup branch surfaces both SSO and access-key paths
// because we can't tell which kind of operator reached it.
func (t *BaseToolsManager) awsAuthHint() string {
	ctx := t.configHandler.GetContext()
	profile := ctx
	cfg := t.configHandler.GetHeader()
	if cfg != nil && cfg.AWS != nil && cfg.AWS.Profile != nil && *cfg.AWS.Profile != "" {
		profile = *cfg.AWS.Profile
	}
	if t.shell.IsGlobal() {
		state := t.detectAWSProfileState(ambientAWSConfigPath(), profile)
		switch state {
		case awsProfileSSO:
			return fmt.Sprintf("AWS SSO session for %q has likely expired. Run:\n  aws sso login --profile %s", profile, profile)
		case awsProfileKeys:
			return fmt.Sprintf("AWS access keys for %q were rejected by STS. Verify or rotate with:\n  aws configure --profile %s", profile, profile)
		default:
			return fmt.Sprintf("No AWS credentials configured for profile %q yet. Run one of:\n  aws configure sso --profile %s   (SSO)\n  aws configure --profile %s       (access keys)", profile, profile, profile)
		}
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
	state := t.detectAWSProfileState(awsConfigPath, profile)
	switch state {
	case awsProfileSSO:
		return fmt.Sprintf("AWS SSO session for %q has likely expired. Run:\n  %saws sso login --profile %s", profile, prefix, profile)
	case awsProfileKeys:
		return fmt.Sprintf("AWS access keys for %q were rejected by STS. Verify or rotate with:\n  %saws configure --profile %s", profile, prefix, profile)
	default:
		return fmt.Sprintf("No AWS credentials configured for context %q yet. Run one of:\n  %saws configure sso --profile %s   (SSO)\n  %saws configure --profile %s       (access keys)", ctx, prefix, profile, prefix, profile)
	}
}

// detectAWSProfileState classifies the named profile in the AWS config INI at path. Missing
// file, parse errors, or an empty matching section resolve to awsProfileNone.
func (t *BaseToolsManager) detectAWSProfileState(path, profileName string) awsProfileState {
	data, err := t.shims.ReadFile(path)
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

// =============================================================================
// Helpers
// =============================================================================

// hasAmbientAWSCredentials reports whether the parent env carries AWS credentials via
// IRSA (token file plus role ARN), ECS container creds, or a complete static keypair.
// checkAWSAuth uses it to decide whether a missing aws CLI should be treated as a no-op
// rather than a hard error — terraform's SDK will exercise the native chain at apply
// time. The IRSA check requires both AWS_WEB_IDENTITY_TOKEN_FILE and AWS_ROLE_ARN because
// the SDK's web identity provider needs both halves to assume the role.
func hasAmbientAWSCredentials() bool {
	if os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != "" && os.Getenv("AWS_ROLE_ARN") != "" {
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

// ambientAWSConfigPath returns the path the AWS CLI would read for profile config when
// no windsor-scoped overrides are in play: AWS_CONFIG_FILE if set, otherwise
// $HOME/.aws/config. Used by the global-mode auth hint to inspect the user's existing
// profile state. Returns "" if the home directory cannot be resolved and AWS_CONFIG_FILE
// is unset, in which case profile-state detection falls through to the first-time-setup
// branch — the right outcome when we genuinely can't tell what's there.
func ambientAWSConfigPath() string {
	if cf := os.Getenv("AWS_CONFIG_FILE"); cf != "" {
		return cf
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".aws", "config")
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
