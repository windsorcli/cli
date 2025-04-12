package tools

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	sh "github.com/windsorcli/cli/pkg/shell"
)

type Mocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *sh.MockShell
}

// setupMocks sets up all necessary mocks and shims for testing, with cleanup registered via t.Cleanup.
func setupMocks(t *testing.T) *Mocks {
	t.Helper()

	// Save original shim values
	originalOsStat := osStat
	originalExecLookPath := execLookPath

	// Set up mock implementations
	osStat = func(name string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}

	execLookPath = func(name string) (string, error) {
		switch name {
		case "docker", "colima", "limactl", "kubectl", "talosctl", "terraform", "asdf", "aqua", "op", "docker-compose", "docker-cli-plugin-docker-compose":
			return "/usr/bin/" + name, nil
		default:
			return "", exec.ErrNotFound
		}
	}

	// Create a mock injector
	mockInjector := di.NewInjector()

	// Create a mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}

	// Create a mock shell
	mockShell := sh.NewMockShell()

	// Set up default version responses
	mockShell.ExecSilentFunc = func(name string, args ...string) (string, error) {
		switch name {
		case "docker":
			if args[0] == "version" {
				return fmt.Sprintf("Docker version %s", constants.MINIMUM_VERSION_DOCKER), nil
			}
			if args[0] == "compose" {
				return fmt.Sprintf("Docker Compose version %s", constants.MINIMUM_VERSION_DOCKER_COMPOSE), nil
			}
		case "colima":
			if args[0] == "version" {
				return fmt.Sprintf("Colima version %s", constants.MINIMUM_VERSION_COLIMA), nil
			}
		case "limactl":
			if args[0] == "--version" {
				return fmt.Sprintf("limactl version %s", constants.MINIMUM_VERSION_LIMA), nil
			}
		case "kubectl":
			if args[0] == "version" && args[1] == "--client" {
				return fmt.Sprintf("Client Version: v%s", constants.MINIMUM_VERSION_KUBECTL), nil
			}
		case "talosctl":
			if len(args) == 3 && args[0] == "version" && args[1] == "--client" && args[2] == "--short" {
				return fmt.Sprintf("v%s", constants.MINIMUM_VERSION_TALOSCTL), nil
			}
		case "terraform":
			if args[0] == "version" {
				return fmt.Sprintf("Terraform v%s", constants.MINIMUM_VERSION_TERRAFORM), nil
			}
		case "op":
			if args[0] == "--version" {
				return fmt.Sprintf("1Password CLI %s", constants.MINIMUM_VERSION_1PASSWORD), nil
			}
		}
		return "", fmt.Errorf("command not found")
	}

	// Register the mock config handler and shell in the injector
	mockInjector.Register("configHandler", mockConfigHandler)
	mockInjector.Register("shell", mockShell)

	// Register cleanup to restore original shim values
	t.Cleanup(func() {
		osStat = originalOsStat
		execLookPath = originalExecLookPath
	})

	return &Mocks{
		Injector:      mockInjector,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
	}
}

func TestToolsManager_NewToolsManager(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)

		toolsManager := NewToolsManager(mocks.Injector)

		if toolsManager == nil {
			t.Errorf("Expected tools manager to be non-nil")
		}
	})
}

func TestToolsManager_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)

		toolsManager := NewToolsManager(mocks.Injector)

		err := toolsManager.Initialize()

		if err != nil {
			t.Errorf("Expected Initialize to succeed, but got error: %v", err)
		}
	})
}

func TestToolsManager_WriteManifest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)

		toolsManager := NewToolsManager(mocks.Injector)

		err := toolsManager.WriteManifest()

		if err != nil {
			t.Errorf("Expected WriteManifest to succeed, but got error: %v", err)
		}
	})
}

func TestToolsManager_Install(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.Install()

		if err != nil {
			t.Errorf("Expected InstallTools to succeed, but got error: %v", err)
		}
	})
}

