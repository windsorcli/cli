package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Test Typed Source
// =============================================================================

func TestTypedSource_EnsureRoot(t *testing.T) {
	t.Run("CreatesRootWindsorYamlWhenMissing", func(t *testing.T) {
		source := newTypedSource(NewShims())
		projectRoot := t.TempDir()

		if err := source.EnsureRoot(projectRoot); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		rootPath := filepath.Join(projectRoot, "windsor.yaml")
		content, err := os.ReadFile(rootPath)
		if err != nil {
			t.Fatalf("Expected windsor.yaml to exist, got %v", err)
		}
		if !contains(string(content), "version: v1alpha2") {
			t.Errorf("Expected v1alpha2 root config, got %s", string(content))
		}
	})
}

func TestTypedSource_LoadRoot(t *testing.T) {
	t.Run("ReturnsContextMapFromRootContextsBlock", func(t *testing.T) {
		source := newTypedSource(NewShims())
		projectRoot := t.TempDir()
		rootConfig := `version: v1alpha1
contexts:
  local:
    provider: docker
    cluster:
      workers:
        count: 2
`
		if err := os.WriteFile(filepath.Join(projectRoot, "windsor.yaml"), []byte(rootConfig), 0644); err != nil {
			t.Fatalf("Expected no error writing root config, got %v", err)
		}

		contextMap, found, err := source.LoadRoot(projectRoot, "local")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !found {
			t.Fatal("Expected root config to be found")
		}
		if contextMap["provider"] != "docker" {
			t.Errorf("Expected provider=docker, got %v", contextMap["provider"])
		}
	})
}

