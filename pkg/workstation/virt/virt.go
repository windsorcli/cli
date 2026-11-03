// The Virt is the virtualization abstraction of the workstation.
// It provides the Virt interface and the BaseVirt that the VM and container runtimes embed.
// ColimaVirt runs the VM, and DockerVirt and IncusVirt run the containers.
// Each implementation runs its tool through the shell of the runtime.

package virt

import (
	"context"
	"os"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Constants
// =============================================================================

// RETRY_WAIT is the number of seconds between VM retries. It is 0 when CI is "true".
var RETRY_WAIT = func() int {
	return map[bool]int{true: 0, false: 2}[os.Getenv("CI") == "true"]
}()

// =============================================================================
// Types
// =============================================================================

// VMInfo holds the address and the resources of a VM.
type VMInfo struct {
	Address string
	Arch    string
	CPUs    int
	Disk    int
	Memory  int
	Name    string
}

// BaseVirt holds the dependencies that every Virt implementation shares.
type BaseVirt struct {
	runtime       *runtime.Runtime
	shell         shell.Shell
	configHandler config.ConfigHandler
	projectRoot   string
	shims         *Shims
}

// =============================================================================
// Interfaces
// =============================================================================

// Virt starts, stops, and writes the configuration of a VM or a container runtime.
type Virt interface {
	Up(ctx context.Context) error
	Down(ctx context.Context) error
	WriteConfig() error
}

// VirtualMachine is a Virt that runs a VM.
type VirtualMachine interface {
	Virt
}

// ContainerRuntime is a Virt that runs containers.
type ContainerRuntime interface {
	Virt
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseVirt returns a BaseVirt for rt. It panics when rt, its shell, or its config handler is nil.
func NewBaseVirt(rt *runtime.Runtime) *BaseVirt {
	if rt == nil {
		panic("runtime is required")
	}
	if rt.Shell == nil {
		panic("shell is required on runtime")
	}
	if rt.ConfigHandler == nil {
		panic("config handler is required on runtime")
	}

	return &BaseVirt{
		runtime:       rt,
		shell:         rt.Shell,
		configHandler: rt.ConfigHandler,
		projectRoot:   rt.ProjectRoot,
		shims:         NewShims(),
	}
}
