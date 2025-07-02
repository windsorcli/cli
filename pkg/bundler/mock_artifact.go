package bundler

import (
	"github.com/windsorcli/cli/pkg/di"
)

// The MockArtifact is a mock implementation of the Artifact interface for testing.
// It provides function fields that can be overridden to control behavior during tests.
// It serves as a test double for the Artifact interface in unit tests.
// It enables isolation and verification of component interactions with the artifact system.

// =============================================================================
// Types
// =============================================================================

// MockArtifact is a mock implementation of the Artifact interface
type MockArtifact struct {
	InitializeFunc func(injector di.Injector) error
	AddFileFunc    func(path string, content []byte) error
	CreateFunc     func(outputPath string, tag string) (string, error)
	PushFunc       func(registryBase string, repoName string, tag string) error
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

// Create calls the mock CreateFunc if set, otherwise returns empty string and nil error
func (m *MockArtifact) Create(outputPath string, tag string) (string, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(outputPath, tag)
	}
	return "", nil
}

// Push calls the mock PushFunc if set, otherwise returns nil
func (m *MockArtifact) Push(registryBase string, repoName string, tag string) error {
	if m.PushFunc != nil {
		return m.PushFunc(registryBase, repoName, tag)
	}
	return nil
}

// Ensure MockArtifact implements Artifact interface
var _ Artifact = (*MockArtifact)(nil)
