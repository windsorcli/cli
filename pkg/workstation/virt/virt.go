// The virt package is a virtualization management system
// It provides interfaces and base implementations for managing virtual machines and containers
// It serves as the core abstraction layer for virtualization operations in the Windsor CLI
// It supports both VM-based (Colima) and container-based (Docker) virtualization

package virt

import (
	"os"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Constants
// =============================================================================

// RETRY_WAIT is the number of seconds to wait between retries when starting or stopping a VM
// If running in CI, no wait is performed
var RETRY_WAIT = func() int {
	return map[bool]int{true: 0, false: 2}[os.Getenv("CI") == "true"]
}()

// =============================================================================
// Types
// =============================================================================

// VMInfo is a struct that holds the information about the VM
type VMInfo struct {
	Address string
	Arch    string
	CPUs    int
	Disk    int
	Memory  int
	Name    string
}

type ContainerInfo struct {
	Name    string
	Address string
	Labels  map[string]string
}

type BaseVirt struct {
	runtime       *runtime.Runtime
	shell         shell.Shell
	configHandler config.ConfigHandler
	shims         *Shims
}

// =============================================================================
// Interfaces
// =============================================================================

// Virt defines methods for the virt operations
type Virt interface {
	Up() error
	Down() error
	WriteConfig() error
}

// VirtualMachine defines methods for VirtualMachine operations
type VirtualMachine interface {
	Virt
	GetNetworkName() string
	GetIncusRemote() string
}

// ContainerRuntime defines methods for container operations
type ContainerRuntime interface {
	Virt
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseVirt creates a new BaseVirt instance
func NewBaseVirt(rt *runtime.Runtime) *BaseVirt {
	return &BaseVirt{
		runtime:       rt,
		shell:         rt.Shell,
		configHandler: rt.ConfigHandler,
		shims:         NewShims(),
	}
}

// setShims sets the shims for testing purposes
func (v *BaseVirt) setShims(shims *Shims) {
	v.shims = shims
}
