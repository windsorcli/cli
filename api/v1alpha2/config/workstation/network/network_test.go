package workstation

import (
	"testing"
)

// TestNetworkConfig_Merge tests the Merge method of NetworkConfig
func TestNetworkConfig_Merge(t *testing.T) {
	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &NetworkConfig{
			CIDRBlock: stringPtr("10.0.0.0/24"),
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("10.0.0.10"),
				End:   stringPtr("10.0.0.20"),
			},
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if base.CIDRBlock == nil || *base.CIDRBlock != *original.CIDRBlock {
			t.Errorf("Expected CIDRBlock to remain unchanged")
		}
		if base.LoadBalancerIPs == nil {
			t.Error("Expected LoadBalancerIPs to remain initialized")
		}
		if base.LoadBalancerIPs.Start == nil || *base.LoadBalancerIPs.Start != *original.LoadBalancerIPs.Start {
			t.Errorf("Expected LoadBalancerIPs.Start to remain unchanged")
		}
		if base.LoadBalancerIPs.End == nil || *base.LoadBalancerIPs.End != *original.LoadBalancerIPs.End {
			t.Errorf("Expected LoadBalancerIPs.End to remain unchanged")
		}
	})

	t.Run("MergeWithEmptyOverlay", func(t *testing.T) {
		base := &NetworkConfig{
			CIDRBlock: stringPtr("10.0.0.0/24"),
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("10.0.0.10"),
				End:   stringPtr("10.0.0.20"),
			},
		}
		overlay := &NetworkConfig{}

		base.Merge(overlay)

		if base.CIDRBlock == nil || *base.CIDRBlock != "10.0.0.0/24" {
			t.Errorf("Expected CIDRBlock to remain '10.0.0.0/24'")
		}
		if base.LoadBalancerIPs == nil {
			t.Error("Expected LoadBalancerIPs to remain initialized")
		}
		if base.LoadBalancerIPs.Start == nil || *base.LoadBalancerIPs.Start != "10.0.0.10" {
			t.Errorf("Expected LoadBalancerIPs.Start to remain '10.0.0.10'")
		}
		if base.LoadBalancerIPs.End == nil || *base.LoadBalancerIPs.End != "10.0.0.20" {
			t.Errorf("Expected LoadBalancerIPs.End to remain '10.0.0.20'")
		}
	})

	t.Run("MergeWithPartialOverlay", func(t *testing.T) {
		base := &NetworkConfig{
			CIDRBlock: stringPtr("10.0.0.0/24"),
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("10.0.0.10"),
				End:   stringPtr("10.0.0.20"),
			},
		}
		overlay := &NetworkConfig{
			CIDRBlock: stringPtr("192.168.1.0/24"),
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("192.168.1.10"),
				End:   stringPtr("192.168.1.20"),
			},
		}

		base.Merge(overlay)

		if base.CIDRBlock == nil || *base.CIDRBlock != "192.168.1.0/24" {
			t.Errorf("Expected CIDRBlock to be '192.168.1.0/24', got %s", *base.CIDRBlock)
		}
		if base.LoadBalancerIPs == nil {
			t.Error("Expected LoadBalancerIPs to be initialized")
		}
		if base.LoadBalancerIPs.Start == nil || *base.LoadBalancerIPs.Start != "192.168.1.10" {
			t.Errorf("Expected LoadBalancerIPs.Start to be '192.168.1.10', got %s", *base.LoadBalancerIPs.Start)
		}
		if base.LoadBalancerIPs.End == nil || *base.LoadBalancerIPs.End != "192.168.1.20" {
			t.Errorf("Expected LoadBalancerIPs.End to be '192.168.1.20', got %s", *base.LoadBalancerIPs.End)
		}
	})

	t.Run("MergeWithOnlyCIDRBlock", func(t *testing.T) {
		base := &NetworkConfig{
			CIDRBlock: stringPtr("10.0.0.0/24"),
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("10.0.0.10"),
				End:   stringPtr("10.0.0.20"),
			},
		}
		overlay := &NetworkConfig{
			CIDRBlock: stringPtr("192.168.1.0/24"),
		}

		base.Merge(overlay)

		if base.CIDRBlock == nil || *base.CIDRBlock != "192.168.1.0/24" {
			t.Errorf("Expected CIDRBlock to be '192.168.1.0/24', got %s", *base.CIDRBlock)
		}
		if base.LoadBalancerIPs == nil {
			t.Error("Expected LoadBalancerIPs to remain initialized")
		}
		if base.LoadBalancerIPs.Start == nil || *base.LoadBalancerIPs.Start != "10.0.0.10" {
			t.Errorf("Expected LoadBalancerIPs.Start to remain '10.0.0.10'")
		}
		if base.LoadBalancerIPs.End == nil || *base.LoadBalancerIPs.End != "10.0.0.20" {
			t.Errorf("Expected LoadBalancerIPs.End to remain '10.0.0.20'")
		}
	})

	t.Run("MergeWithOnlyLoadBalancerIPs", func(t *testing.T) {
		base := &NetworkConfig{
			CIDRBlock: stringPtr("10.0.0.0/24"),
		}
		overlay := &NetworkConfig{
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("10.0.0.10"),
				End:   stringPtr("10.0.0.20"),
			},
		}

		base.Merge(overlay)

		if base.CIDRBlock == nil || *base.CIDRBlock != "10.0.0.0/24" {
			t.Errorf("Expected CIDRBlock to remain '10.0.0.0/24'")
		}
		if base.LoadBalancerIPs == nil {
			t.Error("Expected LoadBalancerIPs to be initialized")
		}
		if base.LoadBalancerIPs.Start == nil || *base.LoadBalancerIPs.Start != "10.0.0.10" {
			t.Errorf("Expected LoadBalancerIPs.Start to be '10.0.0.10'")
		}
		if base.LoadBalancerIPs.End == nil || *base.LoadBalancerIPs.End != "10.0.0.20" {
			t.Errorf("Expected LoadBalancerIPs.End to be '10.0.0.20'")
		}
	})

	t.Run("MergeWithPartialLoadBalancerIPs", func(t *testing.T) {
		base := &NetworkConfig{
			CIDRBlock: stringPtr("10.0.0.0/24"),
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("10.0.0.10"),
				End:   stringPtr("10.0.0.20"),
			},
		}
		overlay := &NetworkConfig{
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("10.0.0.15"),
			},
		}

		base.Merge(overlay)

		if base.CIDRBlock == nil || *base.CIDRBlock != "10.0.0.0/24" {
			t.Errorf("Expected CIDRBlock to remain '10.0.0.0/24'")
		}
		if base.LoadBalancerIPs == nil {
			t.Error("Expected LoadBalancerIPs to remain initialized")
		}
		if base.LoadBalancerIPs.Start == nil || *base.LoadBalancerIPs.Start != "10.0.0.15" {
			t.Errorf("Expected LoadBalancerIPs.Start to be '10.0.0.15'")
		}
		if base.LoadBalancerIPs.End == nil || *base.LoadBalancerIPs.End != "10.0.0.20" {
			t.Errorf("Expected LoadBalancerIPs.End to remain '10.0.0.20'")
		}
	})

	t.Run("MergeWithNilBaseLoadBalancerIPs", func(t *testing.T) {
		base := &NetworkConfig{
			CIDRBlock: stringPtr("10.0.0.0/24"),
		}
		overlay := &NetworkConfig{
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("10.0.0.10"),
				End:   stringPtr("10.0.0.20"),
			},
		}

		base.Merge(overlay)

		if base.CIDRBlock == nil || *base.CIDRBlock != "10.0.0.0/24" {
			t.Errorf("Expected CIDRBlock to remain '10.0.0.0/24'")
		}
		if base.LoadBalancerIPs == nil {
			t.Error("Expected LoadBalancerIPs to be initialized")
		}
		if base.LoadBalancerIPs.Start == nil || *base.LoadBalancerIPs.Start != "10.0.0.10" {
			t.Errorf("Expected LoadBalancerIPs.Start to be '10.0.0.10'")
		}
		if base.LoadBalancerIPs.End == nil || *base.LoadBalancerIPs.End != "10.0.0.20" {
			t.Errorf("Expected LoadBalancerIPs.End to be '10.0.0.20'")
		}
	})
}

