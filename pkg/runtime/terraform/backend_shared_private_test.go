package terraform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

func writeContext(t *testing.T, root, name, values string) {
	t.Helper()
	dir := filepath.Join(root, "contexts", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "values.yaml"), []byte(values), 0644); err != nil {
		t.Fatal(err)
	}
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTerraformProvider_SharedBackendContexts(t *testing.T) {
	const sharedValues = "terraform:\n  backend:\n    type: s3\n    s3:\n      bucket: org-state\n"
	setup := func(t *testing.T, autoPrefix bool) (*DefaultTerraformProvider, string) {
		t.Helper()
		root := t.TempDir()
		m := setupMocks(t)
		provider := m.Provider
		mocks := provider.configHandler.(*config.MockConfigHandler)
		mocks.GetContextFunc = func() string { return "staging" }
		mocks.GetTerraformBackendTypeFunc = func() string { return "s3" }
		mocks.GetBoolFunc = func(key string, defaultValue ...bool) bool { return autoPrefix }
		mocks.GetStringFunc = func(key string, defaultValue ...string) string { return "" }
		mocks.GetFunc = func(key string) any {
			if key == "terraform.backend.s3" {
				return map[string]any{"bucket": "org-state"}
			}
			return nil
		}
		m.Shell.GetProjectRootFunc = func() (string, error) { return root, nil }
		provider.Shims.ReadDir = os.ReadDir
		return provider, root
	}

	t.Run("WarnsWhenAnotherContextSharesTheBucket", func(t *testing.T) {
		// Given two contexts with the same bucket and no auto_prefix
		provider, root := setup(t, false)
		writeContext(t, root, "prod", sharedValues)

		// When the shared backend check runs
		warnings := provider.SharedBackendContexts()

		// Then it names both contexts and the setting that fixes it
		if len(warnings) != 1 || !strings.Contains(warnings[0], `"prod"`) || !strings.Contains(warnings[0], "auto_prefix") {
			t.Fatalf("expected one warning for prod, got %v", warnings)
		}
	})

	t.Run("IgnoresAContextWithAnotherBucket", func(t *testing.T) {
		// Given a context with a different bucket
		provider, root := setup(t, false)
		writeContext(t, root, "prod", strings.Replace(sharedValues, "org-state", "other-state", 1))

		// When the shared backend check runs
		warnings := provider.SharedBackendContexts()

		// Then it returns nothing
		if len(warnings) != 0 {
			t.Fatalf("expected no warnings, got %v", warnings)
		}
	})

	t.Run("IgnoresAContextWithAnotherPrefix", func(t *testing.T) {
		// Given a context with the same bucket and its own prefix
		provider, root := setup(t, false)
		writeContext(t, root, "prod", sharedValues+"    prefix: prod/\n")

		// When the shared backend check runs
		warnings := provider.SharedBackendContexts()

		// Then it returns nothing, because the keys differ
		if len(warnings) != 0 {
			t.Fatalf("expected no warnings, got %v", warnings)
		}
	})

	t.Run("IgnoresAContextThatSetsAutoPrefix", func(t *testing.T) {
		// Given a context with the same bucket and auto_prefix on
		provider, root := setup(t, false)
		writeContext(t, root, "prod", sharedValues+"    auto_prefix: true\n")

		// When the shared backend check runs
		warnings := provider.SharedBackendContexts()

		// Then it returns nothing
		if len(warnings) != 0 {
			t.Fatalf("expected no warnings, got %v", warnings)
		}
	})

	t.Run("ReturnsNothingWhenAutoPrefixIsOn", func(t *testing.T) {
		// Given the active context has auto_prefix on
		provider, root := setup(t, true)
		writeContext(t, root, "prod", sharedValues)

		// When the shared backend check runs
		warnings := provider.SharedBackendContexts()

		// Then it returns nothing
		if len(warnings) != 0 {
			t.Fatalf("expected no warnings, got %v", warnings)
		}
	})
}
