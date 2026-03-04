package blueprint

import (
	"os"
	"path/filepath"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupExplainHandler(t *testing.T, bp *blueprintv1alpha1.Blueprint, scope map[string]any, provenance map[string][]ProvenanceEntry) *BaseBlueprintHandler {
	t.Helper()
	rt := &runtime.Runtime{}
	proc := &BaseBlueprintProcessor{
		runtime:    rt,
		provenance: provenance,
	}
	if proc.provenance == nil {
		proc.provenance = make(map[string][]ProvenanceEntry)
	}
	return &BaseBlueprintHandler{
		runtime:           rt,
		processor:         proc,
		composedBlueprint: bp,
		composedScope:     scope,
	}
}

type explainTestProcessor struct{}

func (explainTestProcessor) ProcessFacets(target *blueprintv1alpha1.Blueprint, facets []blueprintv1alpha1.Facet, sourceName ...string) (map[string]any, []string, error) {
	return nil, nil, nil
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestParseExplainPath(t *testing.T) {
	t.Run("ParsesTerraformInputPath", func(t *testing.T) {
		// Given a terraform input path string
		path := "terraform.cluster.inputs.cluster_endpoint"

		// When parsing the path
		p, err := ParseExplainPath(path)

		// Then the path should be parsed as a terraform input
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Kind != ExplainPathKindTerraformInput {
			t.Errorf("expected kind %d, got %d", ExplainPathKindTerraformInput, p.Kind)
		}
		if p.Segment != "cluster" {
			t.Errorf("expected segment %q, got %q", "cluster", p.Segment)
		}
		if p.Key != "cluster_endpoint" {
			t.Errorf("expected key %q, got %q", "cluster_endpoint", p.Key)
		}
	})

	t.Run("ParsesKustomizeSubstitutionPath", func(t *testing.T) {
		// Given a kustomize substitution path string
		path := "kustomize.monitoring.substitutions.cluster_domain"

		// When parsing the path
		p, err := ParseExplainPath(path)

		// Then the path should be parsed as a kustomize substitution
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Kind != ExplainPathKindKustomizeSubstitution {
			t.Errorf("expected kind %d, got %d", ExplainPathKindKustomizeSubstitution, p.Kind)
		}
		if p.Segment != "monitoring" {
			t.Errorf("expected segment %q, got %q", "monitoring", p.Segment)
		}
		if p.Key != "cluster_domain" {
			t.Errorf("expected key %q, got %q", "cluster_domain", p.Key)
		}
	})

	t.Run("ParsesKustomizeComponentsPath", func(t *testing.T) {
		// Given a kustomize components path string
		path := "kustomize.monitoring.components"

		// When parsing the path
		p, err := ParseExplainPath(path)

		// Then the path should be parsed as kustomize components
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Kind != ExplainPathKindKustomizeComponents {
			t.Errorf("expected kind %d, got %d", ExplainPathKindKustomizeComponents, p.Kind)
		}
		if p.Segment != "monitoring" {
			t.Errorf("expected segment %q, got %q", "monitoring", p.Segment)
		}
		if p.Key != "" {
			t.Errorf("expected empty key, got %q", p.Key)
		}
	})

	t.Run("ParsesConfigMapPath", func(t *testing.T) {
		// Given a configMap path string
		path := "configMaps.values-common.CONTEXT"

		// When parsing the path
		p, err := ParseExplainPath(path)

		// Then the path should be parsed as a configMap
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Kind != ExplainPathKindConfigMap {
			t.Errorf("expected kind %d, got %d", ExplainPathKindConfigMap, p.Kind)
		}
		if p.Segment != "values-common" {
			t.Errorf("expected segment %q, got %q", "values-common", p.Segment)
		}
		if p.Key != "CONTEXT" {
			t.Errorf("expected key %q, got %q", "CONTEXT", p.Key)
		}
	})

	t.Run("ReturnsErrorForEmptyPath", func(t *testing.T) {
		// Given an empty path string
		path := ""

		// When parsing the path
		_, err := ParseExplainPath(path)

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error for empty path")
		}
	})

	t.Run("ReturnsErrorForMalformedPath", func(t *testing.T) {
		// Given a path with insufficient segments
		path := "terraform.cluster"

		// When parsing the path
		_, err := ParseExplainPath(path)

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error for malformed path")
		}
	})

	t.Run("ReturnsErrorForUnknownPrefix", func(t *testing.T) {
		// Given a path with an unrecognized prefix
		path := "unknown.foo.bar"

		// When parsing the path
		_, err := ParseExplainPath(path)

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error for unknown prefix")
		}
	})

	t.Run("TrimsWhitespace", func(t *testing.T) {
		// Given a path with leading and trailing space
		path := "  kustomize.monitoring.components  "

		// When parsing the path
		p, err := ParseExplainPath(path)

		// Then the path should be parsed as kustomize components
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Kind != ExplainPathKindKustomizeComponents || p.Segment != "monitoring" {
			t.Errorf("expected kustomize components monitoring, got kind=%d segment=%q", p.Kind, p.Segment)
		}
	})

	t.Run("ReturnsErrorForInvalidTerraformPath", func(t *testing.T) {
		// Given a terraform path with wrong middle segment (not "inputs")
		path := "terraform.cluster.outputs.endpoint"

		// When parsing the path
		_, err := ParseExplainPath(path)

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error for terraform path without inputs segment")
		}
	})

	t.Run("ReturnsErrorForInvalidKustomizePath", func(t *testing.T) {
		// Given a kustomize path with 4 segments but middle is not "substitutions"
		path := "kustomize.monitoring.foo.bar"

		// When parsing the path
		_, err := ParseExplainPath(path)

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error for kustomize path without substitutions or components")
		}
	})
}

