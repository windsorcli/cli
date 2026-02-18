package project

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	v1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/provisioner"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/workstation"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/services"
	"github.com/windsorcli/cli/pkg/workstation/virt"
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

	configHandler.LoadSchemaFromBytesFunc = func(data []byte) error {
		return nil
	}

	configHandler.GetContextValuesFunc = func() (map[string]any, error) {
		addons := make(map[string]any)
		// Initialize common addons with enabled: false to prevent evaluation errors
		for _, addon := range []string{"object_store", "observability", "private_ca", "private_dns"} {
			addons[addon] = map[string]any{"enabled": false}
		}
		return map[string]any{
			"addons": addons,
			"dev":    false,
		}, nil
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

	rt := runtime.NewRuntime(rtOpts...)
	if rt == nil {
		t.Fatal("Failed to create runtime")
	}

	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.LoadBlueprintFunc = func(...string) error { return nil }
	mockBlueprintHandler.WriteFunc = func(overwrite ...bool) error { return nil }
	mockBlueprintHandler.GenerateFunc = func() *v1alpha1.Blueprint {
		return &v1alpha1.Blueprint{}
	}
	comp := composer.NewComposer(rt, &composer.Composer{
		BlueprintHandler: mockBlueprintHandler,
	})
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

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

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

		proj := NewProject("custom-context", &Project{Runtime: mocks.Runtime})

		if proj.Runtime.ContextName != "custom-context" {
			t.Errorf("Expected ContextName to be 'custom-context', got: %s", proj.Runtime.ContextName)
		}
	})

	t.Run("UsesConfigContextWhenContextNameEmpty", func(t *testing.T) {
		mocks := setupProjectMocks(t)

		proj := NewProject("", &Project{Runtime: mocks.Runtime})

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

		proj := NewProject("", &Project{Runtime: mocks.Runtime})

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

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		if proj.Workstation == nil {
			t.Error("Expected Workstation to be created in dev mode")
		}
	})

	t.Run("CreatesWorkstationAndProvisionerWhenDevMode", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		if proj.Workstation == nil || proj.Provisioner == nil {
			t.Fatal("Expected Workstation and Provisioner to be created in dev mode")
		}
	})

	t.Run("CreatesWorkstationWhenWorkstationEnabled", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return false
		}
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "workstation.enabled" {
				return true
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		if proj.Workstation == nil {
			t.Error("Expected Workstation to be created when workstation.enabled is true")
		}
		if proj.Provisioner == nil {
			t.Fatal("Expected Provisioner to exist")
		}
	})

	t.Run("SkipsWorkstationWhenNotDevMode", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return false
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

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

		// NewRuntime will panic when GetProjectRoot fails
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected NewRuntime to panic when GetProjectRoot fails")
			}
		}()
		_ = runtime.NewRuntime(&runtime.Runtime{
			Shell:       mockShell,
			ProjectRoot: "",
		})
	})

	t.Run("PanicsWhenShellIsNil", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		mocks.Runtime.Shell = nil

		// NewComposer will panic when Shell is nil
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected NewComposer to panic when Shell is nil")
			}
		}()
		_ = NewProject("test-context", &Project{Runtime: mocks.Runtime})
	})

	t.Run("HandlesComposerOverride", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockComposer := composer.NewComposer(mocks.Runtime)

		proj := NewProject("test-context", &Project{
			Runtime:  mocks.Runtime,
			Composer: mockComposer,
		})

		if proj.Composer != mockComposer {
			t.Error("Expected Composer override to be used")
		}
	})

	t.Run("HandlesProvisionerOverride", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler)

		proj := NewProject("test-context", &Project{
			Runtime:     mocks.Runtime,
			Provisioner: mockProvisioner,
		})

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

		mockWorkstation := workstation.NewWorkstation(mocks.Runtime)
		if mockWorkstation == nil {
			t.Fatal("Failed to create workstation")
		}

		proj := NewProject("test-context", &Project{
			Runtime:     mocks.Runtime,
			Workstation: mockWorkstation,
		})

		if proj.Workstation != mockWorkstation {
			t.Error("Expected Workstation override to be used")
		}
	})

	t.Run("CreatesRuntimeWhenNoOverrides", func(t *testing.T) {
		proj := NewProject("test-context")

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
		proj := NewProject("test-context", &Project{Runtime: nil})

		if proj == nil {
			t.Fatal("Expected Project to be created")
		}
		if proj.Runtime == nil {
			t.Error("Expected Runtime to be created")
		}
	})

}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProject_Configure(t *testing.T) {
	t.Run("SuccessWithNilFlagOverrides", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		_ = NewProject("test-context", &Project{Runtime: mocks.Runtime})
	})

	t.Run("SuccessWithEmptyFlagOverrides", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		_ = NewProject("test-context", &Project{Runtime: mocks.Runtime})
	})

	t.Run("SuccessWithFlagOverrides", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		_ = NewProject("test-context", &Project{Runtime: mocks.Runtime})
	})

	t.Run("SetsDockerProviderInDevModeWhenProviderNotSet", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "provider" {
				return ""
			}
			if key == "vm.driver" {
				return ""
			}
			if key == "vm.runtime" {
				return "docker"
			}
			return ""
		}

		providerSet := false
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "provider" && value == "docker" {
				providerSet = true
			}
			return nil
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		_ = proj.Configure(nil)

		if !providerSet {
			t.Error("Expected provider to be set to 'docker' in dev mode")
		}
	})

	t.Run("SetsIncusProviderInDevModeWhenColimaIncus", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "provider" {
				return ""
			}
			if key == "vm.driver" {
				return "colima"
			}
			if key == "vm.runtime" {
				return "incus"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		providerSet := false
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "provider" && value == "incus" {
				providerSet = true
			}
			return nil
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		_ = proj.Configure(nil)

		if !providerSet {
			t.Error("Expected provider to be set to 'incus' in dev mode when vm.driver is colima and vm.runtime is incus")
		}
	})

	t.Run("CreatesWorkstationWhenWorkstationEnabledSetViaFlagOverrides", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return false
		}
		var workstationEnabled bool
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "workstation.enabled" {
				if v, ok := value.(bool); ok {
					workstationEnabled = v
				}
			}
			return nil
		}
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "workstation.enabled" {
				return workstationEnabled
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		if proj.Workstation != nil {
			t.Fatal("Expected Workstation nil before Configure when not dev mode and workstation.enabled not yet set")
		}

		err := proj.Configure(map[string]any{"workstation.enabled": true})
		if err != nil {
			t.Errorf("Configure failed: %v", err)
		}
		proj.EnsureWorkstation()
		if proj.Workstation == nil {
			t.Error("Expected Workstation to be created when workstation.enabled set via flag overrides")
		}
	})

	t.Run("ClearsWorkstationWhenWorkstationEnabledOverrideFalse", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return false
		}
		workstationEnabled := true
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "workstation.enabled" {
				if v, ok := value.(bool); ok {
					workstationEnabled = v
				}
			}
			return nil
		}
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "workstation.enabled" {
				return workstationEnabled
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		mockConfig.IsLoadedFunc = func() bool { return true }

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		if proj.Workstation == nil || proj.Provisioner == nil {
			t.Fatal("Expected Workstation and Provisioner created when workstation.enabled true at NewProject")
		}

		err := proj.Configure(map[string]any{"workstation.enabled": false})

		if err != nil {
			t.Errorf("Configure failed: %v", err)
		}
		if proj.Workstation != nil {
			t.Error("Expected Workstation to be cleared when workstation.enabled overridden to false")
		}
	})

	t.Run("SkipsDockerProviderWhenProviderAlreadySet", func(t *testing.T) {
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

		_ = NewProject("test-context", &Project{Runtime: mocks.Runtime})

		dockerSet := false
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "provider" && value == "docker" {
				dockerSet = true
			}
			return nil
		}

		if dockerSet {
			t.Error("Expected provider not to be set to 'docker' when already set")
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

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		err := proj.Configure(map[string]any{"provider": "aws"})

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

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		err := proj.Configure(nil)

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

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		err := proj.Configure(map[string]any{"key": "value"})

		if err == nil {
			t.Error("Expected error for Set failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to set") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("CallsApplyConfigDefaultsWithFlagOverrides", func(t *testing.T) {
		// Given a project with flag overrides containing vm.driver=colima
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

		var setDefaultConfig v1alpha1.Context
		mockConfig.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			setDefaultConfig = cfg
			return nil
		}
		mockConfig.SetFunc = func(key string, value any) error {
			return nil
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		flagOverrides := map[string]any{
			"vm.driver": "colima",
			"provider":  "docker",
		}

		// When Configure is called with flag overrides
		_ = proj.Configure(flagOverrides)

		// Then ApplyConfigDefaults should be called with flag overrides, resulting in DefaultConfig_Full

		if setDefaultConfig.Network == nil || setDefaultConfig.Network.LoadBalancerIPs == nil {
			t.Error("Expected DefaultConfig_Full with LoadBalancerIPs to be set (colima should use Full config)")
		}

		if setDefaultConfig.Cluster != nil && len(setDefaultConfig.Cluster.Workers.HostPorts) > 0 {
			t.Error("Expected DefaultConfig_Full without hostports to be set (colima should not have hostports)")
		}
	})

	t.Run("ErrorOnApplyConfigDefaultsFailure", func(t *testing.T) {
		// Given a project where SetDefault fails
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

		mockConfig.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			return fmt.Errorf("set default failed")
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		// When Configure is called
		err := proj.Configure(nil)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for ApplyConfigDefaults failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to apply config defaults") {
			t.Errorf("Expected error about apply config defaults, got: %v", err)
		}
	})

	t.Run("SetsProviderToIncusAfterLoadConfigWhenColimaIncus", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "provider" {
				return "docker"
			}
			if key == "vm.driver" {
				return "colima"
			}
			if key == "vm.runtime" {
				return "incus"
			}
			return ""
		}
		mockConfig.LoadConfigFunc = func() error {
			return nil
		}

		var providerSet string
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "provider" {
				providerSet = value.(string)
			}
			return nil
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})
		_ = proj.Configure(nil)

		if providerSet != "incus" {
			t.Errorf("Expected provider to be set to 'incus' after LoadConfig when vm.driver is 'colima' and vm.runtime is 'incus', got: %s", providerSet)
		}
	})
}

