package blueprint_test

import (
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Test Setup
// =============================================================================

// scopeBlock returns a config block with name and a map value.
func scopeBlock(name string, value map[string]any) blueprintv1alpha1.ConfigBlock {
	return blueprintv1alpha1.ConfigBlock{Name: name, Body: map[string]any{"value": value}}
}

// lbFacet returns a core facet that writes the config block lb_effective.
func lbFacet() blueprintv1alpha1.Facet {
	return blueprintv1alpha1.Facet{
		Metadata: blueprintv1alpha1.Metadata{Name: "platform-base"},
		Config:   []blueprintv1alpha1.ConfigBlock{scopeBlock("lb_effective", map[string]any{"mode": "nodeport"})},
	}
}

// composeScopeSources runs ProcessGlobally over a core source and a home source that layers on core.
// schemaNames are the top-level schema properties.
func composeScopeSources(t *testing.T, core, home blueprint.SourceFacetSet, schemaNames ...string) error {
	t.Helper()
	mocks := setupProcessorMocks(t)
	props := make(map[string]any, len(schemaNames))
	for _, name := range schemaNames {
		props[name] = map[string]any{}
	}
	mocks.ConfigHandler.GetSchemaFunc = func() map[string]any {
		return map[string]any{"properties": props}
	}
	mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
		return map[string]any{}, nil
	}
	processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
	core.Name, core.Depth = "core", 0
	home.Name, home.Depth = "home", 1
	if core.Target == nil {
		core.Target = &blueprintv1alpha1.Blueprint{}
	}
	if home.Target == nil {
		home.Target = &blueprintv1alpha1.Blueprint{}
	}
	_, err := processor.ProcessGlobally([]blueprint.SourceFacetSet{core, home})
	return err
}

