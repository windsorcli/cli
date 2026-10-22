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
// Test Public Methods
// =============================================================================

func TestToolsManager_CheckAuth(t *testing.T) {
	setup := func(t *testing.T, configStr string) (*Mocks, *BaseToolsManager) {
		t.Helper()
		// Clear every env var hasAmbientAWSCredentials consults so the binary-missing
		// escape hatch in checkAWSAuth behaves identically on a dev laptop, an EKS pod,
		// and a GitHub Actions runner. AWS_ROLE_ARN is included because the IRSA path
		// requires both the token-file and role-ARN env vars to count as ambient.
		// AWS_CONFIG_FILE / AWS_SHARED_CREDENTIALS_FILE redirect ambientAWSProfileExists
		// to a non-existent path so global-mode tests don't accidentally read the
		// developer's real ~/.aws/config — tests opt in to a populated ambient config
		// via the withAmbientProfile helper.
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
		t.Setenv("AWS_ROLE_ARN", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
		t.Setenv("AWS_ACCESS_KEY_ID", "")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "")
		t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "no-ambient-config"))
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "no-ambient-credentials"))
		mocks := setupMocks(t, &SetupOptions{ConfigStr: configStr})
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		return mocks, toolsManager
	}

	t.Run("NoCloudPlatformIsNoOp", func(t *testing.T) {
		// Given a context with no AWS platform or aws block
		_, toolsManager := setup(t, defaultConfig)
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then it returns nil without invoking any cloud-CLI calls
		if err != nil {
			t.Errorf("Expected CheckAuth to be a no-op for non-cloud contexts, got %v", err)
		}
	})

	// awsBinaryMock wires LookPath and the version shell call so CheckAuth can reach the
	// sts-get-caller-identity step. Every sub-test that exercises CheckAuth beyond the binary
	// check uses this to get past the preliminaries.
	awsBinaryMock := func(t *testing.T, mocks *Mocks) {
		t.Helper()
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "aws" {
				return "/usr/bin/aws", nil
			}
			return originalExecLookPath(name)
		}
	}

	t.Run("AWSPlatformWithAwsCliMissing", func(t *testing.T) {
		// Given platform: aws and the aws CLI is not in PATH
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "aws" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}

		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the missing-binary error surfaces with the vendor install URL — the binary
		// check is part of CheckAuth now so bootstrap/up preflights catch a missing CLI
		// before they try to resolve credentials.
		if err == nil {
			t.Fatal("Expected error when aws CLI is missing")
		}
		if !strings.Contains(err.Error(), "AWS CLI") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected 'AWS CLI ... not found on PATH' error, got: %v", err)
		}
	})

	t.Run("AWSPlatformWithValidCredentials", func(t *testing.T) {
		// Given platform: aws, the aws CLI is installed at the minimum version, and sts
		// get-caller-identity succeeds
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t, mocks)
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "aws" && len(args) == 1 && args[0] == "--version" {
				return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
			}
			if command == "aws" && len(args) == 2 && args[0] == "sts" && args[1] == "get-caller-identity" {
				return `{"Arn":"arn:aws:iam::123456789012:user/test"}`, nil
			}
			return "", fmt.Errorf("command not mocked: %s %v", command, args)
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then it succeeds
		if err != nil {
			t.Errorf("Expected CheckAuth to succeed when sts resolves, got %v", err)
		}
	})

	t.Run("AWSPlatformWithStsFailure", func(t *testing.T) {
		// Given platform: aws, the aws CLI is installed, but credentials cannot be resolved
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t, mocks)
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "aws" && len(args) == 1 && args[0] == "--version" {
				return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
			}
			if command == "aws" && len(args) == 2 && args[0] == "sts" && args[1] == "get-caller-identity" {
				return "", fmt.Errorf("Unable to locate credentials")
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then it surfaces ONLY the actionable hint — no doubled "command execution failed"
		// + raw aws-CLI stderr + hint stack. The hint names the actionable next step, which
		// is the only thing the operator needs to see.
		if err == nil {
			t.Fatal("Expected error when sts get-caller-identity fails")
		}
		if !strings.Contains(errorText(err), "aws configure") && !strings.Contains(errorText(err), "aws sso login") {
			t.Errorf("Expected an aws sso/configure remediation hint in error, got: %v", err)
		}
		// Guard against the noise regression: the raw shell-exec wrapper text and aws-CLI
		// stderr must not be present in the surfaced error. If either reappears it means
		// somebody re-introduced the %v err interpolation.
		if strings.Contains(err.Error(), "command execution failed") {
			t.Errorf("Expected raw shell-exec error text to be suppressed, got: %v", err)
		}
		if strings.Contains(err.Error(), "aws credentials did not resolve") {
			t.Errorf("Expected the redundant 'aws credentials did not resolve' prefix to be removed, got: %v", err)
		}
	})

	// withAmbientProfile writes configBody to a tmp file and points
	// AWS_CONFIG_FILE at it so ambientAWSProfileExists has a deterministic
	// ambient config to read. Pass empty configBody to test the "ambient has
	// no profile" case (CI runner with OIDC env keys and no ~/.aws/config).
	withAmbientProfile := func(t *testing.T, configBody string) {
		t.Helper()
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config")
		credentialsPath := filepath.Join(dir, "credentials")
		if err := os.WriteFile(configPath, []byte(configBody), 0644); err != nil {
			t.Fatalf("write ambient config: %v", err)
		}
		if err := os.WriteFile(credentialsPath, nil, 0644); err != nil {
			t.Fatalf("write ambient credentials: %v", err)
		}
		t.Setenv("AWS_CONFIG_FILE", configPath)
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)
	}

	// writeContextProfile seeds the named profile into the handler's reported
	// config root so contextHasAWSProfile returns true. Uses the existing
	// configHandler (which points at the per-test temp dir set up by setupMocks)
	// rather than overriding GetConfigRoot, so the path the env vars get is the
	// same path the test asserts against.
	writeContextProfile := func(t *testing.T, mocks *Mocks, profileName string) string {
		t.Helper()
		root, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("GetConfigRoot: %v", err)
		}
		awsDir := filepath.Join(root, ".aws")
		if err := os.MkdirAll(awsDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		header := "[profile " + profileName + "]"
		if profileName == "default" {
			header = "[default]"
		}
		body := header + "\nregion = us-west-2\n"
		if err := os.WriteFile(filepath.Join(awsDir, "config"), []byte(body), 0644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		return root
	}

	t.Run("AWSPlatformInjectsContextEnvForSts", func(t *testing.T) {
		// Given platform: aws, the aws CLI is installed, and the context's
		// .aws/config already declares the [profile test] section that
		// windsor would pin sts against
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t, mocks)
		root := writeContextProfile(t, mocks, "test")
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "aws" && len(args) == 1 && args[0] == "--version" {
				return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
			}
			return "", fmt.Errorf("command not mocked")
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "aws" && len(args) == 2 && args[0] == "sts" && args[1] == "get-caller-identity" {
				capturedEnv = env
				return `{"Arn":"arn:aws:iam::123456789012:user/test"}`, nil
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed, got %v", err)
		}
		// Then sts received the context-scoped AWS env so it resolves against the context's
		// .aws/config and AWS_PROFILE rather than whatever happens to be active in the parent
		// shell — without this, CheckAuth could green-light bootstrap using stale [default]
		// credentials that terraform apply would later fail with
		wantConfig := filepath.ToSlash(filepath.Join(root, ".aws", "config"))
		wantCreds := filepath.ToSlash(filepath.Join(root, ".aws", "credentials"))
		if capturedEnv["AWS_CONFIG_FILE"] != wantConfig {
			t.Errorf("AWS_CONFIG_FILE = %q, want %q", capturedEnv["AWS_CONFIG_FILE"], wantConfig)
		}
		if capturedEnv["AWS_SHARED_CREDENTIALS_FILE"] != wantCreds {
			t.Errorf("AWS_SHARED_CREDENTIALS_FILE = %q, want %q", capturedEnv["AWS_SHARED_CREDENTIALS_FILE"], wantCreds)
		}
		if capturedEnv["AWS_PROFILE"] != "test" {
			t.Errorf("AWS_PROFILE = %q, want %q", capturedEnv["AWS_PROFILE"], "test")
		}
	})

	t.Run("AWSPlatformWithProfileOverrideUsesAwsProfile", func(t *testing.T) {
		// Given platform: aws, an explicit aws.profile override, and the
		// context's .aws/config declares that override profile
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
    aws:
      profile: company-prod
`)
		awsBinaryMock(t, mocks)
		writeContextProfile(t, mocks, "company-prod")
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "aws" && len(args) == 1 && args[0] == "--version" {
				return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
			}
			return "", fmt.Errorf("command not mocked")
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			capturedEnv = env
			return `{"Arn":"arn:aws:iam::123456789012:user/x"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed, got %v", err)
		}
		// Then AWS_PROFILE comes from aws.profile, not the context name — operators who
		// alias their context to a differently-named upstream profile (e.g. company-prod)
		// must have sts resolve against that profile, not the context label
		if capturedEnv["AWS_PROFILE"] != "company-prod" {
			t.Errorf("AWS_PROFILE = %q, want %q", capturedEnv["AWS_PROFILE"], "company-prod")
		}
	})

	t.Run("AWSPlatformStsHintPrefixesEnvWhenShellNotSourced", func(t *testing.T) {
		// Given platform: aws, the binary check passes, sts fails, the context's .aws/config
		// has an SSO profile entry, and the process env does NOT have AWS_CONFIG_FILE
		// pointing at the context (plain shell — `windsor env` has not been sourced)
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t, mocks)
		t.Setenv("AWS_CONFIG_FILE", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`
[profile test]
sso_session = company
sso_account_id = 123456789012
`), nil
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("Token has expired")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the surfaced error prepends AWS_CONFIG_FILE= / AWS_SHARED_CREDENTIALS_FILE= so
		// the suggested `aws sso login` writes its token into the context path even though
		// the operator's shell has no windsor env loaded — without the prefix the token would
		// land in ~/.aws and the next windsor check would silently re-fail
		if err == nil {
			t.Fatal("Expected CheckAuth to fail when sts errors")
		}
		if !strings.Contains(errorText(err), "AWS_CONFIG_FILE=") {
			t.Errorf("Expected AWS_CONFIG_FILE= prefix when shell env is not sourced, got: %v", err)
		}
		if !strings.Contains(errorText(err), "aws sso login --profile test") {
			t.Errorf("Expected sso-login hint with profile, got: %v", err)
		}
	})

	t.Run("AWSPlatformStsHintDropsEnvPrefixWhenShellIsSourced", func(t *testing.T) {
		// Given platform: aws, the binary check passes, sts fails, the context's .aws/config
		// has an SSO profile entry, and the process env's AWS_CONFIG_FILE /
		// AWS_SHARED_CREDENTIALS_FILE already point at the context's .aws/ paths — the
		// operator has sourced `windsor env` for this context
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t, mocks)
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("GetConfigRoot failed: %v", err)
		}
		t.Setenv("AWS_CONFIG_FILE", filepath.ToSlash(filepath.Join(configRoot, ".aws", "config")))
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.ToSlash(filepath.Join(configRoot, ".aws", "credentials")))
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`
[profile test]
sso_session = company
sso_account_id = 123456789012
`), nil
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("Token has expired")
		}
		// When CheckAuth runs
		err = toolsManager.CheckAuth()
		// Then the hint emits a bare `aws sso login --profile test` with no env prefix —
		// the operator's shell will resolve AWS_CONFIG_FILE from its own env, so prefixing
		// would just be noise in the common case (they're in a windsor-managed shell)
		if err == nil {
			t.Fatal("Expected CheckAuth to fail when sts errors")
		}
		if !strings.Contains(errorText(err), "aws sso login --profile test") {
			t.Errorf("Expected sso-login hint with profile, got: %v", err)
		}
		if strings.Contains(err.Error(), "AWS_CONFIG_FILE=") {
			t.Errorf("Hint should omit AWS_CONFIG_FILE= prefix when shell env already points at context, got: %v", err)
		}
	})

	t.Run("AWSPlatformStsHintFirstTimeOffersSSOAndAccessKeys", func(t *testing.T) {
		// Given platform: aws, the binary check passes, sts fails, and the context's
		// .aws/config is empty — the first-time-setup state where no profile is present
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t, mocks)
		t.Setenv("AWS_CONFIG_FILE", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("Unable to locate credentials")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the surfaced error presents BOTH `aws configure sso` and `aws configure`
		// paths — CI service accounts, accounts not enrolled in SSO, and operators handed
		// programmatic keys need the access-key command, and we cannot infer which path the
		// operator is on at this point in the flow. Dropping either path sends some fraction
		// of operators to a command that will not work for their account type.
		if err == nil {
			t.Fatal("Expected CheckAuth to fail when sts errors")
		}
		if !strings.Contains(errorText(err), "aws configure sso --profile test") {
			t.Errorf("Expected SSO setup hint, got: %v", err)
		}
		if !strings.Contains(errorText(err), "aws configure --profile test") {
			t.Errorf("Expected access-key setup hint, got: %v", err)
		}
	})

	t.Run("AWSPlatformStsHintHonorsAwsProfileOverride", func(t *testing.T) {
		// Given platform: aws, an explicit aws.profile that differs from the context name,
		// and a context-scoped .aws/config containing only the override-named SSO profile
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
    aws:
      profile: company-prod
`)
		awsBinaryMock(t, mocks)
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`
[profile company-prod]
sso_session = company
sso_account_id = 123456789012
`), nil
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("Token has expired")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the hint suggests `aws sso login --profile company-prod` rather than
		// --profile test — operators with aliased upstream profiles get a runnable command,
		// not a misleading first-time-setup hint
		if err == nil {
			t.Fatal("Expected CheckAuth to fail when sts errors")
		}
		if !strings.Contains(errorText(err), "aws sso login --profile company-prod") {
			t.Errorf("Expected sso-login hint to use aws.profile override, got: %v", err)
		}
		if strings.Contains(err.Error(), "--profile test") {
			t.Errorf("Hint should not reference context name when aws.profile is set, got: %v", err)
		}
	})

	t.Run("AWSPlatformStsHintQuotesPathsWithSpacesInPlainShell", func(t *testing.T) {
		// Given platform: aws, a projectRoot whose path contains a space, and no AWS env in
		// the current process — the hint will take the prefix-emitting branch
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t, mocks)
		t.Setenv("AWS_CONFIG_FILE", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/Users/foo/my projects", nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`
[profile test]
sso_session = company
`), nil
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("Token has expired")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the emitted env-prefix double-quotes the path so the suggested command pastes
		// into a shell as a single token — an unquoted path with a space would cause the
		// shell to split at the space and treat `projects/.aws/config` as a separate command,
		// breaking the copy-pasteable contract the hint advertises
		if err == nil {
			t.Fatal("Expected CheckAuth to fail when sts errors")
		}
		if !strings.Contains(errorText(err), `AWS_CONFIG_FILE="/Users/foo/my projects/contexts/test/.aws/config"`) {
			t.Errorf("Expected AWS_CONFIG_FILE to be double-quoted in hint, got: %v", err)
		}
	})

	t.Run("AWSPlatformConfigRootFailureSurfacesError", func(t *testing.T) {
		// Given platform: aws, the aws CLI is installed, and GetProjectRoot fails so the
		// context-scoped AWS env cannot be computed
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t, mocks)
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root not found")
		}
		stsCalled := false
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			stsCalled = true
			return "", nil
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the configRoot-resolution failure surfaces as an error and sts is never
		// invoked — letting the exec fall through with a nil env would silently validate
		// against the operator's ambient shell credentials, defeating the preflight
		if err == nil {
			t.Fatal("Expected CheckAuth to fail when configRoot cannot be resolved")
		}
		if !strings.Contains(err.Error(), "context-scoped AWS env") {
			t.Errorf("Expected context-scoped AWS env error, got: %v", err)
		}
		if stsCalled {
			t.Error("sts must not be invoked when the context-scoped env is unresolvable")
		}
	})

	t.Run("AWSPlatformOmitsAWSProfileWhenContextHasNoProfile", func(t *testing.T) {
		// Given a project-mode context whose .aws/ has no profile defined —
		// the operator has not run `aws configure` here and is relying on
		// ambient creds (static keys / IRSA / ECS / IMDS / whatever). Path X
		// must not pin AWS_PROFILE in this state, because the pin would point
		// at a profile section that doesn't exist and abort the SDK's chain
		// with "profile not found" before any provider gets a chance to run.
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t, mocks)
		t.Setenv("AWS_ACCESS_KEY_ID", "AKIATEST")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		var capturedEnv map[string]string
		var capturedEnvSeen bool
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			capturedEnv = env
			capturedEnvSeen = true
			return `{"Arn":"arn:aws:iam::123456789012:user/ci"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed, got %v", err)
		}
		// Then AWS_PROFILE is absent so the SDK chain runs naturally, but the
		// file paths still flow so a future `aws configure` writes into the
		// context's .aws/ rather than contaminating ~/.aws/
		if !capturedEnvSeen {
			t.Fatal("Expected sts to be invoked")
		}
		if _, ok := capturedEnv["AWS_PROFILE"]; ok {
			t.Errorf("Expected AWS_PROFILE to be absent when context has no profile, got %q", capturedEnv["AWS_PROFILE"])
		}
		if capturedEnv["AWS_CONFIG_FILE"] == "" {
			t.Errorf("Expected AWS_CONFIG_FILE to still flow (write redirection), got empty")
		}
		if capturedEnv["AWS_SHARED_CREDENTIALS_FILE"] == "" {
			t.Errorf("Expected AWS_SHARED_CREDENTIALS_FILE to still flow (write redirection), got empty")
		}
	})

	t.Run("AWSPlatformAmbientCredsWithoutAwsCliIsNoOp", func(t *testing.T) {
		// Given a lean CI image that has IRSA creds exported but no aws CLI binary —
		// typical of a minimal GitHub Actions runner using OIDC federation where
		// awscli was never added to the image. IRSA requires both AWS_WEB_IDENTITY_TOKEN_FILE
		// and AWS_ROLE_ARN to count as ambient.
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/var/run/secrets/eks.amazonaws.com/serviceaccount/token")
		t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/my-role")
		mocks.Shims.LookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}
		stsCalled := false
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			stsCalled = true
			return "", nil
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then CheckAuth accepts that it can't preflight here and returns nil — terraform's
		// AWS provider will exercise the native credential chain at apply time and surface
		// its own error if IRSA is actually misconfigured. A hard requirement on the aws
		// CLI in this case would be a false negative, since the CLI isn't part of the
		// runtime credential path for terraform at all.
		if err != nil {
			t.Errorf("Expected CheckAuth to be a no-op under IRSA without aws CLI, got %v", err)
		}
		if stsCalled {
			t.Error("sts must not be invoked when the aws CLI is absent")
		}
	})

	t.Run("GlobalModeDefersStsToAmbientAWSConfigButKeepsProfile", func(t *testing.T) {
		// Given platform: aws, global mode, and the operator's ambient
		// ~/.aws/config carries the matching [profile test] section so the
		// SDK can satisfy the pin
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		mocks.Shell.IsGlobalFunc = func() bool { return true }
		awsBinaryMock(t, mocks)
		withAmbientProfile(t, "[profile test]\nregion = us-west-2\n")
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			capturedEnv = env
			return `{"Arn":"arn:aws:iam::123456789012:user/x"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed, got %v", err)
		}
		// Then AWS_CONFIG_FILE / AWS_SHARED_CREDENTIALS_FILE are NOT injected (the
		// SDK resolves from the operator's ambient ~/.aws/), but AWS_PROFILE IS
		// injected from the context name so sts checks the profile the context
		// targets — without it sts would fall through to [default] and fail even
		// when the right profile is logged in.
		if _, ok := capturedEnv["AWS_CONFIG_FILE"]; ok {
			t.Errorf("AWS_CONFIG_FILE should not be set in global mode, got %q", capturedEnv["AWS_CONFIG_FILE"])
		}
		if _, ok := capturedEnv["AWS_SHARED_CREDENTIALS_FILE"]; ok {
			t.Errorf("AWS_SHARED_CREDENTIALS_FILE should not be set in global mode, got %q", capturedEnv["AWS_SHARED_CREDENTIALS_FILE"])
		}
		if capturedEnv["AWS_PROFILE"] != "test" {
			t.Errorf("AWS_PROFILE = %q, want %q (context name fallback)", capturedEnv["AWS_PROFILE"], "test")
		}
	})

	t.Run("GlobalModeWithExplicitProfileStillInjectsAWSProfile", func(t *testing.T) {
		// Given platform: aws, global mode, an explicit aws.profile override,
		// and the operator's ambient config defines that profile
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
    aws:
      profile: company-prod
`)
		mocks.Shell.IsGlobalFunc = func() bool { return true }
		awsBinaryMock(t, mocks)
		withAmbientProfile(t, "[profile company-prod]\nregion = us-east-1\n")
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			capturedEnv = env
			return `{"Arn":"arn:aws:iam::123456789012:user/x"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed, got %v", err)
		}
		// Then AWS_PROFILE is set (the operator explicitly asked for that profile,
		// even in global mode) but config-file paths are NOT — sts resolves the
		// profile against the ambient ~/.aws/config.
		if capturedEnv["AWS_PROFILE"] != "company-prod" {
			t.Errorf("AWS_PROFILE = %q, want %q", capturedEnv["AWS_PROFILE"], "company-prod")
		}
		if _, ok := capturedEnv["AWS_CONFIG_FILE"]; ok {
			t.Errorf("AWS_CONFIG_FILE should not be set in global mode, got %q", capturedEnv["AWS_CONFIG_FILE"])
		}
		if _, ok := capturedEnv["AWS_SHARED_CREDENTIALS_FILE"]; ok {
			t.Errorf("AWS_SHARED_CREDENTIALS_FILE should not be set in global mode, got %q", capturedEnv["AWS_SHARED_CREDENTIALS_FILE"])
		}
	})

	t.Run("AWSConfigBlockTriggersAuthCheck", func(t *testing.T) {
		// Given the context has an aws block (no platform set) and creds are invalid
		mocks, toolsManager := setup(t, `
contexts:
  test:
    aws:
      region: us-east-1
`)
		awsBinaryMock(t, mocks)
		stsCalled := false
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "aws" && len(args) == 1 && args[0] == "--version" {
				return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
			}
			if command == "aws" && len(args) == 2 && args[0] == "sts" && args[1] == "get-caller-identity" {
				stsCalled = true
				return "", fmt.Errorf("Unable to locate credentials")
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then sts was invoked and the error surfaced — an aws block alone is enough to gate
		// on credentials, matching the env-printer activation rule
		if !stsCalled {
			t.Error("Expected sts get-caller-identity to be invoked when aws block is present")
		}
		if err == nil {
			t.Error("Expected CheckAuth to surface the credential failure")
		}
	})
}

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
