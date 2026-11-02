package blueprint

import (
	"errors"
	"os"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
)

// =============================================================================
// Test Setup
// =============================================================================

type HandlerTestMocks struct {
	Shell           *shell.MockShell
	ConfigHandler   *config.MockConfigHandler
	ArtifactBuilder *artifact.MockArtifact
	Evaluator       *evaluator.MockExpressionEvaluator
	Runtime         *runtime.Runtime
	Shims           *Shims
}

func setupHandlerMocks(t *testing.T) *HandlerTestMocks {
	t.Helper()

	tmpDir := t.TempDir()
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	mockArtifact := artifact.NewMockArtifact()

	realEvaluator := evaluator.NewExpressionEvaluator(mockConfigHandler, tmpDir, tmpDir)
	mockEvaluator := evaluator.NewMockExpressionEvaluator()

	mockEvaluator.EvaluateFunc = realEvaluator.Evaluate
	mockEvaluator.EvaluateMapFunc = realEvaluator.EvaluateMap
	mockEvaluator.SetTemplateDataFunc = realEvaluator.SetTemplateData
	mockEvaluator.RegisterFunc = realEvaluator.Register

	rt := &runtime.Runtime{
		Shims:         runtime.NewShims(),
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
		Evaluator:     mockEvaluator,
	}

	mocks := &HandlerTestMocks{
		Shell:           mockShell,
		ConfigHandler:   mockConfigHandler,
		ArtifactBuilder: mockArtifact,
		Evaluator:       mockEvaluator,
		Runtime:         rt,
		Shims:           NewShims(),
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
	})

	return mocks
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestHandler_processAndCompose(t *testing.T) {
	t.Run("CallsSetTerraformComponentsWhenProviderExists", func(t *testing.T) {
		// Given a handler with terraform provider and composed blueprint with components
		mocks := setupHandlerMocks(t)
		mockTerraformProvider := &terraform.MockTerraformProvider{}
		var setComponentsCalled bool
		var receivedComponents []blueprintv1alpha1.TerraformComponent
		mockTerraformProvider.SetTerraformComponentsFunc = func(components []blueprintv1alpha1.TerraformComponent) {
			setComponentsCalled = true
			receivedComponents = components
		}
		mocks.Runtime.TerraformProvider = mockTerraformProvider
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		trueVal := true
		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "rds"},
			},
		}
		handler.sourceBlueprintLoaders["template"] = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return templateBp },
			getSourceNameFunc: func() string { return "template" },
		}
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{
					Sources: []blueprintv1alpha1.Source{
						{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
					},
				}
			},
			getSourceNameFunc: func() string { return "user" },
		}

		// When processing and composing
		err := handler.processAndCompose()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// And SetTerraformComponents should be called
		if !setComponentsCalled {
			t.Error("Expected SetTerraformComponents to be called")
		}
		// And components should be passed correctly
		if len(receivedComponents) != 2 {
			t.Errorf("Expected 2 components to be set, got %d", len(receivedComponents))
		}
	})

	t.Run("SkipsSetTerraformComponentsWhenProviderIsNil", func(t *testing.T) {
		// Given a handler with no terraform provider
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TerraformProvider = nil
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
		}
		trueVal := true
		handler.sourceBlueprintLoaders["template"] = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return templateBp },
			getSourceNameFunc: func() string { return "template" },
		}
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{
					Sources: []blueprintv1alpha1.Source{
						{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
					},
				}
			},
			getSourceNameFunc: func() string { return "user" },
		}

		// When processing and composing
		err := handler.processAndCompose()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// And composed blueprint should be set
		if handler.composedBlueprint == nil {
			t.Error("Expected composed blueprint to be set")
		}
	})

	t.Run("ResolvesConfigBlockRefInTerraformInputsWhenTemplateHasFacetWithConfig", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		templateBp := &blueprintv1alpha1.Blueprint{}
		handler.sourceBlueprintLoaders["template"] = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return templateBp },
			getSourceNameFunc: func() string { return "template" },
			getFacetsFunc: func() []blueprintv1alpha1.Facet {
				return []blueprintv1alpha1.Facet{
					{
						Metadata: blueprintv1alpha1.Metadata{Name: "facet-with-config"},
						Config: []blueprintv1alpha1.ConfigBlock{
							{
								Name: "talos_common_docker",
								Body: map[string]any{
									"value": map[string]any{
										"patchVars":    map[string]any{"key": "val"},
										"common_patch": "${string(talos_common_docker.patchVars)}",
										"patches":      "${string(talos_common_docker.common_patch)}",
									},
								},
							},
						},
						TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
							{
								TerraformComponent: blueprintv1alpha1.TerraformComponent{
									Path:   "cluster/talos",
									Inputs: map[string]any{"common_config_patches": "${talos_common_docker.patches}"},
								},
							},
						},
					},
				}
			},
		}
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return &blueprintv1alpha1.Blueprint{} },
			getSourceNameFunc: func() string { return "user" },
		}

		err := handler.processAndCompose()
		if err != nil {
			t.Fatalf("processAndCompose failed: %v", err)
		}
		if handler.composedBlueprint == nil {
			t.Fatal("Expected composed blueprint to be set")
		}
		if len(handler.composedBlueprint.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(handler.composedBlueprint.TerraformComponents))
		}
		commonConfigPatches := handler.composedBlueprint.TerraformComponents[0].Inputs["common_config_patches"]
		if commonConfigPatches == nil {
			t.Error("Expected common_config_patches to be resolved, got nil")
		}
		if s, ok := commonConfigPatches.(string); ok && strings.Contains(s, "${") {
			t.Errorf("Expected common_config_patches to be resolved (no expression), got %q", s)
		}
	})

	t.Run("DownstreamSourceSeesConfigDerivedByUpstreamSource", func(t *testing.T) {
		// Given a source (core) that derives dns.private_domain, and a second source
		// (consumer) that layers on core and reads that derived value in a substitution
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		coreBp := &blueprintv1alpha1.Blueprint{}
		handler.sourceBlueprintLoaders["core"] = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return coreBp },
			getSourceNameFunc: func() string { return "core" },
			getFacetsFunc: func() []blueprintv1alpha1.Facet {
				return []blueprintv1alpha1.Facet{
					{
						Metadata: blueprintv1alpha1.Metadata{Name: "platform-base"},
						Config: []blueprintv1alpha1.ConfigBlock{
							{Name: "dns", Body: map[string]any{"value": map[string]any{"private_domain": "core-derived"}}},
						},
					},
				}
			},
		}

		consumerBp := &blueprintv1alpha1.Blueprint{Sources: []blueprintv1alpha1.Source{{Name: "core"}}}
		handler.sourceBlueprintLoaders["consumer"] = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return consumerBp },
			getSourceNameFunc: func() string { return "consumer" },
			getFacetsFunc: func() []blueprintv1alpha1.Facet {
				return []blueprintv1alpha1.Facet{
					{
						Metadata: blueprintv1alpha1.Metadata{Name: "addon-app"},
						Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
							{Kustomization: blueprintv1alpha1.Kustomization{
								Name:          "app",
								Path:          "app",
								Substitutions: map[string]string{"external_domain": "${dns.private_domain ?? ''}"},
							}},
						},
					},
				}
			},
		}

		trueVal := true
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{
					Sources: []blueprintv1alpha1.Source{
						{Name: "core", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
						{Name: "consumer", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
					},
				}
			},
			getSourceNameFunc: func() string { return "user" },
		}

		// When processing and composing
		if err := handler.processAndCompose(); err != nil {
			t.Fatalf("processAndCompose failed: %v", err)
		}

		// Then the consumer's substitution resolves to the value core derived, not an empty string
		var appSub string
		found := false
		for _, k := range handler.composedBlueprint.Kustomizations {
			if k.Name == "app" {
				appSub = k.Substitutions["external_domain"]
				found = true
			}
		}
		if !found {
			t.Fatal("Expected composed kustomization 'app'")
		}
		if appSub != "core-derived" {
			t.Errorf("Expected downstream to see upstream-derived dns.private_domain 'core-derived', got %q", appSub)
		}
	})

	t.Run("FacetDerivedConfigWinsOverRawContextValue", func(t *testing.T) {
		// Given a raw context value (e.g. a schema default already reflected in config.yaml)
		// for a key that a facet also derives
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"talos_common": map[string]any{"k8s_service_host": "raw-default"},
			}, nil
		}
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		templateBp := &blueprintv1alpha1.Blueprint{}
		handler.sourceBlueprintLoaders["template"] = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return templateBp },
			getSourceNameFunc: func() string { return "template" },
			getFacetsFunc: func() []blueprintv1alpha1.Facet {
				return []blueprintv1alpha1.Facet{
					{
						Metadata: blueprintv1alpha1.Metadata{Name: "override-facet"},
						Config: []blueprintv1alpha1.ConfigBlock{
							{
								Name: "talos_common",
								Body: map[string]any{"value": map[string]any{"k8s_service_host": "override-value"}},
							},
						},
					},
				}
			},
		}
		trueVal := true
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{
					Sources: []blueprintv1alpha1.Source{
						{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
					},
				}
			},
			getSourceNameFunc: func() string { return "user" },
		}

		// When processing and composing
		if err := handler.processAndCompose(); err != nil {
			t.Fatalf("processAndCompose failed: %v", err)
		}

		// Then the composed scope reflects the facet-derived value, not the raw context default
		talosCommon, ok := handler.composedScope["talos_common"].(map[string]any)
		if !ok {
			t.Fatal("Expected composed scope to contain 'talos_common'")
		}
		if host := talosCommon["k8s_service_host"]; host != "override-value" {
			t.Errorf("Expected facet-derived 'override-value' to win over raw context default, got %q", host)
		}
	})

	t.Run("ExplicitOperatorValueWinsOverUnconditionalFacetConfig", func(t *testing.T) {
		// Given an operator's own explicit value for a key. A facet unconditionally computes a
		// different value for that same key, with no `??` guard of its own.
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		operatorValues := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 3}},
		}
		mocks.ConfigHandler.GetSetValuesFunc = func() map[string]any { return operatorValues }
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		templateBp := &blueprintv1alpha1.Blueprint{}
		handler.sourceBlueprintLoaders["template"] = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return templateBp },
			getSourceNameFunc: func() string { return "template" },
			getFacetsFunc: func() []blueprintv1alpha1.Facet {
				return []blueprintv1alpha1.Facet{
					{
						Metadata: blueprintv1alpha1.Metadata{Name: "cluster-sizing"},
						Config: []blueprintv1alpha1.ConfigBlock{
							{
								Name: "cluster",
								Body: map[string]any{"value": map[string]any{"controlplanes": map[string]any{"cpu": 7}}},
							},
						},
					},
				}
			},
		}
		trueVal := true
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{
					Sources: []blueprintv1alpha1.Source{
						{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
					},
				}
			},
			getSourceNameFunc: func() string { return "user" },
		}

		// When processing and composing
		if err := handler.processAndCompose(); err != nil {
			t.Fatalf("processAndCompose failed: %v", err)
		}

		// Then the composed scope reflects the operator's explicit value, not the facet's
		// unconditional computation
		cluster, ok := handler.composedScope["cluster"].(map[string]any)
		if !ok {
			t.Fatal("Expected composed scope to contain 'cluster'")
		}
		controlplanes, ok := cluster["controlplanes"].(map[string]any)
		if !ok {
			t.Fatalf("Expected cluster.controlplanes to be a map, got %T", cluster["controlplanes"])
		}
		if cpu := controlplanes["cpu"]; cpu != 3 {
			t.Errorf("Expected operator's explicit cpu=3 to win over facet-computed cpu=7, got %v", cpu)
		}
	})

	t.Run("CallsSetConfigScopeWhenProviderExists", func(t *testing.T) {
		// Given a handler with terraform provider and loaders
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		var setConfigScopeCalled bool
		mockTerraformProvider := &terraform.MockTerraformProvider{}
		mockTerraformProvider.SetTerraformComponentsFunc = func([]blueprintv1alpha1.TerraformComponent) {}
		mockTerraformProvider.SetConfigScopeFunc = func(map[string]any) {
			setConfigScopeCalled = true
		}
		mocks.Runtime.TerraformProvider = mockTerraformProvider
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		templateBp := &blueprintv1alpha1.Blueprint{}
		handler.sourceBlueprintLoaders["template"] = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return templateBp },
			getSourceNameFunc: func() string { return "template" },
		}
		trueVal := true
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{
					Sources: []blueprintv1alpha1.Source{
						{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
					},
				}
			},
			getSourceNameFunc: func() string { return "user" },
		}

		// When processing and composing
		err := handler.processAndCompose()

		// Then SetConfigScope should be called when provider exists
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !setConfigScopeCalled {
			t.Error("Expected SetConfigScope to be called when provider exists")
		}
	})
}

