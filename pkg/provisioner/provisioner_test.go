package provisioner

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
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
	terraformruntime "github.com/windsorcli/cli/pkg/runtime/terraform"
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

// captureStderr redirects os.Stderr for the duration of fn and returns what fn wrote to it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = origStderr

	w.Close()
	output, _ := io.ReadAll(r)
	return string(output)
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

	// Use a real temp dir for ConfigRoot and seed it with .kube/config so
	// destroy paths' kubeconfigPresent() probe returns true by default — most
	// tests assume the cluster is reachable. Tests that need the cluster-gone
	// path override Runtime.ConfigRoot to a directory without the kubeconfig
	// before constructing the provisioner.
	configRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configRoot, ".kube"), 0755); err != nil {
		t.Fatalf("failed to seed kubeconfig dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configRoot, ".kube", "config"), []byte("apiVersion: v1\nkind: Config\n"), 0644); err != nil {
		t.Fatalf("failed to seed kubeconfig: %v", err)
	}
	rt := &runtime.Runtime{
		ContextName:       "test-context",
		ProjectRoot:       "/test/project",
		ConfigRoot:        configRoot,
		TemplateRoot:      "/test/project/contexts/_template",
		ConfigHandler:     configHandler,
		Shell:             mockShell,
		Evaluator:         evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/project/contexts/_template"),
		TerraformProvider: &terraformruntime.MockTerraformProvider{},
	}

	terraformStack := terraforminfra.NewMockStack()
	terraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
		return false, nil
	}
	terraformStack.DestroyAllFunc = func(blueprint *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
		return terraforminfra.DestroyOutcome{}, nil
	}

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

// eventRecorder records every presenter event it receives.
type eventRecorder struct {
	events []presenter.Event
}

func (r *eventRecorder) Emit(_ context.Context, e presenter.Event) {
	r.events = append(r.events, e)
}

// operations returns the messages of the operations that started, in order.
func (r *eventRecorder) operations() []string {
	var ops []string
	for _, e := range r.events {
		if e.Kind == presenter.KindApplying {
			ops = append(ops, e.Message)
		}
	}
	return ops
}

// layerRecorder records the per-kustomization calls a layerReporter receives.
type layerRecorder struct {
	calls []string
}

func (r *layerRecorder) Progress(name, text string) {
	r.calls = append(r.calls, "progress "+name+": "+text)
}

func (r *layerRecorder) Start(name string) {
	r.calls = append(r.calls, "start "+name)
}

func (r *layerRecorder) Done(name string) {
	r.calls = append(r.calls, "done "+name)
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
		_, err := provisioner.Up(context.Background(), blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		_, err := provisioner.Up(context.Background(), nil)

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
		_, err := provisioner.Up(context.Background(), blueprint)

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
		_, err := provisioner.Up(context.Background(), blueprint)

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
		mockStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, fmt.Errorf("terraform stack up failed")
		}
		opts := &Provisioner{
			TerraformStack: mockStack,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		_, err := provisioner.Up(context.Background(), blueprint)

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
		var capturedOnApply []func(ctx context.Context, id string) (bool, error)
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			capturedOnApply = onApply
			return false, nil
		}
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)
		provisioner.OnTerraformApply(func(ctx context.Context, id string) (bool, error) { return false, nil })

		_, _ = provisioner.Up(context.Background(), createTestBlueprint())

		if len(capturedOnApply) != 1 {
			t.Errorf("Expected stack to receive 1 onApply hook, got %d", len(capturedOnApply))
		}
	})

	t.Run("RefusesUnresolvedBackendField", func(t *testing.T) {
		// Backend names no real component; must not collapse to "no backend".
		mocks := setupProvisionerMocks(t)
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "gcs"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network/gcp-vpc"},
			},
		}
		_, err := provisioner.Up(context.Background(), bp)
		if err == nil || !strings.Contains(err.Error(), `"backend"`) {
			t.Errorf("expected refusal naming the unresolved backend, got %v", err)
		}
	})
}

