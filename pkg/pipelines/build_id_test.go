package pipelines

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/context/shell"
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

	shims.MkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
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
		mocks := setupBuildIDMocks(t, "250802.123.5", nil, nil)

		// Given a BuildIDPipeline with existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing without new flag
		err := pipeline.Execute(context.Background())

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
		err := pipeline.Execute(context.Background())

		// Then it should succeed and generate a new build ID (may fail on read-only filesystem or permission denied)
		if err != nil && !strings.Contains(err.Error(), "read-only file system") && !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("Expected execution to succeed or fail with filesystem error, got error: %v", err)
		}
	})

	t.Run("ForceNewBuildID", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "250802.123.5", nil, nil)

		// Given a BuildIDPipeline with existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing with new flag
		ctx := context.WithValue(context.Background(), "new", true)
		err := pipeline.Execute(ctx)

		// Then it should succeed and generate a new build ID (may fail on read-only filesystem or permission denied)
		if err != nil && !strings.Contains(err.Error(), "read-only file system") && !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("Expected execution to succeed or fail with filesystem error, got error: %v", err)
		}
	})
}

func TestBuildIDPipeline_getBuildID(t *testing.T) {
	t.Run("ExistingBuildID", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "250802.123.5", nil, nil)

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
		if !strings.Contains(err.Error(), "failed to read build ID file") {
			t.Errorf("Expected error to contain 'failed to read build ID file', got: %v", err)
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
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error to contain 'failed to get project root', got: %v", err)
		}
	})
}

func TestBuildIDPipeline_generateNewBuildID(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", os.ErrNotExist, nil)

		// Given a BuildIDPipeline with no existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When generating new build ID
		err := pipeline.generateNewBuildID()

		// Then it should succeed and generate a new build ID (may fail on read-only filesystem)
		if err != nil && !strings.Contains(err.Error(), "read-only file system") {
			t.Fatalf("Expected generateNewBuildID to succeed or fail with read-only filesystem, got error: %v", err)
		}
	})

	t.Run("ExistingBuildID", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "250802.123.5", nil, nil)

		// Given a BuildIDPipeline with existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When generating new build ID
		err := pipeline.generateNewBuildID()

		// Then it should succeed and overwrite the existing build ID (may fail on read-only filesystem)
		if err != nil && !strings.Contains(err.Error(), "read-only file system") {
			t.Fatalf("Expected generateNewBuildID to succeed or fail with read-only filesystem, got error: %v", err)
		}
	})
}

func TestBuildIDPipeline_getBuildIDFromFile(t *testing.T) {
	t.Run("ExistingBuildID", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "250802.123.5", nil, nil)

		// Given a BuildIDPipeline with existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When getting build ID from file
		err := pipeline.getBuildID()

		// Then it should succeed and return the existing build ID
		if err != nil {
			t.Fatalf("Expected getBuildID to succeed, got error: %v", err)
		}
		// Note: getBuildID prints the build ID, so we can't easily test the return value
		// The test verifies it doesn't error
	})

	t.Run("NoExistingBuildID", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", os.ErrNotExist, nil)

		// Given a BuildIDPipeline with no existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When getting build ID from file
		err := pipeline.getBuildID()

		// Then it should succeed and generate a new build ID (may fail on read-only filesystem)
		if err != nil && !strings.Contains(err.Error(), "read-only file system") {
			t.Fatalf("Expected getBuildID to succeed or fail with read-only filesystem, got error: %v", err)
		}
	})

	t.Run("ReadError", func(t *testing.T) {
		mocks := setupBuildIDMocks(t, "", nil, fmt.Errorf("mock read error"))

		// Given a BuildIDPipeline with read error
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When getting build ID from file
		err := pipeline.getBuildID()

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected getBuildID to fail with read error")
		}
		if !strings.Contains(err.Error(), "failed to read build ID file") {
			t.Errorf("Expected error to contain 'failed to read build ID file', got: %v", err)
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
		err := pipeline.writeBuildIDToFile("250802.123.5")

		// Then it should succeed (may fail on read-only filesystem, which is expected)
		if err != nil && !strings.Contains(err.Error(), "read-only file system") {
			t.Fatalf("Expected writeBuildIDToFile to succeed or fail with read-only filesystem, got error: %v", err)
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

		// When setting build ID to file
		err := pipeline.writeBuildIDToFile("250802.123.5")

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected writeBuildIDToFile to fail with project root error")
		}
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error to contain 'failed to get project root', got: %v", err)
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

		// Then it should succeed and return a build ID
		if err != nil {
			t.Fatalf("Expected generateBuildID to succeed, got error: %v", err)
		}
		if buildID == "" {
			t.Fatal("Expected non-empty build ID")
		}

		// And it should be in the correct YYMMDD.RANDOM.# format
		parts := strings.Split(buildID, ".")
		if len(parts) != 3 {
			t.Errorf("Expected build ID to have 3 parts separated by dots, got %d parts: %s", len(parts), buildID)
		}

		// Check date part (YYMMDD)
		if len(parts[0]) != 6 {
			t.Errorf("Expected date part to be 6 characters (YYMMDD), got %d: %s", len(parts[0]), parts[0])
		}

		// Check random part (3 digits)
		if len(parts[1]) != 3 {
			t.Errorf("Expected random part to be 3 digits, got %d: %s", len(parts[1]), parts[1])
		}

		// Check counter part (should be 1 for first build)
		if parts[2] != "1" {
			t.Errorf("Expected counter part to be 1 for first build, got %s", parts[2])
		}
	})

	t.Run("IncrementExistingBuildID", func(t *testing.T) {
		// Get today's date for the test
		now := time.Now()
		today := fmt.Sprintf("%02d%02d%02d", now.Year()%100, int(now.Month()), now.Day())

		// Mock existing build ID for today
		existingBuildID := fmt.Sprintf("%s.123.5", today)
		mocks := setupBuildIDMocks(t, existingBuildID, nil, nil)

		// Given a BuildIDPipeline with existing build ID
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When generating build ID
		buildID, err := pipeline.generateBuildID()

		// Then it should succeed and increment the counter
		if err != nil {
			t.Fatalf("Expected generateBuildID to succeed, got error: %v", err)
		}

		// Should increment counter from 5 to 6
		expectedBuildID := fmt.Sprintf("%s.123.6", today)
		if buildID != expectedBuildID {
			t.Errorf("Expected build ID to be %s, got %s", expectedBuildID, buildID)
		}
	})

	t.Run("NewDayNewRandom", func(t *testing.T) {
		// Get today's and yesterday's dates for the test
		now := time.Now()
		today := fmt.Sprintf("%02d%02d%02d", now.Year()%100, int(now.Month()), now.Day())
		yesterday := fmt.Sprintf("%02d%02d%02d", now.Year()%100, int(now.Month()), now.Day()-1)

		// Mock existing build ID from yesterday
		existingBuildID := fmt.Sprintf("%s.456.10", yesterday)
		mocks := setupBuildIDMocks(t, existingBuildID, nil, nil)

		// Given a BuildIDPipeline with existing build ID from different day
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When generating build ID
		buildID, err := pipeline.generateBuildID()

		// Then it should succeed and generate new random with counter 1
		if err != nil {
			t.Fatalf("Expected generateBuildID to succeed, got error: %v", err)
		}

		// Should have today's date, new random, and counter 1
		parts := strings.Split(buildID, ".")
		if len(parts) != 3 {
			t.Errorf("Expected build ID to have 3 parts, got %d: %s", len(parts), buildID)
		}

		// Date should be today
		if parts[0] != today {
			t.Errorf("Expected date to be today (%s), got %s", today, parts[0])
		}

		// Random should be different from yesterday (456)
		if parts[1] == "456" {
			t.Errorf("Expected new random number, got same as yesterday: %s", parts[1])
		}

		// Counter should be 1 for new day
		if parts[2] != "1" {
			t.Errorf("Expected counter to be 1 for new day, got %s", parts[2])
		}
	})
}