// TestHandler_CrossSourceConfig pins the cross-source config behaviors: a referencing (consumer)
// source can override a value an upstream (core) source consumes, activate an upstream capability,
// disable one, and have upstream derivations recompute — while an explicit operator value still wins.
func TestHandler_CrossSourceConfig(t *testing.T) {
	t.Run("OverridesUpstreamConsumedValue", func(t *testing.T) {
		// Given core consumes dns.domain in a kustomization and consumer overrides dns.domain
		core := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "platform-base"},
			Config:   []blueprintv1alpha1.ConfigBlock{configBlock("dns", map[string]any{"domain": "core-default"})},
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{{Kustomization: blueprintv1alpha1.Kustomization{
				Name: "dns", Path: "dns", Substitutions: map[string]string{"domain": "${dns.domain ?? ''}"},
			}}},
		}}
		consumer := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "override"},
			Config:   []blueprintv1alpha1.ConfigBlock{configBlock("dns", map[string]any{"domain": "consumer-override"})},
		}}

		// When composed
		bp := composeCrossSource(t, core, consumer, nil)

		// Then core's component resolves to the consumer's override
		got, found := kustSub(bp, "dns", "domain")
		if !found {
			t.Fatal("expected composed kustomization 'dns'")
		}
		if got != "consumer-override" {
			t.Errorf("expected 'consumer-override', got %q", got)
		}
	})

	t.Run("ActivatesUpstreamCapability", func(t *testing.T) {
		// Given a core facet gated on flags.identity that consumer turns on via config
		core := []blueprintv1alpha1.Facet{{
			Metadata:       blueprintv1alpha1.Metadata{Name: "identity"},
			When:           "flags.identity == true",
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{{Kustomization: blueprintv1alpha1.Kustomization{Name: "identity", Path: "identity"}}},
		}}
		consumer := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "enable-identity"},
			Config:   []blueprintv1alpha1.ConfigBlock{configBlock("flags", map[string]any{"identity": true})},
		}}

		// When composed
		bp := composeCrossSource(t, core, consumer, nil)

		// Then core's gated capability is included
		if !hasKustomization(bp, "identity") {
			t.Error("expected core 'identity' kustomization to be activated by downstream config")
		}
	})

	t.Run("DisablesUpstreamCapability", func(t *testing.T) {
		// Given core enables its own capability and consumer turns it off
		core := []blueprintv1alpha1.Facet{{
			Metadata:       blueprintv1alpha1.Metadata{Name: "identity"},
			When:           "flags.identity == true",
			Config:         []blueprintv1alpha1.ConfigBlock{configBlock("flags", map[string]any{"identity": true})},
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{{Kustomization: blueprintv1alpha1.Kustomization{Name: "identity", Path: "identity"}}},
		}}
		consumer := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "disable-identity"},
			Config:   []blueprintv1alpha1.ConfigBlock{configBlock("flags", map[string]any{"identity": false})},
		}}

		// When composed
		bp := composeCrossSource(t, core, consumer, nil)

		// Then the capability is excluded
		if hasKustomization(bp, "identity") {
			t.Error("expected downstream to disable core 'identity' capability")
		}
	})

	t.Run("UpstreamDerivationRecomputesFromOverride", func(t *testing.T) {
		// Given core derives net.host from dns.domain and consumer overrides dns.domain
		core := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "platform-base"},
			Config: []blueprintv1alpha1.ConfigBlock{
				configBlock("dns", map[string]any{"domain": "core"}),
				configBlock("net", map[string]any{"host": "${dns.domain}.svc"}),
			},
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{{Kustomization: blueprintv1alpha1.Kustomization{
				Name: "net", Path: "net", Substitutions: map[string]string{"host": "${net.host ?? ''}"},
			}}},
		}}
		consumer := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "override"},
			Config:   []blueprintv1alpha1.ConfigBlock{configBlock("dns", map[string]any{"domain": "consumer"})},
		}}

		// When composed
		bp := composeCrossSource(t, core, consumer, nil)

		// Then the upstream derivation reflects the override
		got, found := kustSub(bp, "net", "host")
		if !found {
			t.Fatal("expected composed kustomization 'net'")
		}
		if got != "consumer.svc" {
			t.Errorf("expected derived 'consumer.svc', got %q", got)
		}
	})

	t.Run("OverridesUpstreamFluxSubstitution", func(t *testing.T) {
		// Given core defines a cni flux system install substitution and consumer overrides it
		core := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "cni-base"},
			FluxSystems: []blueprintv1alpha1.FluxSystem{{
				Name: "cni", Path: "cni",
				Install: &blueprintv1alpha1.Kustomization{Components: []string{"helm"}, Substitutions: map[string]string{"host": "core"}},
			}},
		}}
		consumer := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "cni-override"},
			FluxSystems: []blueprintv1alpha1.FluxSystem{{
				Name:    "cni",
				Install: &blueprintv1alpha1.Kustomization{Substitutions: map[string]string{"host": "consumer"}},
			}},
		}}

		// When composed
		bp := composeCrossSource(t, core, consumer, nil)

		// Then the composed cni flux system carries the downstream override
		var got string
		found := false
		for _, sys := range bp.FluxSystems {
			if sys.Name == "cni" && sys.Install != nil {
				got = sys.Install.Substitutions["host"]
				found = true
			}
		}
		if !found {
			t.Fatalf("expected cni flux system with install tier; got %d flux systems", len(bp.FluxSystems))
		}
		if got != "consumer" {
			t.Errorf("expected downstream flux override 'consumer', got %q", got)
		}
	})

	t.Run("ExplicitOperatorValueVetoesDownstreamActivation", func(t *testing.T) {
		// Given the operator explicitly disables identity, a downstream enable must not override it
		core := []blueprintv1alpha1.Facet{{
			Metadata:       blueprintv1alpha1.Metadata{Name: "identity"},
			When:           "flags.identity == true",
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{{Kustomization: blueprintv1alpha1.Kustomization{Name: "identity", Path: "identity"}}},
		}}
		consumer := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "enable-identity"},
			Config:   []blueprintv1alpha1.ConfigBlock{configBlock("flags", map[string]any{"identity": true})},
		}}
		operator := map[string]any{"flags": map[string]any{"identity": false}}

		// When composed with an explicit operator opt-out
		bp := composeCrossSource(t, core, consumer, operator)

		// Then the operator's explicit false wins
		if hasKustomization(bp, "identity") {
			t.Error("expected explicit operator flags.identity=false to veto downstream activation")
		}
	})
}