func TestProvisioner_UpProgress(t *testing.T) {
	t.Run("RunsTheApplyAsOneTerraformOperation", func(t *testing.T) {
		// Given a provisioner with a mock stack and a recording presenter
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mocks.TerraformStack})
		rec := &eventRecorder{}

		// When the blueprint is applied
		_, err := provisioner.Up(presenter.NewContext(context.Background(), rec), createTestBlueprint())

		// Then one "Applying terraform" operation starts and ends
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(rec.events) != 2 || rec.events[0].Message != "Applying terraform" || rec.events[1].Kind != presenter.KindApplied {
			t.Errorf("unexpected events %+v", rec.events)
		}
	})

	t.Run("EndsTheOperationAsFailedWhenTheStackFails", func(t *testing.T) {
		// Given a provisioner whose stack apply fails
		mocks := setupProvisionerMocks(t)
		stack := terraforminfra.NewMockStack()
		stack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, fmt.Errorf("apply failed")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: stack})
		rec := &eventRecorder{}

		// When the blueprint is applied
		_, err := provisioner.Up(presenter.NewContext(context.Background(), rec), createTestBlueprint())

		// Then the operation ends as failed with the error
		if err == nil || len(rec.events) != 2 || rec.events[1].Kind != presenter.KindFailed {
			t.Errorf("expected a failed operation, got err=%v events=%+v", err, rec.events)
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

		err := provisioner.Apply(context.Background(), createTestBlueprint(), "remote/path")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Apply(context.Background(), nil, "remote/path")

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

		err := provisioner.Apply(context.Background(), createTestBlueprint(), "remote/path")

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

		_ = provisioner.Apply(context.Background(), createTestBlueprint(), "remote/path")

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

		err := provisioner.Apply(context.Background(), createTestBlueprint(), "remote/path")

		if err == nil {
			t.Error("Expected error for terraform stack apply failure")
		}
		if !strings.Contains(err.Error(), "failed to run terraform apply for") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("WarnsAboutOrphanedLocalStateBeforeApplying", func(t *testing.T) {
		// windsorcli/cli#3367: `windsor apply terraform <component>` never ran the
		// orphaned-local-state check at all — only Up did, and only outside a
		// backend pivot. A single-component apply must surface the same warning,
		// including under a plain local backend, which is the common case here.
		mocks := setupProvisionerMocks(t)
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.ApplyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error { return nil }
		mockStack.ListLocalStateComponentIDsFunc = func() ([]string, error) {
			return []string{"remote/old-path"}, nil
		}
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			return componentID == "remote/old-path", nil
		}
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var err error
		stderrOutput := captureStderr(t, func() {
			err = provisioner.Apply(context.Background(), createTestBlueprint(), "remote/path")
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !strings.Contains(stderrOutput, "remote/old-path") {
			t.Errorf("Expected a warning naming the orphaned componentID, got: %q", stderrOutput)
		}
	})
}

func TestProvisioner_MigrateState(t *testing.T) {
	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		skipped, err := provisioner.MigrateState(context.Background(), nil)

		if err == nil {
			t.Fatal("Expected error for nil blueprint")
		}
		if skipped != nil {
			t.Errorf("Expected nil skipped on nil-blueprint error, got %v", skipped)
		}
	})

	t.Run("ForwardsSkippedListOnSuccess", func(t *testing.T) {
		// Given a stack that skips two components and returns no error
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(blueprint *blueprintv1alpha1.Blueprint) ([]string, error) {
			return []string{"network", "cluster"}, nil
		}
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When MigrateState runs
		skipped, err := provisioner.MigrateState(context.Background(), createTestBlueprint())

		// Then the skip list is forwarded untouched to the caller
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(skipped) != 2 || skipped[0] != "network" || skipped[1] != "cluster" {
			t.Errorf("Expected skipped=[network, cluster], got %v", skipped)
		}
	})

	t.Run("PreservesSkippedListOnError", func(t *testing.T) {
		// Given a stack that returns both a partial skip list AND an error — e.g. network
		// was missing (skipped) then cluster's terraform init failed. The provisioner
		// must forward both so bootstrap can assemble a "skipped A, then B failed"
		// diagnostic. Dropping the skip list on the error path contradicts the
		// Stack.MigrateState contract and strands callers who route through the
		// provisioner without the signal they need to investigate.
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(blueprint *blueprintv1alpha1.Blueprint) ([]string, error) {
			return []string{"network"}, fmt.Errorf("terraform init failed for cluster")
		}
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When MigrateState runs
		skipped, err := provisioner.MigrateState(context.Background(), createTestBlueprint())

		// Then the provisioner surfaces both the underlying failure and the partial skip
		if err == nil {
			t.Fatal("Expected MigrateState to surface the stack error")
		}
		if !strings.Contains(err.Error(), "failed to migrate terraform state") {
			t.Errorf("Expected wrapped error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "terraform init failed for cluster") {
			t.Errorf("Expected underlying cause preserved via %%w, got: %v", err)
		}
		if len(skipped) != 1 || skipped[0] != "network" {
			t.Errorf("Expected skipped=[network] preserved on error path, got %v", skipped)
		}
	})
}

