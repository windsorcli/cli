package blueprint_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Test Setup
// =============================================================================

// usageFacet is a facet file with config keys and direct contributions. Each value has its own line.
const usageFacet = `kind: Facet
config:
  - name: images
    when: arch == 'amd64'
    value:
      tags:
        agent: v1
  - name: images
    when: arch == 'arm64'
    value:
      tags:
        agent: develop
  - name: talos
    value:
      image_tag: v1.13
      version: "${trimPrefix(talos.image_tag, 'v')}"
  - name: idle
    value:
      tag: v9
  - name: listed
    value:
      items:
        - v3
terraform:
  - path: cluster/talos
    inputs:
      image: direct:v1
  - path: cluster/off
    inputs:
      image: off:v1
kustomize:
  - name: app
    substitute:
      image: app:v1
flux:
  - name: csi
    install:
      substitutions:
        image: install:v1
    resources:
      - name: extra
        substitutions:
          image: extra:v1
substitutions:
  image: global:v1
`

// usageLine returns the 1-based line of the first line of usageFacet that contains text.
func usageLine(t *testing.T, text string) int {
	t.Helper()
	for i, line := range strings.Split(usageFacet, "\n") {
		if strings.Contains(line, text) {
			return i + 1
		}
	}
	t.Fatalf("usageFacet has no line with %q", text)
	return 0
}

// usageCollector returns a collector that loaded usageFacet, and the path of the facet. The amd64
// images block is active and the arm64 block is not.
func usageCollector(t *testing.T) (*blueprint.DefaultTraceCollector, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "facet.yaml")
	if err := os.WriteFile(path, []byte(usageFacet), 0o600); err != nil {
		t.Fatal(err)
	}
	c := blueprint.NewTraceCollector()
	c.RecordFacet(path)
	c.RecordConfigBlock("config.images", blueprint.ConfigBlockRecord{FacetPath: path, RawValue: map[string]any{"tags": map[string]any{"agent": "v1"}}})
	c.RecordConfigBlock("config.talos", blueprint.ConfigBlockRecord{FacetPath: path, RawValue: map[string]any{"image_tag": "v1.13", "version": "${trimPrefix(talos.image_tag, 'v')}"}})
	c.RecordConfigBlock("config.idle", blueprint.ConfigBlockRecord{FacetPath: path, RawValue: map[string]any{"tag": "v9"}})
	c.RecordConfigBlock("config.listed", blueprint.ConfigBlockRecord{FacetPath: path, RawValue: map[string]any{"items": []any{"v3"}}})
	return c, path
}

// usageBlueprint is a composed blueprint with one enabled and one disabled Terraform component.
func usageBlueprint() *blueprintv1alpha1.Blueprint {
	return &blueprintv1alpha1.Blueprint{
		TerraformComponents: []blueprintv1alpha1.TerraformComponent{
			{Path: "cluster/talos"},
			{Path: "cluster/off", Enabled: &blueprintv1alpha1.BoolExpression{Value: boolPtr(false)}},
		},
		Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "app"}},
		FluxSystems:    []blueprintv1alpha1.FluxSystem{{Name: "csi"}},
	}
}

// boolPtr returns a pointer to value.
func boolPtr(value bool) *bool {
	return &value
}

