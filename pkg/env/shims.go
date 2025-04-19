// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

package env

import (
	"crypto/rand"
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
	YamlUnmarshal  func([]byte, interface{}) error
	YamlMarshal    func(interface{}) ([]byte, error)
	Remove         func(string) error
	RemoveAll      func(string) error
	CryptoRandRead func([]byte) (int, error)
	Goos           func() string
	UserHomeDir    func() (string, error)
	MkdirAll       func(string, os.FileMode) error
	ReadFile       func(string) ([]byte, error)
	LookPath       func(string) (string, error)
	LookupEnv      func(string) (string, bool)
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
		Remove:         os.Remove,
		RemoveAll:      os.RemoveAll,
		CryptoRandRead: func(b []byte) (int, error) { return rand.Read(b) },
		Goos:           func() string { return runtime.GOOS },
		UserHomeDir:    os.UserHomeDir,
		MkdirAll:       os.MkdirAll,
		ReadFile:       os.ReadFile,
		LookPath:       exec.LookPath,
		LookupEnv:      os.LookupEnv,
	}
}

// stat is a variable that holds the os.Stat function for mocking
var stat = os.Stat

// Define a variable for os.Getwd() for easier testing
var getwd = os.Getwd

// Define a variable for filepath.Glob for easier testing
var glob = filepath.Glob

// Wrapper function for os.WriteFile
var writeFile = os.WriteFile

// Wrapper function for os.ReadDir
var readDir = os.ReadDir

// Wrapper function for yaml.Unmarshal
var yamlUnmarshal = yaml.Unmarshal

// Wrapper function for yaml.Marshal
var yamlMarshal = yaml.Marshal

// Wrapper for os.Remove for mocking in tests
var osRemove = os.Remove

// Wrapper for os.RemoveAll for mocking in tests
var osRemoveAll = os.RemoveAll

// Wrapper for crypto/rand.Read for mocking in tests
var cryptoRandRead = func(b []byte) (int, error) {
	return rand.Read(b)
}

// intPtr returns a pointer to an int value
func intPtr(i int) *int {
	return &i
}

// stringPtr returns a pointer to a string value
func stringPtr(s string) *string {
	return &s
}

// Define a variable for runtime.GOOS for easier testing
var goos = func() string {
	return runtime.GOOS
}

// Define a variable for os.UserHomeDir for easier testing
var osUserHomeDir = os.UserHomeDir

// Define a variable for os.MkdirAll for easier testing
var mkdirAll = os.MkdirAll

// Define a variable for os.ReadFile for easier testing
var readFile = os.ReadFile

// Define a variable for exec.LookPath for easier testing
var execLookPath = exec.LookPath

// Define a variable for os.LookupEnv for easier testing
var osLookupEnv = os.LookupEnv
