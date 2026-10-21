// The Shims are the system call abstractions of the network package.
// It provides the shared shims of internal/shims, the ReadLink and MkdirTemp shims, and the
// NetworkInterfaceProvider that reads the network interfaces of the host.
// Tests replace the shims and the provider to simulate the host.

package network

import (
	"net"
	"os"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Interfaces
// =============================================================================

// NetworkInterfaceProvider abstracts the system's network interface operations
type NetworkInterfaceProvider interface {
	Interfaces() ([]net.Interface, error)
	InterfaceAddrs(iface net.Interface) ([]net.Addr, error)
}

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the network package shims.
type Shims struct {
	shims.Shims
	ReadLink  func(string) (string, error)
	MkdirTemp func(dir, pattern string) (string, error)
}

// RealNetworkInterfaceProvider is the real implementation of NetworkInterfaceProvider
type RealNetworkInterfaceProvider struct{}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims:     *shims.NewShims(),
		ReadLink:  os.Readlink,
		MkdirTemp: os.MkdirTemp,
	}
}

// NewNetworkInterfaceProvider creates a new real implementation of NetworkInterfaceProvider
func NewNetworkInterfaceProvider() NetworkInterfaceProvider {
	return &RealNetworkInterfaceProvider{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Interfaces returns the system's network interfaces
func (p *RealNetworkInterfaceProvider) Interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

// InterfaceAddrs returns the addresses of a network interface
func (p *RealNetworkInterfaceProvider) InterfaceAddrs(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure RealNetworkInterfaceProvider implements the NetworkInterfaceProvider interface
var _ NetworkInterfaceProvider = (*RealNetworkInterfaceProvider)(nil)
