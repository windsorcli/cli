package pipelines

import (
	"context"
	"fmt"
	"time"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/environment/tools"
	"github.com/windsorcli/cli/pkg/infrastructure/cluster"
	"github.com/windsorcli/cli/pkg/infrastructure/kubernetes"
)

// The CheckPipeline is a specialized component that manages tool version checking and node health checking functionality.
// It provides check-specific command execution including tools verification and cluster node health validation,
// configuration validation, and shell integration for the Windsor CLI check command.
// The CheckPipeline handles both basic tool checking and advanced node health monitoring operations.

// =============================================================================
// Types
// =============================================================================

// CheckPipeline implements health checking functionality for tools and cluster nodes
type CheckPipeline struct {
	BasePipeline

	toolsManager      tools.ToolsManager
	clusterClient     cluster.ClusterClient
	kubernetesManager kubernetes.KubernetesManager
}

// =============================================================================
// Constructor
// =============================================================================

// NewCheckPipeline creates a new CheckPipeline instance
func NewCheckPipeline() *CheckPipeline {
	return &CheckPipeline{
		BasePipeline: *NewBasePipeline(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the CheckPipeline by resolving dependencies
func (p *CheckPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}

	p.toolsManager = p.withToolsManager()
	p.clusterClient = p.withClusterClient()
	p.withKubernetesClient()
	p.kubernetesManager = p.withKubernetesManager()

	if p.toolsManager != nil {
		if err := p.toolsManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize tools manager: %w", err)
		}
	}

	if p.kubernetesManager != nil {
		if err := p.kubernetesManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize kubernetes manager: %w", err)
		}
	}

	return nil
}

