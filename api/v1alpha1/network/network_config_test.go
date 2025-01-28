package network

import (
	"reflect"
	"testing"
)

func TestNetworkConfig_Merge(t *testing.T) {
	t.Run("MergeWithNonNilValues", func(t *testing.T) {
		base := &NetworkConfig{
			CIDRBlock: ptrString("10.0.0.0/24"),
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: ptrString("10.0.0.10"),
				End:   ptrString("10.0.0.20"),
			},
		}
		overlay := &NetworkConfig{
			CIDRBlock: ptrString("192.168.1.0/24"),
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: ptrString("192.168.1.10"),
				End:   ptrString("192.168.1.20"),
			},
		}
		base.Merge(overlay)

		if base.CIDRBlock == nil || *base.CIDRBlock != "192.168.1.0/24" {
			t.Errorf("CIDRBlock mismatch: expected %v, got %v", "192.168.1.0/24", base.CIDRBlock)
		}
		if base.LoadBalancerIPs == nil || *base.LoadBalancerIPs.Start != "192.168.1.10" {
			t.Errorf("LoadBalancerIPs.Start mismatch: expected %v, got %v", "192.168.1.10", base.LoadBalancerIPs.Start)
		}
		if base.LoadBalancerIPs == nil || *base.LoadBalancerIPs.End != "192.168.1.20" {
			t.Errorf("LoadBalancerIPs.End mismatch: expected %v, got %v", "192.168.1.20", base.LoadBalancerIPs.End)
		}
	})

	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &NetworkConfig{
			CIDRBlock: ptrString("10.0.0.0/24"),
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: ptrString("10.0.0.10"),
				End:   ptrString("10.0.0.20"),
			},
		}
		var overlay *NetworkConfig = nil
		base.Merge(overlay)

		if base.CIDRBlock == nil || *base.CIDRBlock != "10.0.0.0/24" {
			t.Errorf("CIDRBlock mismatch: expected %v, got %v", "10.0.0.0/24", base.CIDRBlock)
		}
		if base.LoadBalancerIPs == nil || *base.LoadBalancerIPs.Start != "10.0.0.10" {
			t.Errorf("LoadBalancerIPs.Start mismatch: expected %v, got %v", "10.0.0.10", base.LoadBalancerIPs.Start)
		}
		if base.LoadBalancerIPs == nil || *base.LoadBalancerIPs.End != "10.0.0.20" {
			t.Errorf("LoadBalancerIPs.End mismatch: expected %v, got %v", "10.0.0.20", base.LoadBalancerIPs.End)
		}
	})

	t.Run("MergeWithNilBaseLoadBalancerIPs", func(t *testing.T) {
		base := &NetworkConfig{
			CIDRBlock:       ptrString("10.0.0.0/24"),
			LoadBalancerIPs: nil,
		}
		overlay := &NetworkConfig{
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: ptrString("192.168.1.10"),
				End:   ptrString("192.168.1.20"),
			},
		}
		base.Merge(overlay)

		if base.LoadBalancerIPs == nil || *base.LoadBalancerIPs.Start != "192.168.1.10" {
			t.Errorf("LoadBalancerIPs.Start mismatch: expected %v, got %v", "192.168.1.10", base.LoadBalancerIPs.Start)
		}
		if base.LoadBalancerIPs == nil || *base.LoadBalancerIPs.End != "192.168.1.20" {
			t.Errorf("LoadBalancerIPs.End mismatch: expected %v, got %v", "192.168.1.20", base.LoadBalancerIPs.End)
		}
	})
}

func TestNetworkConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &NetworkConfig{
			CIDRBlock: ptrString("192.168.1.0/24"),
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: ptrString("192.168.1.10"),
				End:   ptrString("192.168.1.20"),
			},
		}

		copy := original.Copy()

		if !reflect.DeepEqual(original, copy) {
			t.Errorf("Copy mismatch: expected %v, got %v", original, copy)
		}

		// Modify the copy and ensure original is unchanged
		copy.CIDRBlock = ptrString("10.0.0.0/24")
		if original.CIDRBlock == nil || *original.CIDRBlock == *copy.CIDRBlock {
			t.Errorf("Original CIDRBlock was modified: expected %v, got %v", "192.168.1.0/24", *copy.CIDRBlock)
		}
		copy.LoadBalancerIPs.Start = ptrString("10.0.0.10")
		if original.LoadBalancerIPs.Start == nil || *original.LoadBalancerIPs.Start == *copy.LoadBalancerIPs.Start {
			t.Errorf("Original LoadBalancerIPs.Start was modified: expected %v, got %v", "192.168.1.10", *copy.LoadBalancerIPs.Start)
		}
	})

	t.Run("CopyWithNilValues", func(t *testing.T) {
		original := &NetworkConfig{
			CIDRBlock: nil,
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: nil,
				End:   nil,
			},
		}

		copy := original.Copy()

		if !reflect.DeepEqual(original, copy) {
			t.Errorf("Copy mismatch: expected %v, got %v", original, copy)
		}

		// Ensure the copy is not nil
		if copy == nil {
			t.Errorf("Copy is nil, expected a non-nil copy")
		}
	})

	t.Run("CopyWithNilNetworkConfig", func(t *testing.T) {
		var original *NetworkConfig = nil

		copy := original.Copy()

		// Ensure the copy is nil
		if copy != nil {
			t.Errorf("Copy is not nil, expected a nil copy")
		}
	})
}

// Helper function to create pointers for basic types
func ptrString(s string) *string {
	return &s
}
