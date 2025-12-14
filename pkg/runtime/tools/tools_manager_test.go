package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime/config"
	sh "github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	ConfigHandler config.ConfigHandler
	Shell         *sh.MockShell
}

type SetupOptions struct {
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

var defaultConfig = `
contexts:
  test:
    docker:
      enabled: true
    cluster:
      enabled: true
`

// Global test setup helper that creates a temporary directory and mocks
// This is used by most test functions to establish a clean test environment
func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Store original working directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Create temp dir using testing.TempDir()
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	options := &SetupOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}

	shell := sh.NewMockShell()
	shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
		switch {
		case name == "docker" && len(args) >= 2 && args[0] == "version" && args[1] == "--format":
			return fmt.Sprintf("%s", constants.MinimumVersionDocker), nil
		case name == "docker" && args[0] == "version":
			return fmt.Sprintf("Docker version %s", constants.MinimumVersionDocker), nil
		case name == "docker" && args[0] == "compose" && args[1] == "version":
			return fmt.Sprintf("Docker Compose version %s", constants.MinimumVersionDockerCompose), nil
		case name == "docker-compose" && args[0] == "version":
			return fmt.Sprintf("docker-compose version %s", constants.MinimumVersionDockerCompose), nil
		case name == "colima" && args[0] == "version":
			return fmt.Sprintf("colima version %s", constants.MinimumVersionColima), nil
		case name == "limactl" && args[0] == "--version":
			return fmt.Sprintf("limactl version %s", constants.MinimumVersionLima), nil
		case name == "talosctl" && args[0] == "version" && args[1] == "--client" && args[2] == "--short":
			return fmt.Sprintf("v%s", constants.MinimumVersionTalosctl), nil
		case name == "terraform" && args[0] == "version":
			return fmt.Sprintf("Terraform v%s", constants.MinimumVersionTerraform), nil
		case name == "op" && args[0] == "--version":
			return fmt.Sprintf("1Password CLI %s", constants.MinimumVersion1Password), nil
		}
		return "", fmt.Errorf("command not found")
	}

	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewConfigHandler(shell)
	} else {
		configHandler = options.ConfigHandler
	}

	configHandler.SetContext("test")

	if err := configHandler.LoadConfigString(defaultConfig); err != nil {
		t.Fatalf("Failed to load default config: %v", err)
	}
	if options.ConfigStr != "" {
		if err := configHandler.LoadConfigString(options.ConfigStr); err != nil {
			t.Fatalf("Failed to load options config: %v", err)
		}
	}

	originalExecLookPath := execLookPath
	originalOsStat := osStat

	execLookPath = func(name string) (string, error) {
		switch name {
		case "docker", "docker-compose", "docker-cli-plugin-docker-compose", "talosctl", "terraform", "op", "colima", "limactl":
			return "/usr/bin/" + name, nil
		default:
			return "", exec.ErrNotFound
		}
	}

	osStat = func(name string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}

	t.Cleanup(func() {
		execLookPath = originalExecLookPath
		osStat = originalOsStat

		os.Unsetenv("WINDSOR_PROJECT_ROOT")

		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	return &Mocks{
		Shell:         shell,
		ConfigHandler: configHandler,
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

// Tests for core ToolsManager functionality
func TestToolsManager_NewToolsManager(t *testing.T) {
	setup := func(t *testing.T) *Mocks {
		t.Helper()
		return setupMocks(t)
	}

	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setup(t)
		// When creating a new tools manager
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		// Then the tools manager should be created successfully
		if toolsManager == nil {
			t.Errorf("Expected tools manager to be non-nil")
		}
	})
}

// Tests for initialization process
func TestToolsManager_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given a tools manager with mock dependencies
		_, toolsManager := setup(t)
		// Then it should be created
		if toolsManager == nil {
			t.Error("Expected tools manager to be created")
		}
	})
}

