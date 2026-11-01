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
