package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

type BootstrapMocks struct {
	ConfigHandler     config.ConfigHandler
	Shell             *shell.MockShell
	Shims             *Shims
	BlueprintHandler  *blueprint.MockBlueprintHandler
	TerraformStack    *terraforminfra.MockStack
	KubernetesManager *kubernetes.MockKubernetesManager
	ToolsManager      *tools.MockToolsManager
	Runtime           *runtime.Runtime
	TmpDir            string
}

func setupBootstrapTest(t *testing.T, opts ...*SetupOptions) *BootstrapMocks {
	t.Helper()

	bootstrapPlatform = ""
	bootstrapBlueprint = ""
	bootstrapSetFlags = []string{}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetContextFunc = func() string { return "test-context" }
	mockConfigHandler.IsDevModeFunc = func(contextName string) bool { return false }
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		if key == "terraform.enabled" {
			return true
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}
	mockConfigHandler.IsLoadedFunc = func() bool { return true }
	mockConfigHandler.LoadConfigFunc = func() error { return nil }
	mockConfigHandler.SaveConfigFunc = func(hasSetFlags ...bool) error { return nil }
	mockConfigHandler.GenerateContextIDFunc = func() error { return nil }
	mockConfigHandler.ValidateContextValuesFunc = func() error { return nil }

	testOpts := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		testOpts = opts[0]
	}
	testOpts.ConfigHandler = mockConfigHandler
	baseMocks := setupMocks(t, testOpts)
	tmpDir := baseMocks.TmpDir
	mockConfigHandler.GetConfigRootFunc = func() (string, error) { return tmpDir + "/contexts/test-context", nil }

	baseMocks.Shell.AddCurrentDirToTrustedFileFunc = func() error { return nil }
	baseMocks.Shell.WriteResetTokenFunc = func() (string, error) { return "test-token", nil }

	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.LoadBlueprintFunc = func(...string) error { return nil }
	mockBlueprintHandler.WriteFunc = func(overwrite ...bool) error { return nil }
	testBlueprint := &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{Name: "test"},
	}
	mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }
	mockBlueprintHandler.GenerateResolvedFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }

	mockTerraformStack := terraforminfra.NewMockStack()
	mockTerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error { return nil }

	mockKubernetesManager := kubernetes.NewMockKubernetesManager()
	mockKubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error { return nil }
	mockKubernetesManager.WaitForKustomizationsFunc = func(message string, blueprint *blueprintv1alpha1.Blueprint) error { return nil }

	rt := runtime.NewRuntime(&runtime.Runtime{
		Shell:         baseMocks.Shell,
		ConfigHandler: baseMocks.ConfigHandler,
		ProjectRoot:   tmpDir,
		ToolsManager:  baseMocks.ToolsManager,
	})
	if rt == nil {
		t.Fatal("Failed to create runtime")
	}

	return &BootstrapMocks{
		ConfigHandler:     baseMocks.ConfigHandler,
		Shell:             baseMocks.Shell,
		Shims:             baseMocks.Shims,
		BlueprintHandler:  mockBlueprintHandler,
		TerraformStack:    mockTerraformStack,
		KubernetesManager: mockKubernetesManager,
		ToolsManager:      baseMocks.ToolsManager,
		Runtime:           rt,
		TmpDir:            tmpDir,
	}
}