// contribute records a contribution of value from the facet at path.
func contribute(c *blueprint.DefaultTraceCollector, composedPath, path, value string) {
	c.RecordContribution(composedPath, blueprint.TraceContribution{FacetPath: path, RawValue: value})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDefaultTraceCollector_UnusedFacetValue(t *testing.T) {
	t.Run("KeepsAKeyThatAComposedInputReads", func(t *testing.T) {
		// Given an input that reads the parent map of a key through a function
		c, path := usageCollector(t)
		contribute(c, "terraform.cluster/talos.inputs.tag", path, "${get(images.tags, 'agent') ?? ''}")
		c.Finalize(usageBlueprint(), nil, "")

		// When the value of the active block is checked
		unused, note := c.UnusedFacetValue(path, usageLine(t, "agent: v1"))

		// Then the value is used
		if unused || note != "" {
			t.Errorf("expected a used value, got unused=%v note=%q", unused, note)
		}
	})

	t.Run("RemovesAValueOfABlockThatIsNotActive", func(t *testing.T) {
		// Given a reference to a block name that has an active and an inactive block
		c, path := usageCollector(t)
		contribute(c, "terraform.cluster/talos.inputs.tag", path, "${images.tags.agent}")
		c.Finalize(usageBlueprint(), nil, "")

		// When the value of the inactive block is checked
		unused, note := c.UnusedFacetValue(path, usageLine(t, "agent: develop"))

		// Then the value is unused
		if !unused || note != "" {
			t.Errorf("expected an unused value, got unused=%v note=%q", unused, note)
		}
	})

	t.Run("RemovesAKeyThatNoComposedValueReads", func(t *testing.T) {
		// Given a composed value that reads another block
		c, path := usageCollector(t)
		contribute(c, "terraform.cluster/talos.inputs.tag", path, "${images.tags.agent}")
		c.Finalize(usageBlueprint(), nil, "")

		// When the value of the unread block is checked
		unused, _ := c.UnusedFacetValue(path, usageLine(t, "tag: v9"))

		// Then the value is unused
		if !unused {
			t.Error("expected an unused value")
		}
	})

	t.Run("FollowsAReferenceThroughAnotherConfigKey", func(t *testing.T) {
		// Given an input that reads a key whose expression reads the tagged key
		c, path := usageCollector(t)
		contribute(c, "terraform.cluster/talos.inputs.version", path, "${talos.version}")
		c.Finalize(usageBlueprint(), nil, "")

		// When the tagged key is checked
		unused, note := c.UnusedFacetValue(path, usageLine(t, "image_tag: v1.13"))

		// Then the value is used
		if unused || note != "" {
			t.Errorf("expected a used value, got unused=%v note=%q", unused, note)
		}
	})

	t.Run("KeepsAKeyWhenALaterBlockSetsOtherKeys", func(t *testing.T) {
		// Given a later block with the same name that sets a different key
		c, path := usageCollector(t)
		c.RecordConfigBlock("config.images", blueprint.ConfigBlockRecord{FacetPath: "/other.yaml", RawValue: map[string]any{"tags": map[string]any{"other": "x"}}})
		contribute(c, "terraform.cluster/talos.inputs.tag", path, "${images.tags.agent}")
		c.Finalize(usageBlueprint(), nil, "")

		// When the key of the earlier block is checked
		unused, note := c.UnusedFacetValue(path, usageLine(t, "agent: v1"))

		// Then the value is used
		if unused || note != "" {
			t.Errorf("expected a used value, got unused=%v note=%q", unused, note)
		}
	})

	t.Run("RemovesAValueThatALaterActiveBlockOverrides", func(t *testing.T) {
		// Given two active blocks with the same name, and a final value from the later block
		c, path := usageCollector(t)
		c.RecordConfigBlock("config.images", blueprint.ConfigBlockRecord{FacetPath: path, RawValue: map[string]any{"tags": map[string]any{"agent": "develop"}}})
		contribute(c, "terraform.cluster/talos.inputs.tag", path, "${images.tags.agent}")
		c.Finalize(usageBlueprint(), map[string]any{"images": map[string]any{"tags": map[string]any{"agent": "develop"}}}, "")

		// When both values are checked
		overridden, _ := c.UnusedFacetValue(path, usageLine(t, "agent: v1"))
		final, _ := c.UnusedFacetValue(path, usageLine(t, "agent: develop"))

		// Then only the final value is used
		if !overridden || final {
			t.Errorf("expected v1 to be unused and develop to be used, got %v and %v", overridden, final)
		}
	})

	t.Run("KeepsAValueThatTheOperatorOverrides", func(t *testing.T) {
		// Given a final value that no block sets
		c, path := usageCollector(t)
		contribute(c, "terraform.cluster/talos.inputs.tag", path, "${images.tags.agent}")
		c.Finalize(usageBlueprint(), map[string]any{"images": map[string]any{"tags": map[string]any{"agent": "v7"}}}, "")

		// When the value of the block is checked
		unused, _ := c.UnusedFacetValue(path, usageLine(t, "agent: v1"))

		// Then the tag stays
		if unused {
			t.Error("expected a used value")
		}
	})

	t.Run("KeepsAKeyThatADeferredExpressionReads", func(t *testing.T) {
		// Given an input that combines a config key with a terraform output
		c, path := usageCollector(t)
		contribute(c, "terraform.cluster/talos.inputs.tag", path, "${images.tags.agent + terraform_output('cluster', 'id')}")
		c.Finalize(usageBlueprint(), nil, "")

		// When the key is checked
		unused, note := c.UnusedFacetValue(path, usageLine(t, "agent: v1"))

		// Then the value is used
		if unused || note != "" {
			t.Errorf("expected a used value, got unused=%v note=%q", unused, note)
		}
	})

	t.Run("KeepsAKeyThatABlueprintFileReads", func(t *testing.T) {
		// Given a blueprint file value that reads the key
		c, path := usageCollector(t)
		c.RecordBlueprintValue(map[string]any{"tag": "${images.tags.agent}"})
		c.Finalize(usageBlueprint(), nil, "")

		// When the key is checked
		unused, _ := c.UnusedFacetValue(path, usageLine(t, "agent: v1"))

		// Then the value is used
		if unused {
			t.Error("expected a used value")
		}
	})

	t.Run("IgnoresAReferenceOfADisabledComponent", func(t *testing.T) {
		// Given a disabled component that reads the key
		c, path := usageCollector(t)
		contribute(c, "terraform.cluster/off.inputs.tag", path, "${images.tags.agent}")
		c.Finalize(usageBlueprint(), nil, "")

		// When the key is checked
		unused, _ := c.UnusedFacetValue(path, usageLine(t, "agent: v1"))

		// Then the value is unused
		if !unused {
			t.Error("expected an unused value")
		}
	})

	t.Run("KeepsEachDirectContributionOfAnEnabledEntry", func(t *testing.T) {
		// Given a contribution for each kind of composed value
		c, path := usageCollector(t)
		cases := map[string]string{
			"direct:v1":  "terraform.cluster/talos.inputs.image",
			"app:v1":     "kustomize.app.substitutions.image",
			"install:v1": "flux.csi.install.substitutions.image",
			"extra:v1":   "flux.csi.resources-extra.substitutions.image",
			"global:v1":  "substitutions.image",
		}
		for value, composedPath := range cases {
			contribute(c, composedPath, path, value)
		}
		c.Finalize(usageBlueprint(), nil, "")

		for value := range cases {
			// When the line of the value is checked
			unused, note := c.UnusedFacetValue(path, usageLine(t, value))

			// Then the value is used
			if unused || note != "" {
				t.Errorf("expected %s to be used, got unused=%v note=%q", value, unused, note)
			}
		}
	})

	t.Run("RemovesADirectValueWithNoContribution", func(t *testing.T) {
		// Given a facet entry that composition did not include
		c, path := usageCollector(t)
		c.Finalize(usageBlueprint(), nil, "")

		// When its value is checked
		unused, note := c.UnusedFacetValue(path, usageLine(t, "extra:v1"))

		// Then the value is unused
		if !unused || note != "" {
			t.Errorf("expected an unused value, got unused=%v note=%q", unused, note)
		}
	})

	t.Run("KeepsEveryValueWhenAnExpressionCallsJsonnet", func(t *testing.T) {
		// Given a composed value that a Jsonnet file computes from the whole scope
		c, path := usageCollector(t)
		contribute(c, "terraform.cluster/talos.inputs.all", path, "${jsonnet('values.jsonnet')}")
		c.Finalize(usageBlueprint(), nil, "")

		// When an unread key is checked
		unused, note := c.UnusedFacetValue(path, usageLine(t, "tag: v9"))

		// Then the value stays and a note tells why
		if unused || !strings.Contains(note, "jsonnet") {
			t.Errorf("expected a kept value and a note, got unused=%v note=%q", unused, note)
		}
	})

	t.Run("KeepsEveryValueWhenAnExpressionDoesNotParse", func(t *testing.T) {
		// Given a composed value that does not parse
		c, path := usageCollector(t)
		contribute(c, "terraform.cluster/talos.inputs.bad", path, "${images +}")
		c.Finalize(usageBlueprint(), nil, "")

		// When an unread key is checked
		unused, note := c.UnusedFacetValue(path, usageLine(t, "tag: v9"))

		// Then the value stays and there is a note
		if unused || note == "" {
			t.Errorf("expected a kept value and a note, got unused=%v note=%q", unused, note)
		}
	})

	t.Run("KeepsAValueOnALineThatItCannotMap", func(t *testing.T) {
		// Given a value inside a list below a config key
		c, path := usageCollector(t)
		c.Finalize(usageBlueprint(), nil, "")

		// When the list item is checked
		unused, note := c.UnusedFacetValue(path, usageLine(t, "- v3"))

		// Then the value stays and there is a note
		if unused || !strings.Contains(note, "facet.yaml") {
			t.Errorf("expected a kept value and a note, got unused=%v note=%q", unused, note)
		}
	})

	t.Run("IgnoresAFileThatIsNotAFacet", func(t *testing.T) {
		// Given a file that composition did not load as a facet
		c, _ := usageCollector(t)
		c.Finalize(usageBlueprint(), nil, "")

		// When a line of the file is checked
		unused, note := c.UnusedFacetValue("/contexts/local/values.yaml", 1)

		// Then the value stays and there is no note
		if unused || note != "" {
			t.Errorf("expected a kept value and no note, got unused=%v note=%q", unused, note)
		}
	})
}
