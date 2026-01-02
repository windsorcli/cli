package vm

import (
	"reflect"
	"testing"
)

func TestVMConfig_Merge(t *testing.T) {
	t.Run("MergeWithNonNilValues", func(t *testing.T) {
		base := &VMConfig{
			Address: ptrString("10.0.0.1"),
			Arch:    ptrString("arm64"),
			CPU:     ptrInt(2),
			Disk:    ptrInt(50),
			Driver:  ptrString("virtualbox"),
			Memory:  ptrInt(4096),
			Runtime: ptrString("docker"),
		}
		overlay := &VMConfig{
			Address: ptrString("192.168.1.1"),
			Arch:    ptrString("x86_64"),
			CPU:     ptrInt(4),
			Disk:    ptrInt(100),
			Driver:  ptrString("kvm"),
			Memory:  ptrInt(8192),
			Runtime: ptrString("incus"),
		}
		base.Merge(overlay)

		if base.Address == nil || *base.Address != "192.168.1.1" {
			t.Errorf("Address mismatch: expected %v, got %v", "192.168.1.1", base.Address)
		}
		if base.Arch == nil || *base.Arch != "x86_64" {
			t.Errorf("Arch mismatch: expected %v, got %v", "x86_64", base.Arch)
		}
		if base.CPU == nil || *base.CPU != 4 {
			t.Errorf("CPU mismatch: expected %v, got %v", 4, base.CPU)
		}
		if base.Disk == nil || *base.Disk != 100 {
			t.Errorf("Disk mismatch: expected %v, got %v", 100, base.Disk)
		}
		if base.Driver == nil || *base.Driver != "kvm" {
			t.Errorf("Driver mismatch: expected %v, got %v", "kvm", base.Driver)
		}
		if base.Memory == nil || *base.Memory != 8192 {
			t.Errorf("Memory mismatch: expected %v, got %v", 8192, base.Memory)
		}
		if base.Runtime == nil || *base.Runtime != "incus" {
			t.Errorf("Runtime mismatch: expected %v, got %v", "incus", base.Runtime)
		}
	})

	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &VMConfig{
			Address: ptrString("10.0.0.1"),
			Arch:    ptrString("arm64"),
			CPU:     ptrInt(2),
			Disk:    ptrInt(50),
			Driver:  ptrString("virtualbox"),
			Memory:  ptrInt(4096),
			Runtime: ptrString("docker"),
		}
		var overlay *VMConfig = nil
		base.Merge(overlay)

		if base.Address == nil || *base.Address != "10.0.0.1" {
			t.Errorf("Address mismatch: expected %v, got %v", "10.0.0.1", base.Address)
		}
		if base.Arch == nil || *base.Arch != "arm64" {
			t.Errorf("Arch mismatch: expected %v, got %v", "arm64", base.Arch)
		}
		if base.CPU == nil || *base.CPU != 2 {
			t.Errorf("CPU mismatch: expected %v, got %v", 2, base.CPU)
		}
		if base.Disk == nil || *base.Disk != 50 {
			t.Errorf("Disk mismatch: expected %v, got %v", 50, base.Disk)
		}
		if base.Driver == nil || *base.Driver != "virtualbox" {
			t.Errorf("Driver mismatch: expected %v, got %v", "virtualbox", base.Driver)
		}
		if base.Memory == nil || *base.Memory != 4096 {
			t.Errorf("Memory mismatch: expected %v, got %v", 4096, base.Memory)
		}
		if base.Runtime == nil || *base.Runtime != "docker" {
			t.Errorf("Runtime mismatch: expected %v, got %v", "docker", base.Runtime)
		}
	})
}

func TestVMConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &VMConfig{
			Address: ptrString("192.168.1.1"),
			Arch:    ptrString("x86_64"),
			CPU:     ptrInt(4),
			Disk:    ptrInt(100),
			Driver:  ptrString("kvm"),
			Memory:  ptrInt(8192),
			Runtime: ptrString("incus"),
		}

		copy := original.Copy()

		if !reflect.DeepEqual(original, copy) {
			t.Errorf("Copy mismatch: expected %v, got %v", original, copy)
		}

		// Modify the copy and ensure original is unchanged
		copy.CPU = ptrInt(8)
		if original.CPU == nil || *original.CPU == *copy.CPU {
			t.Errorf("Original CPU was modified: expected %v, got %v", 4, *copy.CPU)
		}
		copy.Memory = ptrInt(16384)
		if original.Memory == nil || *original.Memory == *copy.Memory {
			t.Errorf("Original Memory was modified: expected %v, got %v", 8192, *copy.Memory)
		}
		copy.Runtime = ptrString("docker")
		if original.Runtime == nil || *original.Runtime == *copy.Runtime {
			t.Errorf("Original Runtime was modified: expected %v, got %v", "incus", *copy.Runtime)
		}
	})

	t.Run("CopyWithNilValues", func(t *testing.T) {
		original := &VMConfig{
			Address: nil,
			Arch:    nil,
			CPU:     nil,
			Disk:    nil,
			Driver:  nil,
			Memory:  nil,
			Runtime: nil,
		}

		copy := original.Copy()

		if !reflect.DeepEqual(original, copy) {
			t.Errorf("Copy mismatch: expected %v, got %v", original, copy)
		}
	})

	t.Run("CopyNil", func(t *testing.T) {
		var original *VMConfig = nil
		mockCopy := original.Copy()
		if mockCopy != nil {
			t.Errorf("Mock copy should be nil, got %v", mockCopy)
		}
	})
}

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrInt(i int) *int {
	return &i
}
