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

func TestPaths_InitWritesValuesUnderBase(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "paths-base")
	helpers.MarkAsGitRepo(t, dir)

	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}

	if _, err := os.Stat(filepath.Join(dir, "infra", "contexts", "local", "values.yaml")); err != nil {
		t.Errorf("expected values.yaml under infra/contexts: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "contexts")); err == nil {
		t.Error("expected no contexts directory at the project root")
	}
}

func TestPaths_EnvReadsRootAndContextValues(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "paths-base")
	contextValues := filepath.Join(dir, "infra", "contexts", "local", "values.yaml")
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
}

func TestPaths_EnvInjectsTerraformDotEnvInsideTheTerraformDirectory(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "paths-base")
	dotEnvPath := filepath.Join(dir, "infra", "contexts", "local", "terraform", ".env")
	if err := os.MkdirAll(filepath.Dir(dotEnvPath), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dotEnvPath, []byte("HYPERV_HOST=hyperv.local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	stdout, stderr, err := helpers.RunCLI(filepath.Join(dir, "infra", "terraform", "null"), []string{"env"}, env)
	if err != nil {
		t.Fatalf("env: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(string(stdout), "HYPERV_HOST=hyperv.local") {
		t.Errorf("expected HYPERV_HOST=hyperv.local in output, got:\n%s", stdout)
	}
}

func TestPaths_BundleWritesTheStandardArchiveLayout(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "paths-base")
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
		if strings.HasPrefix(name, "infra/") {
			t.Errorf("archive entry %s keeps the base directory", name)
		}
	}
}

func TestPaths_PlanTerraformReadsTheTerraformDirectoryUnderBase(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "paths-base")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	_, stderr, err := helpers.RunCLI(dir, []string{"plan", "terraform", "null"}, env)

	if err != nil {
		t.Fatalf("plan terraform null: %v\nstderr: %s", err, stderr)
	}
}

func TestPaths_ShowKustomizationMapsThePathUnderBase(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "paths-base")
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
	if k.Spec.Path != "infra/kustomize/app" {
		t.Errorf("expected spec.path infra/kustomize/app, got %q", k.Spec.Path)
	}
}

func TestPaths_PlanKustomizeBuildsFromTheKustomizeDirectoryUnderBase(t *testing.T) {
	t.Parallel()
	skipIfFluxNotInstalled(t)
	skipIfKustomizeNotInstalled(t)
	dir, env := helpers.PrepareFixture(t, "paths-base")
	env = append(env, "WINDSOR_CONTEXT=local")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"plan", "kustomize", "app"}, env)

	if err != nil {
		t.Fatalf("plan kustomize app: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stdout), "kind: ConfigMap") {
		t.Errorf("expected the built ConfigMap in the plan, got:\n%s", stdout)
	}
}

func TestPaths_ShowArtifactsFindsTagsUnderBase(t *testing.T) {
	for _, args := range [][]string{{"show", "artifacts"}, {"show", "artifacts", "--all"}} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			t.Parallel()
			dir, env := helpers.PrepareFixture(t, "paths-base")
			tagged := "images:\n  - name: ghcr.io/example/app # artifact: type=image name=ghcr.io/example/app\n    tag: 1.0.0\n"
			if err := os.WriteFile(filepath.Join(dir, "infra", "kustomize", "app", "values.yaml"), []byte(tagged), 0644); err != nil {
				t.Fatal(err)
			}
			env = append(env, "WINDSOR_CONTEXT=local")

			stdout, stderr, err := helpers.RunCLI(dir, args, env)

			if err != nil {
				t.Fatalf("%v: %v\nstderr: %s", args, err, stderr)
			}
			if !strings.Contains(string(stdout), "reference: ghcr.io/example/app") {
				t.Errorf("expected the tagged image in the output, got:\n%s", stdout)
			}
		})
	}
}

func TestPaths_FacetFileHelperReadsRelativeToTheFacetUnderBase(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "paths-base")
	facets := filepath.Join(dir, "infra", "contexts", "_template", "facets")
	if err := os.MkdirAll(facets, 0755); err != nil {
		t.Fatal(err)
	}
	facet := "kind: Facet\napiVersion: blueprints.windsorcli.dev/v1alpha1\nmetadata:\n  name: files\nterraform:\n  - path: \"null\"\n    inputs:\n      content: ${file(\"data.txt\")}\n"
	if err := os.WriteFile(filepath.Join(facets, "files.yaml"), []byte(facet), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(facets, "data.txt"), []byte("hello-from-base\n"), 0644); err != nil {
		t.Fatal(err)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(string(stdout), "hello-from-base") {
		t.Errorf("expected the file content in the blueprint, got:\n%s", stdout)
	}
}

func TestPaths_WindsorTestRunsTheTemplateTestsUnderBase(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "facet-requires")
	if err := os.MkdirAll(filepath.Join(dir, "infra"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "contexts"), filepath.Join(dir, "infra", "contexts")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "windsor.yaml"), []byte("version: v1alpha2\npaths:\n  base: infra\n"), 0644); err != nil {
		t.Fatal(err)
	}
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init"}, env); err != nil {
		t.Fatalf("init: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=ok")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"test"}, env)

	if err != nil {
		t.Fatalf("windsor test: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if out := string(stdout) + string(stderr); !strings.Contains(out, "PASS") && !strings.Contains(out, "✓") {
		t.Errorf("expected PASS or ✓ in output: %s", out)
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
