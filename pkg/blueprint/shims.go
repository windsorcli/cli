package blueprint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/goccy/go-yaml"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	k8syaml "sigs.k8s.io/yaml"
)

// Shims provides testable wrappers around external dependencies for the blueprint package.
// This enables dependency injection and mocking in unit tests while maintaining
// clean separation between business logic and external system interactions.
type Shims struct {
	Stat               func(string) (os.FileInfo, error)
	ReadFile           func(string) ([]byte, error)
	ReadDir            func(string) ([]os.DirEntry, error)
	Walk               func(string, filepath.WalkFunc) error
	WriteFile          func(string, []byte, os.FileMode) error
	Remove             func(string) error
	MkdirAll           func(string, os.FileMode) error
	YamlMarshal        func(any) ([]byte, error)
	YamlUnmarshal      func([]byte, any) error
	YamlMarshalNonNull func(any) ([]byte, error)
	K8sYamlUnmarshal   func([]byte, any) error
	NewFakeClient      func(...client.Object) client.WithWatch
	RegexpMatchString  func(pattern, s string) (bool, error)
	TimeAfter          func(d time.Duration) <-chan time.Time
	NewTicker          func(d time.Duration) *time.Ticker
	TickerStop         func(*time.Ticker)
	JsonMarshal        func(any) ([]byte, error)
	JsonUnmarshal      func([]byte, any) error
	FilepathBase       func(string) string
}

// NewShims creates a new Shims instance with default implementations
// that delegate to the actual system functions and libraries.
func NewShims() *Shims {
	return &Shims{
		Stat:      os.Stat,
		ReadFile:  os.ReadFile,
		ReadDir:   os.ReadDir,
		Walk:      filepath.Walk,
		WriteFile: os.WriteFile,
		Remove:    os.Remove,
		MkdirAll:  os.MkdirAll,
		YamlMarshal: func(v any) ([]byte, error) {
			return yaml.Marshal(v)
		},
		YamlUnmarshal: func(data []byte, v any) error {
			return yaml.Unmarshal(data, v)
		},
		YamlMarshalNonNull: func(v any) ([]byte, error) {
			return yaml.MarshalWithOptions(v, yaml.WithComment(yaml.CommentMap{}))
		},
		K8sYamlUnmarshal: func(data []byte, v any) error {
			return k8syaml.Unmarshal(data, v)
		},
		NewFakeClient: func(objs ...client.Object) client.WithWatch {
			scheme := runtime.NewScheme()
			return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
		},
		RegexpMatchString: regexp.MatchString,
		TimeAfter:         time.After,
		NewTicker:         time.NewTicker,
		TickerStop: func(ticker *time.Ticker) {
			ticker.Stop()
		},
		JsonMarshal:   json.Marshal,
		JsonUnmarshal: json.Unmarshal,
		FilepathBase:  filepath.Base,
	}
}
