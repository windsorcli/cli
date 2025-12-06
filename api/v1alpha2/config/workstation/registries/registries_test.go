package workstation

import (
	"testing"
)

// TestRegistriesConfig_Merge tests the Merge method of RegistriesConfig
func TestRegistriesConfig_Merge(t *testing.T) {
	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &RegistriesConfig{
			Enabled: boolPtr(true),
			Registries: map[string]RegistryConfig{
				"docker.io": {
					Remote:   "docker.io",
					Local:    "localhost:5000",
					HostName: "localhost",
					HostPort: 5000,
				},
				"quay.io": {
					Remote:   "quay.io",
					Local:    "localhost:5001",
					HostName: "localhost",
					HostPort: 5001,
				},
			},
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if base.Enabled == nil || *base.Enabled != *original.Enabled {
			t.Errorf("Expected Enabled to remain unchanged")
		}
		if len(base.Registries) != len(original.Registries) {
			t.Errorf("Expected Registries to remain unchanged")
		}
		for name, registry := range base.Registries {
			if registry.Remote != original.Registries[name].Remote {
				t.Errorf("Expected Registries[%s].Remote to remain unchanged", name)
			}
			if registry.Local != original.Registries[name].Local {
				t.Errorf("Expected Registries[%s].Local to remain unchanged", name)
			}
			if registry.HostName != original.Registries[name].HostName {
				t.Errorf("Expected Registries[%s].HostName to remain unchanged", name)
			}
			if registry.HostPort != original.Registries[name].HostPort {
				t.Errorf("Expected Registries[%s].HostPort to remain unchanged", name)
			}
		}
	})

	t.Run("MergeWithEmptyOverlay", func(t *testing.T) {
		base := &RegistriesConfig{
			Enabled: boolPtr(true),
			Registries: map[string]RegistryConfig{
				"docker.io": {
					Remote:   "docker.io",
					Local:    "localhost:5000",
					HostName: "localhost",
					HostPort: 5000,
				},
			},
		}
		overlay := &RegistriesConfig{}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Expected Enabled to remain true")
		}
		if len(base.Registries) != 1 {
			t.Errorf("Expected Registries to remain unchanged")
		}
		if base.Registries["docker.io"].Remote != "docker.io" {
			t.Errorf("Expected Registries[docker.io].Remote to remain 'docker.io'")
		}
		if base.Registries["docker.io"].Local != "localhost:5000" {
			t.Errorf("Expected Registries[docker.io].Local to remain 'localhost:5000'")
		}
		if base.Registries["docker.io"].HostName != "localhost" {
			t.Errorf("Expected Registries[docker.io].HostName to remain 'localhost'")
		}
		if base.Registries["docker.io"].HostPort != 5000 {
			t.Errorf("Expected Registries[docker.io].HostPort to remain 5000")
		}
	})

	t.Run("MergeWithPartialOverlay", func(t *testing.T) {
		base := &RegistriesConfig{
			Enabled: boolPtr(false),
			Registries: map[string]RegistryConfig{
				"docker.io": {
					Remote:   "docker.io",
					Local:    "localhost:5000",
					HostName: "localhost",
					HostPort: 5000,
				},
			},
		}
		overlay := &RegistriesConfig{
			Enabled: boolPtr(true),
			Registries: map[string]RegistryConfig{
				"quay.io": {
					Remote:   "quay.io",
					Local:    "localhost:5001",
					HostName: "localhost",
					HostPort: 5001,
				},
				"gcr.io": {
					Remote:   "gcr.io",
					Local:    "localhost:5002",
					HostName: "localhost",
					HostPort: 5002,
				},
			},
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Expected Enabled to be true, got %v", *base.Enabled)
		}
		if len(base.Registries) != 2 {
			t.Errorf("Expected 2 registries, got %d", len(base.Registries))
		}
		if base.Registries["quay.io"].Remote != "quay.io" {
			t.Errorf("Expected Registries[quay.io].Remote to be 'quay.io'")
		}
		if base.Registries["quay.io"].Local != "localhost:5001" {
			t.Errorf("Expected Registries[quay.io].Local to be 'localhost:5001'")
		}
		if base.Registries["quay.io"].HostName != "localhost" {
			t.Errorf("Expected Registries[quay.io].HostName to be 'localhost'")
		}
		if base.Registries["quay.io"].HostPort != 5001 {
			t.Errorf("Expected Registries[quay.io].HostPort to be 5001")
		}
		if base.Registries["gcr.io"].Remote != "gcr.io" {
			t.Errorf("Expected Registries[gcr.io].Remote to be 'gcr.io'")
		}
		if base.Registries["gcr.io"].Local != "localhost:5002" {
			t.Errorf("Expected Registries[gcr.io].Local to be 'localhost:5002'")
		}
		if base.Registries["gcr.io"].HostName != "localhost" {
			t.Errorf("Expected Registries[gcr.io].HostName to be 'localhost'")
		}
		if base.Registries["gcr.io"].HostPort != 5002 {
			t.Errorf("Expected Registries[gcr.io].HostPort to be 5002")
		}
	})

	t.Run("MergeWithOnlyEnabled", func(t *testing.T) {
		base := &RegistriesConfig{
			Enabled: boolPtr(false),
			Registries: map[string]RegistryConfig{
				"docker.io": {
					Remote:   "docker.io",
					Local:    "localhost:5000",
					HostName: "localhost",
					HostPort: 5000,
				},
			},
		}
		overlay := &RegistriesConfig{
			Enabled: boolPtr(true),
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Expected Enabled to be true, got %v", *base.Enabled)
		}
		if len(base.Registries) != 1 {
			t.Errorf("Expected Registries to remain unchanged")
		}
		if base.Registries["docker.io"].Remote != "docker.io" {
			t.Errorf("Expected Registries[docker.io].Remote to remain 'docker.io'")
		}
		if base.Registries["docker.io"].Local != "localhost:5000" {
			t.Errorf("Expected Registries[docker.io].Local to remain 'localhost:5000'")
		}
		if base.Registries["docker.io"].HostName != "localhost" {
			t.Errorf("Expected Registries[docker.io].HostName to remain 'localhost'")
		}
		if base.Registries["docker.io"].HostPort != 5000 {
			t.Errorf("Expected Registries[docker.io].HostPort to remain 5000")
		}
	})

	t.Run("MergeWithOnlyRegistries", func(t *testing.T) {
		base := &RegistriesConfig{
			Enabled: boolPtr(false),
		}
		overlay := &RegistriesConfig{
			Registries: map[string]RegistryConfig{
				"quay.io": {
					Remote:   "quay.io",
					Local:    "localhost:5001",
					HostName: "localhost",
					HostPort: 5001,
				},
			},
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Expected Enabled to remain false, got %v", *base.Enabled)
		}
		if len(base.Registries) != 1 {
			t.Errorf("Expected 1 registry, got %d", len(base.Registries))
		}
		if base.Registries["quay.io"].Remote != "quay.io" {
			t.Errorf("Expected Registries[quay.io].Remote to be 'quay.io'")
		}
		if base.Registries["quay.io"].Local != "localhost:5001" {
			t.Errorf("Expected Registries[quay.io].Local to be 'localhost:5001'")
		}
		if base.Registries["quay.io"].HostName != "localhost" {
			t.Errorf("Expected Registries[quay.io].HostName to be 'localhost'")
		}
		if base.Registries["quay.io"].HostPort != 5001 {
			t.Errorf("Expected Registries[quay.io].HostPort to be 5001")
		}
	})

	t.Run("MergeWithNilBaseEnabled", func(t *testing.T) {
		base := &RegistriesConfig{
			Registries: map[string]RegistryConfig{
				"docker.io": {
					Remote:   "docker.io",
					Local:    "localhost:5000",
					HostName: "localhost",
					HostPort: 5000,
				},
			},
		}
		overlay := &RegistriesConfig{
			Enabled: boolPtr(true),
			Registries: map[string]RegistryConfig{
				"quay.io": {
					Remote:   "quay.io",
					Local:    "localhost:5001",
					HostName: "localhost",
					HostPort: 5001,
				},
			},
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Expected Enabled to be true, got %v", *base.Enabled)
		}
		if len(base.Registries) != 1 {
			t.Errorf("Expected 1 registry, got %d", len(base.Registries))
		}
		if base.Registries["quay.io"].Remote != "quay.io" {
			t.Errorf("Expected Registries[quay.io].Remote to be 'quay.io'")
		}
		if base.Registries["quay.io"].Local != "localhost:5001" {
			t.Errorf("Expected Registries[quay.io].Local to be 'localhost:5001'")
		}
		if base.Registries["quay.io"].HostName != "localhost" {
			t.Errorf("Expected Registries[quay.io].HostName to be 'localhost'")
		}
		if base.Registries["quay.io"].HostPort != 5001 {
			t.Errorf("Expected Registries[quay.io].HostPort to be 5001")
		}
	})

	t.Run("MergeWithNilBaseRegistries", func(t *testing.T) {
		base := &RegistriesConfig{
			Enabled: boolPtr(false),
		}
		overlay := &RegistriesConfig{
			Enabled: boolPtr(true),
			Registries: map[string]RegistryConfig{
				"docker.io": {
					Remote:   "docker.io",
					Local:    "localhost:5000",
					HostName: "localhost",
					HostPort: 5000,
				},
				"quay.io": {
					Remote:   "quay.io",
					Local:    "localhost:5001",
					HostName: "localhost",
					HostPort: 5001,
				},
			},
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Expected Enabled to be true, got %v", *base.Enabled)
		}
		if len(base.Registries) != 2 {
			t.Errorf("Expected 2 registries, got %d", len(base.Registries))
		}
		if base.Registries["docker.io"].Remote != "docker.io" {
			t.Errorf("Expected Registries[docker.io].Remote to be 'docker.io'")
		}
		if base.Registries["docker.io"].Local != "localhost:5000" {
			t.Errorf("Expected Registries[docker.io].Local to be 'localhost:5000'")
		}
		if base.Registries["docker.io"].HostName != "localhost" {
			t.Errorf("Expected Registries[docker.io].HostName to be 'localhost'")
		}
		if base.Registries["docker.io"].HostPort != 5000 {
			t.Errorf("Expected Registries[docker.io].HostPort to be 5000")
		}
		if base.Registries["quay.io"].Remote != "quay.io" {
			t.Errorf("Expected Registries[quay.io].Remote to be 'quay.io'")
		}
		if base.Registries["quay.io"].Local != "localhost:5001" {
			t.Errorf("Expected Registries[quay.io].Local to be 'localhost:5001'")
		}
		if base.Registries["quay.io"].HostName != "localhost" {
			t.Errorf("Expected Registries[quay.io].HostName to be 'localhost'")
		}
		if base.Registries["quay.io"].HostPort != 5001 {
			t.Errorf("Expected Registries[quay.io].HostPort to be 5001")
		}
	})
}

