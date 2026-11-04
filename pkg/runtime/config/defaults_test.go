package config_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

func TestDefaultConfigurations(t *testing.T) {
	t.Run("DefaultConfig_HasPlatformNone", func(t *testing.T) {
		if config.DefaultConfig.Platform != "none" {
			t.Error("Expected DefaultConfig platform to be 'none'")
		}
	})

	t.Run("DefaultConfig_DevIsEmpty", func(t *testing.T) {
		if config.DefaultConfig_Dev.Network != nil {
			t.Error("network config should not be set (schema default)")
		}
		if config.DefaultConfig_Dev.Terraform != nil {
			t.Error("terraform config should not be set (schema default)")
		}
		if config.DefaultConfig_Dev.Cluster != nil {
			t.Error("cluster config should not be set (schema/facet default)")
		}
		if config.DefaultConfig_Dev.Docker != nil {
			t.Error("docker config should not be set (facet default)")
		}
		if config.DefaultConfig_Dev.DNS != nil {
			t.Error("dns config should not be set (schema/facet default)")
		}
	})
}

func TestDefaultTerraformBackendTypeForPlatform(t *testing.T) {
	testCases := []struct {
		platform string
		want     string
	}{
		{"aws", "s3"},
		{"azure", "azurerm"},
		{"gcp", "gcs"},
		{"metal", "kubernetes"},
		{"docker", "kubernetes"},
		{"incus", "kubernetes"},
		{"hetzner", "kubernetes"},
		{"hyperv", "kubernetes"},
		{"vsphere", "kubernetes"},
		{"proxmox", "kubernetes"},
		{"none", ""},
		{"", ""},
		{"unrecognized", ""},
	}
	for _, tc := range testCases {
		t.Run(tc.platform, func(t *testing.T) {
			if got := config.DefaultTerraformBackendTypeForPlatform(tc.platform); got != tc.want {
				t.Errorf("Expected platform %q to map to %q, got %q", tc.platform, tc.want, got)
			}
		})
	}
}

func TestInitNodeDefaults(t *testing.T) {
	nodePort := map[string]any{"service_type": "NodePort"}
	node := func(address string) map[string]any {
		return map[string]any{"node": address, "endpoint": address + ":50000"}
	}

	testCases := []struct {
		name   string
		values map[string]any
		want   map[string]any
	}{
		{
			name:   "HypervNodePortGetsOneControlPlaneAndOneWorker",
			values: map[string]any{"platform": "hyperv", "gateway": nodePort},
			want: map[string]any{
				"cluster.controlplanes.nodes.controlplane-1": node("10.5.0.10"),
				"cluster.workers.nodes.worker-1":             node("10.5.0.20"),
			},
		},
		{
			name: "HypervPerGatewayNodePortUsesCIDRBlockAndCounts",
			values: map[string]any{
				"platform": "hyperv",
				"gateway":  map[string]any{"external": nodePort, "internal": nodePort},
				"network":  map[string]any{"cidr_block": "192.168.50.0/24"},
				"cluster": map[string]any{
					"controlplanes": map[string]any{"count": 2},
					"workers":       map[string]any{"count": "0"},
				},
			},
			want: map[string]any{
				"cluster.controlplanes.nodes.controlplane-1": node("192.168.50.10"),
				"cluster.controlplanes.nodes.controlplane-2": node("192.168.50.11"),
			},
		},
		{
			name: "WorkersStartAfterElevenControlPlanes",
			values: map[string]any{
				"platform": "hyperv",
				"gateway":  nodePort,
				"cluster":  map[string]any{"controlplanes": map[string]any{"count": 11}, "workers": map[string]any{"count": 0}},
			},
			want: func() map[string]any {
				want := map[string]any{}
				for n := 1; n <= 11; n++ {
					want[fmt.Sprintf("cluster.controlplanes.nodes.controlplane-%d", n)] = node(fmt.Sprintf("10.5.0.%d", 9+n))
				}
				return want
			}(),
		},
		{
			name: "WorkerFollowsLastOfElevenControlPlanes",
			values: map[string]any{
				"platform": "hyperv",
				"gateway":  nodePort,
				"cluster": map[string]any{
					"controlplanes": map[string]any{"count": 11, "nodes": map[string]any{"cp": node("10.5.0.99")}},
				},
			},
			want: map[string]any{"cluster.workers.nodes.worker-1": node("10.5.0.21")},
		},
		{
			name: "PoolWithNodesKeepsItsNodes",
			values: map[string]any{
				"platform": "hyperv",
				"gateway":  nodePort,
				"cluster":  map[string]any{"controlplanes": map[string]any{"nodes": map[string]any{"cp": node("10.5.0.99")}}},
			},
			want: map[string]any{"cluster.workers.nodes.worker-1": node("10.5.0.20")},
		},
		{
			name:   "HostOutsideCIDRBlockGetsNoEntry",
			values: map[string]any{"platform": "hyperv", "gateway": nodePort, "network": map[string]any{"cidr_block": "10.5.0.0/28"}},
			want:   map[string]any{"cluster.controlplanes.nodes.controlplane-1": node("10.5.0.10")},
		},
		{
			name: "OneGatewayOnLoadBalancerGetsNothing",
			values: map[string]any{
				"platform": "hyperv",
				"gateway":  map[string]any{"service_type": "NodePort", "internal": map[string]any{"service_type": "LoadBalancer"}},
			},
			want: map[string]any{},
		},
		{
			name:   "HypervLoadBalancerGetsNothing",
			values: map[string]any{"platform": "hyperv"},
			want:   map[string]any{},
		},
		{
			name:   "OtherPlatformGetsNothing",
			values: map[string]any{"platform": "vsphere", "gateway": nodePort},
			want:   map[string]any{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := config.InitNodeDefaults(tc.values); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Expected %v, got %v", tc.want, got)
			}
		})
	}
}
