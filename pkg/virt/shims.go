// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

package virt

import (
	"encoding/json"
	"io"
	"os"
	"runtime"

	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/mem"
)

// =============================================================================
// Types
// =============================================================================

// YAMLEncoder is an interface for encoding YAML data.
type YAMLEncoder interface {
	Encode(v any) error
	Close() error
}

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	Setenv         func(key, value string) error
	UnmarshalJSON  func(data []byte, v any) error
	UserHomeDir    func() (string, error)
	MkdirAll       func(path string, perm os.FileMode) error
	WriteFile      func(name string, data []byte, perm os.FileMode) error
	Rename         func(oldpath, newpath string) error
	Stat           func(name string) (os.FileInfo, error)
	GOARCH         func() string
	NumCPU         func() int
	VirtualMemory  func() (*mem.VirtualMemoryStat, error)
	MarshalYAML    func(v any) ([]byte, error)
	NewYAMLEncoder func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		Setenv:        os.Setenv,
		UnmarshalJSON: json.Unmarshal,
		UserHomeDir:   os.UserHomeDir,
		MkdirAll:      os.MkdirAll,
		WriteFile:     os.WriteFile,
		Rename:        os.Rename,
		Stat:          os.Stat,
		GOARCH:        func() string { return runtime.GOARCH },
		NumCPU:        func() int { return runtime.NumCPU() },
		VirtualMemory: mem.VirtualMemory,
		MarshalYAML:   yaml.Marshal,
		NewYAMLEncoder: func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return yaml.NewEncoder(w, opts...)
		},
	}
}

// ptrString is a function that creates a pointer to a string.
func ptrString(s string) *string {
	return &s
}

// ptrBool is a function that creates a pointer to a bool.
func ptrBool(b bool) *bool {
	return &b
}
