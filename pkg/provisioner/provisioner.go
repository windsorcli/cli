package provisioner

import (
	"context"
	"fmt"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/cluster"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	k8sclient "github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/tui"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// The Provisioner package provides high-level infrastructure provisioning functionality
// for terraform operations, kubernetes cluster interactions, and cluster API operations.
// It consolidates the creation and management of terraform stacks, kubernetes managers,
// and cluster clients, providing a unified interface for infrastructure lifecycle operations
// across the Windsor CLI.

// =============================================================================
// Types
// =============================================================================

// Provisioner manages the lifecycle of all infrastructure components (terraform, kubernetes, clusters).
// It provides a unified interface for creating, initializing, and managing these infrastructure components
// with proper dependency injection and error handling.
type Provisioner struct {
	configHandler config.ConfigHandler
	shell         shell.Shell
	evaluator     evaluator.ExpressionEvaluator
	contextName   string
	projectRoot   string
	configRoot    string
	runtime       *runtime.Runtime

	TerraformStack    terraforminfra.Stack
	FluxStack         fluxinfra.Stack
	onTerraformApply  []func(id string) error
	KubernetesManager kubernetes.KubernetesManager
	KubernetesClient  k8sclient.KubernetesClient
	ClusterClient     cluster.ClusterClient
	blueprintHandler  blueprint.BlueprintHandler
}

// =============================================================================
// Constructor
// =============================================================================

// NewProvisioner creates a new Provisioner instance with the provided runtime and blueprint handler.
// It sets up kubernetes manager and kubernetes client. Terraform stack and cluster client
// are initialized lazily when needed by the Up(), Down(), and WaitForHealth() methods.
// Panics if runtime or blueprintHandler are nil.
func NewProvisioner(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler, opts ...*Provisioner) *Provisioner {
	if rt == nil {
		panic("runtime is required")
	}
	if rt.ConfigHandler == nil {
		panic("config handler is required on runtime")
	}
	if rt.Shell == nil {
		panic("shell is required on runtime")
	}
	if rt.Evaluator == nil {
		panic("evaluator is required on runtime")
	}
	if blueprintHandler == nil {
		panic("blueprint handler is required")
	}

	provisioner := &Provisioner{
		configHandler:    rt.ConfigHandler,
		shell:            rt.Shell,
		evaluator:        rt.Evaluator,
		contextName:      rt.ContextName,
		projectRoot:      rt.ProjectRoot,
		configRoot:       rt.ConfigRoot,
		runtime:          rt,
		blueprintHandler: blueprintHandler,
	}

	if len(opts) > 0 && opts[0] != nil {
		overrides := opts[0]
		if overrides.TerraformStack != nil {
			provisioner.TerraformStack = overrides.TerraformStack
		}
		if overrides.FluxStack != nil {
			provisioner.FluxStack = overrides.FluxStack
		}
		if overrides.KubernetesManager != nil {
			provisioner.KubernetesManager = overrides.KubernetesManager
		}
		if overrides.KubernetesClient != nil {
			provisioner.KubernetesClient = overrides.KubernetesClient
		}
		if overrides.ClusterClient != nil {
			provisioner.ClusterClient = overrides.ClusterClient
		}
	}

	if provisioner.KubernetesClient == nil {
		provisioner.KubernetesClient = k8sclient.NewDynamicKubernetesClient()
	}

	if provisioner.KubernetesManager == nil {
		provisioner.KubernetesManager = kubernetes.NewKubernetesManager(provisioner.KubernetesClient, rt.ConfigHandler)
	}

	return provisioner
}

// =============================================================================
// Public Methods
// =============================================================================

// OnTerraformApply registers a hook to run after each Terraform component apply.
func (i *Provisioner) OnTerraformApply(fn func(id string) error) {
	if fn != nil {
		i.onTerraformApply = append(i.onTerraformApply, fn)
	}
}

// Up orchestrates the high-level infrastructure deployment process. It runs Terraform apply when terraform.enabled
// and the stack exists, invoking the given onApply hooks after each component apply (after any hooks registered via
// OnTerraformApply). The blueprint parameter is required.
func (i *Provisioner) Up(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return nil
	}
	hooks := append([]func(id string) error{}, i.onTerraformApply...)
	hooks = append(hooks, onApply...)
	if err := i.TerraformStack.Up(blueprint, hooks...); err != nil {
		return fmt.Errorf("failed to run terraform up: %w", err)
	}
	return nil
}

