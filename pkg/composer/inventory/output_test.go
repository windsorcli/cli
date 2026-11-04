package inventory_test

import (
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/composer/inventory"
)

// =============================================================================
// Test Setup
// =============================================================================

func testInventory() inventory.Inventory {
	return inventory.Inventory{Artifacts: []inventory.Entry{
		{Kind: inventory.KindBlueprint, Transport: inventory.TransportOCI, Reference: "ghcr.io/windsorcli/core", Version: "v0.9.0"},
		{Kind: inventory.KindChart, Transport: inventory.TransportHTTP, Reference: "https://kubernetes.github.io/ingress-nginx", Name: "ingress-nginx", Version: "4.15.1", Destination: "registry.example.com/windsor/ingress-nginx:4.15.1"},
		{Kind: inventory.KindChart, Transport: inventory.TransportOCI, Reference: "ghcr.io/openebs/charts/openebs", Name: "openebs", Version: "4.6.2", Destination: "registry.example.com/windsor/openebs:4.6.2"},
		{Kind: inventory.KindChart, Transport: inventory.TransportOCI, Reference: "registry.example.com/windsor/kyverno", Name: "kyverno", Version: "3.9.2", Destination: "registry.example.com/windsor/kyverno:3.9.2"},
		{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: "quay.io/coreos/etcd", Version: "v3.7.2", Digest: "sha256:abc", Destination: "mirror.test:5000/coreos/etcd:v3.7.2@sha256:abc"},
		{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: "docker.io/library/busybox", Version: "1.37"},
		{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: "mirror.test:5000/app", Digest: "sha256:def", Destination: "mirror.test:5000/app@sha256:def"},
	}}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestEntry_Upstream(t *testing.T) {
	t.Run("FormatsEachKind", func(t *testing.T) {
		// Given one entry of each transport
		inv := testInventory()

		// When Upstream is called on each
		got := []string{inv.Artifacts[0].Upstream(), inv.Artifacts[1].Upstream(), inv.Artifacts[2].Upstream(), inv.Artifacts[4].Upstream()}

		// Then each has its pull reference
		want := []string{
			"ghcr.io/windsorcli/core:v0.9.0",
			"https://kubernetes.github.io/ingress-nginx/ingress-nginx:4.15.1",
			"oci://ghcr.io/openebs/charts/openebs:4.6.2",
			"quay.io/coreos/etcd:v3.7.2@sha256:abc",
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("entry %d: expected %q, got %q", i, want[i], got[i])
			}
		}
	})
}

func TestFluxMirror(t *testing.T) {
	t.Run("WritesExactTagSelectors", func(t *testing.T) {
		// Given an inventory with charts, images and a blueprint
		inv := testInventory()

		// When FluxMirror is called
		cfg, notes, err := inventory.FluxMirror(inv, "oci://registry.example.com/windsor/")

		// Then HTTP charts are charts entries and OCI artifacts select their exact tag
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.APIVersion != inventory.FluxMirrorAPIVersion || cfg.Kind != "Config" {
			t.Errorf("unexpected header: %+v", cfg)
		}
		if len(cfg.Charts) != 1 || cfg.Charts[0] != (inventory.FluxMirrorChart{Name: "ingress-nginx", Source: "https://kubernetes.github.io/ingress-nginx", Destination: "oci://registry.example.com/windsor", Version: "4.15.1"}) {
			t.Errorf("unexpected charts: %+v", cfg.Charts)
		}
		if len(cfg.Artifacts) != 2 {
			t.Fatalf("expected 2 artifacts, got %+v", cfg.Artifacts)
		}
		openebs := cfg.Artifacts[0]
		if openebs.Source != "ghcr.io/openebs/charts/openebs" || openebs.Destination != "registry.example.com/windsor/openebs" || openebs.Selector.Regex.Pattern != `^4\.6\.2$` || !openebs.IncludeReferrers {
			t.Errorf("unexpected chart artifact: %+v", openebs)
		}
		if cfg.Artifacts[1].Destination != "mirror.test:5000/coreos/etcd" {
			t.Errorf("unexpected image artifact: %+v", cfg.Artifacts[1])
		}

		// And each left-out artifact gives a note
		joined := strings.Join(notes, "\n")
		for _, want := range []string{"kyverno:3.9.2 is already in registry.example.com/windsor", "busybox:1.37 has no destination", "app@sha256:def has no tag"} {
			if !strings.Contains(joined, want) {
				t.Errorf("expected note %q in:\n%s", want, joined)
			}
		}
	})

	t.Run("NeedsChartsToForCharts", func(t *testing.T) {
		// Given an inventory with charts
		inv := testInventory()

		// When FluxMirror is called without a chart registry
		_, _, err := inventory.FluxMirror(inv, "")

		// Then an error asks for --charts-to
		if err == nil || !strings.Contains(err.Error(), "--charts-to") {
			t.Fatalf("expected a --charts-to error, got %v", err)
		}
	})
}

func TestListLines(t *testing.T) {
	t.Run("WritesDestinationAndUpstream", func(t *testing.T) {
		// Given an inventory
		inv := testInventory()

		// When ListLines is called
		lines := strings.Split(strings.TrimSuffix(inventory.ListLines(inv), "\n"), "\n")

		// Then each artifact has one tab-separated line
		if len(lines) != len(inv.Artifacts) {
			t.Fatalf("expected %d lines, got %d", len(inv.Artifacts), len(lines))
		}
		if lines[2] != "registry.example.com/windsor/openebs:4.6.2\toci://ghcr.io/openebs/charts/openebs:4.6.2" {
			t.Errorf("unexpected line %q", lines[2])
		}
		if lines[0] != "\tghcr.io/windsorcli/core:v0.9.0" {
			t.Errorf("unexpected line %q", lines[0])
		}
	})
}
