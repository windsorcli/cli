// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

package envvars

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/goccy/go-yaml"
)

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	Stat           func(string) (os.FileInfo, error)
	Getwd          func() (string, error)
	Glob           func(string) ([]string, error)
	WriteFile      func(string, []byte, os.FileMode) error
	ReadDir        func(string) ([]os.DirEntry, error)
	YamlUnmarshal  func([]byte, any) error
	YamlMarshal    func(any) ([]byte, error)
	JsonUnmarshal  func([]byte, any) error
	Remove         func(string) error
	RemoveAll      func(string) error
	CryptoRandRead func([]byte) (int, error)
	Goos           func() string
	UserHomeDir    func() (string, error)
	MkdirAll       func(string, os.FileMode) error
	ReadFile       func(string) ([]byte, error)
	LookPath       func(string) (string, error)
	LookupEnv      func(string) (string, bool)
	Environ        func() []string
	Getenv         func(string) string
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		Stat:           os.Stat,
		Getwd:          os.Getwd,
		Glob:           filepath.Glob,
		WriteFile:      os.WriteFile,
		ReadDir:        os.ReadDir,
		YamlUnmarshal:  yaml.Unmarshal,
		YamlMarshal:    yaml.Marshal,
		JsonUnmarshal:  json.Unmarshal,
		Remove:         os.Remove,
		RemoveAll:      os.RemoveAll,
		CryptoRandRead: func(b []byte) (int, error) { return rand.Read(b) },
		Goos:           func() string { return runtime.GOOS },
		UserHomeDir:    os.UserHomeDir,
		MkdirAll:       os.MkdirAll,
		ReadFile:       os.ReadFile,
		LookPath:       exec.LookPath,
		LookupEnv:      os.LookupEnv,
		Environ:        os.Environ,
		Getenv:         os.Getenv,
	}
}