// Tests for manifest writing functionality
func TestToolsManager_WriteManifest(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		mocks := setupMocks(t, &SetupOptions{ConfigStr: ""})
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given an initialized tools manager with empty config
		_, toolsManager := setup(t)
		// When writing the tools manifest
		err := toolsManager.WriteManifest()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected WriteManifest to return error: nil, but got: %v", err)
		}
	})
}

// Tests for installation process
func TestToolsManager_Install(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given an initialized tools manager
		_, toolsManager := setup(t)
		// When installing required tools
		err := toolsManager.Install()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected Install to succeed, but got error: %v", err)
		}
	})
}

// Tests for the main Check functionality that validates tool versions
func TestToolsManager_Check(t *testing.T) {
	setup := func(t *testing.T, configStr string) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t, &SetupOptions{ConfigStr: configStr})
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// When all tools are enabled and available with correct versions
		mocks, toolsManager := setup(t, defaultConfig)
		// Given all tools are available with correct versions
		toolVersions := map[string][]string{
			"docker":         {"version", "--format"},
			"docker-compose": {"version"},
			"colima":         {"version"},
			"limactl":        {"--version"},
			"talosctl":       {"version", "--client", "--short"},
			"terraform":      {"version"},
			"op":             {"--version"},
		}
		// When checking tool versions
		err := toolsManager.Check()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected Check to succeed, but got error: %v", err)
		}
		// And all tool versions should be validated
		for tool, args := range toolVersions {
			output, err := mocks.Shell.ExecSilent(tool, args...)
			if err != nil {
				t.Errorf("Failed to get %s version: %v", tool, err)
				continue
			}
			if !strings.Contains(output, constants.MinimumVersionDocker) &&
				!strings.Contains(output, constants.MinimumVersionDockerCompose) &&
				!strings.Contains(output, constants.MinimumVersionColima) &&
				!strings.Contains(output, constants.MinimumVersionLima) &&
				!strings.Contains(output, constants.MinimumVersionTalosctl) &&
				!strings.Contains(output, constants.MinimumVersionTerraform) &&
				!strings.Contains(output, constants.MinimumVersion1Password) {
				t.Errorf("Expected %s version check to pass, got output: %s", tool, output)
			}
		}
	})

	t.Run("DockerDisabled", func(t *testing.T) {
		// When docker is disabled in config
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("docker.enabled", false)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" || name == "docker-compose" || name == "docker-cli-plugin-docker-compose" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		err := toolsManager.Check()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected Check to succeed when docker is disabled, but got error: %v", err)
		}
	})

	t.Run("AllToolsDisabled", func(t *testing.T) {
		// When all tools are disabled in config
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("docker.enabled", false)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" || name == "docker-compose" || name == "docker-cli-plugin-docker-compose" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		err := toolsManager.Check()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected Check to succeed when all tools are disabled, but got error: %v", err)
		}
	})

	t.Run("DockerEnabledButNotAvailable", func(t *testing.T) {
		// When docker is enabled but not available in PATH
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("docker.enabled", true)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" || name == "docker-compose" || name == "docker-cli-plugin-docker-compose" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		err := toolsManager.Check()
		// Then an error indicating docker check failed should be returned
		if err == nil || !strings.Contains(err.Error(), "docker check failed") {
			t.Errorf("Expected Check to fail when docker is enabled but not available, but got: %v", err)
		}
	})

	t.Run("TerraformEnabledButNotAvailable", func(t *testing.T) {
		// When terraform is enabled but not available in PATH
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("terraform.enabled", true)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		err := toolsManager.Check()
		// Then an error indicating terraform check failed should be returned
		if err == nil || !strings.Contains(err.Error(), "terraform check failed") {
			t.Errorf("Expected Check to fail when terraform is enabled but not available, but got: %v", err)
		}
	})

	t.Run("ColimaEnabledButNotAvailable", func(t *testing.T) {
		// When colima is enabled but not available in PATH
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("vm.driver", "colima")
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "colima" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		err := toolsManager.Check()
		// Then an error indicating colima check failed should be returned
		if err == nil || !strings.Contains(err.Error(), "colima check failed") {
			t.Errorf("Expected Check to fail when colima is enabled but not available, but got: %v", err)
		}
	})

	t.Run("OnePasswordEnabledButNotAvailable", func(t *testing.T) {
		// When 1Password is enabled but not available in PATH
		configStr := `
contexts:
  test:
    secrets:
      onepassword:
        vaults:
          test1:
            name: Test1
            url: test.1password.com
          test2:
            name: Test2
            url: test.1password.com
`
		mocks, toolsManager := setup(t, configStr)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "op" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "op" {
				return "", fmt.Errorf("1Password CLI is not available in the PATH")
			}
			return originalExecSilent(name, args...)
		}
		err := toolsManager.Check()
		// Then an error indicating 1Password check failed should be returned
		if err == nil {
			t.Error("Expected error when 1Password is enabled but not available")
		} else if !strings.Contains(err.Error(), "1password check failed: 1Password CLI is not available in the PATH") {
			t.Errorf("Expected error to contain '1password check failed: 1Password CLI is not available in the PATH', got: %v", err)
		}
	})

	t.Run("MultipleToolFailures", func(t *testing.T) {
		// Given multiple tools are enabled but fail checks
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("docker.enabled", true)

		// Mock failures for multiple tools
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" {
				return "", fmt.Errorf("%s is not available in the PATH", name)
			}
			return originalExecLookPath(name)
		}

		// When checking tool versions
		err := toolsManager.Check()

		// Then an error should be returned for the first failing tool
		if err == nil {
			t.Error("Expected error when multiple tools fail checks")
		} else if !strings.Contains(err.Error(), "docker check failed") {
			t.Errorf("Expected error to contain 'docker check failed', got: %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

// Tests for Docker and Docker Compose version validation
func TestToolsManager_checkDocker(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
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
		_, toolsManager := setup(t)
		execLookPath = func(name string) (string, error) {
			return "", fmt.Errorf("docker is not available in the PATH")
		}
		err := toolsManager.checkDocker()
		// Then an error indicating docker is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "docker is not available in the PATH") {
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
		if err == nil || !strings.Contains(err.Error(), "docker version 1.0.0 is below the minimum required version") {
			t.Errorf("Expected docker version too low error, got %v", err)
		}
	})

	t.Run("DockerComposeVersionThroughDockerCompose", func(t *testing.T) {
		// When docker compose is available as a standalone command
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			if name == "docker" && args[0] == "compose" {
				return "", fmt.Errorf("command not found")
			}
			if name == "docker-compose" && args[0] == "version" {
				return "docker-compose version 2.25.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		err := toolsManager.checkDocker()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected success with docker-compose version check, got %v", err)
		}
	})

	t.Run("DockerComposeVersionTooLow", func(t *testing.T) {
		// When docker compose version is below minimum required version
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			if name == "docker" && args[0] == "compose" {
				return "Docker Compose version 1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		err := toolsManager.checkDocker()
		// Then an error indicating version is too low should be returned
		if err == nil || !strings.Contains(err.Error(), "docker-compose version 1.0.0 is below the minimum required version") {
			t.Errorf("Expected docker-compose version too low error, got %v", err)
		}
	})

	t.Run("DockerComposePluginFallback", func(t *testing.T) {
		// When docker compose is available as a plugin
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		execLookPath = func(name string) (string, error) {
			if name == "docker" || name == "docker-cli-plugin-docker-compose" {
				return "/usr/bin/" + name, nil
			}
			return "", fmt.Errorf("not found")
		}
		err := toolsManager.checkDocker()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected success with docker-cli-plugin-docker-compose fallback, got %v", err)
		}
	})

	t.Run("DockerComposeNotAvailable", func(t *testing.T) {
		// When neither docker compose nor its plugin are available
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		execLookPath = func(name string) (string, error) {
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "", fmt.Errorf("not found")
		}
		err := toolsManager.checkDocker()
		// Then an error indicating docker-compose is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "docker-compose is not available in the PATH") {
			t.Errorf("Expected docker-compose not available error, got %v", err)
		}
	})
}

