// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"regexp"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

// =============================================================================
// Types
// =============================================================================

// Shims provides testable interfaces for external dependencies
type Shims struct {
	// Other operations
	RegexpMatchString func(pattern string, s string) (bool, error)
	YamlMarshal       func(v any) ([]byte, error)
	YamlUnmarshal     func(data []byte, v any, opts ...yaml.JSONOpt) error
	K8sYamlUnmarshal  func(data []byte, v any, opts ...yaml.JSONOpt) error
	TimeSleep         func(d time.Duration)
	ToUnstructured    func(obj any) (map[string]any, error)
	FromUnstructured  func(obj map[string]any, target any) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	shims := &Shims{
		RegexpMatchString: regexp.MatchString,
		YamlMarshal:       yaml.Marshal,
		YamlUnmarshal:     yaml.Unmarshal,
		K8sYamlUnmarshal:  yaml.Unmarshal,
		TimeSleep:         time.Sleep,
		ToUnstructured: func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		},
		FromUnstructured: func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		},
	}

	return shims
}
