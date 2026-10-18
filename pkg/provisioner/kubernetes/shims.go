// The Shims is the set of replaceable time and conversion functions of the kubernetes package.
// It provides the clock, the sleep, and the unstructured converter that the manager uses.
// Tests replace a field to control time or to make a conversion fail.
// NewShims returns the standard library and apimachinery implementations.

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