// Tests for Colima and Limactl version validation
func TestToolsManager_checkColima(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		if err == nil || !strings.Contains(err.Error(), "colima is not available in the PATH") {
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
		if err == nil || !strings.Contains(err.Error(), "colima version 0.5.0 is below the minimum required version") {
			t.Errorf("Expected colima version too low error, got %v", err)
		}
	})

	t.Run("LimactlNotAvailable", func(t *testing.T) {
		// When limactl is not found in PATH
		mocks, toolsManager := setup(t)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		if err == nil || !strings.Contains(err.Error(), "limactl is not available in the PATH") {
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
		if err == nil || !strings.Contains(err.Error(), "limactl version 0.5.0 is below the minimum required version") {
			t.Errorf("Expected limactl version too low error, got %v", err)
		}
	})
}

// Tests for Terraform version validation
func TestToolsManager_checkTerraform(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given terraform is available with correct version
		_, toolsManager := setup(t)
		// When checking terraform version
		err := toolsManager.checkTerraform()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected checkTerraform to succeed, but got error: %v", err)
		}
	})

	t.Run("TerraformNotAvailable", func(t *testing.T) {
		// Given neither terraform nor tofu is found in PATH
		_, toolsManager := setup(t)
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			if name == "terraform" || name == "tofu" {
				return "", fmt.Errorf("%s is not available in the PATH", name)
			}
			return "/usr/bin/" + name, nil
		}
		// When checking terraform version
		err := toolsManager.checkTerraform()
		// Then an error indicating terraform or tofu is not available should be returned
		if err == nil || (!strings.Contains(err.Error(), "terraform is not available in the PATH") && !strings.Contains(err.Error(), "tofu is not available in the PATH")) {
			t.Errorf("Expected terraform or tofu not available error, got %v", err)
		}
	})

	t.Run("TerraformVersionInvalidResponse", func(t *testing.T) {
		// Given terraform version response is invalid
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "terraform" && args[0] == "version" {
				return "Invalid version response", nil
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking terraform version
		err := toolsManager.checkTerraform()
		// Then an error indicating version extraction failed should be returned
		if err == nil || !strings.Contains(err.Error(), "failed to extract terraform version") {
			t.Errorf("Expected failed to extract terraform version error, got %v", err)
		}
	})

	t.Run("TerraformVersionTooLow", func(t *testing.T) {
		// Given terraform version is below minimum required version
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "terraform" && args[0] == "version" {
				return "Terraform v0.1.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking terraform version
		err := toolsManager.checkTerraform()
		// Then an error indicating version is too low should be returned
		if err == nil || !strings.Contains(err.Error(), "terraform version 0.1.0 is below the minimum required version") {
			t.Errorf("Expected terraform version too low error, got %v", err)
		}
	})
}

// Tests for 1Password CLI version validation
func TestToolsManager_checkOnePassword(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
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
		_, toolsManager := setup(t)
		execLookPath = func(name string) (string, error) {
			if name == "op" {
				return "", fmt.Errorf("1Password CLI is not available in the PATH")
			}
			return "/usr/bin/" + name, nil
		}
		// When checking 1Password CLI version
		err := toolsManager.checkOnePassword()
		// Then an error indicating CLI is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "1Password CLI is not available in the PATH") {
			t.Errorf("Expected 1Password CLI is not available in the PATH error, got %v", err)
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
		// Then an error indicating CLI is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "1Password CLI is not available in the PATH") {
			t.Errorf("Expected 1Password CLI is not available in the PATH error, got %v", err)
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
		if err == nil || !strings.Contains(err.Error(), "1Password CLI version 1.0.0 is below the minimum required version") {
			t.Errorf("Expected 1Password CLI version too low error, got %v", err)
		}
	})
}