func TestProvisioner_MigrateComponentState(t *testing.T) {
	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		if err := provisioner.MigrateComponentState(nil, "backend"); err == nil {
			t.Fatal("Expected error for nil blueprint")
		}
	})

	t.Run("ForwardsComponentIDToStack", func(t *testing.T) {
		// Given a stack that records the requested component ID
		mocks := setupProvisionerMocks(t)
		var seenID string
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateComponentStateFunc = func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
			seenID = componentID
			return nil
		}
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When MigrateComponentState runs
		if err := provisioner.MigrateComponentState(createTestBlueprint(), "backend"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the component ID is forwarded untouched
		if seenID != "backend" {
			t.Errorf("Expected component ID 'backend' forwarded, got %q", seenID)
		}
	})

	t.Run("WrapsStackErrorWithComponentName", func(t *testing.T) {
		// Given the stack returns an error, the provisioner must wrap it with the
		// component name so the surfaced message names what failed; bootstrap shows
		// this directly to the operator.
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateComponentStateFunc = func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
			return fmt.Errorf("init failed")
		}
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		err := provisioner.MigrateComponentState(createTestBlueprint(), "backend")
		if err == nil {
			t.Fatal("Expected error from stack to surface")
		}
		if !strings.Contains(err.Error(), "backend") {
			t.Errorf("Expected wrapped error to name component, got: %v", err)
		}
		if !strings.Contains(err.Error(), "init failed") {
			t.Errorf("Expected underlying cause preserved, got: %v", err)
		}
	})
}

func TestProvisioner_OnTerraformApply(t *testing.T) {
	t.Run("RegistersCallbackInvokedByStackOnUp", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		var hookCalled bool
		var hookID string
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			if len(onApply) > 0 && onApply[0] != nil {
				_, _ = onApply[0](context.Background(), "test-component")
			}
			return false, nil
		}
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)
		provisioner.OnTerraformApply(func(ctx context.Context, id string) (bool, error) {
			hookCalled = true
			hookID = id
			return false, nil
		})

		_, err := provisioner.Up(context.Background(), createTestBlueprint())

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

