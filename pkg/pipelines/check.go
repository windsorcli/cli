package pipelines

import (
	"context"
	"fmt"
	"time"

	"github.com/windsorcli/cli/pkg/cluster"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/tools"
)

// The CheckPipeline is a specialized component that manages tool version checking and node health checking functionality.
// It provides check-specific command execution including tools verification and cluster node health validation,
// configuration validation, and shell integration for the Windsor CLI check command.
// The CheckPipeline handles both basic tool checking and advanced node health monitoring operations.

// =============================================================================
// Types
// =============================================================================

// CheckPipeline provides tool checking and node health checking functionality
type CheckPipeline struct {
	BasePipeline

	toolsManager  tools.ToolsManager
	clusterClient cluster.ClusterClient
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

// Initialize creates and registers the required components for the check pipeline including
// tools manager and cluster client dependencies. It validates component initialization
// and ensures proper setup for both tool checking and node health monitoring operations.
func (p *CheckPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}

	p.toolsManager = p.withToolsManager()
	p.clusterClient = p.withClusterClient()

	if p.toolsManager != nil {
		if err := p.toolsManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize tools manager: %w", err)
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
// It validates node health status and optionally checks for specific versions,
// requiring nodes to be specified via context parameters and supporting timeout configuration.
func (p *CheckPipeline) executeNodeHealthCheck(ctx context.Context) error {
	if p.clusterClient == nil {
		return fmt.Errorf("No cluster client found")
	}
	defer p.clusterClient.Close()

	nodes := ctx.Value("nodes")
	if nodes == nil {
		return fmt.Errorf("No nodes specified. Use --nodes flag to specify nodes to check")
	}

	nodeAddresses, ok := nodes.([]string)
	if !ok {
		return fmt.Errorf("Invalid nodes parameter type")
	}

	if len(nodeAddresses) == 0 {
		return fmt.Errorf("No nodes specified. Use --nodes flag to specify nodes to check")
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

	var checkCtx context.Context
	var cancel context.CancelFunc
	if timeoutDuration > 0 {
		checkCtx, cancel = context.WithTimeout(ctx, timeoutDuration)
	} else {
		checkCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	if err := p.clusterClient.WaitForNodesHealthy(checkCtx, nodeAddresses, expectedVersion); err != nil {
		return fmt.Errorf("nodes failed health check: %w", err)
	}

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

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*CheckPipeline)(nil)
