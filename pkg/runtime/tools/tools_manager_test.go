package tools_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime/config"
	sh "github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	ConfigHandler config.ConfigHandler
	Shell         *sh.MockShell
	Shims         *tools.Shims
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
		case name == "colima" && args[0] == "version":
			return fmt.Sprintf("colima version %s", constants.MinimumVersionColima), nil
		case name == "limactl" && args[0] == "--version":
			return fmt.Sprintf("limactl version %s", constants.MinimumVersionLima), nil
		case name == "terraform" && args[0] == "version":
			return fmt.Sprintf("Terraform v%s", constants.MinimumVersionTerraform), nil
		case name == "op" && args[0] == "--version":
			return fmt.Sprintf("1Password CLI %s", constants.MinimumVersion1Password), nil
		case name == "sops" && args[0] == "--version":
			return fmt.Sprintf("sops %s", constants.MinimumVersionSOPS), nil
		}
		return "", fmt.Errorf("command not found")
	}
	shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
		result, err := shell.ExecSilentFunc(command, args...)
		if err == nil {
			return result, err
		}
		if !strings.Contains(err.Error(), "command not found") {
			return result, err
		}
		var legacyArgs []string
		switch {
		case command == "docker" && len(args) == 1 && args[0] == "--version":
			legacyArgs = []string{"version"}
		default:
			legacyArgs = nil
		}
		if legacyArgs != nil {
			legacyResult, legacyErr := shell.ExecSilentFunc(command, legacyArgs...)
			if legacyErr == nil {
				return legacyResult, legacyErr
			}
			if !strings.Contains(legacyErr.Error(), "command not found") {
				return legacyResult, legacyErr
			}
			return result, err
		}
		switch {
		case command == "docker" && len(args) == 1 && args[0] == "--version":
			return fmt.Sprintf("Docker version %s", constants.MinimumVersionDocker), nil
		case command == "colima" && len(args) == 1 && args[0] == "version":
			return fmt.Sprintf("colima version %s", constants.MinimumVersionColima), nil
		case command == "limactl" && len(args) == 1 && args[0] == "--version":
			return fmt.Sprintf("limactl version %s", constants.MinimumVersionLima), nil
		case command == "terraform" && len(args) == 1 && args[0] == "version":
			return fmt.Sprintf("Terraform v%s", constants.MinimumVersionTerraform), nil
		case command == "op" && len(args) == 1 && args[0] == "--version":
			return fmt.Sprintf("1Password CLI %s", constants.MinimumVersion1Password), nil
		case command == "sops" && len(args) == 1 && args[0] == "--version":
			return fmt.Sprintf("sops %s", constants.MinimumVersionSOPS), nil
		case command == "kubelogin" && len(args) == 1 && args[0] == "--version":
			return fmt.Sprintf("kubelogin version %s", constants.MinimumVersionKubelogin), nil
		default:
			return result, err
		}
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

	shims := tools.NewShims()
	shims.LookPath = func(name string) (string, error) {
		switch name {
		case "docker", "terraform", "op", "colima", "limactl", "sops":
			return "/usr/bin/" + name, nil
		default:
			return "", exec.ErrNotFound
		}
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")

		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	return &Mocks{
		Shell:         shell,
		ConfigHandler: configHandler,
		Shims:         shims,
	}
}

