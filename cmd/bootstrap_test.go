package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
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
	bootstrapYes = false

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
	mockBlueprintHandler.GenerateResolvedFunc = func() (*blueprintv1alpha1.Blueprint, error) { return testBlueprint, nil }

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

	t.Run("NotifiesDuringInstallWithResolvedBlueprint", func(t *testing.T) {
		// Given a bootstrap test project wired with a MockNotifier
		mocks := setupBootstrapTest(t)
		proj := newBootstrapTestProject(mocks)

		notifier := fluxinfra.NewMockNotifier()
		var notifyCalled bool
		var notifyBlueprint *blueprintv1alpha1.Blueprint
		notifier.NotifyFunc = func(ctx context.Context, bp *blueprintv1alpha1.Blueprint) error {
			notifyCalled = true
			notifyBlueprint = bp
			return nil
		}
		proj.Provisioner.Notifier = notifier

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)

		// When executing bootstrap
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success, got %v", err)
		}

		// Then Notify fires during the install step (inside Provisioner.Install's
		// progress scope, before Wait) and receives the resolved blueprint so
		// flux's reconcile request targets the fully-substituted sources.
		if !notifyCalled {
			t.Error("Expected Notify to be called during Install")
		}
		if notifyBlueprint == nil {
			t.Error("Expected Notify to receive the resolved blueprint, got nil")
		}
	})

	t.Run("NotifyFailureIsTolerated", func(t *testing.T) {
		// Given a MockNotifier that returns an error (simulating an internal
		// bug: production Notifier is supposed to convert all errors to nil,
		// but bootstrap should still succeed even if that contract is ever
		// broken because the caller discards the error).
		mocks := setupBootstrapTest(t)
		proj := newBootstrapTestProject(mocks)

		notifier := fluxinfra.NewMockNotifier()
		notifier.NotifyFunc = func(ctx context.Context, bp *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("simulated notify failure")
		}
		proj.Provisioner.Notifier = notifier

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)

		// When executing bootstrap
		// Then bootstrap completes successfully regardless of the notifier error
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected bootstrap success even when Notify errors, got %v", err)
		}
	})

	t.Run("OverridesBackendToLocalThenMigratesAfterApply", func(t *testing.T) {
		mocks := setupBootstrapTest(t)

		backendBlueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return backendBlueprint }
		mocks.BlueprintHandler.GenerateResolvedFunc = func() (*blueprintv1alpha1.Blueprint, error) { return backendBlueprint, nil }

		var timeline []string
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				timeline = append(timeline, fmt.Sprintf("set=%v", value))
			}
			return nil
		}
		mocks.TerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
			timeline = append(timeline, "up")
			return nil
		}
		mocks.TerraformStack.MigrateStateFunc = func(blueprint *blueprintv1alpha1.Blueprint) ([]string, error) {
			timeline = append(timeline, "migrate")
			return nil, nil
		}

		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success, got %v", err)
		}

		expected := []string{"set=local", "up", "set=kubernetes", "migrate", "up"}
		if len(timeline) != len(expected) {
			t.Fatalf("Expected timeline %v, got %v", expected, timeline)
		}
		for i, step := range expected {
			if timeline[i] != step {
				t.Errorf("Expected timeline[%d]=%q, got %q (full: %v)", i, step, timeline[i], timeline)
			}
		}
	})

	t.Run("SkipsBackendDanceWhenBlueprintHasNoBackendComponent", func(t *testing.T) {
		mocks := setupBootstrapTest(t)

		var timeline []string
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				timeline = append(timeline, fmt.Sprintf("set=%v", value))
			}
			return nil
		}
		mocks.TerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
			timeline = append(timeline, "up")
			return nil
		}
		mocks.TerraformStack.MigrateStateFunc = func(blueprint *blueprintv1alpha1.Blueprint) ([]string, error) {
			timeline = append(timeline, "migrate")
			return nil, nil
		}

		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success, got %v", err)
		}
		expected := []string{"up"}
		if len(timeline) != len(expected) {
			t.Fatalf("Expected timeline %v, got %v", expected, timeline)
		}
		for i, step := range expected {
			if timeline[i] != step {
				t.Errorf("Expected timeline[%d]=%q, got %q (full: %v)", i, step, timeline[i], timeline)
			}
		}
	})

	t.Run("FailsWhenMigrateStateReportsSkippedComponents", func(t *testing.T) {
		mocks := setupBootstrapTest(t)

		backendBlueprint := &blueprintv1alpha1.Blueprint{
			Metadata:            blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Path: "backend"}},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return backendBlueprint }

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.TerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
			return nil
		}
		mocks.TerraformStack.MigrateStateFunc = func(blueprint *blueprintv1alpha1.Blueprint) ([]string, error) {
			return []string{"network", "cluster"}, nil
		}

		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)

		err := cmd.Execute()
		if err == nil {
			t.Fatal("Expected bootstrap to fail when MigrateState reports skipped components")
		}
		if !strings.Contains(err.Error(), "skipped") {
			t.Errorf("Expected error to mention skipped components, got: %v", err)
		}
		if !strings.Contains(err.Error(), "network") || !strings.Contains(err.Error(), "cluster") {
			t.Errorf("Expected error to name skipped components, got: %v", err)
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

	t.Run("GuardReadsConfigRootFromConfigHandlerNotRuntimeField", func(t *testing.T) {
		// Guards against regression where the prompt reads proj.Runtime.ConfigRoot directly
		// instead of the canonical ConfigHandler.GetConfigRoot(). If the two diverge (because
		// NewProject hasn't mutated Runtime.ConfigRoot, or is mutated later), the prompt must
		// still fire based on the ConfigHandler's answer — that's the canonical source.
		mocks := setupBootstrapTest(t)
		// Point GetConfigRoot at a distinct directory from whatever NewProject would compute
		// (which is <tmpDir>/contexts/test-context). If the code still reads Runtime.ConfigRoot,
		// it will look at the wrong directory, find no values.yaml, and skip the prompt — which
		// would cause this test to succeed trivially; we explicitly check for the prompt-cancelled
		// path to catch that regression.
		guardRoot := mocks.TmpDir + "/distinct-config-root"
		if err := os.MkdirAll(guardRoot, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(guardRoot+"/values.yaml", []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("write values.yaml: %v", err)
		}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetConfigRootFunc = func() (string, error) { return guardRoot, nil }

		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("n\n"))

		// When executing bootstrap with "n" on stdin
		err := cmd.Execute()

		// Then the prompt must have fired (guard saw values.yaml at guardRoot via GetConfigRoot)
		// and been cancelled. If the code regressed to reading Runtime.ConfigRoot, the prompt
		// wouldn't have fired at guardRoot and bootstrap would have proceeded — a non-cancel outcome.
		if err == nil {
			t.Fatal("Expected cancellation: the prompt must fire based on ConfigHandler.GetConfigRoot(), not Runtime.ConfigRoot")
		}
		if !strings.Contains(err.Error(), "cancelled") {
			t.Errorf("Expected cancellation error indicating the prompt fired at the canonical config root, got: %v", err)
		}
	})

	t.Run("PromptsAndProceedsOnYes", func(t *testing.T) {
		// Given a context whose values.yaml already exists (simulating prior configuration)
		mocks := setupBootstrapTest(t)
		configRoot := mocks.TmpDir + "/contexts/test-context"
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(configRoot+"/values.yaml", []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("write values.yaml: %v", err)
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("y\n"))

		// When executing bootstrap with "y" on stdin
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success when user confirms with y, got %v", err)
		}
	})

	t.Run("CancelsOnNo", func(t *testing.T) {
		// Given a context whose values.yaml already exists and the user declines
		mocks := setupBootstrapTest(t)
		configRoot := mocks.TmpDir + "/contexts/test-context"
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(configRoot+"/values.yaml", []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("write values.yaml: %v", err)
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("n\n"))

		// When executing bootstrap with "n" on stdin
		err := cmd.Execute()

		// Then an error surfaces and bootstrap did not continue
		if err == nil {
			t.Fatal("Expected cancellation error when user declines")
		}
		if !strings.Contains(err.Error(), "cancelled") {
			t.Errorf("Expected cancellation error, got %v", err)
		}
	})

	t.Run("CancelsOnEmptyStdin", func(t *testing.T) {
		// Given a context whose values.yaml exists and stdin is empty (non-interactive)
		mocks := setupBootstrapTest(t)
		configRoot := mocks.TmpDir + "/contexts/test-context"
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(configRoot+"/values.yaml", []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("write values.yaml: %v", err)
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader(""))

		// When executing bootstrap with no stdin input
		err := cmd.Execute()

		// Then it cancels rather than silently proceeding — non-interactive callers must pass --yes
		if err == nil {
			t.Fatal("Expected cancellation on empty stdin (non-interactive without --yes)")
		}
		if !strings.Contains(err.Error(), "cancelled") {
			t.Errorf("Expected cancellation error, got %v", err)
		}
	})

	t.Run("YesFlagSkipsPrompt", func(t *testing.T) {
		// Given a context whose values.yaml exists and --yes is passed
		mocks := setupBootstrapTest(t)
		configRoot := mocks.TmpDir + "/contexts/test-context"
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(configRoot+"/values.yaml", []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("write values.yaml: %v", err)
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--yes"})
		cmd.SetContext(ctx)
		// Intentionally no stdin — --yes must bypass the prompt entirely.

		// When executing bootstrap with --yes
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected --yes to skip the prompt, got %v", err)
		}
	})

	t.Run("GlobalModeSummaryConfirmProceedsOnYes", func(t *testing.T) {
		// Given a global-mode runtime, the bootstrap summary + confirmation
		// prompt fires before apply; answering "y" lets bootstrap continue
		// and Up is invoked. The summary is built from blueprint + config —
		// no terraform PlanSummary call is involved.
		mocks := setupBootstrapTest(t)
		mocks.Runtime.Global = true
		var upCalled bool
		mocks.TerraformStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			upCalled = true
			return nil
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("y\n"))

		// When executing bootstrap
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success on yes-confirm, got %v", err)
		}

		// Then Up runs after the prompt is accepted
		if !upCalled {
			t.Error("Expected Up to be called after summary-confirm")
		}
	})

	t.Run("GlobalModeSummaryConfirmExitsCleanlyOnNo", func(t *testing.T) {
		// Given a global-mode runtime, declining the summary-confirm prompt
		// exits cleanly (no error). The context has already been configured
		// and saved before the prompt, so "no" is a valid no-op outcome —
		// not a failure — and the operator can re-run with --yes later to apply.
		mocks := setupBootstrapTest(t)
		mocks.Runtime.Global = true
		var upCalled, installCalled bool
		mocks.TerraformStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			upCalled = true
			return nil
		}
		mocks.KubernetesManager.ApplyBlueprintFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			installCalled = true
			return nil
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("n\n"))

		// When executing bootstrap with "n" on stdin
		err := cmd.Execute()

		// Then bootstrap exits cleanly with no error and apply/install never run
		if err != nil {
			t.Fatalf("Expected clean exit on declined plan-confirm, got error: %v", err)
		}
		if upCalled {
			t.Error("Up must not be called after a declined summary-confirm")
		}
		if installCalled {
			t.Error("Install must not be called after a declined summary-confirm")
		}
	})

	t.Run("GlobalModeSummaryConfirmExitsCleanlyOnEmptyStdin", func(t *testing.T) {
		// Given a global-mode runtime and empty stdin (non-interactive without
		// --yes), the prompt receives EOF and treats it as "no" — exit is clean
		// rather than producing a non-zero failure.
		mocks := setupBootstrapTest(t)
		mocks.Runtime.Global = true
		var upCalled bool
		mocks.TerraformStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			upCalled = true
			return nil
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader(""))

		// When executing bootstrap with empty stdin
		err := cmd.Execute()

		// Then bootstrap exits cleanly and apply never runs
		if err != nil {
			t.Fatalf("Expected clean exit on EOF stdin, got error: %v", err)
		}
		if upCalled {
			t.Error("Up must not be called when EOF declines summary-confirm")
		}
	})

	t.Run("GlobalModeSummaryConfirmSkippedWithYesFlag", func(t *testing.T) {
		// Given a global-mode runtime and --yes, the summary-confirm prompt is
		// suppressed and bootstrap proceeds straight to apply with no stdin needed.
		mocks := setupBootstrapTest(t)
		mocks.Runtime.Global = true
		var upCalled bool
		mocks.TerraformStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			upCalled = true
			return nil
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--yes"})
		cmd.SetContext(ctx)
		// No stdin — --yes must skip both confirmation prompts.

		// When executing bootstrap with --yes
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success with --yes, got %v", err)
		}

		// Then Up still runs with --yes (apply isn't gated on stdin)
		if !upCalled {
			t.Error("Up must still run with --yes")
		}
	})

	t.Run("LocalModeSkipsSummaryConfirm", func(t *testing.T) {
		// Given a local-project runtime (Global=false), the global-only
		// summary-confirm prompt does not fire and bootstrap proceeds without
		// stdin.
		mocks := setupBootstrapTest(t)
		mocks.Runtime.Global = false
		var upCalled bool
		mocks.TerraformStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			upCalled = true
			return nil
		}
		proj := newBootstrapTestProject(mocks)

		cmd := createTestBootstrapCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		// No stdin — local-mode bootstrap with no values.yaml has no prompts at all.

		// When executing bootstrap
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success in local mode, got %v", err)
		}

		// Then Up runs without any prompt
		if !upCalled {
			t.Error("Up must run in local-project mode")
		}
	})
}