func TestExplainPath_String(t *testing.T) {
	t.Run("ReturnsTerraformInputString", func(t *testing.T) {
		// Given a terraform input ExplainPath
		p := ExplainPath{Kind: ExplainPathKindTerraformInput, Segment: "cluster", Key: "endpoint"}

		// When converting to string
		s := p.String()

		// Then the canonical path string should be returned
		if s != "terraform.cluster.inputs.endpoint" {
			t.Errorf("expected %q, got %q", "terraform.cluster.inputs.endpoint", s)
		}
	})

	t.Run("ReturnsKustomizeSubstitutionString", func(t *testing.T) {
		// Given a kustomize substitution ExplainPath
		p := ExplainPath{Kind: ExplainPathKindKustomizeSubstitution, Segment: "monitoring", Key: "domain"}

		// When converting to string
		s := p.String()

		// Then the canonical path string should be returned
		if s != "kustomize.monitoring.substitutions.domain" {
			t.Errorf("expected %q, got %q", "kustomize.monitoring.substitutions.domain", s)
		}
	})

	t.Run("ReturnsKustomizeComponentsString", func(t *testing.T) {
		// Given a kustomize components ExplainPath
		p := ExplainPath{Kind: ExplainPathKindKustomizeComponents, Segment: "monitoring"}

		// When converting to string
		s := p.String()

		// Then the canonical path string should be returned
		if s != "kustomize.monitoring.components" {
			t.Errorf("expected %q, got %q", "kustomize.monitoring.components", s)
		}
	})

	t.Run("ReturnsConfigMapString", func(t *testing.T) {
		// Given a configMap ExplainPath
		p := ExplainPath{Kind: ExplainPathKindConfigMap, Segment: "values-common", Key: "CONTEXT"}

		// When converting to string
		s := p.String()

		// Then the canonical path string should be returned
		if s != "configMaps.values-common.CONTEXT" {
			t.Errorf("expected %q, got %q", "configMaps.values-common.CONTEXT", s)
		}
	})

	t.Run("ReturnsEmptyStringForUnknownKind", func(t *testing.T) {
		// Given an ExplainPath with an unknown kind value (default branch in String)
		p := ExplainPath{Kind: ExplainPathKind(99), Segment: "x", Key: "y"}

		// When converting to string
		s := p.String()

		// Then an empty string should be returned
		if s != "" {
			t.Errorf("expected empty string, got %q", s)
		}
	})
}

