//go:build integration
// +build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Test Helpers
// =============================================================================

// pathsLayout names the source directories of a paths fixture, relative to the project root.
type pathsLayout struct {
	fixture   string
	terraform string
	kustomize string
	contexts  string
}

var pathsLayouts = []pathsLayout{
	{fixture: "paths-base", terraform: "infra/terraform", kustomize: "infra/kustomize", contexts: "infra/contexts"},
}

// skipIfKustomizeNotInstalled skips the test when the kustomize CLI is absent.
func skipIfKustomizeNotInstalled(t *testing.T) {
	t.Helper()
	cmd := exec.Command("kustomize", "version")
	cmd.Dir = t.TempDir()
	if err := cmd.Run(); err != nil {
		t.Skipf("kustomize CLI not available (%v)", err)
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestPaths_InitWritesValuesInTheConfiguredContextsDirectory(t *testing.T) {
	for _, layout := range pathsLayouts {
		t.Run(layout.fixture, func(t *testing.T) {
			t.Parallel()
			dir, env := helpers.CopyFixtureOnly(t, layout.fixture)
			helpers.MarkAsGitRepo(t, dir)

			_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
			if err != nil {
				t.Fatalf("init local: %v\nstderr: %s", err, stderr)
			}

			valuesPath := filepath.Join(dir, filepath.FromSlash(layout.contexts), "local", "values.yaml")
			if _, err := os.Stat(valuesPath); err != nil {
				t.Errorf("expected values.yaml at %s: %v", valuesPath, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "contexts")); err == nil {
				t.Error("expected no contexts directory at the project root")
			}
		})
	}
}

func TestPaths_EnvReadsRootAndContextValues(t *testing.T) {
	for _, layout := range pathsLayouts {
		t.Run(layout.fixture, func(t *testing.T) {
			t.Parallel()
			dir, env := helpers.PrepareFixture(t, layout.fixture)
			contextValues := filepath.Join(dir, filepath.FromSlash(layout.contexts), "local", "values.yaml")
			if err := os.WriteFile(contextValues, []byte("environment:\n  CONTEXT_VAR: from-context\n"), 0644); err != nil {
				t.Fatalf("write values.yaml: %v", err)
			}
			env = append(env, "WINDSOR_CONTEXT=local")

			stdout, stderr, err := helpers.RunCLI(dir, []string{"env"}, env)
			if err != nil {
				t.Fatalf("env: %v\nstderr: %s", err, stderr)
			}

			for _, want := range []string{"ROOT_VAR=from-root", "CONTEXT_VAR=from-context"} {
				if !strings.Contains(string(stdout), want) {
					t.Errorf("expected %s in output, got:\n%s", want, stdout)
				}
			}
		})
	}
}

func TestPaths_EnvInjectsTerraformDotEnvInsideTheConfiguredTerraformDirectory(t *testing.T) {
	for _, layout := range pathsLayouts {
		t.Run(layout.fixture, func(t *testing.T) {
			t.Parallel()
			dir, env := helpers.PrepareFixture(t, layout.fixture)
			dotEnvPath := filepath.Join(dir, filepath.FromSlash(layout.contexts), "local", "terraform", ".env")
			if err := os.MkdirAll(filepath.Dir(dotEnvPath), 0750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dotEnvPath, []byte("HYPERV_HOST=hyperv.local\n"), 0600); err != nil {
				t.Fatal(err)
			}
			env = append(env, "WINDSOR_CONTEXT=local")

			componentDir := filepath.Join(dir, filepath.FromSlash(layout.terraform), "null")
			stdout, stderr, err := helpers.RunCLI(componentDir, []string{"env"}, env)
			if err != nil {
				t.Fatalf("env: %v\nstderr: %s", err, stderr)
			}

			if !strings.Contains(string(stdout), "HYPERV_HOST=hyperv.local") {
				t.Errorf("expected HYPERV_HOST=hyperv.local in output, got:\n%s", stdout)
			}
		})
	}
}

func TestPaths_BundleWritesTheStandardArchiveLayout(t *testing.T) {
	for _, layout := range pathsLayouts {
		t.Run(layout.fixture, func(t *testing.T) {
			t.Parallel()
			dir, env := helpers.PrepareFixture(t, layout.fixture)
			outPath := filepath.Join(dir, "out.tar.gz")

			_, stderr, err := helpers.RunCLI(dir, []string{"bundle", "-t", "paths:v0.0.1", "-o", outPath}, env)
			if err != nil {
				t.Fatalf("bundle: %v\nstderr: %s", err, stderr)
			}

			entries := readBundleEntries(t, outPath)
			for _, want := range []string{"_template/blueprint.yaml", "kustomize/app/kustomization.yaml", "terraform/null/main.tf"} {
				if _, ok := entries[want]; !ok {
					t.Errorf("expected %s in the archive", want)
				}
			}
			for name := range entries {
				for _, moved := range []string{"infra/"} {
					if strings.HasPrefix(name, moved) {
						t.Errorf("archive entry %s keeps the project directory", name)
					}
				}
			}
		})
	}
}

func TestPaths_PlanTerraformReadsTheConfiguredTerraformDirectory(t *testing.T) {
	for _, layout := range pathsLayouts {
		t.Run(layout.fixture, func(t *testing.T) {
			t.Parallel()
			dir, env := helpers.CopyFixtureOnly(t, layout.fixture)
			helpers.MarkAsGitRepo(t, dir)
			if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
				t.Fatalf("init local: %v\nstderr: %s", err, stderr)
			}
			env = append(env, "WINDSOR_CONTEXT=local")

			_, stderr, err := helpers.RunCLI(dir, []string{"plan", "terraform", "null"}, env)

			if err != nil {
				t.Fatalf("plan terraform null: %v\nstderr: %s", err, stderr)
			}
		})
	}
}

func TestPaths_ShowKustomizationMapsThePathToTheConfiguredDirectory(t *testing.T) {
	for _, layout := range pathsLayouts {
		t.Run(layout.fixture, func(t *testing.T) {
			t.Parallel()
			dir, env := helpers.PrepareFixture(t, layout.fixture)
			env = append(env, "WINDSOR_CONTEXT=local")

			stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "kustomization", "app"}, env)
			if err != nil {
				t.Fatalf("show kustomization app: %v\nstderr: %s", err, stderr)
			}

			var k struct {
				Spec struct {
					Path string `yaml:"path"`
				} `yaml:"spec"`
			}
			if err := yaml.Unmarshal(stdout, &k); err != nil {
				t.Fatalf("parse kustomization YAML: %v\nstdout: %s", err, stdout)
			}
			if want := layout.kustomize + "/app"; k.Spec.Path != want {
				t.Errorf("expected spec.path %s, got %q", want, k.Spec.Path)
			}
		})
	}
}

