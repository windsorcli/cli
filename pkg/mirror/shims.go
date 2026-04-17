package mirror

import (
	"io"
	"net/http"
	"os"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// The Shims provides mockable wrappers around system, network, and OCI registry
// operations used by the mirror package.
// It provides a dependency-injection seam for HTTP, OCI registry I/O, YAML parsing,
// and time functions so mirror logic can be exercised deterministically in unit tests.
// The default implementations call real system, network, and registry APIs; tests
// replace individual fields with in-memory fakes.

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and registry operations.
type Shims struct {
	HttpGet           func(url string) (*http.Response, error)
	ReadAll           func(r io.Reader) ([]byte, error)
	YamlUnmarshal     func(data []byte, v any) error
	Sleep             func(d time.Duration)
	Now               func() time.Time
	ParseReference    func(ref string, opts ...name.Option) (name.Reference, error)
	RemoteImage       func(ref name.Reference, opts ...remote.Option) (v1.Image, error)
	RemoteGet         func(ref name.Reference, opts ...remote.Option) (*remote.Descriptor, error)
	RemoteWrite       func(ref name.Reference, img v1.Image, opts ...remote.Option) error
	RemoteWriteLayer  func(repo name.Repository, layer v1.Layer, opts ...remote.Option) error
	ImageLayers       func(img v1.Image) ([]v1.Layer, error)
	LayerUncompressed func(layer v1.Layer) (io.ReadCloser, error)
	EmptyImage        func() v1.Image
	AppendLayers      func(base v1.Image, layers ...v1.Layer) (v1.Image, error)
	ConfigFile        func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error)
	MediaType         func(img v1.Image, mt types.MediaType) v1.Image
	ConfigMediaType   func(img v1.Image, mt types.MediaType) v1.Image
	Annotations       func(img v1.Image, anns map[string]string) v1.Image
	CraneCopy         func(src, dst string) error
	CraneManifest     func(ref string) ([]byte, error)
	MkdirAll          func(path string, perm os.FileMode) error
	WriteFile         func(name string, data []byte, perm os.FileMode) error
	Stat              func(name string) (os.FileInfo, error)
}

// =============================================================================
// Helpers
// =============================================================================

// defaultHTTPClient enforces a bounded overall timeout on every HTTP request
// issued by the mirror package. Without this, a slow or hanging upstream
// (e.g. a chart repository that neither serves nor 404s a .prov file) can
// stall the whole mirror run indefinitely.
var defaultHTTPClient = &http.Client{Timeout: 60 * time.Second}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims populated with real implementations for each hook.
func NewShims() *Shims {
	return &Shims{
		HttpGet:       defaultHTTPClient.Get,
		ReadAll:       io.ReadAll,
		YamlUnmarshal: yaml.Unmarshal,
		Sleep:         time.Sleep,
		Now:           time.Now,
		ParseReference: func(ref string, opts ...name.Option) (name.Reference, error) {
			return name.ParseReference(ref, opts...)
		},
		RemoteImage:       remote.Image,
		RemoteGet:         remote.Get,
		RemoteWrite:       remote.Write,
		RemoteWriteLayer:  remote.WriteLayer,
		ImageLayers:       func(img v1.Image) ([]v1.Layer, error) { return img.Layers() },
		LayerUncompressed: func(l v1.Layer) (io.ReadCloser, error) { return l.Uncompressed() },
		EmptyImage:        func() v1.Image { return empty.Image },
		AppendLayers:      mutate.AppendLayers,
		ConfigFile:        mutate.ConfigFile,
		MediaType:         mutate.MediaType,
		ConfigMediaType:   mutate.ConfigMediaType,
		Annotations: func(img v1.Image, anns map[string]string) v1.Image {
			return mutate.Annotations(img, anns).(v1.Image)
		},
		CraneCopy: func(src, dst string) error {
			return crane.Copy(src, dst,
				crane.WithAuthFromKeychain(authn.DefaultKeychain),
				crane.WithTransport(&http.Transport{
					Proxy:                 http.ProxyFromEnvironment,
					TLSHandshakeTimeout:   30 * time.Second,
					ResponseHeaderTimeout: 60 * time.Second,
					IdleConnTimeout:       90 * time.Second,
					ExpectContinueTimeout: 1 * time.Second,
				}),
			)
		},
		CraneManifest: func(ref string) ([]byte, error) {
			return crane.Manifest(ref,
				crane.WithAuthFromKeychain(authn.DefaultKeychain),
			)
		},
		MkdirAll:  os.MkdirAll,
		WriteFile: os.WriteFile,
		Stat:      os.Stat,
	}
}
