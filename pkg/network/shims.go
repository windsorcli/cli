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
// Shims
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		Goos:      goos,
		Stat:      stat,
		WriteFile: writeFile,
		ReadFile:  readFile,
		ReadLink:  readLink,
		MkdirAll:  mkdirAll,
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

// =============================================================================
// Global Shims
// =============================================================================

// goos is a function that returns the current operating system, allowing for override
var goos = func() string {
	return runtime.GOOS
}

// stat is a wrapper around os.Stat
var stat = os.Stat

// writeFile is a wrapper around os.WriteFile
var writeFile = os.WriteFile

// readFile is a wrapper around os.ReadFile
var readFile = os.ReadFile

// readLink is a wrapper around os.Readlink
var readLink = os.Readlink

// mkdirAll is a wrapper around os.MkdirAll
var mkdirAll = os.MkdirAll