// Down orchestrates the high-level infrastructure teardown process. It executes terraform destroy operations
// for all components in the stack in reverse dependency order. Components with Destroy set to false are skipped.
// This method coordinates terraform, kubernetes, and cluster operations to tear down the infrastructure.
// Initializes components as needed. The blueprint parameter is required. If terraform is disabled (terraform.enabled is false),
// terraform operations are skipped. Returns an error if any destroy operation fails.
func (i *Provisioner) Down(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return nil
	}
	if err := i.TerraformStack.Down(blueprint); err != nil {
		return fmt.Errorf("failed to run terraform down: %w", err)
	}
	return nil
}

// Apply runs terraform init, plan, and apply for a single component identified by componentID.
// Returns an error if terraform is disabled, the stack cannot be initialized, the component is
// not found, or any terraform operation fails.
func (i *Provisioner) Apply(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	if err := i.TerraformStack.Apply(blueprint, componentID); err != nil {
		return fmt.Errorf("failed to run terraform apply for %s: %w", componentID, err)
	}
	return nil
}

// Plan runs terraform init and plan for a single component identified by componentID.
// It does not apply any changes. Returns an error if the terraform stack cannot be initialized,
// the component is not found, or any terraform operation fails.
func (i *Provisioner) Plan(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	if err := i.TerraformStack.Plan(blueprint, componentID); err != nil {
		return fmt.Errorf("failed to run terraform plan for %s: %w", componentID, err)
	}
	return nil
}

// PlanSummary holds aggregated plan results across all infrastructure layers.
// Terraform contains one entry per enabled component; Kustomize contains one
// entry per non-destroyOnly kustomization. Either slice may be nil when the
// corresponding layer is absent from the blueprint or its tooling is unavailable.
// Hints contains upgrade suggestions collected when required CLI tools are absent.
type PlanSummary struct {
	Terraform []terraforminfra.TerraformComponentPlan
	Kustomize []fluxinfra.KustomizePlan
	Hints     []string
}

// PlanTerraformAll runs terraform init and plan for every enabled component, streaming
// output directly. Returns an error if blueprint is nil, the stack cannot be initialised,
// or any component's plan fails.
func (i *Provisioner) PlanTerraformAll(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.PlanAll(blueprint)
}

// PlanTerraformAllJSON runs terraform plan -json for every enabled component, streaming
// machine-readable JSON lines output directly to stdout. Returns an error if blueprint
// is nil, the stack cannot be initialised, or any component's plan fails.
func (i *Provisioner) PlanTerraformAllJSON(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.PlanAllJSON(blueprint)
}

// PlanTerraformJSON runs terraform plan -json for a single component, streaming
// machine-readable JSON lines output directly to stdout. Returns an error if blueprint
// is nil, the stack cannot be initialised, or the plan fails.
func (i *Provisioner) PlanTerraformJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.PlanJSON(blueprint, componentID)
}

// PlanKustomizeJSON runs kustomize build for the named kustomization (or all when componentID
// is "all") and writes the rendered manifests as JSON to stdout. Returns an error if blueprint
// is nil, the stack cannot be initialised, or the build fails.
func (i *Provisioner) PlanKustomizeJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureFluxStack(); err != nil {
		return err
	}
	return i.FluxStack.PlanJSON(blueprint, componentID)
}