func TestTypedSource_LoadRootVersions(t *testing.T) {
	load := func(t *testing.T, files map[string]string) (map[string]any, bool, error) {
		t.Helper()
		projectRoot := t.TempDir()
		for name, content := range files {
			path := filepath.Join(projectRoot, name)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatalf("Expected no error creating %s, got %v", path, err)
			}
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatalf("Expected no error writing %s, got %v", path, err)
			}
		}
		return newTypedSource(NewShims()).LoadRoot(projectRoot, "local")
	}

	t.Run("LoadsV1alpha1RootWithContextsAndTerraformDriver", func(t *testing.T) {
		contextMap, found, err := load(t, map[string]string{
			"windsor.yaml": "version: v1alpha1\nterraform:\n  driver: tofu\ncontexts:\n  local:\n    platform: docker\n",
		})
		if err != nil || !found {
			t.Fatalf("Expected the root to load, got found=%v err=%v", found, err)
		}
		if contextMap["platform"] != "docker" {
			t.Errorf("Expected platform=docker, got %v", contextMap["platform"])
		}
	})

	t.Run("LoadsRootWithNoVersionAsV1alpha1", func(t *testing.T) {
		contextMap, found, err := load(t, map[string]string{
			"windsor.yaml": "contexts:\n  local:\n    platform: docker\n",
		})
		if err != nil || !found || contextMap["platform"] != "docker" {
			t.Fatalf("Expected the root to load, got map=%v found=%v err=%v", contextMap, found, err)
		}
	})

	t.Run("LoadsV1alpha1RootBesideContextWindsorYaml", func(t *testing.T) {
		_, found, err := load(t, map[string]string{
			"windsor.yaml":                "version: v1alpha1\n",
			"contexts/local/windsor.yaml": "platform: docker\n",
		})
		if err != nil || !found {
			t.Fatalf("Expected the root to load, got found=%v err=%v", found, err)
		}
	})

	t.Run("LoadsMinimalV1alpha2Root", func(t *testing.T) {
		contextMap, found, err := load(t, map[string]string{"windsor.yaml": "version: v1alpha2\n"})
		if err != nil || !found || contextMap != nil {
			t.Fatalf("Expected the root to load with no context map, got map=%v found=%v err=%v", contextMap, found, err)
		}
	})

	t.Run("LoadsFullV1alpha2Root", func(t *testing.T) {
		_, found, err := load(t, map[string]string{
			"windsor.yaml": "version: v1alpha2\nenvironment:\n  AWS_DEFAULT_REGION: us-east-2\npaths:\n  terraform: infra/terraform\n  kustomize: infra/kustomize\n  contexts: infra/contexts\n",
		})
		if err != nil || !found {
			t.Fatalf("Expected the root to load, got found=%v err=%v", found, err)
		}
	})

	t.Run("LoadsV1alpha2RootInAProjectPathWithGlobCharacters", func(t *testing.T) {
		projectRoot := filepath.Join(t.TempDir(), "proj[1")
		if err := os.MkdirAll(filepath.Join(projectRoot, "contexts", "local"), 0755); err != nil {
			t.Fatalf("Expected no error creating the project, got %v", err)
		}
		if err := os.WriteFile(filepath.Join(projectRoot, "windsor.yaml"), []byte("version: v1alpha2\n"), 0644); err != nil {
			t.Fatalf("Expected no error writing the root, got %v", err)
		}
		_, found, err := newTypedSource(NewShims()).LoadRoot(projectRoot, "local")
		if err != nil || !found {
			t.Fatalf("Expected the root to load, got found=%v err=%v", found, err)
		}
	})

	rejections := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"ContextsKey", map[string]string{"windsor.yaml": "version: v1alpha2\ncontexts:\n  local: {}\n"}, []string{"windsor.yaml", `"contexts"`, "windsor migrate"}},
		{"UnknownKey", map[string]string{"windsor.yaml": "version: v1alpha2\nenviroment: {}\n"}, []string{"windsor.yaml", `"enviroment"`, "windsor migrate"}},
		{"TerraformKey", map[string]string{"windsor.yaml": "version: v1alpha2\nterraform:\n  driver: tofu\n"}, []string{"windsor.yaml", `"terraform"`, "windsor migrate"}},
		{"UnsupportedVersion", map[string]string{"windsor.yaml": "version: v1alpha3\n"}, []string{"windsor.yaml", "v1alpha3", "windsor migrate"}},
		{"NonStringVersion", map[string]string{"windsor.yaml": "version: 2\n"}, []string{"windsor.yaml", "2", "windsor migrate"}},
		{"EnvironmentNotAMap", map[string]string{"windsor.yaml": "version: v1alpha2\nenvironment: [a]\n"}, []string{"windsor.yaml", `"environment"`}},
		{"EnvironmentValueNotAString", map[string]string{"windsor.yaml": "version: v1alpha2\nenvironment:\n  PORT: 8080\n"}, []string{"windsor.yaml", `"environment.PORT"`}},
		{"PathsNotAnObject", map[string]string{"windsor.yaml": "version: v1alpha2\npaths: infra\n"}, []string{"windsor.yaml", `"paths"`}},
		{"PathsUnknownKey", map[string]string{"windsor.yaml": "version: v1alpha2\npaths:\n  state: x\n"}, []string{"windsor.yaml", `"state"`}},
		{"PathsValueNotAString", map[string]string{"windsor.yaml": "version: v1alpha2\npaths:\n  terraform: [a]\n"}, []string{"windsor.yaml", `"paths.terraform"`}},
		{"ContextWindsorYaml", map[string]string{"windsor.yaml": "version: v1alpha2\n", "contexts/prod/windsor.yaml": "platform: aws\n"}, []string{filepath.Join("contexts", "prod", "windsor.yaml"), "windsor migrate"}},
		{"ContextWindsorYml", map[string]string{"windsor.yaml": "version: v1alpha2\n", "contexts/prod/windsor.yml": "platform: aws\n"}, []string{filepath.Join("contexts", "prod", "windsor.yml"), "windsor migrate"}},
	}
	for _, tc := range rejections {
		t.Run("Rejects"+tc.name, func(t *testing.T) {
			_, _, err := load(t, tc.files)
			if err == nil {
				t.Fatal("Expected an error, got none")
			}
			var we *werror.WindsorError
			if !errors.As(err, &we) {
				t.Fatalf("Expected a WindsorError, got %T", err)
			}
			text := we.Message + " " + we.Remediation
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("Expected %q in %q", want, text)
				}
			}
		})
	}
}

func TestTypedSource_LoadContext(t *testing.T) {
	t.Run("LoadsLegacyContextWindsorYaml", func(t *testing.T) {
		source := newTypedSource(NewShims())
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		if err := os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte("provider: aws\n"), 0644); err != nil {
			t.Fatalf("Expected no error writing context windsor.yaml, got %v", err)
		}

		contextMap, found, err := source.LoadContext(projectRoot, contextName)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !found {
			t.Fatal("Expected legacy context config to be found")
		}
		if contextMap["provider"] != "aws" {
			t.Errorf("Expected provider=aws, got %v", contextMap["provider"])
		}
	})

	t.Run("LoadsLegacyContextWindsorYmlFallback", func(t *testing.T) {
		source := newTypedSource(NewShims())
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		if err := os.WriteFile(filepath.Join(contextDir, "windsor.yml"), []byte("provider: gcp\n"), 0644); err != nil {
			t.Fatalf("Expected no error writing context windsor.yml, got %v", err)
		}

		contextMap, found, err := source.LoadContext(projectRoot, contextName)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !found {
			t.Fatal("Expected legacy context config fallback to be found")
		}
		if contextMap["provider"] != "gcp" {
			t.Errorf("Expected provider=gcp, got %v", contextMap["provider"])
		}
	})
}
