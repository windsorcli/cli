package network

import (
	"net"
	"os"
	"runtime"
)

// goos is a wrapper around runtime.GOOS
func goos() string { return runtime.GOOS }

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

// NetworkInterfaceProvider abstracts the system's network interface operations
type NetworkInterfaceProvider interface {
	Interfaces() ([]net.Interface, error)
	InterfaceAddrs(iface net.Interface) ([]net.Addr, error)
}

// RealNetworkInterfaceProvider is the real implementation of NetworkInterfaceProvider
type RealNetworkInterfaceProvider struct{}

// Interfaces returns the system's network interfaces
func (p *RealNetworkInterfaceProvider) Interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

// InterfaceAddrs returns the addresses of a network interface
func (p *RealNetworkInterfaceProvider) InterfaceAddrs(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}
