package v1alpha1

import (
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/kustomize"
	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestKustomization_ToFluxKustomization(t *testing.T) {
	t.Run("BasicConversionWithDefaults", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
			Substitutions: map[string]string{
				"domain": "example.com",
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if result.Name != "test-kustomization" {
			t.Errorf("Expected name 'test-kustomization', got '%s'", result.Name)
		}
		if result.Namespace != "test-namespace" {
			t.Errorf("Expected namespace 'test-namespace', got '%s'", result.Namespace)
		}
		if result.Spec.SourceRef.Name != "default-source" {
			t.Errorf("Expected source name 'default-source', got '%s'", result.Spec.SourceRef.Name)
		}
		if result.Spec.SourceRef.Kind != "GitRepository" {
			t.Errorf("Expected source kind 'GitRepository', got '%s'", result.Spec.SourceRef.Kind)
		}
		if result.Spec.Path != "kustomize/test/path" {
			t.Errorf("Expected path 'kustomize/test/path', got '%s'", result.Spec.Path)
		}
		if result.Spec.Interval.Duration != constants.DefaultFluxPrimaryKustomizationInterval {
			t.Errorf("Expected primary-repository default interval (no explicit source set), got %v", result.Spec.Interval.Duration)
		}
		// PostBuild should have component-specific ConfigMap when substitutions exist
		if result.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild to be set when substitutions exist")
		}
		if len(result.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Fatalf("Expected 1 SubstituteFrom reference (values-test-kustomization), got %d", len(result.Spec.PostBuild.SubstituteFrom))
		}
		if result.Spec.PostBuild.SubstituteFrom[0].Name != "values-test-kustomization" {
			t.Errorf("Expected SubstituteFrom to be values-test-kustomization, got '%s'", result.Spec.PostBuild.SubstituteFrom[0].Name)
		}
	})

	t.Run("WithAllFieldsSet", func(t *testing.T) {
		interval := DurationString{Duration: 5 * time.Minute}
		retryInterval := DurationString{Duration: 2 * time.Minute}
		timeout := DurationString{Duration: 10 * time.Minute}
		wait := true
		force := false
		prune := true
		destroy := false

		kustomization := &Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "custom-source",
			DependsOn:     []string{"dep1", "dep2"},
			Interval:      &interval,
			RetryInterval: &retryInterval,
			Timeout:       &timeout,
			Wait:          &wait,
			Force:         &force,
			Prune:         &prune,
			Destroy:       boolExprPtr(destroy),
			Components:    []string{"comp1", "comp2"},
			Patches: []BlueprintPatch{
				{
					Patch: "apiVersion: v1\nkind: Service\nmetadata:\n  name: test",
					Target: &kustomize.Selector{
						Kind:      "Service",
						Name:      "test",
						Namespace: "test-ns",
					},
				},
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if result.Spec.SourceRef.Name != "custom-source" {
			t.Errorf("Expected source name 'custom-source', got '%s'", result.Spec.SourceRef.Name)
		}
		if len(result.Spec.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(result.Spec.DependsOn))
		}
		if result.Spec.DependsOn[0].Name != "dep1" || result.Spec.DependsOn[0].Namespace != "test-namespace" {
			t.Errorf("Expected dependency dep1 in test-namespace, got %v", result.Spec.DependsOn[0])
		}
		if result.Spec.Interval.Duration != 5*time.Minute {
			t.Errorf("Expected interval 5m, got %v", result.Spec.Interval.Duration)
		}
		if result.Spec.RetryInterval.Duration != 2*time.Minute {
			t.Errorf("Expected retry interval 2m, got %v", result.Spec.RetryInterval.Duration)
		}
		if result.Spec.Timeout.Duration != 10*time.Minute {
			t.Errorf("Expected timeout 10m, got %v", result.Spec.Timeout.Duration)
		}
		if result.Spec.Wait != wait {
			t.Errorf("Expected wait %v, got %v", wait, result.Spec.Wait)
		}
		if result.Spec.Force != force {
			t.Errorf("Expected force %v, got %v", force, result.Spec.Force)
		}
		if result.Spec.Prune != prune {
			t.Errorf("Expected prune %v, got %v", prune, result.Spec.Prune)
		}
		if result.Spec.DeletionPolicy != "MirrorPrune" {
			t.Errorf("Expected deletion policy 'MirrorPrune', got '%s'", result.Spec.DeletionPolicy)
		}
		if len(result.Spec.Components) != 2 {
			t.Errorf("Expected 2 components, got %d", len(result.Spec.Components))
		}
		if len(result.Spec.Patches) != 1 {
			t.Errorf("Expected 1 patch, got %d", len(result.Spec.Patches))
		}
		if result.Spec.Patches[0].Target.Kind != "Service" {
			t.Errorf("Expected patch target kind 'Service', got '%s'", result.Spec.Patches[0].Target.Kind)
		}
	})

	t.Run("WithSubstitutions", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
			Substitutions: map[string]string{
				"domain": "example.com",
				"region": "us-west-2",
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if result.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild to be set when there are substitutions")
		}
		if len(result.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Fatalf("Expected 1 SubstituteFrom reference (values-test-kustomization), got %d", len(result.Spec.PostBuild.SubstituteFrom))
		}
		if result.Spec.PostBuild.SubstituteFrom[0].Name != "values-test-kustomization" {
			t.Errorf("Expected SubstituteFrom to be values-test-kustomization, got '%s'", result.Spec.PostBuild.SubstituteFrom[0].Name)
		}
	})

	t.Run("WithoutSubstitutions", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		// PostBuild should not be set when there are no substitutions
		if result.Spec.PostBuild != nil {
			t.Errorf("Expected PostBuild to be nil when there are no substitutions, got %v", result.Spec.PostBuild)
		}
	})

	t.Run("CrdLayerSkipsPostBuildEvenWithConfigMaps", func(t *testing.T) {
		configMaps := map[string]map[string]string{"values-common": {"key": "value"}}

		// A normal kustomization picks up the global ConfigMaps via PostBuild.
		normal := (&Kustomization{Name: "telemetry-base", Path: "telemetry/base"}).
			ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull, configMaps)
		if normal.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild on a normal kustomization with global ConfigMaps")
		}

		// The CRD layer skips PostBuild entirely.
		crds := (&Kustomization{Name: CrdLayerName, Path: CrdLayerName}).
			ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull, configMaps)
		if crds.Spec.PostBuild != nil {
			t.Errorf("Expected nil PostBuild for the %q layer, got %v", CrdLayerName, crds.Spec.PostBuild)
		}
	})

	t.Run("WithOCISource", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "oci-source",
		}

		sources := []Source{
			{
				Name: "oci-source",
				Url:  "oci://example.com/repo",
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", sources, constants.GitopsModePull)

		if result.Spec.SourceRef.Kind != "OCIRepository" {
			t.Errorf("Expected source kind 'OCIRepository', got '%s'", result.Spec.SourceRef.Kind)
		}
		if result.Spec.SourceRef.Name != "oci-source" {
			t.Errorf("Expected source name 'oci-source', got '%s'", result.Spec.SourceRef.Name)
		}
	})

	t.Run("WithGitSource", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "git-source",
		}

		sources := []Source{
			{
				Name: "git-source",
				Url:  "https://example.com/repo.git",
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", sources, constants.GitopsModePull)

		if result.Spec.SourceRef.Kind != "GitRepository" {
			t.Errorf("Expected source kind 'GitRepository', got '%s'", result.Spec.SourceRef.Kind)
		}
		if result.Spec.SourceRef.Name != "git-source" {
			t.Errorf("Expected source name 'git-source', got '%s'", result.Spec.SourceRef.Name)
		}
	})

	t.Run("WithEmptyPath", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "",
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if result.Spec.Path != "kustomize/test-kustomization" {
			t.Errorf("Expected path 'kustomize/test-kustomization', got '%s'", result.Spec.Path)
		}
	})

	t.Run("WithPathBackslashes", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test\\path\\with\\backslashes",
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if result.Spec.Path != "kustomize/test/path/with/backslashes" {
			t.Errorf("Expected path with forward slashes, got '%s'", result.Spec.Path)
		}
	})

	t.Run("WithDestroyTrue", func(t *testing.T) {
		destroy := true
		kustomization := &Kustomization{
			Name:    "test-kustomization",
			Path:    "test/path",
			Destroy: boolExprPtr(destroy),
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if result.Spec.DeletionPolicy != "WaitForTermination" {
			t.Errorf("Expected deletion policy 'WaitForTermination', got '%s'", result.Spec.DeletionPolicy)
		}
	})

	t.Run("WithDestroyFalse", func(t *testing.T) {
		destroy := false
		kustomization := &Kustomization{
			Name:    "test-kustomization",
			Path:    "test/path",
			Destroy: boolExprPtr(destroy),
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if result.Spec.DeletionPolicy != "MirrorPrune" {
			t.Errorf("Expected deletion policy 'MirrorPrune', got '%s'", result.Spec.DeletionPolicy)
		}
	})

	t.Run("WithDestroyNil", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:    "test-kustomization",
			Path:    "test/path",
			Destroy: nil,
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if result.Spec.DeletionPolicy != "WaitForTermination" {
			t.Errorf("Expected deletion policy 'WaitForTermination' when Destroy is nil, got '%s'", result.Spec.DeletionPolicy)
		}
	})

	t.Run("WithEmptySourceUsesDefault", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "",
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if result.Spec.SourceRef.Name != "default-source" {
			t.Errorf("Expected source name 'default-source', got '%s'", result.Spec.SourceRef.Name)
		}
	})

	t.Run("WithTemplateSourceLocalUsesDefault", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "template",
		}
		sources := []Source{{Name: "template", Url: ""}}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", sources, constants.GitopsModePull)

		if result.Spec.SourceRef.Name != "default-source" {
			t.Errorf("Expected source name 'default-source' when template is local, got '%s'", result.Spec.SourceRef.Name)
		}
	})

	t.Run("WithTemplateSourceRemoteUsesTemplate", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "template",
		}
		sources := []Source{{Name: "template", Url: "https://github.com/example/template.git"}}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", sources, constants.GitopsModePull)

		if result.Spec.SourceRef.Name != "template" {
			t.Errorf("Expected source name 'template' when template is remote, got '%s'", result.Spec.SourceRef.Name)
		}
	})

	t.Run("WithZeroIntervalUsesDefault", func(t *testing.T) {
		zeroInterval := DurationString{Duration: 0}
		kustomization := &Kustomization{
			Name:     "test-kustomization",
			Path:     "test/path",
			Interval: &zeroInterval,
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if result.Spec.Interval.Duration != constants.DefaultFluxPrimaryKustomizationInterval {
			t.Errorf("Expected primary-repository default interval (no explicit source set), got %v", result.Spec.Interval.Duration)
		}
	})

	t.Run("WithPatchesWithoutTarget", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
			Patches: []BlueprintPatch{
				{
					Patch: "apiVersion: v1\nkind: Service\nmetadata:\n  name: test",
				},
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if len(result.Spec.Patches) != 1 {
			t.Fatalf("Expected 1 patch, got %d", len(result.Spec.Patches))
		}
		if result.Spec.Patches[0].Target != nil {
			t.Error("Expected patch target to be nil")
		}
		if result.Spec.Patches[0].Patch != "apiVersion: v1\nkind: Service\nmetadata:\n  name: test" {
			t.Errorf("Expected patch content, got '%s'", result.Spec.Patches[0].Patch)
		}
	})

	t.Run("WithEmptyPatchIgnored", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
			Patches: []BlueprintPatch{
				{
					Patch: "",
				},
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if len(result.Spec.Patches) != 0 {
			t.Errorf("Expected 0 patches (empty patch ignored), got %d", len(result.Spec.Patches))
		}
	})

	t.Run("TypeMetaAndObjectMeta", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{}, constants.GitopsModePull)

		if result.Kind != "Kustomization" {
			t.Errorf("Expected Kind 'Kustomization', got '%s'", result.Kind)
		}
		if result.APIVersion != "kustomize.toolkit.fluxcd.io/v1" {
			t.Errorf("Expected APIVersion 'kustomize.toolkit.fluxcd.io/v1', got '%s'", result.APIVersion)
		}
		if result.Name != "test-kustomization" {
			t.Errorf("Expected Name 'test-kustomization', got '%s'", result.Name)
		}
		if result.Namespace != "test-namespace" {
			t.Errorf("Expected Namespace 'test-namespace', got '%s'", result.Namespace)
		}
	})

	t.Run("NamespaceFieldOverridesDefault", func(t *testing.T) {
		// Given a kustomization that pins its own namespace
		kustomization := &Kustomization{
			Name:      "test-kustomization",
			Path:      "test/path",
			Namespace: "custom-ns",
		}

		// When converted with a different default namespace
		result := kustomization.ToFluxKustomization("default-ns", "default-source", []Source{}, constants.GitopsModePull)

		// Then the rendered Kustomization lives in the per-kustomization namespace
		if result.Namespace != "custom-ns" {
			t.Errorf("Expected metadata.namespace 'custom-ns', got '%s'", result.Namespace)
		}
		// And spec.targetNamespace stays empty when not configured
		if result.Spec.TargetNamespace != "" {
			t.Errorf("Expected empty spec.targetNamespace, got '%s'", result.Spec.TargetNamespace)
		}
		// And spec.sourceRef.namespace is pinned to the gitops namespace so Flux
		// resolves the source where it actually lives, not in the override namespace
		// (an empty SourceRef.Namespace would default to the Kustomization's own
		// namespace and produce a source-not-found reconcile error).
		if result.Spec.SourceRef.Namespace != "default-ns" {
			t.Errorf("Expected spec.sourceRef.namespace 'default-ns', got '%s'", result.Spec.SourceRef.Namespace)
		}
	})

	t.Run("SourceRefNamespaceStaysEmptyWhenNamespaceNotOverridden", func(t *testing.T) {
		// Given a kustomization that does not override its namespace
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
		}

		// When converted
		result := kustomization.ToFluxKustomization("default-ns", "default-source", []Source{}, constants.GitopsModePull)

		// Then SourceRef.Namespace is left empty so Flux defaults it to the
		// Kustomization's own namespace, preserving prior serialized output.
		if result.Spec.SourceRef.Namespace != "" {
			t.Errorf("Expected empty spec.sourceRef.namespace, got '%s'", result.Spec.SourceRef.Namespace)
		}
	})

	t.Run("DependsOnReferencesUseDefaultNamespaceEvenWhenNamespaceOverridden", func(t *testing.T) {
		// Given a kustomization that overrides its own namespace and has dependencies
		kustomization := &Kustomization{
			Name:      "test-kustomization",
			Path:      "test/path",
			Namespace: "custom-ns",
			DependsOn: []string{"dep1"},
		}

		// When converted
		result := kustomization.ToFluxKustomization("default-ns", "default-source", []Source{}, constants.GitopsModePull)

		// Then DependsOn references resolve in the default namespace, not the override.
		// Cross-namespace dependencies are not currently supported and would require a
		// richer DependsOn schema; this guards against accidentally changing that contract.
		if len(result.Spec.DependsOn) != 1 || result.Spec.DependsOn[0].Namespace != "default-ns" {
			t.Errorf("Expected DependsOn[0] in default-ns, got %+v", result.Spec.DependsOn)
		}
	})

	t.Run("TargetNamespacePopulatesSpec", func(t *testing.T) {
		// Given a kustomization that asks Flux to rewrite reconciled resource namespaces
		kustomization := &Kustomization{
			Name:            "test-kustomization",
			Path:            "test/path",
			TargetNamespace: "workload-ns",
		}

		// When converted
		result := kustomization.ToFluxKustomization("default-ns", "default-source", []Source{}, constants.GitopsModePull)

		// Then spec.targetNamespace carries through to the Flux Kustomization
		if result.Spec.TargetNamespace != "workload-ns" {
			t.Errorf("Expected spec.targetNamespace 'workload-ns', got '%s'", result.Spec.TargetNamespace)
		}
		// And the Kustomization object itself stays in the default namespace
		if result.Namespace != "default-ns" {
			t.Errorf("Expected metadata.namespace 'default-ns', got '%s'", result.Namespace)
		}
	})

	t.Run("DefaultIntervalIsSameRegardlessOfGitopsMode", func(t *testing.T) {
		// Given a kustomization with no explicit Interval or Source (resolves to the
		// blueprint's own/primary source)
		kustomization := &Kustomization{Name: "k", Path: "p"}

		// When converted under both pull and push mode
		pullResult := kustomization.ToFluxKustomization("ns", "src", []Source{}, constants.GitopsModePull)
		pushResult := kustomization.ToFluxKustomization("ns", "src", []Source{}, constants.GitopsModePush)

		// Then both render the same default interval: the notifier re-triggers reconciliation
		// on every apply/up/bootstrap regardless of mode, so the interval has no reason to
		// differ between them
		if pullResult.Spec.Interval.Duration != constants.DefaultFluxPrimaryKustomizationInterval {
			t.Errorf("Expected pull-mode interval %v, got %v", constants.DefaultFluxPrimaryKustomizationInterval, pullResult.Spec.Interval.Duration)
		}
		if pushResult.Spec.Interval.Duration != constants.DefaultFluxPrimaryKustomizationInterval {
			t.Errorf("Expected push-mode interval %v, got %v", constants.DefaultFluxPrimaryKustomizationInterval, pushResult.Spec.Interval.Duration)
		}
	})

	t.Run("DefaultIntervalIsShortWhenSourceResolvesToPrimaryRepository", func(t *testing.T) {
		// Given a kustomization with no explicit Source (falls back to the blueprint's own
		// default source, e.g. the top-level repository: field)
		kustomization := &Kustomization{Name: "k", Path: "p"}

		// When converted
		result := kustomization.ToFluxKustomization("ns", "default-source", []Source{}, constants.GitopsModePull)

		// Then it gets the short, continuously-polled primary-repository default: this content
		// is presumed live and actively pushed, not a pinned vendor dependency
		if result.Spec.Interval.Duration != constants.DefaultFluxPrimaryKustomizationInterval {
			t.Errorf("Expected primary-repository interval %v, got %v", constants.DefaultFluxPrimaryKustomizationInterval, result.Spec.Interval.Duration)
		}
	})

	t.Run("DefaultIntervalIsLongForNamedVendorSource", func(t *testing.T) {
		// Given a kustomization that explicitly names a vendor source distinct from the
		// blueprint's default source
		kustomization := &Kustomization{Name: "k", Path: "p", Source: "core"}

		// When converted
		result := kustomization.ToFluxKustomization("ns", "default-source", []Source{}, constants.GitopsModePull)

		// Then it gets the long, backstop-only default: vendor content is pinned and
		// explicitly re-triggered, not continuously tracked
		if result.Spec.Interval.Duration != constants.DefaultFluxKustomizationInterval {
			t.Errorf("Expected vendor-source interval %v, got %v", constants.DefaultFluxKustomizationInterval, result.Spec.Interval.Duration)
		}
	})

	t.Run("BlueprintIntervalOverrideBeatsDefault", func(t *testing.T) {
		// Given a kustomization that explicitly sets a short Interval
		override := DurationString{Duration: 2 * time.Minute}
		kustomization := &Kustomization{Name: "k", Path: "p", Interval: &override}

		// When converted
		result := kustomization.ToFluxKustomization("ns", "src", []Source{}, constants.GitopsModePull)

		// Then the user's explicit Interval wins over the default
		if result.Spec.Interval.Duration != override.Duration {
			t.Errorf("Expected explicit interval %v to override default, got %v", override.Duration, result.Spec.Interval.Duration)
		}
	})

	t.Run("DecryptionUnsetByDefault", func(t *testing.T) {
		// Given a kustomization with no Decryption
		kustomization := &Kustomization{Name: "k", Path: "p"}

		// When converted
		result := kustomization.ToFluxKustomization("ns", "src", []Source{}, constants.GitopsModePull)

		// Then spec.decryption is nil (Flux default: no decryption)
		if result.Spec.Decryption != nil {
			t.Errorf("Expected nil Decryption, got %+v", result.Spec.Decryption)
		}
	})

	t.Run("DecryptionThreadedIntoSpec", func(t *testing.T) {
		// Given a kustomization declaring sops decryption via a key Secret
		kustomization := &Kustomization{
			Name:       "k",
			Path:       "p",
			Decryption: &Decryption{Provider: "sops", SecretRef: &DecryptionSecretRef{Name: "sops-age"}},
		}

		// When converted
		result := kustomization.ToFluxKustomization("ns", "src", []Source{}, constants.GitopsModePull)

		// Then spec.decryption carries the provider and secretRef name
		if result.Spec.Decryption == nil {
			t.Fatal("Expected Decryption to be set")
		}
		if result.Spec.Decryption.Provider != "sops" {
			t.Errorf("Expected provider 'sops', got '%s'", result.Spec.Decryption.Provider)
		}
		if result.Spec.Decryption.SecretRef == nil || result.Spec.Decryption.SecretRef.Name != "sops-age" {
			t.Errorf("Expected secretRef name 'sops-age', got %+v", result.Spec.Decryption.SecretRef)
		}
	})

	t.Run("DecryptionProviderOnlyOmitsSecretRef", func(t *testing.T) {
		// Given a kustomization with a provider but no secretRef
		kustomization := &Kustomization{
			Name:       "k",
			Path:       "p",
			Decryption: &Decryption{Provider: "sops"},
		}

		// When converted
		result := kustomization.ToFluxKustomization("ns", "src", []Source{}, constants.GitopsModePull)

		// Then the provider is set and secretRef stays nil
		if result.Spec.Decryption == nil || result.Spec.Decryption.Provider != "sops" {
			t.Fatalf("Expected provider 'sops', got %+v", result.Spec.Decryption)
		}
		if result.Spec.Decryption.SecretRef != nil {
			t.Errorf("Expected nil secretRef, got %+v", result.Spec.Decryption.SecretRef)
		}
	})
}