func TestPaths_PlanKustomizeBuildsFromTheConfiguredKustomizeDirectory(t *testing.T) {
	skipIfFluxNotInstalled(t)
	skipIfKustomizeNotInstalled(t)
	for _, layout := range pathsLayouts {
		t.Run(layout.fixture, func(t *testing.T) {
			t.Parallel()
			dir, env := helpers.PrepareFixture(t, layout.fixture)
			env = append(env, "WINDSOR_CONTEXT=local")

			stdout, stderr, err := helpers.RunCLI(dir, []string{"plan", "kustomize", "app"}, env)

			if err != nil {
				t.Fatalf("plan kustomize app: %v\nstderr: %s", err, stderr)
			}
			if !strings.Contains(string(stdout), "kind: ConfigMap") {
				t.Errorf("expected the built ConfigMap in the plan, got:\n%s", stdout)
			}
		})
	}
}

func TestPaths_RejectsAnInvalidPathsBlock(t *testing.T) {
	cases := map[string]string{
		"AbsolutePath":      "paths:\n  base: /abs\n",
		"ParentElement":     "paths:\n  base: ../outside\n",
		"InsideWindsorDir":  "paths:\n  base: .windsor/infra\n",
		"PerKindKey":        "paths:\n  terraform: tf\n",
		"BaseIsProjectRoot": "paths:\n  base: .\n",
		"EmptyValue":        "paths:\n  base: \"\"\n",
	}
	for name, block := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir, env := helpers.PrepareFixture(t, "default")
			if err := os.WriteFile(filepath.Join(dir, "windsor.yaml"), []byte("version: v1alpha2\n"+block), 0644); err != nil {
				t.Fatal(err)
			}
			env = append(env, "WINDSOR_CONTEXT=default")

			_, stderr, err := helpers.RunCLI(dir, []string{"env"}, env)

			if err == nil {
				t.Fatal("expected failure but command succeeded")
			}
			if !strings.Contains(string(stderr), "paths") {
				t.Errorf("expected stderr to name the paths block, got: %s", stderr)
			}
		})
	}
}
