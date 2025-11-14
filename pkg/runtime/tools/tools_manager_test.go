package tools

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	sh "github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	Shell         *sh.MockShell
}

type SetupOptions struct {
	Injector      di.Injector
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

	var injector di.Injector
	if options.Injector == nil {
		injector = di.NewInjector()
	} else {
		injector = options.Injector
	}

	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewConfigHandler(injector)
	} else {
		configHandler = options.ConfigHandler
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
		case name == "kubectl" && args[0] == "version" && args[1] == "--client":
			return fmt.Sprintf("Client Version: v%s", constants.MinimumVersionKubectl), nil
		case name == "talosctl" && args[0] == "version" && args[1] == "--client" && args[2] == "--short":
			return fmt.Sprintf("v%s", constants.MinimumVersionTalosctl), nil
		case name == "terraform" && args[0] == "version":
			return fmt.Sprintf("Terraform v%s", constants.MinimumVersionTerraform), nil
		case name == "op" && args[0] == "--version":
			return fmt.Sprintf("1Password CLI %s", constants.MinimumVersion1Password), nil
		case name == "aws" && args[0] == "--version":
			return fmt.Sprintf("aws-cli/%s Python/3.13.3 Darwin/24.0.0 exe/x86_64", constants.MinimumVersionAWSCLI), nil
		}
		return "", fmt.Errorf("command not found")
	}

	injector.Register("configHandler", configHandler)
	injector.Register("shell", shell)

	configHandler.Initialize()
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
		case "docker", "docker-compose", "docker-cli-plugin-docker-compose", "kubectl", "talosctl", "terraform", "op", "colima", "limactl":
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
		Injector:      injector,
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
		toolsManager := NewToolsManager(mocks.Injector)
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
		toolsManager := NewToolsManager(mocks.Injector)
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given a tools manager with mock dependencies
		_, toolsManager := setup(t)
		// When initializing the tools manager
		err := toolsManager.Initialize()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected Initialize to succeed, but got error: %v", err)
		}
	})
}

