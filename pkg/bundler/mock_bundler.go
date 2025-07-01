package bundler

import (
	"github.com/windsorcli/cli/pkg/di"
)

// The MockBundler is a mock implementation of the Bundler interface for testing.
// It provides function fields that can be overridden to control behavior during tests.
// It serves as a test double for the Bundler interface in unit tests.
// It enables isolation and verification of component interactions with the bundler system.

// =============================================================================
// Types
// =============================================================================

// MockBundler is a mock implementation of the Bundler interface
type MockBundler struct {
	InitializeFunc func(injector di.Injector) error
	BundleFunc     func(artifact Artifact) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockBundler creates a new MockBundler instance
func NewMockBundler() *MockBundler {
	return &MockBundler{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockBundler) Initialize(injector di.Injector) error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc(injector)
	}
	return nil
}

// Bundle calls the mock BundleFunc if set, otherwise returns nil
func (m *MockBundler) Bundle(artifact Artifact) error {
	if m.BundleFunc != nil {
		return m.BundleFunc(artifact)
	}
	return nil
}

// Ensure MockBundler implements Bundler interface
var _ Bundler = (*MockBundler)(nil)
