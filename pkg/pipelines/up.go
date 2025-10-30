package pipelines

import (
	"context"
	"fmt"
	"os"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/environment/envvars"
	"github.com/windsorcli/cli/pkg/environment/tools"
	terraforminfra "github.com/windsorcli/cli/pkg/infrastructure/terraform"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// The UpPipeline is a specialized component that manages the infrastructure deployment phase
// of the Windsor environment setup. It focuses on tools installation, virtual machine startup,
// container runtime startup, network configuration, and stack deployment.
// The UpPipeline assumes that env and init pipelines have already been executed and handled
// environment variables, secrets, and basic configuration setup.

// =============================================================================
// Types
// =============================================================================

// UpPipeline provides infrastructure deployment functionality for the up command
type UpPipeline struct {
	BasePipeline
	toolsManager     tools.ToolsManager
	virtualMachine   virt.VirtualMachine
	containerRuntime virt.ContainerRuntime
	networkManager   network.NetworkManager
	stack            terraforminfra.Stack
	envPrinters      []envvars.EnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewUpPipeline creates a new UpPipeline instance
func NewUpPipeline() *UpPipeline {
	return &UpPipeline{
		BasePipeline: *NewBasePipeline(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the up pipeline components including tools manager, virtual machine,
// container runtime, network manager, stack, and blueprint handler. It only initializes
// the components needed for the infrastructure deployment phase, since env and init
// pipelines handle the earlier setup phases.
// Initialize sets up the up pipeline components by first constructing all dependencies
// using the "with" methods, then initializing them in sequence. This ensures all
// dependencies are present before any initialization logic is invoked.
func (p *UpPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}

	p.toolsManager = p.withToolsManager()
	p.virtualMachine = p.withVirtualMachine()
	p.containerRuntime = p.withContainerRuntime()
	p.networkManager = p.withNetworking()
	p.stack = p.withStack()

	envPrinters, err := p.withEnvPrinters()
	if err != nil {
		return fmt.Errorf("failed to create env printers: %w", err)
	}
	p.envPrinters = envPrinters

	for _, envPrinter := range p.envPrinters {
		if err := envPrinter.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize env printer: %w", err)
		}
	}

	if p.toolsManager != nil {
		if err := p.toolsManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize tools manager: %w", err)
		}
	}
	if p.virtualMachine != nil {
		if err := p.virtualMachine.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize virtual machine: %w", err)
		}
	}
	if p.containerRuntime != nil {
		if err := p.containerRuntime.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize container runtime: %w", err)
		}
	}

	if secureShell := p.injector.Resolve("secureShell"); secureShell != nil {
		if secureShellInterface, ok := secureShell.(shell.Shell); ok {
			if err := secureShellInterface.Initialize(); err != nil {
				return fmt.Errorf("failed to initialize secure shell: %w", err)
			}
		}
	}

	if p.networkManager != nil {
		if err := p.networkManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize network manager: %w", err)
		}
	}
	if p.stack != nil {
		if err := p.stack.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize stack: %w", err)
		}
	}

	return nil
}

// Execute performs the infrastructure deployment operations including tools installation,
// VM/container startup, networking configuration, stack deployment, and optional blueprint installation.
func (p *UpPipeline) Execute(ctx context.Context) error {
	// Set NO_CACHE environment variable to prevent caching during up operations
	if err := p.shims.Setenv("NO_CACHE", "true"); err != nil {
		return fmt.Errorf("Error setting NO_CACHE environment variable: %w", err)
	}

	// Set environment variables globally in the process (similar to old controller.SetEnvironmentVariables())
	for _, envPrinter := range p.envPrinters {
		envVars, err := envPrinter.GetEnvVars()
		if err != nil {
			return fmt.Errorf("error getting environment variables: %w", err)
		}
		for key, value := range envVars {
			if err := p.shims.Setenv(key, value); err != nil {
				return fmt.Errorf("error setting environment variable %s: %w", key, err)
			}
		}
	}

	// Check and install tools
	if p.toolsManager != nil {
		if err := p.toolsManager.Check(); err != nil {
			return fmt.Errorf("Error checking tools: %w", err)
		}
		if err := p.toolsManager.Install(); err != nil {
			return fmt.Errorf("Error installing tools: %w", err)
		}
	}

	// Start virtual machine if using colima
	vmDriverConfig := p.configHandler.GetString("vm.driver")
	if vmDriverConfig == "colima" {
		if p.virtualMachine == nil {
			return fmt.Errorf("No virtual machine found")
		}
		if err := p.virtualMachine.Up(); err != nil {
			return fmt.Errorf("Error running virtual machine Up command: %w", err)
		}
	}

	// Start container runtime if enabled
	containerRuntimeEnabled := p.configHandler.GetBool("docker.enabled")
	if containerRuntimeEnabled {
		if p.containerRuntime == nil {
			return fmt.Errorf("No container runtime found")
		}
		if err := p.containerRuntime.Up(); err != nil {
			return fmt.Errorf("Error running container runtime Up command: %w", err)
		}
	}

	// Configure networking
	if p.networkManager == nil {
		return fmt.Errorf("No network manager found")
	}

	// Configure networking for the virtual machine
	if vmDriverConfig == "colima" {
		if err := p.networkManager.ConfigureGuest(); err != nil {
			return fmt.Errorf("Error configuring guest network: %w", err)
		}
		if err := p.networkManager.ConfigureHostRoute(); err != nil {
			return fmt.Errorf("Error configuring host network: %w", err)
		}
	}

	// Configure DNS settings
	if dnsEnabled := p.configHandler.GetBool("dns.enabled"); dnsEnabled {
		fmt.Fprintf(os.Stderr, "→ ⚠️  DNS configuration may require administrative privileges\n")

		if err := p.networkManager.ConfigureDNS(); err != nil {
			return fmt.Errorf("Error configuring DNS: %w", err)
		}
	}

	// Bring up the stack
	if p.stack == nil {
		return fmt.Errorf("No stack found")
	}
	if err := p.stack.Up(); err != nil {
		return fmt.Errorf("Error running stack Up command: %w", err)
	}

	// Print success message
	fmt.Fprintln(os.Stderr, "Windsor environment set up successfully.")

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*UpPipeline)(nil)