func TestToolsManager_Check(t *testing.T) {
	mockShellExec := func(toolVersions map[string]string) func(name string, args ...string) (string, error) {
		return func(name string, args ...string) (string, error) {
			if version, exists := toolVersions[name]; exists {
				return fmt.Sprintf("version %s", version), nil
			}
			return "", fmt.Errorf("%s not found", name)
		}
	}

	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)
		toolVersions := map[string]string{
			"docker":         constants.MINIMUM_VERSION_DOCKER,
			"colima":         constants.MINIMUM_VERSION_COLIMA,
			"limactl":        constants.MINIMUM_VERSION_LIMA,
			"kubectl":        constants.MINIMUM_VERSION_KUBECTL,
			"talosctl":       constants.MINIMUM_VERSION_TALOSCTL,
			"terraform":      constants.MINIMUM_VERSION_TERRAFORM,
			"op":             constants.MINIMUM_VERSION_1PASSWORD,
			"docker-compose": constants.MINIMUM_VERSION_DOCKER_COMPOSE,
		}
		mocks.Shell.ExecSilentFunc = mockShellExec(toolVersions)

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.Check()

		if err != nil {
			t.Errorf("Expected Check to succeed, but got error: %v", err)
		}
	})

	t.Run("DockerCheckFailed", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}

		execLookPath = func(name string) (string, error) {
			if name == "docker" {
				return "", fmt.Errorf("docker is not available in the PATH")
			}
			return "/usr/bin/" + name, nil
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.Check()

		if err == nil || !strings.Contains(err.Error(), "docker is not available in the PATH") {
			t.Errorf("Expected docker is not available in the PATH error, got %v", err)
		}
	})

	t.Run("KubectlCheckFailed", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "cluster.enabled" {
				return true
			}
			return false
		}

		execLookPath = func(name string) (string, error) {
			if name == "kubectl" {
				return "", fmt.Errorf("kubectl is not available in the PATH")
			}
			return "/usr/bin/" + name, nil
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.Check()

		if err == nil || !strings.Contains(err.Error(), "kubectl is not available in the PATH") {
			t.Errorf("Expected kubectl is not available in the PATH error, got %v", err)
		}
	})

	t.Run("TerraformCheckFailed", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}

		execLookPath = func(name string) (string, error) {
			if name == "terraform" {
				return "", fmt.Errorf("terraform is not available in the PATH")
			}
			return "/usr/bin/" + name, nil
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.Check()

		if err == nil || !strings.Contains(err.Error(), "terraform is not available in the PATH") {
			t.Errorf("Expected terraform is not available in the PATH error, got %v", err)
		}
	})

	t.Run("TalosctlCheckFailed", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "cluster.enabled" {
				return false
			}
			return false
		}
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}

		execLookPath = func(name string) (string, error) {
			if name == "talosctl" {
				return "", fmt.Errorf("talosctl is not available in the PATH")
			}
			if name == "docker" || name == "docker-compose" || name == "docker-cli-plugin-docker-compose" {
				return "/usr/bin/" + name, nil
			}
			return "", fmt.Errorf("%s is not available in the PATH", name)
		}

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			if name == "docker" && args[0] == "compose" {
				return "Docker Compose version 2.24.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.Check()

		if err == nil || !strings.Contains(err.Error(), "talosctl is not available in the PATH") {
			t.Errorf("Expected talosctl is not available in the PATH error, got %v", err)
		}
	})

	t.Run("ColimaCheckFailed", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "cluster.enabled" {
				return false
			}
			return false
		}
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}

		execLookPath = func(name string) (string, error) {
			if name == "colima" {
				return "", fmt.Errorf("colima is not available in the PATH")
			}
			if name == "docker" || name == "docker-compose" || name == "docker-cli-plugin-docker-compose" {
				return "/usr/bin/" + name, nil
			}
			return "", fmt.Errorf("%s is not available in the PATH", name)
		}

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			if name == "docker" && args[0] == "compose" {
				return "Docker Compose version 2.24.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.Check()

		if err == nil || !strings.Contains(err.Error(), "colima is not available in the PATH") {
			t.Errorf("Expected colima is not available in the PATH error, got %v", err)
		}
	})

	t.Run("OnePasswordCheckFailed", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "1password.vaults" {
				return map[string]string{"test": "test"}
			}
			return nil
		}

		execLookPath = func(name string) (string, error) {
			if name == "op" {
				return "", fmt.Errorf("1Password CLI is not available in the PATH")
			}
			return "/usr/bin/" + name, nil
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.Check()

		if err == nil || !strings.Contains(err.Error(), "1Password CLI is not available in the PATH") {
			t.Errorf("Expected 1Password CLI is not available in the PATH error, got %v", err)
		}
	})
}

