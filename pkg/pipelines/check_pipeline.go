package pipelines

import (
	"context"
	"fmt"
	"time"

	"github.com/windsorcli/cli/pkg/cluster"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/tools"
)

// The CheckPipeline is a specialized component that manages tool version checking and node health checking functionality.
// It provides check-specific command execution including tools verification and cluster node health validation,
// configuration validation, and shell integration for the Windsor CLI check command.
// The CheckPipeline handles both basic tool checking and advanced node health monitoring operations.

// =============================================================================
// Types
// =============================================================================

// CheckConstructors defines constructor functions for CheckPipeline dependencies
type CheckConstructors struct {
	NewConfigHandler func(di.Injector) config.ConfigHandler
	NewShell         func(di.Injector) shell.Shell
	NewToolsManager  func(di.Injector) tools.ToolsManager
	NewClusterClient func(di.Injector) cluster.ClusterClient
	NewShims         func() *Shims
}

// CheckPipeline provides tool checking and node health checking functionality
type CheckPipeline struct {
	BasePipeline

	constructors CheckConstructors

	configHandler config.ConfigHandler
	shell         shell.Shell
	toolsManager  tools.ToolsManager
	clusterClient cluster.ClusterClient
	shims         *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewCheckPipeline creates a new CheckPipeline instance with optional constructors
func NewCheckPipeline(constructors ...CheckConstructors) *CheckPipeline {
	var ctors CheckConstructors
	if len(constructors) > 0 {
		ctors = constructors[0]
	} else {
		ctors = CheckConstructors{
			NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
				return config.NewYamlConfigHandler(injector)
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewDefaultShell(injector)
			},
			NewToolsManager: func(injector di.Injector) tools.ToolsManager {
				return tools.NewToolsManager(injector)
			},
			NewClusterClient: func(injector di.Injector) cluster.ClusterClient {
				return cluster.NewTalosClusterClient(injector)
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}
	}

	return &CheckPipeline{
		BasePipeline: *NewBasePipeline(),
		constructors: ctors,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize creates and registers all required components for the check pipeline.
// It sets up the config handler, shell, tools manager, and cluster client in the correct order,
// registering each component with the dependency injector and initializing them sequentially
// to ensure proper dependency resolution.
func (p *CheckPipeline) Initialize(injector di.Injector) error {
	p.shims = p.constructors.NewShims()

	if existing := injector.Resolve("shell"); existing != nil {
		p.shell = existing.(shell.Shell)
	} else {
		p.shell = p.constructors.NewShell(injector)
		injector.Register("shell", p.shell)
	}
	p.BasePipeline.shell = p.shell

	if existing := injector.Resolve("configHandler"); existing != nil {
		p.configHandler = existing.(config.ConfigHandler)
	} else {
		p.configHandler = p.constructors.NewConfigHandler(injector)
		injector.Register("configHandler", p.configHandler)
	}
	p.BasePipeline.configHandler = p.configHandler

	if existing := injector.Resolve("toolsManager"); existing != nil {
		p.toolsManager = existing.(tools.ToolsManager)
	} else {
		p.toolsManager = p.constructors.NewToolsManager(injector)
		injector.Register("toolsManager", p.toolsManager)
	}

	if existing := injector.Resolve("clusterClient"); existing != nil {
		p.clusterClient = existing.(cluster.ClusterClient)
	} else {
		p.clusterClient = p.constructors.NewClusterClient(injector)
		injector.Register("clusterClient", p.clusterClient)
	}

	if err := p.shell.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize shell: %w", err)
	}

	if err := p.configHandler.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize config handler: %w", err)
	}

	if err := p.loadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := p.toolsManager.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize tools manager: %w", err)
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