// assertErrorContains fails the test unless err is not nil and holds every part.
func assertErrorContains(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error that holds %q, got nil", parts)
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("expected the error to hold %q, got %v", part, err)
		}
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProcessor_ProcessGlobally_SourceScope(t *testing.T) {
	t.Run("ConsumerCannotReadSourceInternal", func(t *testing.T) {
		// Given a source "core" whose facet writes the internal block lb_effective
		mocks := setupProcessorMocks(t)
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
		homeTarget := &blueprintv1alpha1.Blueprint{}
		sources := []blueprint.SourceFacetSet{
			{Name: "core", Depth: 0, Target: &blueprintv1alpha1.Blueprint{}, Facets: []blueprintv1alpha1.Facet{lbFacet()}},
			// And a consumer "home" whose facet reads lb_effective
			{Name: "home", Depth: 1, Target: homeTarget, Facets: []blueprintv1alpha1.Facet{{
				Metadata:      blueprintv1alpha1.Metadata{Name: "app-frigate"},
				Substitutions: map[string]string{"lb_mode": "${lb_effective.mode}"},
			}}},
		}

		// When the processor composes both sources
		_, err := processor.ProcessGlobally(sources)

		// Then composition fails, names the variable and its owner, and points at exports
		if err == nil {
			t.Fatalf("expected an error, got nil; home read lb_effective.mode as %q", homeTarget.Substitutions["lb_mode"])
		}
		assertErrorContains(t, err, `facet "app-frigate" in blueprint "home" reads config block "lb_effective", which blueprint "core" owns`, "exports:")
	})

	t.Run("ConsumerCannotReadSourceInternalInWhen", func(t *testing.T) {
		// Given a consumer facet whose when reads a core internal
		home := blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "app-frigate"},
			When:     "lb_effective.mode == 'nodeport'",
		}}}

		// When both sources compose
		err := composeScopeSources(t, blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{lbFacet()}}, home)

		// Then composition fails and names the variable
		assertErrorContains(t, err, `reads config block "lb_effective"`)
	})

	t.Run("ConsumerCannotReadSourceInternalInTerraformInputs", func(t *testing.T) {
		// Given a consumer facet whose terraform input reads a core internal
		home := blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "app-frigate"},
			TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{{
				TerraformComponent: blueprintv1alpha1.TerraformComponent{
					Path:   "frigate",
					Inputs: map[string]any{"lb_mode": "${lb_effective.mode}"},
				},
			}},
		}}}

		// When both sources compose
		err := composeScopeSources(t, blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{lbFacet()}}, home)

		// Then composition fails and names the variable
		assertErrorContains(t, err, `reads config block "lb_effective"`)
	})

	t.Run("ConsumerCannotRequireSourceInternal", func(t *testing.T) {
		// Given a consumer facet whose requires paths name a core internal
		home := blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "app-frigate"},
			Requires: []blueprintv1alpha1.RequirementBlock{{Paths: []string{"lb_effective.mode"}}},
		}}}

		// When both sources compose
		err := composeScopeSources(t, blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{lbFacet()}}, home)

		// Then composition fails and names the variable
		assertErrorContains(t, err, `reads config block "lb_effective"`)
	})

	t.Run("SourceCannotReadConsumerInternal", func(t *testing.T) {
		// Given a consumer facet that writes an internal block, and a core facet that reads it
		core := blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{{
			Metadata:      blueprintv1alpha1.Metadata{Name: "platform-base"},
			Substitutions: map[string]string{"app": "${app_effective.name ?? ''}"},
		}}}
		home := blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "app-frigate"},
			Config:   []blueprintv1alpha1.ConfigBlock{scopeBlock("app_effective", map[string]any{"name": "frigate"})},
		}}}

		// When both sources compose
		err := composeScopeSources(t, core, home)

		// Then composition fails, because the scope rule works in both directions
		assertErrorContains(t, err, `facet "platform-base" in blueprint "core" reads config block "app_effective", which blueprint "home" owns`)
	})

	t.Run("FacetsReadInternalsOfTheirOwnBlueprint", func(t *testing.T) {
		// Given two core facets, where one writes lb_effective and the other reads it
		coreTarget := &blueprintv1alpha1.Blueprint{}
		core := blueprint.SourceFacetSet{Target: coreTarget, Facets: []blueprintv1alpha1.Facet{
			lbFacet(),
			{
				Metadata:      blueprintv1alpha1.Metadata{Name: "option-gateway"},
				When:          "lb_effective.mode == 'nodeport'",
				Substitutions: map[string]string{"lb_mode": "${lb_effective.mode}"},
			},
		}}

		// When core composes with a consumer that does not touch lb_effective
		err := composeScopeSources(t, core, blueprint.SourceFacetSet{})

		// Then composition succeeds and the core facet reads its own internal
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got := coreTarget.Substitutions["lb_mode"]; got != "nodeport" {
			t.Errorf("expected lb_mode nodeport, got %q", got)
		}
	})

	t.Run("SchemaValueIsReadableAndWritableAcrossBlueprints", func(t *testing.T) {
		// Given a core facet that writes the schema value dns and reads it in a substitution
		coreTarget := &blueprintv1alpha1.Blueprint{}
		homeTarget := &blueprintv1alpha1.Blueprint{}
		core := blueprint.SourceFacetSet{Target: coreTarget, Facets: []blueprintv1alpha1.Facet{{
			Metadata:      blueprintv1alpha1.Metadata{Name: "platform-base"},
			Config:        []blueprintv1alpha1.ConfigBlock{scopeBlock("dns", map[string]any{"domain": "core"})},
			Substitutions: map[string]string{"domain": "${dns.domain}"},
		}}}
		// And a consumer facet that writes dns too and reads it
		home := blueprint.SourceFacetSet{Target: homeTarget, Facets: []blueprintv1alpha1.Facet{{
			Metadata:      blueprintv1alpha1.Metadata{Name: "app-frigate"},
			Config:        []blueprintv1alpha1.ConfigBlock{scopeBlock("dns", map[string]any{"domain": "home"})},
			Substitutions: map[string]string{"domain": "${dns.domain}"},
		}}}

		// When both sources compose with dns in the schema
		err := composeScopeSources(t, core, home, "dns")

		// Then composition succeeds and both sources read the consumer's value
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if coreTarget.Substitutions["domain"] != "home" || homeTarget.Substitutions["domain"] != "home" {
			t.Errorf("expected both domains home, got core %q and home %q", coreTarget.Substitutions["domain"], homeTarget.Substitutions["domain"])
		}
	})

	t.Run("ConsumerReadsExportedBlock", func(t *testing.T) {
		// Given a core source that exports lb_effective, and a consumer facet that reads it
		homeTarget := &blueprintv1alpha1.Blueprint{}
		core := blueprint.SourceFacetSet{Exports: []string{"lb_effective"}, Facets: []blueprintv1alpha1.Facet{lbFacet()}}
		home := blueprint.SourceFacetSet{Target: homeTarget, Facets: []blueprintv1alpha1.Facet{{
			Metadata:      blueprintv1alpha1.Metadata{Name: "app-frigate"},
			Substitutions: map[string]string{"lb_mode": "${lb_effective.mode}"},
		}}}

		// When both sources compose
		err := composeScopeSources(t, core, home)

		// Then composition succeeds and the consumer reads the exported value
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got := homeTarget.Substitutions["lb_mode"]; got != "nodeport" {
			t.Errorf("expected lb_mode nodeport, got %q", got)
		}
	})

	t.Run("ConsumerCannotWriteExportedBlock", func(t *testing.T) {
		// Given a core source that exports lb_effective, and a consumer facet that writes it
		core := blueprint.SourceFacetSet{Exports: []string{"lb_effective"}, Facets: []blueprintv1alpha1.Facet{lbFacet()}}
		home := blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "app-frigate"},
			Config:   []blueprintv1alpha1.ConfigBlock{scopeBlock("lb_effective", map[string]any{"mode": "loadbalancer"})},
		}}}

		// When both sources compose
		err := composeScopeSources(t, core, home)

		// Then composition fails and names the writer and the owner
		assertErrorContains(t, err, `facet "app-frigate" in blueprint "home" writes config block "lb_effective", which blueprint "core" owns`)
	})

	t.Run("TwoBlueprintsCannotDefineTheSameInternal", func(t *testing.T) {
		// Given a core facet and a consumer facet that both write lb_effective, and neither exports it
		home := blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "app-frigate"},
			Config:   []blueprintv1alpha1.ConfigBlock{scopeBlock("lb_effective", map[string]any{"mode": "loadbalancer"})},
		}}}

		// When both sources compose
		err := composeScopeSources(t, blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{lbFacet()}}, home)

		// Then composition fails and names both blueprints
		assertErrorContains(t, err, `blueprints "core" and "home" define the same config block "lb_effective"`)
	})

	t.Run("ExportMustNameADefinedBlock", func(t *testing.T) {
		// Given a core source that exports a block that none of its facets define
		core := blueprint.SourceFacetSet{Exports: []string{"lb_efective"}, Facets: []blueprintv1alpha1.Facet{lbFacet()}}

		// When both sources compose
		err := composeScopeSources(t, core, blueprint.SourceFacetSet{})

		// Then composition fails and names the export
		assertErrorContains(t, err, `blueprint "core" exports "lb_efective", but none of its facets define that config block`)
	})

	t.Run("NamesBlueprintsByTheirMetadataName", func(t *testing.T) {
		// Given a consumer source whose blueprint is named manager, and whose facet reads a core internal
		home := blueprint.SourceFacetSet{
			Target: &blueprintv1alpha1.Blueprint{Metadata: blueprintv1alpha1.Metadata{Name: "manager"}},
			Facets: []blueprintv1alpha1.Facet{{
				Metadata:      blueprintv1alpha1.Metadata{Name: "addon-registry"},
				Substitutions: map[string]string{"a": "${lb_effective.mode}"},
			}},
		}

		// When both sources compose
		err := composeScopeSources(t, blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{lbFacet()}}, home)

		// Then the error names the consumer by its blueprint name, not its source name
		assertErrorContains(t, err, `facet "addon-registry" in blueprint "manager" reads`)
	})

	t.Run("ExportOfAnotherBlueprintsBlockDoesNotMoveOwnership", func(t *testing.T) {
		// Given a consumer that exports lb_effective, which only core defines
		coreTarget := &blueprintv1alpha1.Blueprint{}
		core := blueprint.SourceFacetSet{Target: coreTarget, Facets: []blueprintv1alpha1.Facet{
			lbFacet(),
			{Metadata: blueprintv1alpha1.Metadata{Name: "option-gateway"}, Substitutions: map[string]string{"lb_mode": "${lb_effective.mode}"}},
		}}
		home := blueprint.SourceFacetSet{Exports: []string{"lb_effective"}}

		// When both sources compose
		err := composeScopeSources(t, core, home)

		// Then only the bad export fails, and core keeps its own block
		assertErrorContains(t, err, `blueprint "home" exports "lb_effective", but none of its facets define that config block`)
		if strings.Contains(err.Error(), `"core" writes`) || strings.Contains(err.Error(), `"core" reads`) {
			t.Errorf("expected no error for core's own block, got %v", err)
		}
	})

	t.Run("ReportsEveryViolationInOneError", func(t *testing.T) {
		// Given two consumer facets that each read a core internal
		home := blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{
			{Metadata: blueprintv1alpha1.Metadata{Name: "app-frigate"}, Substitutions: map[string]string{"a": "${lb_effective.mode}"}},
			{Metadata: blueprintv1alpha1.Metadata{Name: "app-immich"}, Substitutions: map[string]string{"a": "${lb_effective.mode}"}},
		}}

		// When both sources compose
		err := composeScopeSources(t, blueprint.SourceFacetSet{Facets: []blueprintv1alpha1.Facet{lbFacet()}}, home)

		// Then the one error names both facets
		assertErrorContains(t, err, `facet "app-frigate"`, `facet "app-immich"`)
	})
}
