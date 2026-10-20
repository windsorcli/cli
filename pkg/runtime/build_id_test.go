package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
		if runtime.GOOS == "windows" {
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
		if runtime.GOOS == "windows" {
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

func TestRuntime_GenerateBuildID(t *testing.T) {
	t.Run("ErrorWhenOpenRootFails", func(t *testing.T) {
		// Given a runtime whose project root cannot be opened
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ProjectRoot = t.TempDir()
		rt.Shims.OpenRoot = func(string) (*os.Root, error) {
			return nil, fmt.Errorf("open root failed")
		}

		// When GenerateBuildID is called
		_, err := rt.GenerateBuildID()

		// Then an error about generating the build ID is returned
		if err == nil || !strings.Contains(err.Error(), "failed to generate build ID") {
			t.Errorf("Expected generate build ID error, got: %v", err)
		}
	})

	t.Run("GeneratesAndSavesBuildID", func(t *testing.T) {
		// Given a runtime with no existing build ID
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		// When GenerateBuildID is called
		buildID, err := rt.GenerateBuildID()

		// Then a new build ID should be generated and saved

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if buildID == "" {
			t.Error("Expected build ID to be generated")
		}
	})

	t.Run("IncrementsBuildIDOnSubsequentCalls", func(t *testing.T) {
		// Given a runtime with an existing build ID
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		buildID1, err := rt.GenerateBuildID()
		if err != nil {
			t.Fatalf("Failed to generate first build ID: %v", err)
		}

		// When GenerateBuildID is called again
		buildID2, err := rt.GenerateBuildID()

		// Then the build ID should be incremented
		if err != nil {
			t.Fatalf("Failed to generate second build ID: %v", err)
		}

		if buildID1 == buildID2 {
			t.Error("Expected build IDs to be different")
		}
	})

	t.Run("ErrorOnInvalidFormat", func(t *testing.T) {
		// Given a runtime with an invalid build ID format
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		// When incrementBuildID is called with invalid format
		buildID, err := rt.incrementBuildID("invalid", "251112")

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error for invalid format")
		}

		if buildID != "" {
			t.Error("Expected empty build ID on error")
		}
	})

	t.Run("ErrorOnInvalidCounter", func(t *testing.T) {
		// Given a runtime with an invalid build ID counter
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		// When incrementBuildID is called with invalid counter
		buildID, err := rt.incrementBuildID("251112.123.abc", "251112")

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error for invalid counter")
		}

		if buildID != "" {
			t.Error("Expected empty build ID on error")
		}
	})

	t.Run("ResetsCounterOnDateChange", func(t *testing.T) {
		// Given a runtime with a build ID from a previous date
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		oldDate := "251111"
		newDate := "251112"
		existingBuildID := fmt.Sprintf("%s.123.5", oldDate)

		// When incrementBuildID is called with a new date
		buildID, err := rt.incrementBuildID(existingBuildID, newDate)

		// Then the counter should be reset

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !strings.HasPrefix(buildID, newDate) {
			t.Errorf("Expected build ID to start with new date %s, got: %s", newDate, buildID)
		}

		if !strings.HasSuffix(buildID, ".1") {
			t.Errorf("Expected build ID to end with .1, got: %s", buildID)
		}
	})

	t.Run("IncrementsCounterOnSameDate", func(t *testing.T) {
		// Given a runtime with a build ID from the same date
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		date := "251112"
		existingBuildID := fmt.Sprintf("%s.123.5", date)

		// When incrementBuildID is called with the same date
		buildID, err := rt.incrementBuildID(existingBuildID, date)

		// Then the counter should be incremented

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !strings.HasPrefix(buildID, date) {
			t.Errorf("Expected build ID to start with date %s, got: %s", date, buildID)
		}

		if !strings.Contains(buildID, ".123.6") {
			t.Errorf("Expected build ID to contain incremented counter .123.6, got: %s", buildID)
		}
	})

	t.Run("ErrorWhenBuildIDFileIsCorrupt", func(t *testing.T) {
		// Given a build ID file that does not parse
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ProjectRoot = t.TempDir()
		if err := os.MkdirAll(filepath.Join(rt.ProjectRoot, ".windsor"), 0750); err != nil {
			t.Fatalf("Failed to create build ID directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(rt.ProjectRoot, ".windsor", ".build-id"), []byte("garbage"), 0600); err != nil {
			t.Fatalf("Failed to write build ID file: %v", err)
		}

		// When GenerateBuildID is called
		_, err := rt.GenerateBuildID()

		// Then an error with the corrupt build ID hint is returned
		if err == nil || !strings.Contains(err.Error(), "invalid build ID format") {
			t.Fatalf("Expected invalid build ID format error, got: %v", err)
		}
		if we := werror.Find(err); we == nil || we.Remediation != corruptBuildIDHint {
			t.Errorf("Expected the corrupt build ID hint, got: %v", we)
		}
	})

	t.Run("ErrorWhenBuildIDFileCannotBeWritten", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping on Windows: os.Chmod does not prevent file writes")
		}

		// Given a read-only build ID directory with no build ID file
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ProjectRoot = t.TempDir()
		buildIDDir := filepath.Join(rt.ProjectRoot, ".windsor")
		if err := os.MkdirAll(buildIDDir, 0750); err != nil {
			t.Fatalf("Failed to create build ID directory: %v", err)
		}
		if err := os.Chmod(buildIDDir, 0500); err != nil {
			t.Fatalf("Failed to set directory permissions: %v", err)
		}
		defer os.Chmod(buildIDDir, 0750)

		// When GenerateBuildID is called
		_, err := rt.GenerateBuildID()

		// Then an error with a write permission hint is returned
		if err == nil || !strings.Contains(err.Error(), "failed to write build ID file") {
			t.Fatalf("Expected write build ID file error, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "write permission") {
			t.Errorf("Expected a remediation about write permission, got: %v", we)
		}
	})

	t.Run("ErrorWhenWriteBuildIDToFileFails", func(t *testing.T) {
		if runtime.GOOS == "windows" {
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

		// When GenerateBuildID is called
		_, err := rt.GenerateBuildID()

		// Then an error should be returned

		if err == nil {
			t.Fatal("Expected error when writeBuildIDToFile fails")
		}

		if !strings.Contains(err.Error(), "failed to set build ID") && !strings.Contains(err.Error(), "failed to read build ID file") {
			t.Errorf("Expected error about setting or reading build ID, got: %v", err)
		}
	})
}