// Tests for manifest writing functionality
func TestToolsManager_WriteManifest(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		mocks := setupMocks(t, &SetupOptions{ConfigStr: ""})
		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()
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
		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()
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
		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()
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
			"kubectl":        {"version", "--client"},
			"talosctl":       {"version", "--client", "--short"},
			"terraform":      {"version"},
			"op":             {"--version"},
			"aws":            {"--version"},
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
				!strings.Contains(output, constants.MinimumVersionKubectl) &&
				!strings.Contains(output, constants.MinimumVersionTalosctl) &&
				!strings.Contains(output, constants.MinimumVersionTerraform) &&
				!strings.Contains(output, constants.MinimumVersion1Password) &&
				!strings.Contains(output, constants.MinimumVersionAWSCLI) {
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

	t.Run("ClusterDisabled", func(t *testing.T) {
		// When cluster is disabled in config
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("cluster.enabled", false)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "kubectl" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		err := toolsManager.Check()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected Check to succeed when cluster is disabled, but got error: %v", err)
		}
	})

	t.Run("AllToolsDisabled", func(t *testing.T) {
		// When all tools are disabled in config
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("docker.enabled", false)
		mocks.ConfigHandler.Set("cluster.enabled", false)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" || name == "docker-compose" || name == "docker-cli-plugin-docker-compose" || name == "kubectl" {
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

	t.Run("ClusterEnabledButNotAvailable", func(t *testing.T) {
		// When cluster is enabled but kubectl not available in PATH
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("cluster.enabled", true)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "kubectl" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		err := toolsManager.Check()
		// Then an error indicating kubectl check failed should be returned
		if err == nil || !strings.Contains(err.Error(), "kubectl check failed") {
			t.Errorf("Expected Check to fail when cluster is enabled but not available, but got: %v", err)
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
		mocks.ConfigHandler.Set("aws.enabled", true)
		mocks.ConfigHandler.Set("cluster.enabled", true)

		// Mock failures for multiple tools
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" || name == "aws" || name == "kubectl" {
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
		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()
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
		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()
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
				return "Colima version 0.7.0", nil
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
				return "Colima version 0.7.0", nil
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
				return "Colima version 0.7.0", nil
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

// Tests for Kubectl version validation
func TestToolsManager_checkKubectl(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// When kubectl is available with correct version
		_, toolsManager := setup(t)
		err := toolsManager.checkKubectl()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected checkKubectl to succeed, but got error: %v", err)
		}
	})

	t.Run("KubectlVersionInvalidResponse", func(t *testing.T) {
		// When kubectl returns an invalid version response
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubectl" && args[0] == "version" {
				return "Invalid version response", nil
			}
			return "", fmt.Errorf("command not found")
		}
		err := toolsManager.checkKubectl()
		// Then an error indicating version extraction failed should be returned
		if err == nil || !strings.Contains(err.Error(), "failed to extract kubectl version") {
			t.Errorf("Expected failed to extract kubectl version error, got %v", err)
		}
	})

	t.Run("VersionTooLow", func(t *testing.T) {
		// When kubectl version is below minimum required version
		mocks, toolsManager := setup(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubectl" && args[0] == "version" && args[1] == "--client" {
				return "Client Version: v1.20.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		err := toolsManager.checkKubectl()
		// Then an error indicating version is too low should be returned
		if err == nil || !strings.Contains(err.Error(), "kubectl version 1.20.0 is below the minimum required version 1.27.0") {
			t.Errorf("Expected kubectl version too low error, got %v", err)
		}
	})
}

// Tests for Terraform version validation
func TestToolsManager_checkTerraform(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()
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
		// Given terraform is not found in PATH
		_, toolsManager := setup(t)
		execLookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "", fmt.Errorf("terraform is not available in the PATH")
			}
			return "/usr/bin/" + name, nil
		}
		// When checking terraform version
		err := toolsManager.checkTerraform()
		// Then an error indicating terraform is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "terraform is not available in the PATH") {
			t.Errorf("Expected terraform not available error, got %v", err)
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
		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()
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

// Tests for AWS CLI version validation
func TestToolsManager_checkAwsCli(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given AWS CLI is available with correct version
		mocks, toolsManager := setup(t)
		execLookPath = func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "aws" && args[0] == "--version" {
				return "aws-cli/2.15.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking AWS CLI version
		err := toolsManager.checkAwsCli()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected checkAwsCli to succeed, but got error: %v", err)
		}
	})

	t.Run("AwsCliNotAvailable", func(t *testing.T) {
		// Given AWS CLI is not found in PATH
		_, toolsManager := setup(t)
		execLookPath = func(name string) (string, error) {
			if name == "aws" {
				return "", fmt.Errorf("aws cli is not available in the PATH")
			}
			return "/usr/bin/" + name, nil
		}
		// When checking AWS CLI version
		err := toolsManager.checkAwsCli()
		// Then an error indicating AWS CLI is not available should be returned
		if err == nil || !strings.Contains(err.Error(), "aws cli is not available in the PATH") {
			t.Errorf("Expected aws cli not available error, got %v", err)
		}
	})

	t.Run("AwsCliVersionInvalidResponse", func(t *testing.T) {
		// Given AWS CLI version response is invalid
		mocks, toolsManager := setup(t)
		execLookPath = func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "aws" && args[0] == "--version" {
				return "Invalid version response", nil
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking AWS CLI version
		err := toolsManager.checkAwsCli()
		// Then an error indicating version extraction failed should be returned
		if err == nil || !strings.Contains(err.Error(), "failed to extract aws cli version") {
			t.Errorf("Expected failed to extract aws cli version error, got %v", err)
		}
	})

	t.Run("AwsCliVersionTooLow", func(t *testing.T) {
		// Given AWS CLI version is below minimum required version
		mocks, toolsManager := setup(t)
		execLookPath = func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		}
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "aws" && args[0] == "--version" {
				return "aws-cli/1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}
		// When checking AWS CLI version
		err := toolsManager.checkAwsCli()
		// Then an error indicating version is too low should be returned
		if err == nil || !strings.Contains(err.Error(), "aws cli version 1.0.0 is below the minimum required version") {
			t.Errorf("Expected aws cli version too low error, got %v", err)
		}
	})
}

// =============================================================================
// Test Public Helpers
// =============================================================================

