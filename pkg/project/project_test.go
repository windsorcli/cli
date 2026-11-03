package project_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
	v1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	terraformprovider "github.com/windsorcli/cli/pkg/runtime/terraform"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/workstation"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

// ProjectTestMocks holds the mock dependencies for Project tests.
type ProjectTestMocks struct {
	Runtime       *runtime.Runtime
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Workstation   *workstation.Workstation
	Composer      *composer.Composer
	Provisioner   *provisioner.Provisioner
}

// setupProjectMocks creates the Project test mocks and applies the given overrides.
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
		// Given a runtime with mock dependencies
		mocks := setupProjectMocks(t)

		// When NewProject is called
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// Then the project has its dependencies and the given context name
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
		// Given a runtime with mock dependencies
		mocks := setupProjectMocks(t)

		// When NewProject is called with a custom context name
		proj := project.NewProject("custom-context", &project.Project{Runtime: mocks.Runtime})

		// Then the runtime uses the custom context name
		if proj.Runtime.ContextName != "custom-context" {
			t.Errorf("Expected ContextName to be 'custom-context', got: %s", proj.Runtime.ContextName)
		}
	})

	t.Run("UsesConfigContextWhenContextNameEmpty", func(t *testing.T) {
		// Given a config with test-context as the current context
		mocks := setupProjectMocks(t)

		// When NewProject is called with an empty context name
		proj := project.NewProject("", &project.Project{Runtime: mocks.Runtime})

		// Then the runtime uses the config context
		if proj.Runtime.ContextName != "test-context" {
			t.Errorf("Expected ContextName to be 'test-context', got: %s", proj.Runtime.ContextName)
		}
	})

	t.Run("UsesLocalWhenContextNameAndConfigContextEmpty", func(t *testing.T) {
		// Given a config with no current context
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GetContextFunc = func() string {
			return ""
		}

		// When NewProject is called with an empty context name
		proj := project.NewProject("", &project.Project{Runtime: mocks.Runtime})

		// Then the runtime uses the local context
		if proj.Runtime.ContextName != "local" {
			t.Errorf("Expected ContextName to be 'local', got: %s", proj.Runtime.ContextName)
		}
	})

	t.Run("CreatesWorkstationWhenDevMode", func(t *testing.T) {
		// Given a config in dev mode
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		// When NewProject is called
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// Then the project has a workstation
		if proj.Workstation == nil {
			t.Error("Expected Workstation to be created in dev mode")
		}
	})

	t.Run("CreatesWorkstationAndProvisionerWhenDevMode", func(t *testing.T) {
		// Given a config in dev mode
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		// When NewProject is called
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// Then the project has a workstation and a provisioner
		if proj.Workstation == nil || proj.Provisioner == nil {
			t.Fatal("Expected Workstation and Provisioner to be created in dev mode")
		}
	})

	t.Run("CreatesWorkstationWhenWorkstationRuntimeSet", func(t *testing.T) {
		// Given a config outside dev mode with workstation.runtime set to colima
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return false
		}
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "workstation.runtime" {
				return "colima"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When NewProject is called and then EnsureWorkstation runs
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// Then the workstation exists only after EnsureWorkstation runs
		if proj.Workstation != nil {
			t.Error("Expected Workstation to be nil until EnsureWorkstation resolves config")
		}

		proj.EnsureWorkstation()

		if proj.Workstation == nil {
			t.Error("Expected Workstation to be created when workstation.runtime is set")
		}
		if proj.Provisioner == nil {
			t.Fatal("Expected Provisioner to exist")
		}
	})

	t.Run("SkipsWorkstationWhenNotDevMode", func(t *testing.T) {
		// Given a config outside dev mode with no workstation runtime
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return false
		}

		// When NewProject is called and then EnsureWorkstation runs
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// Then the project has no workstation
		if proj.Workstation != nil {
			t.Error("Expected Workstation to be nil when not in dev mode")
		}

		proj.EnsureWorkstation()

		if proj.Workstation != nil {
			t.Error("Expected Workstation to remain nil after EnsureWorkstation when workstation.runtime is unset")
		}
	})

	t.Run("PanicsWhenShellIsNil", func(t *testing.T) {
		// Given a runtime in dev mode with no shell
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		mocks.Runtime.Shell = nil

		// Then it panics
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected NewComposer to panic when Shell is nil")
			}
		}()
		// When NewProject is called
		_ = project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})
	})

	t.Run("HandlesComposerOverride", func(t *testing.T) {
		// Given a composer override
		mocks := setupProjectMocks(t)
		mockComposer := composer.NewComposer(mocks.Runtime)

		// When NewProject is called with the override
		proj := project.NewProject("test-context", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: mockComposer,
		})

		// Then the project uses the composer override
		if proj.Composer != mockComposer {
			t.Error("Expected Composer override to be used")
		}
	})

	t.Run("HandlesProvisionerOverride", func(t *testing.T) {
		// Given a provisioner override
		mocks := setupProjectMocks(t)
		mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler)

		// When NewProject is called with the override
		proj := project.NewProject("test-context", &project.Project{
			Runtime:     mocks.Runtime,
			Provisioner: mockProvisioner,
		})

		// Then the project uses the provisioner override
		if proj.Provisioner != mockProvisioner {
			t.Error("Expected Provisioner override to be used")
		}
	})

	t.Run("HandlesWorkstationOverride", func(t *testing.T) {
		// Given a workstation override in dev mode
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		mockWorkstation := workstation.NewWorkstation(mocks.Runtime)
		if mockWorkstation == nil {
			t.Fatal("Failed to create workstation")
		}

		// When NewProject is called with the override
		proj := project.NewProject("test-context", &project.Project{
			Runtime:     mocks.Runtime,
			Workstation: mockWorkstation,
		})

		// Then the project uses the workstation override
		if proj.Workstation != mockWorkstation {
			t.Error("Expected Workstation override to be used")
		}
	})

	t.Run("CreatesRuntimeWhenNoOverrides", func(t *testing.T) {
		// Given no overrides
		// When NewProject is called
		proj := project.NewProject("test-context")

		// Then the project creates its own runtime, composer, and provisioner
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
		// Given overrides with a nil runtime
		// When NewProject is called
		proj := project.NewProject("test-context", &project.Project{Runtime: nil})

		// Then the project creates its own runtime
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
		// Given a project with mock dependencies
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// When Configure is called with nil flag overrides
		err := proj.Configure(nil)

		// Then no error is returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithEmptyFlagOverrides", func(t *testing.T) {
		// Given a project with mock dependencies
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// When Configure is called with empty flag overrides
		err := proj.Configure(map[string]any{})

		// Then no error is returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithFlagOverrides", func(t *testing.T) {
		// Given a project with mock dependencies
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// When Configure is called with flag overrides
		err := proj.Configure(map[string]any{"platform": "docker"})

		// Then no error is returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SetsDockerPlatformInDevModeWhenPlatformNotSet", func(t *testing.T) {
		// Given dev mode with vm.runtime set to docker
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.runtime" {
				return "docker"
			}
			return ""
		}

		platformSet := false
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "platform" && value == "docker" {
				platformSet = true
			}
			return nil
		}

		// When Configure is called
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})
		_ = proj.Configure(nil)

		// Then the platform is set to docker
		if !platformSet {
			t.Error("Expected platform to be set to 'docker' in dev mode")
		}
	})

	t.Run("SetsIncusPlatformInDevModeWhenColimaIncus", func(t *testing.T) {
		// Given dev mode with colima and vm.runtime set to incus
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "workstation.runtime" {
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

		platformSet := false
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "platform" && value == "incus" {
				platformSet = true
			}
			return nil
		}

		// When Configure is called
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})
		_ = proj.Configure(nil)

		// Then the platform is set to incus
		if !platformSet {
			t.Error("Expected platform to be set to 'incus' in dev mode when workstation.runtime is colima and vm.runtime is incus")
		}
	})

	t.Run("CreatesWorkstationWhenWorkstationRuntimeSetViaFlagOverrides", func(t *testing.T) {
		// Given a config outside dev mode with no workstation runtime
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return false
		}
		var wsRuntime string
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "workstation.runtime" {
				if v, ok := value.(string); ok {
					wsRuntime = v
				}
			}
			return nil
		}
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "workstation.runtime" {
				return wsRuntime
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		if proj.Workstation != nil {
			t.Fatal("Expected Workstation nil before Configure when not dev mode and workstation.runtime not set")
		}

		// When Configure sets workstation.runtime to colima
		err := proj.Configure(map[string]any{"workstation.runtime": "colima"})
		// Then Configure succeeds and EnsureWorkstation creates the workstation
		if err != nil {
			t.Errorf("Configure failed: %v", err)
		}
		proj.EnsureWorkstation()
		if proj.Workstation == nil {
			t.Error("Expected Workstation to be created when workstation.runtime set via flag overrides")
		}
	})

	t.Run("SkipsDockerProviderWhenProviderAlreadySet", func(t *testing.T) {
		// Given dev mode with the provider set to aws
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

		// When NewProject is called
		_ = project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		dockerSet := false
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "provider" && value == "docker" {
				dockerSet = true
			}
			return nil
		}

		// Then the provider is not set to docker
		if dockerSet {
			t.Error("Expected provider not to be set to 'docker' when already set")
		}
	})

	t.Run("NoErrorWhenLegacyPlatformDefaultSetterWouldFail", func(t *testing.T) {
		// Given a config that fails to set aws.enabled
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "aws.enabled" {
				return fmt.Errorf("set aws.enabled failed")
			}
			return nil
		}

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// When Configure is called with platform aws
		err := proj.Configure(map[string]any{"platform": "aws"})

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorOnLoadConfigFailure", func(t *testing.T) {
		// Given a config that fails to load
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.LoadConfigFunc = func() error {
			return fmt.Errorf("load config failed")
		}

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// When Configure is called
		err := proj.Configure(nil)

		// Then an error about the config load is returned
		if err == nil {
			t.Error("Expected error for LoadConfig failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorOnSetFailure", func(t *testing.T) {
		// Given a config that fails on every set
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.SetFunc = func(key string, value any) error {
			return fmt.Errorf("set failed")
		}

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// When Configure is called with a flag override
		err := proj.Configure(map[string]any{"key": "value"})

		// Then an error about the set is returned
		if err == nil {
			t.Error("Expected error for Set failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to set") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("CallsApplyConfigDefaultsWithFlagOverrides", func(t *testing.T) {
		// Given a project with flag overrides containing workstation.runtime=colima
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

		var setDefaultConfig cliconfig.Context
		mockConfig.SetDefaultFunc = func(cfg cliconfig.Context) error {
			setDefaultConfig = cfg
			return nil
		}
		mockConfig.SetFunc = func(key string, value any) error {
			return nil
		}

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		flagOverrides := map[string]any{
			"workstation.runtime": "colima",
			"provider":            "docker",
		}

		// When Configure is called with flag overrides
		_ = proj.Configure(flagOverrides)

		// Then the default config has no platform
		_ = setDefaultConfig
		if setDefaultConfig.Platform != "" {
			t.Error("Expected DefaultConfig_Dev with no platform set")
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

		mockConfig.SetDefaultFunc = func(cfg cliconfig.Context) error {
			return fmt.Errorf("set default failed")
		}

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// When Configure is called
		err := proj.Configure(nil)

		// Then an error about config defaults is returned
		if err == nil {
			t.Error("Expected error for ApplyConfigDefaults failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to apply config defaults") {
			t.Errorf("Expected error about apply config defaults, got: %v", err)
		}
	})

	t.Run("SetsPlatformToIncusAfterLoadConfigWhenColimaIncus", func(t *testing.T) {
		// Given dev mode with platform docker, colima, and vm.runtime incus
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "platform" {
				return "docker"
			}
			if key == "workstation.runtime" {
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

		var platformSet string
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "platform" {
				platformSet = value.(string)
			}
			return nil
		}

		// When Configure is called
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})
		_ = proj.Configure(nil)

		// Then the platform changes to incus
		if platformSet != "incus" {
			t.Errorf("Expected platform to be set to 'incus' after LoadConfig when workstation.runtime is 'colima' and vm.runtime is 'incus', got: %s", platformSet)
		}
	})
}

func TestProject_ComposeBlueprint(t *testing.T) {
	t.Run("SuccessWhenLoadBlueprintSucceeds", func(t *testing.T) {
		// Given a blueprint handler that loads without error
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		// When ComposeBlueprint is called
		err := proj.ComposeBlueprint()

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("PassesBlueprintURLToLoadBlueprint", func(t *testing.T) {
		// Given a blueprint handler that records its load arguments
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
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: comp})

		// When ComposeBlueprint is called with an OCI URL
		err := proj.ComposeBlueprint("oci://ghcr.io/org/blueprint:v1")

		// Then LoadBlueprint receives the OCI URL
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(loadArgs) != 1 || loadArgs[0] != "oci://ghcr.io/org/blueprint:v1" {
			t.Errorf("Expected LoadBlueprint to be called with oci URL, got: %v", loadArgs)
		}
	})

	t.Run("ErrorOnGenerateContextIDFailure", func(t *testing.T) {
		// Given a config that fails to generate a context ID
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GenerateContextIDFunc = func() error {
			return fmt.Errorf("generate context ID failed")
		}
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		// When ComposeBlueprint is called
		err := proj.ComposeBlueprint()

		// Then an error about the context ID is returned
		if err == nil {
			t.Error("Expected error for GenerateContextID failure")
			return
		}
		if !strings.Contains(err.Error(), "failed to generate context ID") {
			t.Errorf("Expected error to mention context ID, got: %v", err)
		}
	})

	t.Run("ErrorOnLoadBlueprintFailure", func(t *testing.T) {
		// Given a blueprint handler that fails to load
		mocks := setupProjectMocks(t)
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
		mockBlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return fmt.Errorf("load blueprint failed")
		}
		comp := composer.NewComposer(mocks.Runtime, &composer.Composer{
			BlueprintHandler: mockBlueprintHandler,
		})
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: comp})

		// When ComposeBlueprint is called
		err := proj.ComposeBlueprint()

		// Then the load error is returned
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
		// Given a project outside dev mode
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		// When Initialize is called
		_ = proj.Initialize(false)
		// Then Initialize completes without a panic

	})

	t.Run("SuccessWithWorkstation", func(t *testing.T) {
		// Given a configured dev mode project with no network manager
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		if err := proj.Configure(nil); err != nil {
			t.Fatalf("Failed to configure project: %v", err)
		}

		if proj.Workstation == nil {
			t.Fatal("Expected workstation to be created")
		}

		proj.Workstation.NetworkManager = nil

		// When Initialize is called
		_ = proj.Initialize(false)
		// Then Initialize completes without a panic

	})

	t.Run("CallsContainerRuntimeWriteConfig", func(t *testing.T) {
		// Given a configured dev mode project with a mock container runtime
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

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

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

		// When Initialize is called
		_ = proj.Initialize(false)

		// Then the container runtime writes its config
		if !writeConfigCalled {
			t.Error("Expected ContainerRuntime.WriteConfig to be called")
		}
	})

	t.Run("ErrorOnContainerRuntimeWriteConfigFailure", func(t *testing.T) {
		// Given a dev mode project with a container runtime that fails to write config
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

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

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

		// When Initialize is called
		err := proj.Initialize(false)

		// Then an error about the container runtime config is returned
		if err == nil {
			t.Error("Expected error for ContainerRuntime.WriteConfig failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to write container runtime config") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("SuccessWithOverwriteTrue", func(t *testing.T) {
		// Given a project outside dev mode
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		// When Initialize is called with overwrite set
		_ = proj.Initialize(true)
		// Then Initialize completes without a panic

	})

	t.Run("ErrorOnGenerateContextIDFailure", func(t *testing.T) {
		// Given a config that fails to generate a context ID
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GenerateContextIDFunc = func() error {
			return fmt.Errorf("generate context ID failed")
		}

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		// When Initialize is called
		err := proj.Initialize(false)

		// Then an error about the context ID is returned
		if err == nil {
			t.Error("Expected error for GenerateContextID failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to generate context ID") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorOnSaveConfigFailure", func(t *testing.T) {
		// Given a config that fails to save
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.SaveConfigFunc = func(hasSetFlags ...bool) error {
			return fmt.Errorf("save config failed")
		}

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		// When Initialize is called
		err := proj.Initialize(false)

		// Then an error about the config save is returned
		if err == nil {
			t.Error("Expected error for SaveConfig failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to save config") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorOnLoadBlueprintFailure", func(t *testing.T) {
		// Given a blueprint handler that fails to load
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
		mockBlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return fmt.Errorf("load blueprint failed")
		}
		proj.Composer.BlueprintHandler = mockBlueprintHandler

		// When Initialize is called
		err := proj.Initialize(false)

		// Then an error about the blueprint load is returned
		if err == nil {
			t.Error("Expected error for LoadBlueprint failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to load blueprint data") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorOnGenerateFailure", func(t *testing.T) {
		// Given a blueprint handler that fails to write
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
		mockBlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			return fmt.Errorf("generate failed")
		}
		proj.Composer.BlueprintHandler = mockBlueprintHandler

		// When Initialize is called
		err := proj.Initialize(false)

		// Then an error about infrastructure generation is returned
		if err == nil {
			t.Error("Expected error for Generate failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to generate infrastructure") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorOnPrepareToolsFailure", func(t *testing.T) {
		// Given a tools manager that fails its check
		mocks := setupProjectMocks(t)
		_ = project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.CheckFunc = func() error {
			return fmt.Errorf("prepare tools failed")
		}
		proj.Runtime.ToolsManager = mockToolsManager

		// When Initialize is called
		err := proj.Initialize(false)

		// Then an error about the tools check is returned
		if err == nil {
			t.Error("Expected error for PrepareTools failure")
			return
		}

		if !strings.Contains(err.Error(), "error checking tools") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

// TestProject_SetToolRequirements checks that Initialize checks all tools when no requirements are set.
// A set value, also an empty Requirements, replaces that default.
func TestProject_SetToolRequirements(t *testing.T) {
	t.Run("UnsetDefaultsToAllRequirements", func(t *testing.T) {
		// Given a project with no SetToolRequirements call
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		var observed tools.Requirements
		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.CheckRequirementsFunc = func(reqs tools.Requirements) error {
			observed = reqs
			return nil
		}
		proj.Runtime.ToolsManager = mockToolsManager

		// When Initialize is called
		_ = proj.Initialize(false)

		// Then the tools manager receives AllRequirements
		want := tools.AllRequirements()
		if observed != want {
			t.Errorf("expected unset toolRequirements to default to AllRequirements (%+v), got %+v", want, observed)
		}
	})

	t.Run("SetEmptyMeansCheckNothing", func(t *testing.T) {
		// Given a project with an empty tool requirements set
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})
		proj.SetToolRequirements(tools.Requirements{})

		var observed tools.Requirements
		observed.Docker = true
		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.CheckRequirementsFunc = func(reqs tools.Requirements) error {
			observed = reqs
			return nil
		}
		proj.Runtime.ToolsManager = mockToolsManager

		// When Initialize is called
		_ = proj.Initialize(false)

		// Then the tools manager receives the empty set
		if observed != (tools.Requirements{}) {
			t.Errorf("expected SetToolRequirements({}) to forward an empty set; got %+v", observed)
		}
	})

	t.Run("SetNarrowForwardsExactRequest", func(t *testing.T) {
		// Given a project with a narrow tool requirements set
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})
		want := tools.Requirements{Docker: true, Colima: true, Secrets: true}
		proj.SetToolRequirements(want)

		var observed tools.Requirements
		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.CheckRequirementsFunc = func(reqs tools.Requirements) error {
			observed = reqs
			return nil
		}
		proj.Runtime.ToolsManager = mockToolsManager

		// When Initialize is called
		_ = proj.Initialize(false)

		// Then the tools manager receives the exact requested set
		if observed != want {
			t.Errorf("expected forwarded Requirements %+v, got %+v", want, observed)
		}
	})
}

func TestProject_PerformCleanup(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a config that records the clean call
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		cleanCalled := false
		mockConfig.CleanFunc = func() error {
			cleanCalled = true
			return nil
		}

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// When PerformCleanup is called
		err := proj.PerformCleanup()

		// Then Clean runs and no error is returned
		if !cleanCalled {
			t.Error("Expected Clean to be called")
		}
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorOnCleanFailure", func(t *testing.T) {
		// Given a config that fails to clean
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.CleanFunc = func() error {
			return fmt.Errorf("clean failed")
		}

		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// When PerformCleanup is called
		err := proj.PerformCleanup()

		// Then an error about the context cleanup is returned
		if err == nil {
			t.Error("Expected error for Clean failure")
			return
		}

		if !strings.Contains(err.Error(), "error cleaning up context specific artifacts") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorReadingContextDir", func(t *testing.T) {
		// Given a path where a regular file occupies the context directory slot
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		parent := filepath.Join(mocks.Runtime.ProjectRoot, ".windsor", "contexts")
		if err := os.MkdirAll(parent, 0755); err != nil {
			t.Fatalf("Failed to create parent dir: %v", err)
		}
		contextPath := filepath.Join(parent, "test-context")
		if err := os.WriteFile(contextPath, []byte("not a dir"), 0644); err != nil {
			t.Fatalf("Failed to write file at context path: %v", err)
		}

		// When PerformCleanup is called
		err := proj.PerformCleanup()

		// Then the error mentions the context directory
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error reading .windsor/contexts/test-context") {
			t.Errorf("Expected context-dir read error, got: %v", err)
		}

		// And the error tells the operator to remove the file
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "Remove the file .windsor/contexts/test-context") {
			t.Errorf("Expected remediation to remove the file, got: %v", we)
		}
	})

	t.Run("ErrorRemovingCredentials", func(t *testing.T) {
		// Given a RemoveAll shim that fails
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})
		proj.Shims().RemoveAll = func(path string) error {
			return fmt.Errorf("remove failed")
		}

		// When PerformCleanup is called
		err := proj.PerformCleanup()

		// Then the error names the credentials directory
		if err == nil || !strings.Contains(err.Error(), "error deleting .kube") || !strings.Contains(err.Error(), "remove failed") {
			t.Errorf("Expected wrapped .kube delete error, got: %v", err)
		}
	})

	t.Run("ErrorStattingContextDir", func(t *testing.T) {
		// Given a Stat shim that fails with an error other than not-exist
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})
		proj.Shims().Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrPermission
		}

		// When PerformCleanup is called
		err := proj.PerformCleanup()

		// Then the error mentions the context directory
		if err == nil || !strings.Contains(err.Error(), "error reading .windsor/contexts/test-context") {
			t.Errorf("Expected context-dir read error, got: %v", err)
		}
	})

	t.Run("ErrorListingContextDir", func(t *testing.T) {
		// Given a context directory that exists and a ReadDir shim that fails
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})
		contextDir := filepath.Join(mocks.Runtime.ProjectRoot, ".windsor", "contexts", "test-context")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context dir: %v", err)
		}
		proj.Shims().ReadDir = func(name string) ([]os.DirEntry, error) {
			return nil, fmt.Errorf("readdir failed")
		}

		// When PerformCleanup is called
		err := proj.PerformCleanup()

		// Then the error mentions the context directory
		if err == nil || !strings.Contains(err.Error(), "readdir failed") {
			t.Errorf("Expected wrapped readdir error, got: %v", err)
		}
	})

	t.Run("PreservesWorkstationYaml", func(t *testing.T) {
		// Given a context dir containing workstation.yaml alongside other artifacts
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		contextDir := filepath.Join(mocks.Runtime.ProjectRoot, ".windsor", "contexts", "test-context")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context dir: %v", err)
		}
		wsFile := filepath.Join(contextDir, "workstation.yaml")
		if err := os.WriteFile(wsFile, []byte("state: preserved"), 0644); err != nil {
			t.Fatalf("Failed to write workstation.yaml: %v", err)
		}
		otherFile := filepath.Join(contextDir, "ephemeral.txt")
		if err := os.WriteFile(otherFile, []byte("ephemeral"), 0644); err != nil {
			t.Fatalf("Failed to write ephemeral.txt: %v", err)
		}

		// When PerformCleanup is called
		err := proj.PerformCleanup()

		// Then workstation.yaml remains and the other files are deleted
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if _, statErr := os.Stat(wsFile); os.IsNotExist(statErr) {
			t.Error("Expected workstation.yaml to be preserved, but it was deleted")
		}
		if _, statErr := os.Stat(otherFile); !os.IsNotExist(statErr) {
			t.Error("Expected ephemeral.txt to be deleted, but it still exists")
		}
	})

	t.Run("RemovesLocalClusterCredentials", func(t *testing.T) {
		// Given a config root holding .kube and .talos credentials alongside operator state to keep
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		configRoot := filepath.Join(mocks.Runtime.ProjectRoot, "contexts", "test-context")
		kubeCfg := filepath.Join(configRoot, ".kube", "config")
		talosCfg := filepath.Join(configRoot, ".talos", "config")
		awsCfg := filepath.Join(configRoot, ".aws", "config")
		tfvars := filepath.Join(configRoot, "terraform", "cluster.tfvars")
		for _, f := range []string{kubeCfg, talosCfg, awsCfg, tfvars} {
			if err := os.MkdirAll(filepath.Dir(f), 0755); err != nil {
				t.Fatalf("Failed to create dir for %s: %v", f, err)
			}
			if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
				t.Fatalf("Failed to write %s: %v", f, err)
			}
		}

		// When PerformCleanup is called
		if err := proj.PerformCleanup(); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the stale local-cluster credentials are removed
		if _, err := os.Stat(filepath.Join(configRoot, ".kube")); !os.IsNotExist(err) {
			t.Error("Expected .kube to be removed")
		}
		if _, err := os.Stat(filepath.Join(configRoot, ".talos")); !os.IsNotExist(err) {
			t.Error("Expected .talos to be removed")
		}
		// And unrelated operator state is preserved
		if _, err := os.Stat(awsCfg); os.IsNotExist(err) {
			t.Error("Expected .aws credentials to be preserved")
		}
		if _, err := os.Stat(tfvars); os.IsNotExist(err) {
			t.Error("Expected hand-edited tfvars to be preserved")
		}
	})

	t.Run("SucceedsWhenCredentialsAbsent", func(t *testing.T) {
		// Given a config root with no .kube or .talos directories
		mocks := setupProjectMocks(t)
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime})

		// When PerformCleanup is called
		// Then no error is returned
		if err := proj.PerformCleanup(); err != nil {
			t.Errorf("Expected no error when credentials are absent, got: %v", err)
		}
	})
}