// Tests for kubelogin version validation
func TestToolsManager_checkKubelogin(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given kubelogin is available with correct version
		mocks, toolsManager := setup(t)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		_, toolsManager := setup(t)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "kubelogin" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		// When checking kubelogin version
		err := toolsManager.checkKubelogin()
		// Then an error indicating kubelogin is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "kubelogin is not available in the PATH") {
			t.Errorf("Expected kubelogin not available error, got %v", err)
		}
	})

	t.Run("KubeloginVersionInvalidResponse", func(t *testing.T) {
		// Given kubelogin version response is invalid
		mocks, toolsManager := setup(t)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		if err == nil || !strings.Contains(err.Error(), "kubelogin version 0.1.0 is below the minimum required version") {
			t.Errorf("Expected kubelogin version too low error, got %v", err)
		}
	})

	t.Run("KubeloginCommandError", func(t *testing.T) {
		// Given kubelogin command execution fails
		mocks, toolsManager := setup(t)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		// Then an error indicating kubelogin is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "kubelogin is not available in the PATH") {
			t.Errorf("Expected kubelogin is not available in the PATH error, got %v", err)
		}
	})

	t.Run("AZURE_FEDERATED_TOKEN_FILESetButAZURE_CLIENT_IDMissing", func(t *testing.T) {
		// Given AZURE_FEDERATED_TOKEN_FILE is set but AZURE_CLIENT_ID is missing
		mocks, toolsManager := setup(t)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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

// =============================================================================
// Test Helpers
// =============================================================================

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

// Tests for GetTerraformCommand functionality
func TestToolsManager_GetTerraformCommand(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		return mocks, toolsManager
	}

	t.Run("ReturnsTerraformWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a tools manager with nil config handler
		shell := sh.NewMockShell()
		toolsManager := NewToolsManager(nil, shell)
		// When GetTerraformCommand is called
		command := toolsManager.GetTerraformCommand()
		// Then it should return "terraform"
		if command != "terraform" {
			t.Errorf("Expected 'terraform', got %s", command)
		}
	})

	t.Run("ReturnsTofuWhenDriverIsOpentofu", func(t *testing.T) {
		// Given a tools manager with opentofu driver configured
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`terraform:
  driver: opentofu
`), 0644); err != nil {
			t.Fatalf("Failed to write windsor.yaml: %v", err)
		}
		// When GetTerraformCommand is called
		command := toolsManager.GetTerraformCommand()
		// Then it should return "tofu"
		if command != "tofu" {
			t.Errorf("Expected 'tofu', got %s", command)
		}
	})

	t.Run("ReturnsTerraformWhenDriverIsTerraform", func(t *testing.T) {
		// Given a tools manager with terraform driver configured
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`terraform:
  driver: terraform
`), 0644); err != nil {
			t.Fatalf("Failed to write windsor.yaml: %v", err)
		}
		// When GetTerraformCommand is called
		command := toolsManager.GetTerraformCommand()
		// Then it should return "terraform"
		if command != "terraform" {
			t.Errorf("Expected 'terraform', got %s", command)
		}
	})

	t.Run("ReturnsTerraformWhenDriverNotConfigured", func(t *testing.T) {
		// Given a tools manager with no driver configured
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`contexts:
  test: {}
`), 0644); err != nil {
			t.Fatalf("Failed to write windsor.yaml: %v", err)
		}
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "/usr/bin/terraform", nil
			}
			return "", exec.ErrNotFound
		}
		// When GetTerraformCommand is called
		command := toolsManager.GetTerraformCommand()
		// Then it should return "terraform" (detected)
		if command != "terraform" {
			t.Errorf("Expected 'terraform', got %s", command)
		}
	})
}

