package inventory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	blueprintcomposer "github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/composer/inventory"
)

// =============================================================================
// Test Setup
// =============================================================================

const testModuleMain = `
variable "chart_repository" {
  type    = string
  default = "oci://ghcr.io/example/charts"
}

variable "operator_version" {
  type    = string
  default = "0.1.0"
}

variable "image_registry" {
  type    = string
  default = "ghcr.io/example"
}

variable "enable_extra" {
  type    = bool
  default = false
}

locals {
  components = concat(["core"], var.enable_extra ? ["extra"] : [])
  repository = "${var.image_registry}/operator"
}

resource "kubernetes_namespace_v1" "system" {
  metadata {
    name = "system"
  }
}

resource "helm_release" "operator" {
  repository = var.chart_repository
  chart      = "operator"
  name       = "operator"
  version    = var.operator_version
  namespace  = "system"
  values = [yamlencode({
    image      = { repository = local.repository }
    components = local.components
    namespace  = kubernetes_namespace_v1.system.metadata[0].name
  })]
  set = [{ name = "replicas", value = "2" }]
}

resource "helm_release" "ingress" {
  repository = "https://kubernetes.github.io/ingress-nginx"
  chart      = "ingress-nginx"
  name       = "ingress"
  version    = "4.15.1"
}

resource "helm_release" "skipped" {
  count      = 0
  repository = "oci://ghcr.io/example/charts"
  chart      = "skipped"
  version    = "1.0.0"
}

resource "kubernetes_job_v1" "migrate" {
  metadata {
    name = "migrate"
  }
  spec {
    template {
      metadata {}
      spec {
        init_container {
          image = "busybox:1.37"
        }
        container {
          image = "${var.image_registry}/migrate:1.0.0"
        }
      }
    }
  }
}

module "child" {
  source   = "./child"
  registry = var.image_registry
}
`

const testModuleChild = `
variable "registry" {
  type = string
}

resource "kubernetes_manifest" "agent" {
  manifest = {
    apiVersion = "apps/v1"
    kind       = "DaemonSet"
    metadata   = { name = "agent", namespace = "system" }
    spec = {
      template = {
        spec = {
          containers = [{ name = "agent", image = "${var.registry}/agent:2.0.0" }]
        }
      }
    }
  }
}
`