// PlanTerraformComponentSummary plans a single Terraform component and returns its
// structured result. Returns an error only when blueprint is nil or stack initialisation fails.
func (i *Provisioner) PlanTerraformComponentSummary(blueprint *blueprintv1alpha1.Blueprint, componentID string) (terraforminfra.TerraformComponentPlan, error) {
	if blueprint == nil {
		return terraforminfra.TerraformComponentPlan{}, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return terraforminfra.TerraformComponentPlan{}, err
	}
	if i.TerraformStack == nil {
		return terraforminfra.TerraformComponentPlan{}, fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.PlanComponentSummary(blueprint, componentID), nil
}

// PlanKustomizeComponentSummary plans a single Flux kustomization and returns its
// structured result. Returns an error only when blueprint is nil or stack initialisation fails.
func (i *Provisioner) PlanKustomizeComponentSummary(blueprint *blueprintv1alpha1.Blueprint, name string) (fluxinfra.KustomizePlan, error) {
	if blueprint == nil {
		return fluxinfra.KustomizePlan{}, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureFluxStack(); err != nil {
		return fluxinfra.KustomizePlan{}, err
	}
	return i.FluxStack.PlanComponentSummary(blueprint, name), nil
}

// PlanTerraformSummary runs a best-effort summary plan across every Terraform
// component in the blueprint without touching the Flux/Kustomize layer.
// Returns an error only when blueprint is nil or stack initialisation fails.
func (i *Provisioner) PlanTerraformSummary(blueprint *blueprintv1alpha1.Blueprint) (*PlanSummary, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	summary := &PlanSummary{}

	if err := i.ensureTerraformStack(); err != nil {
		return nil, err
	}
	if i.TerraformStack != nil {
		summary.Terraform = i.TerraformStack.PlanSummary(blueprint)
	}

	return summary, nil
}

// PlanKustomizeSummary runs a best-effort summary plan across every Flux
// kustomization in the blueprint without touching the Terraform layer.
// Returns an error only when blueprint is nil or stack initialisation fails.
func (i *Provisioner) PlanKustomizeSummary(blueprint *blueprintv1alpha1.Blueprint) (*PlanSummary, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	summary := &PlanSummary{}

	if err := i.ensureFluxStack(); err != nil {
		return nil, err
	}
	summary.Kustomize, summary.Hints = i.FluxStack.PlanSummary(blueprint)

	return summary, nil
}

// PlanAll runs a best-effort summary plan across every Terraform component and
// Flux kustomization in the blueprint. It initialises both stacks as needed and
// collects per-component results without aborting on individual failures, so
// callers always receive as complete a picture as possible. Returns an error only
// when blueprint is nil or stack initialisation itself fails.
func (i *Provisioner) PlanAll(blueprint *blueprintv1alpha1.Blueprint) (*PlanSummary, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	tfSummary, err := i.PlanTerraformSummary(blueprint)
	if err != nil {
		return nil, err
	}

	k8sSummary, err := i.PlanKustomizeSummary(blueprint)
	if err != nil {
		return nil, err
	}

	return &PlanSummary{
		Terraform: tfSummary.Terraform,
		Kustomize: k8sSummary.Kustomize,
		Hints:     k8sSummary.Hints,
	}, nil
}

// PlanKustomizeAll runs flux diff for every non-destroyOnly kustomization in the blueprint.
// Returns an error if the flux CLI is not found or any diff fails.
func (i *Provisioner) PlanKustomizeAll(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureFluxStack(); err != nil {
		return err
	}
	if err := i.FluxStack.PlanAll(blueprint); err != nil {
		return fmt.Errorf("error planning kustomize: %w", err)
	}
	return nil
}

// PlanKustomizeAllJSON runs kustomize build for every non-destroyOnly kustomization and
// writes JSON to stdout. Returns an error if the kustomize CLI is not found or any build fails.
func (i *Provisioner) PlanKustomizeAllJSON(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureFluxStack(); err != nil {
		return err
	}
	return i.FluxStack.PlanAllJSON(blueprint)
}

// PlanKustomization runs flux diff for a single kustomization identified by componentID.
// Returns an error if the flux CLI is not found, the component is not in the blueprint, or the diff fails.
func (i *Provisioner) PlanKustomization(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureFluxStack(); err != nil {
		return err
	}
	if err := i.FluxStack.Plan(blueprint, componentID); err != nil {
		return fmt.Errorf("error planning kustomize for %s: %w", componentID, err)
	}
	return nil
}

// Install orchestrates the high-level kustomization installation process from the blueprint.
// It initializes the kubernetes manager and applies all blueprint resources in order: creates namespace,
// applies source repositories, and applies all kustomizations. The blueprint must be provided as a parameter.
// Returns an error if any step fails.
func (i *Provisioner) Install(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	if err := tui.WithProgress("Installing blueprint resources", func() error {
		return i.KubernetesManager.ApplyBlueprint(blueprint, i.fluxNamespace())
	}); err != nil {
		return fmt.Errorf("failed to apply blueprint: %w", err)
	}

	return nil
}

// Wait waits for kustomizations from the blueprint to be ready. It initializes the kubernetes manager
// if needed and polls the status of all kustomizations until they are ready or a timeout occurs.
// The timeout is calculated from the longest dependency chain in the blueprint. Returns an error if
// the kubernetes manager is not configured, initialization fails, or waiting times out.
func (i *Provisioner) Wait(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	if err := i.KubernetesManager.WaitForKustomizations("Waiting for kustomizations to be ready", blueprint); err != nil {
		return fmt.Errorf("failed waiting for kustomizations: %w", err)
	}

	return nil
}

// Uninstall orchestrates the high-level kustomization teardown process from the blueprint.
// It initializes the kubernetes manager and deletes all blueprint kustomizations, including
// handling cleanup kustomizations. The blueprint must be provided as a parameter.
// Returns an error if any step fails.
func (i *Provisioner) Uninstall(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	if err := tui.WithProgress("Uninstalling blueprint resources", func() error {
		return i.KubernetesManager.DeleteBlueprint(blueprint, i.fluxNamespace())
	}); err != nil {
		return fmt.Errorf("failed to delete blueprint: %w", err)
	}

	return nil
}

// CheckNodeHealth performs health checks for cluster nodes and Kubernetes endpoints.
// It supports checking node health via cluster client (for Talos/Omni clusters) and/or
// Kubernetes API health checks. The method handles timeout configuration, version checking,
// and node readiness verification. Returns an error if any health check fails.
func (i *Provisioner) CheckNodeHealth(ctx context.Context, options NodeHealthCheckOptions, outputFunc func(string)) error {
	hasNodeCheck := len(options.Nodes) > 0
	hasK8sCheck := options.K8SEndpointProvided

	if !hasNodeCheck && !hasK8sCheck {
		return fmt.Errorf("no health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform")
	}

	if hasNodeCheck {
		if i.ClusterClient == nil {
			clusterDriver := i.configHandler.GetString("cluster.driver", "")
			if values, err := i.configHandler.GetContextValues(); err == nil && values != nil {
				if clusterMap, ok := values["cluster"].(map[string]any); ok {
					if driver, ok := clusterMap["driver"].(string); ok {
						clusterDriver = driver
					}
				}
			}
			if clusterDriver == "talos" {
				i.ClusterClient = cluster.NewTalosClusterClient()
			}
		}

		if i.ClusterClient == nil {
			if !hasK8sCheck {
				return fmt.Errorf("no health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform")
			}
			// If we have k8s check, we can continue without cluster client
		}

		if i.ClusterClient != nil {
			defer i.ClusterClient.Close()

			var checkCtx context.Context
			var cancel context.CancelFunc
			if options.Timeout > 0 {
				checkCtx, cancel = context.WithTimeout(ctx, options.Timeout)
			} else {
				checkCtx, cancel = context.WithCancel(ctx)
			}
			defer cancel()

			if err := i.ClusterClient.WaitForNodesHealthy(checkCtx, options.Nodes, options.Version, options.SkipServices); err != nil {
				if hasK8sCheck {
					if outputFunc != nil {
						outputFunc(fmt.Sprintf("Warning: Cluster client failed (%v), continuing with Kubernetes checks\n", err))
					}
				} else {
					return fmt.Errorf("nodes failed health check: %w", err)
				}
			} else {
				if outputFunc != nil {
					message := fmt.Sprintf("All %d nodes are healthy", len(options.Nodes))
					if options.Version != "" {
						message += fmt.Sprintf(" and running version %s", options.Version)
					}
					outputFunc(message)
				}
			}
		}
	}

	if hasK8sCheck {
		if i.KubernetesManager == nil {
			return fmt.Errorf("no kubernetes manager found")
		}

		k8sEndpointStr := options.K8SEndpoint
		if k8sEndpointStr == "true" {
			k8sEndpointStr = ""
		}

		var nodeNames []string
		if options.CheckNodeReady {
			if hasNodeCheck {
				nodeNames = options.Nodes
			} else {
				return fmt.Errorf("--ready flag requires --nodes to be specified")
			}
		}

		if len(nodeNames) > 0 && outputFunc != nil {
			outputFunc(fmt.Sprintf("Waiting for %d nodes to be Ready...", len(nodeNames)))
		}

		if err := i.KubernetesManager.WaitForKubernetesHealthy(ctx, k8sEndpointStr, outputFunc, nodeNames...); err != nil {
			return fmt.Errorf("kubernetes health check failed: %w", err)
		}

		if outputFunc != nil {
			if len(nodeNames) > 0 {
				readyStatus, err := i.KubernetesManager.GetNodeReadyStatus(ctx, nodeNames)
				allFoundAndReady := err == nil && len(readyStatus) == len(nodeNames)
				for _, ready := range readyStatus {
					if !ready {
						allFoundAndReady = false
						break
					}
				}

				if allFoundAndReady {
					if k8sEndpointStr != "" {
						outputFunc(fmt.Sprintf("Kubernetes API endpoint %s is healthy and all nodes are Ready", k8sEndpointStr))
					} else {
						outputFunc("Kubernetes API endpoint (kubeconfig default) is healthy and all nodes are Ready")
					}
				} else {
					if k8sEndpointStr != "" {
						outputFunc(fmt.Sprintf("Kubernetes API endpoint %s is healthy", k8sEndpointStr))
					} else {
						outputFunc("Kubernetes API endpoint (kubeconfig default) is healthy")
					}
				}
			} else {
				if k8sEndpointStr != "" {
					outputFunc(fmt.Sprintf("Kubernetes API endpoint %s is healthy", k8sEndpointStr))
				} else {
					outputFunc("Kubernetes API endpoint (kubeconfig default) is healthy")
				}
			}
		}
	}

	return nil
}

// NodeHealthCheckOptions contains options for node health checking.
type NodeHealthCheckOptions struct {
	Nodes               []string
	Timeout             time.Duration
	Version             string
	K8SEndpoint         string
	K8SEndpointProvided bool
	CheckNodeReady      bool
	SkipServices        []string
}

// Close releases resources held by provisioner components.
// It closes cluster client connections if present. This method should be called when the
// provisioner instance is no longer needed to clean up resources.
func (i *Provisioner) Close() {
	if i.ClusterClient != nil {
		i.ClusterClient.Close()
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// ensureTerraformStack initializes the TerraformStack if terraform is enabled and the stack is not already initialized.
// Returns an error if initialization fails, or nil if terraform is disabled or already initialized.
func (i *Provisioner) ensureTerraformStack() error {
	if i.TerraformStack != nil {
		return nil
	}
	if i.configHandler.GetBool("terraform.enabled", true) {
		i.TerraformStack = terraforminfra.NewStack(i.runtime)
	}
	return nil
}

// ensureFluxStack initializes the FluxStack if it is not already initialized.
func (i *Provisioner) ensureFluxStack() error {
	if i.FluxStack != nil {
		return nil
	}
	i.FluxStack = fluxinfra.NewStack(i.runtime, i.KubernetesManager)
	return nil
}

// fluxNamespace returns the configured Flux system namespace, defaulting to DefaultFluxSystemNamespace.
func (i *Provisioner) fluxNamespace() string {
	return i.configHandler.GetString("flux.namespace", constants.DefaultFluxSystemNamespace)
}
