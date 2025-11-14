package provisioner

import (
	stdcontext "context"
	"fmt"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner/cluster"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	k8sclient "github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
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

type Mocks struct {
	Injector           di.Injector
	ConfigHandler      config.ConfigHandler
	Shell              *shell.MockShell
	TerraformStack     *terraforminfra.MockStack
	KubernetesManager  *kubernetes.MockKubernetesManager
	KubernetesClient   k8sclient.KubernetesClient
	ClusterClient      *cluster.MockClusterClient
	ProvisionerRuntime *ProvisionerRuntime
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

	execCtx := &runtime.Runtime{
		ContextName:   "test-context",
		ProjectRoot:   "/test/project",
		ConfigRoot:    "/test/project/contexts/test-context",
		TemplateRoot:  "/test/project/contexts/_template",
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         mockShell,
	}

	provisionerCtx := &ProvisionerRuntime{
		Runtime:           *execCtx,
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
		Injector:           injector,
		ConfigHandler:      configHandler,
		Shell:              mockShell,
		TerraformStack:     terraformStack,
		KubernetesManager:  kubernetesManager,
		KubernetesClient:   kubernetesClient,
		ClusterClient:      clusterClient,
		ProvisionerRuntime: provisionerCtx,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewProvisioner(t *testing.T) {
	t.Run("CreatesProvisionerWithDependencies", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)

		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		mocks.ProvisionerRuntime.ClusterClient = nil

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}

		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		if provisioner.ClusterClient == nil {
			t.Error("Expected cluster client to be created for talos driver")
		}
	})

	t.Run("CreatesClusterClientForOmni", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ProvisionerRuntime.ClusterClient = nil

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "omni"
			}
			return ""
		}

		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		if provisioner.ClusterClient == nil {
			t.Error("Expected cluster client to be created for omni driver")
		}
	})

	t.Run("SkipsClusterClientForOtherDrivers", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ProvisionerRuntime.ClusterClient = nil

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "k3s"
			}
			return ""
		}

		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		if provisioner.ClusterClient != nil {
			t.Error("Expected cluster client to be nil for non-talos/omni driver")
		}
	})

	t.Run("UsesExistingDependencies", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)

		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)
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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)
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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)
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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)
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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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

func TestProvisioner_Uninstall(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}

		blueprint := createTestBlueprint()
		err := provisioner.Uninstall(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithCleanupKustomizations", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}

		blueprint := createTestBlueprint()
		destroyFalse := false
		blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{
				Name:    "test-kustomization-1",
				Destroy: &destroyFalse,
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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)
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

	t.Run("ErrorKubernetesManagerInitialize", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return fmt.Errorf("initialize failed")
		}

		blueprint := createTestBlueprint()
		err := provisioner.Uninstall(blueprint)

		if err == nil {
			t.Error("Expected error for kubernetes manager initialize failure")
		}

		if !strings.Contains(err.Error(), "failed to initialize kubernetes manager") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorDeleteBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("delete blueprint failed")
		}

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)
		provisioner.ClusterClient = nil

		provisioner.Close()

		if provisioner.ClusterClient != nil {
			t.Error("Expected nil cluster client to remain nil after Close")
		}
	})
}

// =============================================================================
// Test ProvisionerRuntime
// =============================================================================

func TestProvisionerRuntime(t *testing.T) {
	t.Run("CreatesProvisionerRuntime", func(t *testing.T) {
		execCtx := &runtime.Runtime{
			ContextName:  "test-context",
			ProjectRoot:  "/test/project",
			ConfigRoot:   "/test/project/contexts/test-context",
			TemplateRoot: "/test/project/contexts/_template",
		}

		provisionerCtx := &ProvisionerRuntime{
			Runtime: *execCtx,
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

// =============================================================================
// Test CheckNodeHealth
// =============================================================================

func TestProvisioner_CheckNodeHealth(t *testing.T) {
	t.Run("SuccessWithNodeCheckOnly", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx stdcontext.Context, nodeAddresses []string, expectedVersion string) error {
			return nil
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1", "10.0.0.2"},
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, outputFunc)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx stdcontext.Context, nodeAddresses []string, expectedVersion string) error {
			if expectedVersion != "v1.5.0" {
				t.Errorf("Expected version 'v1.5.0', got: %q", expectedVersion)
			}
			return nil
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			Version:             "v1.5.0",
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, outputFunc)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx stdcontext.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, outputFunc)

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
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx stdcontext.Context, nodeAddresses []string, expectedVersion string) error {
			return nil
		}
		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx stdcontext.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithNodeReadinessCheck", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx stdcontext.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			if len(nodeNames) != 1 || nodeNames[0] != "10.0.0.1" {
				t.Errorf("Expected node name '10.0.0.1', got: %v", nodeNames)
			}
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx stdcontext.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": true}, nil
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNoHealthChecksSpecified", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		options := NodeHealthCheckOptions{
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when no health checks specified")
		}

		if !strings.Contains(err.Error(), "no health checks specified") {
			t.Errorf("Expected error about no health checks, got: %v", err)
		}
	})

	t.Run("ErrorClusterClientWaitForNodesHealthy", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx stdcontext.Context, nodeAddresses []string, expectedVersion string) error {
			return fmt.Errorf("cluster health check failed")
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when cluster health check fails")
		}

		if !strings.Contains(err.Error(), "nodes failed health check") {
			t.Errorf("Expected error about nodes failed health check, got: %v", err)
		}
	})

	t.Run("WarningClusterClientFailureWithK8sCheck", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx stdcontext.Context, nodeAddresses []string, expectedVersion string) error {
			return fmt.Errorf("cluster health check failed")
		}
		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx stdcontext.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, outputFunc)

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

	t.Run("ErrorKubernetesManagerInitialize", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return fmt.Errorf("initialization failed")
		}

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when Kubernetes manager initialization fails")
		}

		if !strings.Contains(err.Error(), "failed to initialize kubernetes manager") {
			t.Errorf("Expected error about initialization failure, got: %v", err)
		}
	})

	t.Run("ErrorKubernetesManagerWaitForKubernetesHealthy", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx stdcontext.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return fmt.Errorf("kubernetes health check failed")
		}

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when Kubernetes health check fails")
		}

		if !strings.Contains(err.Error(), "kubernetes health check failed") {
			t.Errorf("Expected error about kubernetes health check, got: %v", err)
		}
	})

	t.Run("ErrorCheckNodeReadyRequiresNodes", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when --ready flag used without --nodes")
		}

		if !strings.Contains(err.Error(), "--ready flag requires --nodes") {
			t.Errorf("Expected error about --ready requiring --nodes, got: %v", err)
		}
	})

	t.Run("ErrorNoKubernetesManager", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)
		provisioner.KubernetesManager = nil

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when Kubernetes manager is nil")
		}

		if !strings.Contains(err.Error(), "no kubernetes manager found") {
			t.Errorf("Expected error about no kubernetes manager, got: %v", err)
		}
	})

	t.Run("SuccessWithDefaultTimeout", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.ProvisionerRuntime)

		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx stdcontext.Context, nodeAddresses []string, expectedVersion string) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Error("Expected context to have deadline")
			}
			if deadline.IsZero() {
				t.Error("Expected non-zero deadline")
			}
			return nil
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			Timeout:             5 * time.Minute,
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(stdcontext.Background(), options, nil)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})
}
