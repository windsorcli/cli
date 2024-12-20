package virt

import (
	"fmt"

	"os"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
)

// RETRY_WAIT is the number of seconds to wait between retries when starting or stopping a VM
// If running in CI, no wait is performed
var RETRY_WAIT = func() int {
	return map[bool]int{true: 0, false: 2}[os.Getenv("CI") == "true"]
}()

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

// Virt defines methods for the virt operations
type Virt interface {
	Initialize() error
	Up() error
	Down() error
	PrintInfo() error
	WriteConfig() error
}

type BaseVirt struct {
	injector       di.Injector
	shell          shell.Shell
	contextHandler context.ContextHandler
	configHandler  config.ConfigHandler
}

// VirtualMachine defines methods for VirtualMachine operations
type VirtualMachine interface {
	Virt
	GetVMInfo() (VMInfo, error)
}

// ContainerRuntime defines methods for container operations
type ContainerRuntime interface {
	Virt
	GetContainerInfo(name ...string) ([]ContainerInfo, error)
}

// NewBaseVirt creates a new BaseVirt instance
func NewBaseVirt(injector di.Injector) *BaseVirt {
	return &BaseVirt{injector: injector}
}

// Initialize is a method that initializes the virt environment
func (v *BaseVirt) Initialize() error {
	shellInstance, ok := v.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving shell")
	}
	v.shell = shellInstance

	contextHandlerInstance, ok := v.injector.Resolve("contextHandler").(context.ContextHandler)
	if !ok {
		return fmt.Errorf("error resolving context handler")
	}
	v.contextHandler = contextHandlerInstance

	configHandler, ok := v.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving configHandler")
	}
	v.configHandler = configHandler

	return nil
}