func TestToolsManager_checkDocker(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)
		originalGetBoolFunc := mocks.ConfigHandler.GetBoolFunc
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return originalGetBoolFunc(key, defaultValue...)
		}

		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" || name == "docker-cli-plugin-docker-compose" {
				return "/usr/bin/" + name, nil
			}
			return originalExecLookPath(name)
		}
		defer func() { execLookPath = originalExecLookPath }()

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			if name == "docker" && args[0] == "compose" {
				return "Docker Compose version 2.24.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkDocker()

		if err != nil {
			t.Errorf("Expected checkDocker to succeed, but got error: %v", err)
		}
	})

	t.Run("DockerNotAvailable", func(t *testing.T) {
		mocks := setupMocks(t)
		originalGetBoolFunc := mocks.ConfigHandler.GetBoolFunc
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return originalGetBoolFunc(key, defaultValue...)
		}

		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		defer func() { execLookPath = originalExecLookPath }()

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkDocker()

		if err == nil || !strings.Contains(err.Error(), "docker is not available in the PATH") {
			t.Errorf("Expected docker is not available in the PATH error, got %v", err)
		}
	})

	t.Run("InvalidDockerVersionResponse", func(t *testing.T) {
		mocks := setupMocks(t)
		originalGetBoolFunc := mocks.ConfigHandler.GetBoolFunc
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return originalGetBoolFunc(key, defaultValue...)
		}

		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return originalExecLookPath(name)
		}
		defer func() { execLookPath = originalExecLookPath }()

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Invalid version response", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkDocker()

		if err == nil || !strings.Contains(err.Error(), "failed to extract Docker version") {
			t.Errorf("Expected failed to extract Docker version error, got %v", err)
		}
	})

	t.Run("DockerVersionTooLow", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}

		execLookPath = func(name string) (string, error) {
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "/usr/bin/" + name, nil
		}

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 19.03.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.Check()

		if err == nil || !strings.Contains(err.Error(), "docker version 19.03.0 is below the minimum required version") {
			t.Errorf("Expected docker version too low error, got %v", err)
		}
	})

	t.Run("DockerComposePluginInstalled", func(t *testing.T) {
		mocks := setupMocks(t)
		originalGetBoolFunc := mocks.ConfigHandler.GetBoolFunc
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return originalGetBoolFunc(key, defaultValue...)
		}

		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" || name == "docker-cli-plugin-docker-compose" {
				return "/usr/bin/" + name, nil
			}
			return originalExecLookPath(name)
		}
		defer func() { execLookPath = originalExecLookPath }()

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkDocker()

		if err != nil {
			t.Errorf("Expected checkDocker to succeed, but got error: %v", err)
		}
	})

	t.Run("DockerComposeInstalled", func(t *testing.T) {
		mocks := setupMocks(t)
		originalGetBoolFunc := mocks.ConfigHandler.GetBoolFunc
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return originalGetBoolFunc(key, defaultValue...)
		}

		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" || name == "docker-compose" {
				return "/usr/bin/" + name, nil
			}
			return originalExecLookPath(name)
		}
		defer func() { execLookPath = originalExecLookPath }()

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			if name == "docker-compose" && args[0] == "version" {
				return "Docker Compose version 2.24.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkDocker()

		if err != nil {
			t.Errorf("Expected checkDocker to succeed, but got error: %v", err)
		}
	})

	t.Run("DockerCliPluginComposeInstalled", func(t *testing.T) {
		mocks := setupMocks(t)
		originalGetBoolFunc := mocks.ConfigHandler.GetBoolFunc
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return originalGetBoolFunc(key, defaultValue...)
		}

		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "docker" || name == "docker-cli-plugin-docker-compose" {
				return "/usr/bin/" + name, nil
			}
			return originalExecLookPath(name)
		}
		defer func() { execLookPath = originalExecLookPath }()

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkDocker()

		if err != nil {
			t.Errorf("Expected checkDocker to succeed, but got error: %v", err)
		}
	})

	t.Run("DockerComposeVersionTooLow", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}

		execLookPath = func(name string) (string, error) {
			if name == "docker" || name == "docker-compose" {
				return "/usr/bin/" + name, nil
			}
			return "", fmt.Errorf("%s is not available in the PATH", name)
		}

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			if name == "docker-compose" && args[0] == "version" && args[1] == "--short" {
				return "1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkDocker()

		if err == nil || !strings.Contains(err.Error(), "docker-compose version 1.0.0 is below the minimum required version") {
			t.Errorf("Expected docker-compose version too low error, got %v", err)
		}
	})

	t.Run("DockerComposeNotAvailable", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}

		execLookPath = func(name string) (string, error) {
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			if name == "docker-compose" || name == "docker-cli-plugin-docker-compose" {
				return "", fmt.Errorf("docker-compose is not available in the PATH")
			}
			return "", fmt.Errorf("%s is not available in the PATH", name)
		}

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "docker" && args[0] == "version" {
				return "Docker version 25.0.0", nil
			}
			if name == "docker" && args[0] == "compose" {
				return "", fmt.Errorf("docker compose is not available")
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkDocker()

		if err == nil || !strings.Contains(err.Error(), "docker-compose is not available in the PATH") {
			t.Errorf("Expected docker-compose not available error, got %v", err)
		}
	})
}

