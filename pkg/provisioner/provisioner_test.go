package provisioner

import (
	"fmt"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner/cluster"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	k8sclient "github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/context/shell"
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

type Mocks struct {
	Injector                     di.Injector
	ConfigHandler                config.ConfigHandler
	Shell                        *shell.MockShell
	TerraformStack               *terraforminfra.MockStack
	KubernetesManager            *kubernetes.MockKubernetesManager
	KubernetesClient             k8sclient.KubernetesClient
	ClusterClient                *cluster.MockClusterClient
	ProvisionerExecutionContext  *ProvisionerExecutionContext
}

// setupProvisionerMocks creates mock components for testing the Provisioner
func setupProvisionerMocks(t *testing.T) *Mocks {
	t.Helper()

	injector := di.NewInjector()
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

	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/test/project", nil
	}

	terraformStack := terraforminfra.NewMockStack(injector)
	kubernetesManager := kubernetes.NewMockKubernetesManager(injector)
	kubernetesClient := k8sclient.NewMockKubernetesClient()
	clusterClient := cluster.NewMockClusterClient()

	execCtx := &context.ExecutionContext{
		ContextName:   "test-context",
		ProjectRoot:   "/test/project",
		ConfigRoot:    "/test/project/contexts/test-context",
		TemplateRoot:  "/test/project/contexts/_template",
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         mockShell,
	}

	provisionerCtx := &ProvisionerExecutionContext{
		ExecutionContext:  *execCtx,
		TerraformStack:    terraformStack,
		KubernetesManager: kubernetesManager,
		KubernetesClient:  kubernetesClient,
		ClusterClient:     clusterClient,
	}

	injector.Register("shell", mockShell)
	injector.Register("configHandler", configHandler)
	injector.Register("terraformStack", terraformStack)
	injector.Register("kubernetesManager", kubernetesManager)
	injector.Register("kubernetesClient", kubernetesClient)
	injector.Register("clusterClient", clusterClient)

	return &Mocks{
		Injector:                    injector,
		ConfigHandler:               configHandler,
		Shell:                       mockShell,
		TerraformStack:              terraformStack,
		KubernetesManager:           kubernetesManager,
		KubernetesClient:            kubernetesClient,
		ClusterClient:               clusterClient,
		ProvisionerExecutionContext: provisionerCtx,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewProvisioner(t *testing.T) {
	t.Run("CreatesProvisionerWithDependencies", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)

		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		if provisioner == nil {
			t.Fatal("Expected Provisioner to be created")
		}

		if provisioner.Injector != mocks.Injector {
			t.Error("Expected injector to be set")
		}

		if provisioner.Shell != mocks.Shell {
			t.Error("Expected shell to be set")
		}

		if provisioner.ConfigHandler != mocks.ConfigHandler {
			t.Error("Expected config handler to be set")
		}

		if provisioner.TerraformStack == nil {
			t.Error("Expected terraform stack to be initialized")
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
		mocks.ProvisionerExecutionContext.ClusterClient = nil

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}

		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		if provisioner.ClusterClient == nil {
			t.Error("Expected cluster client to be created for talos driver")
		}
	})

	t.Run("CreatesClusterClientForOmni", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ProvisionerExecutionContext.ClusterClient = nil

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "omni"
			}
			return ""
		}

		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		if provisioner.ClusterClient == nil {
			t.Error("Expected cluster client to be created for omni driver")
		}
	})

	t.Run("SkipsClusterClientForOtherDrivers", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ProvisionerExecutionContext.ClusterClient = nil

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "k3s"
			}
			return ""
		}

		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		if provisioner.ClusterClient != nil {
			t.Error("Expected cluster client to be nil for non-talos/omni driver")
		}
	})

	t.Run("UsesExistingDependencies", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)

		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

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
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProvisioner_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		mocks.TerraformStack.InitializeFunc = func() error {
			return nil
		}
		mocks.TerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			return nil
		}

		blueprint := createTestBlueprint()
		err := provisioner.Up(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		err := provisioner.Up(nil)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilTerraformStack", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)
		provisioner.TerraformStack = nil

		blueprint := createTestBlueprint()
		err := provisioner.Up(blueprint)

		if err == nil {
			t.Error("Expected error for nil terraform stack")
		}

		if !strings.Contains(err.Error(), "terraform stack not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorTerraformStackInitialize", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		mocks.TerraformStack.InitializeFunc = func() error {
			return fmt.Errorf("initialize failed")
		}

		blueprint := createTestBlueprint()
		err := provisioner.Up(blueprint)

		if err == nil {
			t.Error("Expected error for terraform stack initialize failure")
		}

		if !strings.Contains(err.Error(), "failed to initialize terraform stack") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorTerraformStackUp", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		mocks.TerraformStack.InitializeFunc = func() error {
			return nil
		}
		mocks.TerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("up failed")
		}

		blueprint := createTestBlueprint()
		err := provisioner.Up(blueprint)

		if err == nil {
			t.Error("Expected error for terraform stack up failure")
		}

		if !strings.Contains(err.Error(), "failed to run terraform up") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestProvisioner_Down(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		mocks.TerraformStack.InitializeFunc = func() error {
			return nil
		}
		mocks.TerraformStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			return nil
		}

		blueprint := createTestBlueprint()
		err := provisioner.Down(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		err := provisioner.Down(nil)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilTerraformStack", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)
		provisioner.TerraformStack = nil

		blueprint := createTestBlueprint()
		err := provisioner.Down(blueprint)

		if err == nil {
			t.Error("Expected error for nil terraform stack")
		}

		if !strings.Contains(err.Error(), "terraform stack not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorTerraformStackInitialize", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		mocks.TerraformStack.InitializeFunc = func() error {
			return fmt.Errorf("initialize failed")
		}

		blueprint := createTestBlueprint()
		err := provisioner.Down(blueprint)

		if err == nil {
			t.Error("Expected error for terraform stack initialize failure")
		}

		if !strings.Contains(err.Error(), "failed to initialize terraform stack") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorTerraformStackDown", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		mocks.TerraformStack.InitializeFunc = func() error {
			return nil
		}
		mocks.TerraformStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("down failed")
		}

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

