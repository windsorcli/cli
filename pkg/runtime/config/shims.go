package config

import (
	"crypto/rand"
	"os"
	"regexp"

	"github.com/goccy/go-yaml"
)

// =============================================================================
// New Shims
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	ReadFile          func(string) ([]byte, error)
	WriteFile         func(string, []byte, os.FileMode) error
	RemoveAll         func(string) error
	Getenv            func(string) string
	Setenv            func(string, string) error
	Stat              func(string) (os.FileInfo, error)
	MkdirAll          func(string, os.FileMode) error
	YamlMarshal       func(any) ([]byte, error)
	YamlUnmarshal     func([]byte, any) error
	CryptoRandRead    func([]byte) (int, error)
	RegexpMatchString func(pattern, s string) (bool, error)
}

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		ReadFile:          os.ReadFile,
		WriteFile:         os.WriteFile,
		RemoveAll:         os.RemoveAll,
		Getenv:            os.Getenv,
		Setenv:            os.Setenv,
		Stat:              os.Stat,
		MkdirAll:          os.MkdirAll,
		YamlMarshal:       yaml.Marshal,
		YamlUnmarshal:     yaml.Unmarshal,
		CryptoRandRead:    func(b []byte) (int, error) { return rand.Read(b) },
		RegexpMatchString: regexp.MatchString,
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}

func ptrInt(i int) *int {
	return &i
}