func TestProject_Up(t *testing.T) {
	t.Run("SuccessWithoutWorkstation", func(t *testing.T) {
		// Given a project with terraform disabled
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			return false
		}
		proj := project.NewProject("test-context", &project.Project{Runtime: mocks.Runtime, Composer: mocks.Composer})

		// When Up is called
		blueprint, _, err := proj.Up(context.Background())

		// Then Up returns a blueprint and no error
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if blueprint == nil {
			t.Error("Expected non-nil blueprint")
		}
	})

	t.Run("SuccessWithWorkstationPassesOnApplyToProvisioner", func(t *testing.T) {
		// Given a dev mode workstation and a terraform stack that records apply hooks
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockBH := mocks.Composer.BlueprintHandler.(*blueprint.MockBlueprintHandler)
		mockBH.GenerateFunc = func() *v1alpha1.Blueprint {
			return &v1alpha1.Blueprint{
				TerraformComponents: []v1alpha1.TerraformComponent{
					{Path: "workstation"},
				},
			}
		}
		var capturedOnApply []func(ctx context.Context, id string) (bool, error)
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *v1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			capturedOnApply = onApply
			return false, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}
		proj := project.NewProject("test-context", &project.Project{
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

		// When Up is called
		blueprint, _, err := proj.Up(context.Background())

		// Then the provisioner receives a non-nil apply hook
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if blueprint == nil {
			t.Error("Expected non-nil blueprint")
		}
		if len(capturedOnApply) == 0 {
			t.Error("Expected provisioner to receive at least one onApply hook when workstation present")
		}
		if len(capturedOnApply) > 0 && capturedOnApply[0] == nil {
			t.Error("Expected provisioner to receive non-nil onApply hook")
		}
	})

	t.Run("ErrorFromWorkstationUp", func(t *testing.T) {
		// Given a dev mode workstation with a VM that fails to start
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *v1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}
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
		proj := project.NewProject("test-context", &project.Project{
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

		// When Up is called
		_, _, err := proj.Up(context.Background())

		// Then an error about the workstation start is returned
		if err == nil {
			t.Error("Expected error when Workstation.Up fails")
			return
		}
		if !strings.Contains(err.Error(), "error starting workstation") {
			t.Errorf("Expected error about starting workstation, got: %v", err)
		}
	})

	t.Run("ClusterPrivilegeHaltsApplyHookWithoutError", func(t *testing.T) {
		// Given a workstation component and a network manager that needs privilege for the cluster
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}
		mockBH := mocks.Composer.BlueprintHandler.(*blueprint.MockBlueprintHandler)
		mockBH.GenerateFunc = func() *v1alpha1.Blueprint {
			return &v1alpha1.Blueprint{
				TerraformComponents: []v1alpha1.TerraformComponent{
					{Path: "workstation"},
				},
			}
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *v1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			for _, fn := range onApply {
				haltAfter, err := fn(context.Background(), "workstation")
				if err != nil {
					return false, err
				}
				if haltAfter {
					return true, nil
				}
			}
			return false, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		mockTF := &terraformprovider.MockTerraformProvider{}
		mockTF.GetTerraformOutputsFunc = func(_ string) (map[string]any, error) {
			return map[string]any{}, nil
		}
		mocks.Runtime.TerraformProvider = mockTF

		mockNetwork := network.NewMockNetworkManager()
		mockNetwork.NeedsPrivilegeForClusterFunc = func() bool { return true }

		proj := project.NewProject("test-context", &project.Project{
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

		// When Up is called
		_, halted, err := proj.Up(context.Background())

		// Then Up halts with no error and defers one required network task
		if err != nil {
			t.Fatalf("expected no error on halt, got: %v", err)
		}
		if !halted {
			t.Error("expected halted=true to propagate from the apply hook")
		}
		items := proj.Workstation.DeferredWork()
		if len(items) != 1 || !items[0].Required || items[0].Command != "windsor configure network" {
			t.Errorf("expected one required deferred-work item for 'windsor configure network', got: %v", items)
		}
	})

	t.Run("ErrorFromProvisionerUp", func(t *testing.T) {
		// Given a terraform stack that fails to apply
		mocks := setupProjectMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *v1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, fmt.Errorf("terraform up failed")
		}
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})
		proj := project.NewProject("test-context", &project.Project{
			Runtime:     mocks.Runtime,
			Composer:    mocks.Composer,
			Provisioner: prov,
		})

		// When Up is called
		_, _, err := proj.Up(context.Background())

		// Then an error about the infrastructure start is returned
		if err == nil {
			t.Error("Expected error when Provisioner.Up fails")
			return
		}
		if !strings.Contains(err.Error(), "error starting infrastructure") {
			t.Errorf("Expected error about starting infrastructure, got: %v", err)
		}
	})
}

