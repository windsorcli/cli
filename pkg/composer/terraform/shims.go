// The Shims are the system call abstractions of the terraform package.
// It provides the shared shims of internal/shims and the tar, path, and file shims of the module resolvers.
// Tests replace the shims to simulate file system state, tar archives, and terraform init output.
// NewShims returns the shims with the real implementations.

package terraform

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// TarReader is the subset of tar.Reader that the terraform package uses.
type TarReader interface {
	Next() (*tar.Header, error)
	Read([]byte) (int, error)
}

// Shims holds the shared system call primitives and the terraform-specific shims.
type Shims struct {
	shims.Shims
	Chdir          func(dir string) error
	FilepathRel    func(basepath, targpath string) (string, error)
	NewBytesReader func(b []byte) *bytes.Reader
	NewTarReader   func(r io.Reader) TarReader
	EOFError       func() error
	TypeDir        func() byte
	Create         func(name string) (*os.File, error)
	Copy           func(dst io.Writer, src io.Reader) (int64, error)
	Chmod          func(name string, mode os.FileMode) error
	FilepathAbs    func(path string) (string, error)
	FilepathBase   func(path string) string
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims:       *shims.NewShims(),
		Chdir:       os.Chdir,
		FilepathRel: filepath.Rel,
		NewBytesReader: func(b []byte) *bytes.Reader {
			return bytes.NewReader(b)
		},
		NewTarReader: func(r io.Reader) TarReader {
			return tar.NewReader(r)
		},
		EOFError: func() error {
			return io.EOF
		},
		TypeDir: func() byte {
			return tar.TypeDir
		},
		Create:       os.Create,
		Copy:         io.Copy,
		Chmod:        os.Chmod,
		FilepathAbs:  filepath.Abs,
		FilepathBase: filepath.Base,
	}
}
