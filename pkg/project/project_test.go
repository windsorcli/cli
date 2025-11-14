package project

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/context/shell"
	"github.com/windsorcli/cli/pkg/context/tools"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/workstation"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Workstation   *workstation.Workstation
	Composer      *composer.Composer
	Provisioner   *provisioner.Provisioner
}

func setupMocks(t *testing.T) *Mocks {
	t.Helper()

	tmpDir := t.TempDir()
	configRoot := filepath.Join(tmpDir, "contexts", "test-context")

	injector := di.NewInjector()
	configHandler := config.NewMockConfigHandler()
	mockShell := shell.NewMockShell()

	configHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "cluster.driver":
			return "talos"
		case "provider":
			return ""
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}

	configHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		return false
	}

	configHandler.GetContextFunc = func() string {
		return "test-context"
	}

	configHandler.IsDevModeFunc = func(contextName string) bool {
		return false
	}

	configHandler.LoadConfigFunc = func() error {
		return nil
	}

	configHandler.SetFunc = func(key string, value any) error {
		return nil
	}

	configHandler.GenerateContextIDFunc = func() error {
		return nil
	}

	configHandler.SaveConfigFunc = func(hasSetFlags ...bool) error {
		return nil
	}

	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	mockShell.GetSessionTokenFunc = func() (string, error) {
		return "test-session-token", nil
	}

	configHandler.GetConfigRootFunc = func() (string, error) {
		return configRoot, nil
	}

	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.CheckFunc = func() error {
		return nil
	}

	injector.Register("shell", mockShell)
	injector.Register("configHandler", configHandler)
	injector.Register("toolsManager", mockToolsManager)

	baseCtx := &context.ExecutionContext{
		Injector: injector,
	}

	ctx, err := context.NewContext(baseCtx)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	provCtx := &provisioner.ProvisionerExecutionContext{
		ExecutionContext: *ctx,
	}
	prov := provisioner.NewProvisioner(provCtx)

	composerCtx := &composer.ComposerExecutionContext{
		ExecutionContext: *ctx,
	}
	comp := composer.NewComposer(composerCtx)

	return &Mocks{
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         mockShell,
		Provisioner:   prov,
		Composer:      comp,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewProject(t *testing.T) {
	t.Run("CreatesProjectWithDependencies", func(t *testing.T) {
		mocks := setupMocks(t)

		proj, err := NewProject(mocks.Injector, "test-context")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if proj == nil {
			t.Fatal("Expected Project to be created")
		}

		if proj.Context == nil {
			t.Error("Expected Context to be set")
		}

		if proj.Provisioner == nil {
			t.Error("Expected Provisioner to be set")
		}

		if proj.Composer == nil {
			t.Error("Expected Composer to be set")
		}

		if proj.Context.ContextName != "test-context" {
			t.Errorf("Expected ContextName to be 'test-context', got: %s", proj.Context.ContextName)
		}
	})

	t.Run("UsesProvidedContextName", func(t *testing.T) {
		mocks := setupMocks(t)

		proj, err := NewProject(mocks.Injector, "custom-context")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if proj.Context.ContextName != "custom-context" {
			t.Errorf("Expected ContextName to be 'custom-context', got: %s", proj.Context.ContextName)
		}
	})

	t.Run("UsesConfigContextWhenContextNameEmpty", func(t *testing.T) {
		mocks := setupMocks(t)

		proj, err := NewProject(mocks.Injector, "")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if proj.Context.ContextName != "test-context" {
			t.Errorf("Expected ContextName to be 'test-context', got: %s", proj.Context.ContextName)
		}
	})

	t.Run("UsesLocalWhenContextNameAndConfigContextEmpty", func(t *testing.T) {
		mocks := setupMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GetContextFunc = func() string {
			return ""
		}

		proj, err := NewProject(mocks.Injector, "")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if proj.Context.ContextName != "local" {
			t.Errorf("Expected ContextName to be 'local', got: %s", proj.Context.ContextName)
		}
	})

	t.Run("CreatesWorkstationWhenDevMode", func(t *testing.T) {
		mocks := setupMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		proj, err := NewProject(mocks.Injector, "test-context")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if proj.Workstation == nil {
			t.Error("Expected Workstation to be created in dev mode")
		}
	})

	t.Run("SkipsWorkstationWhenNotDevMode", func(t *testing.T) {
		mocks := setupMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return false
		}

		proj, err := NewProject(mocks.Injector, "test-context")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if proj.Workstation != nil {
			t.Error("Expected Workstation to be nil when not in dev mode")
		}
	})

	t.Run("ErrorOnContextInitializationFailure", func(t *testing.T) {
		var injector di.Injector

		proj, err := NewProject(injector, "test-context")

		if err == nil {
			t.Error("Expected error for context initialization failure")
			return
		}

		if proj != nil {
			t.Error("Expected Project to be nil on error")
		}

		if !strings.Contains(err.Error(), "failed to initialize context") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProject_Configure(t *testing.T) {
	t.Run("SuccessWithNilFlagOverrides", func(t *testing.T) {
		mocks := setupMocks(t)
		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		err = proj.Configure(nil)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithEmptyFlagOverrides", func(t *testing.T) {
		mocks := setupMocks(t)
		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		err = proj.Configure(make(map[string]any))

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithFlagOverrides", func(t *testing.T) {
		mocks := setupMocks(t)
		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		flagOverrides := map[string]any{
			"provider": "aws",
			"key":      "value",
		}

		err = proj.Configure(flagOverrides)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SetsGenericProviderInDevModeWhenProviderNotSet", func(t *testing.T) {
		mocks := setupMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "provider" {
				return ""
			}
			return ""
		}

		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		providerSet := false
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "provider" && value == "generic" {
				providerSet = true
			}
			return nil
		}

		err = proj.Configure(nil)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !providerSet {
			t.Error("Expected provider to be set to 'generic' in dev mode")
		}
	})

	t.Run("SkipsGenericProviderWhenProviderAlreadySet", func(t *testing.T) {
		mocks := setupMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "provider" {
				return "aws"
			}
			return ""
		}

		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		genericSet := false
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "provider" && value == "generic" {
				genericSet = true
			}
			return nil
		}

		err = proj.Configure(nil)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if genericSet {
			t.Error("Expected provider not to be set to 'generic' when already set")
		}
	})

	t.Run("ErrorOnApplyProviderDefaultsFailure", func(t *testing.T) {
		mocks := setupMocks(t)
		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "cluster.driver" {
				return fmt.Errorf("set cluster.driver failed")
			}
			return nil
		}

		err = proj.Configure(map[string]any{"provider": "aws"})

		if err == nil {
			t.Error("Expected error for ApplyProviderDefaults failure")
			return
		}
	})

	t.Run("ErrorOnLoadConfigFailure", func(t *testing.T) {
		mocks := setupMocks(t)
		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.LoadConfigFunc = func() error {
			return fmt.Errorf("load config failed")
		}

		err = proj.Configure(nil)

		if err == nil {
			t.Error("Expected error for LoadConfig failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorOnSetFailure", func(t *testing.T) {
		mocks := setupMocks(t)
		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.SetFunc = func(key string, value any) error {
			return fmt.Errorf("set failed")
		}

		err = proj.Configure(map[string]any{"key": "value"})

		if err == nil {
			t.Error("Expected error for Set failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to set") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestProject_Initialize(t *testing.T) {
	t.Run("SuccessWithoutWorkstation", func(t *testing.T) {
		mocks := setupMocks(t)
		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		err = proj.Initialize(false)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithWorkstation", func(t *testing.T) {
		mocks := setupMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		if proj.Workstation == nil {
			t.Fatal("Expected workstation to be created")
		}

		proj.Workstation.NetworkManager = nil

		err = proj.Initialize(false)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithOverwriteTrue", func(t *testing.T) {
		mocks := setupMocks(t)
		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		err = proj.Initialize(true)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorOnGenerateContextIDFailure", func(t *testing.T) {
		mocks := setupMocks(t)
		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GenerateContextIDFunc = func() error {
			return fmt.Errorf("generate context ID failed")
		}

		err = proj.Initialize(false)

		if err == nil {
			t.Error("Expected error for GenerateContextID failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to generate context ID") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorOnSaveConfigFailure", func(t *testing.T) {
		mocks := setupMocks(t)
		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.SaveConfigFunc = func(hasSetFlags ...bool) error {
			return fmt.Errorf("save config failed")
		}

		err = proj.Initialize(false)

		if err == nil {
			t.Error("Expected error for SaveConfig failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to save config") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

}

func TestProject_PerformCleanup(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)
		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		cleanCalled := false
		mockConfig.CleanFunc = func() error {
			cleanCalled = true
			return nil
		}

		err = proj.PerformCleanup()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !cleanCalled {
			t.Error("Expected Clean to be called")
		}
	})

	t.Run("ErrorOnCleanFailure", func(t *testing.T) {
		mocks := setupMocks(t)
		proj, err := NewProject(mocks.Injector, "test-context")
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.CleanFunc = func() error {
			return fmt.Errorf("clean failed")
		}

		err = proj.PerformCleanup()

		if err == nil {
			t.Error("Expected error for Clean failure")
			return
		}

		if !strings.Contains(err.Error(), "error cleaning up context specific artifacts") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}
