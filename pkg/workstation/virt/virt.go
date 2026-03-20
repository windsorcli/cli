// The virt package is a virtualization management system
// It provides interfaces and base implementations for managing virtual machines and containers
// It serves as the core abstraction layer for virtualization operations in the Windsor CLI
// It supports both VM-based (Colima) and container-based (Docker) virtualization

package virt

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
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
	projectRoot   string
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

// setShims sets the shims for testing purposes
func (v *BaseVirt) setShims(shims *Shims) {
	v.shims = shims
}

// withProgress runs fn with a spinner and prints success or failure. If verbose, prints message and runs fn without spinner.
func (v *BaseVirt) withProgress(message string, fn func() error) error {
	if v.shell.IsVerbose() {
		fmt.Fprintln(os.Stderr, message)
		return fn()
	}
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"), spinner.WithWriter(os.Stderr))
	spin.Suffix = " " + message
	spin.Start()
	err := fn()
	spin.Stop()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
		return err
	}
	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
	return nil
}
