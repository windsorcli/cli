package workstation

import (
	"testing"
)

// TestVMConfig_Merge tests the Merge method of VMConfig
func TestVMConfig_Merge(t *testing.T) {
	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &VMConfig{
			Address: stringPtr("192.168.1.100"),
			Arch:    stringPtr("x86_64"),
			CPU:     intPtr(4),
			Disk:    intPtr(100),
			Driver:  stringPtr("qemu"),
			Memory:  intPtr(8192),
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if base.Address == nil || *base.Address != *original.Address {
			t.Errorf("Expected Address to remain unchanged")
		}
		if base.Arch == nil || *base.Arch != *original.Arch {
			t.Errorf("Expected Arch to remain unchanged")
		}
		if base.CPU == nil || *base.CPU != *original.CPU {
			t.Errorf("Expected CPU to remain unchanged")
		}
		if base.Disk == nil || *base.Disk != *original.Disk {
			t.Errorf("Expected Disk to remain unchanged")
		}
		if base.Driver == nil || *base.Driver != *original.Driver {
			t.Errorf("Expected Driver to remain unchanged")
		}
		if base.Memory == nil || *base.Memory != *original.Memory {
			t.Errorf("Expected Memory to remain unchanged")
		}
	})

	t.Run("MergeWithEmptyOverlay", func(t *testing.T) {
		base := &VMConfig{
			Address: stringPtr("192.168.1.100"),
			Arch:    stringPtr("x86_64"),
			CPU:     intPtr(4),
			Disk:    intPtr(100),
			Driver:  stringPtr("qemu"),
			Memory:  intPtr(8192),
		}
		overlay := &VMConfig{}

		base.Merge(overlay)

		if base.Address == nil || *base.Address != "192.168.1.100" {
			t.Errorf("Expected Address to remain '192.168.1.100'")
		}
		if base.Arch == nil || *base.Arch != "x86_64" {
			t.Errorf("Expected Arch to remain 'x86_64'")
		}
		if base.CPU == nil || *base.CPU != 4 {
			t.Errorf("Expected CPU to remain 4")
		}
		if base.Disk == nil || *base.Disk != 100 {
			t.Errorf("Expected Disk to remain 100")
		}
		if base.Driver == nil || *base.Driver != "qemu" {
			t.Errorf("Expected Driver to remain 'qemu'")
		}
		if base.Memory == nil || *base.Memory != 8192 {
			t.Errorf("Expected Memory to remain 8192")
		}
	})

	t.Run("MergeWithPartialOverlay", func(t *testing.T) {
		base := &VMConfig{
			Address: stringPtr("192.168.1.100"),
			Arch:    stringPtr("x86_64"),
			CPU:     intPtr(4),
			Disk:    intPtr(100),
			Driver:  stringPtr("qemu"),
			Memory:  intPtr(8192),
		}
		overlay := &VMConfig{
			Address: stringPtr("10.0.0.50"),
			Arch:    stringPtr("arm64"),
			CPU:     intPtr(8),
			Disk:    intPtr(200),
			Driver:  stringPtr("virtualbox"),
			Memory:  intPtr(16384),
		}

		base.Merge(overlay)

		if base.Address == nil || *base.Address != "10.0.0.50" {
			t.Errorf("Expected Address to be '10.0.0.50', got %s", *base.Address)
		}
		if base.Arch == nil || *base.Arch != "arm64" {
			t.Errorf("Expected Arch to be 'arm64', got %s", *base.Arch)
		}
		if base.CPU == nil || *base.CPU != 8 {
			t.Errorf("Expected CPU to be 8, got %d", *base.CPU)
		}
		if base.Disk == nil || *base.Disk != 200 {
			t.Errorf("Expected Disk to be 200, got %d", *base.Disk)
		}
		if base.Driver == nil || *base.Driver != "virtualbox" {
			t.Errorf("Expected Driver to be 'virtualbox', got %s", *base.Driver)
		}
		if base.Memory == nil || *base.Memory != 16384 {
			t.Errorf("Expected Memory to be 16384, got %d", *base.Memory)
		}
	})

	t.Run("MergeWithOnlyAddress", func(t *testing.T) {
		base := &VMConfig{
			Address: stringPtr("192.168.1.100"),
			Arch:    stringPtr("x86_64"),
			CPU:     intPtr(4),
			Disk:    intPtr(100),
			Driver:  stringPtr("qemu"),
			Memory:  intPtr(8192),
		}
		overlay := &VMConfig{
			Address: stringPtr("10.0.0.50"),
		}

		base.Merge(overlay)

		if base.Address == nil || *base.Address != "10.0.0.50" {
			t.Errorf("Expected Address to be '10.0.0.50', got %s", *base.Address)
		}
		if base.Arch == nil || *base.Arch != "x86_64" {
			t.Errorf("Expected Arch to remain 'x86_64'")
		}
		if base.CPU == nil || *base.CPU != 4 {
			t.Errorf("Expected CPU to remain 4")
		}
		if base.Disk == nil || *base.Disk != 100 {
			t.Errorf("Expected Disk to remain 100")
		}
		if base.Driver == nil || *base.Driver != "qemu" {
			t.Errorf("Expected Driver to remain 'qemu'")
		}
		if base.Memory == nil || *base.Memory != 8192 {
			t.Errorf("Expected Memory to remain 8192")
		}
	})

	t.Run("MergeWithOnlyArch", func(t *testing.T) {
		base := &VMConfig{
			Address: stringPtr("192.168.1.100"),
			Arch:    stringPtr("x86_64"),
			CPU:     intPtr(4),
			Disk:    intPtr(100),
			Driver:  stringPtr("qemu"),
			Memory:  intPtr(8192),
		}
		overlay := &VMConfig{
			Arch: stringPtr("arm64"),
		}

		base.Merge(overlay)

		if base.Address == nil || *base.Address != "192.168.1.100" {
			t.Errorf("Expected Address to remain '192.168.1.100'")
		}
		if base.Arch == nil || *base.Arch != "arm64" {
			t.Errorf("Expected Arch to be 'arm64', got %s", *base.Arch)
		}
		if base.CPU == nil || *base.CPU != 4 {
			t.Errorf("Expected CPU to remain 4")
		}
		if base.Disk == nil || *base.Disk != 100 {
			t.Errorf("Expected Disk to remain 100")
		}
		if base.Driver == nil || *base.Driver != "qemu" {
			t.Errorf("Expected Driver to remain 'qemu'")
		}
		if base.Memory == nil || *base.Memory != 8192 {
			t.Errorf("Expected Memory to remain 8192")
		}
	})

	t.Run("MergeWithOnlyCPU", func(t *testing.T) {
		base := &VMConfig{
			Address: stringPtr("192.168.1.100"),
			Arch:    stringPtr("x86_64"),
			CPU:     intPtr(4),
			Disk:    intPtr(100),
			Driver:  stringPtr("qemu"),
			Memory:  intPtr(8192),
		}
		overlay := &VMConfig{
			CPU: intPtr(8),
		}

		base.Merge(overlay)

		if base.Address == nil || *base.Address != "192.168.1.100" {
			t.Errorf("Expected Address to remain '192.168.1.100'")
		}
		if base.Arch == nil || *base.Arch != "x86_64" {
			t.Errorf("Expected Arch to remain 'x86_64'")
		}
		if base.CPU == nil || *base.CPU != 8 {
			t.Errorf("Expected CPU to be 8, got %d", *base.CPU)
		}
		if base.Disk == nil || *base.Disk != 100 {
			t.Errorf("Expected Disk to remain 100")
		}
		if base.Driver == nil || *base.Driver != "qemu" {
			t.Errorf("Expected Driver to remain 'qemu'")
		}
		if base.Memory == nil || *base.Memory != 8192 {
			t.Errorf("Expected Memory to remain 8192")
		}
	})

	t.Run("MergeWithOnlyDisk", func(t *testing.T) {
		base := &VMConfig{
			Address: stringPtr("192.168.1.100"),
			Arch:    stringPtr("x86_64"),
			CPU:     intPtr(4),
			Disk:    intPtr(100),
			Driver:  stringPtr("qemu"),
			Memory:  intPtr(8192),
		}
		overlay := &VMConfig{
			Disk: intPtr(200),
		}

		base.Merge(overlay)

		if base.Address == nil || *base.Address != "192.168.1.100" {
			t.Errorf("Expected Address to remain '192.168.1.100'")
		}
		if base.Arch == nil || *base.Arch != "x86_64" {
			t.Errorf("Expected Arch to remain 'x86_64'")
		}
		if base.CPU == nil || *base.CPU != 4 {
			t.Errorf("Expected CPU to remain 4")
		}
		if base.Disk == nil || *base.Disk != 200 {
			t.Errorf("Expected Disk to be 200, got %d", *base.Disk)
		}
		if base.Driver == nil || *base.Driver != "qemu" {
			t.Errorf("Expected Driver to remain 'qemu'")
		}
		if base.Memory == nil || *base.Memory != 8192 {
			t.Errorf("Expected Memory to remain 8192")
		}
	})

	t.Run("MergeWithOnlyDriver", func(t *testing.T) {
		base := &VMConfig{
			Address: stringPtr("192.168.1.100"),
			Arch:    stringPtr("x86_64"),
			CPU:     intPtr(4),
			Disk:    intPtr(100),
			Driver:  stringPtr("qemu"),
			Memory:  intPtr(8192),
		}
		overlay := &VMConfig{
			Driver: stringPtr("virtualbox"),
		}

		base.Merge(overlay)

		if base.Address == nil || *base.Address != "192.168.1.100" {
			t.Errorf("Expected Address to remain '192.168.1.100'")
		}
		if base.Arch == nil || *base.Arch != "x86_64" {
			t.Errorf("Expected Arch to remain 'x86_64'")
		}
		if base.CPU == nil || *base.CPU != 4 {
			t.Errorf("Expected CPU to remain 4")
		}
		if base.Disk == nil || *base.Disk != 100 {
			t.Errorf("Expected Disk to remain 100")
		}
		if base.Driver == nil || *base.Driver != "virtualbox" {
			t.Errorf("Expected Driver to be 'virtualbox', got %s", *base.Driver)
		}
		if base.Memory == nil || *base.Memory != 8192 {
			t.Errorf("Expected Memory to remain 8192")
		}
	})

	t.Run("MergeWithOnlyMemory", func(t *testing.T) {
		base := &VMConfig{
			Address: stringPtr("192.168.1.100"),
			Arch:    stringPtr("x86_64"),
			CPU:     intPtr(4),
			Disk:    intPtr(100),
			Driver:  stringPtr("qemu"),
			Memory:  intPtr(8192),
		}
		overlay := &VMConfig{
			Memory: intPtr(16384),
		}

		base.Merge(overlay)

		if base.Address == nil || *base.Address != "192.168.1.100" {
			t.Errorf("Expected Address to remain '192.168.1.100'")
		}
		if base.Arch == nil || *base.Arch != "x86_64" {
			t.Errorf("Expected Arch to remain 'x86_64'")
		}
		if base.CPU == nil || *base.CPU != 4 {
			t.Errorf("Expected CPU to remain 4")
		}
		if base.Disk == nil || *base.Disk != 100 {
			t.Errorf("Expected Disk to remain 100")
		}
		if base.Driver == nil || *base.Driver != "qemu" {
			t.Errorf("Expected Driver to remain 'qemu'")
		}
		if base.Memory == nil || *base.Memory != 16384 {
			t.Errorf("Expected Memory to be 16384, got %d", *base.Memory)
		}
	})

	t.Run("MergeWithNilBaseFields", func(t *testing.T) {
		base := &VMConfig{}
		overlay := &VMConfig{
			Address: stringPtr("10.0.0.50"),
			Arch:    stringPtr("arm64"),
			CPU:     intPtr(8),
			Disk:    intPtr(200),
			Driver:  stringPtr("virtualbox"),
			Memory:  intPtr(16384),
		}

		base.Merge(overlay)

		if base.Address == nil || *base.Address != "10.0.0.50" {
			t.Errorf("Expected Address to be '10.0.0.50'")
		}
		if base.Arch == nil || *base.Arch != "arm64" {
			t.Errorf("Expected Arch to be 'arm64'")
		}
		if base.CPU == nil || *base.CPU != 8 {
			t.Errorf("Expected CPU to be 8")
		}
		if base.Disk == nil || *base.Disk != 200 {
			t.Errorf("Expected Disk to be 200")
		}
		if base.Driver == nil || *base.Driver != "virtualbox" {
			t.Errorf("Expected Driver to be 'virtualbox'")
		}
		if base.Memory == nil || *base.Memory != 16384 {
			t.Errorf("Expected Memory to be 16384")
		}
	})
}

