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

func TestToolsManager_CheckAuthGCP(t *testing.T) {
	setup := func(t *testing.T, configStr string) (*Mocks, *BaseToolsManager) {
		t.Helper()
		// Clear every env var the ambient-credentials guard consults so CheckAuth behaves
		// identically across dev laptops, GKE Workload Identity pods, and CI runners.
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
		t.Setenv("CLOUDSDK_CONFIG", "")
		mocks := setupMocks(t, &SetupOptions{ConfigStr: configStr})
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		return mocks, toolsManager
	}

	gcloudBinaryMock := func(t *testing.T, mocks *Mocks) {
		t.Helper()
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "gcloud" {
				return "/usr/bin/gcloud", nil
			}
			return "", exec.ErrNotFound
		}
	}

	t.Run("GCPPlatformWithGcloudCliMissing", func(t *testing.T) {
		// Given platform: gcp and the gcloud CLI is not in PATH
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: gcp
`)
		mocks.Shims.LookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}

		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the missing-binary error surfaces with the vendor install URL
		if err == nil {
			t.Fatal("Expected error when gcloud CLI is missing")
		}
		if !strings.Contains(err.Error(), "Google Cloud CLI") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected 'Google Cloud CLI ... not found on PATH' error, got: %v", err)
		}
	})

	t.Run("GCPPlatformWithAmbientCredentialsAndGcloudMissing", func(t *testing.T) {
		// Given platform: gcp, ambient Application Default Credentials are present
		// (GOOGLE_APPLICATION_CREDENTIALS set, as google-github-actions/auth does in CI),
		// and the gcloud CLI is not in PATH
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: gcp
`)
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/var/run/secrets/gcp/key.json")
		mocks.Shims.LookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}

		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the missing-binary error still surfaces — ambient credentials satisfy
		// terraform's SDK auth but not gke-gcloud-auth-plugin or gcloud-shelling teardown
		// cleanup, so a missing gcloud must never pass silently.
		if err == nil {
			t.Fatal("Expected error when gcloud CLI is missing, even with ambient credentials")
		}
		if !strings.Contains(err.Error(), "Google Cloud CLI") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected 'Google Cloud CLI ... not found on PATH' error, got: %v", err)
		}
	})

	t.Run("GCPPlatformWithValidCredentials", func(t *testing.T) {
		// Given platform: gcp, the gcloud CLI is installed, and application-default
		// credentials mint a live access token
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: gcp
`)
		gcloudBinaryMock(t, mocks)
		var tokenArgs []string
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "gcloud" && len(args) == 1 && args[0] == "--version" {
				return fmt.Sprintf("Google Cloud SDK %s\n", constants.MinimumVersionGCP), nil
			}
			return "", fmt.Errorf("command not mocked: %s %v", command, args)
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "gcloud" && len(args) >= 1 && args[0] == "auth" {
				tokenArgs = args
				return "ya29.fake-token", nil
			}
			return "", fmt.Errorf("command not mocked: %v", args)
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the probe is `gcloud auth application-default print-access-token` — a live
		// token mint, not a cached-account listing.
		if err != nil {
			t.Errorf("Expected CheckAuth to succeed when the token mint resolves, got %v", err)
		}
		joined := strings.Join(tokenArgs, " ")
		if joined != "auth application-default print-access-token" {
			t.Errorf("Expected the print-access-token probe, got args: %v", tokenArgs)
		}
	})

	t.Run("GCPPlatformExpiredCredentialsSurfacesActionableHint", func(t *testing.T) {
		// Given platform: gcp, the gcloud CLI is installed, but application-default
		// credentials have expired or were never configured
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: gcp
`)
		gcloudBinaryMock(t, mocks)
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("Google Cloud SDK %s\n", constants.MinimumVersionGCP), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("ERROR: (gcloud.auth.application-default.print-access-token) Reauthentication failed")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the surfaced error names ONLY the actionable next step, without the doubled
		// "command execution failed" + raw gcloud stderr + hint stack.
		if err == nil {
			t.Fatal("Expected CheckAuth to fail when print-access-token errors")
		}
		if !strings.Contains(errorText(err), "gcloud auth application-default login") {
			t.Errorf("Expected `gcloud auth application-default login` remediation in error, got: %v", err)
		}
		if strings.Contains(err.Error(), "command execution failed") {
			t.Errorf("Expected raw shell-exec error text to be suppressed, got: %v", err)
		}
		if strings.Contains(err.Error(), "Reauthentication failed") {
			t.Errorf("Expected vendor stderr to be suppressed in favor of the windsor hint, got: %v", err)
		}
	})

	t.Run("GCPPlatformInjectsContextScopedConfigDir", func(t *testing.T) {
		// Given platform: gcp and the gcloud CLI is installed
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: gcp
`)
		gcloudBinaryMock(t, mocks)
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("Google Cloud SDK %s\n", constants.MinimumVersionGCP), nil
		}
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			capturedEnv = env
			return "ya29.fake-token", nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed, got %v", err)
		}
		// Then the probe received the context-scoped CLOUDSDK_CONFIG so it resolves against
		// the context's .gcp/gcloud/ rather than whatever happens to be active in the
		// parent shell.
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("GetConfigRoot failed: %v", err)
		}
		want := filepath.ToSlash(filepath.Join(configRoot, ".gcp", "gcloud"))
		if capturedEnv["CLOUDSDK_CONFIG"] != want {
			t.Errorf("CLOUDSDK_CONFIG = %q, want %q", capturedEnv["CLOUDSDK_CONFIG"], want)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestToolsManager_checkGCPBinary(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given gcloud is available and meets the minimum version
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "gcloud" {
				return "/usr/bin/gcloud", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "gcloud" && len(args) == 1 && args[0] == "--version" {
				return fmt.Sprintf("Google Cloud SDK %s\n", constants.MinimumVersionGCP), nil
			}
			return "", fmt.Errorf("command not mocked: %s %v", command, args)
		}
		// When checking the gcloud binary
		err := toolsManager.checkGCPBinary()
		// Then no error — checkGCPBinary must NOT invoke the auth probe; that lives in
		// CheckAuth, which fires only from bootstrap/up/apply preflights and `windsor check`.
		if err != nil {
			t.Errorf("Expected checkGCPBinary to succeed, got %v", err)
		}
	})

	t.Run("GcloudNotAvailable", func(t *testing.T) {
		// Given gcloud is not in PATH
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "gcloud" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		// When checking gcloud
		err := toolsManager.checkGCPBinary()
		// Then error mentions not on PATH and points to the vendor install URL
		if err == nil {
			t.Fatal("Expected error when gcloud is not in PATH")
		}
		if !strings.Contains(err.Error(), "Google Cloud CLI") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected 'Google Cloud CLI ... not found on PATH' in error, got: %v", err)
		}
		if !strings.Contains(errorText(err), "https://cloud.google.com/sdk/docs/install") {
			t.Errorf("Expected vendor install URL in error, got: %v", err)
		}
	})

	t.Run("VersionTooLow", func(t *testing.T) {
		// Given gcloud reports a version below the minimum
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "gcloud" {
				return "/usr/bin/gcloud", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "gcloud" && len(args) == 1 && args[0] == "--version" {
				return "Google Cloud SDK 1.2.3\n", nil
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When checking gcloud
		err := toolsManager.checkGCPBinary()
		// Then the version-too-low error surfaces
		if err == nil {
			t.Fatal("Expected error when gcloud version is too low")
		}
		if !strings.Contains(err.Error(), "below the minimum required version") {
			t.Errorf("Expected version-too-low error, got: %v", err)
		}
	})
}

func Test_gcpAuthHint(t *testing.T) {
	t.Run("ProjectModeWithoutSourcedShellPrefixesContextScopedConfigDir", func(t *testing.T) {
		// Given a toolsManager in project mode and the parent shell does NOT have
		// CLOUDSDK_CONFIG pointing at the context (plain shell — windsor env hasn't
		// been sourced)
		t.Setenv("CLOUDSDK_CONFIG", "")
		mocks := setupMocks(t, &SetupOptions{ConfigStr: `
contexts:
  test:
    platform: gcp
`})
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		// When gcpAuthError runs
		hint := errorText(toolsManager.gcpAuthError())
		// Then the suggested login is prefixed with CLOUDSDK_CONFIG pointing at the
		// context's .gcp/gcloud/ so credentials land in the right place.
		if !strings.Contains(hint, "CLOUDSDK_CONFIG=") {
			t.Errorf("Expected CLOUDSDK_CONFIG prefix when shell env doesn't point at context, got: %q", hint)
		}
		if !strings.Contains(hint, "gcloud auth application-default login") {
			t.Errorf("Expected `gcloud auth application-default login` in hint, got: %q", hint)
		}
	})

	t.Run("ProjectModeWithSourcedShellOmitsEnvPrefix", func(t *testing.T) {
		// Given a toolsManager in project mode where the shell already exports
		// CLOUDSDK_CONFIG pointing at the context (windsor env has been sourced)
		mocks := setupMocks(t, &SetupOptions{ConfigStr: `
contexts:
  test:
    platform: gcp
`})
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("GetConfigRoot failed: %v", err)
		}
		t.Setenv("CLOUDSDK_CONFIG", filepath.ToSlash(filepath.Join(configRoot, ".gcp", "gcloud")))
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		// When gcpAuthError runs
		hint := errorText(toolsManager.gcpAuthError())
		// Then the hint emits a bare login command with no env prefix.
		if strings.Contains(hint, "CLOUDSDK_CONFIG=") {
			t.Errorf("Hint should omit CLOUDSDK_CONFIG= prefix when shell env already points at context, got: %q", hint)
		}
	})

	t.Run("GlobalModeOmitsEnvPrefix", func(t *testing.T) {
		// Given a toolsManager in global mode
		mocks := setupMocks(t)
		mocks.Shell.IsGlobalFunc = func() bool { return true }
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		// When gcpAuthError runs
		hint := errorText(toolsManager.gcpAuthError())
		// Then no CLOUDSDK_CONFIG prefix is emitted — in global mode windsor defers to the
		// operator's ambient ~/.config/gcloud.
		if strings.Contains(hint, "CLOUDSDK_CONFIG=") {
			t.Errorf("Expected no CLOUDSDK_CONFIG prefix in global mode, got: %q", hint)
		}
		if !strings.Contains(hint, "gcloud auth application-default login") {
			t.Errorf("Expected `gcloud auth application-default login` in hint, got: %q", hint)
		}
	})
}

func Test_gcpContextEnv(t *testing.T) {
	t.Run("ProjectModeReturnsContextScopedConfigDir", func(t *testing.T) {
		// Given a tools manager in project mode with no ambient GCP credentials
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims

		// When gcpContextEnv is invoked
		env, err := toolsManager.gcpContextEnv()

		// Then CLOUDSDK_CONFIG points at the context's .gcp/gcloud/, mirroring gcp_env.go.
		if err != nil {
			t.Fatalf("gcpContextEnv returned error: %v", err)
		}
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("GetConfigRoot failed: %v", err)
		}
		want := filepath.ToSlash(filepath.Join(configRoot, ".gcp", "gcloud"))
		if env["CLOUDSDK_CONFIG"] != want {
			t.Errorf("CLOUDSDK_CONFIG = %q, want %q", env["CLOUDSDK_CONFIG"], want)
		}
	})

	t.Run("GlobalModeReturnsNil", func(t *testing.T) {
		// Given a tools manager in global mode
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
		mocks := setupMocks(t)
		mocks.Shell.IsGlobalFunc = func() bool { return true }
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims

		// When gcpContextEnv is invoked
		env, err := toolsManager.gcpContextEnv()

		// Then (nil, nil) is returned — the SDK should resolve from the operator's ambient
		// ~/.config/gcloud rather than a context-scoped dir.
		if err != nil {
			t.Fatalf("gcpContextEnv returned error: %v", err)
		}
		if env != nil {
			t.Errorf("expected nil env in global mode, got %v", env)
		}
	})

	t.Run("AmbientCredentialsReturnsNil", func(t *testing.T) {
		// Given ambient Application Default Credentials (GOOGLE_APPLICATION_CREDENTIALS set)
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/var/run/secrets/gcp/key.json")
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims

		// When gcpContextEnv is invoked
		env, err := toolsManager.gcpContextEnv()

		// Then (nil, nil) — overriding CLOUDSDK_CONFIG would mask the native credential chain.
		if err != nil {
			t.Fatalf("gcpContextEnv returned error: %v", err)
		}
		if env != nil {
			t.Errorf("expected nil env under ambient credentials, got %v", env)
		}
	})
}