func TestToolsManager_checkColima(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "colima" && args[0] == "version" {
				return "Colima version 0.7.0", nil
			}
			if name == "limactl" && args[0] == "--version" {
				return "limactl version 1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkColima()

		if err != nil {
			t.Errorf("Expected checkColima to succeed, but got error: %v", err)
		}
	})

	t.Run("ColimaNotAvailable", func(t *testing.T) {
		mocks := setupMocks(t)

		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "colima" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		defer func() { execLookPath = originalExecLookPath }()

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "limactl" && args[0] == "--version" {
				return "limactl version 1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkColima()

		if err == nil || !strings.Contains(err.Error(), "colima is not available in the PATH") {
			t.Errorf("Expected colima not available error, got %v", err)
		}
	})

	t.Run("InvalidColimaVersionResponse", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "colima" && args[0] == "version" {
				return "Invalid version response", nil
			}
			if name == "limactl" && args[0] == "--version" {
				return "limactl version 1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkColima()

		if err == nil || !strings.Contains(err.Error(), "failed to extract colima version") {
			t.Errorf("Expected failed to extract colima version error, got %v", err)
		}
	})

	t.Run("ColimaVersionTooLow", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "colima" && args[0] == "version" {
				return "Colima version 0.5.0", nil
			}
			if name == "limactl" && args[0] == "--version" {
				return "limactl version 1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkColima()

		if err == nil || !strings.Contains(err.Error(), "colima version 0.5.0 is below the minimum required version") {
			t.Errorf("Expected colima version too low error, got %v", err)
		}
	})

	t.Run("LimactlNotAvailable", func(t *testing.T) {
		mocks := setupMocks(t)

		originalExecLookPath := execLookPath
		execLookPath = func(name string) (string, error) {
			if name == "limactl" {
				return "", exec.ErrNotFound
			}
			return originalExecLookPath(name)
		}
		defer func() { execLookPath = originalExecLookPath }()

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "colima" && args[0] == "version" {
				return "Colima version 0.7.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkColima()

		if err == nil || !strings.Contains(err.Error(), "limactl is not available in the PATH") {
			t.Errorf("Expected limactl not available error, got %v", err)
		}
	})

	t.Run("InvalidLimactlVersionResponse", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "limactl" && args[0] == "--version" {
				return "Invalid version response", nil
			}
			if name == "colima" && args[0] == "version" {
				return "Colima version 0.7.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkColima()

		if err == nil || !strings.Contains(err.Error(), "failed to extract limactl version") {
			t.Errorf("Expected failed to extract limactl version error, got %v", err)
		}
	})

	t.Run("LimactlVersionTooLow", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "limactl" && args[0] == "--version" {
				return "Limactl version 0.5.0", nil
			}
			if name == "colima" && args[0] == "version" {
				return "Colima version 0.7.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkColima()

		if err == nil || !strings.Contains(err.Error(), "limactl version 0.5.0 is below the minimum required version") {
			t.Errorf("Expected limactl version too low error, got %v", err)
		}
	})
}

func TestToolsManager_checkKubectl(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkKubectl()

		if err != nil {
			t.Errorf("Expected checkKubectl to succeed, but got error: %v", err)
		}
	})

	t.Run("KubectlVersionInvalidResponse", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubectl" && args[0] == "version" && args[1] == "--client" {
				return "Invalid version response", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkKubectl()

		if err == nil || !strings.Contains(err.Error(), "failed to extract kubectl version") {
			t.Errorf("Expected failed to extract kubectl version error, got %v", err)
		}
	})

	t.Run("KubectlVersionTooLow", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "kubectl" && args[0] == "version" && args[1] == "--client" {
				return "Client Version: v1.20.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkKubectl()

		if err == nil || !strings.Contains(err.Error(), "kubectl version 1.20.0 is below the minimum required version") {
			t.Errorf("Expected kubectl version too low error, got %v", err)
		}
	})
}