func TestKustomization_HealthCheckExprs(t *testing.T) {
	cnpgReady := HealthCheckExpr{
		APIVersion: "postgresql.cnpg.io/v1",
		Kind:       "Cluster",
		Current:    "status.conditions.exists(e, e.type == 'Ready' && e.status == 'True')",
	}

	t.Run("UnmarshalsCamelCaseKeys", func(t *testing.T) {
		// Given a kustomization authored with healthCheckExprs
		yamlData := []byte(`name: identity-resources-database
healthCheckExprs:
  - apiVersion: postgresql.cnpg.io/v1
    kind: Cluster
    current: status.conditions.exists(e, e.type == 'Ready' && e.status == 'True')
    inProgress: status.conditions.exists(e, e.type == 'Initialized')
    failed: status.conditions.exists(e, e.type == 'Failed')
`)

		// When unmarshaled
		var k Kustomization
		if err := yaml.Unmarshal(yamlData, &k); err != nil {
			t.Fatalf("Failed to unmarshal Kustomization with healthCheckExprs: %v", err)
		}

		// Then every camelCase key lands on its field
		if len(k.HealthCheckExprs) != 1 {
			t.Fatalf("Expected 1 health check expression, got %d", len(k.HealthCheckExprs))
		}
		expr := k.HealthCheckExprs[0]
		if expr.APIVersion != "postgresql.cnpg.io/v1" {
			t.Errorf("Expected APIVersion 'postgresql.cnpg.io/v1', got %q", expr.APIVersion)
		}
		if expr.Kind != "Cluster" {
			t.Errorf("Expected Kind 'Cluster', got %q", expr.Kind)
		}
		if expr.Current != "status.conditions.exists(e, e.type == 'Ready' && e.status == 'True')" {
			t.Errorf("Expected Current CEL expression, got %q", expr.Current)
		}
		if expr.InProgress != "status.conditions.exists(e, e.type == 'Initialized')" {
			t.Errorf("Expected InProgress CEL expression, got %q", expr.InProgress)
		}
		if expr.Failed != "status.conditions.exists(e, e.type == 'Failed')" {
			t.Errorf("Expected Failed CEL expression, got %q", expr.Failed)
		}
	})

	t.Run("ConvertsToFluxHealthCheckExprs", func(t *testing.T) {
		// Given a waiting kustomization with a CNPG health check
		wait := true
		kustomization := &Kustomization{
			Name:             "identity-resources-database",
			Wait:             &wait,
			HealthCheckExprs: []HealthCheckExpr{cnpgReady},
		}

		// When converted to a Flux Kustomization
		result := kustomization.ToFluxKustomization("system-gitops", "core", []Source{}, constants.GitopsModePull)

		// Then the expression is carried onto spec.healthCheckExprs
		if len(result.Spec.HealthCheckExprs) != 1 {
			t.Fatalf("Expected 1 health check expression on the Flux spec, got %d", len(result.Spec.HealthCheckExprs))
		}
		got := result.Spec.HealthCheckExprs[0]
		if got.APIVersion != cnpgReady.APIVersion || got.Kind != cnpgReady.Kind {
			t.Errorf("Expected %s/%s under evaluation, got %s/%s", cnpgReady.APIVersion, cnpgReady.Kind, got.APIVersion, got.Kind)
		}
		if got.Current != cnpgReady.Current {
			t.Errorf("Expected Current %q, got %q", cnpgReady.Current, got.Current)
		}
	})

	t.Run("OmitsFluxHealthCheckExprsWhenUnset", func(t *testing.T) {
		// Given a kustomization with no health check expressions
		kustomization := &Kustomization{Name: "policy"}

		// When converted to a Flux Kustomization
		result := kustomization.ToFluxKustomization("system-gitops", "core", []Source{}, constants.GitopsModePull)

		// Then spec.healthCheckExprs stays nil so it is omitted from the rendered output
		if result.Spec.HealthCheckExprs != nil {
			t.Errorf("Expected nil HealthCheckExprs, got %+v", result.Spec.HealthCheckExprs)
		}
	})

	t.Run("DeepCopyIndependentOfOriginal", func(t *testing.T) {
		// Given a kustomization with a health check expression
		original := &Kustomization{Name: "k", HealthCheckExprs: []HealthCheckExpr{cnpgReady}}

		// When deep-copied and the copy mutated
		clone := original.DeepCopy()
		clone.HealthCheckExprs[0].Current = "false"

		// Then the original is unchanged
		if original.HealthCheckExprs[0].Current != cnpgReady.Current {
			t.Errorf("Expected original unchanged, got %q", original.HealthCheckExprs[0].Current)
		}
	})

	t.Run("OverlayReplacesMatchingApiVersionAndKind", func(t *testing.T) {
		// Given a base and an overlay gating the same resource kind
		base := Kustomization{Name: "k", HealthCheckExprs: []HealthCheckExpr{cnpgReady}}
		overlay := Kustomization{Name: "k", HealthCheckExprs: []HealthCheckExpr{{
			APIVersion: "postgresql.cnpg.io/v1",
			Kind:       "Cluster",
			Current:    "status.conditions.exists(e, e.type == 'ConsistentSystemID')",
		}}}

		// When merged
		merged := MergeKustomizationFields(base, overlay)

		// Then the overlay expression replaces the base one rather than duplicating it
		if len(merged.HealthCheckExprs) != 1 {
			t.Fatalf("Expected 1 health check expression, got %d", len(merged.HealthCheckExprs))
		}
		if merged.HealthCheckExprs[0].Current != "status.conditions.exists(e, e.type == 'ConsistentSystemID')" {
			t.Errorf("Expected overlay expression to win, got %q", merged.HealthCheckExprs[0].Current)
		}
	})

	t.Run("OverlayAppendsUnmatchedKindAndLeavesBaseIntact", func(t *testing.T) {
		// Given a base gating one kind and an overlay gating another
		base := Kustomization{Name: "k", HealthCheckExprs: []HealthCheckExpr{cnpgReady}}
		overlay := Kustomization{Name: "k", HealthCheckExprs: []HealthCheckExpr{{
			APIVersion: "helm.toolkit.fluxcd.io/v2",
			Kind:       "HelmRelease",
			Current:    "status.conditions.exists(e, e.type == 'Ready' && e.status == 'True')",
		}}}

		// When merged
		merged := MergeKustomizationFields(base, overlay)

		// Then both gates survive and the base slice is not mutated
		if len(merged.HealthCheckExprs) != 2 {
			t.Fatalf("Expected 2 health check expressions, got %d", len(merged.HealthCheckExprs))
		}
		if merged.HealthCheckExprs[0].Kind != "Cluster" || merged.HealthCheckExprs[1].Kind != "HelmRelease" {
			t.Errorf("Expected Cluster then HelmRelease, got %s then %s", merged.HealthCheckExprs[0].Kind, merged.HealthCheckExprs[1].Kind)
		}
		if len(base.HealthCheckExprs) != 1 {
			t.Errorf("Expected base left intact, got %d expressions", len(base.HealthCheckExprs))
		}
	})

	t.Run("EmptyOverlayLeavesBaseExpressions", func(t *testing.T) {
		// Given a base with a health check and an overlay without one
		base := Kustomization{Name: "k", HealthCheckExprs: []HealthCheckExpr{cnpgReady}}
		overlay := Kustomization{Name: "k"}

		// When merged
		merged := MergeKustomizationFields(base, overlay)

		// Then the base expression survives
		if len(merged.HealthCheckExprs) != 1 || merged.HealthCheckExprs[0].Current != cnpgReady.Current {
			t.Errorf("Expected base expression preserved, got %+v", merged.HealthCheckExprs)
		}
	})
}