// Tests for existing tools manager detection
func TestCheckExistingToolsManager(t *testing.T) {
	setup := func(t *testing.T) *Mocks {
		t.Helper()
		return setupMocks(t)
	}

	t.Run("NoToolsManager", func(t *testing.T) {
		// Given no tools manager is installed or configured
		setup(t)
		projectRoot := "/path/to/project"
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		execLookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}
		// When checking for existing tools manager
		managerName, err := CheckExistingToolsManager(projectRoot)
		// Then no error should be returned and manager name should be empty
		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "" {
			t.Errorf("Expected manager name to be empty, but got: %v", managerName)
		}
	})

	t.Run("DetectsAqua", func(t *testing.T) {
		// Given a project with aqua configuration
		setup(t)
		projectRoot := "/path/to/project/with/aqua"
		osStat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "aqua.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		// When checking for existing tools manager
		managerName, err := CheckExistingToolsManager(projectRoot)
		// Then aqua should be detected as the tools manager
		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "aqua" {
			t.Errorf("Expected manager name to be 'aqua', but got: %v", managerName)
		}
	})

	t.Run("DetectsAsdf", func(t *testing.T) {
		// Given a project with asdf configuration
		setup(t)
		projectRoot := "/path/to/project/with/asdf"
		osStat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".tool-versions") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		// When checking for existing tools manager
		managerName, err := CheckExistingToolsManager(projectRoot)
		// Then asdf should be detected as the tools manager
		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "asdf" {
			t.Errorf("Expected manager name to be 'asdf', but got: %v", managerName)
		}
	})

	t.Run("DetectsAquaInPath", func(t *testing.T) {
		// Given aqua is available in system PATH
		setup(t)
		projectRoot := "/path/to/project"
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		execLookPath = func(name string) (string, error) {
			if name == "aqua" {
				return "/usr/bin/aqua", nil
			}
			return "", exec.ErrNotFound
		}
		// When checking for existing tools manager
		managerName, err := CheckExistingToolsManager(projectRoot)
		// Then aqua should be detected as the tools manager
		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "aqua" {
			t.Errorf("Expected manager name to be 'aqua', but got: %v", managerName)
		}
	})

	t.Run("DetectsAsdfInPath", func(t *testing.T) {
		// Given asdf is available in system PATH
		setup(t)
		projectRoot := "/path/to/project"
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		execLookPath = func(name string) (string, error) {
			if name == "asdf" {
				return "/usr/bin/asdf", nil
			}
			if name == "aqua" {
				return "", exec.ErrNotFound
			}
			return "", exec.ErrNotFound
		}
		// When checking for existing tools manager
		managerName, err := CheckExistingToolsManager(projectRoot)
		// Then asdf should be detected as the tools manager
		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "asdf" {
			t.Errorf("Expected manager name to be 'asdf', but got: %v", managerName)
		}
	})

	t.Run("PrioritizesAquaOverAsdf", func(t *testing.T) {
		// Given both aqua.yaml and .tool-versions exist in project
		setup(t)
		projectRoot := "/path/to/project"
		osStat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "aqua.yaml") {
				return nil, nil
			}
			if strings.Contains(name, ".tool-versions") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		// When checking for existing tools manager
		managerName, err := CheckExistingToolsManager(projectRoot)
		// Then aqua should be selected over asdf
		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "aqua" {
			t.Errorf("Expected manager name to be 'aqua', but got: %v", managerName)
		}
	})

	t.Run("PrioritizesAquaInPathOverAsdfInPath", func(t *testing.T) {
		// Given both aqua and asdf are available in system PATH
		setup(t)
		projectRoot := "/path/to/project"
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		execLookPath = func(name string) (string, error) {
			if name == "aqua" {
				return "/usr/bin/aqua", nil
			}
			if name == "asdf" {
				return "/usr/bin/asdf", nil
			}
			return "", exec.ErrNotFound
		}
		// When checking for existing tools manager
		managerName, err := CheckExistingToolsManager(projectRoot)
		// Then aqua should be selected over asdf
		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "aqua" {
			t.Errorf("Expected manager name to be 'aqua', but got: %v", managerName)
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
		{"VersionWithColima", "Colima version 0.7.0", "0.7.0"},
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
