package project

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/workstation"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/services"
)

// =============================================================================
// Test Setup
// =============================================================================

// ProjectTestMocks contains all the mock dependencies for testing the Project
type ProjectTestMocks struct {
	Runtime       *runtime.Runtime
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Workstation   *workstation.Workstation
	Composer      *composer.Composer
	Provisioner   *provisioner.Provisioner
}

// setupProjectMocks creates mock components for testing the Project with optional overrides
func setupProjectMocks(t *testing.T, opts ...func(*ProjectTestMocks)) *ProjectTestMocks {
	t.Helper()

	tmpDir := t.TempDir()
	configRoot := filepath.Join(tmpDir, "contexts", "test-context")

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

	rtOpts := []*runtime.Runtime{
		{
			Shell:         mockShell,
			ConfigHandler: configHandler,
			ProjectRoot:   tmpDir,
			ToolsManager:  mockToolsManager,
		},
	}

	rt, err := runtime.NewRuntime(rtOpts...)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	comp := composer.NewComposer(rt)
	prov := provisioner.NewProvisioner(rt, comp.BlueprintHandler)

	mocks := &ProjectTestMocks{
		Runtime:       rt,
		ConfigHandler: configHandler,
		Shell:         mockShell,
		Provisioner:   prov,
		Composer:      comp,
	}

	// Apply any overrides
	for _, opt := range opts {
		opt(mocks)
	}

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewProject(t *testing.T) {
	t.Run("CreatesProjectWithDependencies", func(t *testing.T) {
		mocks := setupProjectMocks(t)

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if proj == nil {
			t.Fatal("Expected Project to be created")
		}

		if proj.Runtime == nil {
			t.Error("Expected Runtime to be set")
		}

		if proj.Provisioner == nil {
			t.Error("Expected Provisioner to be set")
		}

		if proj.Composer == nil {
			t.Error("Expected Composer to be set")
		}

		if proj.Runtime.ContextName != "test-context" {
			t.Errorf("Expected ContextName to be 'test-context', got: %s", proj.Runtime.ContextName)
		}
	})

	t.Run("UsesProvidedContextName", func(t *testing.T) {
		mocks := setupProjectMocks(t)

		proj, err := NewProject("custom-context", &Project{Runtime: mocks.Runtime})

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if proj.Runtime.ContextName != "custom-context" {
			t.Errorf("Expected ContextName to be 'custom-context', got: %s", proj.Runtime.ContextName)
		}
	})

	t.Run("UsesConfigContextWhenContextNameEmpty", func(t *testing.T) {
		mocks := setupProjectMocks(t)

		proj, err := NewProject("", &Project{Runtime: mocks.Runtime})

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if proj.Runtime.ContextName != "test-context" {
			t.Errorf("Expected ContextName to be 'test-context', got: %s", proj.Runtime.ContextName)
		}
	})

	t.Run("UsesLocalWhenContextNameAndConfigContextEmpty", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GetContextFunc = func() string {
			return ""
		}

		proj, err := NewProject("", &Project{Runtime: mocks.Runtime})

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if proj.Runtime.ContextName != "local" {
			t.Errorf("Expected ContextName to be 'local', got: %s", proj.Runtime.ContextName)
		}
	})

	t.Run("CreatesWorkstationWhenDevMode", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if proj.Workstation == nil {
			t.Error("Expected Workstation to be created in dev mode")
		}
	})

	t.Run("SkipsWorkstationWhenNotDevMode", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return false
		}

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if proj.Workstation != nil {
			t.Error("Expected Workstation to be nil when not in dev mode")
		}
	})

	t.Run("ErrorOnContextInitializationFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockShell := mocks.Shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("failed to get project root")
		}

		rt, err := runtime.NewRuntime(&runtime.Runtime{
			Shell:       mockShell,
			ProjectRoot: "",
		})
		if err == nil {
			proj, err := NewProject("test-context", &Project{Runtime: rt})
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
		}
	})

	t.Run("ErrorOnApplyConfigDefaultsFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsLoadedFunc = func() bool {
			return false
		}
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "dev" {
				return fmt.Errorf("set dev failed")
			}
			return nil
		}

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		if err == nil {
			t.Fatal("Expected error for ApplyConfigDefaults failure")
		}
		if proj != nil {
			t.Error("Expected Project to be nil on error")
		}
	})

	t.Run("ErrorOnWorkstationCreationFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		mocks.Runtime.Shell = nil

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		if err == nil {
			t.Fatal("Expected error for workstation creation failure")
		}
		if proj != nil {
			t.Error("Expected Project to be nil on error")
		}
		if !strings.Contains(err.Error(), "failed to create workstation") {
			t.Errorf("Expected error about workstation creation, got: %v", err)
		}
	})

	t.Run("HandlesComposerOverride", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockComposer := composer.NewComposer(mocks.Runtime)

		proj, err := NewProject("test-context", &Project{
			Runtime:  mocks.Runtime,
			Composer: mockComposer,
		})

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if proj.Composer != mockComposer {
			t.Error("Expected Composer override to be used")
		}
	})

	t.Run("HandlesProvisionerOverride", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler)

		proj, err := NewProject("test-context", &Project{
			Runtime:     mocks.Runtime,
			Provisioner: mockProvisioner,
		})

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if proj.Provisioner != mockProvisioner {
			t.Error("Expected Provisioner override to be used")
		}
	})

	t.Run("HandlesWorkstationOverride", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		mockWorkstation, err := workstation.NewWorkstation(mocks.Runtime)
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		proj, err := NewProject("test-context", &Project{
			Runtime:     mocks.Runtime,
			Workstation: mockWorkstation,
		})

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if proj.Workstation != mockWorkstation {
			t.Error("Expected Workstation override to be used")
		}
	})

	t.Run("CreatesRuntimeWhenNoOverrides", func(t *testing.T) {
		proj, err := NewProject("test-context")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if proj == nil {
			t.Fatal("Expected Project to be created")
		}
		if proj.Runtime == nil {
			t.Error("Expected Runtime to be created")
		}
		if proj.Composer == nil {
			t.Error("Expected Composer to be created")
		}
		if proj.Provisioner == nil {
			t.Error("Expected Provisioner to be created")
		}
	})

	t.Run("CreatesRuntimeWhenOverridesHasNilRuntime", func(t *testing.T) {
		proj, err := NewProject("test-context", &Project{Runtime: nil})

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if proj == nil {
			t.Fatal("Expected Project to be created")
		}
		if proj.Runtime == nil {
			t.Error("Expected Runtime to be created")
		}
	})

	t.Run("ErrorOnRuntimeCreationFailure", func(t *testing.T) {
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("failed to get project root")
		}

		proj, err := NewProject("test-context", &Project{
			Runtime: &runtime.Runtime{
				Shell: mockShell,
			},
		})

		if err == nil {
			t.Fatal("Expected error for runtime creation failure")
		}
		if proj != nil {
			t.Error("Expected Project to be nil on error")
		}
		if !strings.Contains(err.Error(), "failed to initialize context") && !strings.Contains(err.Error(), "failed to get project root") && !strings.Contains(err.Error(), "config handler not available") {
			t.Errorf("Expected error about initializing context, getting project root, or config handler, got: %v", err)
		}
	})

}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProject_Configure(t *testing.T) {
	t.Run("SuccessWithNilFlagOverrides", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		err = proj.Configure(nil)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithEmptyFlagOverrides", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		err = proj.Configure(make(map[string]any))

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithFlagOverrides", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
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
		mocks := setupProjectMocks(t)
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

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
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
		mocks := setupProjectMocks(t)
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

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
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
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "cluster.driver" {
				return fmt.Errorf("set cluster.driver failed")
			}
			return nil
		}

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		err = proj.Configure(map[string]any{"provider": "aws"})

		if err == nil {
			t.Error("Expected error for ApplyProviderDefaults failure")
			return
		}
	})

	t.Run("ErrorOnLoadConfigFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.LoadConfigFunc = func() error {
			return fmt.Errorf("load config failed")
		}

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
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
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.SetFunc = func(key string, value any) error {
			return fmt.Errorf("set failed")
		}

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
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
		mocks := setupProjectMocks(t)
		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		err = proj.Initialize(false)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithWorkstation", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
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
		mocks := setupProjectMocks(t)
		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		err = proj.Initialize(true)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorOnGenerateContextIDFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GenerateContextIDFunc = func() error {
			return fmt.Errorf("generate context ID failed")
		}

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
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
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.SaveConfigFunc = func(hasSetFlags ...bool) error {
			return fmt.Errorf("save config failed")
		}

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
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

	t.Run("ErrorOnLoadBlueprintFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
		mockBlueprintHandler.LoadBlueprintFunc = func() error {
			return fmt.Errorf("load blueprint failed")
		}
		proj.Composer.BlueprintHandler = mockBlueprintHandler

		err = proj.Initialize(false)

		if err == nil {
			t.Error("Expected error for LoadBlueprint failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to load blueprint data") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorOnGenerateFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
		mockBlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			return fmt.Errorf("generate failed")
		}
		proj.Composer.BlueprintHandler = mockBlueprintHandler

		err = proj.Initialize(false)

		if err == nil {
			t.Error("Expected error for Generate failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to generate infrastructure") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorOnPrepareToolsFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.CheckFunc = func() error {
			return fmt.Errorf("prepare tools failed")
		}
		proj.Runtime.ToolsManager = mockToolsManager

		err = proj.Initialize(false)

		if err == nil {
			t.Error("Expected error for PrepareTools failure")
			return
		}

		if !strings.Contains(err.Error(), "error checking tools") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorOnAssignIPsFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		if proj.Workstation == nil {
			t.Fatal("Expected workstation to be created")
		}

		mockNetworkManager := network.NewMockNetworkManager()
		mockNetworkManager.AssignIPsFunc = func(services []services.Service) error {
			return fmt.Errorf("assign IPs failed")
		}
		proj.Workstation.NetworkManager = mockNetworkManager

		err = proj.Initialize(false)

		if err == nil {
			t.Error("Expected error for AssignIPs failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to assign IPs to network manager") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

}

func TestProject_PerformCleanup(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		cleanCalled := false
		mockConfig.CleanFunc = func() error {
			cleanCalled = true
			return nil
		}

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
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
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.CleanFunc = func() error {
			return fmt.Errorf("clean failed")
		}

		proj, err := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
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