func TestKustomization_HealthChecks(t *testing.T) {
	webhookDeployment := HealthCheckRef{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "kyverno-admission-controller",
		Namespace:  "system-policy",
	}

	t.Run("UnmarshalsCamelCaseKeys", func(t *testing.T) {
		// Given a kustomization authored with healthChecks
		yamlData := []byte(`name: policy-install
healthChecks:
  - apiVersion: apps/v1
    kind: Deployment
    name: kyverno-admission-controller
    namespace: system-policy
`)

		// When unmarshaled
		var k Kustomization
		if err := yaml.Unmarshal(yamlData, &k); err != nil {
			t.Fatalf("Failed to unmarshal Kustomization with healthChecks: %v", err)
		}

		// Then every camelCase key lands on its field
		if len(k.HealthChecks) != 1 {
			t.Fatalf("Expected 1 health check reference, got %d", len(k.HealthChecks))
		}
		ref := k.HealthChecks[0]
		if ref.APIVersion != "apps/v1" {
			t.Errorf("Expected APIVersion 'apps/v1', got %q", ref.APIVersion)
		}
		if ref.Kind != "Deployment" {
			t.Errorf("Expected Kind 'Deployment', got %q", ref.Kind)
		}
		if ref.Name != "kyverno-admission-controller" {
			t.Errorf("Expected Name 'kyverno-admission-controller', got %q", ref.Name)
		}
		if ref.Namespace != "system-policy" {
			t.Errorf("Expected Namespace 'system-policy', got %q", ref.Namespace)
		}
	})

	t.Run("ConvertsToFluxHealthChecksAndForcesWaitFalse", func(t *testing.T) {
		// Given a kustomization that explicitly requests Wait true, alongside a health check ref
		wait := true
		kustomization := &Kustomization{
			Name:         "policy-install",
			Wait:         &wait,
			HealthChecks: []HealthCheckRef{webhookDeployment},
		}

		// When converted to a Flux Kustomization
		result := kustomization.ToFluxKustomization("system-gitops", "core", []Source{}, constants.GitopsModePull)

		// Then the reference is carried onto spec.healthChecks
		if len(result.Spec.HealthChecks) != 1 {
			t.Fatalf("Expected 1 health check reference on the Flux spec, got %d", len(result.Spec.HealthChecks))
		}
		got := result.Spec.HealthChecks[0]
		if got.APIVersion != webhookDeployment.APIVersion || got.Kind != webhookDeployment.Kind {
			t.Errorf("Expected %s/%s, got %s/%s", webhookDeployment.APIVersion, webhookDeployment.Kind, got.APIVersion, got.Kind)
		}
		if got.Name != webhookDeployment.Name || got.Namespace != webhookDeployment.Namespace {
			t.Errorf("Expected %s/%s, got %s/%s", webhookDeployment.Namespace, webhookDeployment.Name, got.Namespace, got.Name)
		}

		// And Flux's own Wait is forced false, since Flux ignores spec.healthChecks otherwise,
		// even though this kustomization explicitly asked for Wait true
		if result.Spec.Wait {
			t.Error("Expected Wait forced false when HealthChecks is set, got true")
		}
	})

	t.Run("OmitsFluxHealthChecksWhenUnsetAndLeavesWaitAlone", func(t *testing.T) {
		// Given a kustomization with no health check references
		kustomization := &Kustomization{Name: "policy"}

		// When converted to a Flux Kustomization
		result := kustomization.ToFluxKustomization("system-gitops", "core", []Source{}, constants.GitopsModePull)

		// Then spec.healthChecks stays nil so it is omitted from the rendered output
		if result.Spec.HealthChecks != nil {
			t.Errorf("Expected nil HealthChecks, got %+v", result.Spec.HealthChecks)
		}

		// And Wait keeps its default rather than being forced false
		if !result.Spec.Wait {
			t.Error("Expected Wait to keep its default of true, got false")
		}
	})

	t.Run("DeepCopyIndependentOfOriginal", func(t *testing.T) {
		// Given a kustomization with a health check reference
		original := &Kustomization{Name: "k", HealthChecks: []HealthCheckRef{webhookDeployment}}

		// When deep-copied and the copy mutated
		clone := original.DeepCopy()
		clone.HealthChecks[0].Name = "other"

		// Then the original is unchanged
		if original.HealthChecks[0].Name != webhookDeployment.Name {
			t.Errorf("Expected original unchanged, got %q", original.HealthChecks[0].Name)
		}
	})

	t.Run("OverlayReplacesMatchingReference", func(t *testing.T) {
		// Given a base and an overlay naming the same resource
		base := Kustomization{Name: "k", HealthChecks: []HealthCheckRef{webhookDeployment}}
		overlay := Kustomization{Name: "k", HealthChecks: []HealthCheckRef{{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "kyverno-admission-controller",
			Namespace:  "system-policy",
		}}}

		// When merged
		merged := MergeKustomizationFields(base, overlay)

		// Then the overlay reference replaces the base one rather than duplicating it
		if len(merged.HealthChecks) != 1 {
			t.Fatalf("Expected 1 health check reference, got %d", len(merged.HealthChecks))
		}
	})

	t.Run("OverlayReplacesMatchingReferenceRegardlessOfNamespace", func(t *testing.T) {
		// Given a base entry with Namespace left empty (defaults to the kustomization's own
		// namespace) and an overlay entry naming the same resource with Namespace spelled out
		base := Kustomization{Name: "k", HealthChecks: []HealthCheckRef{{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "kyverno-admission-controller",
		}}}
		overlay := Kustomization{Name: "k", HealthChecks: []HealthCheckRef{webhookDeployment}}

		// When merged
		merged := MergeKustomizationFields(base, overlay)

		// Then the two entries are recognized as the same resource and replace rather than
		// duplicate, since Namespace does not participate in the identity match
		if len(merged.HealthChecks) != 1 {
			t.Fatalf("Expected 1 health check reference, got %d", len(merged.HealthChecks))
		}
		if merged.HealthChecks[0].Namespace != webhookDeployment.Namespace {
			t.Errorf("Expected the overlay's namespace to win, got %q", merged.HealthChecks[0].Namespace)
		}
	})

	t.Run("OverlayAppendsUnmatchedReferenceAndLeavesBaseIntact", func(t *testing.T) {
		// Given a base naming one resource and an overlay naming another
		base := Kustomization{Name: "k", HealthChecks: []HealthCheckRef{webhookDeployment}}
		overlay := Kustomization{Name: "k", HealthChecks: []HealthCheckRef{{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "kyverno-background-controller",
			Namespace:  "system-policy",
		}}}

		// When merged
		merged := MergeKustomizationFields(base, overlay)

		// Then both references survive and the base slice is not mutated
		if len(merged.HealthChecks) != 2 {
			t.Fatalf("Expected 2 health check references, got %d", len(merged.HealthChecks))
		}
		if merged.HealthChecks[0].Name != "kyverno-admission-controller" || merged.HealthChecks[1].Name != "kyverno-background-controller" {
			t.Errorf("Expected admission-controller then background-controller, got %s then %s", merged.HealthChecks[0].Name, merged.HealthChecks[1].Name)
		}
		if len(base.HealthChecks) != 1 {
			t.Errorf("Expected base left intact, got %d references", len(base.HealthChecks))
		}
	})

	t.Run("EmptyOverlayLeavesBaseReferences", func(t *testing.T) {
		// Given a base with a health check reference and an overlay without one
		base := Kustomization{Name: "k", HealthChecks: []HealthCheckRef{webhookDeployment}}
		overlay := Kustomization{Name: "k"}

		// When merged
		merged := MergeKustomizationFields(base, overlay)

		// Then the base reference survives
		if len(merged.HealthChecks) != 1 || merged.HealthChecks[0].Name != webhookDeployment.Name {
			t.Errorf("Expected base reference preserved, got %+v", merged.HealthChecks)
		}
	})
}

