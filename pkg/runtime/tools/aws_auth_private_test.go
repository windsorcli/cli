package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime/config"
	sh "github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestToolsManager_checkAWSBinary(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given aws is available and meets the minimum version
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "aws" {
				return "/usr/bin/aws", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "aws" && len(args) == 1 && args[0] == "--version" {
				return fmt.Sprintf("aws-cli/%s Python/3.11.8 Darwin/24.1.0", constants.MinimumVersionAWS), nil
			}
			return "", fmt.Errorf("command not mocked: %s %v", command, args)
		}
		// When checking the aws binary
		err := toolsManager.checkAWSBinary()
		// Then no error should be returned. checkAWSBinary must NOT invoke sts — that lives
		// in CheckAuth, which is called only from bootstrap/up/apply preflights and from
		// `windsor check`.
		if err != nil {
			t.Errorf("Expected checkAWSBinary to succeed, got %v", err)
		}
	})

	t.Run("AwsNotAvailable", func(t *testing.T) {
		// Given aws is not in PATH
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "aws" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		// When checking aws
		err := toolsManager.checkAWSBinary()
		// Then error mentions not on PATH and points to vendor install URL
		if err == nil {
			t.Fatal("Expected error when aws is not in PATH")
		}
		if !strings.Contains(err.Error(), "AWS CLI") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected 'AWS CLI ... not found on PATH' in error, got: %v", err)
		}
		if !strings.Contains(errorText(err), "https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html") {
			t.Errorf("Expected vendor install URL in error, got: %v", err)
		}
	})

	t.Run("VersionCommandFails", func(t *testing.T) {
		// Given aws is in PATH but `aws --version` errors
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "aws" {
				return "/usr/bin/aws", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "aws" && len(args) == 1 && args[0] == "--version" {
				return "", fmt.Errorf("permission denied")
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When checking aws
		err := toolsManager.checkAWSBinary()
		// Then the version-command error surfaces
		if err == nil || !strings.Contains(err.Error(), "aws --version failed") {
			t.Errorf("Expected 'aws --version failed' error, got: %v", err)
		}
	})

	t.Run("VersionUnparseable", func(t *testing.T) {
		// Given aws returns output without a parseable semver
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "aws" {
				return "/usr/bin/aws", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "aws" && len(args) == 1 && args[0] == "--version" {
				return "aws-cli banana", nil
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When checking aws
		err := toolsManager.checkAWSBinary()
		// Then the extraction-failure error surfaces
		if err == nil || !strings.Contains(err.Error(), "failed to extract aws CLI version") {
			t.Errorf("Expected 'failed to extract aws CLI version' error, got: %v", err)
		}
	})

	t.Run("VersionTooLow", func(t *testing.T) {
		// Given aws reports a version below the minimum
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "aws" {
				return "/usr/bin/aws", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "aws" && len(args) == 1 && args[0] == "--version" {
				return "aws-cli/1.2.3 Python/3.11.8 Darwin/24.1.0", nil
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When checking aws
		err := toolsManager.checkAWSBinary()
		// Then the version-too-low error surfaces
		if err == nil || !strings.Contains(err.Error(), "below the minimum required version") {
			t.Errorf("Expected version-too-low error, got: %v", err)
		}
	})

}

func Test_awsContextEnv(t *testing.T) {
	t.Run("GlobalModeWithoutContextOrProfileReturnsNil", func(t *testing.T) {
		// Given a tools manager in global mode with no aws.profile override AND no context
		// name. This degenerate state is only reachable through a configHandler that
		// returns "" from GetContext — the production handler always falls back to "local",
		// so this path cannot be hit in production. The test pins the contract: when there
		// is nothing to override, return (nil, nil) so the caller passes no env to STS
		// and STS resolves against the ambient environment.
		mockConfig := &config.MockConfigHandler{
			GetContextFunc: func() string { return "" },
		}
		mockShell := sh.NewMockShell()
		mockShell.IsGlobalFunc = func() bool { return true }
		toolsManager := NewToolsManager(mockConfig, mockShell)

		// When awsContextEnv is invoked
		env, err := toolsManager.awsContextEnv()

		// Then (nil, nil) is returned — STS gets no override
		if err != nil {
			t.Fatalf("awsContextEnv returned error: %v", err)
		}
		if env != nil {
			t.Errorf("expected nil env when no profile and no context in global mode, got %v", env)
		}
	})

	t.Run("GlobalModeFallsBackToContextNameWhenNoProfile", func(t *testing.T) {
		// Given a tools manager in global mode with no aws.profile override but a
		// non-empty context name, and the operator's ambient ~/.aws/config defines
		// [profile local] (the typical local-dev case where the operator has
		// authenticated against the matching profile already).
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config")
		credentialsPath := filepath.Join(dir, "credentials")
		if err := os.WriteFile(configPath, []byte("[profile local]\nregion = us-west-2\n"), 0644); err != nil {
			t.Fatalf("write ambient config: %v", err)
		}
		if err := os.WriteFile(credentialsPath, nil, 0644); err != nil {
			t.Fatalf("write ambient credentials: %v", err)
		}
		t.Setenv("AWS_CONFIG_FILE", configPath)
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)
		mockConfig := &config.MockConfigHandler{
			GetContextFunc: func() string { return "local" },
		}
		mockShell := sh.NewMockShell()
		mockShell.IsGlobalFunc = func() bool { return true }
		toolsManager := NewToolsManager(mockConfig, mockShell)

		// When awsContextEnv is invoked
		env, err := toolsManager.awsContextEnv()

		// Then AWS_PROFILE flows from the context name (matched against the
		// ambient profile) and config-file paths are omitted (global mode
		// defers to the operator's ambient locations)
		if err != nil {
			t.Fatalf("awsContextEnv returned error: %v", err)
		}
		if got := env["AWS_PROFILE"]; got != "local" {
			t.Errorf("AWS_PROFILE = %q, want %q", got, "local")
		}
		if _, ok := env["AWS_CONFIG_FILE"]; ok {
			t.Errorf("AWS_CONFIG_FILE should not be set in global mode, got %q", env["AWS_CONFIG_FILE"])
		}
		if _, ok := env["AWS_SHARED_CREDENTIALS_FILE"]; ok {
			t.Errorf("AWS_SHARED_CREDENTIALS_FILE should not be set in global mode, got %q", env["AWS_SHARED_CREDENTIALS_FILE"])
		}
	})

	t.Run("GlobalModeOmitsAWSProfileWhenAmbientLacksProfile", func(t *testing.T) {
		// Given a tools manager in global mode on a CI runner: OIDC env-var
		// credentials are exported in the parent env but there is no
		// ~/.aws/config at all. Emitting AWS_PROFILE=<context> here would
		// pin the aws CLI at a profile that does not exist, fail with
		// "config profile (X) could not be found", and mask the working
		// ambient credentials behind a misleading auth error.
		dir := t.TempDir()
		t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "missing-config"))
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "missing-credentials"))
		mockConfig := &config.MockConfigHandler{
			GetContextFunc: func() string { return "aws" },
		}
		mockShell := sh.NewMockShell()
		mockShell.IsGlobalFunc = func() bool { return true }
		toolsManager := NewToolsManager(mockConfig, mockShell)

		// When awsContextEnv is invoked
		env, err := toolsManager.awsContextEnv()

		// Then (nil, nil) is returned — STS gets no override and the SDK's
		// credential chain runs naturally against the env-var keys / IMDS
		if err != nil {
			t.Fatalf("awsContextEnv returned error: %v", err)
		}
		if env != nil {
			t.Errorf("expected nil env when ambient config lacks the named profile, got %v", env)
		}
	})

	t.Run("ProjectModeEmitsAWSProfileWhenContextHasProfile", func(t *testing.T) {
		// Given a project-mode context whose .aws/config defines [profile local]
		root := t.TempDir()
		awsDir := filepath.Join(root, ".aws")
		if err := os.MkdirAll(awsDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(awsDir, "config"), []byte("[profile local]\nregion = us-west-2\n"), 0644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		mockConfig := &config.MockConfigHandler{
			GetContextFunc:    func() string { return "local" },
			GetConfigRootFunc: func() (string, error) { return root, nil },
		}
		mockShell := sh.NewMockShell()
		mockShell.IsGlobalFunc = func() bool { return false }
		toolsManager := NewToolsManager(mockConfig, mockShell)

		// When awsContextEnv is invoked
		env, err := toolsManager.awsContextEnv()

		// Then all three vars flow: file paths for write redirection + AWS_PROFILE
		// because contextHasAWSProfile found the [profile local] section
		if err != nil {
			t.Fatalf("awsContextEnv returned error: %v", err)
		}
		if got := env["AWS_PROFILE"]; got != "local" {
			t.Errorf("AWS_PROFILE = %q, want %q", got, "local")
		}
		if got := env["AWS_CONFIG_FILE"]; got != filepath.ToSlash(filepath.Join(awsDir, "config")) {
			t.Errorf("AWS_CONFIG_FILE = %q, unexpected", got)
		}
		if got := env["AWS_SHARED_CREDENTIALS_FILE"]; got != filepath.ToSlash(filepath.Join(awsDir, "credentials")) {
			t.Errorf("AWS_SHARED_CREDENTIALS_FILE = %q, unexpected", got)
		}
	})

	t.Run("ProjectModeOmitsAWSProfileWhenContextHasNoProfile", func(t *testing.T) {
		// Given a project-mode context with no .aws/ files written yet — the
		// operator has not run `aws configure` here. Path X must still emit the
		// file paths (write redirection) but suppress AWS_PROFILE so the SDK's
		// chain can run against env keys / IMDS / IRSA / ECS without tripping
		// "profile not found" on the empty context.
		root := t.TempDir()
		mockConfig := &config.MockConfigHandler{
			GetContextFunc:    func() string { return "local" },
			GetConfigRootFunc: func() (string, error) { return root, nil },
		}
		mockShell := sh.NewMockShell()
		mockShell.IsGlobalFunc = func() bool { return false }
		toolsManager := NewToolsManager(mockConfig, mockShell)

		// When awsContextEnv is invoked
		env, err := toolsManager.awsContextEnv()

		// Then file paths flow but AWS_PROFILE is absent
		if err != nil {
			t.Fatalf("awsContextEnv returned error: %v", err)
		}
		if _, ok := env["AWS_PROFILE"]; ok {
			t.Errorf("AWS_PROFILE should be absent when context has no profile, got %q", env["AWS_PROFILE"])
		}
		if _, ok := env["AWS_CONFIG_FILE"]; !ok {
			t.Errorf("AWS_CONFIG_FILE should still flow for write redirection")
		}
		if _, ok := env["AWS_SHARED_CREDENTIALS_FILE"]; !ok {
			t.Errorf("AWS_SHARED_CREDENTIALS_FILE should still flow for write redirection")
		}
	})
}

func Test_awsAuthHint(t *testing.T) {
	t.Run("GlobalModeOmitsEnvPrefixAndReadsAmbientConfig", func(t *testing.T) {
		// Given a toolsManager in global mode with an SSO profile in the ambient
		// ~/.aws/config (which detectAWSProfileState reads via ReadFile)
		mocks := setupMocks(t)
		mocks.Shell.IsGlobalFunc = func() bool { return true }
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`
[profile test]
sso_session = company
sso_account_id = 123456789012
`), nil
		}
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		// When awsAuthError runs
		hint := errorText(toolsManager.awsAuthError())
		// Then the hint suggests `aws sso login` against the operator's existing config
		// (no AWS_CONFIG_FILE prefix), because in global mode windsor is deferring to
		// the ambient ~/.aws/ rather than scoping config to a context directory.
		if !strings.Contains(hint, "aws sso login --profile test") {
			t.Errorf("Expected ambient SSO login hint in global mode, got: %q", hint)
		}
		if strings.Contains(hint, "AWS_CONFIG_FILE=") {
			t.Errorf("Expected no AWS_CONFIG_FILE prefix in global mode, got: %q", hint)
		}
	})

	t.Run("ConfigRootFailureOffersBothSSOAndAccessKeys", func(t *testing.T) {
		// Given a toolsManager whose configHandler cannot resolve configRoot — the hint's
		// defensive fallback branch. This is reached when CheckAuth ran through awsContextEnv
		// via ambient credentials (which short-circuits GetConfigRoot), sts still failed, and
		// awsAuthHint is invoked for the first-and-only GetConfigRoot call which then errors.
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root not found")
		}
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		// When awsAuthError runs
		hint := errorText(toolsManager.awsAuthError())
		// Then the fallback surfaces BOTH setup paths — dropping access-key guidance would
		// send CI service accounts and non-SSO operators to a command they cannot complete,
		// contradicting the function header's contract that first-time setup always offers
		// both since nothing in this code path distinguishes which kind of operator is here
		if !strings.Contains(hint, "aws configure sso --profile") {
			t.Errorf("Expected SSO setup hint in configRoot-failure fallback, got: %q", hint)
		}
		if !strings.Contains(hint, "aws configure --profile") {
			t.Errorf("Expected access-key setup hint in configRoot-failure fallback, got: %q", hint)
		}
	})
}

