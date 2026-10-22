package tools

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Test Private Methods
// =============================================================================

// Tests for Docker and Docker Compose version validation
func TestToolsManager_checkDocker(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// When all required tools are available with correct versions
		_, toolsManager := setup(t)
		err := toolsManager.checkDocker()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected checkDocker to succeed, but got error: %v", err)
		}
	})

	t.Run("DockerNotAvailable", func(t *testing.T) {
		// When docker is not found in PATH
		mocks, toolsManager := setup(t)
		mocks.Shims.LookPath = func(name string) (string, error) {
			return "", fmt.Errorf("docker is not available in the PATH")
		}
		err := toolsManager.checkDocker()
		// Then an error indicating docker is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "Docker") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected docker not available error, got %v", err)
		}
	})

	t.Run("DockerVersionTooLow", func(t *testing.T) {
		// When docker version is below minimum required version
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		err := toolsManager.checkDocker()
		// Then an error indicating version is too low should be returned
		if err == nil || !strings.Contains(err.Error(), "Docker 1.0.0 is below the minimum required version") {
			t.Errorf("Expected docker version too low error, got %v", err)
		}
	})

	t.Run("DockerOnlySucceeds", func(t *testing.T) {
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "", fmt.Errorf("not found")
		}
		err := toolsManager.checkDocker()
		if err != nil {
			t.Errorf("Expected success with docker only (no compose required), got %v", err)
		}
	})

	t.Run("ColimaModeSkipsDaemonChecks", func(t *testing.T) {
		mocks, toolsManager := setup(t)
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		daemonCheckCalled := false
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				daemonCheckCalled = true
				return "", fmt.Errorf("Cannot connect to the Docker daemon")
			}
			return "", fmt.Errorf("command not found")
		}
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "", fmt.Errorf("not found")
		}
		err := toolsManager.checkDocker()
		if err != nil {
			t.Errorf("Expected checkDocker to succeed in Colima mode, but got error: %v", err)
		}
		if daemonCheckCalled {
			t.Errorf("Expected docker daemon check to be skipped in Colima mode, but it was called")
		}
	})
}

// Tests for Colima and Limactl version validation
func TestToolsManager_checkColima(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// When both colima and limactl are available with correct versions
		_, toolsManager := setup(t)
		err := toolsManager.checkColima()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected checkColima to succeed, but got error: %v", err)
		}
	})

	t.Run("ColimaNotAvailable", func(t *testing.T) {
		// When colima is not found in PATH
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "colima" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "limactl" && args[0] == "--version" {
				return "limactl version 1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		err := toolsManager.checkColima()
		// Then an error indicating colima is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "Colima") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected colima not available error, got %v", err)
		}
	})

	t.Run("InvalidColimaVersionResponse", func(t *testing.T) {
		// When colima version response is invalid
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "colima" && args[0] == "version" {
				return "Invalid version response", nil
			}
			if name == "limactl" && args[0] == "--version" {
				return "limactl version 1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		err := toolsManager.checkColima()
		// Then an error indicating version extraction failed should be returned
		if err == nil || !strings.Contains(err.Error(), "failed to extract colima version") {
			t.Errorf("Expected failed to extract colima version error, got %v", err)
		}
	})

	t.Run("ColimaVersionTooLow", func(t *testing.T) {
		// When colima version is below minimum required version
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "colima" && args[0] == "version" {
				return "Colima version 0.5.0", nil
			}
			if name == "limactl" && args[0] == "--version" {
				return "limactl version 1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		err := toolsManager.checkColima()
		// Then an error indicating version is too low should be returned
		if err == nil || !strings.Contains(err.Error(), "Colima 0.5.0 is below the minimum required version") {
			t.Errorf("Expected colima version too low error, got %v", err)
		}
	})

	t.Run("LimactlNotAvailable", func(t *testing.T) {
		// When limactl is not found in PATH
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "limactl" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "colima" && args[0] == "version" {
				return fmt.Sprintf("Colima version %s", constants.MinimumVersionColima), nil
			}
			return "", fmt.Errorf("command not found")
		}
		err := toolsManager.checkColima()
		// Then an error indicating limactl is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "Lima") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected limactl not available error, got %v", err)
		}
	})

	t.Run("InvalidLimactlVersionResponse", func(t *testing.T) {
		// When limactl version response is invalid
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "limactl" && args[0] == "--version" {
				return "Invalid version response", nil
			}
			if name == "colima" && args[0] == "version" {
				return fmt.Sprintf("Colima version %s", constants.MinimumVersionColima), nil
			}
			return "", fmt.Errorf("command not found")
		}
		err := toolsManager.checkColima()
		// Then an error indicating version extraction failed should be returned
		if err == nil || !strings.Contains(err.Error(), "failed to extract limactl version") {
			t.Errorf("Expected failed to extract limactl version error, got %v", err)
		}
	})

	t.Run("LimactlVersionTooLow", func(t *testing.T) {
		// When limactl version is below minimum required version
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "limactl" && args[0] == "--version" {
				return "Limactl version 0.5.0", nil
			}
			if name == "colima" && args[0] == "version" {
				return fmt.Sprintf("Colima version %s", constants.MinimumVersionColima), nil
			}
			return "", fmt.Errorf("command not found")
		}
		err := toolsManager.checkColima()
		// Then an error indicating version is too low should be returned
		if err == nil || !strings.Contains(err.Error(), "Lima 0.5.0 is below the minimum required version") {
			t.Errorf("Expected limactl version too low error, got %v", err)
		}
	})
}