func TestExplain(t *testing.T) {
	t.Run("ResolvesLiteralTerraformInput", func(t *testing.T) {
		// Given a handler with a composed blueprint containing a literal terraform input
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "networking", Inputs: map[string]any{"domain_name": "example.com"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.networking": {{
				FacetPath:  "/tmp/facets/base.yaml",
				SourceName: "template",
				Ordinal:    100,
				Strategy:   "merge",
				RawInputs:  map[string]any{"domain_name": "example.com"},
			}},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining the terraform input path
		trace, err := h.Explain("terraform.networking.inputs.domain_name")

		// Then the trace should resolve the literal value
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if trace.Value != "example.com" {
			t.Errorf("expected value %q, got %q", "example.com", trace.Value)
		}
		if len(trace.Contributions) == 0 {
			t.Fatal("expected at least one contribution")
		}
		if trace.Contributions[0].SourceName != "template" {
			t.Errorf("expected source %q, got %q", "template", trace.Contributions[0].SourceName)
		}
	})

	t.Run("ResolvesExpressionTerraformInput", func(t *testing.T) {
		// Given a handler with a composed blueprint containing an expression-derived input
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "networking", Inputs: map[string]any{"cidr_block": "10.0.0.0/16"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.networking": {{
				FacetPath:  "/tmp/facets/base.yaml",
				SourceName: "template",
				Ordinal:    100,
				Strategy:   "merge",
				RawInputs:  map[string]any{"cidr_block": "${network.cidr_block}"},
			}},
		}
		scope := map[string]any{
			"network": map[string]any{"cidr_block": "10.0.0.0/16"},
		}
		h := setupExplainHandler(t, bp, scope, prov)

		// When explaining the expression input
		trace, err := h.Explain("terraform.networking.inputs.cidr_block")

		// Then the trace should include the expression
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if trace.Value != "10.0.0.0/16" {
			t.Errorf("expected value %q, got %q", "10.0.0.0/16", trace.Value)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatal("expected an effective contribution")
		}
		if effective.Expression != "${network.cidr_block}" {
			t.Errorf("expected expression %q, got %q", "${network.cidr_block}", effective.Expression)
		}
	})

	t.Run("ResolvesDeferredTerraformInput", func(t *testing.T) {
		// Given a handler with a composed blueprint containing a deferred expression
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Inputs: map[string]any{"endpoint": "${cluster.endpoint ?? fallback.endpoint}"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.cluster": {{
				FacetPath:  "/tmp/facets/provider.yaml",
				SourceName: "template",
				Ordinal:    200,
				Strategy:   "merge",
				RawInputs:  map[string]any{"endpoint": "${cluster.endpoint ?? fallback.endpoint}"},
			}},
			"config.fallback.endpoint": {{
				FacetPath: "/tmp/facets/provider.yaml",
				Line:      12,
			}},
		}
		scope := map[string]any{
			"fallback": map[string]any{"endpoint": "${terraform_output(\"compute\")}"},
		}
		h := setupExplainHandler(t, bp, scope, prov)

		// When explaining the deferred input
		trace, err := h.Explain("terraform.cluster.inputs.endpoint")

		// Then the value should contain the unresolved expression
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if trace.Value != "${cluster.endpoint ?? fallback.endpoint}" {
			t.Errorf("expected deferred expression as value, got %q", trace.Value)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatal("expected an effective contribution")
		}
		hasDeferredRef := false
		for _, ref := range effective.ScopeRefs {
			if ref.Name == "fallback.endpoint" && ref.Status == "deferred" {
				hasDeferredRef = true
			}
		}
		if !hasDeferredRef {
			t.Error("expected a deferred scope ref for fallback.endpoint")
		}
	})

	t.Run("ResolvesExpressionWithTernary", func(t *testing.T) {
		// Given an expression using a ternary conditional
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"out": "yes"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x": {{
				FacetPath: "/tmp/f.yaml",
				RawInputs: map[string]any{"out": "${addons.enabled ? 'yes' : 'no'}"},
			}},
		}
		scope := map[string]any{"addons": map[string]any{"enabled": true}}
		h := setupExplainHandler(t, bp, scope, prov)

		// When explaining the input
		trace, err := h.Explain("terraform.x.inputs.out")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if trace.Value != "yes" {
			t.Errorf("expected value %q, got %q", "yes", trace.Value)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatal("expected an effective contribution")
		}
		var found bool
		for _, ref := range effective.ScopeRefs {
			if ref.Name == "addons.enabled" {
				found = true
				break
			}
		}
		// Then the trace should resolve and include the scope ref from the ternary
		if !found {
			t.Error("expected scope ref addons.enabled from ternary expression")
		}
	})

	t.Run("ResolvesExpressionWithRefInCallArg", func(t *testing.T) {
		// Given an expression with a function call whose argument is a scope ref
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"out": "10.0.0.1"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x": {{
				FacetPath: "/tmp/f.yaml",
				RawInputs: map[string]any{"out": "${env(network.ip)}"},
			}},
		}
		scope := map[string]any{"network": map[string]any{"ip": "IP_ADDR"}}
		h := setupExplainHandler(t, bp, scope, prov)

		// When explaining the input
		trace, err := h.Explain("terraform.x.inputs.out")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatal("expected an effective contribution")
		}
		var found bool
		for _, ref := range effective.ScopeRefs {
			if ref.Name == "network.ip" {
				found = true
				break
			}
		}
		// Then the trace should include the scope ref from the call argument
		if !found {
			t.Error("expected scope ref network.ip from call argument")
		}
	})

	t.Run("ResolvesExpressionWithLet", func(t *testing.T) {
		// Given an expression using let that references config
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"out": "resolved"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x": {{
				FacetPath: "/tmp/f.yaml",
				RawInputs: map[string]any{"out": "${let x = config.base; x}"},
			}},
		}
		scope := map[string]any{"config": map[string]any{"base": "resolved"}}
		h := setupExplainHandler(t, bp, scope, prov)

		// When explaining the input
		trace, err := h.Explain("terraform.x.inputs.out")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatal("expected an effective contribution")
		}
		var found bool
		for _, ref := range effective.ScopeRefs {
			if ref.Name == "config.base" {
				found = true
				break
			}
		}
		// Then the trace should include the scope ref from the let expression
		if !found {
			t.Error("expected scope ref config.base from let expression")
		}
	})

	t.Run("ResolvesExpressionWithScopeRefToMap", func(t *testing.T) {
		// Given an expression that references a scope ref resolving to a map
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"config": "${refs.block}"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x": {{FacetPath: "/tmp/f.yaml", RawInputs: map[string]any{"config": "${refs.block}"}}},
		}
		scope := map[string]any{
			"refs": map[string]any{"block": map[string]any{"key": "val"}},
		}
		h := setupExplainHandler(t, bp, scope, prov)

		// When explaining the input (value is resolved from scope map)
		trace, err := h.Explain("terraform.x.inputs.config")

		// Then the trace should be produced without error and value should be present
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if trace.Value != "map[key:val]" && trace.Value != "{\"key\":\"val\"}" {
			// Value is fmt'd map from evaluator
			if trace.Value == "" {
				t.Error("expected non-empty value")
			}
		}
	})

	t.Run("ExpandScopeRefNotSet", func(t *testing.T) {
		// Given an expression referencing a path whose root is in scope but path is not set
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"out": ""}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x": {{FacetPath: "/tmp/f.yaml", RawInputs: map[string]any{"out": "${missing.nested}"}}},
		}
		scope := map[string]any{"missing": map[string]any{}}
		h := setupExplainHandler(t, bp, scope, prov)

		// When explaining the input
		trace, err := h.Explain("terraform.x.inputs.out")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatal("expected an effective contribution")
		}
		var ref *ExplainScopeRef
		for i := range effective.ScopeRefs {
			if effective.ScopeRefs[i].Name == "missing.nested" {
				ref = &effective.ScopeRefs[i]
				break
			}
		}
		// Then the scope ref should have status "not set"
		if ref == nil {
			t.Fatal("expected scope ref missing.nested")
		}
		if ref.Status != "not set" {
			t.Errorf("expected status %q, got %q", "not set", ref.Status)
		}
	})

	t.Run("ExpandScopeRefCycle", func(t *testing.T) {
		// Given an expression where a ref's raw value references itself (cycle)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"out": "val"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x": {{FacetPath: "/tmp/f.yaml", RawInputs: map[string]any{"out": "${self}"}}},
			"config.self": {{FacetPath: "/tmp/f.yaml", RawConfigValue: "${self}"}},
		}
		scope := map[string]any{"self": "val"}
		h := setupExplainHandler(t, bp, scope, prov)

		// When explaining the input
		trace, err := h.Explain("terraform.x.inputs.out")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatal("expected an effective contribution")
		}
		var selfRef *ExplainScopeRef
		for i := range effective.ScopeRefs {
			if effective.ScopeRefs[i].Name == "self" {
				selfRef = &effective.ScopeRefs[i]
				break
			}
		}
		if selfRef == nil {
			t.Fatal("expected scope ref self")
		}
		var cycleInNested bool
		for _, n := range selfRef.Nested {
			if n.Name == "self" && n.Status == "cycle" {
				cycleInNested = true
				break
			}
		}
		// Then the nested ref should show status cycle
		if !cycleInNested {
			t.Error("expected nested ref self with status cycle")
		}
	})

	t.Run("ExpandScopeRefMapWithNestedExpr", func(t *testing.T) {
		// Given an expression referencing a ref whose raw value is a map containing nested expressions
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"out": "resolved"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x":       {{FacetPath: "/tmp/f.yaml", RawInputs: map[string]any{"out": "${refs.block}"}}},
			"config.refs.block": {{FacetPath: "/tmp/f.yaml", RawConfigValue: map[string]any{"inner": "${other.ref}"}}},
		}
		scope := map[string]any{
			"refs":  map[string]any{"block": map[string]any{"inner": "resolved"}},
			"other": map[string]any{"ref": "resolved"},
		}
		h := setupExplainHandler(t, bp, scope, prov)

		// When explaining the input
		trace, err := h.Explain("terraform.x.inputs.out")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatal("expected an effective contribution")
		}
		var blockRef *ExplainScopeRef
		for i := range effective.ScopeRefs {
			if effective.ScopeRefs[i].Name == "refs.block" {
				blockRef = &effective.ScopeRefs[i]
				break
			}
		}
		if blockRef == nil {
			t.Fatal("expected scope ref refs.block")
		}
		var found bool
		for _, n := range blockRef.Nested {
			if n.Name == "refs.block.inner" {
				found = true
				break
			}
		}
		// Then the nested ref should be expanded from the map-with-expr
		if !found {
			t.Error("expected nested ref refs.block.inner from map-with-expr expansion")
		}
	})

	t.Run("ExpandScopeRefNilValue", func(t *testing.T) {
		// Given a ref that is in scope but value is nil and no provenance
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"out": ""}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x": {{FacetPath: "/tmp/f.yaml", RawInputs: map[string]any{"out": "${nilkey}"}}},
		}
		scope := map[string]any{"nilkey": nil}
		h := setupExplainHandler(t, bp, scope, prov)

		// When explaining the input
		trace, err := h.Explain("terraform.x.inputs.out")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatal("expected an effective contribution")
		}
		var found bool
		for _, ref := range effective.ScopeRefs {
			if ref.Name == "nilkey" {
				found = true
				break
			}
		}
		// Then the scope ref should still be present
		if !found {
			t.Error("expected scope ref nilkey")
		}
	})

	t.Run("ResolvesKustomizeSubstitution", func(t *testing.T) {
		// Given a handler with a composed blueprint containing a kustomize substitution
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "monitoring", Substitutions: map[string]string{"domain": "test.example.com"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"kustomize.monitoring": {{
				FacetPath:  "/tmp/facets/base.yaml",
				SourceName: "template",
				Ordinal:    100,
				Strategy:   "merge",
				RawSubs:    map[string]string{"domain": "${dns.domain}"},
			}},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining the substitution
		trace, err := h.Explain("kustomize.monitoring.substitutions.domain")

		// Then the trace should resolve the substitution value
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if trace.Value != "test.example.com" {
			t.Errorf("expected value %q, got %q", "test.example.com", trace.Value)
		}
	})

	t.Run("ResolvesKustomizeComponents", func(t *testing.T) {
		// Given a handler with a composed blueprint containing kustomize components
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "monitoring", Components: []string{"prometheus", "grafana"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"kustomize.monitoring": {{
				FacetPath:     "/tmp/facets/base.yaml",
				SourceName:    "template",
				Ordinal:       100,
				Strategy:      "merge",
				RawComponents: []string{"prometheus", "grafana"},
			}},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining the components list
		trace, err := h.Explain("kustomize.monitoring.components")

		// Then the trace should list each component as a contribution
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(trace.Contributions) != 2 {
			t.Fatalf("expected 2 contributions, got %d", len(trace.Contributions))
		}
		if trace.Contributions[0].Expression != "prometheus" {
			t.Errorf("expected first component %q, got %q", "prometheus", trace.Contributions[0].Expression)
		}
		if trace.Contributions[1].Expression != "grafana" {
			t.Errorf("expected second component %q, got %q", "grafana", trace.Contributions[1].Expression)
		}
	})

	t.Run("ExplainResolvesLineFromRealFacetFileForComponents", func(t *testing.T) {
		// Given a real facet YAML file on disk with kustomize components
		dir := t.TempDir()
		facetPath := filepath.Join(dir, "facet.yaml")
		facetYAML := `kustomize:
  - name: monitoring
    path: monitoring
    components:
      - prometheus
      - grafana
`
		if err := os.WriteFile(facetPath, []byte(facetYAML), 0644); err != nil {
			t.Fatalf("write facet: %v", err)
		}
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "monitoring", Components: []string{"prometheus", "grafana"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"kustomize.monitoring": {{
				FacetPath:     facetPath,
				SourceName:    "template",
				RawComponents: []string{"prometheus", "grafana"},
			}},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining the components path
		trace, err := h.Explain("kustomize.monitoring.components")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Then each contribution should have a non-zero line from the facet file
		if len(trace.Contributions) != 2 {
			t.Fatalf("expected 2 contributions, got %d", len(trace.Contributions))
		}
		if trace.Contributions[0].Line == 0 {
			t.Error("expected non-zero line from real facet file for prometheus")
		}
		if trace.Contributions[1].Line == 0 {
			t.Error("expected non-zero line from real facet file for grafana")
		}
	})

	t.Run("ExplainResolvesLineFromRealFacetFileForTerraformInput", func(t *testing.T) {
		// Given a real facet YAML file with terraform/inputs
		dir := t.TempDir()
		facetPath := filepath.Join(dir, "facet.yaml")
		facetYAML := `terraform:
  - name: cluster
    inputs:
      domain_name: example.com
`
		if err := os.WriteFile(facetPath, []byte(facetYAML), 0644); err != nil {
			t.Fatalf("write facet: %v", err)
		}
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Inputs: map[string]any{"domain_name": "example.com"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.cluster": {{
				FacetPath:  facetPath,
				SourceName: "template",
				RawInputs:  map[string]any{"domain_name": "example.com"},
			}},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining the terraform input path
		trace, err := h.Explain("terraform.cluster.inputs.domain_name")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Then the contribution should have a non-zero line from the facet file
		if len(trace.Contributions) == 0 {
			t.Fatal("expected at least one contribution")
		}
		if trace.Contributions[0].Line == 0 {
			t.Error("expected non-zero line from real facet file for domain_name key")
		}
	})

	t.Run("ExplainResolvesNestedConfigKeyLineFromRealFacetFile", func(t *testing.T) {
		// Given a real facet with config block and nested value map, and provenance for the parent key
		dir := t.TempDir()
		facetPath := filepath.Join(dir, "facet.yaml")
		facetYAML := `config:
  - name: cluster
    value:
      nested:
        value: val
`
		if err := os.WriteFile(facetPath, []byte(facetYAML), 0644); err != nil {
			t.Fatalf("write facet: %v", err)
		}
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"domain": "${cluster.nested.value}"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x": {{
				FacetPath:  facetPath,
				SourceName: "template",
				RawInputs:  map[string]any{"domain": "${cluster.nested.value}"},
			}},
			"config.cluster.nested": {{
				FacetPath:      facetPath,
				Line:           1,
				RawConfigValue: map[string]any{"value": "val"},
			}},
		}
		scope := map[string]any{
			"cluster": map[string]any{
				"nested": map[string]any{"value": "val"},
			},
		}
		h := setupExplainHandler(t, bp, scope, prov)

		// When explaining the input
		trace, err := h.Explain("terraform.x.inputs.domain")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatal("expected an effective contribution")
		}
		var scopeRef *ExplainScopeRef
		for i := range effective.ScopeRefs {
			if effective.ScopeRefs[i].Name == "cluster.nested.value" {
				scopeRef = &effective.ScopeRefs[i]
				break
			}
		}
		// Then the scope ref should have a non-zero line from the nested key
		if scopeRef == nil {
			t.Fatal("expected scope ref cluster.nested.value")
		}
		if scopeRef.Line == 0 {
			t.Error("expected non-zero Line from resolveNestedConfigLine with real facet (astMapKeyLine)")
		}
	})

	t.Run("ExplainResolvesComponentLineWhenEntryMatchesExpressionPathPrefix", func(t *testing.T) {
		// Given a real facet with components list containing an expression with path-prefix literal
		dir := t.TempDir()
		facetPath := filepath.Join(dir, "facet.yaml")
		facetYAML := `kustomize:
  - name: monitoring
    path: monitoring
    components:
      - ${'base/' + base}
`
		if err := os.WriteFile(facetPath, []byte(facetYAML), 0644); err != nil {
			t.Fatalf("write facet: %v", err)
		}
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "monitoring", Components: []string{"base/prometheus"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"kustomize.monitoring": {{
				FacetPath:     facetPath,
				SourceName:    "template",
				RawComponents: []string{"${'base/' + base}"},
			}},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining the components path
		trace, err := h.Explain("kustomize.monitoring.components")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Then the contribution should have a non-zero line from the expression match
		if len(trace.Contributions) != 1 {
			t.Fatalf("expected 1 contribution, got %d", len(trace.Contributions))
		}
		if trace.Contributions[0].Line == 0 {
			t.Error("expected non-zero line from expression path-prefix match (astSeqEntryMatch expression branch)")
		}
	})

	t.Run("ResolvesConfigMapValue", func(t *testing.T) {
		// Given a handler with a composed blueprint containing a configMap
		bp := &blueprintv1alpha1.Blueprint{
			ConfigMaps: map[string]map[string]string{
				"values-common": {"CONTEXT": "default"},
			},
		}
		h := setupExplainHandler(t, bp, nil, nil)

		// When explaining the configMap value
		trace, err := h.Explain("configMaps.values-common.CONTEXT")

		// Then the trace should resolve the configMap value
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if trace.Value != "default" {
			t.Errorf("expected value %q, got %q", "default", trace.Value)
		}
		if len(trace.Contributions) == 0 {
			t.Fatal("expected at least one contribution")
		}
		if trace.Contributions[0].SourceName != "composition (runtime config)" {
			t.Errorf("expected source %q, got %q", "composition (runtime config)", trace.Contributions[0].SourceName)
		}
	})

	t.Run("ReturnsErrorForMissingComponent", func(t *testing.T) {
		// Given a handler with an empty composed blueprint
		bp := &blueprintv1alpha1.Blueprint{}
		h := setupExplainHandler(t, bp, nil, nil)

		// When explaining a nonexistent terraform component
		_, err := h.Explain("terraform.nonexistent.inputs.foo")

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error for missing component")
		}
	})

	t.Run("ReturnsErrorWhenBlueprintNotComposed", func(t *testing.T) {
		// Given a handler with no composed blueprint
		h := setupExplainHandler(t, nil, nil, nil)

		// When explaining any path
		_, err := h.Explain("terraform.cluster.inputs.endpoint")

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error when blueprint not composed")
		}
	})

	t.Run("MarksEffectiveContributor", func(t *testing.T) {
		// Given a handler with two provenance entries where the second replaces the first
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "networking", Inputs: map[string]any{"domain_name": "override.com"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.networking": {
				{
					FacetPath:  "/tmp/facets/base.yaml",
					SourceName: "template",
					Ordinal:    100,
					Strategy:   "merge",
					RawInputs:  map[string]any{"domain_name": "base.com"},
				},
				{
					FacetPath:  "/tmp/facets/override.yaml",
					SourceName: "template",
					Ordinal:    200,
					Strategy:   "replace",
					RawInputs:  map[string]any{"domain_name": "override.com"},
				},
			},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining the input
		trace, err := h.Explain("terraform.networking.inputs.domain_name")

		// Then the replace contribution should be marked effective
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(trace.Contributions) != 2 {
			t.Fatalf("expected 2 contributions, got %d", len(trace.Contributions))
		}
		if trace.Contributions[0].Effective {
			t.Error("first contribution (lower ordinal merge) should not be effective")
		}
		if !trace.Contributions[1].Effective {
			t.Error("second contribution (higher ordinal replace) should be effective")
		}
	})

	t.Run("ResolvesTerraformInputWithNestedKey", func(t *testing.T) {
		// Given a handler with a terraform input using a dotted key
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"nested": map[string]any{"deep": map[string]any{"key": "val"}}}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x": {{FacetPath: "/tmp/f.yaml", RawInputs: map[string]any{"nested": map[string]any{"deep": map[string]any{"key": "val"}}}}},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining the nested key path
		trace, err := h.Explain("terraform.x.inputs.nested.deep.key")

		// Then the trace should resolve the nested value
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if trace.Value != "val" {
			t.Errorf("expected value %q, got %q", "val", trace.Value)
		}
	})

	t.Run("ResolvesTerraformInputWithMapValue", func(t *testing.T) {
		// Given a handler with a terraform input whose value is a map (hits formatValue map branch)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"tags": map[string]any{"env": "test", "role": "web"}}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x": {{FacetPath: "/tmp/f.yaml", RawInputs: map[string]any{"tags": map[string]any{"env": "test"}}}},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining the map input
		trace, err := h.Explain("terraform.x.inputs.tags")

		// Then the trace should format the map (JSON, possibly truncated)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if trace.Value == "" {
			t.Error("expected non-empty value for map input")
		}
	})

	t.Run("ResolvesKustomizeComponentsWithExpressionDerivedEntry", func(t *testing.T) {
		// Given a handler where a resolved component came from an expression in RawComponents
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "lb", Components: []string{"fluentd"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"kustomize.lb": {{
				FacetPath:     "/tmp/f.yaml",
				SourceName:    "template",
				RawComponents: []string{"${'fluentd'}"},
			}},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining the components list
		trace, err := h.Explain("kustomize.lb.components")

		// Then the entry should be attributed to the facet with the expression
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(trace.Contributions) != 1 {
			t.Fatalf("expected 1 contribution, got %d", len(trace.Contributions))
		}
		if trace.Contributions[0].SourceName != "template" {
			t.Errorf("expected source %q, got %q", "template", trace.Contributions[0].SourceName)
		}
		if trace.Contributions[0].Expression != "fluentd" {
			t.Errorf("expected expression %q, got %q", "fluentd", trace.Contributions[0].Expression)
		}
	})

	t.Run("ResolvesKustomizeComponentsWithUnmatchedEntry", func(t *testing.T) {
		// Given a handler where one resolved component has no matching provenance
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "lb", Components: []string{"prometheus", "unknown-component"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"kustomize.lb": {{
				FacetPath:     "/tmp/f.yaml",
				RawComponents: []string{"prometheus"},
			}},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining the components list
		trace, err := h.Explain("kustomize.lb.components")

		// Then the unmatched entry should show as "composed blueprint"
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(trace.Contributions) != 2 {
			t.Fatalf("expected 2 contributions, got %d", len(trace.Contributions))
		}
		var composed *ExplainContribution
		for i := range trace.Contributions {
			if trace.Contributions[i].SourceName == "composed blueprint" {
				composed = &trace.Contributions[i]
				break
			}
		}
		if composed == nil {
			t.Fatal("expected one contribution with source \"composed blueprint\"")
		}
		if composed.Expression != "unknown-component" {
			t.Errorf("expected expression %q, got %q", "unknown-component", composed.Expression)
		}
	})

	t.Run("ExplainWithNonProvenanceProcessor", func(t *testing.T) {
		// Given a handler whose processor is not BaseBlueprintProcessor (e.g. mock)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"key": "val"}},
			},
		}
		h := &BaseBlueprintHandler{
			runtime:           &runtime.Runtime{},
			processor:         explainTestProcessor{},
			composedBlueprint: bp,
			composedScope:     nil,
		}

		// When explaining the input
		trace, err := h.Explain("terraform.x.inputs.key")

		// Then we still get a trace with "composed blueprint" as the contribution
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if trace.Value != "val" {
			t.Errorf("expected value %q, got %q", "val", trace.Value)
		}
		if len(trace.Contributions) != 1 || trace.Contributions[0].SourceName != "composed blueprint" {
			t.Errorf("expected single contribution \"composed blueprint\", got %v", trace.Contributions)
		}
	})

	t.Run("ReturnsErrorForMissingKustomization", func(t *testing.T) {
		// Given a handler with a blueprint that has no kustomizations
		bp := &blueprintv1alpha1.Blueprint{}
		h := setupExplainHandler(t, bp, nil, nil)

		// When explaining a nonexistent kustomization
		_, err := h.Explain("kustomize.nonexistent.components")

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error for missing kustomization")
		}
	})

	t.Run("KustomizationWithNilSubstitutions", func(t *testing.T) {
		// Given a handler with a kustomization that has nil substitutions
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "monitoring", Substitutions: nil},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"kustomize.monitoring": {{FacetPath: "/tmp/f.yaml", SourceName: "template"}},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining any substitution key
		trace, err := h.Explain("kustomize.monitoring.substitutions.domain")

		// Then the trace value is empty and no error
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if trace.Value != "" {
			t.Errorf("expected empty value, got %q", trace.Value)
		}
	})

	t.Run("KustomizationSubstitutionKeyMissing", func(t *testing.T) {
		// Given a handler with a kustomization whose substitutions omit the requested key
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "monitoring", Substitutions: map[string]string{"other": "val"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"kustomize.monitoring": {{FacetPath: "/tmp/f.yaml", RawSubs: map[string]string{"other": "val"}}},
		}
		h := setupExplainHandler(t, bp, nil, prov)

		// When explaining the missing substitution key
		trace, err := h.Explain("kustomize.monitoring.substitutions.missing")

		// Then the trace value is empty and no error
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if trace.Value != "" {
			t.Errorf("expected empty value, got %q", trace.Value)
		}
	})

	t.Run("ConfigMapNotFound", func(t *testing.T) {
		// Given a handler with a blueprint that has no such configMap
		bp := &blueprintv1alpha1.Blueprint{
			ConfigMaps: map[string]map[string]string{"other": {}},
		}
		h := setupExplainHandler(t, bp, nil, nil)

		// When explaining a nonexistent configMap name
		_, err := h.Explain("configMaps.nonexistent.KEY")

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error for configMap not found")
		}
	})

	t.Run("ConfigMapKeyNotFound", func(t *testing.T) {
		// Given a handler with a configMap that does not contain the key
		bp := &blueprintv1alpha1.Blueprint{
			ConfigMaps: map[string]map[string]string{
				"values-common": {"OTHER": "val"},
			},
		}
		h := setupExplainHandler(t, bp, nil, nil)

		// When explaining a missing key
		_, err := h.Explain("configMaps.values-common.MISSING")

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error for configMap key not found")
		}
	})

	t.Run("TerraformComponentHasNoInputs", func(t *testing.T) {
		// Given a handler with a terraform component that has nil inputs
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Inputs: nil},
			},
		}
		h := setupExplainHandler(t, bp, nil, nil)

		// When explaining an input path
		_, err := h.Explain("terraform.cluster.inputs.endpoint")

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error when component has no inputs")
		}
	})

	t.Run("ResolvesNestedConfigLineWhenScopeRefIsDeeperThanProvenance", func(t *testing.T) {
		// Given an expression that references a nested config path and provenance only has the parent key
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"domain": "val"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x": {{
				FacetPath:  "/tmp/f.yaml",
				SourceName: "template",
				RawInputs:  map[string]any{"domain": "${cluster.nested.deep}"},
			}},
			"config.cluster.nested": {{
				FacetPath:      "/tmp/f.yaml",
				Line:           10,
				RawConfigValue: map[string]any{"deep": "val"},
			}},
		}
		scope := map[string]any{
			"cluster": map[string]any{
				"nested": map[string]any{"deep": "val"},
			},
		}
		h := setupExplainHandler(t, bp, scope, prov)

		// When explaining the input (resolveScopeRefs expands the nested ref)
		trace, err := h.Explain("terraform.x.inputs.domain")

		// Then the trace should resolve and the scope ref should be present
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatal("expected an effective contribution")
		}
		var found bool
		for _, ref := range effective.ScopeRefs {
			if ref.Name == "cluster.nested.deep" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected scope ref cluster.nested.deep")
		}
	})

	t.Run("ExplainUsesTraceCollectorWhenSet", func(t *testing.T) {
		// Given a handler with a pre-loaded trace collector and an expression that references scope
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "x", Inputs: map[string]any{"domain": "resolved.example.com"}},
			},
		}
		prov := map[string][]ProvenanceEntry{
			"terraform.x": {{
				FacetPath:  "/tmp/f.yaml",
				SourceName: "template",
				RawInputs:  map[string]any{"domain": "${dns.domain}"},
			}},
		}
		scope := map[string]any{"dns": map[string]any{"domain": "resolved.example.com"}}
		collector := NewTraceCollector()
		collector.Record(SourceLocation{DocumentPath: "terraform.x.inputs.domain"}, []string{"dns.domain"}, nil)
		h := setupExplainHandler(t, bp, scope, prov)
		h.SetTraceCollector(collector)

		// When explaining the input
		trace, err := h.Explain("terraform.x.inputs.domain")

		// Then the trace resolves and scope refs come from the collector
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		effective := findEffective(trace.Contributions)
		if effective == nil {
			t.Fatal("expected an effective contribution")
		}
		var found bool
		for _, ref := range effective.ScopeRefs {
			if ref.Name == "dns.domain" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected scope ref dns.domain from collector")
		}
	})
}

