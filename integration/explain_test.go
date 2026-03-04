//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

func TestExplain(t *testing.T) {
	t.Run("ConfigMapLiteralValue", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		stdout, stderr, err := helpers.RunCLI(dir, []string{"explain", "configMaps.values-common.CONTEXT"}, env)
		if err != nil {
			t.Fatalf("explain failed: %v\nstderr: %s", err, stderr)
		}
		assertExplainOutput(t, string(stdout), explainExpectation{
			header:         "configMaps.values-common.CONTEXT = default",
			sourceContains: "composition (runtime config)",
		})
	})

	t.Run("ConfigMapComputedValue", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		stdout, stderr, err := helpers.RunCLI(dir, []string{"explain", "configMaps.values-common.LOADBALANCER_IP_RANGE"}, env)
		if err != nil {
			t.Fatalf("explain failed: %v\nstderr: %s", err, stderr)
		}
		assertExplainOutput(t, string(stdout), explainExpectation{
			header:         "configMaps.values-common.LOADBALANCER_IP_RANGE = 10.5.1.10-10.5.1.100",
			sourceContains: "composition (runtime config)",
		})
	})

	t.Run("TerraformInputLiteral", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		stdout, stderr, err := helpers.RunCLI(dir, []string{"explain", "terraform.networking.inputs.domain_name"}, env)
		if err != nil {
			t.Fatalf("explain failed: %v\nstderr: %s", err, stderr)
		}
		assertExplainOutput(t, string(stdout), explainExpectation{
			header:         "terraform.networking.inputs.domain_name = test.example.com",
			sourceContains: "option-explain-test.yaml",
		})
	})

	t.Run("TerraformInputExpression", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		stdout, stderr, err := helpers.RunCLI(dir, []string{"explain", "terraform.networking.inputs.cidr_block"}, env)
		if err != nil {
			t.Fatalf("explain failed: %v\nstderr: %s", err, stderr)
		}
		assertExplainOutput(t, string(stdout), explainExpectation{
			header:         "terraform.networking.inputs.cidr_block = 10.5.0.0/16",
			sourceContains: "option-explain-test.yaml",
		})
	})

	t.Run("KustomizeSubstitutionLiteral", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		stdout, stderr, err := helpers.RunCLI(dir, []string{"explain", "kustomize.monitoring.substitutions.cluster_domain"}, env)
		if err != nil {
			t.Fatalf("explain failed: %v\nstderr: %s", err, stderr)
		}
		assertExplainOutput(t, string(stdout), explainExpectation{
			header:         "kustomize.monitoring.substitutions.cluster_domain = test.example.com",
			sourceContains: "option-explain-test.yaml",
		})
	})

	t.Run("KustomizeMultipleContributors", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		stdout, stderr, err := helpers.RunCLI(dir, []string{"explain", "kustomize.monitoring.substitutions.log_level"}, env)
		if err != nil {
			t.Fatalf("explain failed: %v\nstderr: %s", err, stderr)
		}
		assertExplainOutput(t, string(stdout), explainExpectation{
			header:         "kustomize.monitoring.substitutions.log_level = debug",
			sourceContains: "option-ordinal-override.yaml",
		})
	})

	t.Run("KustomizeComponentsList", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		stdout, stderr, err := helpers.RunCLI(dir, []string{"explain", "kustomize.monitoring.components"}, env)
		if err != nil {
			t.Fatalf("explain failed: %v\nstderr: %s", err, stderr)
		}
		out := string(stdout)
		if !strings.Contains(out, "kustomize.monitoring.components") {
			t.Errorf("expected header containing 'kustomize.monitoring.components', got:\n%s", out)
		}
		if !strings.Contains(out, "prometheus") {
			t.Errorf("expected 'prometheus' in output, got:\n%s", out)
		}
		if !strings.Contains(out, "option-explain-test.yaml") {
			t.Errorf("expected source 'option-explain-test.yaml' in output, got:\n%s", out)
		}
	})

	t.Run("KustomizeComponentsMultiFacet", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		stdout, stderr, err := helpers.RunCLI(dir, []string{"explain", "kustomize.monitoring.components"}, env)
		if err != nil {
			t.Fatalf("explain failed: %v\nstderr: %s", err, stderr)
		}
		out := string(stdout)
		if !strings.Contains(out, "prometheus") {
			t.Errorf("expected 'prometheus' in output, got:\n%s", out)
		}
		if !strings.Contains(out, "grafana") {
			t.Errorf("expected 'grafana' in output, got:\n%s", out)
		}
		if !strings.Contains(out, "option-explain-test.yaml") {
			t.Errorf("expected source 'option-explain-test.yaml' in output, got:\n%s", out)
		}
		if !strings.Contains(out, "option-ordinal-override.yaml") {
			t.Errorf("expected source 'option-ordinal-override.yaml' in output, got:\n%s", out)
		}
	})

	t.Run("ScopeRefChainWithMapConfigBlock", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		stdout, stderr, err := helpers.RunCLI(dir, []string{"explain", "terraform.networking.inputs.net_config_ref"}, env)
		if err != nil {
			t.Fatalf("explain failed: %v\nstderr: %s", err, stderr)
		}
		out := string(stdout)
		if !strings.Contains(out, "terraform.networking.inputs.net_config_ref") {
			t.Errorf("expected header with path, got:\n%s", out)
		}
		if !strings.Contains(out, "net_config") {
			t.Errorf("expected scope ref 'net_config' in output, got:\n%s", out)
		}
		if !strings.Contains(out, "net_config.primary_cidr") {
			t.Errorf("expected nested ref 'net_config.primary_cidr' from map expansion, got:\n%s", out)
		}
		if !strings.Contains(out, "net_config.lb_start") {
			t.Errorf("expected nested ref 'net_config.lb_start' from map expansion, got:\n%s", out)
		}
		if !strings.Contains(out, "network.cidr_block") {
			t.Errorf("expected terminal ref 'network.cidr_block' from chain resolution, got:\n%s", out)
		}
	})

	t.Run("DeferredTerraformOutputResolvesViaFallback", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		stdout, stderr, err := helpers.RunCLI(dir, []string{"explain", "terraform.deferred-cluster.inputs.api_endpoint"}, env)
		if err != nil {
			t.Fatalf("explain failed: %v\nstderr: %s", err, stderr)
		}
		assertExplainOutput(t, string(stdout), explainExpectation{
			header:             "terraform.deferred-cluster.inputs.api_endpoint = https://localhost:6443",
			sourceContains:     "option-deferred-test.yaml",
			expressionContains: "deferred_endpoint.endpoint",
		})
	})

	t.Run("DeferredConfigBlockDoesNotAffectLiterals", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		stdout, stderr, err := helpers.RunCLI(dir, []string{"explain", "terraform.deferred-cluster.inputs.static_value"}, env)
		if err != nil {
			t.Fatalf("explain failed: %v\nstderr: %s", err, stderr)
		}
		assertExplainOutput(t, string(stdout), explainExpectation{
			header:         "terraform.deferred-cluster.inputs.static_value = hello",
			sourceContains: "option-deferred-test.yaml",
		})
	})

	t.Run("KustomizeSubstitutionMissingKey", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		stdout, stderr, err := helpers.RunCLI(dir, []string{"explain", "kustomize.monitoring.substitutions.nonexistent"}, env)
		if err != nil {
			t.Fatalf("explain failed: %v\nstderr: %s", err, stderr)
		}
		out := string(stdout)
		if !strings.Contains(out, "kustomize.monitoring.substitutions.nonexistent (empty)") {
			t.Errorf("expected empty value header, got:\n%s", out)
		}
	})

	t.Run("ErrorUnknownTerraformComponent", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		_, stderr, err := helpers.RunCLI(dir, []string{"explain", "terraform.nonexistent.inputs.foo"}, env)
		if err == nil {
			t.Fatal("expected non-zero exit for unknown terraform component")
		}
		stderrStr := string(stderr)
		if !strings.Contains(stderrStr, "not found") {
			t.Errorf("stderr should mention 'not found'; got:\n%s", stderrStr)
		}
		if !strings.Contains(stderrStr, "nonexistent") {
			t.Errorf("stderr should mention component name 'nonexistent'; got:\n%s", stderrStr)
		}
	})

	t.Run("ErrorUnknownKustomization", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		_, stderr, err := helpers.RunCLI(dir, []string{"explain", "kustomize.bogus.substitutions.foo"}, env)
		if err == nil {
			t.Fatal("expected non-zero exit for unknown kustomization")
		}
		stderrStr := string(stderr)
		if !strings.Contains(stderrStr, "not found") {
			t.Errorf("stderr should mention 'not found'; got:\n%s", stderrStr)
		}
		if !strings.Contains(stderrStr, "bogus") {
			t.Errorf("stderr should mention kustomization name 'bogus'; got:\n%s", stderrStr)
		}
	})

	t.Run("ErrorUnknownConfigMap", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "facet-composition")
		env = append(env, "WINDSOR_CONTEXT=default")
		_, stderr, err := helpers.RunCLI(dir, []string{"explain", "configMaps.nonexistent.KEY"}, env)
		if err == nil {
			t.Fatal("expected non-zero exit for unknown configMap")
		}
		stderrStr := string(stderr)
		if !strings.Contains(stderrStr, "not found") {
			t.Errorf("stderr should mention 'not found'; got:\n%s", stderrStr)
		}
		if !strings.Contains(stderrStr, "nonexistent") {
			t.Errorf("stderr should mention configMap name 'nonexistent'; got:\n%s", stderrStr)
		}
	})

	t.Run("ErrorMalformedPath", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "default")
		env = append(env, "WINDSOR_CONTEXT=default")
		_, stderr, err := helpers.RunCLI(dir, []string{"explain", "not-a-valid-path"}, env)
		if err == nil {
			t.Fatal("expected non-zero exit for malformed path")
		}
		stderrStr := string(stderr)
		if !strings.Contains(stderrStr, "invalid path") {
			t.Errorf("stderr should contain 'invalid path'; got:\n%s", stderrStr)
		}
	})

	t.Run("ErrorMissingPathArg", func(t *testing.T) {
		t.Parallel()
		dir, env := helpers.PrepareFixture(t, "default")
		env = append(env, "WINDSOR_CONTEXT=default")
		_, stderr, err := helpers.RunCLI(dir, []string{"explain"}, env)
		if err == nil {
			t.Fatal("expected non-zero exit when path argument missing")
		}
		stderrStr := string(stderr)
		if !strings.Contains(stderrStr, "accepts 1 arg") && !strings.Contains(stderrStr, "Usage") {
			t.Errorf("stderr should indicate missing argument or show usage; got:\n%s", stderrStr)
		}
	})
}

// explainExpectation defines expected content of windsor explain output.
type explainExpectation struct {
	header             string
	sourceContains     string
	expressionContains string
}

// assertExplainOutput validates the explain output: first line is the path = value header,
// followed by the effective contributor source and optional expression.
func assertExplainOutput(t *testing.T, out string, want explainExpectation) {
	t.Helper()
	out = strings.TrimRight(out, "\n")

	if !strings.HasPrefix(out, want.header) {
		lines := strings.SplitN(out, "\n", 2)
		t.Errorf("header: want %q, got %q", want.header, lines[0])
	}
	if want.sourceContains != "" && !strings.Contains(out, want.sourceContains) {
		t.Errorf("want source containing %q in output:\n%s", want.sourceContains, out)
	}
	if want.expressionContains != "" && !strings.Contains(out, want.expressionContains) {
		t.Errorf("want expression containing %q in output:\n%s", want.expressionContains, out)
	}
}
