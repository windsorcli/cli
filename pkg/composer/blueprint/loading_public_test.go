package blueprint_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestLoadBlueprint(t *testing.T) {
	t.Run("ComposesADefaultBlueprintWithoutATemplate", func(t *testing.T) {
		// Given a project with no template and no user blueprint
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha1\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then a default blueprint with no components is composed
		if bp.Metadata.Name != "default" || len(bp.Kustomizations) != 0 || len(bp.TerraformComponents) != 0 {
			t.Errorf("expected an empty default blueprint, got %+v", bp)
		}
	})

	t.Run("ComposesTheLocalTemplateWithoutAUserBlueprint", func(t *testing.T) {
		// Given a template and no user blueprint
		f := newFixture(t)
		f.project(t)

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the blueprint is named for the context and installs the template source
		if bp.Metadata.Name != "local" || bp.Metadata.Description != "Blueprint for the local context" {
			t.Errorf("unexpected metadata %+v", bp.Metadata)
		}
		if len(bp.Sources) != 1 || bp.Sources[0].Name != "template" || !bp.Sources[0].Install.IsInstalled() {
			t.Errorf("expected one installed template source, got %+v", bp.Sources)
		}
		if bp.ConfigMaps["values-common"]["CONTEXT"] != "local" {
			t.Errorf("expected values-common CONTEXT=local, got %v", bp.ConfigMaps)
		}
	})

	t.Run("EvaluatesExpressionsAgainstContextValues", func(t *testing.T) {
		// Given a template component whose input references the platform
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/_template/blueprint.yaml", blueprintHeader+"metadata:\n  name: template\nterraform:\n- path: network\n  inputs:\n    platform: ${platform}\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the input holds the context's platform
		if got := component(bp, "network"); got == nil || got.Inputs["platform"] != "docker" {
			t.Errorf("expected platform input docker, got %+v", got)
		}
	})

	t.Run("IncludesOnlyFacetsWhoseConditionHolds", func(t *testing.T) {
		// Given one facet for the docker platform and one for aws
		f := newFixture(t)
		f.project(t)
		f.facet(t, "docker", "when: platform == 'docker'\nkustomize:\n- name: on-docker\n  path: docker\n")
		f.facet(t, "aws", "when: platform == 'aws'\nkustomize:\n- name: on-aws\n  path: aws\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then only the docker facet contributes
		if kustomization(bp, "on-docker") == nil || kustomization(bp, "on-aws") != nil {
			t.Errorf("expected only on-docker, got %+v", bp.Kustomizations)
		}
	})

	t.Run("FailsWithARequirementsErrorWhenARequiredValueIsUnset", func(t *testing.T) {
		// Given an active facet that requires dns.domain, which the context does not set
		f := newFixture(t)
		f.project(t)
		f.facet(t, "dns", "requires:\n- paths: [dns.domain]\n  message: Set a DNS domain.\nkustomize:\n- name: dns\n  path: dns\n")

		// When the blueprint is loaded
		_, err := f.load(t)

		// Then a RequirementsError names the path and carries the facet's message
		var reqErr *blueprint.RequirementsError
		if !errors.As(err, &reqErr) {
			t.Fatalf("expected a RequirementsError, got %v", err)
		}
		if !strings.Contains(err.Error(), "dns.domain") || !strings.Contains(err.Error(), "Set a DNS domain.") {
			t.Errorf("expected the path and message in %q", err.Error())
		}
	})

	t.Run("LoadsWhenRequiredValuesAreSet", func(t *testing.T) {
		// Given a facet that requires dns.domain, which the context sets
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/local/values.yaml", "platform: docker\ndns:\n  domain: test\n")
		f.facet(t, "dns", "requires:\n- paths: [dns.domain]\nkustomize:\n- name: dns\n  path: dns\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the facet contributes
		if kustomization(bp, "dns") == nil {
			t.Errorf("expected the dns kustomization, got %+v", bp.Kustomizations)
		}
	})

	t.Run("ExposesFacetConfigBlocksToExpressions", func(t *testing.T) {
		// Given a facet with a config block that a substitution references
		f := newFixture(t)
		f.project(t)
		f.facet(t, "net", "config:\n- name: net\n  value:\n    cidr: 10.0.0.0/16\nkustomize:\n- name: k\n  path: k\n  substitutions:\n    cidr: ${net.cidr}\n")

		// When the blueprint is loaded
		h := f.mustLoad(t)

		// Then the substitution holds the block value, and the config scope holds the block
		if got := kustomization(h.Generate(), "k").Substitutions["cidr"]; got != "10.0.0.0/16" {
			t.Errorf("expected cidr 10.0.0.0/16, got %q", got)
		}
		if got := h.GetConfigScope()["net"]; !reflect.DeepEqual(got, map[string]any{"cidr": "10.0.0.0/16"}) {
			t.Errorf("expected the net block in the config scope, got %v", got)
		}
	})

	t.Run("MergesALaterFacetWithTheMergeStrategy", func(t *testing.T) {
		// Given an option facet that merges into a kustomization a base facet declares
		f := newFixture(t)
		f.project(t)
		f.facet(t, "base", "kustomize:\n- name: k\n  path: base\n  components: [x]\n")
		f.facet(t, "option-extra", "kustomize:\n- name: k\n  path: extra\n  strategy: merge\n  components: [y]\n")

		// When the blueprint is loaded
		got := kustomization(f.mustLoad(t).Generate(), "k")

		// Then the components union and the later path wins
		if got.Path != "extra" || !reflect.DeepEqual(got.Components, []string{"x", "y"}) {
			t.Errorf("expected path extra and components [x y], got %+v", got)
		}
	})

	t.Run("ReplacesAnEarlierComponentWithTheReplaceStrategy", func(t *testing.T) {
		// Given an option facet that replaces a terraform component a base facet declares
		f := newFixture(t)
		f.project(t)
		f.facet(t, "base", "terraform:\n- name: t\n  path: t\n  inputs:\n    a: 1\n")
		f.facet(t, "option-swap", "terraform:\n- name: t\n  path: t\n  strategy: replace\n  inputs:\n    b: 2\n")

		// When the blueprint is loaded
		got := component(f.mustLoad(t).Generate(), "t")

		// Then only the later inputs remain
		if !reflect.DeepEqual(got.Inputs, map[string]any{"b": uint64(2)}) {
			t.Errorf("expected inputs {b: 2}, got %#v", got.Inputs)
		}
	})

	t.Run("AppliesTheUserBlueprintLast", func(t *testing.T) {
		// Given a template kustomization and a user blueprint that extends it and adds one
		f := newFixture(t)
		f.project(t)
		f.facet(t, "base", "kustomize:\n- name: k\n  path: base\n  components: [x]\n")
		f.write(t, "contexts/local/blueprint.yaml", blueprintHeader+"metadata:\n  name: mine\nsources:\n- name: template\nkustomize:\n- name: k\n  components: [z]\n- name: extra\n  path: extra\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the user components merge in and the user kustomization is added
		if got := kustomization(bp, "k"); got == nil || got.Path != "base" || !reflect.DeepEqual(got.Components, []string{"x", "z"}) {
			t.Errorf("expected k with path base and components [x z], got %+v", got)
		}
		if kustomization(bp, "extra") == nil {
			t.Errorf("expected the user's extra kustomization, got %+v", bp.Kustomizations)
		}
	})

	t.Run("FillsValuesFromTheSchemaDefaults", func(t *testing.T) {
		// Given a template schema with a default and a substitution that reads it
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/_template/schema.yaml", "$schema: https://json-schema.org/draft/2020-12/schema\ntype: object\nproperties:\n  region:\n    type: string\n    default: us-east-1\n")
		f.facet(t, "k", "kustomize:\n- name: k\n  path: k\n  substitutions:\n    region: ${region}\n")

		// When the blueprint is loaded
		got := kustomization(f.mustLoad(t).Generate(), "k").Substitutions["region"]

		// Then the substitution holds the schema default
		if got != "us-east-1" {
			t.Errorf("expected us-east-1, got %q", got)
		}
	})

	t.Run("BuildsConfigMapsFromValuesSubstitutions", func(t *testing.T) {
		// Given values with common and per-kustomization substitutions
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/local/values.yaml", "platform: docker\nsubstitutions:\n  common:\n    domain: example.com\n  k:\n    replicas: 2\n")
		f.facet(t, "k", "kustomize:\n- name: k\n  path: k\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then common values join values-common, and per-kustomization values get their own map
		if bp.ConfigMaps["values-common"]["domain"] != "example.com" {
			t.Errorf("expected domain in values-common, got %v", bp.ConfigMaps["values-common"])
		}
		if bp.ConfigMaps["values-k"]["replicas"] != "2" || kustomization(bp, "k").Substitutions["replicas"] != "2" {
			t.Errorf("expected replicas in values-k and on k, got %v / %v", bp.ConfigMaps["values-k"], kustomization(bp, "k").Substitutions)
		}
	})

	t.Run("RejectsASensitiveValueInASubstitution", func(t *testing.T) {
		// Given a sensitive schema property that a substitution references
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/_template/schema.yaml", "$schema: https://json-schema.org/draft/2020-12/schema\ntype: object\nproperties:\n  token:\n    type: string\n    sensitive: true\n")
		f.write(t, "contexts/local/values.yaml", "platform: docker\ntoken: s3cr3t\n")
		f.facet(t, "leak", "kustomize:\n- name: leak\n  path: leak\n  substitutions:\n    t: ${token}\n")

		// When the blueprint is loaded
		_, err := f.load(t)

		// Then the load fails and names the sensitive property
		if err == nil || !strings.Contains(err.Error(), `references sensitive property "token"`) {
			t.Errorf("expected a sensitive-property error, got %v", err)
		}
	})

	t.Run("RejectsADependencyOnAnUnknownKustomization", func(t *testing.T) {
		// Given a kustomization that depends on one no source declares
		f := newFixture(t)
		f.project(t)
		f.facet(t, "k", "kustomize:\n- name: k\n  path: k\n  dependsOn: [missing]\n")

		// When the blueprint is loaded with validation skipped
		h := blueprint.NewBlueprintHandler(f.runtime, f.artifact)
		h.SetSkipValidation(true)
		_ = f.config.LoadConfig()
		err := h.LoadBlueprint()

		// Then composition still fails, because skipping validation covers only the backend check
		if err == nil || !strings.Contains(err.Error(), `depends on non-existent kustomization "missing"`) {
			t.Errorf("expected a dependency error, got %v", err)
		}
	})

	t.Run("RejectsABackendThatNamesNoComponent", func(t *testing.T) {
		// Given a template whose backend names a component that does not exist
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/_template/blueprint.yaml", blueprintHeader+"metadata:\n  name: template\nbackend: missing\n")

		// When the blueprint is loaded
		_, err := f.load(t)

		// Then the load fails as an invalid blueprint
		if !errors.Is(err, blueprint.ErrBlueprintInvalid) {
			t.Errorf("expected ErrBlueprintInvalid, got %v", err)
		}
	})

	t.Run("AcceptsAnInvalidBackendWhenValidationIsSkipped", func(t *testing.T) {
		// Given a template whose backend names a component that does not exist
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/_template/blueprint.yaml", blueprintHeader+"metadata:\n  name: template\nbackend: missing\n")
		if err := f.config.LoadConfig(); err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		h := blueprint.NewBlueprintHandler(f.runtime, f.artifact)
		h.SetSkipValidation(true)

		// When the blueprint is loaded
		err := h.LoadBlueprint()

		// Then the load succeeds
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("LoadsAnOCISourceAndTagsItsComponents", func(t *testing.T) {
		// Given a user blueprint that declares a published OCI source
		f := newFixture(t)
		f.project(t)
		f.publish(t, "oci://ghcr.io/acme/core:v1.0.0", map[string]string{
			"_template/blueprint.yaml":   blueprintHeader + "metadata:\n  name: core\n",
			"_template/facets/core.yaml": facetHeader + "metadata:\n  name: core\nkustomize:\n- name: from-core\n  path: core\n",
		})
		f.write(t, "contexts/local/blueprint.yaml", blueprintHeader+"metadata:\n  name: local\nsources:\n- name: core\n  url: oci://ghcr.io/acme/core:v1.0.0\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the source's kustomization is composed and names its source
		if got := kustomization(bp, "from-core"); got == nil || got.Source != "core" {
			t.Errorf("expected from-core with source core, got %+v", got)
		}
	})

	t.Run("FailsWhenAnOCISourceCannotBePulled", func(t *testing.T) {
		// Given a user blueprint that declares an OCI source nobody published
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/local/blueprint.yaml", blueprintHeader+"metadata:\n  name: local\nsources:\n- name: core\n  url: oci://ghcr.io/acme/core:v1.0.0\n")

		// When the blueprint is loaded
		_, err := f.load(t)

		// Then the load fails while loading sources
		if err == nil || !strings.Contains(err.Error(), "failed to load sources") {
			t.Errorf("expected a source load error, got %v", err)
		}
	})
}