// TestVMConfig_Copy tests the Copy method of VMConfig
func TestVMConfig_Copy(t *testing.T) {
	t.Run("CopyNilConfig", func(t *testing.T) {
		var config *VMConfig
		copied := config.DeepCopy()

		if copied != nil {
			t.Error("Expected nil copy for nil config")
		}
	})

	t.Run("CopyEmptyConfig", func(t *testing.T) {
		config := &VMConfig{}
		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy of empty config")
		}
		if copied.Address != nil {
			t.Error("Expected Address to be nil in copy")
		}
		if copied.Arch != nil {
			t.Error("Expected Arch to be nil in copy")
		}
		if copied.CPU != nil {
			t.Error("Expected CPU to be nil in copy")
		}
		if copied.Disk != nil {
			t.Error("Expected Disk to be nil in copy")
		}
		if copied.Driver != nil {
			t.Error("Expected Driver to be nil in copy")
		}
		if copied.Memory != nil {
			t.Error("Expected Memory to be nil in copy")
		}
	})

	t.Run("CopyPopulatedConfig", func(t *testing.T) {
		config := &VMConfig{
			Address: stringPtr("192.168.1.100"),
			Arch:    stringPtr("x86_64"),
			CPU:     intPtr(4),
			Disk:    intPtr(100),
			Driver:  stringPtr("qemu"),
			Memory:  intPtr(8192),
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied == config {
			t.Error("Expected copy to be a new instance")
		}
		if copied.Address == nil || *copied.Address != *config.Address {
			t.Errorf("Expected Address to be copied correctly")
		}
		if copied.Arch == nil || *copied.Arch != *config.Arch {
			t.Errorf("Expected Arch to be copied correctly")
		}
		if copied.CPU == nil || *copied.CPU != *config.CPU {
			t.Errorf("Expected CPU to be copied correctly")
		}
		if copied.Disk == nil || *copied.Disk != *config.Disk {
			t.Errorf("Expected Disk to be copied correctly")
		}
		if copied.Driver == nil || *copied.Driver != *config.Driver {
			t.Errorf("Expected Driver to be copied correctly")
		}
		if copied.Memory == nil || *copied.Memory != *config.Memory {
			t.Errorf("Expected Memory to be copied correctly")
		}
	})

	t.Run("CopyWithPartialFields", func(t *testing.T) {
		config := &VMConfig{
			Address: stringPtr("192.168.1.100"),
			CPU:     intPtr(4),
			Memory:  intPtr(8192),
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.Address == nil || *copied.Address != *config.Address {
			t.Errorf("Expected Address to be copied correctly")
		}
		if copied.Arch != nil {
			t.Error("Expected Arch to be nil in copy")
		}
		if copied.CPU == nil || *copied.CPU != *config.CPU {
			t.Errorf("Expected CPU to be copied correctly")
		}
		if copied.Disk != nil {
			t.Error("Expected Disk to be nil in copy")
		}
		if copied.Driver != nil {
			t.Error("Expected Driver to be nil in copy")
		}
		if copied.Memory == nil || *copied.Memory != *config.Memory {
			t.Errorf("Expected Memory to be copied correctly")
		}
	})

	t.Run("CopyWithIndependentValues", func(t *testing.T) {
		config := &VMConfig{
			Address: stringPtr("192.168.1.100"),
			Arch:    stringPtr("x86_64"),
			CPU:     intPtr(4),
			Disk:    intPtr(100),
			Driver:  stringPtr("qemu"),
			Memory:  intPtr(8192),
		}

		copied := config.DeepCopy()

		// Modify original to verify independence
		*config.Address = "10.0.0.50"
		*config.Arch = "arm64"
		*config.CPU = 8
		*config.Disk = 200
		*config.Driver = "virtualbox"
		*config.Memory = 16384

		if *copied.Address != "192.168.1.100" {
			t.Error("Expected copied Address to remain independent")
		}
		if *copied.Arch != "x86_64" {
			t.Error("Expected copied Arch to remain independent")
		}
		if *copied.CPU != 4 {
			t.Error("Expected copied CPU to remain independent")
		}
		if *copied.Disk != 100 {
			t.Error("Expected copied Disk to remain independent")
		}
		if *copied.Driver != "qemu" {
			t.Error("Expected copied Driver to remain independent")
		}
		if *copied.Memory != 8192 {
			t.Error("Expected copied Memory to remain independent")
		}
	})

	t.Run("CopyWithSingleField", func(t *testing.T) {
		config := &VMConfig{
			Address: stringPtr("192.168.1.100"),
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.Address == nil || *copied.Address != *config.Address {
			t.Errorf("Expected Address to be copied correctly")
		}
		if copied.Arch != nil {
			t.Error("Expected Arch to be nil in copy")
		}
		if copied.CPU != nil {
			t.Error("Expected CPU to be nil in copy")
		}
		if copied.Disk != nil {
			t.Error("Expected Disk to be nil in copy")
		}
		if copied.Driver != nil {
			t.Error("Expected Driver to be nil in copy")
		}
		if copied.Memory != nil {
			t.Error("Expected Memory to be nil in copy")
		}
	})
}

func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
