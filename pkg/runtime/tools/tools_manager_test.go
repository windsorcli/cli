package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

	originalExecLookPath := execLookPath
	originalOsStat := osStat

	execLookPath = func(name string) (string, error) {
		switch name {
		case "docker", "terraform", "op", "colima", "limactl", "sops":
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		_, toolsManager := setup(t, configStr)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		} else if !strings.Contains(err.Error(), "Docker") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected error to indicate Docker is missing, got: %v", err)
		}
	})
}

// TestToolsManager_CheckRequirements asserts the per-command requirement gate: if a command
// did NOT request a tool family, the corresponding check must not run even when the project
// config would otherwise enable it. The opposite — requesting a family that the config does
// not enable — is also a no-op (config gate wins). This test pins the contract Phase 5 (per
// command Requirements) is built on; weakening it would silently re-introduce the eager-check
// behavior the registry/Requirements split was meant to fix.
func TestToolsManager_CheckRequirements(t *testing.T) {
	setup := func(t *testing.T, configStr string) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t, &SetupOptions{ConfigStr: configStr})
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		return mocks, toolsManager
	}

	// configWithEverythingEnabled turns on every tool family the manager knows about so a
	// permissive Requirements set would have many ways to fail. Each sub-test then asserts
	// that the narrow Requirements still passes even though the underlying tools are absent.
	configWithEverythingEnabled := `
contexts:
  test:
    docker:
      enabled: true
    terraform:
      enabled: true
    secrets:
      sops:
        enabled: true
      onepassword:
        vaults:
          v1:
            name: V1
            url: example.1password.com
    azure:
      enabled: true
`

	t.Run("EmptyRequirementsSkipsAllChecks", func(t *testing.T) {
		// Given a config where every tool family is enabled and execLookPath fails for
		// EVERY tool we'd otherwise check
		_, toolsManager := setup(t, configWithEverythingEnabled)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}
		t.Cleanup(func() { execLookPath = originalExecLookPath })

		// When CheckRequirements runs with an empty Requirements set
		err := toolsManager.CheckRequirements(Requirements{})

		// Then no check fires and the call succeeds — this is the `windsor init` shape:
		// it has not committed to running any tool yet, so absent tools must not block it.
		if err != nil {
			t.Errorf("Expected empty Requirements to skip all checks, got: %v", err)
		}
	})

	t.Run("TerraformOnlyDoesNotCheckDocker", func(t *testing.T) {
		// Given docker is enabled in config but terraform alone is requested
		_, toolsManager := setup(t, configWithEverythingEnabled)
		dockerLookedUp := false
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" {
				dockerLookedUp = true
			}
			if name == "terraform" || name == "tofu" {
				return "/usr/bin/" + name, nil
			}
			return "", exec.ErrNotFound
		}
		t.Cleanup(func() { execLookPath = originalExecLookPath })

		// When CheckRequirements runs with only Terraform requested
		_ = toolsManager.CheckRequirements(Requirements{Terraform: true})

		// Then the docker lookup was never invoked. This is the `apply terraform <project>`
		// shape — terraform is the only binary that subcommand will exercise.
		if dockerLookedUp {
			t.Error("Expected docker to NOT be looked up when only Terraform is requested")
		}
	})

	t.Run("DockerOnlyDoesNotCheckTerraform", func(t *testing.T) {
		// Given terraform is enabled in config but docker alone is requested
		_, toolsManager := setup(t, configWithEverythingEnabled)
		terraformLookedUp := false
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "terraform" || name == "tofu" {
				terraformLookedUp = true
			}
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "", exec.ErrNotFound
		}
		t.Cleanup(func() { execLookPath = originalExecLookPath })

		// When CheckRequirements runs with only Docker requested
		_ = toolsManager.CheckRequirements(Requirements{Docker: true})

		// Then terraform/tofu was never looked up. This is the `windsor down` shape —
		// stopping the workstation needs the container runtime, not terraform.
		if terraformLookedUp {
			t.Error("Expected terraform/tofu to NOT be looked up when only Docker is requested")
		}
	})

	t.Run("SecretsRequestedChecksBothSopsAndOnePassword", func(t *testing.T) {
		// Given both sops and 1password are enabled in config and Secrets is requested
		_, toolsManager := setup(t, configWithEverythingEnabled)
		sawSops := false
		sawOp := false
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			switch name {
			case "sops":
				sawSops = true
			case "op":
				sawOp = true
			}
			return "/usr/bin/" + name, nil
		}
		t.Cleanup(func() { execLookPath = originalExecLookPath })

		// When CheckRequirements runs with Secrets requested
		_ = toolsManager.CheckRequirements(Requirements{Secrets: true})

		// Then both secrets backends were probed
		if !sawSops {
			t.Error("Expected sops to be looked up when Secrets is requested")
		}
		if !sawOp {
			t.Error("Expected op to be looked up when Secrets is requested")
		}
	})

	t.Run("OverRequestIsHarmlessWhenConfigGateOff", func(t *testing.T) {
		// Given a context with every tool-config gate explicitly off (workstation.runtime
		// pinned to "none" so the docker-needsDocker side-channel is also off)
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("docker.enabled", false)
		mocks.ConfigHandler.Set("terraform.enabled", false)
		mocks.ConfigHandler.Set("secrets.sops.enabled", false)
		mocks.ConfigHandler.Set("azure.enabled", false)
		mocks.ConfigHandler.Set("workstation.runtime", "none")
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}
		t.Cleanup(func() { execLookPath = originalExecLookPath })

		// When CheckRequirements runs with EVERY family requested
		err := toolsManager.CheckRequirements(AllRequirements())

		// Then it succeeds because the config gates skip every actual binary check. This
		// is the safety property that lets bootstrap/up/check pass AllRequirements() without
		// caring which specific tools the project has opted into.
		if err != nil {
			t.Errorf("Expected AllRequirements with no enabled tools to succeed, got: %v", err)
		}
	})

	t.Run("CheckEqualsCheckRequirementsAll", func(t *testing.T) {
		// Given the docker-enabled config + a docker-missing PATH
		mocks, toolsManager := setup(t, defaultConfig)
		mocks.ConfigHandler.Set("docker.enabled", true)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		t.Cleanup(func() { execLookPath = originalExecLookPath })

		// When Check() is called and CheckRequirements(AllRequirements()) is called
		errCheck := toolsManager.Check()
		errAll := toolsManager.CheckRequirements(AllRequirements())

		// Then both fail with the same docker-missing error — Check() is the AllRequirements
		// alias, so any divergence is a regression.
		if (errCheck == nil) != (errAll == nil) {
			t.Fatalf("Check vs CheckRequirements(All) disagreed on success/failure: %v vs %v", errCheck, errAll)
		}
		if errCheck != nil && errCheck.Error() != errAll.Error() {
			t.Errorf("Check vs CheckRequirements(All) returned different errors:\n  Check: %v\n  All:   %v", errCheck, errAll)
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
		execLookPath = func(name string) (string, error) {
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
		execLookPath = func(name string) (string, error) {
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
		if err == nil || !strings.Contains(err.Error(), "Terraform 0.1.0 is below the minimum required version") {
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
		_, toolsManager := setup(t)
		originalExecLookPath := execLookPath
		defer func() {
			execLookPath = originalExecLookPath
		}()
		execLookPath = func(name string) (string, error) {
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
		if err == nil || !strings.Contains(err.Error(), "kubelogin") || !strings.Contains(err.Error(), "not found on PATH") {
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
		if err == nil || !strings.Contains(err.Error(), "kubelogin 0.1.0 is below the minimum required version") {
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
		// Then an error indicating the --version invocation failed should be returned
		if err == nil || !strings.Contains(err.Error(), "kubelogin --version failed") {
			t.Errorf("Expected kubelogin --version failed error, got %v", err)
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

func TestToolsManager_checkAWSBinary(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		return mocks, toolsManager
	}

	t.Run("Success", func(t *testing.T) {
		// Given aws is available and meets the minimum version
		mocks, toolsManager := setup(t)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		_, toolsManager := setup(t)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		if !strings.Contains(err.Error(), "https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html") {
			t.Errorf("Expected vendor install URL in error, got: %v", err)
		}
	})

	t.Run("VersionCommandFails", func(t *testing.T) {
		// Given aws is in PATH but `aws --version` errors
		mocks, toolsManager := setup(t)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
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

func TestToolsManager_CheckAuth(t *testing.T) {
	setup := func(t *testing.T, configStr string) (*Mocks, *BaseToolsManager) {
		t.Helper()
		// Clear every env var the ambient-credentials guard consults so CheckAuth behaves
		// identically on a dev laptop, an EKS pod, and a GitHub Actions runner. Without
		// this, tests that assume context-env injection will intermittently skip injection
		// on machines where a stray AWS_ACCESS_KEY_ID or AWS_WEB_IDENTITY_TOKEN_FILE
		// happens to be exported.
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
		t.Setenv("AWS_ACCESS_KEY_ID", "")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "")
		// Same for Azure ambient-cred env vars
		t.Setenv("AZURE_CLIENT_ID", "")
		t.Setenv("AZURE_TENANT_ID", "")
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "")
		t.Setenv("AZURE_CONFIG_DIR", "")
		mocks := setupMocks(t, &SetupOptions{ConfigStr: configStr})
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
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

	// awsBinaryMock wires execLookPath and the version shell call so CheckAuth can reach the
	// sts-get-caller-identity step. Every sub-test that exercises CheckAuth beyond the binary
	// check uses this to get past the preliminaries.
	awsBinaryMock := func(t *testing.T) {
		t.Helper()
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "aws" {
				return "/usr/bin/aws", nil
			}
			return originalExecLookPath(name)
		}
		t.Cleanup(func() { execLookPath = originalExecLookPath })
	}

	t.Run("AWSPlatformWithAwsCliMissing", func(t *testing.T) {
		// Given platform: aws and the aws CLI is not in PATH
		_, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "aws" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		t.Cleanup(func() { execLookPath = originalExecLookPath })

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
		awsBinaryMock(t)
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
		awsBinaryMock(t)
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
		if !strings.Contains(err.Error(), "aws configure") && !strings.Contains(err.Error(), "aws sso login") {
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

	t.Run("AWSPlatformInjectsContextEnvForSts", func(t *testing.T) {
		// Given platform: aws and the aws CLI is installed
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t)
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
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("GetConfigRoot failed: %v", err)
		}
		wantConfig := filepath.ToSlash(filepath.Join(configRoot, ".aws", "config"))
		wantCreds := filepath.ToSlash(filepath.Join(configRoot, ".aws", "credentials"))
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
		// Given platform: aws and an explicit aws.profile that differs from the context name
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
    aws:
      profile: company-prod
`)
		awsBinaryMock(t)
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
		awsBinaryMock(t)
		t.Setenv("AWS_CONFIG_FILE", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")
		originalReadFile := osReadFile
		osReadFile = func(name string) ([]byte, error) {
			return []byte(`
[profile test]
sso_session = company
sso_account_id = 123456789012
`), nil
		}
		t.Cleanup(func() { osReadFile = originalReadFile })
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
		if !strings.Contains(err.Error(), "AWS_CONFIG_FILE=") {
			t.Errorf("Expected AWS_CONFIG_FILE= prefix when shell env is not sourced, got: %v", err)
		}
		if !strings.Contains(err.Error(), "aws sso login --profile test") {
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
		awsBinaryMock(t)
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("GetConfigRoot failed: %v", err)
		}
		t.Setenv("AWS_CONFIG_FILE", filepath.ToSlash(filepath.Join(configRoot, ".aws", "config")))
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.ToSlash(filepath.Join(configRoot, ".aws", "credentials")))
		originalReadFile := osReadFile
		osReadFile = func(name string) ([]byte, error) {
			return []byte(`
[profile test]
sso_session = company
sso_account_id = 123456789012
`), nil
		}
		t.Cleanup(func() { osReadFile = originalReadFile })
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
		if !strings.Contains(err.Error(), "aws sso login --profile test") {
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
		awsBinaryMock(t)
		t.Setenv("AWS_CONFIG_FILE", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")
		originalReadFile := osReadFile
		osReadFile = func(name string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}
		t.Cleanup(func() { osReadFile = originalReadFile })
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
		if !strings.Contains(err.Error(), "aws configure sso --profile test") {
			t.Errorf("Expected SSO setup hint, got: %v", err)
		}
		if !strings.Contains(err.Error(), "aws configure --profile test") {
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
		awsBinaryMock(t)
		originalReadFile := osReadFile
		osReadFile = func(name string) ([]byte, error) {
			return []byte(`
[profile company-prod]
sso_session = company
sso_account_id = 123456789012
`), nil
		}
		t.Cleanup(func() { osReadFile = originalReadFile })
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
		if !strings.Contains(err.Error(), "aws sso login --profile company-prod") {
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
		awsBinaryMock(t)
		t.Setenv("AWS_CONFIG_FILE", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/Users/foo/my projects", nil
		}
		originalReadFile := osReadFile
		osReadFile = func(name string) ([]byte, error) {
			return []byte(`
[profile test]
sso_session = company
`), nil
		}
		t.Cleanup(func() { osReadFile = originalReadFile })
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
		if !strings.Contains(err.Error(), `AWS_CONFIG_FILE="/Users/foo/my projects/contexts/test/.aws/config"`) {
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
		awsBinaryMock(t)
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

	t.Run("AWSPlatformWebIdentitySkipsProfileInjection", func(t *testing.T) {
		// Given an IRSA/OIDC environment — AWS_WEB_IDENTITY_TOKEN_FILE is set by the EKS
		// pod-identity webhook or a GitHub Actions OIDC step — and aws CLI is installed
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t)
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/var/run/secrets/eks.amazonaws.com/serviceaccount/token")
		t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/my-role")
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		var capturedEnv map[string]string
		var capturedEnvSeen bool
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			capturedEnv = env
			capturedEnvSeen = true
			return `{"Arn":"arn:aws:sts::123456789012:assumed-role/my-role/session"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed under IRSA, got %v", err)
		}
		// Then the shell receives a nil env map — overriding AWS_PROFILE in an IRSA pod
		// would point aws at a profile that does not exist on the pod filesystem and
		// surface a spurious "profile not found" before the SDK's web-identity provider
		// ever runs
		if !capturedEnvSeen {
			t.Fatal("Expected sts to be invoked")
		}
		if capturedEnv != nil {
			t.Errorf("Expected nil env under IRSA (SDK native chain must win), got %v", capturedEnv)
		}
	})

	t.Run("AWSPlatformEcsContainerRoleSkipsProfileInjection", func(t *testing.T) {
		// Given an ECS task with a task role — AWS_CONTAINER_CREDENTIALS_RELATIVE_URI is
		// injected by the ECS agent and points at the 169.254.170.2 metadata endpoint
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t)
		t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/v2/credentials/abc123")
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			capturedEnv = env
			return `{"Arn":"arn:aws:sts::123456789012:assumed-role/task-role/session"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed under ECS container role, got %v", err)
		}
		// Then the shell receives a nil env map so the SDK's container-credentials provider
		// (169.254.170.2) is consulted rather than getting short-circuited by a profile
		// lookup for a config file that doesn't exist in the task
		if capturedEnv != nil {
			t.Errorf("Expected nil env under ECS container role, got %v", capturedEnv)
		}
	})

	t.Run("AWSPlatformEcsAnywhereFullUriSkipsProfileInjection", func(t *testing.T) {
		// Given an ECS-Anywhere / externally-hosted container — AWS_CONTAINER_CREDENTIALS_FULL_URI
		// is set (rather than the RELATIVE_URI the in-cluster agent injects) because the creds
		// endpoint lives on a non-link-local host. The SDK treats FULL_URI identically to
		// RELATIVE_URI for credential resolution; the guard must treat it the same way too.
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t)
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "https://credentials.example.com/role/abc123")
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			capturedEnv = env
			return `{"Arn":"arn:aws:sts::123456789012:assumed-role/ecs-anywhere-role/session"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed under ECS-Anywhere FULL_URI, got %v", err)
		}
		// Then the shell receives a nil env map so the SDK resolves credentials from the
		// externally-hosted endpoint rather than getting short-circuited by a profile lookup
		if capturedEnv != nil {
			t.Errorf("Expected nil env under ECS-Anywhere FULL_URI, got %v", capturedEnv)
		}
	})

	t.Run("AWSPlatformStaticEnvKeysSkipProfileInjection", func(t *testing.T) {
		// Given a CI environment that exports AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY
		// directly (no profile file involved)
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t)
		t.Setenv("AWS_ACCESS_KEY_ID", "AKIATEST")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			capturedEnv = env
			return `{"Arn":"arn:aws:iam::123456789012:user/ci"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed with static env keys, got %v", err)
		}
		// Then the shell receives a nil env map — the static keys already in the parent env
		// are the intended credentials and injecting AWS_PROFILE would route the aws CLI
		// through a profile lookup that ignores them
		if capturedEnv != nil {
			t.Errorf("Expected nil env with static credentials, got %v", capturedEnv)
		}
	})

	t.Run("AWSPlatformAccessKeyWithoutSecretStillInjects", func(t *testing.T) {
		// Given only AWS_ACCESS_KEY_ID (no AWS_SECRET_ACCESS_KEY) — an incomplete
		// credential export that is NOT a working native chain
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		awsBinaryMock(t)
		t.Setenv("AWS_ACCESS_KEY_ID", "AKIATEST")
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return fmt.Sprintf("aws-cli/%s Python/3.11.8 Linux", constants.MinimumVersionAWS), nil
		}
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			capturedEnv = env
			return `{"Arn":"arn:aws:iam::123456789012:user/x"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed, got %v", err)
		}
		// Then context env is still injected — the guard requires BOTH halves of the static
		// keypair before declaring the native chain ready, so a stray AWS_ACCESS_KEY_ID
		// doesn't accidentally disable the context-scoped profile lookup
		if capturedEnv == nil {
			t.Fatal("Expected context env injection when static keypair is incomplete")
		}
		if capturedEnv["AWS_PROFILE"] != "test" {
			t.Errorf("AWS_PROFILE = %q, want %q", capturedEnv["AWS_PROFILE"], "test")
		}
	})

	t.Run("AWSPlatformAmbientCredsWithoutAwsCliIsNoOp", func(t *testing.T) {
		// Given a lean CI image that has IRSA creds exported but no aws CLI binary —
		// typical of a minimal GitHub Actions runner using OIDC federation where
		// awscli was never added to the image
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: aws
`)
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/var/run/secrets/eks.amazonaws.com/serviceaccount/token")
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}
		t.Cleanup(func() { execLookPath = originalExecLookPath })
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

	t.Run("AWSConfigBlockTriggersAuthCheck", func(t *testing.T) {
		// Given the context has an aws block (no platform set) and creds are invalid
		mocks, toolsManager := setup(t, `
contexts:
  test:
    aws:
      region: us-east-1
`)
		awsBinaryMock(t)
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

	// azBinaryMock wires execLookPath + ExecSilentWithTimeoutFunc("az --version") so
	// CheckAuth reaches the `az account show` step. Mirrors awsBinaryMock.
	azBinaryMock := func(t *testing.T, mocks *Mocks) {
		t.Helper()
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "az" {
				return "/usr/bin/az", nil
			}
			return originalExecLookPath(name)
		}
		t.Cleanup(func() { execLookPath = originalExecLookPath })
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) == 1 && args[0] == "--version" {
				return fmt.Sprintf("azure-cli                         %s\n", constants.MinimumVersionAzure), nil
			}
			return "", fmt.Errorf("command not mocked: %s %v", command, args)
		}
	}

	t.Run("AzurePlatformWithAzCliMissing", func(t *testing.T) {
		// Given platform: azure and the az CLI is not in PATH
		_, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "az" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		t.Cleanup(func() { execLookPath = originalExecLookPath })

		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the missing-binary error surfaces — the binary check is part of CheckAuth so
		// bootstrap/up preflights catch a missing CLI before they try to resolve credentials
		if err == nil {
			t.Fatal("Expected error when az CLI is missing")
		}
		if !strings.Contains(err.Error(), "az") || !strings.Contains(err.Error(), "not found on PATH") {
			t.Errorf("Expected 'az ... not found on PATH' error, got: %v", err)
		}
	})

	t.Run("AzurePlatformWithValidCredentials", func(t *testing.T) {
		// Given platform: azure, the az CLI is installed at the minimum version, and
		// `az account show` succeeds
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		azBinaryMock(t, mocks)
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) == 2 && args[0] == "account" && args[1] == "show" {
				return `{"id":"sub-1","tenantId":"tenant-1"}`, nil
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then it succeeds
		if err != nil {
			t.Errorf("Expected CheckAuth to succeed when az account show resolves, got %v", err)
		}
	})

	t.Run("AzurePlatformWithAccountShowFailure", func(t *testing.T) {
		// Given platform: azure, the az CLI is installed, but credentials cannot be resolved
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		azBinaryMock(t, mocks)
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("Please run 'az login' to setup account.")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the surfaced error contains the actionable hint — not the raw az stderr noise
		if err == nil {
			t.Fatal("Expected error when az account show fails")
		}
		if !strings.Contains(err.Error(), "az login") {
			t.Errorf("Expected 'az login' remediation hint in error, got: %v", err)
		}
		// The raw shell-exec wrapper text and az stderr must not leak into the surfaced error
		if strings.Contains(err.Error(), "command execution failed") {
			t.Errorf("Expected raw shell-exec error text to be suppressed, got: %v", err)
		}
		if strings.Contains(err.Error(), "Please run 'az login' to setup account") {
			t.Errorf("Expected raw az stderr to be suppressed, got: %v", err)
		}
	})

	t.Run("AzurePlatformInjectsContextEnvForAccountShow", func(t *testing.T) {
		// Given platform: azure and the az CLI is installed
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		azBinaryMock(t, mocks)
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) == 2 && args[0] == "account" && args[1] == "show" {
				capturedEnv = env
				return `{"id":"sub-1"}`, nil
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed, got %v", err)
		}
		// Then `az account show` received AZURE_CONFIG_DIR pointing at the context-scoped
		// .azure/ so it resolves against the context's token cache rather than ~/.azure
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("GetConfigRoot failed: %v", err)
		}
		wantDir := filepath.ToSlash(filepath.Join(configRoot, ".azure"))
		if capturedEnv["AZURE_CONFIG_DIR"] != wantDir {
			t.Errorf("AZURE_CONFIG_DIR = %q, want %q", capturedEnv["AZURE_CONFIG_DIR"], wantDir)
		}
		if capturedEnv["AZURE_CORE_LOGIN_EXPERIENCE_V2"] != "false" {
			t.Errorf("AZURE_CORE_LOGIN_EXPERIENCE_V2 = %q, want %q", capturedEnv["AZURE_CORE_LOGIN_EXPERIENCE_V2"], "false")
		}
	})

	t.Run("AzurePlatformAccountShowHintPrefixesEnvWhenShellNotSourced", func(t *testing.T) {
		// Given account show fails and AZURE_CONFIG_DIR is not in the process env
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		azBinaryMock(t, mocks)
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("AADSTS70043: The refresh token has expired")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the hint prepends AZURE_CONFIG_DIR= so the suggested az login lands in the context
		if err == nil {
			t.Fatal("Expected CheckAuth to fail when account show errors")
		}
		if !strings.Contains(err.Error(), "AZURE_CONFIG_DIR=") {
			t.Errorf("Expected AZURE_CONFIG_DIR= prefix when shell env is not sourced, got: %v", err)
		}
		if !strings.Contains(err.Error(), "az login") {
			t.Errorf("Expected az login hint, got: %v", err)
		}
	})

	t.Run("AzurePlatformAccountShowHintDropsEnvPrefixWhenShellIsSourced", func(t *testing.T) {
		// Given account show fails and AZURE_CONFIG_DIR already points at the context
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		azBinaryMock(t, mocks)
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("GetConfigRoot failed: %v", err)
		}
		t.Setenv("AZURE_CONFIG_DIR", filepath.ToSlash(filepath.Join(configRoot, ".azure")))
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("AADSTS70043: refresh token has expired")
		}
		// When CheckAuth runs
		err = toolsManager.CheckAuth()
		// Then the hint emits bare `az login` — prefix would be noise since the shell already has it
		if err == nil {
			t.Fatal("Expected CheckAuth to fail when account show errors")
		}
		if !strings.Contains(err.Error(), "az login") {
			t.Errorf("Expected az login hint, got: %v", err)
		}
		if strings.Contains(err.Error(), "AZURE_CONFIG_DIR=") {
			t.Errorf("Hint should omit AZURE_CONFIG_DIR= prefix when shell env already points at context, got: %v", err)
		}
	})

	t.Run("AzureHintIncludesTenantWhenSet", func(t *testing.T) {
		// Given an azure block with tenant_id set and account show fails
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
`)
		azBinaryMock(t, mocks)
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("not logged in")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the hint pins az login to the configured tenant
		if err == nil {
			t.Fatal("Expected CheckAuth to fail")
		}
		if !strings.Contains(err.Error(), "az login --tenant 11111111-2222-3333-4444-555555555555") {
			t.Errorf("Expected --tenant in hint, got: %v", err)
		}
	})

	t.Run("AzureHintIncludesSubscriptionFollowupWhenSet", func(t *testing.T) {
		// Given an azure block with subscription_id set and account show fails
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
    azure:
      subscription_id: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
`)
		azBinaryMock(t, mocks)
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			return "", fmt.Errorf("not logged in")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the hint chains `az account set --subscription`
		if err == nil {
			t.Fatal("Expected CheckAuth to fail")
		}
		if !strings.Contains(err.Error(), "az account set --subscription aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee") {
			t.Errorf("Expected az account set follow-up, got: %v", err)
		}
	})

	t.Run("AzurePlatformWorkloadIdentitySkipsCheckWhenAzMissing", func(t *testing.T) {
		// Given Workload Identity env exported but no az CLI (lean CI image)
		_, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		t.Setenv("AZURE_CLIENT_ID", "client-1")
		t.Setenv("AZURE_TENANT_ID", "tenant-1")
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "/var/run/secrets/azure/tokens/azure-identity-token")
		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}
		t.Cleanup(func() { execLookPath = originalExecLookPath })

		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then CheckAuth is a no-op — defers to the azurerm SDK at apply time
		if err != nil {
			t.Errorf("Expected CheckAuth to be a no-op under Workload Identity without az CLI, got %v", err)
		}
	})

	t.Run("AzurePlatformWorkloadIdentitySkipsCheckEntirely", func(t *testing.T) {
		// Given Workload Identity env is exported and az IS installed
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		azBinaryMock(t, mocks)
		t.Setenv("AZURE_CLIENT_ID", "client-1")
		t.Setenv("AZURE_TENANT_ID", "tenant-1")
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "/var/run/secrets/azure/tokens/azure-identity-token")
		accountShowCalled := false
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) >= 1 && args[0] == "account" {
				accountShowCalled = true
			}
			return `{"id":"sub-1"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed under Workload Identity, got %v", err)
		}
		// Then account show is never invoked — `az` doesn't honor the federated env vars and
		// would false-positive against a host that hasn't run `az login --federated-token`
		if accountShowCalled {
			t.Error("Expected account show to be skipped under ambient WI credentials")
		}
	})

	t.Run("AzurePlatformSpnSecretSkipsCheckEntirely", func(t *testing.T) {
		// Given classic SPN env (CLIENT_ID + TENANT_ID + CLIENT_SECRET)
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		azBinaryMock(t, mocks)
		t.Setenv("AZURE_CLIENT_ID", "client-1")
		t.Setenv("AZURE_TENANT_ID", "tenant-1")
		t.Setenv("AZURE_CLIENT_SECRET", "spn-secret")
		accountShowCalled := false
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) >= 1 && args[0] == "account" {
				accountShowCalled = true
			}
			return `{"id":"sub-1"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed under SPN secret, got %v", err)
		}
		// Then account show is never invoked — same reasoning as Workload Identity
		if accountShowCalled {
			t.Error("Expected account show to be skipped under ambient SPN credentials")
		}
	})

	t.Run("AzurePartialAmbientCredentialsStillInject", func(t *testing.T) {
		// Given only AZURE_CLIENT_ID — incomplete, not a working native chain
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		azBinaryMock(t, mocks)
		t.Setenv("AZURE_CLIENT_ID", "client-1")
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			capturedEnv = env
			return `{"id":"sub-1"}`, nil
		}
		// When CheckAuth runs
		if err := toolsManager.CheckAuth(); err != nil {
			t.Fatalf("Expected CheckAuth to succeed, got %v", err)
		}
		// Then context env is still injected — guard requires the full chain to skip
		if capturedEnv == nil {
			t.Fatal("Expected context env injection when ambient credentials are incomplete")
		}
		if capturedEnv["AZURE_CONFIG_DIR"] == "" {
			t.Errorf("Expected AZURE_CONFIG_DIR to be set, got %v", capturedEnv)
		}
	})

	t.Run("AzureConfigBlockTriggersAuthCheck", func(t *testing.T) {
		// Given the context has an azure block (no platform set) and creds are invalid
		mocks, toolsManager := setup(t, `
contexts:
  test:
    azure:
      subscription_id: sub-1
`)
		azBinaryMock(t, mocks)
		accountShowCalled := false
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "az" && len(args) == 2 && args[0] == "account" && args[1] == "show" {
				accountShowCalled = true
				return "", fmt.Errorf("not logged in")
			}
			return "", fmt.Errorf("command not mocked")
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then account show was invoked and the error surfaced (azure block alone gates auth)
		if !accountShowCalled {
			t.Error("Expected az account show to be invoked when azure block is present")
		}
		if err == nil {
			t.Error("Expected CheckAuth to surface the credential failure")
		}
	})

	t.Run("AzurePlatformConfigRootFailureSurfacesError", func(t *testing.T) {
		// Given GetProjectRoot fails so context-scoped Azure env can't be computed
		mocks, toolsManager := setup(t, `
contexts:
  test:
    platform: azure
`)
		azBinaryMock(t, mocks)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root not found")
		}
		accountShowCalled := false
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			accountShowCalled = true
			return "", nil
		}
		// When CheckAuth runs
		err := toolsManager.CheckAuth()
		// Then the configRoot-resolution failure surfaces and account show is never invoked
		if err == nil {
			t.Fatal("Expected CheckAuth to fail when configRoot cannot be resolved")
		}
		if !strings.Contains(err.Error(), "context-scoped Azure env") {
			t.Errorf("Expected context-scoped Azure env error, got: %v", err)
		}
		if accountShowCalled {
			t.Error("account show must not be invoked when the context-scoped env is unresolvable")
		}
	})
}

func Test_awsAuthHint(t *testing.T) {
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
		// When awsAuthHint runs
		hint := toolsManager.awsAuthHint()
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
	withReadFile := func(t *testing.T, contents string, readErr error) {
		t.Helper()
		original := osReadFile
		osReadFile = func(name string) ([]byte, error) {
			if readErr != nil {
				return nil, readErr
			}
			return []byte(contents), nil
		}
		t.Cleanup(func() { osReadFile = original })
	}

	t.Run("NoFileReturnsNone", func(t *testing.T) {
		withReadFile(t, "", os.ErrNotExist)
		if got := detectAWSProfileState("/irrelevant", "prod"); got != awsProfileNone {
			t.Errorf("expected awsProfileNone for missing file, got %v", got)
		}
	})

	t.Run("ProfileWithSsoSessionReturnsSSO", func(t *testing.T) {
		withReadFile(t, `
[profile prod]
sso_session = company
sso_account_id = 123456789012
sso_role_name = Admin
region = us-east-1
`, nil)
		if got := detectAWSProfileState("/irrelevant", "prod"); got != awsProfileSSO {
			t.Errorf("expected awsProfileSSO, got %v", got)
		}
	})

	t.Run("ProfileWithSsoStartUrlAloneReturnsSSO", func(t *testing.T) {
		// Legacy SSO profile form (pre-IAM-Identity-Center-sessions) uses sso_start_url
		// directly on the profile. detectAWSProfileState treats that as SSO too so expired
		// tokens route to the sso-login hint.
		withReadFile(t, `
[profile prod]
sso_start_url = https://example.awsapps.com/start
sso_region = us-east-1
sso_account_id = 123456789012
`, nil)
		if got := detectAWSProfileState("/irrelevant", "prod"); got != awsProfileSSO {
			t.Errorf("expected awsProfileSSO for legacy sso_start_url profile, got %v", got)
		}
	})

	t.Run("ProfileWithAccessKeysReturnsKeys", func(t *testing.T) {
		withReadFile(t, `
[profile prod]
aws_access_key_id = AKIATEST
aws_secret_access_key = secret
region = us-west-2
`, nil)
		if got := detectAWSProfileState("/irrelevant", "prod"); got != awsProfileKeys {
			t.Errorf("expected awsProfileKeys, got %v", got)
		}
	})

	t.Run("MissingProfileReturnsNone", func(t *testing.T) {
		// Config file has sections for other profiles but nothing for `prod`.
		withReadFile(t, `
[profile dev]
aws_access_key_id = AKIADEV
`, nil)
		if got := detectAWSProfileState("/irrelevant", "prod"); got != awsProfileNone {
			t.Errorf("expected awsProfileNone when profile not present, got %v", got)
		}
	})

	t.Run("DefaultProfileUsesDefaultHeader", func(t *testing.T) {
		// [default] uses a different header form than [profile <name>]; detection must
		// route to the bare header when the profile name is literally "default".
		withReadFile(t, `
[default]
aws_access_key_id = AKIADEFAULT
`, nil)
		if got := detectAWSProfileState("/irrelevant", "default"); got != awsProfileKeys {
			t.Errorf("expected awsProfileKeys for [default], got %v", got)
		}
	})

	t.Run("CommentsAndBlankLinesIgnored", func(t *testing.T) {
		withReadFile(t, `
# header comment
[profile prod]

# inline comment
sso_session = company
`, nil)
		if got := detectAWSProfileState("/irrelevant", "prod"); got != awsProfileSSO {
			t.Errorf("expected awsProfileSSO with comments/blanks, got %v", got)
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
		_ = NewToolsManager(nil, shell)
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

	t.Run("PanicsWhenConfigHandlerOrShellIsNil", func(t *testing.T) {
		// When NewToolsManager is called with nil config handler or shell
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when config handler or shell is nil")
			}
		}()
		_ = NewToolsManager(nil, nil)
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

	t.Run("ReturnsOpenTofuWhenDriverIsOpenTofu", func(t *testing.T) {
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
		// When getTerraformDriver is called
		driver := toolsManager.getTerraformDriver()
		// Then it should return "OpenTofu" (preserves case from config)
		if driver != "OpenTofu" {
			t.Errorf("Expected 'OpenTofu', got %s", driver)
		}
		// And GetTerraformCommand should still return "tofu" (case-insensitive)
		command := toolsManager.GetTerraformCommand()
		if command != "tofu" {
			t.Errorf("Expected GetTerraformCommand to return 'tofu', got %s", command)
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
		// When getTerraformDriver is called
		driver := toolsManager.getTerraformDriver()
		// Then it should return "tofu"
		if driver != "tofu" {
			t.Errorf("Expected 'tofu', got %s", driver)
		}
		// And GetTerraformCommand should return "tofu"
		command := toolsManager.GetTerraformCommand()
		if command != "tofu" {
			t.Errorf("Expected GetTerraformCommand to return 'tofu', got %s", command)
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