// Execute performs the check operation based on the operation type specified in the context.
// It supports both "tools" and "node-health" operations, validating configuration and
// executing the appropriate check functionality with proper error handling and output formatting.
func (p *CheckPipeline) Execute(ctx context.Context) error {
	if !p.configHandler.IsLoaded() {
		return fmt.Errorf("Nothing to check. Have you run \033[1mwindsor init\033[0m?")
	}

	operation := ctx.Value("operation")
	if operation == nil {
		return p.executeToolsCheck(ctx)
	}

	operationType, ok := operation.(string)
	if !ok {
		return fmt.Errorf("Invalid operation type")
	}

	switch operationType {
	case "tools":
		return p.executeToolsCheck(ctx)
	case "node-health":
		return p.executeNodeHealthCheck(ctx)
	default:
		return fmt.Errorf("Unknown operation type: %s", operationType)
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// executeToolsCheck performs tool version checking using the tools manager.
// It validates that all required tools are installed and meet minimum version requirements,
// providing success output when all tools are up to date.
func (p *CheckPipeline) executeToolsCheck(ctx context.Context) error {
	if err := p.toolsManager.Check(); err != nil {
		return fmt.Errorf("Error checking tools: %w", err)
	}

	outputFunc := ctx.Value("output")
	if outputFunc != nil {
		if fn, ok := outputFunc.(func(string)); ok {
			fn("All tools are up to date.")
		}
	}

	return nil
}

// executeNodeHealthCheck performs cluster node health checking using the cluster client.
// It validates node health status and optionally checks for specific versions.
// Nodes must be specified via context parameters. Supports timeout configuration.
// If the Kubernetes endpoint flag is provided, performs Kubernetes API health check.
// Outputs status via the output function in context if present.
func (p *CheckPipeline) executeNodeHealthCheck(ctx context.Context) error {
	nodes := ctx.Value("nodes")
	k8sEndpointProvided := ctx.Value("k8s-endpoint-provided")

	var hasNodeCheck bool
	var hasK8sCheck bool

	if nodes != nil {
		if nodeAddresses, ok := nodes.([]string); ok && len(nodeAddresses) > 0 {
			hasNodeCheck = true
		}
	}

	if k8sEndpointProvided != nil {
		if provided, ok := k8sEndpointProvided.(bool); ok && provided {
			hasK8sCheck = true
		}
	}

	if !hasNodeCheck && !hasK8sCheck {
		return fmt.Errorf("No health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform")
	}

	// If we have nodes but no cluster client and no k8s endpoint, we can't perform any checks
	if hasNodeCheck && p.clusterClient == nil && !hasK8sCheck {
		return fmt.Errorf("No health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform")
	}

	// Handle provider-specific node health checks (when --nodes is provided and cluster client is available)
	if hasNodeCheck && p.clusterClient != nil {
		defer p.clusterClient.Close()

		nodeAddresses, ok := nodes.([]string)
		if !ok {
			return fmt.Errorf("Invalid nodes parameter type")
		}

		timeout := ctx.Value("timeout")
		var timeoutDuration time.Duration
		if timeout != nil {
			if t, ok := timeout.(time.Duration); ok {
				timeoutDuration = t
			}
		}

		version := ctx.Value("version")
		var expectedVersion string
		if version != nil {
			if v, ok := version.(string); ok {
				expectedVersion = v
			}
		}

		// Perform provider-specific node health checks
		var checkCtx context.Context
		var cancel context.CancelFunc
		if timeoutDuration > 0 {
			checkCtx, cancel = context.WithTimeout(ctx, timeoutDuration)
		} else {
			checkCtx, cancel = context.WithCancel(ctx)
		}
		defer cancel()

		if err := p.clusterClient.WaitForNodesHealthy(checkCtx, nodeAddresses, expectedVersion); err != nil {
			// If cluster client fails and we have k8s endpoint, continue with k8s checks
			if hasK8sCheck {
				fmt.Printf("Warning: Cluster client failed (%v), continuing with Kubernetes checks\n", err)
			} else {
				return fmt.Errorf("nodes failed health check: %w", err)
			}
		} else {
			outputFunc := ctx.Value("output")
			if outputFunc != nil {
				if fn, ok := outputFunc.(func(string)); ok {
					message := fmt.Sprintf("All %d nodes are healthy", len(nodeAddresses))
					if expectedVersion != "" {
						message += fmt.Sprintf(" and running version %s", expectedVersion)
					}
					fn(message)
				}
			}
		}
	}

	// Handle Kubernetes health checks (API + optional node Ready state)
	if hasK8sCheck {
		if p.kubernetesManager == nil {
			return fmt.Errorf("No kubernetes manager found")
		}

		k8sEndpoint := ctx.Value("k8s-endpoint")
		var k8sEndpointStr string
		if k8sEndpoint != nil {
			if e, ok := k8sEndpoint.(string); ok {
				if e == "true" {
					k8sEndpointStr = ""
				} else {
					k8sEndpointStr = e
				}
			}
		}

		// Only include nodes in the K8s health check if --ready flag is explicitly specified
		var nodeNames []string
		checkNodeReady := ctx.Value("check-node-ready")
		if checkNodeReady != nil {
			if ready, ok := checkNodeReady.(bool); ok && ready {
				if hasNodeCheck {
					// If specific nodes are provided, check those nodes
					if nodeAddresses, ok := nodes.([]string); ok {
						nodeNames = nodeAddresses
					}
				} else {
					// If --ready is specified but no --nodes are provided, return an error
					return fmt.Errorf("--ready flag requires --nodes to be specified")
				}
			}
		}

		// Show waiting message if we're going to check node readiness
		if len(nodeNames) > 0 {
			outputFunc := ctx.Value("output")
			if outputFunc != nil {
				if fn, ok := outputFunc.(func(string)); ok {
					fn(fmt.Sprintf("Waiting for %d nodes to be Ready...", len(nodeNames)))
				}
			}
		}

		// Get output function for progress feedback
		var progressOutputFunc func(string)
		output := ctx.Value("output")
		if output != nil {
			if fn, ok := output.(func(string)); ok {
				progressOutputFunc = fn
			}
		}

		if err := p.kubernetesManager.WaitForKubernetesHealthy(ctx, k8sEndpointStr, progressOutputFunc, nodeNames...); err != nil {
			return fmt.Errorf("Kubernetes health check failed: %w", err)
		}

		outputFunc := ctx.Value("output")
		if outputFunc != nil {
			if fn, ok := outputFunc.(func(string)); ok {
				if len(nodeNames) > 0 {
					// Check if all requested nodes were found and ready
					readyStatus, err := p.kubernetesManager.GetNodeReadyStatus(ctx, nodeNames)
					allFoundAndReady := err == nil && len(readyStatus) == len(nodeNames)
					for _, ready := range readyStatus {
						if !ready {
							allFoundAndReady = false
							break
						}
					}

					if allFoundAndReady {
						if k8sEndpointStr != "" {
							fn(fmt.Sprintf("Kubernetes API endpoint %s is healthy and all nodes are Ready", k8sEndpointStr))
						} else {
							fn("Kubernetes API endpoint (kubeconfig default) is healthy and all nodes are Ready")
						}
					} else {
						if k8sEndpointStr != "" {
							fn(fmt.Sprintf("Kubernetes API endpoint %s is healthy", k8sEndpointStr))
						} else {
							fn("Kubernetes API endpoint (kubeconfig default) is healthy")
						}
					}
				} else {
					if k8sEndpointStr != "" {
						fn(fmt.Sprintf("Kubernetes API endpoint %s is healthy", k8sEndpointStr))
					} else {
						fn("Kubernetes API endpoint (kubeconfig default) is healthy")
					}
				}
			}
		}
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*CheckPipeline)(nil)
