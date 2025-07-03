package generators

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// The shims package is a system call abstraction layer for the generators package
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	WriteFile         func(name string, data []byte, perm os.FileMode) error
	ReadFile          func(name string) ([]byte, error)
	MkdirAll          func(path string, perm os.FileMode) error
	Stat              func(name string) (os.FileInfo, error)
	MarshalYAML       func(v any) ([]byte, error)
	RemoveAll         func(path string) error
	Chdir             func(dir string) error
	ReadDir           func(name string) ([]os.DirEntry, error)
	Setenv            func(key, value string) error
	YamlUnmarshal     func(data []byte, v any) error
	JsonMarshal       func(v any) ([]byte, error)
	JsonUnmarshal     func(data []byte, v any) error
	FilepathRel       func(basepath, targpath string) (string, error)
	NewTarReader      func(r io.Reader) *tar.Reader
	NewBytesReader    func(data []byte) io.Reader
	ReadAll           func(reader io.Reader) ([]byte, error)
	ParseReference    func(ref string, opts ...name.Option) (name.Reference, error)
	RemoteImage       func(ref name.Reference, options ...remote.Option) (v1.Image, error)
	ImageLayers       func(img v1.Image) ([]v1.Layer, error)
	LayerUncompressed func(layer v1.Layer) (io.ReadCloser, error)
	Create            func(path string) (*os.File, error)
	Copy              func(dst io.Writer, src io.Reader) (int64, error)
	Chmod             func(name string, mode os.FileMode) error
	EOFError          func() error
	TypeDir           func() byte
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		WriteFile:         os.WriteFile,
		ReadFile:          os.ReadFile,
		MkdirAll:          os.MkdirAll,
		Stat:              os.Stat,
		MarshalYAML:       yaml.Marshal,
		RemoveAll:         os.RemoveAll,
		Chdir:             os.Chdir,
		ReadDir:           os.ReadDir,
		Setenv:            os.Setenv,
		YamlUnmarshal:     yaml.Unmarshal,
		JsonMarshal:       json.Marshal,
		JsonUnmarshal:     json.Unmarshal,
		FilepathRel:       filepath.Rel,
		NewTarReader:      tar.NewReader,
		NewBytesReader:    func(data []byte) io.Reader { return bytes.NewReader(data) },
		ReadAll:           io.ReadAll,
		ParseReference:    func(ref string, opts ...name.Option) (name.Reference, error) { return name.ParseReference(ref) },
		RemoteImage:       func(ref name.Reference, options ...remote.Option) (v1.Image, error) { return remote.Image(ref) },
		ImageLayers:       func(img v1.Image) ([]v1.Layer, error) { return img.Layers() },
		LayerUncompressed: func(layer v1.Layer) (io.ReadCloser, error) { return layer.Uncompressed() },
		Create:            os.Create,
		Copy:              io.Copy,
		Chmod:             os.Chmod,
		EOFError:          func() error { return io.EOF },
		TypeDir:           func() byte { return tar.TypeDir },
	}
}
