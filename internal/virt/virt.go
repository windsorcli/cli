package virt

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

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
	Up(verbose ...bool) error
	Down(verbose ...bool) error
	Delete(verbose ...bool) error
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
	resolvedShell, err := v.injector.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}
	shellInstance, ok := resolvedShell.(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved shell is not of type Shell")
	}
	v.shell = shellInstance

	resolvedContextHandler, err := v.injector.Resolve("contextHandler")
	if err != nil {
		return fmt.Errorf("error resolving context handler: %w", err)
	}
	contextHandlerInstance, ok := resolvedContextHandler.(context.ContextHandler)
	if !ok {
		return fmt.Errorf("resolved context handler is not of type ContextHandler")
	}
	v.contextHandler = contextHandlerInstance

	resolvedConfigHandler, err := v.injector.Resolve("configHandler")
	if err != nil {
		return fmt.Errorf("error resolving configHandler: %w", err)
	}
	configHandler, ok := resolvedConfigHandler.(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("resolved configHandler is not of type ConfigHandler")
	}
	v.configHandler = configHandler

	return nil
}