func TestProject_Bootstrap(t *testing.T) {
	t.Run("WorkstationStartsBeforeProvisionerUp", func(t *testing.T) {
		// Given a dev mode workstation with a kubernetes backend
		mocks := setupProjectMocks(t)

		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(contextName string) bool { return true }
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "terraform.backend.type":
				return "kubernetes"
			case "cluster.driver":
				return "talos"
			case "workstation.runtime":
				return "colima"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}

		mockBH := mocks.Composer.BlueprintHandler.(*blueprint.MockBlueprintHandler)
		mockBH.GenerateFunc = func() *v1alpha1.Blueprint {
			return &v1alpha1.Blueprint{
				Backend: "backend",
				TerraformComponents: []v1alpha1.TerraformComponent{
					{Path: "backend"},
					{Path: "workstation"},
				},
			}
		}

		var timeline []string

		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *v1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			timeline = append(timeline, "up")
			return false, nil
		}
		mockStack.MigrateStateFunc = func(blueprint *v1alpha1.Blueprint) ([]string, error) {
			timeline = append(timeline, "migrate")
			return nil, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		proj := project.NewProject("test-context", &project.Project{
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

		mockVM := virt.NewMockVirt()
		mockVM.WriteConfigFunc = func() error { return nil }
		mockVM.UpFunc = func(verbose ...bool) error {
			timeline = append(timeline, "workstation_up")
			return nil
		}
		proj.Workstation.VirtualMachine = mockVM
		proj.Workstation.ContainerRuntime = nil
		proj.Workstation.NetworkManager = nil

		// When Bootstrap is called
		if _, _, _, err := proj.Bootstrap(context.Background(), nil); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the workstation starts before the provisioner applies
		var workstationIdx, upIdx = -1, -1
		for i, step := range timeline {
			if step == "workstation_up" && workstationIdx == -1 {
				workstationIdx = i
			}
			if step == "up" && upIdx == -1 {
				upIdx = i
			}
		}
		if workstationIdx == -1 {
			t.Fatalf("Expected Workstation.Up to run, timeline: %v", timeline)
		}
		if upIdx == -1 {
			t.Fatalf("Expected provisioner Up to run, timeline: %v", timeline)
		}
		if workstationIdx >= upIdx {
			t.Errorf("Expected workstation_up before provisioner up, got workstation_up at %d, up at %d (full: %v)", workstationIdx, upIdx, timeline)
		}
	})

	t.Run("ConfirmFiresBeforeWorkstationStart", func(t *testing.T) {
		// Given a dev mode project and a confirm callback that declines
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.IsDevModeFunc = func(_ string) bool { return true }
		mockConfig.GetBoolFunc = func(key string, _ ...bool) bool {
			return key == "terraform.enabled"
		}

		var timeline []string
		mockBH := mocks.Composer.BlueprintHandler.(*blueprint.MockBlueprintHandler)
		mockBH.GenerateFunc = func() *v1alpha1.Blueprint {
			timeline = append(timeline, "generate")
			return &v1alpha1.Blueprint{}
		}

		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *v1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			timeline = append(timeline, "provisioner_up")
			return false, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		proj := project.NewProject("test-context", &project.Project{
			Runtime:     mocks.Runtime,
			Composer:    mocks.Composer,
			Provisioner: prov,
		})
		if err := proj.Configure(nil); err != nil {
			t.Fatalf("Configure: %v", err)
		}
		mockVM := virt.NewMockVirt()
		mockVM.UpFunc = func(_ ...bool) error {
			timeline = append(timeline, "workstation_up")
			return nil
		}
		if proj.Workstation != nil {
			proj.Workstation.VirtualMachine = mockVM
			proj.Workstation.ContainerRuntime = nil
			proj.Workstation.NetworkManager = nil
		}

		var confirmAt int
		confirm := func(_ *provisioner.BootstrapSummary) bool {
			confirmAt = len(timeline)
			timeline = append(timeline, "confirm")
			return false
		}

		// When Bootstrap is called
		_, applied, _, err := proj.Bootstrap(context.Background(), confirm)
		// Then no workstation or provisioner work runs
		if err != nil {
			t.Fatalf("Expected no error on decline, got: %v", err)
		}
		if applied {
			t.Error("Expected applied=false when confirm declines")
		}

		for _, step := range timeline {
			if step == "workstation_up" {
				t.Errorf("Workstation.Up must not run on decline, timeline: %v", timeline)
			}
			if step == "provisioner_up" {
				t.Errorf("Provisioner Up must not run on decline, timeline: %v", timeline)
			}
		}
		if confirmAt == 0 {
			t.Fatalf("Expected confirm to fire, timeline: %v", timeline)
		}
	})

	t.Run("ConfirmDeclineReturnsBlueprintAndAppliedFalse", func(t *testing.T) {
		// Given a blueprint handler that returns a known blueprint
		mocks := setupProjectMocks(t)
		expectedBP := &v1alpha1.Blueprint{
			TerraformComponents: []v1alpha1.TerraformComponent{{Path: "marker"}},
		}
		mockBH := mocks.Composer.BlueprintHandler.(*blueprint.MockBlueprintHandler)
		mockBH.GenerateFunc = func() *v1alpha1.Blueprint { return expectedBP }
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler)

		proj := project.NewProject("test-context", &project.Project{
			Runtime:     mocks.Runtime,
			Composer:    mocks.Composer,
			Provisioner: prov,
		})

		// When Bootstrap is called and confirm declines
		bp, applied, _, err := proj.Bootstrap(context.Background(), func(_ *provisioner.BootstrapSummary) bool { return false })
		// Then Bootstrap returns the blueprint with applied set to false
		if err != nil {
			t.Fatalf("Expected no error on decline, got: %v", err)
		}
		if applied {
			t.Error("Expected applied=false on decline")
		}
		if bp != expectedBP {
			t.Errorf("Expected blueprint to round-trip, got %#v", bp)
		}
	})

	t.Run("WrapsProvisionerErrorAsStartingInfrastructure", func(t *testing.T) {
		// Given a terraform stack that fails to apply
		mocks := setupProjectMocks(t)
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *v1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, fmt.Errorf("terraform up failed")
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.Composer.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		proj := project.NewProject("test-context", &project.Project{
			Runtime:     mocks.Runtime,
			Composer:    mocks.Composer,
			Provisioner: prov,
		})

		// When Bootstrap is called
		_, _, _, err := proj.Bootstrap(context.Background(), nil)
		// Then the error is wrapped as an infrastructure start error
		if err == nil {
			t.Fatal("Expected error when provisioner errors")
		}
		if !strings.Contains(err.Error(), "error starting infrastructure") {
			t.Errorf("Expected error to be wrapped, got: %v", err)
		}
	})
}
