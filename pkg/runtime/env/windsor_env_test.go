package env_test

import (
	"os"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupWindsorEnvMocks(t *testing.T, overrides ...*EnvTestMocks) *EnvTestMocks {
	t.Helper()
	mocks := setupEnvMocks(t, overrides...)

	// Only load default config if ConfigHandler wasn't overridden
	// If ConfigHandler was injected via overrides, assume test wants to control it
	if len(overrides) == 0 || overrides[0] == nil || overrides[0].ConfigHandler == nil {
		configStr := `
version: v1alpha1
contexts:
  mock-context:
    environment:
      TEST_VAR: test_value
      SECRET_VAR: "{{secret_name}}"
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		mocks.ConfigHandler.SetContext("mock-context")
	}

	// Get the temp dir that was set up in setupMocks
	projectRoot, err := mocks.Shell.GetProjectRoot()
	if err != nil {
		t.Fatalf("Failed to get project root: %v", err)
	}

	// Set up shims for Windsor operations
	mocks.Shims.LookupEnv = func(key string) (string, bool) {
		// Use os.LookupEnv to get the real environment variables
		val, ok := os.LookupEnv(key)
		return val, ok
	}

	// Mock GetSessionToken
	mocks.Shell.GetSessionTokenFunc = func() (string, error) {
		return "mock-token", nil
	}

	t.Cleanup(func() {
		os.Unsetenv("NO_CACHE")
		os.Unsetenv("WINDSOR_MANAGED_ENV")
		mocks.Shell.ResetSessionToken()
	})

	// Set environment variables using the temp dir
	t.Setenv("WINDSOR_CONTEXT", "mock-context")
	t.Setenv("WINDSOR_PROJECT_ROOT", projectRoot)

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestWindsorEnv_PostEnvHook tests the PostEnvHook method of the WindsorEnvPrinter
func TestWindsorEnv_PostEnvHook(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a WindsorEnvPrinter
		mocks := setupWindsorEnvMocks(t)
		windsorEnvPrinter := env.NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, nil, []env.EnvPrinter{})

		// When PostEnvHook is called
		err := windsorEnvPrinter.PostEnvHook()

		// Then no error should be returned
		if err != nil {
			t.Errorf("PostEnvHook() returned an error: %v", err)
		}
	})
}

// TestWindsorEnv_Initialize tests the Initialize method of the WindsorEnvPrinter
func TestWindsorEnv_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a WindsorEnvPrinter
		mocks := setupWindsorEnvMocks(t)
		printer := env.NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, nil, []env.EnvPrinter{})
		printer.SetShims(mocks.Shims)

		// Then printer should be created
		if printer == nil {
			t.Fatal("Expected printer to be created")
		}
	})

	t.Run("WithEvaluator", func(t *testing.T) {
		// Given a WindsorEnvPrinter with an evaluator
		mocks := setupWindsorEnvMocks(t)
		mockEval := evaluator.NewMockExpressionEvaluator()
		printer := env.NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, mockEval, []env.EnvPrinter{})

		// Then printer should be created with evaluator
		if printer == nil {
			t.Fatal("Expected printer to be created")
		}
		if printer.Evaluator() == nil {
			t.Error("Expected evaluator to be set")
		}
	})
}