// Tests for the x86-64-v2 microarchitecture baseline check (cli#1268)
func TestCheckX86V2Baseline(t *testing.T) {
	fullBaseline := x86V2Features{
		HasCX16: true, HasPOPCNT: true, HasSSE3: true, HasSSSE3: true, HasSSE41: true, HasSSE42: true,
	}

	t.Run("SucceedsWhenEveryFeaturePresent", func(t *testing.T) {
		if err := checkX86V2Baseline(fullBaseline); err != nil {
			t.Errorf("Expected no error for a full v2 baseline, got: %v", err)
		}
	})

	t.Run("NamesEachMissingFeature", func(t *testing.T) {
		f := fullBaseline
		f.HasPOPCNT = false
		f.HasSSE42 = false

		err := checkX86V2Baseline(f)

		if err == nil {
			t.Fatal("Expected an error when features are missing, got nil")
		}
		if !strings.Contains(err.Error(), "POPCNT") {
			t.Errorf("Expected error to name POPCNT, got: %v", err)
		}
		if !strings.Contains(err.Error(), "SSE4.2") {
			t.Errorf("Expected error to name SSE4.2, got: %v", err)
		}
		if strings.Contains(err.Error(), "CMPXCHG16B") {
			t.Errorf("Expected error not to name a feature that IS present, got: %v", err)
		}
	})

	t.Run("FailsWhenEveryFeatureMissing", func(t *testing.T) {
		err := checkX86V2Baseline(x86V2Features{})

		if err == nil {
			t.Fatal("Expected an error for a CPU with none of the baseline features, got nil")
		}
		for _, feature := range []string{"CMPXCHG16B", "POPCNT", "SSE3", "SSSE3", "SSE4.1", "SSE4.2"} {
			if !strings.Contains(err.Error(), feature) {
				t.Errorf("Expected error to name %s, got: %v", feature, err)
			}
		}
	})
}

func TestCheckCPUMicroarchitecture(t *testing.T) {
	t.Run("NoOpOnNonAMD64Architectures", func(t *testing.T) {
		// runtime.GOARCH is fixed for this test binary, so this only exercises the
		// gate directly when the test runner itself is non-amd64 (e.g. macOS/Linux
		// arm64 in CI); the feature-level logic is covered by TestCheckX86V2Baseline.
		if runtime.GOARCH == "amd64" {
			t.Skip("test binary is amd64; the non-amd64 no-op path isn't exercised here")
		}
		if err := checkCPUMicroarchitecture(); err != nil {
			t.Errorf("Expected no error on a non-amd64 host, got: %v", err)
		}
	})
}

