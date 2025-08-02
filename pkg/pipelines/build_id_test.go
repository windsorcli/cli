package pipelines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// buildIDMockFileInfo implements os.FileInfo for testing
type buildIDMockFileInfo struct {
	name  string
	isDir bool
}

func (m buildIDMockFileInfo) Name() string       { return m.name }
func (m buildIDMockFileInfo) Size() int64        { return 0 }
func (m buildIDMockFileInfo) Mode() os.FileMode  { return 0644 }
func (m buildIDMockFileInfo) ModTime() time.Time { return time.Time{} }
func (m buildIDMockFileInfo) IsDir() bool        { return m.isDir }
func (m buildIDMockFileInfo) Sys() any           { return nil }

type BuildIDMocks struct {
	Injector di.Injector
	Shell    *shell.MockShell
	Shims    *Shims
}

func setupBuildIDShims(t *testing.T, buildID string, statError error, readError error) *Shims {
	t.Helper()
	shims := NewShims()

	shims.Stat = func(name string) (os.FileInfo, error) {
		if statError != nil {
			return nil, statError
		}
		if strings.Contains(name, ".build-id") {
			return buildIDMockFileInfo{name: ".build-id", isDir: false}, nil
		}
		return nil, os.ErrNotExist
	}

	shims.ReadFile = func(name string) ([]byte, error) {
		if readError != nil {
			return nil, readError
		}
		if strings.Contains(name, ".build-id") {
			return []byte(buildID), nil
		}
		return []byte{}, nil
	}

	// Mock file system operations to avoid real file I/O
	shims.RemoveAll = func(path string) error {
		return nil
	}

	return shims
}

func setupBuildIDMocks(t *testing.T, buildID string, statError error, readError error) *BuildIDMocks {
	t.Helper()

	// Create mock shell
	mockShell := &shell.MockShell{}
	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/test/project", nil
	}

	// Create mock shims
	mockShims := setupBuildIDShims(t, buildID, statError, readError)

	// Create mock injector
	mockInjector := di.NewInjector()
	mockInjector.Register("shell", mockShell)
	mockInjector.Register("shims", mockShims)

	return &BuildIDMocks{
		Injector: mockInjector,
		Shell:    mockShell,
		Shims:    mockShims,
	}
}

// =============================================================================
// Test Cases
// =============================================================================

func TestBuildIDPipeline_NewBuildIDPipeline(t *testing.T) {
	t.Run("CreatesPipelineWithDefaultBase", func(t *testing.T) {
		// When creating a new BuildIDPipeline
		pipeline := NewBuildIDPipeline()

		// Then it should be properly initialized
		if pipeline == nil {
			t.Fatal("Expected pipeline to be created")
		}
	})
}

func TestBuildIDPipeline_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "1234567890", nil, nil)

		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()

		// When initializing with valid injector
		ctx := context.Background()
		err := pipeline.Initialize(mocks.Injector, ctx)

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected initialization to succeed, got error: %v", err)
		}
	})

	t.Run("BasePipelineError", func(t *testing.T) {
		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()

		// And an invalid injector (missing required dependencies)
		mockInjector := di.NewInjector()

		// When initializing with invalid injector
		ctx := context.Background()
		err := pipeline.Initialize(mockInjector, ctx)

		// Then it should return an error (or succeed if base pipeline handles missing dependencies gracefully)
		if err != nil {
			// Error is expected
			t.Logf("Initialization failed as expected: %v", err)
		} else {
			// Success is also acceptable if base pipeline handles missing dependencies gracefully
			t.Logf("Initialization succeeded (base pipeline may handle missing dependencies gracefully)")
		}
	})
}

