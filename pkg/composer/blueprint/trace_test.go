package blueprint_test

import (
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewTraceCollector(t *testing.T) {
	t.Run("ReturnsInitializedCollector", func(t *testing.T) {
		// When creating a new trace collector
		c := blueprint.NewTraceCollector()

		// Then the collector should be non-nil
		if c == nil {
			t.Fatal("expected non-nil collector")
		}
		if got := c.Contributions("terraform.x.inputs.k"); len(got) != 0 {
			t.Errorf("expected no contributions, got %d", len(got))
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDefaultTraceCollector_RecordContribution(t *testing.T) {
	t.Run("StoresContribution", func(t *testing.T) {
		// Given a trace collector
		c := blueprint.NewTraceCollector()

		// When recording a contribution
		c.RecordContribution("terraform.x.inputs.domain", blueprint.TraceContribution{
			FacetPath:  "/f.yaml",
			SourceName: "template",
			Ordinal:    100,
			Strategy:   "merge",
			Line:       5,
			RawValue:   "example.com",
		})

		// Then the contribution is stored
		records := c.Contributions("terraform.x.inputs.domain")
		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}
		if records[0].SourceName != "template" {
			t.Errorf("expected source %q, got %q", "template", records[0].SourceName)
		}
	})

	t.Run("AppendsMultipleContributions", func(t *testing.T) {
		// Given a trace collector with an existing contribution
		c := blueprint.NewTraceCollector()
		c.RecordContribution("terraform.x.inputs.k", blueprint.TraceContribution{Ordinal: 100})

		// When recording a second contribution for the same path
		c.RecordContribution("terraform.x.inputs.k", blueprint.TraceContribution{Ordinal: 200})

		// Then both contributions are stored
		if len(c.Contributions("terraform.x.inputs.k")) != 2 {
			t.Errorf("expected 2 records, got %d", len(c.Contributions("terraform.x.inputs.k")))
		}
	})
}

func TestDefaultTraceCollector_RecordConfigBlock(t *testing.T) {
	t.Run("StoresConfigBlock", func(t *testing.T) {
		// Given a trace collector
		c := blueprint.NewTraceCollector()

		// When recording a config block
		c.RecordConfigBlock("config.cluster.endpoint", blueprint.ConfigBlockRecord{
			FacetPath: "/f.yaml",
			Line:      10,
			RawValue:  "${terraform_output(\"compute\")}",
		})

		// Then the config block is stored
		records := c.ConfigBlocks("config.cluster.endpoint")
		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}
		if records[0].Line != 10 {
			t.Errorf("expected line 10, got %d", records[0].Line)
		}
	})

	t.Run("PopulatesNestedRefsForMapValue", func(t *testing.T) {
		// Given a trace collector
		c := blueprint.NewTraceCollector()

		// When recording a config block with a map value containing expressions
		c.RecordConfigBlock("config.my_patch", blueprint.ConfigBlockRecord{
			FacetPath: "/f.yaml",
			Line:      5,
			RawValue: map[string]any{
				"machine": map[string]any{
					"network": "${net.driver}",
				},
				"cluster": map[string]any{
					"sans": "${cert.list}",
				},
			},
		})

		// Then NestedPaths and NestedRefs are populated on the record
		records := c.ConfigBlocks("config.my_patch")
		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}
		rec := records[0]
		if len(rec.NestedPaths) != 2 {
			t.Fatalf("expected 2 nested paths, got %d: %v", len(rec.NestedPaths), rec.NestedPaths)
		}
		if rec.NestedRefs == nil {
			t.Fatal("expected NestedRefs to be populated")
		}
		netRefs := rec.NestedRefs["machine.network"]
		if len(netRefs) != 1 || netRefs[0] != "net.driver" {
			t.Errorf("expected NestedRefs[machine.network] = [net.driver], got %v", netRefs)
		}
		sansRefs := rec.NestedRefs["cluster.sans"]
		if len(sansRefs) != 1 || sansRefs[0] != "cert.list" {
			t.Errorf("expected NestedRefs[cluster.sans] = [cert.list], got %v", sansRefs)
		}

		// And no child config block entries are created
		if len(c.ConfigBlocks("config.my_patch.machine.network")) != 0 {
			t.Error("expected no child entries for config.my_patch.machine.network")
		}
		if len(c.ConfigBlocks("config.my_patch.cluster.sans")) != 0 {
			t.Error("expected no child entries for config.my_patch.cluster.sans")
		}
	})
}

func TestDefaultTraceCollector_Finalize(t *testing.T) {
	t.Run("StoresBlueprintAndScope", func(t *testing.T) {
		// Given a trace collector
		c := blueprint.NewTraceCollector()
		bp := &blueprintv1alpha1.Blueprint{}
		scope := map[string]any{"key": "val"}

		// When finalizing
		c.Finalize(bp, scope, "/template/root")

		// Then GetTrace queries the stored blueprint
		_, err := c.GetTrace("terraform.x.inputs.k")
		if err == nil || strings.Contains(err.Error(), "not composed") {
			t.Errorf("expected a lookup error from the stored blueprint, got %v", err)
		}
	})
}

func TestDefaultTraceCollector_SettleEntry(t *testing.T) {
	const path = "substitutions.key"
	composed := &blueprintv1alpha1.Blueprint{Substitutions: map[string]string{"key": "value"}}

	setup := func(t *testing.T) *blueprint.DefaultTraceCollector {
		t.Helper()
		c := blueprint.NewTraceCollector()
		c.RecordContribution(path, blueprint.TraceContribution{FacetPath: "/old.yaml", Ordinal: 100, Strategy: "merge", RawValue: "old", Entry: "entry"})
		c.SettleEntry("entry", true, true)
		c.RecordContribution(path, blueprint.TraceContribution{FacetPath: "/new.yaml", Ordinal: 200, Strategy: "merge", RawValue: "new", Entry: "entry"})
		c.Finalize(composed, nil, "")
		return c
	}

	effectiveFacet := func(t *testing.T, c *blueprint.DefaultTraceCollector) string {
		t.Helper()
		trace, err := c.GetTrace(path)
		if err != nil {
			t.Fatalf("GetTrace: %v", err)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatalf("expected an effective contribution, got %+v", trace.Contributions)
		}
		return effective.AbsFacetPath
	}

	t.Run("KeepsBothGroups", func(t *testing.T) {
		// Given contributions from two merges of one entry
		c := setup(t)

		// When the merge keeps both groups
		c.SettleEntry("entry", true, true)

		// Then the new contribution is effective
		if got := effectiveFacet(t, c); got != "/new.yaml" {
			t.Errorf("expected /new.yaml, got %q", got)
		}
	})

	t.Run("DiscardsNewContributions", func(t *testing.T) {
		// Given contributions from two merges of one entry
		c := setup(t)

		// When the merge drops the new contribution
		c.SettleEntry("entry", true, false)

		// Then the existing contribution is effective
		if got := effectiveFacet(t, c); got != "/old.yaml" {
			t.Errorf("expected /old.yaml, got %q", got)
		}
	})

	t.Run("DiscardsExistingContributions", func(t *testing.T) {
		// Given an existing contribution at a higher ordinal than the new contribution
		c := blueprint.NewTraceCollector()
		c.RecordContribution(path, blueprint.TraceContribution{FacetPath: "/old.yaml", Ordinal: 200, Strategy: "merge", RawValue: "old", Entry: "entry"})
		c.SettleEntry("entry", true, true)
		c.RecordContribution(path, blueprint.TraceContribution{FacetPath: "/new.yaml", Ordinal: 100, Strategy: "merge", RawValue: "new", Entry: "entry"})
		c.Finalize(composed, nil, "")

		// When the merge drops the existing contribution
		c.SettleEntry("entry", false, true)

		// Then the new contribution is effective
		if got := effectiveFacet(t, c); got != "/new.yaml" {
			t.Errorf("expected /new.yaml, got %q", got)
		}
	})

	t.Run("IgnoresOtherEntries", func(t *testing.T) {
		// Given contributions from two merges of one entry
		c := setup(t)

		// When another entry settles and drops all of its contributions
		c.SettleEntry("other", false, false)

		// Then the contributions of the first entry are not changed
		if got := effectiveFacet(t, c); got != "/new.yaml" {
			t.Errorf("expected /new.yaml, got %q", got)
		}
	})

	t.Run("NamesComposedBlueprintWhenAllAreDiscarded", func(t *testing.T) {
		// Given contributions from two merges of one entry
		c := setup(t)

		// When the merge drops the entry
		c.SettleEntry("entry", false, false)

		// Then the composed blueprint is the effective source
		trace, err := c.GetTrace(path)
		if err != nil {
			t.Fatalf("GetTrace: %v", err)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil || effective.SourceName != "composed blueprint" {
			t.Errorf("expected the composed blueprint as the source, got %+v", effective)
		}
	})
}

func TestDefaultTraceCollector_GetTrace_MergeAttribution(t *testing.T) {
	for _, kind := range attributionKinds() {
		t.Run(kind.name, func(t *testing.T) {
			t.Run("ReplaceDiscardsKeyThatItOmits", func(t *testing.T) {
				// Given a facet that sets a key and a higher facet that replaces the entry without the key
				tc := composeAttribution(t, &blueprintv1alpha1.Blueprint{}, []blueprintv1alpha1.Facet{
					kind.facet(attributionSpec{name: "low", ordinal: 100, values: map[string]string{"gone": "low"}}),
					kind.facet(attributionSpec{name: "high", ordinal: 200, strategy: "replace", values: map[string]string{"kept": "high"}}),
				})

				// When tracing the omitted key
				trace, err := tc.GetTrace(kind.path("gone"))

				// Then no facet is named for the key
				if kind.absentKeyErrors {
					if err == nil {
						t.Fatalf("expected an error for the absent key, got %+v", trace)
					}
					return
				}
				if err != nil {
					t.Fatalf("GetTrace: %v", err)
				}
				if effective := findEffective(trace.Contributions); effective != nil {
					t.Errorf("expected no effective source, got %+v", effective)
				}
				if len(trace.Contributions) != 1 || !trace.Contributions[0].Discarded {
					t.Errorf("expected one discarded contribution, got %+v", trace.Contributions)
				}
			})

			t.Run("ReplaceAtEqualOrdinalBeatsLaterMerge", func(t *testing.T) {
				// Given two facets at one ordinal, where the first replaces and the second merges
				tc := composeAttribution(t, &blueprintv1alpha1.Blueprint{}, []blueprintv1alpha1.Facet{
					kind.facet(attributionSpec{name: "a-replace", ordinal: 100, strategy: "replace", values: map[string]string{"key": "replace"}}),
					kind.facet(attributionSpec{name: "b-merge", ordinal: 100, values: map[string]string{"key": "merge"}}),
				})

				// When tracing the key
				trace, err := tc.GetTrace(kind.path("key"))

				// Then the trace names the replace facet
				if err != nil {
					t.Fatalf("GetTrace: %v", err)
				}
				if trace.Value != "replace" {
					t.Errorf("expected value 'replace', got %q", trace.Value)
				}
				assertAttributedTo(t, trace, "a-replace")
			})

			t.Run("RemoveAtHigherOrdinalDiscardsLowerEntry", func(t *testing.T) {
				// Given a blueprint value, a facet that sets it, and a higher facet that removes another key
				tc := composeAttribution(t, kind.target(map[string]string{"key": "file"}), []blueprintv1alpha1.Facet{
					kind.facet(attributionSpec{name: "low", ordinal: 100, values: map[string]string{"key": "low"}}),
					kind.facet(attributionSpec{name: "high", ordinal: 200, strategy: "remove", values: map[string]string{"other": ""}}),
				})

				// When tracing the key
				trace, err := tc.GetTrace(kind.path("key"))

				// Then the trace names the composed blueprint and marks the facet as discarded
				if err != nil {
					t.Fatalf("GetTrace: %v", err)
				}
				if trace.Value != "file" {
					t.Errorf("expected value 'file', got %q", trace.Value)
				}
				effective := findEffective(trace.Contributions)
				if effective == nil || effective.SourceName != "composed blueprint" {
					t.Errorf("expected the composed blueprint as the source, got %+v", effective)
				}
				for _, c := range trace.Contributions {
					if c.AbsFacetPath == attributionFacetPath("low") && !c.Discarded {
						t.Errorf("expected the low facet to be discarded, got %+v", c)
					}
				}
			})

			t.Run("MergeKeepsEachContributor", func(t *testing.T) {
				// Given two merging facets, where the higher facet shares one key with the lower facet
				tc := composeAttribution(t, &blueprintv1alpha1.Blueprint{}, []blueprintv1alpha1.Facet{
					kind.facet(attributionSpec{name: "low", ordinal: 100, values: map[string]string{"only": "low", "both": "low"}}),
					kind.facet(attributionSpec{name: "high", ordinal: 200, values: map[string]string{"both": "high"}}),
				})

				// When tracing both keys
				only, onlyErr := tc.GetTrace(kind.path("only"))
				both, bothErr := tc.GetTrace(kind.path("both"))

				// Then each key names the facet whose value survives
				if onlyErr != nil || bothErr != nil {
					t.Fatalf("GetTrace: %v, %v", onlyErr, bothErr)
				}
				assertAttributedTo(t, only, "low")
				assertAttributedTo(t, both, "high")
			})

			t.Run("ReplaceInOneSourceKeepsAnotherSource", func(t *testing.T) {
				// Given a key from one source, and a replace of the same entry inside a second source
				first := &blueprintv1alpha1.Blueprint{}
				second := &blueprintv1alpha1.Blueprint{}
				tc := composeAttributionSources(t, first, []blueprint.SourceFacetSet{
					{Name: "first", Target: first, Facets: []blueprintv1alpha1.Facet{
						kind.facet(attributionSpec{name: "first", ordinal: 100, values: map[string]string{"key": "first"}}),
					}},
					{Name: "second", Depth: 1, Target: second, Facets: []blueprintv1alpha1.Facet{
						kind.facet(attributionSpec{name: "second-low", ordinal: 100, values: map[string]string{"other": "low"}}),
						kind.facet(attributionSpec{name: "second-high", ordinal: 200, strategy: "replace", values: map[string]string{"other": "high"}}),
					}},
				})

				// When tracing the key against the first target
				trace, err := tc.GetTrace(kind.path("key"))

				// Then the trace names the facet of the first source
				if err != nil {
					t.Fatalf("GetTrace: %v", err)
				}
				assertAttributedTo(t, trace, "first")
			})

			if !kind.hasComponents {
				return
			}

			t.Run("ComponentNamesReplacingFacet", func(t *testing.T) {
				// Given two facets that list one component, where the higher facet replaces the entry
				tc := composeAttribution(t, &blueprintv1alpha1.Blueprint{}, []blueprintv1alpha1.Facet{
					kind.facet(attributionSpec{name: "low", ordinal: 100, components: []string{"shared"}}),
					kind.facet(attributionSpec{name: "high", ordinal: 200, strategy: "replace", components: []string{"shared"}}),
				})

				// When tracing the components
				trace, err := tc.GetTrace(kind.componentsPath)

				// Then the component names the replacing facet
				if err != nil {
					t.Fatalf("GetTrace: %v", err)
				}
				assertAttributedTo(t, trace, "high")
			})

			t.Run("ComponentSkipsRemoveDescriptor", func(t *testing.T) {
				// Given a remove that lists a component, and a higher merge that lists it again
				tc := composeAttribution(t, &blueprintv1alpha1.Blueprint{}, []blueprintv1alpha1.Facet{
					kind.facet(attributionSpec{name: "low-remove", ordinal: 100, strategy: "remove", components: []string{"shared"}}),
					kind.facet(attributionSpec{name: "high-merge", ordinal: 200, components: []string{"shared"}}),
				})

				// When tracing the components
				trace, err := tc.GetTrace(kind.componentsPath)

				// Then the component names the merging facet
				if err != nil {
					t.Fatalf("GetTrace: %v", err)
				}
				assertAttributedTo(t, trace, "high-merge")
			})
		})
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

// attributionSpec describes one facet entry of a merge attribution scenario.
type attributionSpec struct {
	name       string
	ordinal    int
	strategy   string
	values     map[string]string
	components []string
}

// attributionKind builds facets, targets, and explain paths for one component kind.
type attributionKind struct {
	name            string
	path            func(key string) string
	componentsPath  string
	hasComponents   bool
	absentKeyErrors bool
	add             func(f *blueprintv1alpha1.Facet, s attributionSpec)
	target          func(values map[string]string) *blueprintv1alpha1.Blueprint
}

// facet returns a facet that holds the entry of spec for this kind.
func (k attributionKind) facet(s attributionSpec) blueprintv1alpha1.Facet {
	ordinal := s.ordinal
	f := blueprintv1alpha1.Facet{
		Metadata: blueprintv1alpha1.Metadata{Name: s.name},
		Path:     attributionFacetPath(s.name),
		Ordinal:  &ordinal,
	}
	k.add(&f, s)
	return f
}

// attributionKinds returns the three component kinds that share the merge attribution rules.
func attributionKinds() []attributionKind {
	return []attributionKind{
		{
			name:            "Terraform",
			path:            func(key string) string { return "terraform.app.inputs." + key },
			absentKeyErrors: true,
			add: func(f *blueprintv1alpha1.Facet, s attributionSpec) {
				f.TerraformComponents = append(f.TerraformComponents, blueprintv1alpha1.ConditionalTerraformComponent{
					TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "app", Inputs: attributionInputs(s.values)},
					Strategy:           s.strategy,
				})
			},
			target: func(values map[string]string) *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{TerraformComponents: []blueprintv1alpha1.TerraformComponent{
					{Path: "app", Inputs: attributionInputs(values)},
				}}
			},
		},
		{
			name:           "Kustomization",
			path:           func(key string) string { return "kustomize.app.substitutions." + key },
			componentsPath: "kustomize.app.components",
			hasComponents:  true,
			add: func(f *blueprintv1alpha1.Facet, s attributionSpec) {
				f.Kustomizations = append(f.Kustomizations, blueprintv1alpha1.ConditionalKustomization{
					Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "app", Substitutions: s.values, Components: s.components},
					Strategy:      s.strategy,
				})
			},
			target: func(values map[string]string) *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "app", Path: "app", Substitutions: values},
				}}
			},
		},
		{
			name:           "FluxTier",
			path:           func(key string) string { return "kustomize.app-install.substitutions." + key },
			componentsPath: "kustomize.app-install.components",
			hasComponents:  true,
			add: func(f *blueprintv1alpha1.Facet, s attributionSpec) {
				f.FluxSystems = append(f.FluxSystems, blueprintv1alpha1.FluxSystem{
					Name:     "app",
					Strategy: s.strategy,
					Install:  &blueprintv1alpha1.Kustomization{Substitutions: s.values, Components: s.components},
				})
			},
			target: func(values map[string]string) *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{FluxSystems: []blueprintv1alpha1.FluxSystem{
					{Name: "app", Install: &blueprintv1alpha1.Kustomization{Substitutions: values}},
				}}
			},
		},
	}
}