func TestHandler_getConfigValues(t *testing.T) {
	t.Run("ReturnsNilWhenConfigHandlerNil", func(t *testing.T) {
		// Given a handler with nil ConfigHandler
		mocks := setupHandlerMocks(t)
		mocks.Runtime.ConfigHandler = nil
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When getting config values
		values := handler.getConfigValues()

		// Then should return nil
		if values != nil {
			t.Error("Expected nil when ConfigHandler is nil")
		}
	})

	t.Run("ReturnsNilWhenGetContextValuesErrors", func(t *testing.T) {
		// Given a handler where GetContextValues returns error
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return nil, errors.New("config error")
		}
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When getting config values
		values := handler.getConfigValues()

		// Then should return nil
		if values != nil {
			t.Error("Expected nil when GetContextValues errors")
		}
	})

	t.Run("ReturnsValuesFromConfigHandler", func(t *testing.T) {
		// Given a handler with working ConfigHandler
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"key": "value"}, nil
		}
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When getting config values
		values := handler.getConfigValues()

		// Then should return values
		if values == nil {
			t.Fatal("Expected non-nil values")
		}
		if values["key"] != "value" {
			t.Errorf("Expected key='value', got '%v'", values["key"])
		}
	})

	t.Run("MergesRepositoryScopeWithoutSecret", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"key": "value"}, nil
		}
		secret := "flux-system"
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.userBlueprintLoader = &mockLoaderImpl{
			getSourceNameFunc: func() string { return "user" },
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{
					Repository: blueprintv1alpha1.Repository{
						Url:        "https://github.com/acme/repo.git",
						SecretName: &secret,
					},
				}
			},
		}

		values := handler.getConfigValues()

		if values == nil {
			t.Fatal("Expected non-nil values")
		}
		repository, ok := values["repository"].(map[string]any)
		if !ok {
			t.Fatal("Expected repository map in config values")
		}
		if repository["url"] != "https://github.com/acme/repo.git" {
			t.Errorf("Expected repository.url to be merged, got %v", repository["url"])
		}
		if _, hasSecret := repository["secretName"]; hasSecret {
			t.Error("Expected repository.secretName to be excluded from scope")
		}
		ref, ok := repository["ref"].(map[string]any)
		if !ok {
			t.Fatal("Expected repository.ref map in config values")
		}
		if ref["branch"] != "main" {
			t.Errorf("Expected repository.ref.branch to default to main, got %v", ref["branch"])
		}
	})
}

