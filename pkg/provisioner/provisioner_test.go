package provisioner

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/provisioner/cluster"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	k8sclient "github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// createTestBlueprint creates a test blueprint with terraform components and kustomizations
func createTestBlueprint() *blueprintv1alpha1.Blueprint {
	return &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{
			Name: "test-blueprint",
		},
		Sources: []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "https://github.com/example/example.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		},
		TerraformComponents: []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "remote/path",
				Inputs: map[string]any{
					"remote_variable1": "default_value",
				},
			},
		},
		Kustomizations: []blueprintv1alpha1.Kustomization{
			{
				Name: "test-kustomization",
			},
		},
	}
}

// ProvisionerTestMocks contains all the mock dependencies for testing the Provisioner
type ProvisionerTestMocks struct {
	ConfigHandler     config.ConfigHandler
	Shell             *shell.MockShell
	TerraformStack    terraforminfra.Stack
	FluxStack         *fluxinfra.MockStack
	KubernetesManager *kubernetes.MockKubernetesManager
	KubernetesClient  k8sclient.KubernetesClient
	ClusterClient     *cluster.MockClusterClient
	Runtime           *runtime.Runtime
	BlueprintHandler  blueprint.BlueprintHandler
}

// setupProvisionerMocks creates mock components for testing the Provisioner with optional overrides
func setupProvisionerMocks(t *testing.T, opts ...func(*ProvisionerTestMocks)) *ProvisionerTestMocks {
	t.Helper()

	configHandler := config.NewMockConfigHandler()
	mockShell := shell.NewMockShell()

	configHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "cluster.driver":
			return "talos"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}

	configHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		switch key {
		case "terraform.enabled":
			return true
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
	}

	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/test/project", nil
	}

	kubernetesManager := kubernetes.NewMockKubernetesManager()
	kubernetesClient := k8sclient.NewMockKubernetesClient()
	clusterClient := cluster.NewMockClusterClient()
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()

	rt := &runtime.Runtime{
		ContextName:   "test-context",
		ProjectRoot:   "/test/project",
		ConfigRoot:    "/test/project/contexts/test-context",
		TemplateRoot:  "/test/project/contexts/_template",
		ConfigHandler: configHandler,
		Shell:         mockShell,
		Evaluator:     evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/project/contexts/_template"),
	}

	terraformStack := terraforminfra.NewMockStack()
	terraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error { return nil }
	terraformStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error { return nil }

	fluxStack := fluxinfra.NewMockStack()

	mocks := &ProvisionerTestMocks{
		ConfigHandler:     configHandler,
		Shell:             mockShell,
		TerraformStack:    terraformStack,
		FluxStack:         fluxStack,
		KubernetesManager: kubernetesManager,
		KubernetesClient:  kubernetesClient,
		ClusterClient:     clusterClient,
		Runtime:           rt,
		BlueprintHandler:  mockBlueprintHandler,
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

func TestNewProvisioner(t *testing.T) {
	t.Run("CreatesProvisionerWithDependencies", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		if provisioner == nil {
			t.Fatal("Expected Provisioner to be created")
		}

		if provisioner.shell != mocks.Shell {
			t.Error("Expected shell to be set")
		}

		if provisioner.configHandler != mocks.ConfigHandler {
			t.Error("Expected config handler to be set")
		}

		if provisioner.TerraformStack != nil {
			t.Error("Expected terraform stack to be nil (lazy loaded)")
		}

		if provisioner.KubernetesManager == nil {
			t.Error("Expected kubernetes manager to be initialized")
		}

		if provisioner.KubernetesClient == nil {
			t.Error("Expected kubernetes client to be initialized")
		}
	})

	t.Run("CreatesClusterClientForTalos", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		if provisioner.ClusterClient != nil {
			t.Error("Expected cluster client to be nil (lazy loaded)")
		}
	})

	t.Run("KeepsClusterClientNilForOmniDriver", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "omni"
			}
			return ""
		}

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		if provisioner.ClusterClient != nil {
			t.Error("Expected cluster client to be nil (omni driver is unsupported)")
		}
	})

	t.Run("SkipsClusterClientForOtherDrivers", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "k3s"
			}
			return ""
		}

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		if provisioner.ClusterClient != nil {
			t.Error("Expected cluster client to be nil for non-talos driver")
		}
	})

	t.Run("UsesExistingDependencies", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)

		opts := &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
			ClusterClient:     mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		if provisioner.TerraformStack != mocks.TerraformStack {
			t.Error("Expected existing terraform stack to be used")
		}

		if provisioner.KubernetesManager != mocks.KubernetesManager {
			t.Error("Expected existing kubernetes manager to be used")
		}

		if provisioner.KubernetesClient != mocks.KubernetesClient {
			t.Error("Expected existing kubernetes client to be used")
		}

		if provisioner.ClusterClient != mocks.ClusterClient {
			t.Error("Expected existing cluster client to be used")
		}
	})

	t.Run("SkipsTerraformStackWhenDisabled", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			return false
		}

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		if provisioner.TerraformStack != nil {
			t.Error("Expected terraform stack to be nil (lazy loaded, and disabled in config)")
		}
	})

}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProvisioner_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		opts := &Provisioner{
			TerraformStack: mocks.TerraformStack,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Up(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Up(nil)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("SuccessSkipsTerraformWhenDisabled", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		blueprint := createTestBlueprint()
		err := provisioner.Up(blueprint)

		if err != nil {
			t.Errorf("Expected no error when terraform is disabled, got: %v", err)
		}

		if provisioner.TerraformStack != nil {
			t.Error("Expected TerraformStack to remain nil when terraform is disabled")
		}
	})

	t.Run("SuccessInitializesTerraformStackLazily", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		if provisioner.TerraformStack != nil {
			t.Error("Expected TerraformStack to be nil before Up() is called")
		}

		blueprint := createTestBlueprint()
		err := provisioner.Up(blueprint)

		if provisioner.TerraformStack == nil {
			t.Error("Expected TerraformStack to be initialized after Up() when terraform is enabled, even if operation fails")
		}

		if err == nil {
			t.Log("Up() succeeded (unexpected but not a test failure)")
		}
	})

	t.Run("ErrorTerraformStackUp", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
			return fmt.Errorf("terraform stack up failed")
		}
		opts := &Provisioner{
			TerraformStack: mockStack,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Up(blueprint)

		if err == nil {
			t.Error("Expected error for terraform stack up failure")
			return
		}

		if !strings.Contains(err.Error(), "failed to run terraform up") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("PassesOnTerraformApplyHooksToStack", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		var capturedOnApply []func(id string) error
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
			capturedOnApply = onApply
			return nil
		}
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)
		provisioner.OnTerraformApply(func(id string) error { return nil })

		_ = provisioner.Up(createTestBlueprint())

		if len(capturedOnApply) != 1 {
			t.Errorf("Expected stack to receive 1 onApply hook, got %d", len(capturedOnApply))
		}
	})

}

