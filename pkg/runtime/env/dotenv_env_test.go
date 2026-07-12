package env

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupDotEnvEnvMocks(t *testing.T, overrides ...*EnvTestMocks) *EnvTestMocks {
	t.Helper()
	mocks := setupEnvMocks(t, overrides...)

	mocks.Shims.LookupEnv = func(key string) (string, bool) {
		return os.LookupEnv(key)
	}

	return mocks
}

// writeDotEnvFixture writes content to contexts/<context>/.env for the mock's config
// root and points Stat/ReadFile at the real filesystem so permission bits are honored.
func writeDotEnvFixture(t *testing.T, mocks *EnvTestMocks, content string, perm os.FileMode) string {
	t.Helper()

	configRoot, err := mocks.ConfigHandler.GetConfigRoot()
	if err != nil {
		t.Fatalf("Failed to get config root: %v", err)
	}
	if err := os.MkdirAll(configRoot, 0750); err != nil {
		t.Fatalf("Failed to create config root: %v", err)
	}

	path := filepath.Join(configRoot, ".env")
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatalf("Failed to write .env fixture: %v", err)
	}

	mocks.Shims.Stat = os.Stat
	mocks.Shims.ReadFile = os.ReadFile

	return path
}

func setupDotEnvPrinter(t *testing.T, eval evaluator.ExpressionEvaluator) (*DotEnvEnvPrinter, *EnvTestMocks) {
	t.Helper()
	mocks := setupDotEnvEnvMocks(t)
	printer := NewDotEnvEnvPrinter(mocks.Shell, mocks.ConfigHandler, eval)
	printer.shims = mocks.Shims
	return printer, mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewDotEnvEnvPrinter(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given valid shell and config handler
		mocks := setupDotEnvEnvMocks(t)

		// When creating a new DotEnvEnvPrinter
		printer := NewDotEnvEnvPrinter(mocks.Shell, mocks.ConfigHandler, nil)

		// Then it should be created successfully
		if printer == nil {
			t.Error("Expected printer to be created")
		}
	})

	t.Run("PanicsWithNilShell", func(t *testing.T) {
		// Given a nil shell
		mocks := setupDotEnvEnvMocks(t)

		// When creating a new DotEnvEnvPrinter
		defer func() {
			// Then it should panic
			if r := recover(); r == nil {
				t.Error("Expected panic, got none")
			}
		}()
		NewDotEnvEnvPrinter(nil, mocks.ConfigHandler, nil)
	})

	t.Run("PanicsWithNilConfigHandler", func(t *testing.T) {
		// Given a nil config handler
		mocks := setupDotEnvEnvMocks(t)

		// When creating a new DotEnvEnvPrinter
		defer func() {
			// Then it should panic
			if r := recover(); r == nil {
				t.Error("Expected panic, got none")
			}
		}()
		NewDotEnvEnvPrinter(mocks.Shell, nil, nil)
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDotEnvEnvPrinter_GetEnvVars(t *testing.T) {
	t.Run("ReturnsEmptyMapWhenFileMissing", func(t *testing.T) {
		// Given a DotEnvEnvPrinter with no .env file on disk
		printer, mocks := setupDotEnvPrinter(t, nil)
		mocks.Shims.Stat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned and the map should be empty
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(envVars) != 0 {
			t.Errorf("Expected empty envVars, got %v", envVars)
		}
	})

	t.Run("LoadsPlainKeyValuePairsAndTracksManagedEnv", func(t *testing.T) {
		// Given a DotEnvEnvPrinter with a plain .env file
		printer, mocks := setupDotEnvPrinter(t, nil)
		writeDotEnvFixture(t, mocks, "HYPERV_USER=admin\nHYPERV_HOST=hyperv.local\n", 0600)

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then the values should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if envVars["HYPERV_USER"] != "admin" {
			t.Errorf("Expected HYPERV_USER=admin, got %q", envVars["HYPERV_USER"])
		}
		if envVars["HYPERV_HOST"] != "hyperv.local" {
			t.Errorf("Expected HYPERV_HOST=hyperv.local, got %q", envVars["HYPERV_HOST"])
		}

		// And both keys should be tracked as managed
		managed := printer.GetManagedEnv()
		for _, key := range []string{"HYPERV_USER", "HYPERV_HOST"} {
			found := false
			for _, m := range managed {
				if m == key {
					found = true
				}
			}
			if !found {
				t.Errorf("Expected %s to be tracked in managed env, got %v", key, managed)
			}
		}
	})

	t.Run("SkipsCommentsBlankLinesAndMalformedLines", func(t *testing.T) {
		// Given a .env file with comments, blank lines, and a malformed line
		printer, mocks := setupDotEnvPrinter(t, nil)
		writeDotEnvFixture(t, mocks, "# a comment\n\nVALID=yes\nNOEQUALSIGN\n", 0600)

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then only the valid key should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(envVars) != 1 || envVars["VALID"] != "yes" {
			t.Errorf("Expected only VALID=yes, got %v", envVars)
		}
	})

	t.Run("EvaluatesExpressionValues", func(t *testing.T) {
		// Given a .env file with a secret expression and a mock evaluator
		mockEval := evaluator.NewMockExpressionEvaluator()
		mockEval.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if strings.Contains(expression, "secret(") {
				return "resolved-secret", nil
			}
			return expression, nil
		}
		printer, mocks := setupDotEnvPrinter(t, mockEval)
		writeDotEnvFixture(t, mocks, `HYPERV_PASSWORD=${secret("op://vault/item/field")}`+"\n", 0600)

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then the expression should be resolved through the evaluator
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if envVars["HYPERV_PASSWORD"] != "resolved-secret" {
			t.Errorf("Expected resolved-secret, got %q", envVars["HYPERV_PASSWORD"])
		}
	})

	t.Run("OmitsCachedKeyWhenAlreadyPresentInShell", func(t *testing.T) {
		// Given a cached shell value for a secret key and caching enabled
		mockEval := evaluator.NewMockExpressionEvaluator()
		evaluateCalled := false
		mockEval.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			evaluateCalled = true
			return "freshly-resolved", nil
		}
		printer, mocks := setupDotEnvPrinter(t, mockEval)
		writeDotEnvFixture(t, mocks, `HYPERV_PASSWORD=${secret("op://vault/item/field")}`+"\n", 0600)

		t.Setenv("NO_CACHE", "0")
		t.Setenv("HYPERV_PASSWORD", "cached-value")

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then the key should be omitted and the evaluator should not be called
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := envVars["HYPERV_PASSWORD"]; exists {
			t.Errorf("Expected HYPERV_PASSWORD to be omitted from envVars, got %v", envVars)
		}
		if evaluateCalled {
			t.Error("Expected evaluator not to be called when a cached value exists")
		}
	})

	t.Run("ReEvaluatesWhenNoCacheSet", func(t *testing.T) {
		// Given a cached shell value but NO_CACHE set
		mockEval := evaluator.NewMockExpressionEvaluator()
		mockEval.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "freshly-resolved", nil
		}
		printer, mocks := setupDotEnvPrinter(t, mockEval)
		writeDotEnvFixture(t, mocks, `HYPERV_PASSWORD=${secret("op://vault/item/field")}`+"\n", 0600)

		t.Setenv("NO_CACHE", "1")
		t.Setenv("HYPERV_PASSWORD", "cached-value")

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then the value should be freshly evaluated
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if envVars["HYPERV_PASSWORD"] != "freshly-resolved" {
			t.Errorf("Expected freshly-resolved, got %q", envVars["HYPERV_PASSWORD"])
		}
	})

	t.Run("RegistersLoadedValuesForScrubbing", func(t *testing.T) {
		// Given a .env file with a plain value
		printer, mocks := setupDotEnvPrinter(t, nil)
		writeDotEnvFixture(t, mocks, "HYPERV_USER=admin\n", 0600)

		var registered []string
		mocks.Shell.RegisterSecretFunc = func(value string) {
			registered = append(registered, value)
		}

		// When GetEnvVars is called
		if _, err := printer.GetEnvVars(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the loaded value should be registered for scrubbing
		found := false
		for _, v := range registered {
			if v == "admin" {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected 'admin' to be registered for scrubbing, got %v", registered)
		}
	})

	t.Run("ReturnsErrorWhenConfigRootFails", func(t *testing.T) {
		// Given a config handler that fails to resolve the config root
		mocks := setupDotEnvEnvMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting project root")
		}
		printer := NewDotEnvEnvPrinter(mocks.Shell, mocks.ConfigHandler, nil)
		printer.shims = mocks.Shims

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving config root") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenStatFailsForNonMissingReason", func(t *testing.T) {
		// Given Stat failing with a non-NotExist error
		printer, mocks := setupDotEnvPrinter(t, nil)
		mocks.Shims.Stat = func(string) (os.FileInfo, error) {
			return nil, fmt.Errorf("mock permission denied")
		}

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error checking") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenReadFileFails", func(t *testing.T) {
		// Given a .env file that exists but fails to read
		printer, mocks := setupDotEnvPrinter(t, nil)
		writeDotEnvFixture(t, mocks, "VALID=yes\n", 0600)
		mocks.Shims.ReadFile = func(string) ([]byte, error) {
			return nil, fmt.Errorf("mock read error")
		}

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error reading") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("WarnsToStderrOnLoosePermissions", func(t *testing.T) {
		// Given a .env file with group/world-readable permissions
		printer, mocks := setupDotEnvPrinter(t, nil)
		writeDotEnvFixture(t, mocks, "VALID=yes\n", 0644)

		var stderr bytes.Buffer
		printer.warningWriter = &stderr

		// When GetEnvVars is called
		if _, err := printer.GetEnvVars(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then a warning should be written
		if !strings.Contains(stderr.String(), "Warning") {
			t.Errorf("Expected a permission warning, got %q", stderr.String())
		}
	})

	t.Run("NoWarningWhenPermissionsAreRestricted", func(t *testing.T) {
		// Given a .env file restricted to the owner
		printer, mocks := setupDotEnvPrinter(t, nil)
		writeDotEnvFixture(t, mocks, "VALID=yes\n", 0600)

		var stderr bytes.Buffer
		printer.warningWriter = &stderr

		// When GetEnvVars is called
		if _, err := printer.GetEnvVars(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then no warning should be written
		if stderr.String() != "" {
			t.Errorf("Expected no warning, got %q", stderr.String())
		}
	})
}
