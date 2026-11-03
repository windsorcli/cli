package cmd

import (
	"fmt"
	"io"
	"strings"
	"testing"

	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
)

// =============================================================================
// Test Setup
// =============================================================================

// =============================================================================
// confirmDestroy Tests
// =============================================================================

func TestConfirmDestroy(t *testing.T) {
	t.Run("SuccessMatchingInput", func(t *testing.T) {
		r := strings.NewReader("production\n")
		w := io.Discard
		err := confirmDestroy(r, w, "This will destroy everything.", "production")
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithWhitespace", func(t *testing.T) {
		r := strings.NewReader("  production  \n")
		w := io.Discard
		err := confirmDestroy(r, w, "This will destroy everything.", "production")
		if err != nil {
			t.Errorf("Expected no error for trimmed whitespace, got: %v", err)
		}
	})

	t.Run("ErrorWrongInput", func(t *testing.T) {
		r := strings.NewReader("wrong\n")
		w := io.Discard
		err := confirmDestroy(r, w, "This will destroy everything.", "production")
		if err == nil {
			t.Error("Expected error for wrong input, got nil")
		}
		if !strings.Contains(err.Error(), "confirmation failed") {
			t.Errorf("Expected confirmation failed error, got: %v", err)
		}
	})

	t.Run("ErrorEmptyInput", func(t *testing.T) {
		r := strings.NewReader("\n")
		w := io.Discard
		err := confirmDestroy(r, w, "This will destroy everything.", "production")
		if err == nil {
			t.Error("Expected error for empty input, got nil")
		}
	})

	t.Run("ErrorNoInput", func(t *testing.T) {
		r := strings.NewReader("")
		w := io.Discard
		err := confirmDestroy(r, w, "This will destroy everything.", "production")
		if err == nil {
			t.Error("Expected error when no input provided, got nil")
		}
		if !strings.Contains(err.Error(), "confirmation aborted") {
			t.Errorf("Expected confirmation aborted error, got: %v", err)
		}
	})
}

func TestWarnPreventDestroy(t *testing.T) {
	t.Run("EmitsNothingWhenNoProtectedResources", func(t *testing.T) {
		// Given plans with no protected addresses
		plans := []terraforminfra.TerraformComponentPlan{
			{ComponentID: "vpc", Destroy: 3},
			{ComponentID: "cluster", Destroy: 5},
		}
		var buf strings.Builder

		// When the warning runs
		warnPreventDestroy(&buf, plans)

		// Then stderr is silent — common path, no noise
		if buf.Len() != 0 {
			t.Errorf("expected no output, got %q", buf.String())
		}
	})

	t.Run("ListsEveryProtectedAddressAcrossComponentsWithRemediation", func(t *testing.T) {
		// Given multiple components, two of which report protected resources
		plans := []terraforminfra.TerraformComponentPlan{
			{ComponentID: "vpc", Destroy: 3},
			{ComponentID: "data", Protected: []string{"aws_db_instance.production"}},
			{ComponentID: "audit", Protected: []string{"aws_s3_bucket.logs", "aws_kms_key.audit"}},
		}
		var buf strings.Builder

		// When the warning runs
		warnPreventDestroy(&buf, plans)

		// Then it names every address, the total count, and the remediation
		out := buf.String()
		if !strings.Contains(out, "3 resource(s)") {
			t.Errorf("expected count '3 resource(s)' in output, got %q", out)
		}
		for _, addr := range []string{"aws_db_instance.production", "aws_s3_bucket.logs", "aws_kms_key.audit"} {
			if !strings.Contains(out, addr) {
				t.Errorf("expected address %q in output, got %q", addr, out)
			}
		}
		if !strings.Contains(out, "remove the lifecycle block") {
			t.Errorf("expected remediation in output, got %q", out)
		}
	})
}

func TestWarnDestroyAttributeDrift(t *testing.T) {
	t.Run("EmitsNothingWhenNoDrift", func(t *testing.T) {
		// Given plans with no drifted attributes
		plans := []terraforminfra.TerraformComponentPlan{
			{ComponentID: "vpc", Destroy: 3},
			{ComponentID: "cluster", Destroy: 5},
		}
		var buf strings.Builder

		// When the warning runs
		warnDestroyAttributeDrift(&buf, plans)

		// Then stderr is silent — common path, no noise
		if buf.Len() != 0 {
			t.Errorf("expected no output, got %q", buf.String())
		}
	})

	t.Run("NamesEveryDriftedAttributeAcrossComponentsWithRemediation", func(t *testing.T) {
		// Given multiple components, one of which reports drift on two resources
		plans := []terraforminfra.TerraformComponentPlan{
			{ComponentID: "vpc", Destroy: 3},
			{ComponentID: "database", DriftedAttributes: []terraforminfra.DestroyAttributeDrift{
				{Address: "google_service_networking_connection.cloudsql", Attribute: "deletion_policy", State: `"DELETE"`, Config: `"REMOVE_PEERING"`},
				{Address: "aws_s3_bucket.logs", Attribute: "force_destroy", State: "false", Config: "true"},
			}},
		}
		var buf strings.Builder

		// When the warning runs
		warnDestroyAttributeDrift(&buf, plans)

		// Then it names every address, its attribute and both values, and the count
		out := buf.String()
		if !strings.Contains(out, "2 resource(s)") {
			t.Errorf("expected count '2 resource(s)' in output, got %q", out)
		}
		if !strings.Contains(out, "google_service_networking_connection.cloudsql: deletion_policy state=\"DELETE\" config=\"REMOVE_PEERING\"") {
			t.Errorf("expected the cloudsql drift line in output, got %q", out)
		}
		if !strings.Contains(out, "aws_s3_bucket.logs: force_destroy state=false config=true") {
			t.Errorf("expected the s3 drift line in output, got %q", out)
		}
		if !strings.Contains(out, "terraform apply") {
			t.Errorf("expected the remediation hint in output, got %q", out)
		}
	})
}