// Tests for 1Password CLI version validation
func TestToolsManager_checkOnePassword(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given 1Password CLI is available with correct version
		_, toolsManager := setup(t)
		// When checking 1Password CLI version
		err := toolsManager.checkOnePassword()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected checkOnePassword to succeed, but got error: %v", err)
		}
	})

	t.Run("OnePasswordNotAvailable", func(t *testing.T) {
		// Given 1Password CLI is not found in PATH
		mocks, toolsManager := setup(t)
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "op" {
				return "", fmt.Errorf("1Password CLI is not available in the PATH")
			}
			return "/usr/bin/" + name, nil
		}
		// When checking 1Password CLI version
		err := toolsManager.checkOnePassword()
		// Then an error indicating CLI is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "1Password CLI") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected 1Password CLI not found error, got %v", err)
		}
	})

	t.Run("OnePasswordCommandError", func(t *testing.T) {
		// Given 1Password CLI command execution fails
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "op" && args[0] == "--version" {
				return "", fmt.Errorf("1Password CLI is not available in the PATH")
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking 1Password CLI version
		err := toolsManager.checkOnePassword()
		// Then an error indicating the --version invocation failed should be returned
		if err == nil || !strings.Contains(err.Error(), "1Password CLI --version failed") {
			t.Errorf("Expected 1Password CLI --version failed error, got %v", err)
		}
	})

	t.Run("OnePasswordVersionInvalidResponse", func(t *testing.T) {
		// Given 1Password CLI version response is invalid
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "op" && args[0] == "--version" {
				return "Invalid version response", nil
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking 1Password CLI version
		err := toolsManager.checkOnePassword()
		// Then an error indicating version extraction failed should be returned
		if err == nil || !strings.Contains(err.Error(), "failed to extract 1Password CLI version") {
			t.Errorf("Expected failed to extract 1Password CLI version error, got %v", err)
		}
	})
	t.Run("OnePasswordVersionTooLow", func(t *testing.T) {
		// Given 1Password CLI version is below minimum required
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "op" && args[0] == "--version" {
				return "1Password CLI 1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking 1Password CLI version
		err := toolsManager.checkOnePassword()
		// Then an error indicating version is too low should be returned
		if err == nil || !strings.Contains(err.Error(), "1Password CLI 1.0.0 is below the minimum required version") {
			t.Errorf("Expected 1Password CLI version too low error, got %v", err)
		}
	})
}

// Tests for SOPS CLI version validation
func TestToolsManager_checkSops(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given SOPS CLI is available with correct version
		_, toolsManager := setup(t)
		// When checking SOPS CLI version
		err := toolsManager.checkSops()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected checkSops to succeed, but got error: %v", err)
		}
	})

	t.Run("SopsNotAvailable", func(t *testing.T) {
		// Given SOPS CLI is not found in PATH
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "sops" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		// When checking SOPS CLI version
		err := toolsManager.checkSops()
		// Then an error indicating CLI is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "SOPS") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected SOPS not found error, got %v", err)
		}
	})

	t.Run("SopsCommandError", func(t *testing.T) {
		// Given SOPS CLI command execution fails
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "sops" && args[0] == "--version" {
				return "", fmt.Errorf("SOPS CLI is not available in the PATH")
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking SOPS CLI version
		err := toolsManager.checkSops()
		// Then an error indicating the --version invocation failed should be returned
		if err == nil || !strings.Contains(err.Error(), "sops --version failed") {
			t.Errorf("Expected sops --version failed error, got %v", err)
		}
	})

	t.Run("SopsVersionInvalidResponse", func(t *testing.T) {
		// Given SOPS CLI version response is invalid
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "sops" && args[0] == "--version" {
				return "Invalid version response", nil
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking SOPS CLI version
		err := toolsManager.checkSops()
		// Then an error indicating version extraction failed should be returned
		if err == nil || !strings.Contains(err.Error(), "failed to extract SOPS CLI version") {
			t.Errorf("Expected failed to extract SOPS CLI version error, got %v", err)
		}
	})

	t.Run("SopsVersionTooLow", func(t *testing.T) {
		// Given SOPS CLI version is below minimum required
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "sops" && args[0] == "--version" {
				return "sops 1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking SOPS CLI version
		err := toolsManager.checkSops()
		// Then an error indicating version is too low should be returned
		if err == nil || !strings.Contains(err.Error(), "SOPS 1.0.0 is below the minimum required version") {
			t.Errorf("Expected SOPS version too low error, got %v", err)
		}
	})
}

// Tests for kubelogin version validation
func TestToolsManager_checkKubelogin(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given kubelogin is available with correct version
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "kubelogin" {
				return "/usr/bin/kubelogin", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubelogin" && args[0] == "--version" {
				return fmt.Sprintf("kubelogin version %s", constants.MinimumVersionKubelogin), nil
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking kubelogin version
		err := toolsManager.checkKubelogin()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected checkKubelogin to succeed, but got error: %v", err)
		}
	})

	t.Run("KubeloginNotAvailable", func(t *testing.T) {
		// Given kubelogin is not found in PATH
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "kubelogin" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		// When checking kubelogin version
		err := toolsManager.checkKubelogin()
		// Then an error indicating kubelogin is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "kubelogin") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected kubelogin not available error, got %v", err)
		}
	})

	t.Run("KubeloginVersionInvalidResponse", func(t *testing.T) {
		// Given kubelogin version response is invalid
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "kubelogin" {
				return "/usr/bin/kubelogin", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubelogin" && args[0] == "--version" {
				return "Invalid version response", nil
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking kubelogin version
		err := toolsManager.checkKubelogin()
		// Then an error indicating version extraction failed should be returned
		if err == nil || !strings.Contains(err.Error(), "failed to extract kubelogin version") {
			t.Errorf("Expected failed to extract kubelogin version error, got %v", err)
		}
	})

	t.Run("KubeloginVersionTooLow", func(t *testing.T) {
		// Given kubelogin version is below minimum required version
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "kubelogin" {
				return "/usr/bin/kubelogin", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubelogin" && args[0] == "--version" {
				return "kubelogin version 0.1.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking kubelogin version
		err := toolsManager.checkKubelogin()
		// Then an error indicating version is too low should be returned
		if err == nil || !strings.Contains(err.Error(), "kubelogin 0.1.0 is below the minimum required version") {
			t.Errorf("Expected kubelogin version too low error, got %v", err)
		}
	})

	t.Run("KubeloginCommandError", func(t *testing.T) {
		// Given kubelogin command execution fails
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "kubelogin" {
				return "/usr/bin/kubelogin", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubelogin" && args[0] == "--version" {
				return "", fmt.Errorf("kubelogin is not available in the PATH")
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking kubelogin version
		err := toolsManager.checkKubelogin()
		// Then an error indicating the --version invocation failed should be returned
		if err == nil || !strings.Contains(err.Error(), "kubelogin --version failed") {
			t.Errorf("Expected kubelogin --version failed error, got %v", err)
		}
	})

	t.Run("AZURE_FEDERATED_TOKEN_FILESetButAZURE_CLIENT_IDMissing", func(t *testing.T) {
		// Given AZURE_FEDERATED_TOKEN_FILE is set but AZURE_CLIENT_ID is missing
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "kubelogin" {
				return "/usr/bin/kubelogin", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubelogin" && args[0] == "--version" {
				return fmt.Sprintf("kubelogin version %s", constants.MinimumVersionKubelogin), nil
			}
			return "", fmt.Errorf("command not found")
		}
		os.Setenv("AZURE_FEDERATED_TOKEN_FILE", "/path/to/token")
		defer os.Unsetenv("AZURE_FEDERATED_TOKEN_FILE")
		os.Unsetenv("AZURE_CLIENT_ID")
		os.Unsetenv("AZURE_TENANT_ID")
		// When checking kubelogin
		err := toolsManager.checkKubelogin()
		// Then an error indicating AZURE_CLIENT_ID is missing should be returned
		if err == nil || !strings.Contains(err.Error(), "AZURE_FEDERATED_TOKEN_FILE is set but AZURE_CLIENT_ID is missing") {
			t.Errorf("Expected AZURE_CLIENT_ID missing error, got %v", err)
		}
	})

	t.Run("AZURE_FEDERATED_TOKEN_FILESetButAZURE_TENANT_IDMissing", func(t *testing.T) {
		// Given AZURE_FEDERATED_TOKEN_FILE is set but AZURE_TENANT_ID is missing
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "kubelogin" {
				return "/usr/bin/kubelogin", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubelogin" && args[0] == "--version" {
				return fmt.Sprintf("kubelogin version %s", constants.MinimumVersionKubelogin), nil
			}
			return "", fmt.Errorf("command not found")
		}
		os.Setenv("AZURE_FEDERATED_TOKEN_FILE", "/path/to/token")
		os.Setenv("AZURE_CLIENT_ID", "test-client-id")
		defer func() {
			os.Unsetenv("AZURE_FEDERATED_TOKEN_FILE")
			os.Unsetenv("AZURE_CLIENT_ID")
		}()
		os.Unsetenv("AZURE_TENANT_ID")
		// When checking kubelogin
		err := toolsManager.checkKubelogin()
		// Then an error indicating AZURE_TENANT_ID is missing should be returned
		if err == nil || !strings.Contains(err.Error(), "AZURE_FEDERATED_TOKEN_FILE is set but AZURE_TENANT_ID is missing") {
			t.Errorf("Expected AZURE_TENANT_ID missing error, got %v", err)
		}
	})

	t.Run("AZURE_CLIENT_SECRETSetButAZURE_CLIENT_IDMissing", func(t *testing.T) {
		// Given AZURE_CLIENT_SECRET is set but AZURE_CLIENT_ID is missing
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "kubelogin" {
				return "/usr/bin/kubelogin", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubelogin" && args[0] == "--version" {
				return fmt.Sprintf("kubelogin version %s", constants.MinimumVersionKubelogin), nil
			}
			return "", fmt.Errorf("command not found")
		}
		os.Setenv("AZURE_CLIENT_SECRET", "test-secret")
		defer os.Unsetenv("AZURE_CLIENT_SECRET")
		os.Unsetenv("AZURE_CLIENT_ID")
		os.Unsetenv("AZURE_TENANT_ID")
		// When checking kubelogin
		err := toolsManager.checkKubelogin()
		// Then an error indicating AZURE_CLIENT_ID is missing should be returned
		if err == nil || !strings.Contains(err.Error(), "AZURE_CLIENT_SECRET is set but AZURE_CLIENT_ID is missing") {
			t.Errorf("Expected AZURE_CLIENT_ID missing error, got %v", err)
		}
	})

	t.Run("AZURE_CLIENT_SECRETSetButAZURE_TENANT_IDMissing", func(t *testing.T) {
		// Given AZURE_CLIENT_SECRET is set but AZURE_TENANT_ID is missing
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "kubelogin" {
				return "/usr/bin/kubelogin", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubelogin" && args[0] == "--version" {
				return fmt.Sprintf("kubelogin version %s", constants.MinimumVersionKubelogin), nil
			}
			return "", fmt.Errorf("command not found")
		}
		os.Setenv("AZURE_CLIENT_SECRET", "test-secret")
		os.Setenv("AZURE_CLIENT_ID", "test-client-id")
		defer func() {
			os.Unsetenv("AZURE_CLIENT_SECRET")
			os.Unsetenv("AZURE_CLIENT_ID")
		}()
		os.Unsetenv("AZURE_TENANT_ID")
		// When checking kubelogin
		err := toolsManager.checkKubelogin()
		// Then an error indicating AZURE_TENANT_ID is missing should be returned
		if err == nil || !strings.Contains(err.Error(), "AZURE_CLIENT_SECRET is set but AZURE_TENANT_ID is missing") {
			t.Errorf("Expected AZURE_TENANT_ID missing error, got %v", err)
		}
	})

	t.Run("AZURE_CLIENT_SECRETSetWithAllRequiredVars", func(t *testing.T) {
		// Given AZURE_CLIENT_SECRET is set with all required environment variables
		mocks, toolsManager := setup(t)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "kubelogin" {
				return "/usr/bin/kubelogin", nil
			}
			return originalExecLookPath(name)
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubelogin" && args[0] == "--version" {
				return fmt.Sprintf("kubelogin version %s", constants.MinimumVersionKubelogin), nil
			}
			return "", fmt.Errorf("command not found")
		}
		os.Setenv("AZURE_CLIENT_SECRET", "test-secret")
		os.Setenv("AZURE_CLIENT_ID", "test-client-id")
		os.Setenv("AZURE_TENANT_ID", "test-tenant-id")
		defer func() {
			os.Unsetenv("AZURE_CLIENT_SECRET")
			os.Unsetenv("AZURE_CLIENT_ID")
			os.Unsetenv("AZURE_TENANT_ID")
		}()
		// When checking kubelogin
		err := toolsManager.checkKubelogin()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected checkKubelogin to succeed with all required env vars, but got error: %v", err)
		}
	})
}

