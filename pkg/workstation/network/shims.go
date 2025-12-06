package network

import (
	"net"
	"os"
	"runtime"
)

// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	Goos      func() string
	Stat      func(string) (os.FileInfo, error)
	WriteFile func(string, []byte, os.FileMode) error
	ReadFile  func(string) ([]byte, error)
	ReadLink  func(string) (string, error)
	MkdirAll  func(string, os.FileMode) error
}

// NetworkInterfaceProvider abstracts the system's network interface operations
type NetworkInterfaceProvider interface {
	Interfaces() ([]net.Interface, error)
	InterfaceAddrs(iface net.Interface) ([]net.Addr, error)
}

// RealNetworkInterfaceProvider is the real implementation of NetworkInterfaceProvider
type RealNetworkInterfaceProvider struct{}

// =============================================================================
// Constructors
// =============================================================================

// NewNetworkInterfaceProvider creates a new real implementation of NetworkInterfaceProvider
func NewNetworkInterfaceProvider() NetworkInterfaceProvider {
	return &RealNetworkInterfaceProvider{}
}

// =============================================================================
// Shims
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		Goos:      func() string { return runtime.GOOS },
		Stat:      os.Stat,
		WriteFile: os.WriteFile,
		ReadFile:  os.ReadFile,
		ReadLink:  os.Readlink,
		MkdirAll:  os.MkdirAll,
	}
}

// Interfaces returns the system's network interfaces
func (p *RealNetworkInterfaceProvider) Interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

// InterfaceAddrs returns the addresses of a network interface
func (p *RealNetworkInterfaceProvider) InterfaceAddrs(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}