func TestProvisioner_Install(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}

		blueprint := createTestBlueprint()
		err := provisioner.Install(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

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
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)
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

	t.Run("ErrorKubernetesManagerInitialize", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return fmt.Errorf("initialize failed")
		}

		blueprint := createTestBlueprint()
		err := provisioner.Install(blueprint)

		if err == nil {
			t.Error("Expected error for kubernetes manager initialize failure")
		}

		if !strings.Contains(err.Error(), "failed to initialize kubernetes manager") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorApplyBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("apply blueprint failed")
		}

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
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, names ...string) error {
			return nil
		}

		blueprint := createTestBlueprint()
		err := provisioner.Wait(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

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
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)
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

	t.Run("ErrorKubernetesManagerInitialize", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return fmt.Errorf("initialize failed")
		}

		blueprint := createTestBlueprint()
		err := provisioner.Wait(blueprint)

		if err == nil {
			t.Error("Expected error for kubernetes manager initialize failure")
		}

		if !strings.Contains(err.Error(), "failed to initialize kubernetes manager") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorWaitForKustomizations", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, names ...string) error {
			return fmt.Errorf("wait failed")
		}

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

func TestProvisioner_Close(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)

		closeCalled := false
		mocks.ClusterClient.CloseFunc = func() {
			closeCalled = true
		}

		provisioner.Close()

		if !closeCalled {
			t.Error("Expected ClusterClient.Close to be called")
		}
	})

	t.Run("HandlesNilClusterClient", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerExecutionContext)
		provisioner.ClusterClient = nil

		provisioner.Close()

		if provisioner.ClusterClient != nil {
			t.Error("Expected nil cluster client to remain nil after Close")
		}
	})
}

// =============================================================================
// Test ProvisionerExecutionContext
// =============================================================================

func TestProvisionerExecutionContext(t *testing.T) {
	t.Run("CreatesProvisionerExecutionContext", func(t *testing.T) {
		execCtx := &context.ExecutionContext{
			ContextName:  "test-context",
			ProjectRoot:  "/test/project",
			ConfigRoot:   "/test/project/contexts/test-context",
			TemplateRoot: "/test/project/contexts/_template",
		}

		provisionerCtx := &ProvisionerExecutionContext{
			ExecutionContext: *execCtx,
		}

		if provisionerCtx.ContextName != "test-context" {
			t.Errorf("Expected context name 'test-context', got: %s", provisionerCtx.ContextName)
		}

		if provisionerCtx.ProjectRoot != "/test/project" {
			t.Errorf("Expected project root '/test/project', got: %s", provisionerCtx.ProjectRoot)
		}

		if provisionerCtx.ConfigRoot != "/test/project/contexts/test-context" {
			t.Errorf("Expected config root '/test/project/contexts/test-context', got: %s", provisionerCtx.ConfigRoot)
		}

		if provisionerCtx.TemplateRoot != "/test/project/contexts/_template" {
			t.Errorf("Expected template root '/test/project/contexts/_template', got: %s", provisionerCtx.TemplateRoot)
		}
	})
}