// =============================================================================
// Test DefaultTraceCollector (public API)
// =============================================================================

func TestNewTraceCollector(t *testing.T) {
	t.Run("ReturnsInitializedCollector", func(t *testing.T) {
		// When creating a new trace collector
		c := NewTraceCollector()

		// Then the collector should be non-nil and return empty refs/paths for unknown keys
		if c == nil {
			t.Fatal("expected non-nil collector")
		}
		if refs := c.GetScopeRefs("any.path"); refs != nil {
			t.Errorf("expected nil scope refs, got %v", refs)
		}
		if paths := c.GetNestedPaths("any.path"); paths != nil {
			t.Errorf("expected nil nested paths, got %v", paths)
		}
	})
}

func TestDefaultTraceCollector_Record(t *testing.T) {
	t.Run("StoresScopeRefsAndNestedPaths", func(t *testing.T) {
		// Given a trace collector
		c := NewTraceCollector()
		loc := SourceLocation{FacetPath: "/f.yaml", DocumentPath: "config.cluster.endpoint"}

		// When recording scope refs and nested paths
		c.Record(loc, []string{"dns.domain"}, []string{"cluster", "endpoint"})

		// Then GetScopeRefs and GetNestedPaths return the stored data
		refs := c.GetScopeRefs(loc.DocumentPath)
		if len(refs) != 1 || refs[0] != "dns.domain" {
			t.Errorf("expected [dns.domain], got %v", refs)
		}
		paths := c.GetNestedPaths(loc.DocumentPath)
		if len(paths) != 2 || paths[0] != "cluster" || paths[1] != "endpoint" {
			t.Errorf("expected [cluster endpoint], got %v", paths)
		}
	})

	t.Run("IgnoresEmptySlices", func(t *testing.T) {
		// Given a trace collector
		c := NewTraceCollector()
		loc := SourceLocation{DocumentPath: "config.x"}

		// When recording with empty scope refs and nested paths
		c.Record(loc, nil, nil)

		// Then nothing is stored
		if c.GetScopeRefs(loc.DocumentPath) != nil {
			t.Error("expected nil scope refs")
		}
		if c.GetNestedPaths(loc.DocumentPath) != nil {
			t.Error("expected nil nested paths")
		}
	})
}

