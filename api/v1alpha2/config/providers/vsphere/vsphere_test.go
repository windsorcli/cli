package vsphere

import (
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestVSphereConfig_Merge(t *testing.T) {
	t.Run("MergeWithAllFields", func(t *testing.T) {
		// Given base and overlay both fully populated
		base := &VSphereConfig{
			Server:       ptrString("vcenter-base.example.com"),
			User:         ptrString("base-user"),
			Datacenter:   ptrString("base-dc"),
			Cluster:      ptrString("base-cluster"),
			Datastore:    ptrString("base-ds"),
			Network:      ptrString("base-net"),
			ResourcePool: ptrString("base-pool"),
			Folder:       ptrString("base-folder"),
			Insecure:     ptrBool(false),
		}
		overlay := &VSphereConfig{
			Server:       ptrString("vcenter-overlay.example.com"),
			User:         ptrString("overlay-user"),
			Datacenter:   ptrString("overlay-dc"),
			Cluster:      ptrString("overlay-cluster"),
			Datastore:    ptrString("overlay-ds"),
			Network:      ptrString("overlay-net"),
			ResourcePool: ptrString("overlay-pool"),
			Folder:       ptrString("overlay-folder"),
			Insecure:     ptrBool(true),
		}

		// When Merge is called
		base.Merge(overlay)

		// Then all fields take overlay values
		if *base.Server != "vcenter-overlay.example.com" {
			t.Errorf("Server mismatch: got %s", *base.Server)
		}
		if *base.User != "overlay-user" {
			t.Errorf("User mismatch: got %s", *base.User)
		}
		if *base.Datacenter != "overlay-dc" {
			t.Errorf("Datacenter mismatch: got %s", *base.Datacenter)
		}
		if *base.Cluster != "overlay-cluster" {
			t.Errorf("Cluster mismatch: got %s", *base.Cluster)
		}
		if *base.Datastore != "overlay-ds" {
			t.Errorf("Datastore mismatch: got %s", *base.Datastore)
		}
		if *base.Network != "overlay-net" {
			t.Errorf("Network mismatch: got %s", *base.Network)
		}
		if *base.ResourcePool != "overlay-pool" {
			t.Errorf("ResourcePool mismatch: got %s", *base.ResourcePool)
		}
		if *base.Folder != "overlay-folder" {
			t.Errorf("Folder mismatch: got %s", *base.Folder)
		}
		if *base.Insecure != true {
			t.Errorf("Insecure mismatch: got %v", *base.Insecure)
		}
	})

	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		// Given a base config
		base := &VSphereConfig{
			Server: ptrString("vcenter.example.com"),
			User:   ptrString("admin"),
		}
		original := base.DeepCopy()

		// When Merge is called with nil
		base.Merge(nil)

		// Then base is unchanged
		if *base.Server != *original.Server {
			t.Error("Server should be unchanged after nil merge")
		}
		if *base.User != *original.User {
			t.Error("User should be unchanged after nil merge")
		}
	})

	t.Run("MergeWithPartialOverlay", func(t *testing.T) {
		// Given a fully populated base and a partial overlay
		base := &VSphereConfig{
			Server:     ptrString("vcenter-base.example.com"),
			User:       ptrString("base-user"),
			Datacenter: ptrString("base-dc"),
		}
		overlay := &VSphereConfig{
			Server: ptrString("vcenter-new.example.com"),
		}

		// When Merge is called
		base.Merge(overlay)

		// Then only Server is updated
		if *base.Server != "vcenter-new.example.com" {
			t.Errorf("Server mismatch: got %s", *base.Server)
		}
		if *base.User != "base-user" {
			t.Errorf("User should remain unchanged, got %s", *base.User)
		}
		if *base.Datacenter != "base-dc" {
			t.Errorf("Datacenter should remain unchanged, got %s", *base.Datacenter)
		}
	})

	t.Run("MergeWithAllNils", func(t *testing.T) {
		// Given two empty configs
		base := &VSphereConfig{}
		overlay := &VSphereConfig{}

		// When Merge is called
		base.Merge(overlay)

		// Then all fields remain nil
		if base.Server != nil || base.User != nil || base.Datacenter != nil ||
			base.Cluster != nil || base.Datastore != nil || base.Network != nil ||
			base.ResourcePool != nil || base.Folder != nil || base.Insecure != nil {
			t.Error("All fields should remain nil after merging two empty configs")
		}
	})
}

func TestVSphereConfig_DeepCopy(t *testing.T) {
	t.Run("DeepCopyWithAllFields", func(t *testing.T) {
		// Given a fully populated config
		original := &VSphereConfig{
			Server:       ptrString("vcenter.example.com"),
			User:         ptrString("admin"),
			Datacenter:   ptrString("DC0"),
			Cluster:      ptrString("cluster-01"),
			Datastore:    ptrString("datastore1"),
			Network:      ptrString("VM Network"),
			ResourcePool: ptrString("Resources"),
			Folder:       ptrString("/DC0/vm"),
			Insecure:     ptrBool(true),
		}

		// When DeepCopy is called
		cp := original.DeepCopy()

		// Then all fields match
		if *cp.Server != *original.Server {
			t.Errorf("Server mismatch: got %s", *cp.Server)
		}
		if *cp.User != *original.User {
			t.Errorf("User mismatch: got %s", *cp.User)
		}
		if *cp.Datacenter != *original.Datacenter {
			t.Errorf("Datacenter mismatch")
		}
		if *cp.Cluster != *original.Cluster {
			t.Errorf("Cluster mismatch")
		}
		if *cp.Datastore != *original.Datastore {
			t.Errorf("Datastore mismatch")
		}
		if *cp.Network != *original.Network {
			t.Errorf("Network mismatch")
		}
		if *cp.ResourcePool != *original.ResourcePool {
			t.Errorf("ResourcePool mismatch")
		}
		if *cp.Folder != *original.Folder {
			t.Errorf("Folder mismatch")
		}
		if *cp.Insecure != *original.Insecure {
			t.Errorf("Insecure mismatch")
		}

		// And the copy is independent
		*original.Server = "changed.example.com"
		if *cp.Server == "changed.example.com" {
			t.Error("DeepCopy should produce an independent copy")
		}
	})

	t.Run("DeepCopyNil", func(t *testing.T) {
		// Given a nil config pointer
		var original *VSphereConfig

		// When DeepCopy is called
		cp := original.DeepCopy()

		// Then the result is nil
		if cp != nil {
			t.Error("Expected nil DeepCopy for nil config")
		}
	})

	t.Run("DeepCopyWithPartialFields", func(t *testing.T) {
		// Given a config with only Server set
		original := &VSphereConfig{
			Server: ptrString("vcenter.example.com"),
		}

		// When DeepCopy is called
		cp := original.DeepCopy()

		// Then Server is copied and other fields are nil
		if cp.Server == nil || *cp.Server != *original.Server {
			t.Errorf("Server mismatch: expected %s", *original.Server)
		}
		if cp.User != nil {
			t.Error("Expected User to be nil in copy")
		}
		if cp.Insecure != nil {
			t.Error("Expected Insecure to be nil in copy")
		}
	})
}

func ptrString(s string) *string { return &s }
func ptrBool(b bool) *bool       { return &b }
