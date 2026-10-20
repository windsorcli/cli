// The Shims are the system call abstractions of the artifact package.
// It provides the shared shims of internal/shims and the archive, OCI image, and registry shims.
// Tests replace the shims to simulate file system state, tar archives, and registry responses.
// NewShims returns the shims with the real implementations.

package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// TarWriter is the subset of tar.Writer that the artifact package uses.
type TarWriter interface {
	WriteHeader(hdr *tar.Header) error
	Write(b []byte) (int, error)
	Close() error
}

// TarReader is the subset of tar.Reader that the artifact package uses.
type TarReader interface {
	Next() (*tar.Header, error)
	Read([]byte) (int, error)
}

// Shims holds the shared system call primitives and the artifact-specific shims.
type Shims struct {
	shims.Shims
	Create            func(name string) (io.WriteCloser, error)
	Walk              func(root string, walkFn filepath.WalkFunc) error
	NewGzipWriter     func(w io.Writer) *gzip.Writer
	NewTarWriter      func(w io.Writer) TarWriter
	FilepathRel       func(basepath, targpath string) (string, error)
	ReadAll           func(reader io.Reader) ([]byte, error)
	ParseReference    func(ref string, opts ...name.Option) (name.Reference, error)
	RemoteImage       func(ref name.Reference, options ...remote.Option) (v1.Image, error)
	ImageLayers       func(img v1.Image) ([]v1.Layer, error)
	LayerUncompressed func(layer v1.Layer) (io.ReadCloser, error)
	AppendLayers      func(base v1.Image, layers ...v1.Layer) (v1.Image, error)
	ConfigFile        func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error)
	MediaType         func(img v1.Image, mt types.MediaType) v1.Image
	ConfigMediaType   func(img v1.Image, mt types.MediaType) v1.Image
	Annotations       func(img v1.Image, anns map[string]string) v1.Image
	EmptyImage        func() v1.Image
	RemoteGet         func(ref name.Reference, options ...remote.Option) (*remote.Descriptor, error)
	RemoteWriteLayer  func(repo name.Repository, layer v1.Layer, options ...remote.Option) error
	RemoteWrite       func(ref name.Reference, img v1.Image, options ...remote.Option) error
	RemoteList        func(repo name.Repository, options ...remote.Option) ([]string, error)
	NewRepository     func(repo string, opts ...name.Option) (name.Repository, error)
	NewBytesReader    func(b []byte) *bytes.Reader
	NewTarReader      func(r io.Reader) TarReader
	Copy              func(dst io.Writer, src io.Reader) (int64, error)
	Chmod             func(name string, mode os.FileMode) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims: *shims.NewShims(),
		// #nosec G304 - User-controlled output path is intentional for build artifact creation
		Create:        func(name string) (io.WriteCloser, error) { return os.Create(name) },
		Walk:          filepath.Walk,
		NewGzipWriter: gzip.NewWriter,
		NewTarWriter:  func(w io.Writer) TarWriter { return tar.NewWriter(w) },
		FilepathRel:   filepath.Rel,
		ReadAll:       io.ReadAll,
		ParseReference: func(ref string, opts ...name.Option) (name.Reference, error) {
			return name.ParseReference(ref, opts...)
		},
		RemoteImage: func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return remote.Image(ref, options...)
		},
		ImageLayers:       func(img v1.Image) ([]v1.Layer, error) { return img.Layers() },
		LayerUncompressed: func(layer v1.Layer) (io.ReadCloser, error) { return layer.Uncompressed() },
		AppendLayers:      mutate.AppendLayers,
		ConfigFile:        mutate.ConfigFile,
		MediaType:         mutate.MediaType,
		ConfigMediaType:   mutate.ConfigMediaType,
		Annotations: func(img v1.Image, anns map[string]string) v1.Image {
			return mutate.Annotations(img, anns).(v1.Image)
		},
		EmptyImage:       func() v1.Image { return empty.Image },
		RemoteGet:        remote.Get,
		RemoteWriteLayer: remote.WriteLayer,
		RemoteWrite:      remote.Write,
		RemoteList:       remote.List,
		NewRepository: func(repo string, opts ...name.Option) (name.Repository, error) {
			return name.NewRepository(repo, opts...)
		},
		NewBytesReader: func(b []byte) *bytes.Reader { return bytes.NewReader(b) },
		NewTarReader:   func(r io.Reader) TarReader { return tar.NewReader(r) },
		Copy:           io.Copy,
		Chmod:          os.Chmod,
	}
}