// writeModule writes files under <root>/terraform/<path>.
func writeModule(t *testing.T, root, path string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		file := filepath.Join(root, "terraform", path, name)
		if err := os.MkdirAll(filepath.Dir(file), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// terraformBlueprint returns a blueprint with one Terraform component and no Kustomizations.
func terraformBlueprint(inputs map[string]any) *blueprintv1alpha1.Blueprint {
	return &blueprintv1alpha1.Blueprint{
		Metadata:            blueprintv1alpha1.Metadata{Name: "local"},
		TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Name: "operator", Path: "apps/operator", Inputs: inputs}},
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseLister_List_Terraform(t *testing.T) {
	t.Run("ReadsChartsAndImagesFromATerraformModule", func(t *testing.T) {
		// Given a module with helm_release, workload and manifest resources, and an input that overrides a default
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		writeModule(t, m.runtime.ProjectRoot, "apps/operator", map[string]string{"main.tf": testModuleMain, "child/main.tf": testModuleChild})
		renderer := &fakeRenderer{out: map[string]string{"operator": `kind: Deployment
metadata: {name: operator, namespace: system}
spec: {template: {spec: {containers: [{image: "ghcr.io/example/operator:0.1.0"}]}}}`}}
		lister := newLister(m)
		lister.SetChartRenderer(renderer)

		// When List is called
		result, err := lister.List(terraformBlueprint(map[string]any{"image_registry": "registry.example.com", "chart_repository": nil}), inventory.Options{})

		// Then it lists both charts, the rendered, workload and manifest images, and skips the count = 0 release
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		operator := findEntry(result.Inventory.Artifacts, inventory.KindChart, "ghcr.io/example/charts/operator")
		if operator == nil || operator.Version != "0.1.0" || operator.Source.Object != "helm_release.operator" || operator.Source.File != "terraform/apps/operator/main.tf" {
			t.Errorf("unexpected operator chart: %+v", operator)
		}
		if ingress := findEntry(result.Inventory.Artifacts, inventory.KindChart, "https://kubernetes.github.io/ingress-nginx"); ingress == nil || ingress.Transport != inventory.TransportHTTP {
			t.Errorf("unexpected ingress chart: %+v", ingress)
		}
		if findEntry(result.Inventory.Artifacts, inventory.KindChart, "ghcr.io/example/charts/skipped") != nil {
			t.Error("expected no chart for a count = 0 release")
		}
		for _, ref := range []string{"ghcr.io/example/operator", "docker.io/library/busybox", "registry.example.com/migrate"} {
			if findEntry(result.Inventory.Artifacts, inventory.KindImage, ref) == nil {
				t.Errorf("expected image %s", ref)
			}
		}
		agent := findEntry(result.Inventory.Artifacts, inventory.KindImage, "registry.example.com/agent")
		if agent == nil || agent.Source.Object != "kubernetes_manifest.agent" || agent.Source.File != "terraform/apps/operator/child/main.tf" {
			t.Errorf("unexpected agent image: %+v", agent)
		}
		if len(m.fluxArgs) != 0 {
			t.Errorf("expected no flux call, got %v", m.fluxArgs)
		}
	})

	t.Run("RendersHelmReleaseValuesAndSetEntries", func(t *testing.T) {
		// Given a module whose helm_release values use a local, a variable and a resource attribute
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		writeModule(t, m.runtime.ProjectRoot, "apps/operator", map[string]string{"main.tf": testModuleMain, "child/main.tf": testModuleChild})
		renderer := &fakeRenderer{}
		lister := newLister(m)
		lister.SetChartRenderer(renderer)

		// When List is called
		_, err := lister.List(terraformBlueprint(map[string]any{"enable_extra": true}), inventory.Options{})

		// Then the request holds the known values, the set entry, and the marker for the resource attribute
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		var req *inventory.ChartRequest
		for i := range renderer.requests {
			if renderer.requests[i].Chart.Name == "operator" {
				req = &renderer.requests[i]
			}
		}
		if req == nil {
			t.Fatal("expected a render request for the operator chart")
		}
		values, _ := req.HelmRelease["spec"].(map[string]any)["values"].(map[string]any)
		image, _ := values["image"].(map[string]any)
		if image["repository"] != "ghcr.io/example/operator" {
			t.Errorf("unexpected image values %v", values)
		}
		if components, _ := values["components"].([]any); len(components) != 2 {
			t.Errorf("expected two components, got %v", values["components"])
		}
		if values["replicas"] != int64(2) {
			t.Errorf("expected replicas 2 from the set entry, got %#v", values["replicas"])
		}
		if !strings.Contains(values["namespace"].(string), "windsor-deferred-value") {
			t.Errorf("expected the deferred marker for a resource attribute, got %v", values["namespace"])
		}
		if md := req.HelmRelease["metadata"].(map[string]any); md["name"] != "operator" || md["namespace"] != "system" {
			t.Errorf("unexpected release metadata %v", md)
		}
	})

	t.Run("FailsWhenADeferredInputReachesAChartVersion", func(t *testing.T) {
		// Given a deferred input for the chart version
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		writeModule(t, m.runtime.ProjectRoot, "apps/operator", map[string]string{"main.tf": testModuleMain, "child/main.tf": testModuleChild})
		lister := newLister(m)

		// When List is called
		_, err := lister.List(terraformBlueprint(map[string]any{"operator_version": blueprintcomposer.DeferredPlaceholder}), inventory.Options{})

		// Then the error names the helm_release and its file
		if err == nil || !strings.Contains(err.Error(), "helm_release.operator in terraform/apps/operator/main.tf") {
			t.Fatalf("expected a deferred error, got %v", err)
		}
	})

	t.Run("FailsWhenADeferredInputReachesAnImage", func(t *testing.T) {
		// Given a deferred input for the image registry
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		writeModule(t, m.runtime.ProjectRoot, "apps/operator", map[string]string{"main.tf": testModuleMain, "child/main.tf": testModuleChild})
		lister := newLister(m)

		// When List is called
		_, err := lister.List(terraformBlueprint(map[string]any{"image_registry": blueprintcomposer.DeferredPlaceholder}), inventory.Options{})

		// Then the error names a Terraform file
		if err == nil || !strings.Contains(err.Error(), "not known before apply") || !strings.Contains(err.Error(), "terraform/apps/operator/") {
			t.Fatalf("expected a deferred error, got %v", err)
		}
	})

	t.Run("NotesComponentsAndModulesFromRemoteSources", func(t *testing.T) {
		// Given a component from a Git source and a module with a registry source
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		writeModule(t, m.runtime.ProjectRoot, "apps/vpc", map[string]string{"main.tf": `module "vpc" {
  source = "terraform-aws-modules/vpc/aws"
}`})
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "local"},
			Sources:  []blueprintv1alpha1.Source{{Name: "extras", Url: "https://github.com/example/extras.git"}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "git", Path: "apps/git", Source: "extras"},
				{Name: "vpc", Path: "apps/vpc"},
			},
		}
		lister := newLister(m)

		// When List is called
		result, err := lister.List(blueprint, inventory.Options{})

		// Then each gives a note
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		notes := strings.Join(result.Notes, "\n")
		if !strings.Contains(notes, `Terraform component "git" comes from extras`) || !strings.Contains(notes, "module.vpc in terraform/apps/vpc/main.tf has a remote source") {
			t.Errorf("unexpected notes %q", notes)
		}
	})

	t.Run("SkipsDisabledComponents", func(t *testing.T) {
		// Given a disabled component whose module directory does not exist
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		disabled := false
		blueprint := terraformBlueprint(nil)
		blueprint.TerraformComponents[0].Enabled = &blueprintv1alpha1.BoolExpression{Value: &disabled}
		lister := newLister(m)

		// When List is called
		result, err := lister.List(blueprint, inventory.Options{})

		// Then the component is not read
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(result.Inventory.Artifacts) != 0 {
			t.Errorf("expected no artifacts, got %+v", result.Inventory.Artifacts)
		}
	})

	t.Run("FailsForAMissingModuleDirectory", func(t *testing.T) {
		// Given an enabled component whose module directory does not exist
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		lister := newLister(m)

		// When List is called
		_, err := lister.List(terraformBlueprint(nil), inventory.Options{})

		// Then the error names the component
		if err == nil || !strings.Contains(err.Error(), `terraform component "operator"`) {
			t.Fatalf("expected a module directory error, got %v", err)
		}
	})

	t.Run("FailsForAModuleThatDoesNotParse", func(t *testing.T) {
		// Given a module with an HCL syntax error
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		writeModule(t, m.runtime.ProjectRoot, "apps/operator", map[string]string{"main.tf": `resource "helm_release" "x" {`})
		lister := newLister(m)

		// When List is called
		_, err := lister.List(terraformBlueprint(nil), inventory.Options{})

		// Then the error names the file
		if err == nil || !strings.Contains(err.Error(), "failed to parse") {
			t.Fatalf("expected a parse error, got %v", err)
		}
	})

	t.Run("EvaluatesFunctionsAndNestedInputs", func(t *testing.T) {
		// Given a version that functions derive from a nested input, an oci:// chart and a local chart
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		writeModule(t, m.runtime.ProjectRoot, "apps/operator", map[string]string{"main.tf": `
variable "cfg" {}

locals {
  first   = tostring(var.cfg.versions[0])
  checks  = alltrue([startswith(local.first, "1"), anytrue([false, true]), endswith("ab", "b"), var.cfg.enabled])
  decoded = yamldecode("tag: 1.2.3")
  digest  = base64decode(base64encode(sha256(basename("/a/x"))))
}

resource "helm_release" "derived" {
  chart   = "oci://ghcr.io/example/charts/derived"
  version = local.checks && length(local.digest) == 64 && length(var.cfg.empty) == 0 ? local.decoded.tag : "0.0.0"
}

resource "helm_release" "local" {
  chart = "./charts/local"
}
`})
		lister := newLister(m)
		inputs := map[string]any{"cfg": map[string]any{"versions": []any{"1.0", 2.5, 3, int64(4), uint64(5)}, "enabled": true, "empty": []any{}, "nested": map[string]any{}}}

		// When List is called
		result, err := lister.List(terraformBlueprint(inputs), inventory.Options{})

		// Then the oci:// chart takes the derived version and the local chart gives a note
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		derived := findEntry(result.Inventory.Artifacts, inventory.KindChart, "ghcr.io/example/charts/derived")
		if derived == nil || derived.Name != "derived" || derived.Version != "1.2.3" {
			t.Errorf("unexpected derived chart: %+v", derived)
		}
		if !strings.Contains(strings.Join(result.Notes, "\n"), `helm_release.local in terraform/apps/operator/main.tf uses chart "./charts/local"`) {
			t.Errorf("expected a local chart note, got %v", result.Notes)
		}
	})

	t.Run("ExpandsForEachAndCountWithSetEntries", func(t *testing.T) {
		// Given for_each and count releases, set entries with a number and a resource attribute, and Terraform function semantics
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		writeModule(t, m.runtime.ProjectRoot, "apps/operator", map[string]string{"main.tf": `
variable "releases" {
  default = { alpha = "1.0.0", beta = "2.0.0" }
}

variable "override" {
  default = ""
}

resource "aws_ecr_repository" "app" {}

resource "helm_release" "app" {
  for_each   = var.releases
  repository = "oci://ghcr.io/example/charts"
  chart      = each.key
  name       = each.key
  version    = each.value
  set {
    name  = "replicas"
    value = 3
  }
  set {
    name  = "image.repository"
    value = aws_ecr_repository.app.repository_url
  }
}

resource "helm_release" "counted" {
  count      = 2
  repository = "oci://ghcr.io/example/charts"
  chart      = "counted"
  name       = "counted-${count.index}"
  version    = coalesce(var.override, replace("v3.0.0", "/^v/", ""))
}
`})
		renderer := &fakeRenderer{}
		lister := newLister(m)
		lister.SetChartRenderer(renderer)

		// When List is called
		result, err := lister.List(terraformBlueprint(nil), inventory.Options{})

		// Then each instance renders, and the set entries carry a number and the marker
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		beta := findEntry(result.Inventory.Artifacts, inventory.KindChart, "ghcr.io/example/charts/beta")
		if beta == nil || beta.Version != "2.0.0" || beta.Source.Object != `helm_release.app["beta"]` {
			t.Errorf("unexpected beta chart: %+v", beta)
		}
		if counted := findEntry(result.Inventory.Artifacts, inventory.KindChart, "ghcr.io/example/charts/counted"); counted == nil || counted.Version != "3.0.0" {
			t.Errorf("unexpected counted chart: %+v", counted)
		}
		if len(renderer.requests) != 4 {
			t.Fatalf("expected 4 render requests, got %d", len(renderer.requests))
		}
		names := make(map[string]bool)
		for _, req := range renderer.requests {
			names[req.HelmRelease["metadata"].(map[string]any)["name"].(string)] = true
		}
		if !names["alpha"] || !names["counted-1"] {
			t.Errorf("unexpected release names %v", names)
		}
		values := renderer.requests[0].HelmRelease["spec"].(map[string]any)["values"].(map[string]any)
		image, _ := values["image"].(map[string]any)
		if values["replicas"] != int64(3) || image["repository"] != "windsor-deferred-value" {
			t.Errorf("unexpected set values %v", values)
		}
	})

	t.Run("ReadsTheModuleUnderTheSourcePathPrefix", func(t *testing.T) {
		// Given a project source whose pathPrefix is infra
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		file := filepath.Join(m.runtime.ProjectRoot, "infra", "apps", "operator", "main.tf")
		if err := os.MkdirAll(filepath.Dir(file), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(`resource "helm_release" "x" {
  chart   = "oci://ghcr.io/example/charts/x"
  version = "1.0.0"
}`), 0600); err != nil {
			t.Fatal(err)
		}
		blueprint := terraformBlueprint(nil)
		blueprint.Sources = []blueprintv1alpha1.Source{{Name: "local", PathPrefix: "infra"}}
		lister := newLister(m)

		// When List is called
		result, err := lister.List(blueprint, inventory.Options{})

		// Then the chart comes from infra/apps/operator
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if x := findEntry(result.Inventory.Artifacts, inventory.KindChart, "ghcr.io/example/charts/x"); x == nil || x.Source.File != "infra/apps/operator/main.tf" {
			t.Errorf("unexpected chart: %+v", x)
		}
	})

	t.Run("FailsForAValuesDocumentThatDoesNotParse", func(t *testing.T) {
		// Given a helm_release whose values document is not YAML
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		writeModule(t, m.runtime.ProjectRoot, "apps/operator", map[string]string{"main.tf": `resource "helm_release" "x" {
  chart   = "oci://ghcr.io/example/charts/x"
  version = "1.0.0"
  values  = ["a: [b"]
}`})
		lister := newLister(m)

		// When List is called
		_, err := lister.List(terraformBlueprint(nil), inventory.Options{})

		// Then the error names the helm_release
		if err == nil || !strings.Contains(err.Error(), "failed to read the values of helm_release.x") {
			t.Fatalf("expected a values error, got %v", err)
		}
	})
}