func TestProvisioner_OnTerraformApply(t *testing.T) {
	t.Run("RegistersCallbackInvokedByStackOnUp", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		var hookCalled bool
		var hookID string
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
			if len(onApply) > 0 && onApply[0] != nil {
				_ = onApply[0]("test-component")
			}
			return nil
		}
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)
		provisioner.OnTerraformApply(func(id string) error {
			hookCalled = true
			hookID = id
			return nil
		})

		err := provisioner.Up(createTestBlueprint())

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !hookCalled {
			t.Error("Expected registered OnTerraformApply callback to be invoked")
		}
		if hookID != "test-component" {
			t.Errorf("Expected hook to be called with component id 'test-component', got %q", hookID)
		}
	})
}

func TestProvisioner_Down(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		opts := &Provisioner{
			TerraformStack: mocks.TerraformStack,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Down(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Down(nil)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("SuccessSkipsTerraformWhenDisabled", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		blueprint := createTestBlueprint()
		err := provisioner.Down(blueprint)

		if err != nil {
			t.Errorf("Expected no error when terraform is disabled, got: %v", err)
		}

		if provisioner.TerraformStack != nil {
			t.Error("Expected TerraformStack to remain nil when terraform is disabled")
		}
	})

	t.Run("SuccessInitializesTerraformStackLazily", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		if provisioner.TerraformStack != nil {
			t.Error("Expected TerraformStack to be nil before Down() is called")
		}

		blueprint := createTestBlueprint()
		err := provisioner.Down(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if provisioner.TerraformStack == nil {
			t.Error("Expected TerraformStack to be initialized after Down() when terraform is enabled")
		}
	})

	t.Run("ErrorTerraformStackDown", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("terraform stack down failed")
		}
		opts := &Provisioner{
			TerraformStack: mockStack,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Down(blueprint)

		if err == nil {
			t.Error("Expected error for terraform stack down failure")
		}

		if !strings.Contains(err.Error(), "failed to run terraform down") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

}

func TestProvisioner_Plan(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.PlanFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error { return nil }
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		err := provisioner.Plan(createTestBlueprint(), "remote/path")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Plan(nil, "remote/path")

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorWhenTerraformDisabled", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Plan(createTestBlueprint(), "remote/path")

		if err == nil {
			t.Error("Expected error when terraform is disabled")
		}
		if !strings.Contains(err.Error(), "terraform is disabled") {
			t.Errorf("Expected 'terraform is disabled' error, got: %v", err)
		}
		if provisioner.TerraformStack != nil {
			t.Error("Expected TerraformStack to remain nil when terraform is disabled")
		}
	})

	t.Run("InitializesTerraformStackLazily", func(t *testing.T) {
		// Given a provisioner with no pre-injected TerraformStack and terraform enabled
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		if provisioner.TerraformStack != nil {
			t.Error("Expected TerraformStack to be nil before Plan() is called")
		}

		// When Plan() is called (it will fail because the directory doesn't exist, but
		// the stack should still be lazily initialized before the plan runs)
		_ = provisioner.Plan(createTestBlueprint(), "remote/path")

		// Then TerraformStack should be initialized regardless of the plan outcome
		if provisioner.TerraformStack == nil {
			t.Error("Expected TerraformStack to be initialized after Plan() when terraform is enabled")
		}
	})

	t.Run("ErrorTerraformStackPlan", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.PlanFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error {
			return fmt.Errorf("terraform stack plan failed")
		}
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		err := provisioner.Plan(createTestBlueprint(), "remote/path")

		if err == nil {
			t.Error("Expected error for terraform stack plan failure")
		}
		if !strings.Contains(err.Error(), "failed to run terraform plan for") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestProvisioner_Apply(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.ApplyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error { return nil }
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		err := provisioner.Apply(createTestBlueprint(), "remote/path")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Apply(nil, "remote/path")

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorWhenTerraformDisabled", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Apply(createTestBlueprint(), "remote/path")

		if err == nil {
			t.Error("Expected error when terraform is disabled")
		}
		if !strings.Contains(err.Error(), "terraform is disabled") {
			t.Errorf("Expected 'terraform is disabled' error, got: %v", err)
		}
		if provisioner.TerraformStack != nil {
			t.Error("Expected TerraformStack to remain nil when terraform is disabled")
		}
	})

	t.Run("InitializesTerraformStackLazily", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		if provisioner.TerraformStack != nil {
			t.Error("Expected TerraformStack to be nil before Apply() is called")
		}

		_ = provisioner.Apply(createTestBlueprint(), "remote/path")

		if provisioner.TerraformStack == nil {
			t.Error("Expected TerraformStack to be initialized after Apply() when terraform is enabled")
		}
	})

	t.Run("ErrorTerraformStackApply", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.ApplyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error {
			return fmt.Errorf("terraform stack apply failed")
		}
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		err := provisioner.Apply(createTestBlueprint(), "remote/path")

		if err == nil {
			t.Error("Expected error for terraform stack apply failure")
		}
		if !strings.Contains(err.Error(), "failed to run terraform apply for") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestProvisioner_ApplyKustomize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a provisioner with a kubernetes manager that applies successfully
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When ApplyKustomize is called with a valid kustomization name
		err := provisioner.ApplyKustomize(createTestBlueprint(), "test-kustomization")

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner with no blueprint
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When ApplyKustomize is called with a nil blueprint
		err := provisioner.ApplyKustomize(nil, "test-kustomization")

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilKubernetesManager", func(t *testing.T) {
		// Given a provisioner with no kubernetes manager
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.KubernetesManager = nil

		// When ApplyKustomize is called
		err := provisioner.ApplyKustomize(createTestBlueprint(), "test-kustomization")

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}
		if !strings.Contains(err.Error(), "kubernetes manager not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorKustomizationNotFound", func(t *testing.T) {
		// Given a provisioner and a blueprint that does not contain the requested kustomization
		mocks := setupProvisionerMocks(t)
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When ApplyKustomize is called with a name not in the blueprint
		err := provisioner.ApplyKustomize(createTestBlueprint(), "nonexistent")

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for missing kustomization")
		}
		if !strings.Contains(err.Error(), "not found in blueprint") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorApplyBlueprintFails", func(t *testing.T) {
		// Given a provisioner whose kubernetes manager returns an error
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("apply blueprint failed")
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When ApplyKustomize is called
		err := provisioner.ApplyKustomize(createTestBlueprint(), "test-kustomization")

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for apply blueprint failure")
		}
		if !strings.Contains(err.Error(), "failed to apply kustomization") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("FiltersToSingleKustomization", func(t *testing.T) {
		// Given a provisioner and a blueprint with multiple kustomizations
		mocks := setupProvisionerMocks(t)
		var appliedBlueprint *blueprintv1alpha1.Blueprint
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			appliedBlueprint = bp
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When ApplyKustomize is called with a specific kustomization name
		err := provisioner.ApplyKustomize(createTestBlueprint(), "test-kustomization")

		// Then only the named kustomization is passed to ApplyBlueprint
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(appliedBlueprint.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization in filtered blueprint, got %d", len(appliedBlueprint.Kustomizations))
		}
		if appliedBlueprint.Kustomizations[0].Name != "test-kustomization" {
			t.Errorf("Expected kustomization name 'test-kustomization', got %q", appliedBlueprint.Kustomizations[0].Name)
		}
	})
}

