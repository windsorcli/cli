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
