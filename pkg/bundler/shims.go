package bundler

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"

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
	NewGzipWriter func(w io.Writer) *gzip.Writer
	NewTarWriter  func(w io.Writer) TarWriter
	YamlUnmarshal func(data []byte, v any) error
	YamlMarshal   func(data any) ([]byte, error)
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		Stat:          os.Stat,
		Create:        func(name string) (io.WriteCloser, error) { return os.Create(name) },
		NewGzipWriter: gzip.NewWriter,
		NewTarWriter:  func(w io.Writer) TarWriter { return tar.NewWriter(w) },
		YamlUnmarshal: yaml.Unmarshal,
		YamlMarshal:   yaml.Marshal,
	}
}
