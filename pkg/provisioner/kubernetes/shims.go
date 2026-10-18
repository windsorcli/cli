// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime"
)

// =============================================================================
// Types
// =============================================================================

// Shims provides testable interfaces for external dependencies
type Shims struct {
	TimeSleep        func(d time.Duration)
	TimeNow          func() time.Time
	ToUnstructured   func(obj any) (map[string]any, error)
	FromUnstructured func(obj map[string]any, target any) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	shims := &Shims{
		TimeSleep: time.Sleep,
		TimeNow:   time.Now,
		ToUnstructured: func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		},
		FromUnstructured: func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		},
	}

	return shims
}