func TestDefaultTraceCollector_RecordValue(t *testing.T) {
	t.Run("ExtractsAndStoresRefsFromStringExpression", func(t *testing.T) {
		// Given a trace collector
		c := NewTraceCollector()
		loc := SourceLocation{DocumentPath: "terraform.x.inputs.domain"}

		// When recording a string value containing an expression
		c.RecordValue(loc, "${dns.domain}")

		// Then scope refs are stored for that path
		refs := c.GetScopeRefs(loc.DocumentPath)
		if len(refs) == 0 {
			t.Error("expected at least one scope ref extracted from expression")
		}
	})

	t.Run("IgnoresNilValue", func(t *testing.T) {
		// Given a trace collector
		c := NewTraceCollector()
		loc := SourceLocation{DocumentPath: "config.x"}

		// When recording nil
		c.RecordValue(loc, nil)

		// Then nothing is stored
		if c.GetScopeRefs(loc.DocumentPath) != nil {
			t.Error("expected nil scope refs")
		}
	})

	t.Run("StoresNestedPathsFromMapWithExpressions", func(t *testing.T) {
		// Given a trace collector
		c := NewTraceCollector()
		loc := SourceLocation{DocumentPath: "config.block"}

		// When recording a map that contains a nested string with an expression
		c.RecordValue(loc, map[string]any{
			"nested": map[string]any{
				"key": "${some.ref}",
			},
		})

		// Then nested paths are stored and child expression refs are recorded
		paths := c.GetNestedPaths(loc.DocumentPath)
		if len(paths) == 0 {
			t.Error("expected nested paths for map with expression")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func findEffective(contributions []ExplainContribution) *ExplainContribution {
	for i := range contributions {
		if contributions[i].Effective {
			return &contributions[i]
		}
	}
	return nil
}