func TestProvisioner_ApplyKustomizeAll(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a provisioner with a kubernetes manager that applies successfully
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When ApplyKustomizeAll is called
		err := provisioner.ApplyKustomizeAll(createTestBlueprint())

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner with no blueprint
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When ApplyKustomizeAll is called with a nil blueprint
		err := provisioner.ApplyKustomizeAll(nil)

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilKubernetesManager", func(t *testing.T) {
		// Given a provisioner with no kubernetes manager
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.KubernetesManager = nil

		// When ApplyKustomizeAll is called
		err := provisioner.ApplyKustomizeAll(createTestBlueprint())

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}
		if !strings.Contains(err.Error(), "kubernetes manager not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorApplyBlueprintFails", func(t *testing.T) {
		// Given a provisioner whose kubernetes manager returns an error
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("apply blueprint failed")
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When ApplyKustomizeAll is called
		err := provisioner.ApplyKustomizeAll(createTestBlueprint())

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for apply blueprint failure")
		}
		if !strings.Contains(err.Error(), "failed to apply blueprint") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestProvisioner_Install(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Install(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Install(nil)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilKubernetesManager", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.KubernetesManager = nil

		blueprint := createTestBlueprint()
		err := provisioner.Install(blueprint)

		if err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}

		if !strings.Contains(err.Error(), "kubernetes manager not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorApplyBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("apply blueprint failed")
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Install(blueprint)

		if err == nil {
			t.Error("Expected error for apply blueprint failure")
		}

		if !strings.Contains(err.Error(), "failed to apply blueprint") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestProvisioner_Wait(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, blueprint *blueprintv1alpha1.Blueprint) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Wait(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Wait(nil)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilKubernetesManager", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.KubernetesManager = nil

		blueprint := createTestBlueprint()
		err := provisioner.Wait(blueprint)

		if err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}

		if !strings.Contains(err.Error(), "kubernetes manager not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorWaitForKustomizations", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, blueprint *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("wait failed")
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Wait(blueprint)

		if err == nil {
			t.Error("Expected error for wait for kustomizations failure")
		}

		if !strings.Contains(err.Error(), "failed waiting for kustomizations") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestProvisioner_Uninstall(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Uninstall(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithCleanupKustomizations", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{
				Name:    "test-kustomization",
				Cleanup: []string{"cleanup/path"},
			},
		}

		err := provisioner.Uninstall(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessSkipsDestroyFalse", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		destroyFalse := false
		blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{
				Name:    "test-kustomization-1",
				Destroy: &blueprintv1alpha1.BoolExpression{Value: &destroyFalse, IsExpr: false},
			},
			{
				Name:    "test-kustomization-2",
				Destroy: nil,
			},
		}

		err := provisioner.Uninstall(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Uninstall(nil)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilKubernetesManager", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.KubernetesManager = nil

		blueprint := createTestBlueprint()
		err := provisioner.Uninstall(blueprint)

		if err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}

		if !strings.Contains(err.Error(), "kubernetes manager not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorDeleteBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("delete blueprint failed")
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Uninstall(blueprint)

		if err == nil {
			t.Error("Expected error for delete blueprint failure")
		}

		if !strings.Contains(err.Error(), "failed to delete blueprint") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestProvisioner_Close(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		closeCalled := false
		mocks.ClusterClient.CloseFunc = func() {
			closeCalled = true
		}
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		provisioner.Close()

		if !closeCalled {
			t.Error("Expected ClusterClient.Close to be called")
		}
	})

	t.Run("HandlesNilClusterClient", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.ClusterClient = nil

		provisioner.Close()

		if provisioner.ClusterClient != nil {
			t.Error("Expected nil cluster client to remain nil after Close")
		}
	})
}

func TestProvisioner_CheckNodeHealth(t *testing.T) {
	t.Run("SuccessWithNodeCheckOnly", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return nil
		}
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1", "10.0.0.2"},
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if len(outputMessages) != 1 {
			t.Errorf("Expected 1 output message, got: %d", len(outputMessages))
		}

		if !strings.Contains(outputMessages[0], "All 2 nodes are healthy") {
			t.Errorf("Expected output about healthy nodes, got: %q", outputMessages[0])
		}
	})

	t.Run("SuccessWithNodeCheckAndVersion", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			if expectedVersion != "v1.5.0" {
				t.Errorf("Expected version 'v1.5.0', got: %q", expectedVersion)
			}
			return nil
		}
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			Version:             "v1.5.0",
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if len(outputMessages) != 1 {
			t.Errorf("Expected 1 output message, got: %d", len(outputMessages))
		}

		if !strings.Contains(outputMessages[0], "v1.5.0") {
			t.Errorf("Expected output about version, got: %q", outputMessages[0])
		}
	})

	t.Run("SuccessWithKubernetesCheckOnly", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if len(outputMessages) != 1 {
			t.Errorf("Expected 1 output message, got: %d", len(outputMessages))
		}

		if !strings.Contains(outputMessages[0], "healthy") {
			t.Errorf("Expected output about healthy endpoint, got: %q", outputMessages[0])
		}
	})

	t.Run("SuccessWithBothChecks", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return nil
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
			ClusterClient:     mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithNodeReadinessCheck", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			if len(nodeNames) != 1 || nodeNames[0] != "10.0.0.1" {
				t.Errorf("Expected node name '10.0.0.1', got: %v", nodeNames)
			}
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": true}, nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNoHealthChecksSpecified", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		options := NodeHealthCheckOptions{
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when no health checks specified")
		}

		if !strings.Contains(err.Error(), "no health checks specified") {
			t.Errorf("Expected error about no health checks, got: %v", err)
		}
	})

	t.Run("ErrorClusterClientWaitForNodesHealthy", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return fmt.Errorf("cluster health check failed")
		}
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when cluster health check fails")
		}

		if !strings.Contains(err.Error(), "nodes failed health check") {
			t.Errorf("Expected error about nodes failed health check, got: %v", err)
		}
	})

	t.Run("WarningClusterClientFailureWithK8sCheck", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return fmt.Errorf("cluster health check failed")
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
			ClusterClient:     mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error (cluster failure should be warning), got: %v", err)
		}

		warningFound := false
		for _, msg := range outputMessages {
			if strings.Contains(msg, "Warning") && strings.Contains(msg, "Cluster client failed") {
				warningFound = true
				break
			}
		}

		if !warningFound {
			t.Error("Expected warning message about cluster client failure")
		}
	})

	t.Run("ErrorKubernetesManagerWaitForKubernetesHealthy", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return fmt.Errorf("kubernetes health check failed")
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when Kubernetes health check fails")
		}

		if !strings.Contains(err.Error(), "kubernetes health check failed") {
			t.Errorf("Expected error about kubernetes health check, got: %v", err)
		}
	})

	t.Run("ErrorCheckNodeReadyRequiresNodes", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when --ready flag used without --nodes")
		}

		if !strings.Contains(err.Error(), "--ready flag requires --nodes") {
			t.Errorf("Expected error about --ready requiring --nodes, got: %v", err)
		}
	})

	t.Run("ErrorNoKubernetesManager", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.KubernetesManager = nil

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when Kubernetes manager is nil")
		}

		if !strings.Contains(err.Error(), "no kubernetes manager found") {
			t.Errorf("Expected error about no kubernetes manager, got: %v", err)
		}
	})

	t.Run("SuccessWithDefaultTimeout", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Error("Expected context to have deadline")
			}
			if deadline.IsZero() {
				t.Error("Expected non-zero deadline")
			}
			return nil
		}
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			Timeout:             5 * time.Minute,
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithZeroTimeout", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return nil
		}
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			Timeout:             0,
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithK8SEndpointTrue", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			if endpoint != "" {
				t.Errorf("Expected empty endpoint when K8SEndpoint is 'true', got: %q", endpoint)
			}
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "true",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if len(outputMessages) != 1 {
			t.Errorf("Expected 1 output message, got: %d", len(outputMessages))
		}

		if !strings.Contains(outputMessages[0], "kubeconfig default") {
			t.Errorf("Expected output about kubeconfig default, got: %q", outputMessages[0])
		}
	})

	t.Run("SuccessWithNodeReadinessCheckAllReady", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": true, "10.0.0.2": true}, nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1", "10.0.0.2"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		foundReadyMessage := false
		for _, msg := range outputMessages {
			if strings.Contains(msg, "all nodes are Ready") {
				foundReadyMessage = true
				break
			}
		}

		if !foundReadyMessage {
			t.Error("Expected output message about all nodes being Ready")
		}
	})

	t.Run("SuccessWithNodeReadinessCheckNotAllReady", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": false}, nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		foundHealthyMessage := false
		for _, msg := range outputMessages {
			if strings.Contains(msg, "is healthy") && !strings.Contains(msg, "all nodes are Ready") {
				foundHealthyMessage = true
				break
			}
		}

		if !foundHealthyMessage {
			t.Error("Expected output message about endpoint being healthy without all nodes Ready")
		}
	})

	t.Run("SuccessWithNodeReadinessCheckGetNodeReadyStatusError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return nil, fmt.Errorf("get node ready status failed")
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		foundHealthyMessage := false
		for _, msg := range outputMessages {
			if strings.Contains(msg, "is healthy") && !strings.Contains(msg, "all nodes are Ready") {
				foundHealthyMessage = true
				break
			}
		}

		if !foundHealthyMessage {
			t.Error("Expected output message about endpoint being healthy when GetNodeReadyStatus fails")
		}
	})

	t.Run("SuccessWithNodeReadinessCheckPartialReadyStatus", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": true}, nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1", "10.0.0.2"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		foundHealthyMessage := false
		for _, msg := range outputMessages {
			if strings.Contains(msg, "is healthy") && !strings.Contains(msg, "all nodes are Ready") {
				foundHealthyMessage = true
				break
			}
		}

		if !foundHealthyMessage {
			t.Error("Expected output message about endpoint being healthy when not all nodes are ready")
		}
	})

	t.Run("ErrorNoHealthChecksWhenNodesProvidedButNoClusterClient", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "" // No cluster driver set, so ClusterClient won't be created
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.ClusterClient = nil

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when nodes provided but no cluster client")
		}

		if !strings.Contains(err.Error(), "no health checks specified") {
			t.Errorf("Expected error about no health checks, got: %v", err)
		}
	})

	t.Run("SuccessWithK8SEndpointEmptyString", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			if endpoint != "" {
				t.Errorf("Expected empty endpoint, got: %q", endpoint)
			}
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if len(outputMessages) != 1 {
			t.Errorf("Expected 1 output message, got: %d", len(outputMessages))
		}

		if !strings.Contains(outputMessages[0], "kubeconfig default") {
			t.Errorf("Expected output about kubeconfig default, got: %q", outputMessages[0])
		}
	})

	t.Run("SuccessWithK8SEndpointAndNodeReadinessCheckDefaultEndpoint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": true}, nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		foundReadyMessage := false
		for _, msg := range outputMessages {
			if strings.Contains(msg, "kubeconfig default") && strings.Contains(msg, "all nodes are Ready") {
				foundReadyMessage = true
				break
			}
		}

		if !foundReadyMessage {
			t.Error("Expected output message about kubeconfig default and all nodes Ready")
		}
	})
}

