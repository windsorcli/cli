package tools

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestToolsManager_CheckAuthAzure(t *testing.T) {
	setup := func(t *testing.T, configStr string) (*Mocks, *BaseToolsManager) {
		t.Helper()
		// Clear every env var the ambient-credentials guards consult so CheckAuth behaves
		// identically across dev laptops, AKS Workload Identity pods, and CI runners.
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
		t.Setenv("AWS_ACCESS_KEY_ID", "")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "")
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "")
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", "")
		t.Setenv("AZURE_CONFIG_DIR", "")
		mocks := setupMocks(t, &SetupOptions{ConfigStr: configStr})
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		return mocks, toolsManager
	}

	azBinaryMock := func(t *testing.T, mocks *Mocks) {
		t.Helper()
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "az" {
				return "/usr/bin/az", nil
			}
			return "", exec.ErrNotFound
		}
	}

	t.Run("AzurePlatformWithAzCliMissing", func(t *testing.T) {
		// Given platform: azure and the az CLI is not in PATH
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		mocks.Shims.LookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}

		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the missing-binary error surfaces with the vendor install URL — the binary
		// check is part of CheckAuth so bootstrap/up preflights catch a missing CLI before
		// they try to resolve credentials.
		if err == nil {
			t.Fatal("Expected error when az CLI is missing")
		}
		if !strings.Contains(err.Error(), "Azure CLI") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected 'Azure CLI ... not found on PATH' error, got: %v", err)
		}
	})

	t.Run("AzurePlatformWithValidCredentials", func(t *testing.T) {
		// Given platform: azure, the az CLI is installed, and `az rest` against ARM's
		// /tenants endpoint returns successfully (the API call AAD validates server-side,
		// not a cache lookup, so a stale access token gets rejected here even when
		// `account get-access-token` would silently return it from msal_token_cache.bin).
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
`)
		azBinaryMock(t, mocks)
		var restArgs []string
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) == 1 && args[0] == "--version" {
				return fmt.Sprintf("azure-cli                         %s\n", constants.MinimumVersionAzure), nil
			}
			return "", fmt.Errorf("command not mocked: %s %v", command, args)
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) >= 1 && args[0] == "rest" {
				restArgs = args
				return ``, nil
			}
			return "", fmt.Errorf("command not mocked: %v", args)
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the probe is `az rest` against management.azure.com/tenants — a real ARM
		// API call that exercises the credential server-side. NOT `az account get-access-
		// token`, which is a cache lookup and silently returns stale tokens.
		if err != nil {
			t.Errorf("Expected CheckAuth to succeed when az rest resolves, got %v", err)
		}
		joined := strings.Join(restArgs, " ")
		if !strings.Contains(joined, "https://management.azure.com/tenants") {
			t.Errorf("Expected ARM /tenants endpoint, got args: %v", restArgs)
		}
		if strings.Contains(joined, "get-access-token") {
			t.Errorf("Probe must not be get-access-token (cache lookup, silently passes on stale tokens), got args: %v", restArgs)
		}
	})

	t.Run("AzurePlatformWithoutTenantIDStillRunsTheProbe", func(t *testing.T) {
		// Given platform: azure with NO azure.tenant_id configured
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		azBinaryMock(t, mocks)
		var restArgs []string
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("azure-cli                         %s\n", constants.MinimumVersionAzure), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) >= 1 && args[0] == "rest" {
				restArgs = args
				return ``, nil
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the same `az rest` probe runs — it doesn't need --tenant because /tenants
		// returns whichever tenants the active credential is authorized for, and the
		// server-side validation catches a dead session regardless of tenant pinning.
		if err != nil {
			t.Errorf("Expected CheckAuth to succeed, got %v", err)
		}
		joined := strings.Join(restArgs, " ")
		if !strings.Contains(joined, "https://management.azure.com/tenants") {
			t.Errorf("Expected ARM /tenants endpoint, got args: %v", restArgs)
		}
		if strings.Contains(joined, "--tenant") {
			t.Errorf("az rest does not take --tenant for /tenants — pinning would be ignored, got args: %v", restArgs)
		}
	})

	t.Run("AzurePlatformIgnoresArmTenantIDEnv", func(t *testing.T) {
		// Given ARM_TENANT_ID is exported in the parent environment (windsor exports it
		// from azure.tenant_id, so reading it back would be circular)
		t.Setenv("ARM_TENANT_ID", "envvar00-1111-2222-3333-444444444444")
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		azBinaryMock(t, mocks)
		var restArgs []string
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("azure-cli                         %s\n", constants.MinimumVersionAzure), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) >= 1 && args[0] == "rest" {
				restArgs = args
				return ``, nil
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then ARM_TENANT_ID does not leak into the probe args — the probe doesn't read
		// it (windsor exports it from azure.tenant_id, so reading it back would echo the
		// field we already check) and the /tenants endpoint doesn't take --tenant anyway.
		if err != nil {
			t.Errorf("Expected CheckAuth to succeed, got %v", err)
		}
		joined := strings.Join(restArgs, " ")
		if strings.Contains(joined, "envvar00-1111-2222-3333-444444444444") {
			t.Errorf("ARM_TENANT_ID must not influence the probe, got args: %v", restArgs)
		}
	})

	t.Run("AzurePlatformExpiredTokenSurfacesActionableHint", func(t *testing.T) {
		// Given platform: azure with tenant_id configured, the az CLI is installed, but
		// `az rest` against ARM fails because the refresh token expired (the AADSTS700082
		// case in the bug report)
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
`)
		azBinaryMock(t, mocks)
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) == 1 && args[0] == "--version" {
				return fmt.Sprintf("azure-cli                         %s\n", constants.MinimumVersionAzure), nil
			}
			return "", fmt.Errorf("command not mocked")
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("AADSTS700082: refresh token expired")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the surfaced error names ONLY the actionable next step — the `az login`
		// command — without the doubled "command execution failed" + raw az stderr stack.
		// The hint is the only thing the operator needs to read to recover.
		if err == nil {
			t.Fatal("Expected CheckAuth to fail when get-access-token errors")
		}
		if !strings.Contains(errorText(err), "az login") {
			t.Errorf("Expected `az login` remediation in error, got: %v", err)
		}
		if strings.Contains(err.Error(), "command execution failed") {
			t.Errorf("Expected raw shell-exec error text to be suppressed, got: %v", err)
		}
		if strings.Contains(err.Error(), "AADSTS700082") {
			t.Errorf("Expected vendor stderr to be suppressed in favor of the windsor hint, got: %v", err)
		}
	})

	t.Run("AzurePlatformInjectsContextScopedConfigDir", func(t *testing.T) {
		// Given platform: azure with tenant_id and the az CLI is installed
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
`)
		azBinaryMock(t, mocks)
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("azure-cli                         %s\n", constants.MinimumVersionAzure), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) >= 1 && args[0] == "rest" {
				capturedEnv = env
			}
			return ``, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed, got %v", err)
		}
		// Then `az account get-access-token` received the context-scoped AZURE_CONFIG_DIR so
		// it resolves against the context's .azure/ rather than whatever happens to be active
		// in the parent shell — without this, CheckAuth could green-light bootstrap using
		// stale global credentials that terraform apply would later fail on.
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("GetConfigRoot failed: %v", err)
		}
		want := filepath.ToSlash(filepath.Join(configRoot, ".azure"))
		if capturedEnv["AZURE_CONFIG_DIR"] != want {
			t.Errorf("AZURE_CONFIG_DIR = %q, want %q", capturedEnv["AZURE_CONFIG_DIR"], want)
		}
	})

	t.Run("AzurePlatformWithTenantIDPinsLoginToTenant", func(t *testing.T) {
		// Given platform: azure with an explicit tenant_id and an expired token
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
    azure:
      tenant_id: 3d8e9161-441c-44e9-b50a-1d9a78c67ab0
`)
		azBinaryMock(t, mocks)
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("azure-cli                         %s\n", constants.MinimumVersionAzure), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("AADSTS700082: refresh token expired")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the suggested `az login` is pinned to the configured tenant — operators with
		// multiple tenants land in the right one without guessing, mirroring how AWS pins the
		// suggestion to a specific profile when aws.profile is set.
		if err == nil {
			t.Fatal("Expected CheckAuth to fail")
		}
		if !strings.Contains(errorText(err), "az login --tenant 3d8e9161-441c-44e9-b50a-1d9a78c67ab0") {
			t.Errorf("Expected tenant-pinned az login command, got: %v", err)
		}
	})

	t.Run("AzureConfigBlockTriggersAuthCheck", func(t *testing.T) {
		// Given the context has an azure block (no platform set) with tenant_id configured,
		// and `az rest` against ARM fails
		mocks, toolsManager := setup(t, `
contexts:
  test:
    azure:
      subscription_id: 00000000-0000-0000-0000-000000000000
      tenant_id: 11111111-2222-3333-4444-555555555555
`)
		azBinaryMock(t, mocks)
		restCalled := false
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("azure-cli                         %s\n", constants.MinimumVersionAzure), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) >= 1 && args[0] == "rest" {
				restCalled = true
				return "", fmt.Errorf("Please run 'az login'")
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then `az rest` was invoked and the error surfaced — an azure block alone is
		// enough to gate on credentials, matching the env-printer activation rule.
		if !restCalled {
			t.Error("Expected az rest to be invoked when azure block is present")
		}
		if err == nil {
			t.Error("Expected CheckAuth to surface the credential failure")
		}
	})

	t.Run("AzurePlatformWorkloadIdentitySkipsCliCheckWhenAzAbsent", func(t *testing.T) {
		// Given AKS Workload Identity has injected AZURE_FEDERATED_TOKEN_FILE and the az CLI
		// is absent — typical of a minimal pod image where the cluster's federation handles
		// auth and the CLI was never installed
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "/var/run/secrets/azure/tokens/azure-identity-token")
		t.Setenv("AZURE_CLIENT_ID", "00000000-0000-0000-0000-000000000000")
		t.Setenv("AZURE_TENANT_ID", "00000000-0000-0000-0000-000000000000")
		mocks.Shims.LookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}
		azCalled := false
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			azCalled = true
			return "", nil
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then CheckAuth is a no-op — terraform's azurerm provider will exercise the native
		// Workload Identity chain at apply time. Requiring az here would be a false negative,
		// since az isn't part of the runtime credential path for terraform under WI at all.
		if err != nil {
			t.Errorf("Expected CheckAuth to be a no-op under Workload Identity without az CLI, got %v", err)
		}
		if azCalled {
			t.Error("az must not be invoked when az is absent under ambient creds")
		}
	})

	t.Run("AzurePlatformGlobalModeOmitsConfigDirInjection", func(t *testing.T) {
		// Given platform: azure with tenant_id and the shell reports global mode (operator
		// invoking windsor outside a project tree)
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
`)
		mocks.Shell.IsGlobalFunc = func() bool { return true }
		azBinaryMock(t, mocks)
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("azure-cli                         %s\n", constants.MinimumVersionAzure), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			capturedEnv = env
			return `{"id":"00000000-0000-0000-0000-000000000000"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed, got %v", err)
		}
		// Then AZURE_CONFIG_DIR is NOT injected — the SDK resolves from the operator's
		// ambient ~/.azure/, matching the AWS global-mode behavior of deferring config-dir
		// scoping to the operator.
		if _, ok := capturedEnv["AZURE_CONFIG_DIR"]; ok {
			t.Errorf("AZURE_CONFIG_DIR should not be set in global mode, got %q", capturedEnv["AZURE_CONFIG_DIR"])
		}
	})

	t.Run("AzureSovereignCloudRoutesProbeToSovereignEndpoint", func(t *testing.T) {
		// Given azure.environment names a sovereign cloud, the probe URI must target
		// that cloud's ARM host — az CLI does not rewrite explicit --uri values, so a
		// hardcoded management.azure.com would hit the wrong endpoint and surface as a
		// false auth failure for usgovernment / china tenants.
		cases := []struct {
			env  string
			host string
		}{
			{"usgovernment", "management.usgovcloudapi.net"},
			{"china", "management.chinacloudapi.cn"},
			{"german", "management.microsoftazure.de"},
		}
		for _, tc := range cases {
			t.Run(tc.env, func(t *testing.T) {
				mocks, toolsManager := setup(t, fmt.Sprintf(`
contexts:
  test:
    platform: azure
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
      environment: %s
`, tc.env))
				azBinaryMock(t, mocks)
				var restArgs []string
				mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
					return fmt.Sprintf("azure-cli                         %s\n", constants.MinimumVersionAzure), nil
				}
				mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
					if command == "az" && len(args) >= 1 && args[0] == "rest" {
						restArgs = args
						return ``, nil
					}
					return "", fmt.Errorf("command not mocked: %v", args)
				}
				if err := toolsManager.CheckAuth(); err != nil {
					t.Fatalf("Expected CheckAuth to succeed for env %q, got %v", tc.env, err)
				}
				wantURI := fmt.Sprintf("https://%s/tenants", tc.host)
				if !strings.Contains(strings.Join(restArgs, " "), wantURI) {
					t.Errorf("Expected probe URI to contain %q for env %q, got args: %v", wantURI, tc.env, restArgs)
				}
			})
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestToolsManager_checkAzureBinary(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given az is available and meets the minimum version
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "az" {
				return "/usr/bin/az", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) == 1 && args[0] == "--version" {
				return fmt.Sprintf("azure-cli                         %s\n", constants.MinimumVersionAzure), nil
			}
			return "", fmt.Errorf("command not mocked: %s %v", command, args)
		}
		// When checking the az binary
		err := toolsManager.checkAzureBinary()
		// Then no error — checkAzureBinary must NOT invoke `az rest`; that lives in
		// CheckAuth, which fires only from bootstrap/up/apply preflights and
		// `windsor check`.
		if err != nil {
			t.Errorf("Expected checkAzureBinary to succeed, got %v", err)
		}
	})

	t.Run("AzNotAvailable", func(t *testing.T) {
		// Given az is not in PATH
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "az" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		// When checking az
		err := toolsManager.checkAzureBinary()
		// Then error mentions not on PATH and points to the vendor install URL
		if err == nil {
			t.Fatal("Expected error when az is not in PATH")
		}
		if !strings.Contains(err.Error(), "Azure CLI") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected 'Azure CLI ... not found on PATH' in error, got: %v", err)
		}
		if !strings.Contains(errorText(err), "https://learn.microsoft.com/en-us/cli/azure/install-azure-cli") {
			t.Errorf("Expected vendor install URL in error, got: %v", err)
		}
	})

	t.Run("VersionTooLow", func(t *testing.T) {
		// Given az reports a version below the minimum
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "az" {
				return "/usr/bin/az", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) == 1 && args[0] == "--version" {
				return "azure-cli                         1.2.3\n", nil
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When checking az
		err := toolsManager.checkAzureBinary()
		// Then the version-too-low error surfaces
		if err == nil {
			t.Fatal("Expected error when az version is too low")
		}
		if !strings.Contains(err.Error(), "below the minimum required version") {
			t.Errorf("Expected version-too-low error, got: %v", err)
		}
	})
}

func Test_armTenantEndpoint(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"", "management.azure.com"},
		{"public", "management.azure.com"},
		{"usgovernment", "management.usgovcloudapi.net"},
		{"china", "management.chinacloudapi.cn"},
		{"german", "management.microsoftazure.de"},
		{"unknown-cloud", "management.azure.com"},
	}
	for _, tc := range cases {
		if got := armTenantEndpoint(tc.env); got != tc.want {
			t.Errorf("armTenantEndpoint(%q) = %q, want %q", tc.env, got, tc.want)
		}
	}
}

func Test_azureAuthHint(t *testing.T) {
	t.Run("ProjectModeWithoutTenantPrefixesContextScopedConfigDir", func(t *testing.T) {
		// Given a toolsManager in project mode with no tenant_id configured, and the parent
		// shell does NOT have AZURE_CONFIG_DIR pointing at the context (plain shell — windsor
		// env hasn't been sourced)
		t.Setenv("AZURE_CONFIG_DIR", "")
		mocks := setupMocks(t, &SetupOptions{ConfigStr: `
contexts:
  test:
    platform: azure
`})
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		// When azureAuthError runs
		hint := errorText(toolsManager.azureAuthError())
		// Then the suggested `az login` is prefixed with AZURE_CONFIG_DIR pointing at the
		// context's .azure/ so credentials land in the right place — operators in a plain
		// shell get a copy-pasteable command that doesn't contaminate ~/.azure with this
		// context's tokens.
		if !strings.Contains(hint, "AZURE_CONFIG_DIR=") {
			t.Errorf("Expected AZURE_CONFIG_DIR prefix when shell env doesn't point at context, got: %q", hint)
		}
		if !strings.Contains(hint, "az login") {
			t.Errorf("Expected `az login` in hint, got: %q", hint)
		}
		if strings.Contains(hint, "--tenant") {
			t.Errorf("Expected no --tenant flag when tenant_id is unset, got: %q", hint)
		}
	})

	t.Run("ProjectModeWithSourcedShellOmitsEnvPrefix", func(t *testing.T) {
		// Given a toolsManager in project mode where the shell already exports
		// AZURE_CONFIG_DIR pointing at the context (windsor env has been sourced)
		mocks := setupMocks(t, &SetupOptions{ConfigStr: `
contexts:
  test:
    platform: azure
`})
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("GetConfigRoot failed: %v", err)
		}
		t.Setenv("AZURE_CONFIG_DIR", filepath.ToSlash(filepath.Join(configRoot, ".azure")))
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		// When azureAuthError runs
		hint := errorText(toolsManager.azureAuthError())
		// Then the hint emits a bare `az login` with no env prefix — the operator's shell
		// will resolve AZURE_CONFIG_DIR from its own env, so prefixing would just be noise
		// in the common case.
		if strings.Contains(hint, "AZURE_CONFIG_DIR=") {
			t.Errorf("Hint should omit AZURE_CONFIG_DIR= prefix when shell env already points at context, got: %q", hint)
		}
	})

	t.Run("GlobalModeOmitsEnvPrefix", func(t *testing.T) {
		// Given a toolsManager in global mode
		mocks := setupMocks(t)
		mocks.Shell.IsGlobalFunc = func() bool { return true }
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		// When azureAuthError runs
		hint := errorText(toolsManager.azureAuthError())
		// Then no AZURE_CONFIG_DIR prefix is emitted — in global mode windsor defers to the
		// operator's ambient ~/.azure/, so prefixing would point them at a context dir we
		// have no business writing to.
		if strings.Contains(hint, "AZURE_CONFIG_DIR=") {
			t.Errorf("Expected no AZURE_CONFIG_DIR prefix in global mode, got: %q", hint)
		}
		if !strings.Contains(hint, "az login") {
			t.Errorf("Expected `az login` in hint, got: %q", hint)
		}
	})

	t.Run("TenantIDIsAppendedToLoginCommand", func(t *testing.T) {
		// Given a context configured with an explicit Azure tenant_id
		mocks := setupMocks(t, &SetupOptions{ConfigStr: `
contexts:
  test:
    platform: azure
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
`})
		mocks.Shell.IsGlobalFunc = func() bool { return true }
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		// When azureAuthError runs
		hint := errorText(toolsManager.azureAuthError())
		// Then the suggested `az login` is pinned to the configured tenant — operators with
		// multiple tenants get a one-shot command that lands in the right tenant without
		// having to think about it.
		if !strings.Contains(hint, "az login --tenant 11111111-2222-3333-4444-555555555555") {
			t.Errorf("Expected tenant-pinned az login command, got: %q", hint)
		}
	})
}

func Test_azureContextEnv(t *testing.T) {
	clearAmbientAzure := func(t *testing.T) {
		t.Helper()
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "")
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", "")
	}

	t.Run("ProjectModeReturnsContextScopedConfigDir", func(t *testing.T) {
		// Given a tools manager in project mode with no ambient azure creds
		clearAmbientAzure(t)
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims

		// When azureContextEnv is invoked
		env, err := toolsManager.azureContextEnv()

		// Then AZURE_CONFIG_DIR points at the context's .azure/ and the v2 login experience
		// is disabled (which mirrors azure_env.go and matches what `az login` from a windsor
		// shell expects).
		if err != nil {
			t.Fatalf("azureContextEnv returned error: %v", err)
		}
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("GetConfigRoot failed: %v", err)
		}
		want := filepath.ToSlash(filepath.Join(configRoot, ".azure"))
		if env["AZURE_CONFIG_DIR"] != want {
			t.Errorf("AZURE_CONFIG_DIR = %q, want %q", env["AZURE_CONFIG_DIR"], want)
		}
		if env["AZURE_CORE_LOGIN_EXPERIENCE_V2"] != "false" {
			t.Errorf("AZURE_CORE_LOGIN_EXPERIENCE_V2 = %q, want %q", env["AZURE_CORE_LOGIN_EXPERIENCE_V2"], "false")
		}
	})

	t.Run("GlobalModeReturnsNil", func(t *testing.T) {
		// Given a tools manager in global mode
		clearAmbientAzure(t)
		mocks := setupMocks(t)
		mocks.Shell.IsGlobalFunc = func() bool { return true }
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims

		// When azureContextEnv is invoked
		env, err := toolsManager.azureContextEnv()

		// Then (nil, nil) is returned — the SDK should resolve from the operator's ambient
		// ~/.azure/ rather than a context-scoped dir.
		if err != nil {
			t.Fatalf("azureContextEnv returned error: %v", err)
		}
		if env != nil {
			t.Errorf("expected nil env in global mode, got %v", env)
		}
	})

	t.Run("AmbientWorkloadIdentityReturnsNil", func(t *testing.T) {
		// Given a Workload Identity environment (AZURE_FEDERATED_TOKEN_FILE set)
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "/var/run/secrets/azure/tokens/azure-identity-token")
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims

		// When azureContextEnv is invoked
		env, err := toolsManager.azureContextEnv()

		// Then (nil, nil) — overriding AZURE_CONFIG_DIR would point az at a directory whose
		// federated-token state isn't compatible with the SDK's WI flow, masking the native
		// credential chain.
		if err != nil {
			t.Fatalf("azureContextEnv returned error: %v", err)
		}
		if env != nil {
			t.Errorf("expected nil env under Workload Identity, got %v", env)
		}
	})

	t.Run("AmbientClientSecretReturnsNil", func(t *testing.T) {
		// Given an SPN environment (AZURE_CLIENT_SECRET set)
		t.Setenv("AZURE_CLIENT_SECRET", "secret")
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims

		// When azureContextEnv is invoked
		env, err := toolsManager.azureContextEnv()

		// Then (nil, nil) — same reasoning as Workload Identity.
		if err != nil {
			t.Fatalf("azureContextEnv returned error: %v", err)
		}
		if env != nil {
			t.Errorf("expected nil env under SPN client_secret, got %v", env)
		}
	})
}