// TestNetworkConfig_Copy tests the Copy method of NetworkConfig
func TestNetworkConfig_Copy(t *testing.T) {
	t.Run("CopyNilConfig", func(t *testing.T) {
		var config *NetworkConfig
		copied := config.DeepCopy()

		if copied != nil {
			t.Error("Expected nil copy for nil config")
		}
	})

	t.Run("CopyEmptyConfig", func(t *testing.T) {
		config := &NetworkConfig{}
		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy of empty config")
		}
		if copied.CIDRBlock != nil {
			t.Error("Expected CIDRBlock to be nil in copy")
		}
		if copied.LoadBalancerIPs != nil {
			t.Error("Expected LoadBalancerIPs to be nil in copy")
		}
	})

	t.Run("CopyPopulatedConfig", func(t *testing.T) {
		config := &NetworkConfig{
			CIDRBlock: stringPtr("10.0.0.0/24"),
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("10.0.0.10"),
				End:   stringPtr("10.0.0.20"),
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied == config {
			t.Error("Expected copy to be a new instance")
		}
		if copied.CIDRBlock == nil || *copied.CIDRBlock != *config.CIDRBlock {
			t.Errorf("Expected CIDRBlock to be copied correctly")
		}
		if copied.LoadBalancerIPs == nil {
			t.Error("Expected LoadBalancerIPs to be copied")
		}
		if copied.LoadBalancerIPs == config.LoadBalancerIPs {
			t.Error("Expected LoadBalancerIPs to be a new instance")
		}
		if copied.LoadBalancerIPs.Start == nil || *copied.LoadBalancerIPs.Start != *config.LoadBalancerIPs.Start {
			t.Errorf("Expected LoadBalancerIPs.Start to be copied correctly")
		}
		if copied.LoadBalancerIPs.End == nil || *copied.LoadBalancerIPs.End != *config.LoadBalancerIPs.End {
			t.Errorf("Expected LoadBalancerIPs.End to be copied correctly")
		}
	})

	t.Run("CopyWithPartialFields", func(t *testing.T) {
		config := &NetworkConfig{
			CIDRBlock: stringPtr("10.0.0.0/24"),
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.CIDRBlock == nil || *copied.CIDRBlock != *config.CIDRBlock {
			t.Errorf("Expected CIDRBlock to be copied correctly")
		}
		if copied.LoadBalancerIPs != nil {
			t.Error("Expected LoadBalancerIPs to be nil in copy")
		}
	})

	t.Run("CopyWithOnlyLoadBalancerIPs", func(t *testing.T) {
		config := &NetworkConfig{
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("10.0.0.10"),
				End:   stringPtr("10.0.0.20"),
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.CIDRBlock != nil {
			t.Error("Expected CIDRBlock to be nil in copy")
		}
		if copied.LoadBalancerIPs == nil {
			t.Error("Expected LoadBalancerIPs to be copied")
		}
		if copied.LoadBalancerIPs.Start == nil || *copied.LoadBalancerIPs.Start != *config.LoadBalancerIPs.Start {
			t.Errorf("Expected LoadBalancerIPs.Start to be copied correctly")
		}
		if copied.LoadBalancerIPs.End == nil || *copied.LoadBalancerIPs.End != *config.LoadBalancerIPs.End {
			t.Errorf("Expected LoadBalancerIPs.End to be copied correctly")
		}
	})

	t.Run("CopyWithPartialLoadBalancerIPs", func(t *testing.T) {
		config := &NetworkConfig{
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("10.0.0.10"),
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.LoadBalancerIPs == nil {
			t.Error("Expected LoadBalancerIPs to be copied")
		}
		if copied.LoadBalancerIPs.Start == nil || *copied.LoadBalancerIPs.Start != *config.LoadBalancerIPs.Start {
			t.Errorf("Expected LoadBalancerIPs.Start to be copied correctly")
		}
		if copied.LoadBalancerIPs.End != nil {
			t.Error("Expected LoadBalancerIPs.End to be nil in copy")
		}
	})

	t.Run("CopyWithIndependentValues", func(t *testing.T) {
		config := &NetworkConfig{
			CIDRBlock: stringPtr("10.0.0.0/24"),
			LoadBalancerIPs: &struct {
				Start *string `yaml:"start,omitempty"`
				End   *string `yaml:"end,omitempty"`
			}{
				Start: stringPtr("10.0.0.10"),
				End:   stringPtr("10.0.0.20"),
			},
		}

		copied := config.DeepCopy()

		// Modify original to verify independence
		*config.CIDRBlock = "192.168.1.0/24"
		*config.LoadBalancerIPs.Start = "192.168.1.10"
		*config.LoadBalancerIPs.End = "192.168.1.20"

		if *copied.CIDRBlock != "10.0.0.0/24" {
			t.Error("Expected copied CIDRBlock to remain independent")
		}
		if *copied.LoadBalancerIPs.Start != "10.0.0.10" {
			t.Error("Expected copied LoadBalancerIPs.Start to remain independent")
		}
		if *copied.LoadBalancerIPs.End != "10.0.0.20" {
			t.Error("Expected copied LoadBalancerIPs.End to remain independent")
		}
	})
}

func stringPtr(s string) *string {
	return &s
}