func TestProvisioner_PlanKustomization(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a provisioner with a mock flux stack
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When PlanKustomization is called with a valid blueprint
		err := provisioner.PlanKustomization(createTestBlueprint(), "test-kustomization")

		// Then no error is returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner with a mock flux stack
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When PlanKustomization is called with a nil blueprint
		err := provisioner.PlanKustomization(nil, "test-kustomization")

		// Then an error is returned
		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
	})

	t.Run("ErrorFluxStackFails", func(t *testing.T) {
		// Given a provisioner whose flux stack returns an error
		mocks := setupProvisionerMocks(t)
		mocks.FluxStack.PlanFunc = func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
			return fmt.Errorf("flux diff failed")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When PlanKustomization is called
		err := provisioner.PlanKustomization(createTestBlueprint(), "test-kustomization")

		// Then the error from the flux stack is returned
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "flux diff failed") {
			t.Errorf("expected flux error in message, got %v", err)
		}
	})
}

func TestProvisioner_PlanTerraformAll(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		err := p.PlanTerraformAll(nil)

		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
	})

	t.Run("DelegatestoStackPlanAll", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		called := false
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanAllFunc = func(bp *blueprintv1alpha1.Blueprint) error {
			called = true
			return nil
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		err := p.PlanTerraformAll(createTestBlueprint())

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !called {
			t.Error("expected TerraformStack.PlanAll to be called")
		}
	})
}

