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
	injector         di.Injector
	shell            shell.Shell
	contextHandler   context.ContextInterface
	cliConfigHandler config.ConfigHandler
}

// VirtualMachine defines methods for VirtualMachine operations
type VirtualMachine interface {
	GetVMInfo() (VMInfo, error)
}

// ContainerRuntime defines methods for container operations
type ContainerRuntime interface {
	GetContainerInfo() ([]ContainerInfo, error)
}

// Initialize is a method that initializes the virt environment
func (v *BaseVirt) Initialize() error {
	resolvedShell, err := v.injector.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}
	v.shell = resolvedShell.(shell.Shell)

	resolvedContextHandler, err := v.injector.Resolve("contextHandler")
	if err != nil {
		return fmt.Errorf("error resolving context handler: %w", err)
	}
	v.contextHandler = resolvedContextHandler.(context.ContextInterface)

	resolvedCliConfigHandler, err := v.injector.Resolve("cliConfigHandler")
	if err != nil {
		return fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}
	v.cliConfigHandler = resolvedCliConfigHandler.(config.ConfigHandler)

	return nil
}
