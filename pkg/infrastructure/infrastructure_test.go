package infrastructure

import (
	"fmt"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/infrastructure/cluster"
	"github.com/windsorcli/cli/pkg/infrastructure/kubernetes"
	terraforminfra "github.com/windsorcli/cli/pkg/infrastructure/terraform"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/types"
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
	Injector                       di.Injector
	ConfigHandler                  config.ConfigHandler
	Shell                          *shell.MockShell
	TerraformStack                 *terraforminfra.MockStack
	KubernetesManager              *kubernetes.MockKubernetesManager
	KubernetesClient               kubernetes.KubernetesClient
	ClusterClient                  *cluster.MockClusterClient
	InfrastructureExecutionContext *InfrastructureExecutionContext
}

// setupInfrastructureMocks creates mock components for testing the Infrastructure
func setupInfrastructureMocks(t *testing.T) *Mocks {
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
	kubernetesClient := kubernetes.NewMockKubernetesClient()
	clusterClient := cluster.NewMockClusterClient()

	execCtx := &types.ExecutionContext{
		ContextName:   "test-context",
		ProjectRoot:   "/test/project",
		ConfigRoot:    "/test/project/contexts/test-context",
		TemplateRoot:  "/test/project/contexts/_template",
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         mockShell,
	}

	infraCtx := &InfrastructureExecutionContext{
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
		Injector:                       injector,
		ConfigHandler:                  configHandler,
		Shell:                          mockShell,
		TerraformStack:                 terraformStack,
		KubernetesManager:              kubernetesManager,
		KubernetesClient:               kubernetesClient,
		ClusterClient:                  clusterClient,
		InfrastructureExecutionContext: infraCtx,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewInfrastructure(t *testing.T) {
	t.Run("CreatesInfrastructureWithDependencies", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)

		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		if infra == nil {
			t.Fatal("Expected Infrastructure to be created")
		}

		if infra.Injector != mocks.Injector {
			t.Error("Expected injector to be set")
		}

		if infra.Shell != mocks.Shell {
			t.Error("Expected shell to be set")
		}

		if infra.ConfigHandler != mocks.ConfigHandler {
			t.Error("Expected config handler to be set")
		}

		if infra.TerraformStack == nil {
			t.Error("Expected terraform stack to be initialized")
		}

		if infra.KubernetesManager == nil {
			t.Error("Expected kubernetes manager to be initialized")
		}

		if infra.KubernetesClient == nil {
			t.Error("Expected kubernetes client to be initialized")
		}
	})

	t.Run("CreatesClusterClientForTalos", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		mocks.InfrastructureExecutionContext.ClusterClient = nil

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}

		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		if infra.ClusterClient == nil {
			t.Error("Expected cluster client to be created for talos driver")
		}
	})

	t.Run("CreatesClusterClientForOmni", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		mocks.InfrastructureExecutionContext.ClusterClient = nil

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "omni"
			}
			return ""
		}

		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		if infra.ClusterClient == nil {
			t.Error("Expected cluster client to be created for omni driver")
		}
	})

	t.Run("SkipsClusterClientForOtherDrivers", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		mocks.InfrastructureExecutionContext.ClusterClient = nil

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "k3s"
			}
			return ""
		}

		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		if infra.ClusterClient != nil {
			t.Error("Expected cluster client to be nil for non-talos/omni driver")
		}
	})

	t.Run("UsesExistingDependencies", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)

		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		if infra.TerraformStack != mocks.TerraformStack {
			t.Error("Expected existing terraform stack to be used")
		}

		if infra.KubernetesManager != mocks.KubernetesManager {
			t.Error("Expected existing kubernetes manager to be used")
		}

		if infra.KubernetesClient != mocks.KubernetesClient {
			t.Error("Expected existing kubernetes client to be used")
		}

		if infra.ClusterClient != mocks.ClusterClient {
			t.Error("Expected existing cluster client to be used")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestInfrastructure_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		mocks.TerraformStack.InitializeFunc = func() error {
			return nil
		}
		mocks.TerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			return nil
		}

		blueprint := createTestBlueprint()
		err := infra.Up(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		err := infra.Up(nil)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilTerraformStack", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)
		infra.TerraformStack = nil

		blueprint := createTestBlueprint()
		err := infra.Up(blueprint)

		if err == nil {
			t.Error("Expected error for nil terraform stack")
		}

		if !strings.Contains(err.Error(), "terraform stack not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorTerraformStackInitialize", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		mocks.TerraformStack.InitializeFunc = func() error {
			return fmt.Errorf("initialize failed")
		}

		blueprint := createTestBlueprint()
		err := infra.Up(blueprint)

		if err == nil {
			t.Error("Expected error for terraform stack initialize failure")
		}

		if !strings.Contains(err.Error(), "failed to initialize terraform stack") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorTerraformStackUp", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		mocks.TerraformStack.InitializeFunc = func() error {
			return nil
		}
		mocks.TerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("up failed")
		}

		blueprint := createTestBlueprint()
		err := infra.Up(blueprint)

		if err == nil {
			t.Error("Expected error for terraform stack up failure")
		}

		if !strings.Contains(err.Error(), "failed to run terraform up") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestInfrastructure_Down(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		mocks.TerraformStack.InitializeFunc = func() error {
			return nil
		}
		mocks.TerraformStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			return nil
		}

		blueprint := createTestBlueprint()
		err := infra.Down(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		err := infra.Down(nil)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilTerraformStack", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)
		infra.TerraformStack = nil

		blueprint := createTestBlueprint()
		err := infra.Down(blueprint)

		if err == nil {
			t.Error("Expected error for nil terraform stack")
		}

		if !strings.Contains(err.Error(), "terraform stack not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorTerraformStackInitialize", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		mocks.TerraformStack.InitializeFunc = func() error {
			return fmt.Errorf("initialize failed")
		}

		blueprint := createTestBlueprint()
		err := infra.Down(blueprint)

		if err == nil {
			t.Error("Expected error for terraform stack initialize failure")
		}

		if !strings.Contains(err.Error(), "failed to initialize terraform stack") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorTerraformStackDown", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		mocks.TerraformStack.InitializeFunc = func() error {
			return nil
		}
		mocks.TerraformStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("down failed")
		}

		blueprint := createTestBlueprint()
		err := infra.Down(blueprint)

		if err == nil {
			t.Error("Expected error for terraform stack down failure")
		}

		if !strings.Contains(err.Error(), "failed to run terraform down") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestInfrastructure_Install(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}

		blueprint := createTestBlueprint()
		err := infra.Install(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		err := infra.Install(nil)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilKubernetesManager", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)
		infra.KubernetesManager = nil

		blueprint := createTestBlueprint()
		err := infra.Install(blueprint)

		if err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}

		if !strings.Contains(err.Error(), "kubernetes manager not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorKubernetesManagerInitialize", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return fmt.Errorf("initialize failed")
		}

		blueprint := createTestBlueprint()
		err := infra.Install(blueprint)

		if err == nil {
			t.Error("Expected error for kubernetes manager initialize failure")
		}

		if !strings.Contains(err.Error(), "failed to initialize kubernetes manager") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorApplyBlueprint", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("apply blueprint failed")
		}

		blueprint := createTestBlueprint()
		err := infra.Install(blueprint)

		if err == nil {
			t.Error("Expected error for apply blueprint failure")
		}

		if !strings.Contains(err.Error(), "failed to apply blueprint") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestInfrastructure_Wait(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, names ...string) error {
			return nil
		}

		blueprint := createTestBlueprint()
		err := infra.Wait(blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		err := infra.Wait(nil)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilKubernetesManager", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)
		infra.KubernetesManager = nil

		blueprint := createTestBlueprint()
		err := infra.Wait(blueprint)

		if err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}

		if !strings.Contains(err.Error(), "kubernetes manager not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorKubernetesManagerInitialize", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return fmt.Errorf("initialize failed")
		}

		blueprint := createTestBlueprint()
		err := infra.Wait(blueprint)

		if err == nil {
			t.Error("Expected error for kubernetes manager initialize failure")
		}

		if !strings.Contains(err.Error(), "failed to initialize kubernetes manager") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorWaitForKustomizations", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		mocks.KubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, names ...string) error {
			return fmt.Errorf("wait failed")
		}

		blueprint := createTestBlueprint()
		err := infra.Wait(blueprint)

		if err == nil {
			t.Error("Expected error for wait for kustomizations failure")
		}

		if !strings.Contains(err.Error(), "failed waiting for kustomizations") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestInfrastructure_Close(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)

		closeCalled := false
		mocks.ClusterClient.CloseFunc = func() {
			closeCalled = true
		}

		infra.Close()

		if !closeCalled {
			t.Error("Expected ClusterClient.Close to be called")
		}
	})

	t.Run("HandlesNilClusterClient", func(t *testing.T) {
		mocks := setupInfrastructureMocks(t)
		infra := NewInfrastructure(mocks.InfrastructureExecutionContext)
		infra.ClusterClient = nil

		infra.Close()

		if infra.ClusterClient != nil {
			t.Error("Expected nil cluster client to remain nil after Close")
		}
	})
}

