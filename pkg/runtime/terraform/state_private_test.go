package terraform

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTerraformProvider_providerScope(t *testing.T) {
	t.Run("DelegatesDirectlyToConfigHandlerEachCall", func(t *testing.T) {
		// Given a config handler backing this provider — isolation from a concurrent windsor
		// process changing context mid-run now lives in ConfigHandler.GetContext's own cache,
		// not in providerScope, so this only needs to verify correct delegation
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetContextFunc = func() string { return "ctxA" }
		mockConfig.GetConfigRootFunc = func() (string, error) { return "/contexts/ctxA", nil }
		mockConfig.GetWindsorScratchPathFunc = func() (string, error) { return "/.windsor/contexts/ctxA", nil }

		// When resolving scope
		contextName, configRoot, scratch, err := provider.providerScope()

		// Then it returns exactly what the config handler reports
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if contextName != "ctxA" || configRoot != "/contexts/ctxA" || scratch != "/.windsor/contexts/ctxA" {
			t.Errorf("Expected 'ctxA' / '/contexts/ctxA' / '/.windsor/contexts/ctxA', got %q / %q / %q", contextName, configRoot, scratch)
		}
	})

	t.Run("PropagatesConfigRootErrorWithoutCallingWindsorScratchPath", func(t *testing.T) {
		// Given GetConfigRoot fails
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetConfigRootFunc = func() (string, error) { return "", fmt.Errorf("config root error") }
		scratchCalled := false
		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			scratchCalled = true
			return "", nil
		}

		// When resolving scope
		_, _, _, err := provider.providerScope()

		// Then the error is returned, and GetWindsorScratchPath is never attempted
		if err == nil {
			t.Fatal("Expected an error")
		}
		if scratchCalled {
			t.Error("Expected GetWindsorScratchPath not to be called once GetConfigRoot fails")
		}
	})
}
