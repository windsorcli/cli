package tools_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestToolsManager_CheckAuthGCP(t *testing.T) {
	setup := func(t *testing.T, configStr string) (*Mocks, *tools.BaseToolsManager) {
		t.Helper()
		// Clear every env var the ambient-credentials guard consults so CheckAuth behaves
		// identically across dev laptops, GKE Workload Identity pods, and CI runners.
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
		t.Setenv("CLOUDSDK_CONFIG", "")
		mocks := setupMocks(t, &SetupOptions{ConfigStr: configStr})
		toolsManager := tools.NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.SetShims(mocks.Shims)
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