func Test_detectAWSProfileState(t *testing.T) {
	withReadFile := func(t *testing.T, contents string, readErr error) *BaseToolsManager {
		t.Helper()
		shims := NewShims()
		shims.ReadFile = func(name string) ([]byte, error) {
			if readErr != nil {
				return nil, readErr
			}
			return []byte(contents), nil
		}
		return &BaseToolsManager{shims: shims}
	}

	t.Run("NoFileReturnsNone", func(t *testing.T) {
		toolsManager := withReadFile(t, "", os.ErrNotExist)
		if got := toolsManager.detectAWSProfileState("/irrelevant", "prod"); got != awsProfileNone {
			t.Errorf("expected awsProfileNone for missing file, got %v", got)
		}
	})

	t.Run("ProfileWithSsoSessionReturnsSSO", func(t *testing.T) {
		toolsManager := withReadFile(t, `
[profile prod]
sso_session = company
sso_account_id = 123456789012
sso_role_name = Admin
region = us-east-1
`, nil)
		if got := toolsManager.detectAWSProfileState("/irrelevant", "prod"); got != awsProfileSSO {
			t.Errorf("expected awsProfileSSO, got %v", got)
		}
	})

	t.Run("ProfileWithSsoStartUrlAloneReturnsSSO", func(t *testing.T) {
		// Legacy SSO profile form (pre-IAM-Identity-Center-sessions) uses sso_start_url
		// directly on the profile. detectAWSProfileState treats that as SSO too so expired
		// tokens route to the sso-login hint.
		toolsManager := withReadFile(t, `
[profile prod]
sso_start_url = https://example.awsapps.com/start
sso_region = us-east-1
sso_account_id = 123456789012
`, nil)
		if got := toolsManager.detectAWSProfileState("/irrelevant", "prod"); got != awsProfileSSO {
			t.Errorf("expected awsProfileSSO for legacy sso_start_url profile, got %v", got)
		}
	})

	t.Run("ProfileWithAccessKeysReturnsKeys", func(t *testing.T) {
		toolsManager := withReadFile(t, `
[profile prod]
aws_access_key_id = AKIATEST
aws_secret_access_key = secret
region = us-west-2
`, nil)
		if got := toolsManager.detectAWSProfileState("/irrelevant", "prod"); got != awsProfileKeys {
			t.Errorf("expected awsProfileKeys, got %v", got)
		}
	})

	t.Run("MissingProfileReturnsNone", func(t *testing.T) {
		// Config file has sections for other profiles but nothing for `prod`.
		toolsManager := withReadFile(t, `
[profile dev]
aws_access_key_id = AKIADEV
`, nil)
		if got := toolsManager.detectAWSProfileState("/irrelevant", "prod"); got != awsProfileNone {
			t.Errorf("expected awsProfileNone when profile not present, got %v", got)
		}
	})

	t.Run("DefaultProfileUsesDefaultHeader", func(t *testing.T) {
		// [default] uses a different header form than [profile <name>]; detection must
		// route to the bare header when the profile name is literally "default".
		toolsManager := withReadFile(t, `
[default]
aws_access_key_id = AKIADEFAULT
`, nil)
		if got := toolsManager.detectAWSProfileState("/irrelevant", "default"); got != awsProfileKeys {
			t.Errorf("expected awsProfileKeys for [default], got %v", got)
		}
	})

	t.Run("CommentsAndBlankLinesIgnored", func(t *testing.T) {
		toolsManager := withReadFile(t, `
# header comment
[profile prod]

# inline comment
sso_session = company
`, nil)
		if got := toolsManager.detectAWSProfileState("/irrelevant", "prod"); got != awsProfileSSO {
			t.Errorf("expected awsProfileSSO with comments/blanks, got %v", got)
		}
	})
}