// =============================================================================
// Test InfrastructureExecutionContext
// =============================================================================

func TestInfrastructureExecutionContext(t *testing.T) {
	t.Run("CreatesInfrastructureExecutionContext", func(t *testing.T) {
		execCtx := &types.ExecutionContext{
			ContextName:  "test-context",
			ProjectRoot:  "/test/project",
			ConfigRoot:   "/test/project/contexts/test-context",
			TemplateRoot: "/test/project/contexts/_template",
		}

		infraCtx := &InfrastructureExecutionContext{
			ExecutionContext: *execCtx,
		}

		if infraCtx.ContextName != "test-context" {
			t.Errorf("Expected context name 'test-context', got: %s", infraCtx.ContextName)
		}

		if infraCtx.ProjectRoot != "/test/project" {
			t.Errorf("Expected project root '/test/project', got: %s", infraCtx.ProjectRoot)
		}

		if infraCtx.ConfigRoot != "/test/project/contexts/test-context" {
			t.Errorf("Expected config root '/test/project/contexts/test-context', got: %s", infraCtx.ConfigRoot)
		}

		if infraCtx.TemplateRoot != "/test/project/contexts/_template" {
			t.Errorf("Expected template root '/test/project/contexts/_template', got: %s", infraCtx.TemplateRoot)
		}
	})
}