func TestProvisioner_PlanTerraformComponentSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		_, err := p.PlanTerraformComponentSummary(nil, "cluster")

		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
	})

	t.Run("PlansOnlyRequestedComponent", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		called := []string{}
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanComponentSummaryFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) terraforminfra.TerraformComponentPlan {
			called = append(called, componentID)
			return terraforminfra.TerraformComponentPlan{ComponentID: componentID, Add: 2}
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		result, err := p.PlanTerraformComponentSummary(createTestBlueprint(), "cluster")

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result.ComponentID != "cluster" || result.Add != 2 {
			t.Errorf("unexpected result: %+v", result)
		}
		if len(called) != 1 || called[0] != "cluster" {
			t.Errorf("expected exactly one call for cluster, got %v", called)
		}
	})
}

func TestProvisioner_PlanKustomizeComponentSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		_, err := p.PlanKustomizeComponentSummary(nil, "flux-system")

		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
	})

	t.Run("PlansOnlyRequestedKustomization", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		called := []string{}
		mocks.FluxStack.PlanComponentSummaryFunc = func(bp *blueprintv1alpha1.Blueprint, name string) fluxinfra.KustomizePlan {
			called = append(called, name)
			return fluxinfra.KustomizePlan{Name: name, Added: 5, IsNew: true}
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		result, err := p.PlanKustomizeComponentSummary(createTestBlueprint(), "flux-system")

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result.Name != "flux-system" || result.Added != 5 {
			t.Errorf("unexpected result: %+v", result)
		}
		if len(called) != 1 || called[0] != "flux-system" {
			t.Errorf("expected exactly one call for flux-system, got %v", called)
		}
	})
}

