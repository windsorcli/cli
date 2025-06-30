package artifact

import "github.com/windsorcli/cli/pkg/di"

// The MockArtifact is a testing component that provides a mock implementation of the Artifact interface.
// It provides customizable function fields for testing different Artifact behaviors.
// The MockArtifact enables isolated testing of components that depend on the Artifact interface,
// allowing for controlled simulation of artifact operations in test scenarios.

// =============================================================================
// Types
// =============================================================================

// MockArtifact provides a mock implementation of the Artifact interface for testing.
type MockArtifact struct {
	InitializeFunc func(injector di.Injector) error
	AddFileFunc    func(path string, content []byte) error
	CreateFunc     func(outputPath string, tag string) (string, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockArtifact creates a new MockArtifact instance
func NewMockArtifact() *MockArtifact {
	return &MockArtifact{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockArtifact) Initialize(injector di.Injector) error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc(injector)
	}
	return nil
}

// AddFile calls the mock AddFileFunc if set, otherwise returns nil
func (m *MockArtifact) AddFile(path string, content []byte) error {
	if m.AddFileFunc != nil {
		return m.AddFileFunc(path, content)
	}
	return nil
}

// Create calls the mock CreateFunc if set, otherwise returns the outputPath and nil
func (m *MockArtifact) Create(outputPath string, tag string) (string, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(outputPath, tag)
	}
	return outputPath, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockArtifact implements Artifact interface
var _ Artifact = (*MockArtifact)(nil)
