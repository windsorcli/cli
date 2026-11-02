package terraform_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestTerraformProvider_ListLocalStateComponentIDs(t *testing.T) {
	t.Run("ReturnsComponentIDsForEveryLocalStateFile", func(t *testing.T) {
		// Given a scratch path with local state under a flat componentID, a nested one
		// (a component whose Path has slashes), and an empty directory with no state file
		scratchPath := t.TempDir()
		for _, dir := range []string{
			filepath.Join(scratchPath, ".tfstate", "network"),
			filepath.Join(scratchPath, ".tfstate", "provisioning", "crossplane-identity", "gcp"),
			filepath.Join(scratchPath, ".tfstate", "empty"),
		} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("Failed to create directory: %v", err)
			}
		}
		for _, dir := range []string{"network", filepath.Join("provisioning", "crossplane-identity", "gcp")} {
			statePath := filepath.Join(scratchPath, ".tfstate", dir, "terraform.tfstate")
			if err := os.WriteFile(statePath, []byte("{}"), 0644); err != nil {
				t.Fatalf("Failed to write state file: %v", err)
			}
		}

		mocks := setupMocks(t)
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return scratchPath, nil
		}

		// When listing local state componentIDs
		ids, err := mocks.Provider.ListLocalStateComponentIDs()

		// Then both componentIDs are returned, sorted, and the empty directory is excluded
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		expected := []string{"network", "provisioning/crossplane-identity/gcp"}
		if len(ids) != len(expected) {
			t.Fatalf("Expected %v, got %v", expected, ids)
		}
		for i, want := range expected {
			if ids[i] != want {
				t.Errorf("Expected %v, got %v", expected, ids)
				break
			}
		}
	})

	t.Run("ReturnsEmptyWhenTfstateRootDoesNotExist", func(t *testing.T) {
		// Given a scratch path with no .tfstate directory at all
		mocks := setupMocks(t)
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return filepath.Join(t.TempDir(), "does-not-exist"), nil
		}

		// When listing local state componentIDs
		ids, err := mocks.Provider.ListLocalStateComponentIDs()

		// Then no error and no componentIDs are returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(ids) != 0 {
			t.Errorf("Expected no componentIDs, got %v", ids)
		}
	})

	t.Run("ScopesToBackendPrefixWhenConfigured", func(t *testing.T) {
		// Given a scratch path with state under a backend-prefixed subtree
		scratchPath := t.TempDir()
		prefixedDir := filepath.Join(scratchPath, ".tfstate", "team-a", "network")
		if err := os.MkdirAll(prefixedDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(prefixedDir, "terraform.tfstate"), []byte("{}"), 0644); err != nil {
			t.Fatalf("Failed to write state file: %v", err)
		}

		mocks := setupMocks(t)
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return scratchPath, nil
		}
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.prefix" {
				return "team-a"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When listing local state componentIDs
		ids, err := mocks.Provider.ListLocalStateComponentIDs()

		// Then the componentID is relative to the prefixed root, not the bare .tfstate root
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		expected := []string{"network"}
		if len(ids) != len(expected) || ids[0] != expected[0] {
			t.Errorf("Expected %v, got %v", expected, ids)
		}
	})

	t.Run("ReturnsErrorWhenScopeFails", func(t *testing.T) {
		// Given a provider whose scope resolution fails
		mocks := setupMocks(t)
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", fmt.Errorf("scratch path error")
		}

		// When listing local state componentIDs
		_, err := mocks.Provider.ListLocalStateComponentIDs()

		// Then an error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})

	t.Run("ReturnsErrorWhenReadDirFailsForOtherReason", func(t *testing.T) {
		// Given a .tfstate root that exists but fails to read for a non-NotExist reason
		mocks := setupMocks(t)
		scratchPath := t.TempDir()
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return scratchPath, nil
		}
		mocks.Provider.Shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			return nil, fmt.Errorf("mock permission denied")
		}

		// When listing local state componentIDs
		_, err := mocks.Provider.ListLocalStateComponentIDs()

		// Then an error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error reading local state directory") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})
}
