package terraform

import (
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

type autoPrefixSettings struct {
	backend    string
	context    string
	id         string
	prefix     string
	autoPrefix bool
}

func newAutoPrefixProvider(t *testing.T, settings autoPrefixSettings) *DefaultTerraformProvider {
	t.Helper()
	provider := setupMocks(t).Provider
	mockConfig := provider.configHandler.(*config.MockConfigHandler)
	mockConfig.GetConfigRootFunc = func() (string, error) { return "/test/config", nil }
	mockConfig.GetWindsorScratchPathFunc = func() (string, error) { return "/test/scratch", nil }
	mockConfig.GetContextFunc = func() string { return settings.context }
	mockConfig.GetTerraformBackendTypeFunc = func() string { return settings.backend }
	mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "terraform.backend.type":
			return settings.backend
		case "terraform.backend.prefix":
			return settings.prefix
		case "id":
			return settings.id
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		if key == "terraform.backend.auto_prefix" {
			return settings.autoPrefix
		}
		return false
	}
	mockConfig.GetFunc = func(key string) any { return nil }
	provider.Shims.Stat = func(path string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	return provider
}

func backendArg(args []string, field string) string {
	for _, arg := range args {
		if value, ok := strings.CutPrefix(arg, "-backend-config="+field+"="); ok {
			return value
		}
	}
	return ""
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTerraformProvider_generateBackendConfigArgs_autoPrefix(t *testing.T) {
	cases := []struct {
		backend string
		field   string
		legacy  string
		scoped  string
	}{
		{"s3", "key", "clusters/aws/terraform.tfstate", "staging-w8k2x9q/clusters/aws/terraform.tfstate"},
		{"azurerm", "key", "clusters/aws/terraform.tfstate", "staging-w8k2x9q/clusters/aws/terraform.tfstate"},
		{"gcs", "prefix", "clusters/aws", "staging-w8k2x9q/clusters/aws"},
		{"kubernetes", "secret_suffix", "clusters-aws", "staging-w8k2x9q-clusters-aws"},
	}
	for _, tc := range cases {
		t.Run("KeepsLegacyKeyWhenOff/"+tc.backend, func(t *testing.T) {
			// Given a context that does not set auto_prefix
			provider := newAutoPrefixProvider(t, autoPrefixSettings{backend: tc.backend, context: "staging", id: "w8k2x9q"})

			// When the backend args are generated
			args, err := provider.generateBackendConfigArgs("clusters/aws", "/test/config")

			// Then the key has no context segment
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got := backendArg(args, tc.field); got != tc.legacy {
				t.Fatalf("expected %q, got %q", tc.legacy, got)
			}
		})

		t.Run("AddsContextSegmentWhenOn/"+tc.backend, func(t *testing.T) {
			// Given a context that sets auto_prefix
			provider := newAutoPrefixProvider(t, autoPrefixSettings{backend: tc.backend, context: "staging", id: "w8k2x9q", autoPrefix: true})

			// When the backend args are generated
			args, err := provider.generateBackendConfigArgs("clusters/aws", "/test/config")

			// Then the key has the context and ID segment before the component path
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got := backendArg(args, tc.field); got != tc.scoped {
				t.Fatalf("expected %q, got %q", tc.scoped, got)
			}
		})

		t.Run("SeparatesTwoContextsOnOneBackend/"+tc.backend, func(t *testing.T) {
			// Given two contexts with the same backend and component, both with auto_prefix
			staging := newAutoPrefixProvider(t, autoPrefixSettings{backend: tc.backend, context: "staging", id: "w8k2x9q", autoPrefix: true})
			prod := newAutoPrefixProvider(t, autoPrefixSettings{backend: tc.backend, context: "prod", id: "p4m7r2k", autoPrefix: true})

			// When each context generates its backend args
			stagingArgs, err := staging.generateBackendConfigArgs("clusters/aws", "/test/config")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			prodArgs, err := prod.generateBackendConfigArgs("clusters/aws", "/test/config")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			// Then the keys differ
			if backendArg(stagingArgs, tc.field) == backendArg(prodArgs, tc.field) {
				t.Fatalf("expected distinct keys, both are %q", backendArg(stagingArgs, tc.field))
			}
		})

		t.Run("PutsPrefixBeforeContextSegment/"+tc.backend, func(t *testing.T) {
			// Given a context with a manual prefix and auto_prefix
			provider := newAutoPrefixProvider(t, autoPrefixSettings{backend: tc.backend, context: "staging", id: "w8k2x9q", prefix: "team-a/", autoPrefix: true})

			// When the backend args are generated
			args, err := provider.generateBackendConfigArgs("clusters/aws", "/test/config")

			// Then the manual prefix comes first
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			got := backendArg(args, tc.field)
			if !strings.HasPrefix(got, "team-a") || !strings.Contains(got, "staging-w8k2x9q") || strings.Index(got, "team-a") > strings.Index(got, "staging") {
				t.Fatalf("expected team-a before staging-w8k2x9q, got %q", got)
			}
		})

		t.Run("ReturnsLegacyKeyForMigration/"+tc.backend, func(t *testing.T) {
			// Given a context with auto_prefix on
			provider := newAutoPrefixProvider(t, autoPrefixSettings{backend: tc.backend, context: "staging", id: "w8k2x9q", autoPrefix: true})

			// When the legacy key is requested
			field, value := provider.legacyBackendKey("clusters/aws")

			// Then it is the key that auto_prefix off produces
			if field != tc.field || value != tc.legacy {
				t.Fatalf("expected %s=%q, got %s=%q", tc.field, tc.legacy, field, value)
			}
		})
	}

	t.Run("ReturnsErrorWhenIDMissing", func(t *testing.T) {
		// Given auto_prefix on and no context ID
		provider := newAutoPrefixProvider(t, autoPrefixSettings{backend: "s3", context: "staging", autoPrefix: true})

		// When the backend args are generated
		_, err := provider.generateBackendConfigArgs("clusters/aws", "/test/config")

		// Then the error names the option and the remediation
		if err == nil || !strings.Contains(err.Error(), "auto_prefix") {
			t.Fatalf("expected an auto_prefix error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenKubernetesSuffixTooLong", func(t *testing.T) {
		// Given a kubernetes backend whose suffix with the segment exceeds 63 characters
		provider := newAutoPrefixProvider(t, autoPrefixSettings{backend: "kubernetes", context: "staging", id: "w8k2x9q", autoPrefix: true})
		path := strings.Repeat("a", 50)

		// When the backend args are generated
		_, err := provider.generateBackendConfigArgs(path, "/test/config")

		// Then it returns an error instead of cutting the suffix
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("DoesNotChangeLocalBackend", func(t *testing.T) {
		// Given a local backend with auto_prefix on
		scoped := newAutoPrefixProvider(t, autoPrefixSettings{backend: "local", context: "staging", id: "w8k2x9q", autoPrefix: true})
		plain := newAutoPrefixProvider(t, autoPrefixSettings{backend: "local", context: "staging", id: "w8k2x9q"})

		// When the backend args are generated
		scopedArgs, err := scoped.generateBackendConfigArgs("clusters/aws", "/test/config")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		plainArgs, err := plain.generateBackendConfigArgs("clusters/aws", "/test/config")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the args are identical
		if strings.Join(scopedArgs, " ") != strings.Join(plainArgs, " ") {
			t.Fatalf("expected equal args, got %v and %v", scopedArgs, plainArgs)
		}
	})
}