func TestToolsManager_checkTalosctl(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkTalosctl()

		if err != nil {
			t.Errorf("Expected checkTalosctl to succeed, but got error: %v", err)
		}
	})

	t.Run("TalosctlVersionInvalidResponse", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "talosctl" && len(args) == 3 && args[0] == "version" && args[1] == "--client" && args[2] == "--short" {
				return "Invalid version response", nil
			}
			return "", fmt.Errorf("command not found")
		}
		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkTalosctl()

		if err == nil || !strings.Contains(err.Error(), "failed to extract talosctl version") {
			t.Errorf("Expected failed to extract talosctl version error, got %v", err)
		}
	})

	t.Run("TalosctlVersionTooLow", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "talosctl" && len(args) == 3 && args[0] == "version" && args[1] == "--client" && args[2] == "--short" {
				return "v0.1.0", nil // Return a version lower than the minimum required
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkTalosctl()

		if err == nil || !strings.Contains(err.Error(), "talosctl version 0.1.0 is below the minimum required version") {
			t.Errorf("Expected talosctl version too low error, got %v", err)
		}
	})
}

func TestToolsManager_checkTerraform(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkTerraform()

		if err != nil {
			t.Errorf("Expected checkTerraform to succeed, but got error: %v", err)
		}
	})

	t.Run("TerraformVersionInvalidResponse", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "terraform" && args[0] == "version" {
				return "Invalid version response", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkTerraform()

		if err == nil || !strings.Contains(err.Error(), "failed to extract terraform version") {
			t.Errorf("Expected failed to extract terraform version error, got %v", err)
		}
	})

	t.Run("TerraformVersionTooLow", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "terraform" && args[0] == "version" {
				return "Terraform v0.1.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkTerraform()

		if err == nil || !strings.Contains(err.Error(), "terraform version 0.1.0 is below the minimum required version") {
			t.Errorf("Expected terraform version too low error, got %v", err)
		}
	})
}

