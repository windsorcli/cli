package pipelines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupBasePipeline(t *testing.T) (*BasePipeline, di.Injector) {
	t.Helper()

	injector := di.NewInjector()
	pipeline := NewBasePipeline()

	return pipeline, injector
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewBasePipeline(t *testing.T) {
	t.Run("CreatesWithDefaults", func(t *testing.T) {
		pipeline := NewBasePipeline()

		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBasePipeline_Initialize(t *testing.T) {
	t.Run("InitializeReturnsNilByDefault", func(t *testing.T) {
		// Given a base pipeline
		pipeline, injector := setupBasePipeline(t)

		// When initializing the pipeline
		err := pipeline.Initialize(injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestBasePipeline_Execute(t *testing.T) {
	t.Run("ExecuteReturnsNilByDefault", func(t *testing.T) {
		// Given a base pipeline
		pipeline, injector := setupBasePipeline(t)

		// When initializing and executing the pipeline
		err := pipeline.Initialize(injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		ctx := context.Background()
		err = pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// =============================================================================
// Test Protected Methods
// =============================================================================

func TestBasePipeline_handleSessionReset(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *shell.MockShell) {
		t.Helper()
		pipeline := NewBasePipeline()
		mockShell := shell.NewMockShell()
		pipeline.shell = mockShell

		// Clean up any existing environment variables
		t.Cleanup(func() {
			os.Unsetenv("WINDSOR_SESSION_TOKEN")
			os.Unsetenv("NO_CACHE")
		})

		return pipeline, mockShell
	}

	t.Run("ReturnsNilWhenShellIsNil", func(t *testing.T) {
		// Given a pipeline with nil shell
		pipeline := &BasePipeline{}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ResetsWhenNoSessionToken", func(t *testing.T) {
		// Given a pipeline with no session token
		pipeline, mockShell := setup(t)

		// Ensure no session token is set
		os.Unsetenv("WINDSOR_SESSION_TOKEN")

		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		resetCalled := false
		mockShell.ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then reset should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !resetCalled {
			t.Error("Expected shell reset to be called")
		}
	})

	t.Run("ResetsWhenResetFlagsTrue", func(t *testing.T) {
		// Given a pipeline with reset flags true
		pipeline, mockShell := setup(t)

		// Set a session token
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}
		resetCalled := false
		mockShell.ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then reset should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !resetCalled {
			t.Error("Expected shell reset to be called")
		}
	})

	t.Run("DoesNotResetWhenSessionTokenExistsAndResetFlagsFalse", func(t *testing.T) {
		// Given a pipeline with session token and reset flags false
		pipeline, mockShell := setup(t)

		// Set a session token
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		resetCalled := false
		mockShell.ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then reset should not be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if resetCalled {
			t.Error("Expected shell reset to not be called")
		}
	})

	t.Run("ReturnsErrorWhenCheckResetFlagsFails", func(t *testing.T) {
		// Given a pipeline where check reset flags fails
		pipeline, mockShell := setup(t)

		// Ensure no session token is set
		os.Unsetenv("WINDSOR_SESSION_TOKEN")

		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("check reset flags error")
		}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "check reset flags error" {
			t.Errorf("Expected check reset flags error, got: %v", err)
		}
	})

	t.Run("HandlesSessionResetWithNoSessionToken", func(t *testing.T) {
		// Given a base pipeline with no session token
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		mockShell.ResetFunc = func(args ...bool) {
			// Reset called
		}
		pipeline.shell = mockShell

		// Ensure no session token is set
		originalToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		os.Unsetenv("WINDSOR_SESSION_TOKEN")
		defer func() {
			if originalToken != "" {
				os.Setenv("WINDSOR_SESSION_TOKEN", originalToken)
			}
		}()

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("HandlesSessionResetWithSessionToken", func(t *testing.T) {
		// Given a base pipeline with session token
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		pipeline.shell = mockShell

		// Set session token
		originalToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer func() {
			if originalToken != "" {
				os.Setenv("WINDSOR_SESSION_TOKEN", originalToken)
			} else {
				os.Unsetenv("WINDSOR_SESSION_TOKEN")
			}
		}()

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("HandlesSessionResetWithNilShell", func(t *testing.T) {
		// Given a base pipeline with nil shell
		pipeline := NewBasePipeline()
		pipeline.shell = nil

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestBasePipeline_loadConfig(t *testing.T) {
	t.Run("ReturnsErrorWhenShellIsNil", func(t *testing.T) {
		// Given a BasePipeline with nil shell
		pipeline := NewBasePipeline()

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when shell is nil")
		}
		if err.Error() != "shell not initialized" {
			t.Errorf("Expected 'shell not initialized' error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a BasePipeline with shell but nil config handler
		pipeline := NewBasePipeline()
		pipeline.shell = shell.NewMockShell()

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when config handler is nil")
		}
		if err.Error() != "config handler not initialized" {
			t.Errorf("Expected 'config handler not initialized' error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShimsIsNil", func(t *testing.T) {
		// Given a BasePipeline with shell and config handler but nil shims
		pipeline := NewBasePipeline()
		pipeline.shell = shell.NewMockShell()
		pipeline.configHandler = config.NewMockConfigHandler()

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when shims is nil")
		}
		if err.Error() != "shims not initialized" {
			t.Errorf("Expected 'shims not initialized' error, got %v", err)
		}
	})

	t.Run("LoadsConfigSuccessfully", func(t *testing.T) {
		// Given a BasePipeline with shell, config handler, and shims
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		projectRoot := t.TempDir()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}
		pipeline.shell = mockShell

		mockConfigHandler := config.NewMockConfigHandler()
		loadConfigCalled := false
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			loadConfigCalled = true
			expectedPath := filepath.Join(projectRoot, "windsor.yaml")
			if path != expectedPath {
				t.Errorf("Expected config path %q, got %q", expectedPath, path)
			}
			return nil
		}
		pipeline.configHandler = mockConfigHandler

		pipeline.shims = NewShims()

		// Create a test config file
		configPath := filepath.Join(projectRoot, "windsor.yaml")
		if err := os.WriteFile(configPath, []byte("test: config"), 0644); err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then no error should be returned and config should be loaded
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !loadConfigCalled {
			t.Error("Expected loadConfig to be called on config handler")
		}
	})

	t.Run("ReturnsErrorWhenGetProjectRootFails", func(t *testing.T) {
		// Given a BasePipeline with failing shell
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}
		pipeline.shell = mockShell

		mockConfigHandler := config.NewMockConfigHandler()
		pipeline.configHandler = mockConfigHandler

		pipeline.shims = NewShims()

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}
		if !strings.Contains(err.Error(), "error retrieving project root") {
			t.Errorf("Expected 'error retrieving project root' in error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenLoadConfigFails", func(t *testing.T) {
		// Given a BasePipeline with config handler that fails to load
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		projectRoot := t.TempDir()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}
		pipeline.shell = mockShell

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			return fmt.Errorf("load config error")
		}
		pipeline.configHandler = mockConfigHandler

		pipeline.shims = NewShims()

		// Create a test config file
		configPath := filepath.Join(projectRoot, "windsor.yaml")
		if err := os.WriteFile(configPath, []byte("test: config"), 0644); err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when loadConfig fails")
		}
		if !strings.Contains(err.Error(), "error loading config file") {
			t.Errorf("Expected 'error loading config file' in error, got %v", err)
		}
	})

	t.Run("SkipsLoadingWhenNoConfigFileExists", func(t *testing.T) {
		// Given a BasePipeline with no config file
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		projectRoot := t.TempDir()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}
		pipeline.shell = mockShell

		mockConfigHandler := config.NewMockConfigHandler()
		loadConfigCalled := false
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			loadConfigCalled = true
			return nil
		}
		pipeline.configHandler = mockConfigHandler

		pipeline.shims = NewShims()

		// When loadConfig is called (no config file exists)
		err := pipeline.loadConfig()

		// Then no error should be returned and loadConfig should not be called
		if err != nil {
			t.Errorf("Expected no error when no config file exists, got %v", err)
		}
		if loadConfigCalled {
			t.Error("Expected loadConfig not to be called when no config file exists")
		}
	})
}

// =============================================================================
// Test Private Methods - withEnvPrinters
// =============================================================================

func TestBasePipeline_withEnvPrinters(t *testing.T) {
	t.Run("CreatesWindsorEnvPrinterByDefault", func(t *testing.T) {
		// Given a base pipeline with minimal configuration
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating env printers
		envPrinters, err := pipeline.withEnvPrinters()

		// Then no error should be returned and Windsor env printer should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(envPrinters) != 1 {
			t.Errorf("Expected 1 env printer, got %d", len(envPrinters))
		}
	})

	t.Run("CreatesMultipleEnvPrintersWhenEnabled", func(t *testing.T) {
		// Given a base pipeline with multiple services enabled
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "aws.enabled":
				return true
			case "azure.enabled":
				return true
			case "docker.enabled":
				return true
			case "cluster.enabled":
				return true
			case "terraform.enabled":
				return true
			default:
				return false
			}
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "cluster.provider":
				return "talos"
			default:
				return ""
			}
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating env printers
		envPrinters, err := pipeline.withEnvPrinters()

		// Then no error should be returned and multiple env printers should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// Should have AWS, Azure, Docker, Kube, Talos, Terraform, and Windsor
		if len(envPrinters) != 7 {
			t.Errorf("Expected 7 env printers, got %d", len(envPrinters))
		}
	})

	t.Run("CreatesOmniAndTalosEnvPrintersWhenOmniProvider", func(t *testing.T) {
		// Given a base pipeline with omni cluster provider
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "cluster.provider":
				return "omni"
			default:
				return ""
			}
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating env printers
		envPrinters, err := pipeline.withEnvPrinters()

		// Then no error should be returned and Omni, Talos, and Windsor env printers should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// Should have Omni, Talos, and Windsor
		if len(envPrinters) != 3 {
			t.Errorf("Expected 3 env printers, got %d", len(envPrinters))
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a base pipeline with nil config handler
		pipeline := NewBasePipeline()
		pipeline.configHandler = nil

		// When creating env printers
		envPrinters, err := pipeline.withEnvPrinters()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "config handler not initialized" {
			t.Errorf("Expected 'config handler not initialized', got: %v", err)
		}
		if envPrinters != nil {
			t.Error("Expected nil env printers")
		}
	})
}

// =============================================================================
// Test Private Methods - withSecretsProviders
// =============================================================================

func TestBasePipeline_withSecretsProviders(t *testing.T) {
	t.Run("ReturnsEmptyWhenNoSecretsConfigured", func(t *testing.T) {
		// Given a base pipeline with no secrets configuration
		pipeline := NewBasePipeline()

		tmpDir := t.TempDir()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetFunc = func(key string) any {
			return nil
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.shims = NewShims()

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then no error should be returned and no providers should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(secretsProviders) != 0 {
			t.Errorf("Expected 0 secrets providers, got %d", len(secretsProviders))
		}
	})

	t.Run("CreatesSopsProviderWhenSecretsFileExists", func(t *testing.T) {
		// Given a base pipeline with secrets file
		pipeline := NewBasePipeline()

		tmpDir := t.TempDir()
		secretsFile := filepath.Join(tmpDir, "secrets.enc.yaml")
		if err := os.WriteFile(secretsFile, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create secrets file: %v", err)
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetFunc = func(key string) any {
			return nil
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.shims = NewShims()
		pipeline.injector = di.NewInjector()

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then no error should be returned and SOPS provider should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(secretsProviders))
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a base pipeline with nil config handler
		pipeline := NewBasePipeline()
		pipeline.configHandler = nil

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "config handler not initialized" {
			t.Errorf("Expected 'config handler not initialized', got: %v", err)
		}
		if secretsProviders != nil {
			t.Error("Expected nil secrets providers")
		}
	})

	t.Run("ReturnsErrorWhenGetConfigRootFails", func(t *testing.T) {
		// Given a base pipeline with failing config root
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}
		pipeline.configHandler = mockConfigHandler

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting config root") {
			t.Errorf("Expected 'error getting config root' in error, got: %v", err)
		}
		if secretsProviders != nil {
			t.Error("Expected nil secrets providers")
		}
	})

	t.Run("CreatesOnePasswordSDKProviderWhenServiceAccountTokenSet", func(t *testing.T) {
		// Given a base pipeline with OnePassword vaults and service account token
		pipeline := NewBasePipeline()

		tmpDir := t.TempDir()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetFunc = func(key string) any {
			if key == "contexts.test-context.secrets.onepassword.vaults" {
				return map[string]secretsConfigType.OnePasswordVault{
					"vault1": {
						Name: "test-vault",
					},
				}
			}
			return nil
		}
		pipeline.configHandler = mockConfigHandler

		mockShims := NewShims()
		// Override Getenv to simulate service account token
		originalGetenv := mockShims.Getenv
		mockShims.Getenv = func(key string) string {
			if key == "OP_SERVICE_ACCOUNT_TOKEN" {
				return "test-token"
			}
			return originalGetenv(key)
		}
		pipeline.shims = mockShims
		pipeline.injector = di.NewInjector()

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then no error should be returned and OnePassword SDK provider should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(secretsProviders))
		}
	})

	t.Run("CreatesOnePasswordCLIProviderWhenNoServiceAccountToken", func(t *testing.T) {
		// Given a base pipeline with OnePassword vaults and no service account token
		pipeline := NewBasePipeline()

		tmpDir := t.TempDir()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetFunc = func(key string) any {
			if key == "contexts.test-context.secrets.onepassword.vaults" {
				return map[string]secretsConfigType.OnePasswordVault{
					"vault1": {
						Name: "test-vault",
					},
				}
			}
			return nil
		}
		pipeline.configHandler = mockConfigHandler

		mockShims := NewShims()
		// Override Getenv to simulate no service account token
		originalGetenv := mockShims.Getenv
		mockShims.Getenv = func(key string) string {
			if key == "OP_SERVICE_ACCOUNT_TOKEN" {
				return ""
			}
			return originalGetenv(key)
		}
		pipeline.shims = mockShims
		pipeline.injector = di.NewInjector()

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then no error should be returned and OnePassword CLI provider should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(secretsProviders))
		}
	})

	t.Run("CreatesSecretsProviderForSecretsDotEncDotYmlFile", func(t *testing.T) {
		// Given a base pipeline with secrets.enc.yml file
		pipeline := NewBasePipeline()

		tmpDir := t.TempDir()
		secretsFile := filepath.Join(tmpDir, "secrets.enc.yml")
		if err := os.WriteFile(secretsFile, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create secrets file: %v", err)
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetFunc = func(key string) any {
			return nil
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.shims = NewShims()
		pipeline.injector = di.NewInjector()

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then no error should be returned and SOPS provider should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(secretsProviders))
		}
	})
}

// =============================================================================
// Test Private Methods - withServices
// =============================================================================

func TestBasePipeline_withServices(t *testing.T) {
	t.Run("ReturnsEmptyWhenDockerDisabled", func(t *testing.T) {
		// Given a base pipeline with Docker disabled
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return false
			}
			return false
		}
		pipeline.configHandler = mockConfigHandler

		// When creating services
		services, err := pipeline.withServices()

		// Then no error should be returned and no services should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(services) != 0 {
			t.Errorf("Expected 0 services, got %d", len(services))
		}
	})

	t.Run("CreatesMultipleServicesWhenDockerEnabled", func(t *testing.T) {
		// Given a base pipeline with Docker and multiple services enabled
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "docker.enabled":
				return true
			case "dns.enabled":
				return true
			case "git.livereload.enabled":
				return true
			case "aws.localstack.enabled":
				return true
			default:
				return false
			}
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "cluster.provider":
				return "talos"
			default:
				return ""
			}
		}
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int {
			switch key {
			case "cluster.control_plane.count":
				return 2
			case "cluster.worker.count":
				return 3
			default:
				return 1
			}
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating services
		services, err := pipeline.withServices()

		// Then no error should be returned and multiple services should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// Should have DNS, Git, AWS, 2 control plane, and 3 worker services
		if len(services) != 8 {
			t.Errorf("Expected 8 services, got %d", len(services))
		}
	})

	t.Run("CreatesRegistryServicesWhenDockerRegistriesConfigured", func(t *testing.T) {
		// Given a base pipeline with Docker registries configured
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}
		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"registry1": {},
						"registry2": {},
					},
				},
			}
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating services
		services, err := pipeline.withServices()

		// Then no error should be returned and registry services should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(services) != 2 {
			t.Errorf("Expected 2 services, got %d", len(services))
		}
	})

	t.Run("CreatesOmniClusterServices", func(t *testing.T) {
		// Given a base pipeline with Omni cluster provider
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.provider" {
				return "omni"
			}
			return ""
		}
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int {
			switch key {
			case "cluster.control_plane.count":
				return 1
			case "cluster.worker.count":
				return 2
			default:
				return 1
			}
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating services
		services, err := pipeline.withServices()

		// Then no error should be returned and cluster services should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// Should have 1 control plane and 2 worker services
		if len(services) != 3 {
			t.Errorf("Expected 3 services, got %d", len(services))
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a base pipeline with nil config handler
		pipeline := NewBasePipeline()
		pipeline.configHandler = nil

		// When creating services
		services, err := pipeline.withServices()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "config handler not initialized" {
			t.Errorf("Expected 'config handler not initialized', got: %v", err)
		}
		if services != nil {
			t.Error("Expected nil services")
		}
	})
}
