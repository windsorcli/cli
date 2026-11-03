package runtime_test

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRuntime_GetBuildID(t *testing.T) {
	t.Run("ReturnsEmptyStringWhenNoBuildIDExists", func(t *testing.T) {
		// Given a runtime with no existing build ID
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		// When GetBuildID is called
		buildID, err := rt.GetBuildID()

		// Then an empty string should be returned (read-only behavior)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if buildID != "" {
			t.Errorf("Expected empty string when no build ID exists, got: %s", buildID)
		}
	})

	t.Run("ReturnsExistingBuildID", func(t *testing.T) {
		// Given a runtime with an existing build ID
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		buildID1, err := rt.GetBuildID()
		if err != nil {
			t.Fatalf("Failed to get initial build ID: %v", err)
		}

		// When GetBuildID is called again
		buildID2, err := rt.GetBuildID()

		// Then the same build ID should be returned
		if err != nil {
			t.Fatalf("Failed to get second build ID: %v", err)
		}

		if buildID1 != buildID2 {
			t.Errorf("Expected build IDs to match, got %s and %s", buildID1, buildID2)
		}
	})

	t.Run("ErrorWhenMkdirAllFails", func(t *testing.T) {
		// Given a runtime whose project root cannot be created
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.Shims.MkdirAll = func(string, os.FileMode) error {
			return fmt.Errorf("mkdir failed")
		}

		// When GetBuildID is called
		_, err := rt.GetBuildID()

		// Then an error about the project root directory is returned
		if err == nil || !strings.Contains(err.Error(), "failed to create project root directory") {
			t.Errorf("Expected project root directory error, got: %v", err)
		}
	})

	t.Run("ErrorWhenOpenRootFails", func(t *testing.T) {
		// Given a runtime whose project root cannot be opened
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ProjectRoot = t.TempDir()
		rt.Shims.OpenRoot = func(string) (*os.Root, error) {
			return nil, fmt.Errorf("open root failed")
		}

		// When GetBuildID is called
		_, err := rt.GetBuildID()

		// Then an error about opening the project root is returned
		if err == nil || !strings.Contains(err.Error(), "failed to open project root") {
			t.Errorf("Expected open project root error, got: %v", err)
		}
	})

	t.Run("ErrorWhenReadFileFails", func(t *testing.T) {
		if goruntime.GOOS == "windows" {
			t.Skip("Skipping on Windows: os.Chmod with 0000 does not prevent file operations")
		}

		// Given a runtime with a build ID file that cannot be read
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		buildIDDir := filepath.Join(tmpDir, ".windsor")
		if err := os.MkdirAll(buildIDDir, 0750); err != nil {
			t.Fatalf("Failed to create build ID directory: %v", err)
		}

		buildIDFile := filepath.Join(buildIDDir, ".build-id")
		if err := os.WriteFile(buildIDFile, []byte("test-build-id"), 0600); err != nil {
			t.Fatalf("Failed to write build ID file: %v", err)
		}

		if err := os.Chmod(buildIDDir, 0000); err != nil {
			t.Fatalf("Failed to set directory permissions: %v", err)
		}
		defer os.Chmod(buildIDDir, 0750)

		// When GetBuildID is called
		_, err := rt.GetBuildID()

		// Then an error should be returned

		if err == nil {
			t.Fatal("Expected error when ReadFile fails")
		}

		if !strings.Contains(err.Error(), "failed to read build ID file") {
			t.Errorf("Expected error about reading build ID file, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "read permission") {
			t.Errorf("Expected a remediation about read permission, got: %v", we)
		}
	})

	t.Run("ErrorWhenWriteBuildIDToFileFails", func(t *testing.T) {
		if goruntime.GOOS == "windows" {
			t.Skip("Skipping on Windows: os.Chmod with 0000 does not prevent file operations")
		}

		// Given a runtime with a build ID directory that cannot be written to
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		buildIDDir := filepath.Join(tmpDir, ".windsor")
		if err := os.MkdirAll(buildIDDir, 0750); err != nil {
			t.Fatalf("Failed to create build ID directory: %v", err)
		}

		if err := os.Chmod(buildIDDir, 0000); err != nil {
			t.Fatalf("Failed to set directory permissions: %v", err)
		}
		defer os.Chmod(buildIDDir, 0750)

		// When GetBuildID is called
		_, err := rt.GetBuildID()

		// Then an error should be returned

		if err == nil {
			t.Fatal("Expected error when writeBuildIDToFile fails")
		}

		if !strings.Contains(err.Error(), "failed to set build ID") && !strings.Contains(err.Error(), "failed to read build ID file") {
			t.Errorf("Expected error about setting or reading build ID, got: %v", err)
		}
	})
}