func TestFailOnDestroyPlanErrors(t *testing.T) {
	t.Run("NilWhenEveryComponentPlannedCleanly", func(t *testing.T) {
		// Given plans that all generated successfully
		plans := []terraforminfra.TerraformComponentPlan{
			{ComponentID: "compute/hcloud", Destroy: 10},
			{ComponentID: "cluster/talos", Destroy: 5},
		}

		// When the gate runs
		err := failOnDestroyPlanErrors(plans)

		// Then it does not halt the destroy
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("HaltsAndNamesEveryComponentThatFailedToPlan", func(t *testing.T) {
		// Given a mix of cleanly-planned and plan-failed components
		plans := []terraforminfra.TerraformComponentPlan{
			{ComponentID: "compute/hcloud", Err: fmt.Errorf("hcloud provider auth failed")},
			{ComponentID: "cluster/talos", Destroy: 10},
			{ComponentID: "dns/zone/hetzner", Err: fmt.Errorf("zone lookup failed")},
		}

		// When the gate runs
		err := failOnDestroyPlanErrors(plans)

		// Then it halts, reports the count, and names each failed component with its error
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		out := err.Error()
		if !strings.Contains(out, "2 component(s)") {
			t.Errorf("expected count '2 component(s)' in error, got %q", out)
		}
		for _, want := range []string{"compute/hcloud", "hcloud provider auth failed", "dns/zone/hetzner", "zone lookup failed"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected %q in error, got %q", want, out)
			}
		}
		if strings.Contains(out, "cluster/talos") {
			t.Errorf("did not expect cleanly-planned component named, got %q", out)
		}
	})
}

func TestFailOnDestroyKustomizePlanErrors(t *testing.T) {
	t.Run("NilWhenEveryKustomizationPlannedCleanly", func(t *testing.T) {
		// Given kustomizations that all generated successfully
		plans := []fluxinfra.KustomizePlan{
			{Name: "my-app", Removed: 4},
			{Name: "ingress", Removed: 2},
		}

		// When the gate runs
		err := failOnDestroyKustomizePlanErrors(plans)

		// Then it does not halt the destroy
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("HaltsAndNamesEveryKustomizationThatFailedToPlan", func(t *testing.T) {
		// Given a mix of cleanly-planned and plan-failed kustomizations
		plans := []fluxinfra.KustomizePlan{
			{Name: "my-app", Err: fmt.Errorf("connection refused")},
			{Name: "ingress", Removed: 2},
		}

		// When the gate runs
		err := failOnDestroyKustomizePlanErrors(plans)

		// Then it halts, reports the count, and names the failed kustomization with its error
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		out := err.Error()
		if !strings.Contains(out, "1 kustomization(s)") {
			t.Errorf("expected count '1 kustomization(s)' in error, got %q", out)
		}
		if !strings.Contains(out, "my-app") || !strings.Contains(out, "connection refused") {
			t.Errorf("expected failed kustomization and its error in output, got %q", out)
		}
		if strings.Contains(out, "ingress") {
			t.Errorf("did not expect cleanly-planned kustomization named, got %q", out)
		}
	})
}

func TestReportSkippedClusterGoneKustomizations(t *testing.T) {
	t.Run("EmitsNothingWhenNoneSkipped", func(t *testing.T) {
		var buf strings.Builder

		reportSkippedClusterGoneKustomizations(&buf, nil)

		if buf.Len() != 0 {
			t.Errorf("expected no output, got %q", buf.String())
		}
	})

	t.Run("NamesTheSingleSkippedKustomizationWithoutOrphanWarning", func(t *testing.T) {
		var buf strings.Builder

		reportSkippedClusterGoneKustomizations(&buf, []string{"my-app"})

		out := buf.String()
		if !strings.Contains(out, `"my-app"`) {
			t.Errorf("expected kustomization name in output, got %q", out)
		}
		if strings.Contains(out, "orphaned") || strings.Contains(out, "cloud console") {
			t.Errorf("expected no orphan-hazard wording for a cluster-gone skip, got %q", out)
		}
	})

	t.Run("NamesEveryKustomizationWithCount", func(t *testing.T) {
		var buf strings.Builder

		reportSkippedClusterGoneKustomizations(&buf, []string{"my-app", "ingress"})

		out := buf.String()
		if !strings.Contains(out, "2 kustomizations") {
			t.Errorf("expected count '2 kustomizations' in output, got %q", out)
		}
		for _, name := range []string{"my-app", "ingress"} {
			if !strings.Contains(out, name) {
				t.Errorf("expected kustomization %q in output, got %q", name, out)
			}
		}
	})
}

// =============================================================================
// Test Cases
// =============================================================================