func TestProvisioner_Notify(t *testing.T) {
	t.Run("DelegatesToInjectedNotifier", func(t *testing.T) {
		// Given a provisioner with a stub Notifier recording the call
		mocks := setupProvisionerMocks(t)
		notifier := fluxinfra.NewMockNotifier()
		var called bool
		var seenBlueprint *blueprintv1alpha1.Blueprint
		notifier.NotifyFunc = func(ctx context.Context, bp *blueprintv1alpha1.Blueprint) error {
			called = true
			seenBlueprint = bp
			return nil
		}
		opts := &Provisioner{Notifier: notifier}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When Notify is called with a blueprint
		blueprint := createTestBlueprint()
		err := provisioner.Notify(context.Background(), blueprint)

		// Then the injected notifier receives the blueprint and nil is returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !called {
			t.Error("Expected Notifier.Notify to be called")
		}
		if seenBlueprint != blueprint {
			t.Error("Expected the same blueprint pointer to be forwarded")
		}
	})

	t.Run("ConstructsDefaultNotifierWhenUnset", func(t *testing.T) {
		// Given a provisioner with no Notifier override
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		if provisioner.Notifier != nil {
			t.Fatal("Expected Notifier to be nil before Notify is called")
		}

		// When Notify is called
		// Then a default Notifier is constructed lazily
		_ = provisioner.Notify(context.Background(), createTestBlueprint())
		if provisioner.Notifier == nil {
			t.Error("Expected Notifier to be constructed on first Notify call")
		}
	})

	t.Run("ReturnsErrorInsteadOfPanicWhenKubernetesClientMissing", func(t *testing.T) {
		// Given a Provisioner built as a struct literal (bypassing NewProvisioner's
		// defaulting) with no KubernetesClient — simulates a future caller or test
		// that forgets to initialise the client field
		provisioner := &Provisioner{runtime: &runtime.Runtime{}}

		// When Notify is called
		err := provisioner.Notify(context.Background(), createTestBlueprint())

		// Then an error is returned rather than a panic inside the TUI spinner
		if err == nil {
			t.Fatal("Expected error on missing KubernetesClient, got nil")
		}
		if !strings.Contains(err.Error(), "kubernetes client") {
			t.Errorf("Expected error to mention kubernetes client, got: %v", err)
		}
	})

	t.Run("ReturnsErrorInsteadOfPanicWhenRuntimeMissing", func(t *testing.T) {
		// Given a Provisioner built as a struct literal with no runtime
		provisioner := &Provisioner{KubernetesClient: k8sclient.NewMockKubernetesClient()}

		// When Notify is called
		err := provisioner.Notify(context.Background(), createTestBlueprint())

		// Then an error is returned rather than panicking in NewNotifier
		if err == nil {
			t.Fatal("Expected error on missing runtime, got nil")
		}
		if !strings.Contains(err.Error(), "runtime") {
			t.Errorf("Expected error to mention runtime, got: %v", err)
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

// =============================================================================
// Test Private Methods
// =============================================================================

func TestProvisioner_kubeconfigPresent(t *testing.T) {
	t.Run("FalseWhenKubeconfigIsMissing", func(t *testing.T) {
		// Given a Stat shim that reports no kubeconfig
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.configRoot = "/config"
		provisioner.shims.Stat = func(string) (os.FileInfo, error) { return nil, fs.ErrNotExist }

		// When the kubeconfig is probed
		present := provisioner.kubeconfigPresent()

		// Then it reports no kubeconfig
		if present {
			t.Error("Expected false when the kubeconfig does not exist")
		}
	})

	t.Run("TrueWhenStatFailsForAnotherReason", func(t *testing.T) {
		// Given a Stat shim that returns a permission error
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.configRoot = "/config"
		provisioner.shims.Stat = func(string) (os.FileInfo, error) { return nil, fs.ErrPermission }

		// When the kubeconfig is probed
		present := provisioner.kubeconfigPresent()

		// Then it reports the kubeconfig as present
		if !present {
			t.Error("Expected true when Stat fails with an error other than not exist")
		}
	})

	t.Run("FalseWhenConfigRootIsEmpty", func(t *testing.T) {
		// Given a provisioner with no config root
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.configRoot = ""

		// When the kubeconfig is probed
		present := provisioner.kubeconfigPresent()

		// Then it reports no kubeconfig
		if present {
			t.Error("Expected false when the config root is empty")
		}
	})
}

func TestProvisioner_recoverHalfMigratedComponents(t *testing.T) {
	t.Run("UpRecoversLeftoverLocalState", func(t *testing.T) {
		// Provisioner.Up runs the recovery sweep before delegating to
		// TerraformStack.Up. For each component with local state but no remote
		// state, the sweep does the two-step reset-and-migrate: under
		// withBackendOverride("local") run InitComponent (writes local pointer
		// file), exit override, then MigrateComponentState (copies local →
		// configured remote). This protects the scenario where the pointer
		// records the configured backend even though state lives in the local
		// file — without the two-step reset, init -migrate-state would migrate
		// empty remote → local and destroy the local state.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network"},
				{Path: "cluster"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("probe-local:%s", componentID))
			return true, nil
		}
		mockStack.HasRemoteStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("probe-remote:%s", componentID))
			return false, nil
		}
		mockStack.InitComponentFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, fmt.Sprintf("init-component:%s", componentID))
			return nil
		}
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, fmt.Sprintf("migrate-component:%s", componentID))
			return nil
		}
		mockStack.RemoveLocalStateFunc = func(componentID string) error {
			ops = append(ops, fmt.Sprintf("remove-local:%s", componentID))
			return nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			ops = append(ops, "up")
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Up(context.Background(), bp); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{
			"probe-local:network",
			"probe-remote:network",
			"set:local",
			"init-component:network",
			"set:azurerm",
			"migrate-component:network",
			"remove-local:network",
			"probe-local:cluster",
			"probe-remote:cluster",
			"set:local",
			"init-component:cluster",
			"set:azurerm",
			"migrate-component:cluster",
			"remove-local:cluster",
			"up",
		}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("ShortCircuitsForLocalBackend", func(t *testing.T) {
		// When the configured backend is "local", local IS the configured backend
		// — there is nothing to migrate to. The sweep must short-circuit before
		// iterating components so a local-backend windsor up does not spend a
		// per-component terraform init + show pair on every invocation.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "cluster"},
			},
		}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		probeLocalCalled := false
		probeRemoteCalled := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.HasLocalStateWithResourcesFunc = func(_ string) (bool, error) {
			probeLocalCalled = true
			return true, nil
		}
		mockStack.HasRemoteStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
			probeRemoteCalled = true
			return true, nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Up(context.Background(), bp); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if probeLocalCalled {
			t.Error("HasLocalStateWithResources must not be called when backend is local — sweep should short-circuit")
		}
		if probeRemoteCalled {
			t.Error("HasRemoteState must not be called when backend is local")
		}
	})

	t.Run("StillDetectsOrphansUnderALocalBackend", func(t *testing.T) {
		// checkOrphanedLocalState is independent of backend type: a componentID can
		// drift under a pure local backend too, and local IS the real backend there —
		// not a staging area recoverHalfMigratedComponents can skip past.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
		}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.ListLocalStateComponentIDsFunc = func() ([]string, error) {
			return []string{"vpc-old"}, nil
		}
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			return componentID == "vpc-old", nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		var err error
		stderrOutput := captureStderr(t, func() {
			_, err = provisioner.Up(context.Background(), bp)
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !strings.Contains(stderrOutput, "vpc-old") {
			t.Errorf("Expected a warning naming the orphaned componentID under a local backend, got: %q", stderrOutput)
		}
	})

	t.Run("AbortsOnProbeFailureToAvoidSilentStateOverwrite", func(t *testing.T) {
		// The reset-and-migrate uses terraform init -migrate-state -force-copy,
		// which unconditionally overwrites the destination. If HasRemoteState
		// fails transiently (auth, network, missing storage) and recovery
		// proceeds, the migrate would silently replace valid remote state with
		// the local file. Fail-fast: probe error is a hard abort with the
		// underlying error wrapped, no migrate runs, no state is touched.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.HasLocalStateWithResourcesFunc = func(_ string) (bool, error) {
			return true, nil
		}
		mockStack.HasRemoteStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
			return false, fmt.Errorf("auth timeout")
		}
		migrateCalled := false
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			migrateCalled = true
			return nil
		}
		initCalled := false
		mockStack.InitComponentFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			initCalled = true
			return nil
		}
		removeCalled := false
		mockStack.RemoveLocalStateFunc = func(_ string) error {
			removeCalled = true
			return nil
		}
		upCalled := false
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			upCalled = true
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Up(context.Background(), bp)
		if err == nil {
			t.Fatal("Expected Up to fail on recovery probe error, got nil")
		}
		if !strings.Contains(err.Error(), "auth timeout") {
			t.Errorf("Expected error to wrap underlying probe error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "recovery sweep aborted") {
			t.Errorf("Expected error to name the recovery sweep, got: %v", err)
		}
		if !strings.Contains(err.Error(), `"network"`) {
			t.Errorf("Expected error to name the affected component, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "overwrite valid remote state") {
			t.Errorf("Expected remediation to explain the data-loss hazard, got: %v", err)
		}
		if initCalled {
			t.Error("InitComponent must not run when probe fails")
		}
		if migrateCalled {
			t.Error("MigrateComponentState must not run when probe fails")
		}
		if removeCalled {
			t.Error("RemoveLocalState must not run when probe fails")
		}
		if upCalled {
			t.Error("TerraformStack.Up must not run when recovery aborts")
		}
	})

	t.Run("DetectsAndWarnsAboutStateStrandedUnderAComponentsPreviousID", func(t *testing.T) {
		// Reproduces windsorcli/cli#3324: a component renamed since its last partial
		// apply leaves local state behind under its old ID. Before this fix,
		// recoverHalfMigratedComponents only iterated the current blueprint's
		// componentIDs, so "crossplane-identity-gcp" (the old ID; the blueprint now
		// only declares "crossplane-identity") was invisible to the sweep. The fix
		// only detects and warns — it deliberately does not migrate or remove
		// anything, since an orphaned ID is just as consistent with a proper
		// destroy-then-remove as with a rename, and the two can't be told apart from
		// an on-disk ID alone.
		//
		// checkOrphanedLocalState (windsorcli/cli#3367) now runs this detection once
		// up front, before recoverHalfMigratedComponents' own sweep — so the orphaned
		// ID is probed first, ahead of the current ID.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "crossplane-identity"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "gcs"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		var probed []string
		mockStack := terraforminfra.NewMockStack()
		mockStack.ListLocalStateComponentIDsFunc = func() ([]string, error) {
			return []string{"crossplane-identity-gcp"}, nil
		}
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			probed = append(probed, componentID)
			// Only the old ID has local state — "crossplane-identity" (the current,
			// renamed-to ID) has never run, so it has none.
			return componentID == "crossplane-identity-gcp", nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, nil
		}

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		var err error
		stderrOutput := captureStderr(t, func() {
			_, err = provisioner.Up(context.Background(), bp)
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Both IDs get probed for local state — the orphaned one by checkOrphanedLocalState
		// up front, the current one by the migration sweep that runs after it.
		expectedProbed := []string{"crossplane-identity-gcp", "crossplane-identity"}
		if len(probed) != len(expectedProbed) {
			t.Fatalf("Expected probes %v, got %v", expectedProbed, probed)
		}
		for i, want := range expectedProbed {
			if probed[i] != want {
				t.Errorf("probe %d: got %q, want %q (full: %v)", i, probed[i], want, probed)
			}
		}
		if !strings.Contains(stderrOutput, "crossplane-identity-gcp") {
			t.Errorf("Expected a warning naming the orphaned componentID, got: %q", stderrOutput)
		}
		if !strings.Contains(stderrOutput, "not migrated automatically") {
			t.Errorf("Expected the warning to say state was not migrated automatically, got: %q", stderrOutput)
		}
	})

	t.Run("DoesNotTreatADisabledComponentAsOrphaned", func(t *testing.T) {
		// A component the operator has merely toggled off (Enabled: false) is still
		// declared in the blueprint — not renamed, not removed. The orphan-detection
		// pass must not mistake it for one just because the enabled-only sweep above
		// skips it before recording its ID.
		mocks := setupProvisionerMocks(t)
		disabled := false
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network", Enabled: &blueprintv1alpha1.BoolExpression{Value: &disabled, IsExpr: false}},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "gcs"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.ListLocalStateComponentIDsFunc = func() ([]string, error) {
			return []string{"network"}, nil
		}
		probeLocalCalled := false
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			probeLocalCalled = true
			return true, nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, nil
		}

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		var err error
		stderrOutput := captureStderr(t, func() {
			_, err = provisioner.Up(context.Background(), bp)
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// The enabled-only sweep skips "network" entirely (disabled), so its local
		// state is never even probed there. The orphan-detection pass must also leave
		// it alone — no probe, no warning — since "network" is still declared.
		if probeLocalCalled {
			t.Error("HasLocalStateWithResources must not be called for a disabled component")
		}
		if strings.Contains(stderrOutput, "network") {
			t.Errorf("Expected no warning about a still-declared component, got: %q", stderrOutput)
		}
	})
}
