// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"regexp"
	"time"

	"sigs.k8s.io/yaml"
)

// =============================================================================
// Types
// =============================================================================

// Shims provides testable interfaces for external dependencies
type Shims struct {
	RegexpMatchString func(pattern string, s string) (bool, error)
	YamlMarshal       func(v interface{}) ([]byte, error)
	YamlUnmarshal     func(data []byte, v interface{}, opts ...yaml.JSONOpt) error
	K8sYamlUnmarshal  func(data []byte, v interface{}, opts ...yaml.JSONOpt) error
	TimeSleep         func(d time.Duration)
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		RegexpMatchString: regexp.MatchString,
		YamlMarshal:       yaml.Marshal,
		YamlUnmarshal:     yaml.Unmarshal,
		K8sYamlUnmarshal:  yaml.Unmarshal,
		TimeSleep:         time.Sleep,
	}
}
