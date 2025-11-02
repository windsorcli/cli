package infrastructure

import (
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/infrastructure/cluster"
	"github.com/windsorcli/cli/pkg/infrastructure/kubernetes"
	terraforminfra "github.com/windsorcli/cli/pkg/infrastructure/terraform"
)

// The Infrastructure package provides high-level infrastructure management functionality
// for terraform operations, kubernetes cluster interactions, and cluster API operations.
// It consolidates the creation and management of terraform stacks, kubernetes managers,
// and cluster clients, providing a unified interface for infrastructure lifecycle operations
// across the Windsor CLI.

// =============================================================================
// Types
// =============================================================================

// InfrastructureExecutionContext holds the execution context for infrastructure operations.
// It embeds the base ExecutionContext and includes all infrastructure-specific dependencies.
type InfrastructureExecutionContext struct {
	context.ExecutionContext

	TerraformStack    terraforminfra.Stack
	KubernetesManager kubernetes.KubernetesManager
	KubernetesClient  kubernetes.KubernetesClient
	ClusterClient     cluster.ClusterClient
}

// Infrastructure manages the lifecycle of all infrastructure components (terraform, kubernetes, clusters).
// It provides a unified interface for creating, initializing, and managing these infrastructure components
// with proper dependency injection and error handling.
type Infrastructure struct {
	*InfrastructureExecutionContext
}

// =============================================================================
// Constructor
// =============================================================================

// NewInfrastructure creates a new Infrastructure instance with the provided execution context.
// It sets up all required infrastructure handlers—terraform stack, kubernetes manager, kubernetes client,
// and cluster client—and registers each handler with the dependency injector for use throughout the
// infrastructure lifecycle. The cluster client is created based on the cluster driver configuration (talos/omni).
// Components are initialized lazily when needed by the Up() and Down() methods.
// Returns a pointer to the Infrastructure struct.
func NewInfrastructure(ctx *InfrastructureExecutionContext) *Infrastructure {
	infra := &Infrastructure{
		InfrastructureExecutionContext: ctx,
	}

	if infra.TerraformStack == nil {
		infra.TerraformStack = terraforminfra.NewWindsorStack(infra.Injector)
		infra.Injector.Register("terraformStack", infra.TerraformStack)
	}

	if infra.KubernetesClient == nil {
		infra.KubernetesClient = kubernetes.NewDynamicKubernetesClient()
		infra.Injector.Register("kubernetesClient", infra.KubernetesClient)
	}

	if infra.KubernetesManager == nil {
		infra.KubernetesManager = kubernetes.NewKubernetesManager(infra.Injector)
		infra.Injector.Register("kubernetesManager", infra.KubernetesManager)
	}

	if infra.ClusterClient == nil {
		clusterDriver := infra.ConfigHandler.GetString("cluster.driver", "")
		if clusterDriver == "talos" || clusterDriver == "omni" {
			infra.ClusterClient = cluster.NewTalosClusterClient(infra.Injector)
			infra.Injector.Register("clusterClient", infra.ClusterClient)
		}
	}

	return infra
}

// =============================================================================
// Public Methods
// =============================================================================

// Up orchestrates the high-level infrastructure deployment process. It executes terraform apply operations
// for all components in the stack. This method coordinates terraform, kubernetes, and cluster operations
// to bring up the complete infrastructure. Initializes components as needed. The blueprint parameter is required.
// Returns an error if any step fails.
func (i *Infrastructure) Up(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.TerraformStack == nil {
		return fmt.Errorf("terraform stack not configured")
	}
	if err := i.TerraformStack.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize terraform stack: %w", err)
	}
	if err := i.TerraformStack.Up(blueprint); err != nil {
		return fmt.Errorf("failed to run terraform up: %w", err)
	}
	return nil
}

// Down orchestrates the high-level infrastructure teardown process. It executes terraform destroy operations
// for all components in the stack in reverse dependency order. Components with Destroy set to false are skipped.
// This method coordinates terraform, kubernetes, and cluster operations to tear down the infrastructure.
// Initializes components as needed. The blueprint parameter is required. Returns an error if any destroy operation fails.
func (i *Infrastructure) Down(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.TerraformStack == nil {
		return fmt.Errorf("terraform stack not configured")
	}
	if err := i.TerraformStack.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize terraform stack: %w", err)
	}
	if err := i.TerraformStack.Down(blueprint); err != nil {
		return fmt.Errorf("failed to run terraform down: %w", err)
	}
	return nil
}

// Install orchestrates the high-level kustomization installation process from the blueprint.
// It initializes the kubernetes manager and applies all blueprint resources in order: creates namespace,
// applies source repositories, and applies all kustomizations. The blueprint must be provided as a parameter.
// Returns an error if any step fails.
func (i *Infrastructure) Install(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}
	if err := i.KubernetesManager.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize kubernetes manager: %w", err)
	}

	if err := i.KubernetesManager.ApplyBlueprint(blueprint, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
		return fmt.Errorf("failed to apply blueprint: %w", err)
	}

	return nil
}

// Wait waits for kustomizations from the blueprint to be ready. It initializes the kubernetes manager
// if needed and polls the status of all kustomizations until they are ready or a timeout occurs.
// Returns an error if the kubernetes manager is not configured, initialization fails, or waiting times out.
func (i *Infrastructure) Wait(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}
	if err := i.KubernetesManager.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize kubernetes manager: %w", err)
	}

	kustomizationNames := make([]string, len(blueprint.Kustomizations))
	for i, k := range blueprint.Kustomizations {
		kustomizationNames[i] = k.Name
	}

	if err := i.KubernetesManager.WaitForKustomizations("⏳ Waiting for kustomizations to be ready", kustomizationNames...); err != nil {
		return fmt.Errorf("failed waiting for kustomizations: %w", err)
	}

	return nil
}

// Close releases resources held by infrastructure components.
// It closes cluster client connections if present. This method should be called when the
// infrastructure instance is no longer needed to clean up resources.
func (i *Infrastructure) Close() {
	if i.ClusterClient != nil {
		i.ClusterClient.Close()
	}
}
