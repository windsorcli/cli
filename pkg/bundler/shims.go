package bundler

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// The Shims provides mockable wrappers around system and file operations for testing.
// It provides a unified interface for all external dependencies including file system,
// compression, and data marshaling operations. The Shims serves as a dependency injection
// layer that enables comprehensive unit testing by allowing all system calls to be mocked.

// =============================================================================
// Types
// =============================================================================

// TarWriter provides an interface for tar writing operations
type TarWriter interface {
	WriteHeader(hdr *tar.Header) error
	Write(b []byte) (int, error)
	Close() error
}

// Shims provides mockable wrappers around system and file operations
type Shims struct {
	Stat          func(name string) (os.FileInfo, error)
	Create        func(name string) (io.WriteCloser, error)
	ReadFile      func(name string) ([]byte, error)
	Walk          func(root string, walkFn filepath.WalkFunc) error
	NewGzipWriter func(w io.Writer) *gzip.Writer
	NewTarWriter  func(w io.Writer) TarWriter
	YamlUnmarshal func(data []byte, v any) error
	FilepathRel   func(basepath, targpath string) (string, error)
	YamlMarshal   func(data any) ([]byte, error)
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		Stat: os.Stat,
		// #nosec G304 - User-controlled output path is intentional for build artifact creation
		Create:        func(name string) (io.WriteCloser, error) { return os.Create(name) },
		ReadFile:      os.ReadFile,
		Walk:          filepath.Walk,
		NewGzipWriter: gzip.NewWriter,
		NewTarWriter:  func(w io.Writer) TarWriter { return tar.NewWriter(w) },
		YamlUnmarshal: yaml.Unmarshal,
		FilepathRel:   filepath.Rel,
		YamlMarshal:   yaml.Marshal,
	}
}
