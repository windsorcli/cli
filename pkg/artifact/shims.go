package artifact

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
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
	Stat              func(name string) (os.FileInfo, error)
	Create            func(name string) (io.WriteCloser, error)
	ReadFile          func(name string) ([]byte, error)
	Walk              func(root string, walkFn filepath.WalkFunc) error
	NewGzipWriter     func(w io.Writer) *gzip.Writer
	NewTarWriter      func(w io.Writer) TarWriter
	YamlUnmarshal     func(data []byte, v any) error
	FilepathRel       func(basepath, targpath string) (string, error)
	YamlMarshal       func(data any) ([]byte, error)
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
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		Stat: os.Stat,
		// #nosec G304 - User-controlled output path is intentional for build artifact creation
		Create:            func(name string) (io.WriteCloser, error) { return os.Create(name) },
		ReadFile:          os.ReadFile,
		Walk:              filepath.Walk,
		NewGzipWriter:     gzip.NewWriter,
		NewTarWriter:      func(w io.Writer) TarWriter { return tar.NewWriter(w) },
		YamlUnmarshal:     yaml.Unmarshal,
		FilepathRel:       filepath.Rel,
		YamlMarshal:       yaml.Marshal,
		ReadAll:           io.ReadAll,
		ParseReference:    func(ref string, opts ...name.Option) (name.Reference, error) { return name.ParseReference(ref) },
		RemoteImage:       func(ref name.Reference, options ...remote.Option) (v1.Image, error) { return remote.Image(ref) },
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
	}
}