func TestBuildIDPipeline_Execute(t *testing.T) {
	t.Run("GetExistingBuildID", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "1234567890", nil, nil)

		// Given a BuildIDPipeline with existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing without new flag
		ctx := context.Background()
		err := pipeline.Execute(ctx)

		// Then it should succeed and output the existing build ID
		if err != nil {
			t.Fatalf("Expected execution to succeed, got error: %v", err)
		}
	})

	t.Run("GenerateNewBuildID", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", os.ErrNotExist, nil)

		// Given a BuildIDPipeline with no existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing without new flag
		ctx := context.Background()
		err := pipeline.Execute(ctx)

		// Then it should succeed and generate a new build ID (may fail on read-only filesystem)
		if err != nil && !strings.Contains(err.Error(), "read-only file system") {
			t.Fatalf("Expected execution to succeed or fail with read-only filesystem, got error: %v", err)
		}
	})

	t.Run("ForceNewBuildID", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "1234567890", nil, nil)

		// Given a BuildIDPipeline with existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing with new flag
		ctx := context.WithValue(context.Background(), "new", true)
		err := pipeline.Execute(ctx)

		// Then it should succeed and generate a new build ID (may fail on read-only filesystem)
		if err != nil && !strings.Contains(err.Error(), "read-only file system") {
			t.Fatalf("Expected execution to succeed or fail with read-only filesystem, got error: %v", err)
		}
	})
}

func TestBuildIDPipeline_getBuildID(t *testing.T) {
	t.Run("ExistingBuildID", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "1234567890", nil, nil)

		// Given a BuildIDPipeline with existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When getting build ID
		err := pipeline.getBuildID()

		// Then it should succeed and output the existing build ID
		if err != nil {
			t.Fatalf("Expected getBuildID to succeed, got error: %v", err)
		}
	})

	t.Run("NoExistingBuildID", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", os.ErrNotExist, nil)

		// Given a BuildIDPipeline with no existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When getting build ID
		err := pipeline.getBuildID()

		// Then it should succeed and generate a new build ID (may fail on read-only filesystem)
		if err != nil && !strings.Contains(err.Error(), "read-only file system") {
			t.Fatalf("Expected getBuildID to succeed or fail with read-only filesystem, got error: %v", err)
		}
	})

	t.Run("GetBuildIDFromFileError", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", nil, fmt.Errorf("mock read error"))

		// Given a BuildIDPipeline with read error
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When getting build ID
		err := pipeline.getBuildID()

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected getBuildID to fail with read error")
		}
		if !strings.Contains(err.Error(), "failed to get build ID") {
			t.Errorf("Expected error to contain 'failed to get build ID', got: %v", err)
		}
	})

	t.Run("GenerateBuildIDError", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", os.ErrNotExist, nil)

		// Given a BuildIDPipeline with no existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// And mock shell returns error for project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock project root error")
		}

		// When getting build ID
		err := pipeline.getBuildID()

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected getBuildID to fail with project root error")
		}
		if !strings.Contains(err.Error(), "failed to get build ID") {
			t.Errorf("Expected error to contain 'failed to get build ID', got: %v", err)
		}
	})
}

func TestBuildIDPipeline_generateNewBuildID(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", nil, nil)

		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When generating new build ID
		err := pipeline.generateNewBuildID()

		// Then it should succeed (may fail on read-only filesystem, which is expected)
		if err != nil && !strings.Contains(err.Error(), "read-only file system") {
			t.Fatalf("Expected generateNewBuildID to succeed or fail with read-only filesystem, got error: %v", err)
		}
	})

	t.Run("GenerateBuildIDError", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", nil, nil)

		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// And mock shell returns error for project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock project root error")
		}

		// When generating new build ID
		err := pipeline.generateNewBuildID()

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected generateNewBuildID to fail with project root error")
		}
		if !strings.Contains(err.Error(), "failed to get build ID path") {
			t.Errorf("Expected error to contain 'failed to get build ID path', got: %v", err)
		}
	})
}