// =============================================================================
// Test Constructor
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
		toolsManager := tools.NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.SetShims(mocks.Shims)
		// Then the tools manager should be created successfully
		if toolsManager == nil {
			t.Errorf("Expected tools manager to be non-nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

// Tests for initialization process
func TestToolsManager_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *tools.BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := tools.NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.SetShims(mocks.Shims)
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

// Tests for installation process
func TestToolsManager_Install(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *tools.BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := tools.NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.SetShims(mocks.Shims)
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
	setup := func(t *testing.T, configStr string) (*Mocks, *tools.BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t, &SetupOptions{ConfigStr: configStr})
		toolsManager := tools.NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.SetShims(mocks.Shims)
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		mocks, toolsManager := setup(t, defaultConfig)
		toolVersions := map[string][]string{
			"docker":    {"version", "--format"},
			"colima":    {"version"},
			"limactl":   {"--version"},
			"terraform": {"version"},
			"op":        {"--version"},
		}
		err := toolsManager.Check()
		if err != nil {
			t.Errorf("Expected Check to succeed, but got error: %v", err)
		}
		for tool, args := range toolVersions {
			output, err := mocks.Shell.ExecSilent(tool, args...)
			if err != nil {
				t.Errorf("Failed to get %s version: %v", tool, err)
				continue
			}
			if !strings.Contains(output, constants.MinimumVersionDocker) &&
				!strings.Contains(output, constants.MinimumVersionColima) &&
				!strings.Contains(output, constants.MinimumVersionLima) &&
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
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "docker" {
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
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "docker" {
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
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "docker" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		err := toolsManager.Check()
		// Then an error indicating docker is missing should be returned
		if err == nil || !strings.Contains(err.Error(), "Docker") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected Check to fail when docker is enabled but not available, but got: %v", err)
		}
	})

	t.Run("TerraformEnabledButNotAvailable", func(t *testing.T) {
		// When terraform is enabled but not available in PATH
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("terraform.enabled", true)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		err := toolsManager.Check()
		// Then an error indicating terraform is missing should be returned
		if err == nil || !strings.Contains(err.Error(), "Terraform") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected Check to fail when terraform is enabled but not available, but got: %v", err)
		}
	})

	t.Run("ColimaEnabledButNotAvailable", func(t *testing.T) {
		// When colima is enabled but not available in PATH
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "colima" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		err := toolsManager.Check()
		// Then an error indicating colima is missing should be returned
		if err == nil || !strings.Contains(err.Error(), "Colima") || !strings.Contains(err.Error(), "not found on PATH") {
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
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
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
		// Then an error indicating 1Password CLI is missing should be returned
		if err == nil {
			t.Error("Expected error when 1Password is enabled but not available")
		} else if !strings.Contains(err.Error(), "1Password CLI") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected error to indicate 1Password CLI is missing, got: %v", err)
		}
	})

	t.Run("SopsEnabledButNotAvailable", func(t *testing.T) {
		// When SOPS is enabled but not available in PATH
		configStr := `
contexts:
  test:
    secrets:
      sops:
        enabled: true
`
		mocks, toolsManager := setup(t, configStr)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "sops" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "sops" {
				return "", fmt.Errorf("SOPS CLI is not available in the PATH")
			}
			return originalExecSilent(name, args...)
		}
		err := toolsManager.Check()
		// Then an error indicating SOPS is missing should be returned
		if err == nil {
			t.Error("Expected error when SOPS is enabled but not available")
		} else if !strings.Contains(err.Error(), "SOPS") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected error to indicate SOPS is missing, got: %v", err)
		}
	})

	t.Run("SopsEnabledWithCorrectVersion", func(t *testing.T) {
		// When SOPS is enabled and available with correct version
		configStr := `
contexts:
  test:
    secrets:
      sops:
        enabled: true
`
		_, toolsManager := setup(t, configStr)
		err := toolsManager.Check()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected Check to succeed when SOPS is enabled with correct version, but got error: %v", err)
		}
	})

	t.Run("AWSPlatformDoesNotTriggerAWSCheckInCheck", func(t *testing.T) {
		// AWS CLI presence and credentials are explicitly OUT of Check(): `windsor init`
		// and `windsor env` go through PrepareTools → Check and have no obligation to have
		// the aws CLI installed or be authenticated. Both checks live on CheckAuth, which
		// fires from bootstrap/up/apply preflights and from `windsor check`. This test
		// ensures Check() stays silent even when platform is aws and the aws CLI is absent.
		configStr := `
contexts:
  test:
    platform: aws
`
		mocks, toolsManager := setup(t, configStr)
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "aws" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		if err := toolsManager.Check(); err != nil {
			t.Errorf("Expected Check to succeed when aws is absent (platform: aws), got: %v", err)
		}
	})

	t.Run("MultipleToolFailures", func(t *testing.T) {
		// Given multiple tools are enabled but fail checks
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("docker.enabled", true)

		// Mock failures for multiple tools
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
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
		} else if !strings.Contains(err.Error(), "Docker") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected error to indicate Docker is missing, got: %v", err)
		}
	})
}

// Tests for Terraform version validation
func TestToolsManager_CheckTerraform(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *tools.BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := tools.NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.SetShims(mocks.Shims)
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given terraform is available with correct version
		_, toolsManager := setup(t)
		// When checking terraform version
		err := toolsManager.CheckTerraform()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected CheckTerraform to succeed, but got error: %v", err)
		}
	})

	t.Run("TerraformNotAvailable", func(t *testing.T) {
		// Given neither terraform nor tofu is found in PATH
		mocks, toolsManager := setup(t)
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "terraform" || name == "tofu" {
				return "", fmt.Errorf("%s is not available in the PATH", name)
			}
			return "/usr/bin/" + name, nil
		}
		// When checking terraform version
		err := toolsManager.CheckTerraform()
		// Then an error indicating terraform or tofu is not available should be returned
		if err == nil || ((!strings.Contains(err.Error(), "Terraform") && !strings.Contains(err.Error(), "OpenTofu")) || !strings.Contains(err.Error(), "not found on PATH")) {
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
		err := toolsManager.CheckTerraform()
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
		err := toolsManager.CheckTerraform()
		// Then an error indicating version is too low should be returned
		if err == nil || !strings.Contains(err.Error(), "Terraform 0.1.0 is below the minimum required version") {
			t.Errorf("Expected terraform version too low error, got %v", err)
		}
	})
}

// Tests for GetTerraformCommand functionality
func TestToolsManager_GetTerraformCommand(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *tools.BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := tools.NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.SetShims(mocks.Shims)
		return mocks, toolsManager
	}

	t.Run("PanicsWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a shell
		shell := sh.NewMockShell()
		// When NewToolsManager is called with nil config handler
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when config handler is nil")
			}
		}()
		_ = tools.NewToolsManager(nil, shell)
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

	t.Run("ReturnsTofuWhenDriverIsTofu", func(t *testing.T) {
		// Given a tools manager with tofu driver configured (alias)
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`terraform:
  driver: tofu
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

	t.Run("ReturnsTofuWhenDriverIsOpenTofu", func(t *testing.T) {
		// Given a tools manager with OpenTofu driver configured (case variation)
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`terraform:
  driver: OpenTofu
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

	t.Run("ReturnsTofuWhenDriverIsOPENTOFU", func(t *testing.T) {
		// Given a tools manager with OPENTOFU driver configured (uppercase)
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`terraform:
  driver: OPENTOFU
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

	t.Run("ReturnsTofuWhenDriverIsTOFU", func(t *testing.T) {
		// Given a tools manager with TOFU driver configured (uppercase alias)
		mocks, toolsManager := setup(t)
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte(`terraform:
  driver: TOFU
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
		mocks.Shims.LookPath = func(name string) (string, error) {
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

// =============================================================================
// Test Helpers
// =============================================================================

// errorText returns the message and the remediation of the WindsorError in err, or err.Error()
// when err has no WindsorError.
func errorText(err error) string {
	if err == nil {
		return ""
	}
	if we := werror.Find(err); we != nil {
		return we.Message + "\n" + we.Remediation
	}
	return err.Error()
}