func TestFluxSystem_TierNames(t *testing.T) {
	t.Run("ReturnsInstallThenResourcesVariantNames", func(t *testing.T) {
		sys := FluxSystem{
			Name:    "cert-manager",
			Install: &Kustomization{Components: []string{"helm-release"}},
			Resources: []FluxVariant{
				{Kustomization: Kustomization{Components: []string{"private-issuer/ca"}}},
				{Kustomization: Kustomization{Name: "extra", Components: []string{"public-issuer"}}},
			},
		}

		names := sys.TierNames()

		want := []string{"cert-manager-install", "cert-manager-resources", "cert-manager-resources-extra"}
		if len(names) != len(want) {
			t.Fatalf("expected %v, got %v", want, names)
		}
		for i := range want {
			if names[i] != want[i] {
				t.Errorf("expected %v, got %v", want, names)
				break
			}
		}
	})

	t.Run("ReturnsEmptyForSystemWithNoTiers", func(t *testing.T) {
		sys := FluxSystem{Name: "empty-sys"}

		if names := sys.TierNames(); len(names) != 0 {
			t.Errorf("expected no tier names for a system with no install or resources, got %v", names)
		}
	})

	t.Run("ReturnsInstallForADeclaredTierWithNoComponents", func(t *testing.T) {
		// A declared install tier renders even with no components: its own
		// kustomization.yaml can carry real content (a namespace, say) on its own.
		sys := FluxSystem{Name: "cert-manager", Install: &Kustomization{}}

		names := sys.TierNames()

		if len(names) != 1 || names[0] != "cert-manager-install" {
			t.Fatalf("expected [cert-manager-install], got %v", names)
		}
	})

	t.Run("DoesNotLeakAnUnrelatedSystemsNamePrefixCollision", func(t *testing.T) {
		// Given a system literally named "cert-manager-resources", distinct from "cert-manager"
		sys := FluxSystem{
			Name:    "cert-manager-resources",
			Install: &Kustomization{Components: []string{"x"}},
		}

		names := sys.TierNames()

		if len(names) != 1 || names[0] != "cert-manager-resources-install" {
			t.Fatalf("expected only cert-manager-resources-install, got %v", names)
		}
	})
}