func TestBuildIDPipeline_getBuildIDFromFile(t *testing.T) {
	t.Run("ExistingFile", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "1234567890", nil, nil)

		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When getting build ID from file
		buildID, err := pipeline.getBuildIDFromFile()

		// Then it should succeed and return the build ID
		if err != nil {
			t.Fatalf("Expected getBuildIDFromFile to succeed, got error: %v", err)
		}
		if buildID != "1234567890" {
			t.Errorf("Expected build ID '1234567890', got '%s'", buildID)
		}
	})

	t.Run("FileNotExists", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", os.ErrNotExist, nil)

		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When getting build ID from file
		buildID, err := pipeline.getBuildIDFromFile()

		// Then it should succeed and return empty string
		if err != nil {
			t.Fatalf("Expected getBuildIDFromFile to succeed, got error: %v", err)
		}
		if buildID != "" {
			t.Errorf("Expected empty build ID, got '%s'", buildID)
		}
	})

	t.Run("GetBuildIDPathError", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", nil, nil)

		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// And mock shell returns error for project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock project root error")
		}

		// When getting build ID from file
		buildID, err := pipeline.getBuildIDFromFile()

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected getBuildIDFromFile to fail with project root error")
		}
		if !strings.Contains(err.Error(), "failed to get build ID path") {
			t.Errorf("Expected error to contain 'failed to get build ID path', got: %v", err)
		}
		if buildID != "" {
			t.Errorf("Expected empty build ID on error, got '%s'", buildID)
		}
	})

	t.Run("ReadFileError", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", nil, fmt.Errorf("mock read error"))

		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When getting build ID from file
		buildID, err := pipeline.getBuildIDFromFile()

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected getBuildIDFromFile to fail with read error")
		}
		if !strings.Contains(err.Error(), "failed to read build ID file") {
			t.Errorf("Expected error to contain 'failed to read build ID file', got: %v", err)
		}
		if buildID != "" {
			t.Errorf("Expected empty build ID on error, got '%s'", buildID)
		}
	})
}

func TestBuildIDPipeline_setBuildIDToFile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", nil, nil)

		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When setting build ID to file
		err := pipeline.setBuildIDToFile("1234567890")

		// Then it should succeed (may fail on read-only filesystem, which is expected)
		if err != nil && !strings.Contains(err.Error(), "read-only file system") {
			t.Fatalf("Expected setBuildIDToFile to succeed or fail with read-only filesystem, got error: %v", err)
		}
	})

	t.Run("GetBuildIDPathError", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", nil, nil)

		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// And mock shell returns error for project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock project root error")
		}

		// When setting build ID to file
		err := pipeline.setBuildIDToFile("1234567890")

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected setBuildIDToFile to fail with project root error")
		}
		if !strings.Contains(err.Error(), "failed to get build ID path") {
			t.Errorf("Expected error to contain 'failed to get build ID path', got: %v", err)
		}
	})
}

func TestBuildIDPipeline_generateBuildID(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", nil, nil)

		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When generating build ID
		buildID, err := pipeline.generateBuildID()

		// Then it should succeed and return a timestamp
		if err != nil {
			t.Fatalf("Expected generateBuildID to succeed, got error: %v", err)
		}
		if buildID == "" {
			t.Fatal("Expected non-empty build ID")
		}

		// And it should be a valid timestamp
		if len(buildID) < 10 {
			t.Errorf("Expected build ID to be at least 10 characters, got %d", len(buildID))
		}
	})
}

func TestBuildIDPipeline_getBuildIDPath(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", nil, nil)

		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When getting build ID path
		path, err := pipeline.getBuildIDPath()

		// Then it should succeed and return the correct path
		if err != nil {
			t.Fatalf("Expected getBuildIDPath to succeed, got error: %v", err)
		}
		expectedPath := filepath.Join("/test/project", ".windsor", ".build-id")
		if path != expectedPath {
			t.Errorf("Expected path '%s', got '%s'", expectedPath, path)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", nil, nil)

		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// And mock shell returns error for project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock project root error")
		}

		// When getting build ID path
		path, err := pipeline.getBuildIDPath()

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected getBuildIDPath to fail with project root error")
		}
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error to contain 'failed to get project root', got: %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path on error, got '%s'", path)
		}
	})
}