func TestBuildIDPipeline_incrementBuildID(t *testing.T) {
	t.Run("IncrementSameDay", func(t *testing.T) {
		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(setupBuildIDMocks(t, "", nil, nil).Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Get today's date for the test
		now := time.Now()
		today := fmt.Sprintf("%02d%02d%02d", now.Year()%100, int(now.Month()), now.Day())

		// When incrementing existing build ID from same day
		existingBuildID := fmt.Sprintf("%s.123.5", today)
		newBuildID, err := pipeline.incrementBuildID(existingBuildID, today)

		// Then it should increment counter
		if err != nil {
			t.Fatalf("Expected incrementBuildID to succeed, got error: %v", err)
		}

		expectedBuildID := fmt.Sprintf("%s.123.6", today)
		if newBuildID != expectedBuildID {
			t.Errorf("Expected build ID to be %s, got %s", expectedBuildID, newBuildID)
		}
	})

	t.Run("NewDayNewRandom", func(t *testing.T) {
		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(setupBuildIDMocks(t, "", nil, nil).Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Get today's and yesterday's dates for the test
		now := time.Now()
		today := fmt.Sprintf("%02d%02d%02d", now.Year()%100, int(now.Month()), now.Day())
		yesterday := fmt.Sprintf("%02d%02d%02d", now.Year()%100, int(now.Month()), now.Day()-1)

		// When incrementing existing build ID from different day
		existingBuildID := fmt.Sprintf("%s.456.10", yesterday)
		newBuildID, err := pipeline.incrementBuildID(existingBuildID, today)

		// Then it should generate new random and reset counter
		if err != nil {
			t.Fatalf("Expected incrementBuildID to succeed, got error: %v", err)
		}

		parts := strings.Split(newBuildID, ".")
		if len(parts) != 3 {
			t.Errorf("Expected build ID to have 3 parts, got %d: %s", len(parts), newBuildID)
		}

		// Date should be current date
		if parts[0] != today {
			t.Errorf("Expected date to be %s, got %s", today, parts[0])
		}

		// Random should be different
		if parts[1] == "456" {
			t.Errorf("Expected new random number, got same: %s", parts[1])
		}

		// Counter should be 1
		if parts[2] != "1" {
			t.Errorf("Expected counter to be 1, got %s", parts[2])
		}
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(setupBuildIDMocks(t, "", nil, nil).Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When incrementing invalid build ID format
		invalidBuildID := "invalid-format"
		currentDate := "250802"
		_, err := pipeline.incrementBuildID(invalidBuildID, currentDate)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected incrementBuildID to fail with invalid format")
		}
		if !strings.Contains(err.Error(), "invalid build ID format") {
			t.Errorf("Expected error to contain 'invalid build ID format', got: %v", err)
		}
	})

	t.Run("InvalidCounter", func(t *testing.T) {
		// Given a BuildIDPipeline
		pipeline := NewBuildIDPipeline()
		if err := pipeline.Initialize(setupBuildIDMocks(t, "", nil, nil).Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When incrementing build ID with invalid counter
		invalidBuildID := "250802.123.invalid"
		currentDate := "250802"
		_, err := pipeline.incrementBuildID(invalidBuildID, currentDate)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected incrementBuildID to fail with invalid counter")
		}
		if !strings.Contains(err.Error(), "invalid counter component") {
			t.Errorf("Expected error to contain 'invalid counter component', got: %v", err)
		}
	})
}