// Tests for version comparison logic
func Test_compareVersion(t *testing.T) {
	tests := []struct {
		name     string
		version1 string
		version2 string
		expected int
	}{
		{"EqualVersions", "1.0.0", "1.0.0", 0},
		{"Version1Greater", "1.2.0", "1.1.9", 1},
		{"Version2Greater", "1.0.0", "1.0.1", -1},
		{"Version1GreaterWithMoreComponents", "1.0.0.1", "1.0.0", 1},
		{"Version2GreaterWithMoreComponents", "1.0.0", "1.0.0.1", -1},
		{"Version1WithPreRelease", "1.0.0-alpha", "1.0.0", -1},
		{"Version2WithPreRelease", "1.0.0", "1.0.0-beta", 1},
		{"Version1WithNonNumeric", "1.0.0-alpha", "1.0.0-beta", -1},
		{"Version2WithNonNumeric", "1.0.0-beta", "1.0.0-alpha", 1},
		{"Version1WithDifferentLength", "1.0", "1.0.0", -1},
		{"Version2WithDifferentLength", "1.0.0", "1.0", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given two version strings
			// When comparing versions
			result := compareVersion(tt.version1, tt.version2)
			// Then the comparison should match expected result
			if result != tt.expected {
				t.Errorf("compareVersion(%s, %s) = %d; want %d", tt.version1, tt.version2, result, tt.expected)
			}
		})
	}
}

// Tests for version string extraction
func Test_extractVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"SimpleVersion", "Docker version 25.0.0", "25.0.0"},
		{"VersionWithPrefix", "Client Version: v1.32.0", "1.32.0"},
		{"VersionWithText", "Terraform v1.7.0", "1.7.0"},
		{"VersionWithMultipleNumbers", "1Password CLI 2.25.0", "2.25.0"},
		{"VersionWithColima", fmt.Sprintf("Colima version %s", constants.MinimumVersionColima), constants.MinimumVersionColima},
		{"VersionWithLima", "limactl version 1.0.0", "1.0.0"},
		{"NoVersion", "Invalid version response", ""},
		{"EmptyString", "", ""},
		{"MultipleVersions", "Version 1.0.0 and 2.0.0", "1.0.0"},
		{"VersionWithExtraText", "Some text 1.2.3 more text", "1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a version string
			// When extracting version
			result := extractVersion(tt.input)
			// Then the extracted version should match expected
			if result != tt.expected {
				t.Errorf("extractVersion(%s) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}