// TestRegistriesConfig_Copy tests the Copy method of RegistriesConfig
func TestRegistriesConfig_Copy(t *testing.T) {
	t.Run("CopyNilConfig", func(t *testing.T) {
		var config *RegistriesConfig
		copied := config.DeepCopy()

		if copied != nil {
			t.Error("Expected nil copy for nil config")
		}
	})

	t.Run("CopyEmptyConfig", func(t *testing.T) {
		config := &RegistriesConfig{}
		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy of empty config")
		}
		if copied.Enabled != nil {
			t.Error("Expected Enabled to be nil in copy")
		}
		if copied.Registries != nil {
			t.Error("Expected Registries to be nil in copy")
		}
	})

	t.Run("CopyPopulatedConfig", func(t *testing.T) {
		config := &RegistriesConfig{
			Enabled: boolPtr(true),
			Registries: map[string]RegistryConfig{
				"docker.io": {
					Remote:   "docker.io",
					Local:    "localhost:5000",
					HostName: "localhost",
					HostPort: 5000,
				},
				"quay.io": {
					Remote:   "quay.io",
					Local:    "localhost:5001",
					HostName: "localhost",
					HostPort: 5001,
				},
				"gcr.io": {
					Remote:   "gcr.io",
					Local:    "localhost:5002",
					HostName: "localhost",
					HostPort: 5002,
				},
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied == config {
			t.Error("Expected copy to be a new instance")
		}
		if copied.Enabled == nil || *copied.Enabled != *config.Enabled {
			t.Errorf("Expected Enabled to be copied correctly")
		}
		if len(copied.Registries) != len(config.Registries) {
			t.Errorf("Expected Registries length to be copied correctly")
		}
		for name, registry := range copied.Registries {
			if registry.Remote != config.Registries[name].Remote {
				t.Errorf("Expected Registries[%s].Remote to be copied correctly", name)
			}
			if registry.Local != config.Registries[name].Local {
				t.Errorf("Expected Registries[%s].Local to be copied correctly", name)
			}
			if registry.HostName != config.Registries[name].HostName {
				t.Errorf("Expected Registries[%s].HostName to be copied correctly", name)
			}
			if registry.HostPort != config.Registries[name].HostPort {
				t.Errorf("Expected Registries[%s].HostPort to be copied correctly", name)
			}
		}
	})

	t.Run("CopyWithPartialFields", func(t *testing.T) {
		config := &RegistriesConfig{
			Enabled: boolPtr(true),
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.Enabled == nil || *copied.Enabled != *config.Enabled {
			t.Errorf("Expected Enabled to be copied correctly")
		}
		if copied.Registries != nil {
			t.Error("Expected Registries to be nil in copy")
		}
	})

	t.Run("CopyWithOnlyRegistries", func(t *testing.T) {
		config := &RegistriesConfig{
			Registries: map[string]RegistryConfig{
				"docker.io": {
					Remote:   "docker.io",
					Local:    "localhost:5000",
					HostName: "localhost",
					HostPort: 5000,
				},
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.Enabled != nil {
			t.Error("Expected Enabled to be nil in copy")
		}
		if len(copied.Registries) != len(config.Registries) {
			t.Errorf("Expected Registries length to be copied correctly")
		}
		for name, registry := range copied.Registries {
			if registry.Remote != config.Registries[name].Remote {
				t.Errorf("Expected Registries[%s].Remote to be copied correctly", name)
			}
			if registry.Local != config.Registries[name].Local {
				t.Errorf("Expected Registries[%s].Local to be copied correctly", name)
			}
			if registry.HostName != config.Registries[name].HostName {
				t.Errorf("Expected Registries[%s].HostName to be copied correctly", name)
			}
			if registry.HostPort != config.Registries[name].HostPort {
				t.Errorf("Expected Registries[%s].HostPort to be copied correctly", name)
			}
		}
	})

	t.Run("CopyWithIndependentValues", func(t *testing.T) {
		config := &RegistriesConfig{
			Enabled: boolPtr(true),
			Registries: map[string]RegistryConfig{
				"docker.io": {
					Remote:   "docker.io",
					Local:    "localhost:5000",
					HostName: "localhost",
					HostPort: 5000,
				},
				"quay.io": {
					Remote:   "quay.io",
					Local:    "localhost:5001",
					HostName: "localhost",
					HostPort: 5001,
				},
			},
		}

		copied := config.DeepCopy()

		// Modify original to verify independence
		*config.Enabled = false
		config.Registries["docker.io"] = RegistryConfig{
			Remote:   "modified-docker.io",
			Local:    "modified-localhost:5000",
			HostName: "modified-localhost",
			HostPort: 9999,
		}
		config.Registries["new-registry"] = RegistryConfig{
			Remote:   "new-registry.io",
			Local:    "localhost:5003",
			HostName: "localhost",
			HostPort: 5003,
		}

		if *copied.Enabled != true {
			t.Error("Expected copied Enabled to remain independent")
		}
		if len(copied.Registries) != 2 {
			t.Error("Expected copied Registries length to remain independent")
		}
		if copied.Registries["docker.io"].Remote != "docker.io" {
			t.Error("Expected copied Registries[docker.io].Remote to remain independent")
		}
		if copied.Registries["docker.io"].Local != "localhost:5000" {
			t.Error("Expected copied Registries[docker.io].Local to remain independent")
		}
		if copied.Registries["docker.io"].HostName != "localhost" {
			t.Error("Expected copied Registries[docker.io].HostName to remain independent")
		}
		if copied.Registries["docker.io"].HostPort != 5000 {
			t.Error("Expected copied Registries[docker.io].HostPort to remain independent")
		}
		if copied.Registries["quay.io"].Remote != "quay.io" {
			t.Error("Expected copied Registries[quay.io].Remote to remain independent")
		}
		if copied.Registries["quay.io"].Local != "localhost:5001" {
			t.Error("Expected copied Registries[quay.io].Local to remain independent")
		}
		if copied.Registries["quay.io"].HostName != "localhost" {
			t.Error("Expected copied Registries[quay.io].HostName to remain independent")
		}
		if copied.Registries["quay.io"].HostPort != 5001 {
			t.Error("Expected copied Registries[quay.io].HostPort to remain independent")
		}
	})

	t.Run("CopyWithEmptyRegistries", func(t *testing.T) {
		config := &RegistriesConfig{
			Enabled:    boolPtr(false),
			Registries: map[string]RegistryConfig{},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.Enabled == nil || *copied.Enabled != *config.Enabled {
			t.Errorf("Expected Enabled to be copied correctly")
		}
		if copied.Registries == nil {
			t.Error("Expected Registries to be initialized as empty map")
		}
		if len(copied.Registries) != 0 {
			t.Error("Expected Registries to be empty map")
		}
	})
}

func boolPtr(b bool) *bool {
	return &b
}