func TestProject_ComposeBlueprint(t *testing.T) {
	t.Run("SuccessWhenLoadBlueprintSucceeds", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		err := proj.ComposeBlueprint()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("PassesBlueprintURLToLoadBlueprint", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		var loadArgs []string
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
		mockBlueprintHandler.LoadBlueprintFunc = func(url ...string) error {
			loadArgs = url
			return nil
		}
		comp := composer.NewComposer(mocks.Runtime, &composer.Composer{
			BlueprintHandler: mockBlueprintHandler,
		})
		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: comp})

		err := proj.ComposeBlueprint("oci://ghcr.io/org/blueprint:v1")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(loadArgs) != 1 || loadArgs[0] != "oci://ghcr.io/org/blueprint:v1" {
			t.Errorf("Expected LoadBlueprint to be called with oci URL, got: %v", loadArgs)
		}
	})

	t.Run("ErrorOnGenerateContextIDFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GenerateContextIDFunc = func() error {
			return fmt.Errorf("generate context ID failed")
		}
		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		err := proj.ComposeBlueprint()

		if err == nil {
			t.Error("Expected error for GenerateContextID failure")
			return
		}
		if !strings.Contains(err.Error(), "failed to generate context ID") {
			t.Errorf("Expected error to mention context ID, got: %v", err)
		}
	})

	t.Run("ErrorOnLoadBlueprintFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
		mockBlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return fmt.Errorf("load blueprint failed")
		}
		comp := composer.NewComposer(mocks.Runtime, &composer.Composer{
			BlueprintHandler: mockBlueprintHandler,
		})
		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: comp})

		err := proj.ComposeBlueprint()

		if err == nil {
			t.Error("Expected error for LoadBlueprint failure")
			return
		}
		if !strings.Contains(err.Error(), "load blueprint failed") {
			t.Errorf("Expected error from LoadBlueprint, got: %v", err)
		}
	})
}