func TestLatestStableSemver(t *testing.T) {
	t.Run("PicksHighestStableSkippingPrereleases", func(t *testing.T) {
		tag, v, ok := latestStableSemver([]string{"v0.5.0", "v0.6.0", "v0.7.0-rc.1", "not-a-version"})
		if !ok {
			t.Fatal("Expected a stable version to be found")
		}
		if tag != "v0.6.0" {
			t.Errorf("Expected v0.6.0 (highest stable), got %q", tag)
		}
		if v == nil || v.String() != "0.6.0" {
			t.Errorf("Expected parsed 0.6.0, got %v", v)
		}
	})

	t.Run("FalseWhenOnlyPrereleasesOrJunk", func(t *testing.T) {
		if _, _, ok := latestStableSemver([]string{"v1.0.0-rc.1", "garbage"}); ok {
			t.Error("Expected no stable version among prereleases/junk")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

type mockLoaderImpl struct {
	loadFunc            func(sourceName, sourceURL string) error
	getBlueprintFunc    func() *blueprintv1alpha1.Blueprint
	getFacetsFunc       func() []blueprintv1alpha1.Facet
	getTemplateDataFunc func() map[string][]byte
	getSourceNameFunc   func() string
}

func (m *mockLoaderImpl) Load(sourceName, sourceURL string) error {
	if m.loadFunc != nil {
		return m.loadFunc(sourceName, sourceURL)
	}
	return nil
}

func (m *mockLoaderImpl) GetBlueprint() *blueprintv1alpha1.Blueprint {
	if m.getBlueprintFunc != nil {
		return m.getBlueprintFunc()
	}
	return nil
}

func (m *mockLoaderImpl) GetFacets() []blueprintv1alpha1.Facet {
	if m.getFacetsFunc != nil {
		return m.getFacetsFunc()
	}
	return nil
}

func (m *mockLoaderImpl) GetTemplateData() map[string][]byte {
	if m.getTemplateDataFunc != nil {
		return m.getTemplateDataFunc()
	}
	return nil
}

func (m *mockLoaderImpl) GetSourceName() string {
	if m.getSourceNameFunc != nil {
		return m.getSourceNameFunc()
	}
	return ""
}

// composeCrossSource wires a core source, a consumer source that lists core (so it composes deeper),
// and a user blueprint selecting both, runs processAndCompose, and returns the composed blueprint.
// contextValues seeds the operator's values (nil means empty).
func composeCrossSource(t *testing.T, coreFacets, consumerFacets []blueprintv1alpha1.Facet, contextValues map[string]any) *blueprintv1alpha1.Blueprint {
	t.Helper()
	mocks := setupHandlerMocks(t)
	mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
		if contextValues == nil {
			return map[string]any{}, nil
		}
		return contextValues, nil
	}
	mocks.ConfigHandler.GetSetValuesFunc = func() map[string]any { return contextValues }
	handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

	coreBp := &blueprintv1alpha1.Blueprint{}
	handler.sourceBlueprintLoaders["core"] = &mockLoaderImpl{
		getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return coreBp },
		getSourceNameFunc: func() string { return "core" },
		getFacetsFunc:     func() []blueprintv1alpha1.Facet { return coreFacets },
	}

	consumerBp := &blueprintv1alpha1.Blueprint{Sources: []blueprintv1alpha1.Source{{Name: "core"}}}
	handler.sourceBlueprintLoaders["consumer"] = &mockLoaderImpl{
		getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return consumerBp },
		getSourceNameFunc: func() string { return "consumer" },
		getFacetsFunc:     func() []blueprintv1alpha1.Facet { return consumerFacets },
	}

	trueVal := true
	handler.userBlueprintLoader = &mockLoaderImpl{
		getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
			return &blueprintv1alpha1.Blueprint{
				Sources: []blueprintv1alpha1.Source{
					{Name: "core", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
					{Name: "consumer", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
				},
			}
		},
		getSourceNameFunc: func() string { return "user" },
	}

	if err := handler.processAndCompose(); err != nil {
		t.Fatalf("processAndCompose failed: %v", err)
	}
	return handler.composedBlueprint
}

// configBlock builds a ConfigBlock named block whose value map is content (exposed at scope as block.<k>).
func configBlock(block string, content map[string]any) blueprintv1alpha1.ConfigBlock {
	return blueprintv1alpha1.ConfigBlock{Name: block, Body: map[string]any{"value": content}}
}

// kustSub returns the named composed kustomization's substitution value and whether it was found.
func kustSub(bp *blueprintv1alpha1.Blueprint, name, key string) (string, bool) {
	for _, k := range bp.Kustomizations {
		if k.Name == name {
			return k.Substitutions[key], true
		}
	}
	return "", false
}

// hasKustomization reports whether the composed blueprint includes a kustomization by name.
func hasKustomization(bp *blueprintv1alpha1.Blueprint, name string) bool {
	for _, k := range bp.Kustomizations {
		if k.Name == name {
			return true
		}
	}
	return false
}
