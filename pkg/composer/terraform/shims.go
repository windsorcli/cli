package terraform

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// The shims package is a system call abstraction layer for the terraform package
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

// =============================================================================
// Types
// =============================================================================

// TarReader provides an interface for tar archive reading operations
type TarReader interface {
	Next() (*tar.Header, error)
	Read([]byte) (int, error)
}

// RealTarReader is the real implementation of TarReader
type RealTarReader struct {
	reader *tar.Reader
}

// Next returns the next header in the tar archive
func (r *RealTarReader) Next() (*tar.Header, error) {
	return r.reader.Next()
}

// Read reads data from the current tar entry
func (r *RealTarReader) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	MkdirAll       func(path string, perm os.FileMode) error
	WriteFile      func(filename string, data []byte, perm os.FileMode) error
	ReadFile       func(filename string) ([]byte, error)
	Stat           func(name string) (os.FileInfo, error)
	Chdir          func(dir string) error
	FilepathRel    func(basepath, targpath string) (string, error)
	JsonUnmarshal  func(data []byte, v any) error
	NewBytesReader func(b []byte) *bytes.Reader
	NewTarReader   func(r io.Reader) TarReader
	EOFError       func() error
	TypeDir        func() byte
	Create         func(name string) (*os.File, error)
	Copy           func(dst io.Writer, src io.Reader) (int64, error)
	Chmod          func(name string, mode os.FileMode) error
	Setenv         func(key, value string) error
	ReadDir        func(name string) ([]os.DirEntry, error)
	RemoveAll      func(path string) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		MkdirAll:      os.MkdirAll,
		WriteFile:     os.WriteFile,
		ReadFile:      os.ReadFile,
		Stat:          os.Stat,
		Chdir:         os.Chdir,
		FilepathRel:   filepath.Rel,
		JsonUnmarshal: json.Unmarshal,
		NewBytesReader: func(b []byte) *bytes.Reader {
			return bytes.NewReader(b)
		},
		NewTarReader: func(r io.Reader) TarReader {
			return &RealTarReader{reader: tar.NewReader(r)}
		},
		EOFError: func() error {
			return io.EOF
		},
		TypeDir: func() byte {
			return tar.TypeDir
		},
		Create:    os.Create,
		Copy:      io.Copy,
		Chmod:     os.Chmod,
		Setenv:    os.Setenv,
		ReadDir:   os.ReadDir,
		RemoveAll: os.RemoveAll,
	}
}