func TestToolsManager_checkOnePassword(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)
		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkOnePassword()

		if err != nil {
			t.Errorf("Expected checkOnePassword to succeed, but got error: %v", err)
		}
	})

	t.Run("OnePasswordNotAvailable", func(t *testing.T) {
		mocks := setupMocks(t)
		execLookPath = func(name string) (string, error) {
			if name == "op" {
				return "", fmt.Errorf("1Password CLI is not available in the PATH")
			}
			return "/usr/bin/" + name, nil
		}
		defer func() { execLookPath = nil }()

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkOnePassword()

		if err == nil || !strings.Contains(err.Error(), "1Password CLI is not available in the PATH") {
			t.Errorf("Expected 1Password CLI is not available in the PATH error, got %v", err)
		}
	})

	t.Run("OnePasswordVersionInvalidResponse", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "op" && args[0] == "--version" {
				return "Invalid version response", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkOnePassword()

		if err == nil || !strings.Contains(err.Error(), "failed to extract 1Password CLI version") {
			t.Errorf("Expected failed to extract 1Password CLI version error, got %v", err)
		}
	})

	t.Run("OnePasswordVersionTooLow", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.Shell.ExecSilentFunc = func(name string, args ...string) (string, error) {
			if name == "op" && args[0] == "--version" {
				return "1Password CLI 1.0.0", nil
			}
			return "", fmt.Errorf("command not found")
		}

		toolsManager := NewToolsManager(mocks.Injector)
		toolsManager.Initialize()

		err := toolsManager.checkOnePassword()

		if err == nil || !strings.Contains(err.Error(), "1Password CLI version 1.0.0 is below the minimum required version") {
			t.Errorf("Expected 1Password CLI version too low error, got %v", err)
		}
	})
}

func TestCheckExistingToolsManager(t *testing.T) {
	t.Run("NoToolsManager", func(t *testing.T) {
		projectRoot := "/path/to/project"

		setupMocks(t)

		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		execLookPath = func(_ string) (string, error) {
			return "", exec.ErrNotFound
		}

		managerName, err := CheckExistingToolsManager(projectRoot)
		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "" {
			t.Errorf("Expected manager name to be empty, but got: %v", managerName)
		}
	})

	t.Run("DetectsAqua", func(t *testing.T) {
		projectRoot := "/path/to/project/with/aqua"

		setupMocks(t)

		osStat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "aqua.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		execLookPath = func(name string) (string, error) {
			if name == "aqua" {
				return "/usr/local/bin/aqua", nil
			}
			return "", exec.ErrNotFound
		}

		managerName, err := CheckExistingToolsManager(projectRoot)

		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "aqua" {
			t.Errorf("Expected manager name to be 'aqua', but got: %v", managerName)
		}
	})

	t.Run("DetectsAsdf", func(t *testing.T) {
		projectRoot := "/path/to/project/with/asdf"

		setupMocks(t)

		osStat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".tool-versions") {
				return nil, nil
			}
			if strings.Contains(name, "aqua.yaml") {
				return nil, os.ErrNotExist
			}
			return nil, os.ErrNotExist
		}

		execLookPath = func(name string) (string, error) {
			if name == "asdf" {
				return "/usr/local/bin/asdf", nil
			}
			if name == "aqua" {
				return "", exec.ErrNotFound
			}
			return "", exec.ErrNotFound
		}

		managerName, err := CheckExistingToolsManager(projectRoot)

		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "asdf" {
			t.Errorf("Expected manager name to be 'asdf', but got: %v", managerName)
		}
	})

	t.Run("DetectsAquaInPath", func(t *testing.T) {
		projectRoot := "/path/to/project"

		setupMocks(t)

		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		execLookPath = func(name string) (string, error) {
			if name == "aqua" {
				return "/usr/local/bin/aqua", nil
			}
			return "", exec.ErrNotFound
		}

		managerName, err := CheckExistingToolsManager(projectRoot)

		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "aqua" {
			t.Errorf("Expected manager name to be 'aqua', but got: %v", managerName)
		}
	})

	t.Run("DetectsAsdfInPath", func(t *testing.T) {
		projectRoot := "/path/to/project"

		setupMocks(t)

		osStat = func(_ string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		execLookPath = func(name string) (string, error) {
			if name == "asdf" {
				return "/usr/local/bin/asdf", nil
			}
			if name == "aqua" {
				return "", exec.ErrNotFound
			}
			return "", exec.ErrNotFound
		}

		managerName, err := CheckExistingToolsManager(projectRoot)

		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "asdf" {
			t.Errorf("Expected manager name to be 'asdf', but got: %v", managerName)
		}
	})
}

func TestCompareVersion(t *testing.T) {
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
			result := compareVersion(tt.version1, tt.version2)
			if result != tt.expected {
				t.Errorf("compareVersion(%s, %s) = %d; want %d", tt.version1, tt.version2, result, tt.expected)
			}
		})
	}
}
