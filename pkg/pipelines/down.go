package pipelines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
	envvars "github.com/windsorcli/cli/pkg/context/env"
	"github.com/windsorcli/cli/pkg/infrastructure/kubernetes"
	terraforminfra "github.com/windsorcli/cli/pkg/infrastructure/terraform"
	"github.com/windsorcli/cli/pkg/resources/blueprint"
	"github.com/windsorcli/cli/pkg/context/shell"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// The DownPipeline is a specialized component that manages the infrastructure teardown phase
// of the Windsor environment. It focuses on blueprint cleanup, stack teardown, container runtime
// shutdown, virtual machine shutdown, and optional cleanup of context-specific artifacts.
// The DownPipeline assumes that env pipeline has been executed to set up environment variables.

// =============================================================================
// Types
// =============================================================================

// DownPipeline provides infrastructure teardown functionality for the down command
type DownPipeline struct {
	BasePipeline
	virtualMachine    virt.VirtualMachine
	containerRuntime  virt.ContainerRuntime
	networkManager    network.NetworkManager
	stack             terraforminfra.Stack
	blueprintHandler  blueprint.BlueprintHandler
	kubernetesClient  kubernetes.KubernetesClient
	kubernetesManager kubernetes.KubernetesManager
	envPrinters       []envvars.EnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewDownPipeline creates a new DownPipeline instance
func NewDownPipeline() *DownPipeline {
	return &DownPipeline{
		BasePipeline: *NewBasePipeline(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the down pipeline components including virtual machine,
// container runtime, network manager, stack, and blueprint handler. It only initializes
// the components needed for the infrastructure teardown phase.
func (p *DownPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}

	p.virtualMachine = p.withVirtualMachine()
	p.containerRuntime = p.withContainerRuntime()
	p.networkManager = p.withNetworking()
	p.stack = p.withStack()
	p.kubernetesClient = p.withKubernetesClient()
	p.kubernetesManager = p.withKubernetesManager()

	p.blueprintHandler = p.withBlueprintHandler()

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
	if p.kubernetesManager != nil {
		if err := p.kubernetesManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize kubernetes manager: %w", err)
		}
	}
	if p.blueprintHandler != nil {
		if err := p.blueprintHandler.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize blueprint handler: %w", err)
		}
	}

	return nil
}

// Execute runs the down pipeline, performing infrastructure teardown in reverse order:
// 1. Set environment variables globally in the process
// 2. Run blueprint cleanup (if not skipped)
// 3. Tear down the stack (if not skipped)
// 4. Tear down container runtime (if enabled)
// 5. Clean up context-specific artifacts (if clean flag is set)
func (p *DownPipeline) Execute(ctx context.Context) error {
	// Run blueprint cleanup before stack down (unless skipped)
	skipK8sFlag := ctx.Value("skipK8s")
	if skipK8sFlag == nil || !skipK8sFlag.(bool) {
		if p.blueprintHandler == nil {
			return fmt.Errorf("No blueprint handler found")
		}
		if err := p.blueprintHandler.LoadConfig(); err != nil {
			return fmt.Errorf("Error loading blueprint config: %w", err)
		}
		if err := p.blueprintHandler.Down(); err != nil {
			return fmt.Errorf("Error running blueprint down: %w", err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Skipping Kubernetes cleanup (--skip-k8s set)")
	}

	// Tear down the stack components (unless skipped)
	skipTerraformFlag := ctx.Value("skipTerraform")
	if skipTerraformFlag == nil || !skipTerraformFlag.(bool) {
		if p.stack == nil {
			return fmt.Errorf("No stack found")
		}
		if p.blueprintHandler == nil {
			return fmt.Errorf("No blueprint handler found")
		}
		// Load blueprint config if not already loaded (e.g., if skipK8s was true)
		if err := p.blueprintHandler.LoadConfig(); err != nil {
			return fmt.Errorf("Error loading blueprint config: %w", err)
		}
		if err := p.blueprintHandler.LoadBlueprint(); err != nil {
			return fmt.Errorf("Error loading blueprint: %w", err)
		}
		blueprint := p.blueprintHandler.Generate()
		if err := p.stack.Down(blueprint); err != nil {
			return fmt.Errorf("Error running stack Down command: %w", err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Skipping Terraform cleanup (--skip-tf set)")
	}

	// Tear down the container runtime if enabled
	containerRuntimeEnabled := p.configHandler.GetBool("docker.enabled")
	skipDockerFlag := ctx.Value("skipDocker")
	if containerRuntimeEnabled && (skipDockerFlag == nil || !skipDockerFlag.(bool)) {
		if p.containerRuntime == nil {
			return fmt.Errorf("No container runtime found")
		}
		if err := p.containerRuntime.Down(); err != nil {
			return fmt.Errorf("Error running container runtime Down command: %w", err)
		}
	} else if skipDockerFlag != nil && skipDockerFlag.(bool) {
		fmt.Fprintln(os.Stderr, "Skipping Docker container cleanup (--skip-docker set)")
	}

	// Clean up context specific artifacts if --clean flag is set
	cleanFlag := ctx.Value("clean")
	if cleanFlag != nil && cleanFlag.(bool) {
		if err := p.performCleanup(); err != nil {
			return fmt.Errorf("Error performing cleanup: %w", err)
		}
	}

	// Print success message
	fmt.Fprintln(os.Stderr, "Windsor environment torn down successfully.")

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// performCleanup performs cleanup of context-specific artifacts including
// configuration cleanup, volumes folder removal, terraform modules removal,
// and generated files removal.
func (p *DownPipeline) performCleanup() error {
	if err := p.configHandler.Clean(); err != nil {
		return fmt.Errorf("Error cleaning up context specific artifacts: %w", err)
	}

	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("Error retrieving project root: %w", err)
	}

	// Delete everything in the .volumes folder
	volumesPath := filepath.Join(projectRoot, ".volumes")
	if err := p.shims.RemoveAll(volumesPath); err != nil {
		return fmt.Errorf("Error deleting .volumes folder: %w", err)
	}

	// Delete the .windsor/.tf_modules folder
	tfModulesPath := filepath.Join(projectRoot, ".windsor", ".tf_modules")
	if err := p.shims.RemoveAll(tfModulesPath); err != nil {
		return fmt.Errorf("Error deleting .windsor/.tf_modules folder: %w", err)
	}

	// Delete .windsor/Corefile
	corefilePath := filepath.Join(projectRoot, ".windsor", "Corefile")
	if err := p.shims.RemoveAll(corefilePath); err != nil {
		return fmt.Errorf("Error deleting .windsor/Corefile: %w", err)
	}

	// Delete .windsor/docker-compose.yaml
	dockerComposePath := filepath.Join(projectRoot, ".windsor", "docker-compose.yaml")
	if err := p.shims.RemoveAll(dockerComposePath); err != nil {
		return fmt.Errorf("Error deleting .windsor/docker-compose.yaml: %w", err)
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*DownPipeline)(nil)
