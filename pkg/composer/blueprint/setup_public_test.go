package blueprint_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type fixture struct {
	root     string
	shell    *shell.MockShell
	config   config.ConfigHandler
	artifact *artifact.MockArtifact
	runtime  *runtime.Runtime
	outputs  map[string]map[string]any
	sources  map[string]string
	tags     []string
}

const blueprintHeader = "kind: Blueprint\napiVersion: blueprints.windsorcli.dev/v1alpha1\n"
const facetHeader = "kind: Facet\napiVersion: blueprints.windsorcli.dev/v1alpha1\n"

// newFixture returns a project in a fresh temporary directory, with the local context selected. Its
// terraform_output helper defers unless asked to resolve, then reads the fixture's outputs map.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("WINDSOR_CONTEXT", "")
	root := t.TempDir()
	t.Setenv("WINDSOR_PROJECT_ROOT", root)
	sh := shell.NewMockShell()
	sh.GetProjectRootFunc = func() (string, error) { return root, nil }
	cfg := config.NewConfigHandler(sh)
	templateRoot := filepath.Join(root, "contexts", "_template")
	eval := evaluator.NewExpressionEvaluator(cfg, root, templateRoot)
	f := &fixture{root: root, shell: sh, config: cfg, artifact: artifact.NewMockArtifact(), outputs: map[string]map[string]any{}, sources: map[string]string{}}
	f.artifact.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
		return parseRef(ref)
	}
	f.artifact.PullFunc = func(refs []string) (map[string]string, error) {
		out := map[string]string{}
		for _, ref := range refs {
			dir, ok := f.sources[ref]
			if !ok {
				return nil, fmt.Errorf("no artifact published at %s", ref)
			}
			registry, repository, tag, _ := parseRef(ref)
			out[artifact.FormatOCIRef(registry, repository, tag)] = dir
		}
		return out, nil
	}
	f.artifact.ListTagsFunc = func(string) ([]string, error) { return f.tags, nil }
	eval.Register("terraform_output", func(params []any, deferred bool) (any, error) {
		component, key := fmt.Sprint(params[0]), fmt.Sprint(params[1])
		if !deferred {
			return nil, &evaluator.DeferredError{Expression: fmt.Sprintf(`terraform_output("%s", "%s")`, component, key)}
		}
		return f.outputs[component][key], nil
	}, new(func(string, string) any))
	f.runtime = &runtime.Runtime{
		ContextName:        "local",
		ProjectRoot:        root,
		ConfigRoot:         filepath.Join(root, "contexts", "local"),
		TemplateRoot:       templateRoot,
		WindsorScratchPath: filepath.Join(root, ".windsor", "contexts", "local"),
		ConfigHandler:      cfg,
		Shell:              sh,
		Evaluator:          eval,
	}
	return f
}

// write creates the file at rel under the project root with content.
func (f *fixture) write(t *testing.T, rel, content string) {
	t.Helper()
	path := filepath.Join(f.root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// project writes a root windsor.yaml, local values that select the docker platform, and an empty
// local template blueprint.
func (f *fixture) project(t *testing.T) {
	t.Helper()
	f.write(t, "windsor.yaml", "version: v1alpha1\n")
	f.write(t, "contexts/local/values.yaml", "platform: docker\n")
	f.write(t, "contexts/_template/blueprint.yaml", blueprintHeader+"metadata:\n  name: template\n")
}

// facet writes the template facet file facets/<name>.yaml with the facet header and body.
func (f *fixture) facet(t *testing.T, name, body string) {
	t.Helper()
	f.write(t, "contexts/_template/facets/"+name+".yaml", facetHeader+"metadata:\n  name: "+name+"\n"+body)
}

// load loads config for the local context and runs LoadBlueprint on a new handler.
func (f *fixture) load(t *testing.T, urls ...string) (*blueprint.BaseBlueprintHandler, error) {
	t.Helper()
	if err := f.config.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	h := blueprint.NewBlueprintHandler(f.runtime, f.artifact)
	return h, h.LoadBlueprint(urls...)
}

// publish makes files, keyed by path inside the artifact, available as the OCI artifact at url.
func (f *fixture) publish(t *testing.T, url string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(f.root, ".artifacts", fmt.Sprint(len(f.sources)))
	for rel, content := range files {
		f.write(t, filepath.Join(".artifacts", fmt.Sprint(len(f.sources)), rel), content)
	}
	f.sources[url] = dir
}

// =============================================================================
// Test Helpers
// =============================================================================

// parseRef splits an oci:// reference into registry, repository, and tag.
func parseRef(ref string) (string, string, string, error) {
	rest, ok := strings.CutPrefix(ref, "oci://")
	if !ok {
		return "", "", "", fmt.Errorf("not an oci reference: %s", ref)
	}
	registry, path, _ := strings.Cut(rest, "/")
	repository, tag, _ := strings.Cut(path, ":")
	return registry, repository, tag, nil
}

// mustLoad runs load and fails the test on error.
func (f *fixture) mustLoad(t *testing.T, urls ...string) *blueprint.BaseBlueprintHandler {
	t.Helper()
	h, err := f.load(t, urls...)
	if err != nil {
		t.Fatalf("LoadBlueprint: %v", err)
	}
	return h
}

// kustomization returns the kustomization called name in bp, or nil.
func kustomization(bp *blueprintv1alpha1.Blueprint, name string) *blueprintv1alpha1.Kustomization {
	for i := range bp.Kustomizations {
		if bp.Kustomizations[i].Name == name {
			return &bp.Kustomizations[i]
		}
	}
	return nil
}

// component returns the terraform component whose ID is id in bp, or nil.
func component(bp *blueprintv1alpha1.Blueprint, id string) *blueprintv1alpha1.TerraformComponent {
	for i := range bp.TerraformComponents {
		if bp.TerraformComponents[i].GetID() == id {
			return &bp.TerraformComponents[i]
		}
	}
	return nil
}