// newBootstrapTestProject wires up a project override with all mocks. No workstation
// is attached by default — bootstrap is expected to proceed through terraform + install
// + wait regardless of whether the context has a workstation.
func newBootstrapTestProject(mocks *BootstrapMocks) *project.Project {
	comp := composer.NewComposer(mocks.Runtime)
	comp.BlueprintHandler = mocks.BlueprintHandler
	mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
		TerraformStack:    mocks.TerraformStack,
		KubernetesManager: mocks.KubernetesManager,
	})
	return project.NewProject("", &project.Project{
		Runtime:     mocks.Runtime,
		Composer:    comp,
		Provisioner: mockProvisioner,
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBootstrapCmd(t *testing.T) {
	createTestBootstrapCmd := func() *cobra.Command {
		cmd := &cobra.Command{
			Use:   "bootstrap [context]",
			Short: "Bootstrap a fresh Windsor environment end-to-end",
			RunE:  bootstrapCmd.RunE,
		}
		bootstrapCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd
	}

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("Success", func(t *testing.T) {
		// Given a project without a workstation and all provisioner steps mocked
		mocks := setupBootstrapTest(t)
		proj := newBootstrapTestProject(mocks)

		installCalled := false
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			installCalled = true
			return nil
		}
		waitCalled := false
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, blueprint *blueprintv1alpha1.Blueprint) error {
			waitCalled = true
			return nil
		}

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)

		// When executing bootstrap
		err := cmd.Execute()

		// Then success, and both Install and Wait ran (Wait is unconditional — no flag)
		if err != nil {
			t.Fatalf("Expected success, got %v", err)
		}
		if !installCalled {
			t.Error("Expected Install to be called")
		}
		if !waitCalled {
			t.Error("Expected Wait to be called unconditionally")
		}
	})

	t.Run("SuccessWithContextArg", func(t *testing.T) {
		// Given a config handler that records SetContext calls
		mocks := setupBootstrapTest(t)
		var setContextArg string
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.SetContextFunc = func(ctx string) error {
			setContextArg = ctx
			return nil
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"staging"})
		cmd.SetContext(ctx)

		// When executing bootstrap with a positional context arg
		err := cmd.Execute()

		// Then SetContext was invoked with the provided name
		if err != nil {
			t.Fatalf("Expected success, got %v", err)
		}
		if setContextArg != "staging" {
			t.Errorf("Expected SetContext(\"staging\"), got %q", setContextArg)
		}
	})

	t.Run("SuccessWithBlueprint", func(t *testing.T) {
		// Given a blueprint handler that records the LoadBlueprint URL
		mocks := setupBootstrapTest(t)
		var loadedURLs []string
		mocks.BlueprintHandler.LoadBlueprintFunc = func(urls ...string) error {
			loadedURLs = append(loadedURLs, urls...)
			return nil
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--blueprint", "oci://example.com/test:v1"})
		cmd.SetContext(ctx)

		// When executing bootstrap with --blueprint
		err := cmd.Execute()

		// Then the URL was passed through to LoadBlueprint
		if err != nil {
			t.Fatalf("Expected success, got %v", err)
		}
		found := false
		for _, u := range loadedURLs {
			if u == "oci://example.com/test:v1" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected blueprint URL to be loaded, got %v", loadedURLs)
		}
	})

	t.Run("SuccessWithSet", func(t *testing.T) {
		// Given a config handler that records SaveConfig calls
		mocks := setupBootstrapTest(t)
		saveConfigCalledWith := []bool{}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.SaveConfigFunc = func(hasSetFlags ...bool) error {
			if len(hasSetFlags) > 0 {
				saveConfigCalledWith = append(saveConfigCalledWith, hasSetFlags[0])
			} else {
				saveConfigCalledWith = append(saveConfigCalledWith, false)
			}
			return nil
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--set", "dns.enabled=false"})
		cmd.SetContext(ctx)

		// When executing bootstrap with --set
		err := cmd.Execute()

		// Then SaveConfig was called with hasSetFlags=true
		if err != nil {
			t.Fatalf("Expected success, got %v", err)
		}
		sawTrue := false
		for _, v := range saveConfigCalledWith {
			if v {
				sawTrue = true
				break
			}
		}
		if !sawTrue {
			t.Errorf("Expected SaveConfig to be called with true, saw %v", saveConfigCalledWith)
		}
	})

	t.Run("MalformedSetReturnsError", func(t *testing.T) {
		// Given a --set value missing the = separator
		mocks := setupBootstrapTest(t)
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--set", "badformat"})
		cmd.SetContext(ctx)

		// When executing bootstrap
		err := cmd.Execute()

		// Then an error is returned that names the offending value
		if err == nil {
			t.Fatal("Expected error for malformed --set value")
		}
		if !strings.Contains(err.Error(), "key=value") || !strings.Contains(err.Error(), "badformat") {
			t.Errorf("Expected error to mention key=value format and bad value, got %v", err)
		}
	})

	t.Run("AddTrustedDirectoryError", func(t *testing.T) {
		// Given a shell that fails to add the current directory to the trusted file
		mocks := setupBootstrapTest(t)
		mocks.Shell.AddCurrentDirToTrustedFileFunc = func() error {
			return fmt.Errorf("trust write failed")
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)

		// When executing bootstrap
		err := cmd.Execute()

		// Then the error is surfaced
		if err == nil {
			t.Fatal("Expected trusted-directory error")
		}
		if !strings.Contains(err.Error(), "trusted file") {
			t.Errorf("Expected trusted-file error, got %v", err)
		}
	})

	t.Run("WriteResetTokenError", func(t *testing.T) {
		// Given a shell that fails to write the reset token
		mocks := setupBootstrapTest(t)
		mocks.Shell.WriteResetTokenFunc = func() (string, error) {
			return "", fmt.Errorf("token write failed")
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)

		// When executing bootstrap
		err := cmd.Execute()

		// Then the error is surfaced
		if err == nil {
			t.Fatal("Expected reset token error")
		}
		if !strings.Contains(err.Error(), "reset token") {
			t.Errorf("Expected reset-token error, got %v", err)
		}
	})
}