// Tests for getTerraformDriver functionality
func TestToolsManager_getTerraformDriver(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		return mocks, toolsManager
	}

	t.Run("FallsBackToDetectionWhenShellIsNil", func(t *testing.T) {
		// Given a tools manager with nil shell
		toolsManager := NewToolsManager(nil, nil)
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "/usr/bin/terraform", nil
			}
			return "", exec.ErrNotFound
		}
		// When getTerraformDriver is called
		driver := toolsManager.getTerraformDriver()
		// Then it should return "terraform" (from detection)
		if driver != "terraform" {
			t.Errorf("Expected 'terraform', got %s", driver)
		}
	})

	t.Run("FallsBackToDetectionWhenGetProjectRootFails", func(t *testing.T) {
		// Given a tools manager with GetProjectRoot error
		mocks, toolsManager := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("failed to get project root")
		}
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "/usr/bin/terraform", nil
			}
			return "", exec.ErrNotFound
		}
		// When getTerraformDriver is called
		driver := toolsManager.getTerraformDriver()
		// Then it should return "terraform" (from detection)
		if driver != "terraform" {
			t.Errorf("Expected 'terraform', got %s", driver)
		}
	})

	t.Run("FallsBackToDetectionWhenWindsorYamlNotExists", func(t *testing.T) {
		// Given a tools manager with no windsor.yaml
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "/usr/bin/terraform", nil
			}
			return "", exec.ErrNotFound
		}
		// When getTerraformDriver is called
		driver := toolsManager.getTerraformDriver()
		// Then it should return "terraform" (from detection)
		if driver != "terraform" {
			t.Errorf("Expected 'terraform', got %s", driver)
		}
	})

	t.Run("FallsBackToDetectionWhenWindsorYamlReadFails", func(t *testing.T) {
		// Given a tools manager with unreadable windsor.yaml
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`test`), 0644); err != nil {
			t.Fatalf("Failed to write windsor.yaml: %v", err)
		}
		if runtime.GOOS != "windows" {
			if err := os.Chmod(windsorYaml, 0000); err != nil {
				t.Fatalf("Failed to chmod windsor.yaml: %v", err)
			}
			defer os.Chmod(windsorYaml, 0644)
		}
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "/usr/bin/terraform", nil
			}
			return "", exec.ErrNotFound
		}
		// When getTerraformDriver is called
		driver := toolsManager.getTerraformDriver()
		// Then it should return "terraform" (from detection)
		if driver != "terraform" {
			t.Errorf("Expected 'terraform', got %s", driver)
		}
	})

	t.Run("FallsBackToDetectionWhenYamlIsInvalid", func(t *testing.T) {
		// Given a tools manager with invalid YAML
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`invalid: yaml: [content`), 0644); err != nil {
			t.Fatalf("Failed to write windsor.yaml: %v", err)
		}
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "/usr/bin/terraform", nil
			}
			return "", exec.ErrNotFound
		}
		// When getTerraformDriver is called
		driver := toolsManager.getTerraformDriver()
		// Then it should return "terraform" (from detection)
		if driver != "terraform" {
			t.Errorf("Expected 'terraform', got %s", driver)
		}
	})

	t.Run("ReturnsOpentofuWhenDriverIsConfigured", func(t *testing.T) {
		// Given a tools manager with opentofu driver configured
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`terraform:
  driver: opentofu
`), 0644); err != nil {
			t.Fatalf("Failed to write windsor.yaml: %v", err)
		}
		// When getTerraformDriver is called
		driver := toolsManager.getTerraformDriver()
		// Then it should return "opentofu"
		if driver != "opentofu" {
			t.Errorf("Expected 'opentofu', got %s", driver)
		}
	})

	t.Run("ReturnsTerraformWhenDriverIsConfigured", func(t *testing.T) {
		// Given a tools manager with terraform driver configured
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`terraform:
  driver: terraform
`), 0644); err != nil {
			t.Fatalf("Failed to write windsor.yaml: %v", err)
		}
		// When getTerraformDriver is called
		driver := toolsManager.getTerraformDriver()
		// Then it should return "terraform"
		if driver != "terraform" {
			t.Errorf("Expected 'terraform', got %s", driver)
		}
	})

	t.Run("FallsBackToDetectionWhenDriverIsEmpty", func(t *testing.T) {
		// Given a tools manager with empty driver
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`terraform:
  driver: ""
`), 0644); err != nil {
			t.Fatalf("Failed to write windsor.yaml: %v", err)
		}
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "/usr/bin/terraform", nil
			}
			return "", exec.ErrNotFound
		}
		// When getTerraformDriver is called
		driver := toolsManager.getTerraformDriver()
		// Then it should return "terraform" (from detection)
		if driver != "terraform" {
			t.Errorf("Expected 'terraform', got %s", driver)
		}
	})

	t.Run("FallsBackToDetectionWhenTerraformSectionMissing", func(t *testing.T) {
		// Given a tools manager with no terraform section
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`contexts:
  test: {}
`), 0644); err != nil {
			t.Fatalf("Failed to write windsor.yaml: %v", err)
		}
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "/usr/bin/terraform", nil
			}
			return "", exec.ErrNotFound
		}
		// When getTerraformDriver is called
		driver := toolsManager.getTerraformDriver()
		// Then it should return "terraform" (from detection)
		if driver != "terraform" {
			t.Errorf("Expected 'terraform', got %s", driver)
		}
	})
}

