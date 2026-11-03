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

	"github.com/windsorcli/cli/internal/werror"
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
	Shims         *Shims
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

	shims := NewShims()
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
// Test Public Methods
// =============================================================================

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
		toolsManager.shims = mocks.Shims
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
		mocks, toolsManager := setup(t, configWithEverythingEnabled)
		mocks.Shims.LookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}

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
		mocks, toolsManager := setup(t, configWithEverythingEnabled)
		dockerLookedUp := false
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "docker" {
				dockerLookedUp = true
			}
			if name == "terraform" || name == "tofu" {
				return "/usr/bin/" + name, nil
			}
			return "", exec.ErrNotFound
		}

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
		mocks, toolsManager := setup(t, configWithEverythingEnabled)
		terraformLookedUp := false
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "terraform" || name == "tofu" {
				terraformLookedUp = true
			}
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "", exec.ErrNotFound
		}

		// When CheckRequirements runs with only Docker requested
		_ = toolsManager.CheckRequirements(Requirements{Docker: true})

		// Then terraform/tofu was never looked up. This is the `windsor down` shape —
		// stopping the workstation needs the container runtime, not terraform.
		if terraformLookedUp {
			t.Error("Expected terraform/tofu to NOT be looked up when only Docker is requested")
		}
	})

	t.Run("TalosClusterDriverWithDockerChecksCPU", func(t *testing.T) {
		// Given docker.enabled AND cluster.driver: talos together — the one
		// combination that must actually wire through to the CPU check. Stubs
		// checkCPUMicroarchitecture itself so the assertion doesn't depend on
		// this test runner's real CPU features.
		mocks, toolsManager := setup(t, `
contexts:
  test:
    docker:
      enabled: true
    cluster:
      driver: talos
`)
		mocks.Shims.LookPath = func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		}

		originalCheckCPUMicroarchitecture := checkCPUMicroarchitecture
		var called bool
		checkCPUMicroarchitecture = func() error {
			called = true
			return nil
		}
		t.Cleanup(func() { checkCPUMicroarchitecture = originalCheckCPUMicroarchitecture })

		// When CheckRequirements runs with Docker requested
		if err := toolsManager.CheckRequirements(Requirements{Docker: true}); err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// Then the CPU check actually fired — this is the wiring a regression
		// (dropped call, or a broken "talos" string match) would silently break
		if !called {
			t.Error("Expected checkCPUMicroarchitecture to be called for docker+talos, got not called")
		}
	})

	t.Run("TalosClusterDriverWithoutDockerNeverChecksCPU", func(t *testing.T) {
		// Given cluster.driver is talos but no docker-family workstation runtime is
		// configured (e.g. Talos on bare metal/cloud) — the CPU microarchitecture
		// check only applies to Talos-in-Docker, so it must never fire here.
		// docker.enabled is explicitly false: setupMocks' own defaultConfig sets
		// it true for context "test", and LoadConfigString merges rather than
		// replaces, so omitting it here would silently leave docker enabled.
		_, toolsManager := setup(t, `
contexts:
  test:
    docker:
      enabled: false
    cluster:
      driver: talos
`)
		originalCheckCPUMicroarchitecture := checkCPUMicroarchitecture
		var called bool
		checkCPUMicroarchitecture = func() error {
			called = true
			return nil
		}
		t.Cleanup(func() { checkCPUMicroarchitecture = originalCheckCPUMicroarchitecture })

		// When CheckRequirements runs with Docker requested
		err := toolsManager.CheckRequirements(Requirements{Docker: true})

		// Then it succeeds trivially — the docker check itself is skipped since
		// needsDocker is false, so the CPU check nested inside it never runs
		if err != nil {
			t.Errorf("Expected no error when docker isn't needed, got: %v", err)
		}
		if called {
			t.Error("Expected checkCPUMicroarchitecture NOT to be called when docker isn't needed")
		}
	})

	t.Run("NonTalosClusterDriverNeverChecksCPU", func(t *testing.T) {
		// Given a docker-family workstation runtime with a non-talos cluster driver
		// (e.g. an EKS/AKS/GKE cluster where docker is only used for local tooling)
		mocks, toolsManager := setup(t, `
contexts:
  test:
    docker:
      enabled: true
    cluster:
      driver: eks
`)
		mocks.Shims.LookPath = func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		}

		originalCheckCPUMicroarchitecture := checkCPUMicroarchitecture
		var called bool
		checkCPUMicroarchitecture = func() error {
			called = true
			return nil
		}
		t.Cleanup(func() { checkCPUMicroarchitecture = originalCheckCPUMicroarchitecture })

		// When CheckRequirements runs with Docker requested
		err := toolsManager.CheckRequirements(Requirements{Docker: true})

		// Then it succeeds and the microarchitecture check never fired — it's
		// scoped to cluster.driver == "talos" only
		if err != nil {
			t.Errorf("Expected no error for a non-talos cluster driver, got: %v", err)
		}
		if called {
			t.Error("Expected checkCPUMicroarchitecture NOT to be called for a non-talos cluster driver")
		}
	})

	t.Run("SecretsRequestedChecksBothSopsAndOnePassword", func(t *testing.T) {
		// Given both sops and 1password are enabled in config and Secrets is requested
		mocks, toolsManager := setup(t, configWithEverythingEnabled)
		sawSops := false
		sawOp := false
		mocks.Shims.LookPath = func(name string) (string, error) {
			switch name {
			case "sops":
				sawSops = true
			case "op":
				sawOp = true
			}
			return "/usr/bin/" + name, nil
		}

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
		mocks.ConfigHandler.Set("workstation.runtime", "none")
		// Azure is gated on platform=="azure" or an azure: block being present, not on
		// a separate `azure.enabled` flag (we removed that to match AWS). Setting
		// azure.enabled=false here would actually OPT IN to the kubelogin check by
		// causing the config handler to materialize the azure block as a side effect.
		mocks.Shims.LookPath = func(name string) (string, error) {
			return "", exec.ErrNotFound
		}

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
		originalExecLookPath := mocks.Shims.LookPath
		mocks.Shims.LookPath = func(name string) (string, error) {
			if name == "docker" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}

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

// Tests for getTerraformDriver functionality
func TestToolsManager_getTerraformDriver(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseToolsManager) {
		t.Helper()
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
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
		mocks.Shims.LookPath = func(name string) (string, error) {
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
		mocks.Shims.LookPath = func(name string) (string, error) {
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
		mocks.Shims.LookPath = func(name string) (string, error) {
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
		mocks.Shims.LookPath = func(name string) (string, error) {
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
		mocks.Shims.LookPath = func(name string) (string, error) {
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
		mocks.Shims.LookPath = func(name string) (string, error) {
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
		toolsManager := NewToolsManager(mocks.ConfigHandler, mocks.Shell)
		toolsManager.shims = mocks.Shims
		return toolsManager
	}

	t.Run("ReturnsTerraformWhenTerraformAvailable", func(t *testing.T) {
		// Given terraform is available in PATH
		toolsManager := setup(t)
		toolsManager.shims.LookPath = func(name string) (string, error) {
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
		toolsManager.shims.LookPath = func(name string) (string, error) {
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
		toolsManager.shims.LookPath = func(name string) (string, error) {
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
		toolsManager.shims.LookPath = func(name string) (string, error) {
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