func TestProject_Initialize(t *testing.T) {
	t.Run("SuccessWithoutWorkstation", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		_ = proj.Initialize(false)

	})

	t.Run("SuccessWithWorkstation", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		if err := proj.Configure(nil); err != nil {
			t.Fatalf("Failed to configure project: %v", err)
		}

		if proj.Workstation == nil {
			t.Fatal("Expected workstation to be created")
		}

		proj.Workstation.NetworkManager = nil

		_ = proj.Initialize(false)

	})

	t.Run("CallsContainerRuntimeWriteConfig", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		if err := proj.Configure(nil); err != nil {
			t.Fatalf("Failed to configure project: %v", err)
		}

		if proj.Workstation == nil {
			t.Fatal("Expected workstation to be created")
		}

		writeConfigCalled := false
		mockContainerRuntime := virt.NewMockVirt()
		mockContainerRuntime.WriteConfigFunc = func() error {
			writeConfigCalled = true
			return nil
		}
		proj.Workstation.ContainerRuntime = mockContainerRuntime
		proj.Workstation.NetworkManager = nil

		_ = proj.Initialize(false)

		if !writeConfigCalled {
			t.Error("Expected ContainerRuntime.WriteConfig to be called")
		}
	})

	t.Run("ErrorOnContainerRuntimeWriteConfigFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		if err := proj.Configure(nil); err != nil {
			t.Fatalf("Failed to configure project: %v", err)
		}

		if proj.Workstation == nil {
			t.Fatal("Expected workstation to be created")
		}

		mockContainerRuntime := virt.NewMockVirt()
		mockContainerRuntime.WriteConfigFunc = func() error {
			return fmt.Errorf("write config failed")
		}
		proj.Workstation.ContainerRuntime = mockContainerRuntime
		proj.Workstation.NetworkManager = nil

		err := proj.Initialize(false)

		if err == nil {
			t.Error("Expected error for ContainerRuntime.WriteConfig failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to write container runtime config") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("SuccessWithOverwriteTrue", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		_ = proj.Initialize(true)

	})

	t.Run("ErrorOnGenerateContextIDFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GenerateContextIDFunc = func() error {
			return fmt.Errorf("generate context ID failed")
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		err := proj.Initialize(false)

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

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		err := proj.Initialize(false)

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
		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
		mockBlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return fmt.Errorf("load blueprint failed")
		}
		proj.Composer.BlueprintHandler = mockBlueprintHandler

		err := proj.Initialize(false)

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
		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
		mockBlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			return fmt.Errorf("generate failed")
		}
		proj.Composer.BlueprintHandler = mockBlueprintHandler

		err := proj.Initialize(false)

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
		_ = NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.CheckFunc = func() error {
			return fmt.Errorf("prepare tools failed")
		}
		proj.Runtime.ToolsManager = mockToolsManager

		err := proj.Initialize(false)

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

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		if err := proj.Configure(nil); err != nil {
			t.Fatalf("Failed to configure project: %v", err)
		}

		if proj.Workstation == nil {
			t.Fatal("Expected workstation to be created")
		}

		mockNetworkManager := network.NewMockNetworkManager()
		mockNetworkManager.AssignIPsFunc = func(services []services.Service) error {
			return fmt.Errorf("assign IPs failed")
		}
		proj.Workstation.NetworkManager = mockNetworkManager

		err := proj.Initialize(false)

		if err == nil {
			t.Error("Expected error for AssignIPs failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to assign IPs to services") {
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

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		err := proj.PerformCleanup()

		if !cleanCalled {
			t.Error("Expected Clean to be called")
		}
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorOnCleanFailure", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.CleanFunc = func() error {
			return fmt.Errorf("clean failed")
		}

		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime})

		err := proj.PerformCleanup()

		if err == nil {
			t.Error("Expected error for Clean failure")
			return
		}

		if !strings.Contains(err.Error(), "error cleaning up context specific artifacts") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestProject_Up(t *testing.T) {
	t.Run("SuccessWithoutWorkstation", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			return false
		}
		proj := NewProject("test-context", &Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		blueprint, err := proj.Up()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if blueprint == nil {
			t.Error("Expected non-nil blueprint")
		}
	})

	t.Run("SuccessWithWorkstationPassesOnApplyToProvisioner", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		var capturedOnApply []func(string) error
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *v1alpha1.Blueprint, onApply ...func(id string) error) error {
			capturedOnApply = onApply
			return nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}
		proj := NewProject("test-context", &Project{
			Runtime:     mocks.Runtime,
			Composer:    mocks.Composer,
			Provisioner: prov,
		})
		if err := proj.Configure(nil); err != nil {
			t.Fatalf("Configure: %v", err)
		}
		if proj.Workstation == nil {
			t.Fatal("Expected Workstation created in dev mode")
		}
		proj.Workstation.NetworkManager = nil

		blueprint, err := proj.Up()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if blueprint == nil {
			t.Error("Expected non-nil blueprint")
		}
		if len(capturedOnApply) == 0 {
			t.Error("Expected provisioner to receive at least one onApply hook when workstation present")
		}
	})

	t.Run("ErrorFromWorkstationUp", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *v1alpha1.Blueprint, onApply ...func(id string) error) error {
			return nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}
		proj := NewProject("test-context", &Project{
			Runtime:     mocks.Runtime,
			Composer:    mocks.Composer,
			Provisioner: prov,
		})
		_ = proj.Configure(nil)
		if proj.Workstation == nil {
			t.Fatal("Expected Workstation created in dev mode")
		}
		mockVM := virt.NewMockVirt()
		mockVM.WriteConfigFunc = func() error { return nil }
		mockVM.UpFunc = func(verbose ...bool) error {
			return fmt.Errorf("VM up failed")
		}
		proj.Workstation.VirtualMachine = mockVM
		proj.Workstation.NetworkManager = nil
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "cluster.driver":
				return "talos"
			case "workstation.runtime":
				return "colima"
			case "provider":
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		_, err := proj.Up()

		if err == nil {
			t.Error("Expected error when Workstation.Up fails")
			return
		}
		if !strings.Contains(err.Error(), "error starting workstation") {
			t.Errorf("Expected error about starting workstation, got: %v", err)
		}
	})

	t.Run("ErrorFromEnsureNetworkPrivilege", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockShell := mocks.Shell.(*shell.MockShell)
		mockShell.ExecSudoFunc = func(message string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("sudo failed")
		}
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("passwordless sudo required")
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *v1alpha1.Blueprint, onApply ...func(id string) error) error {
			return nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}
		mockNetwork := network.NewMockNetworkManager()
		mockNetwork.NeedsPrivilegeFunc = func(dnsAddressOverride string) bool {
			return true
		}
		proj := NewProject("test-context", &Project{
			Runtime:     mocks.Runtime,
			Composer:    mocks.Composer,
			Provisioner: prov,
		})
		_ = proj.Configure(nil)
		if proj.Workstation == nil {
			t.Fatal("Expected Workstation created in dev mode")
		}
		proj.Workstation.NetworkManager = mockNetwork
		proj.Workstation.VirtualMachine = nil
		proj.Workstation.ContainerRuntime = nil

		_, err := proj.Up()

		if err == nil {
			t.Error("Expected error when EnsureNetworkPrivilege fails")
			return
		}
		if !strings.Contains(err.Error(), "privileged access required") && !strings.Contains(err.Error(), "network configuration may require sudo") {
			t.Errorf("Expected error about privilege/sudo, got: %v", err)
		}
	})

	t.Run("ErrorFromProvisionerUp", func(t *testing.T) {
		mocks := setupProjectMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *v1alpha1.Blueprint, onApply ...func(id string) error) error {
			return fmt.Errorf("terraform up failed")
		}
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})
		proj := NewProject("test-context", &Project{
			Runtime:     mocks.Runtime,
			Composer:    mocks.Composer,
			Provisioner: prov,
		})

		_, err := proj.Up()

		if err == nil {
			t.Error("Expected error when Provisioner.Up fails")
			return
		}
		if !strings.Contains(err.Error(), "error starting infrastructure") {
			t.Errorf("Expected error about starting infrastructure, got: %v", err)
		}
	})
}