func TestFluxSystem_EffectivePath(t *testing.T) {
	t.Run("ReturnsPathWhenSet", func(t *testing.T) {
		sys := FluxSystem{Name: "cert-manager", Path: "custom-path"}

		if got := sys.EffectivePath(); got != "custom-path" {
			t.Errorf("expected %q, got %q", "custom-path", got)
		}
	})

	t.Run("FallsBackToNameWhenPathUnset", func(t *testing.T) {
		sys := FluxSystem{Name: "cert-manager"}

		if got := sys.EffectivePath(); got != "cert-manager" {
			t.Errorf("expected %q, got %q", "cert-manager", got)
		}
	})
}

func TestBlueprint_AllKustomizations_InstallTierTimeoutDefault(t *testing.T) {
	t.Run("InstallTierFallsBackToInstallTimeoutDefaultWhenUnset", func(t *testing.T) {
		// Given a flux system whose install tier sets no timeout of its own
		bp := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name:      "cert-manager",
					Install:   &Kustomization{Components: []string{"helm-release"}},
					Resources: []FluxVariant{{Kustomization: Kustomization{Components: []string{"private-issuer/ca"}}}},
				},
			},
		}

		all := bp.AllKustomizations()

		// Then the compiled install tier gets the install-specific default timeout
		install := findKustomization(all, "cert-manager-install")
		if install == nil || install.Timeout == nil || install.Timeout.Duration != constants.DefaultFluxKustomizationInstallTimeout {
			t.Fatalf("expected cert-manager-install timeout %v, got %+v", constants.DefaultFluxKustomizationInstallTimeout, install)
		}

		// And the resources tier is left to ToFluxKustomization's own generic default
		resources := findKustomization(all, "cert-manager-resources")
		if resources == nil || resources.Timeout != nil {
			t.Fatalf("expected cert-manager-resources timeout unset, got %+v", resources)
		}
	})

	t.Run("ExplicitInstallTimeoutOverridesTheDefault", func(t *testing.T) {
		// Given a flux system whose install tier sets its own timeout
		bp := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name:    "lb",
					Install: &Kustomization{Components: []string{"aws-lb-controller"}, Timeout: &DurationString{Duration: 20 * time.Minute}},
				},
			},
		}

		all := bp.AllKustomizations()

		// Then the explicit value wins over the install-tier default
		install := findKustomization(all, "lb-install")
		if install == nil || install.Timeout == nil || install.Timeout.Duration != 20*time.Minute {
			t.Fatalf("expected lb-install timeout 20m, got %+v", install)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestCompileFluxSystemTiers_Secrets(t *testing.T) {
	secretsMap := func() map[string]SecretEntry {
		return map[string]SecretEntry{"cloudflare-creds": {Data: map[string]string{"api_token": "${cdn.cloudflare_api_key}"}}}
	}

	t.Run("AttachesSecretsToInstallTier", func(t *testing.T) {
		// Given a system with an install tier, a resources tier, and secrets
		bp := &Blueprint{FluxSystems: []FluxSystem{{
			Name:      "cdn",
			Install:   &Kustomization{Components: []string{"cloudflared"}},
			Resources: []FluxVariant{{Kustomization: Kustomization{Components: []string{"tunnel"}}}},
			Secrets:   secretsMap(),
		}}}

		// When compiling all kustomizations
		all := bp.AllKustomizations()

		// Then the install tier carries the secrets and the resources tier does not
		install := findKustomizationByName(all, "cdn-install")
		resources := findKustomizationByName(all, "cdn-resources")
		if install == nil || resources == nil {
			t.Fatalf("Expected both tiers, got %v", kustomizationNames(all))
		}
		if install.Secrets["cloudflare-creds"].Data["api_token"] != "${cdn.cloudflare_api_key}" {
			t.Errorf("Expected install tier to carry secrets, got %v", install.Secrets)
		}
		if len(resources.Secrets) != 0 {
			t.Errorf("Expected resources tier to carry no secrets, got %v", resources.Secrets)
		}
	})

	t.Run("FallsBackToResourcesTierWhenNoInstall", func(t *testing.T) {
		// Given a system with only a resources tier
		bp := &Blueprint{FluxSystems: []FluxSystem{{
			Name:      "cdn",
			Resources: []FluxVariant{{Kustomization: Kustomization{Components: []string{"tunnel"}}}},
			Secrets:   secretsMap(),
		}}}

		// When compiling all kustomizations
		all := bp.AllKustomizations()

		// Then the first (resources) tier carries the secrets
		resources := findKustomizationByName(all, "cdn-resources")
		if resources == nil {
			t.Fatalf("Expected resources tier, got %v", kustomizationNames(all))
		}
		if resources.Secrets["cloudflare-creds"].Data["api_token"] != "${cdn.cloudflare_api_key}" {
			t.Errorf("Expected resources tier to carry secrets, got %v", resources.Secrets)
		}
	})
}