// attributionFacetPath returns the file path of the named scenario facet.
func attributionFacetPath(name string) string {
	return "/template/facets/" + name + ".yaml"
}

// attributionInputs converts substitution values to terraform inputs. A nil map stays nil.
func attributionInputs(values map[string]string) map[string]any {
	if values == nil {
		return nil
	}
	inputs := make(map[string]any, len(values))
	for key, value := range values {
		inputs[key] = value
	}
	return inputs
}

// composeAttribution processes facets into target through ProcessGlobally and returns the finalized collector.
func composeAttribution(t *testing.T, target *blueprintv1alpha1.Blueprint, facets []blueprintv1alpha1.Facet) *blueprint.DefaultTraceCollector {
	t.Helper()
	return composeAttributionSources(t, target, []blueprint.SourceFacetSet{{Target: target, Facets: facets}})
}

// composeAttributionSources processes sources through ProcessGlobally and finalizes the collector on target.
func composeAttributionSources(t *testing.T, target *blueprintv1alpha1.Blueprint, sources []blueprint.SourceFacetSet) *blueprint.DefaultTraceCollector {
	t.Helper()
	processor := blueprint.NewBlueprintProcessor(setupProcessorMocks(t).Runtime)
	tc := blueprint.NewTraceCollector()
	processor.SetTraceCollector(tc)
	if _, err := processor.ProcessGlobally(sources); err != nil {
		t.Fatalf("ProcessGlobally: %v", err)
	}
	tc.Finalize(target, nil, "")
	return tc
}

// assertAttributedTo fails unless every effective contribution of trace comes from the named facet.
func assertAttributedTo(t *testing.T, trace *blueprint.ExplainTrace, facet string) {
	t.Helper()
	found := false
	for _, c := range trace.Contributions {
		if !c.Effective {
			continue
		}
		found = true
		if c.AbsFacetPath != attributionFacetPath(facet) {
			t.Errorf("expected source %q, got %+v", facet, c)
		}
	}
	if !found {
		t.Errorf("expected source %q, got no effective contribution in %+v", facet, trace.Contributions)
	}
}
