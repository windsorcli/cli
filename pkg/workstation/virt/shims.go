// The Shims are the system call abstractions of the virt package.
// It provides the shared shims of internal/shims and the runtime, memory, and YAML encoder shims.
// Tests replace the shims to simulate the host and the file system.
// NewShims returns the shims with the real implementations.

package virt

import (
	"io"
	"runtime"

	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/mem"
	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Interfaces
// =============================================================================

// YAMLEncoder is an interface for encoding YAML data.
type YAMLEncoder interface {
	Encode(v any) error
	Close() error
}

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the host shims of the virt package.
type Shims struct {
	shims.Shims
	GOARCH         func() string
	NumCPU         func() int
	VirtualMemory  func() (*mem.VirtualMemoryStat, error)
	NewYAMLEncoder func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims:         *shims.NewShims(),
		GOARCH:        func() string { return runtime.GOARCH },
		NumCPU:        func() int { return runtime.NumCPU() },
		VirtualMemory: mem.VirtualMemory,
		NewYAMLEncoder: func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return yaml.NewEncoder(w, opts...)
		},
	}
}