// Tests for detectTerraformDriver functionality
func TestToolsManager_detectTerraformDriver(t *testing.T) {
	setup := func(t *testing.T) *BaseToolsManager {
		t.Helper()
		mocks := setupMocks(t)
		return NewToolsManager(mocks.ConfigHandler, mocks.Shell)
	}

	t.Run("ReturnsTerraformWhenTerraformAvailable", func(t *testing.T) {
		// Given terraform is available in PATH
		toolsManager := setup(t)
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "/usr/bin/terraform", nil
			}
			return "", exec.ErrNotFound
		}
		// When detectTerraformDriver is called
		driver := toolsManager.detectTerraformDriver()
		// Then it should return "terraform"
		if driver != "terraform" {
			t.Errorf("Expected 'terraform', got %s", driver)
		}
	})

	t.Run("ReturnsOpentofuWhenOnlyTofuAvailable", func(t *testing.T) {
		// Given only tofu is available in PATH
		toolsManager := setup(t)
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			if name == "tofu" {
				return "/usr/bin/tofu", nil
			}
			return "", exec.ErrNotFound
		}
		// When detectTerraformDriver is called
		driver := toolsManager.detectTerraformDriver()
		// Then it should return "opentofu"
		if driver != "opentofu" {
			t.Errorf("Expected 'opentofu', got %s", driver)
		}
	})

	t.Run("ReturnsTerraformWhenNeitherAvailable", func(t *testing.T) {
		// Given neither terraform nor tofu is available
		toolsManager := setup(t)
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}
		// When detectTerraformDriver is called
		driver := toolsManager.detectTerraformDriver()
		// Then it should return "terraform" (default)
		if driver != "terraform" {
			t.Errorf("Expected 'terraform', got %s", driver)
		}
	})

	t.Run("PrefersTerraformOverTofu", func(t *testing.T) {
		// Given both terraform and tofu are available
		toolsManager := setup(t)
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
			if name == "terraform" || name == "tofu" {
				return "/usr/bin/" + name, nil
			}
			return "", exec.ErrNotFound
		}
		// When detectTerraformDriver is called
		driver := toolsManager.detectTerraformDriver()
		// Then it should return "terraform" (preferred)
		if driver != "terraform" {
			t.Errorf("Expected 'terraform', got %s", driver)
		}
	})
}