func TestProvisioner_PlanTerraformSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanTerraformSummary(nil)

		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
		if summary != nil {
			t.Errorf("expected nil summary, got %v", summary)
		}
	})

	t.Run("ReturnsTerraformResultsOnly", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			return []terraforminfra.TerraformComponentPlan{{ComponentID: "cluster", Add: 3}}
		}
		mocks.FluxStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, []string) {
			t.Fatal("FluxStack.PlanSummary should not be called by PlanTerraformSummary")
			return nil, nil
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanTerraformSummary(createTestBlueprint())

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(summary.Terraform) != 1 || summary.Terraform[0].ComponentID != "cluster" {
			t.Errorf("expected terraform result for cluster, got %v", summary.Terraform)
		}
		if summary.Kustomize != nil {
			t.Errorf("expected nil kustomize slice, got %v", summary.Kustomize)
		}
	})
}

func TestProvisioner_PlanKustomizeSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanKustomizeSummary(nil)

		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
		if summary != nil {
			t.Errorf("expected nil summary, got %v", summary)
		}
	})

	t.Run("ReturnsKustomizeResultsOnly", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			t.Fatal("TerraformStack.PlanSummary should not be called by PlanKustomizeSummary")
			return nil
		}
		mocks.FluxStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, []string) {
			return []fluxinfra.KustomizePlan{{Name: "flux-system", Added: 10, IsNew: true}}, []string{"hint1"}
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanKustomizeSummary(createTestBlueprint())

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(summary.Kustomize) != 1 || summary.Kustomize[0].Name != "flux-system" {
			t.Errorf("expected kustomize result for flux-system, got %v", summary.Kustomize)
		}
		if summary.Terraform != nil {
			t.Errorf("expected nil terraform slice, got %v", summary.Terraform)
		}
		if len(summary.Hints) != 1 || summary.Hints[0] != "hint1" {
			t.Errorf("expected hints, got %v", summary.Hints)
		}
	})
}

func TestProvisioner_PlanAll(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		// Given a properly configured provisioner
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When PlanAll is called with nil blueprint
		summary, err := p.PlanAll(nil)

		// Then an error is returned and summary is nil
		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
		if summary != nil {
			t.Errorf("expected nil summary, got %v", summary)
		}
	})

	t.Run("ReturnsTerraformAndKustomizeResults", func(t *testing.T) {
		// Given stacks that return non-empty summaries
		mocks := setupProvisionerMocks(t)
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			return []terraforminfra.TerraformComponentPlan{{ComponentID: "cluster", Add: 3}}
		}
		mocks.FluxStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, []string) {
			return []fluxinfra.KustomizePlan{{Name: "flux-system", Added: 10, IsNew: true}}, nil
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When PlanAll is called
		summary, err := p.PlanAll(createTestBlueprint())

		// Then both layers are present in the summary
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(summary.Terraform) != 1 || summary.Terraform[0].ComponentID != "cluster" {
			t.Errorf("expected terraform result for cluster, got %v", summary.Terraform)
		}
		if len(summary.Kustomize) != 1 || summary.Kustomize[0].Name != "flux-system" {
			t.Errorf("expected kustomize result for flux-system, got %v", summary.Kustomize)
		}
	})

	t.Run("OmitsTerraformWhenDisabled", func(t *testing.T) {
		// Given terraform.enabled=false in config
		mocks := setupProvisionerMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		mocks.FluxStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, []string) {
			return []fluxinfra.KustomizePlan{{Name: "flux-system"}}, nil
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When PlanAll is called
		summary, err := p.PlanAll(createTestBlueprint())

		// Then Terraform slice is nil and Kustomize is populated
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if summary.Terraform != nil {
			t.Errorf("expected nil terraform slice when disabled, got %v", summary.Terraform)
		}
		if len(summary.Kustomize) == 0 {
			t.Errorf("expected kustomize results, got none")
		}
	})
}
