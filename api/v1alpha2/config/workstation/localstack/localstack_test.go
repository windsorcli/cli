package workstation

import (
	"testing"
)

// TestLocalstackConfig_Merge tests the Merge method of LocalstackConfig
func TestLocalstackConfig_Merge(t *testing.T) {
	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &LocalstackConfig{
			Enabled:  boolPtr(true),
			Services: []string{"s3", "dynamodb", "lambda"},
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if base.Enabled == nil || *base.Enabled != *original.Enabled {
			t.Errorf("Expected Enabled to remain unchanged")
		}
		if len(base.Services) != len(original.Services) {
			t.Errorf("Expected Services to remain unchanged")
		}
		for i, service := range base.Services {
			if service != original.Services[i] {
				t.Errorf("Expected Services[%d] to remain unchanged", i)
			}
		}
	})

	t.Run("MergeWithEmptyOverlay", func(t *testing.T) {
		base := &LocalstackConfig{
			Enabled:  boolPtr(true),
			Services: []string{"s3", "dynamodb", "lambda"},
		}
		overlay := &LocalstackConfig{}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Expected Enabled to remain true")
		}
		if len(base.Services) != 3 {
			t.Errorf("Expected Services to remain unchanged")
		}
		expectedServices := []string{"s3", "dynamodb", "lambda"}
		for i, service := range base.Services {
			if service != expectedServices[i] {
				t.Errorf("Expected Services[%d] to be '%s', got '%s'", i, expectedServices[i], service)
			}
		}
	})

	t.Run("MergeWithPartialOverlay", func(t *testing.T) {
		base := &LocalstackConfig{
			Enabled:  boolPtr(false),
			Services: []string{"s3", "dynamodb"},
		}
		overlay := &LocalstackConfig{
			Enabled:  boolPtr(true),
			Services: []string{"lambda", "sqs", "sns"},
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Expected Enabled to be true, got %v", *base.Enabled)
		}
		if len(base.Services) != 3 {
			t.Errorf("Expected 3 services, got %d", len(base.Services))
		}
		expectedServices := []string{"lambda", "sqs", "sns"}
		for i, service := range base.Services {
			if service != expectedServices[i] {
				t.Errorf("Expected Services[%d] to be '%s', got '%s'", i, expectedServices[i], service)
			}
		}
	})

	t.Run("MergeWithOnlyEnabledOverlay", func(t *testing.T) {
		base := &LocalstackConfig{
			Enabled:  boolPtr(false),
			Services: []string{"s3", "dynamodb", "lambda"},
		}
		overlay := &LocalstackConfig{
			Enabled: boolPtr(true),
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Expected Enabled to be true, got %v", *base.Enabled)
		}
		if len(base.Services) != 3 {
			t.Errorf("Expected Services to remain unchanged")
		}
		expectedServices := []string{"s3", "dynamodb", "lambda"}
		for i, service := range base.Services {
			if service != expectedServices[i] {
				t.Errorf("Expected Services[%d] to remain '%s', got '%s'", i, expectedServices[i], service)
			}
		}
	})

	t.Run("MergeWithOnlyServicesOverlay", func(t *testing.T) {
		base := &LocalstackConfig{
			Enabled:  boolPtr(false),
			Services: []string{"s3", "dynamodb"},
		}
		overlay := &LocalstackConfig{
			Services: []string{"lambda", "sqs", "sns"},
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Expected Enabled to remain false, got %v", *base.Enabled)
		}
		if len(base.Services) != 3 {
			t.Errorf("Expected 3 services, got %d", len(base.Services))
		}
		expectedServices := []string{"lambda", "sqs", "sns"}
		for i, service := range base.Services {
			if service != expectedServices[i] {
				t.Errorf("Expected Services[%d] to be '%s', got '%s'", i, expectedServices[i], service)
			}
		}
	})

	t.Run("MergeWithNilBaseEnabled", func(t *testing.T) {
		base := &LocalstackConfig{
			Services: []string{"s3", "dynamodb"},
		}
		overlay := &LocalstackConfig{
			Enabled:  boolPtr(true),
			Services: []string{"lambda", "sqs"},
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Expected Enabled to be true, got %v", *base.Enabled)
		}
		if len(base.Services) != 2 {
			t.Errorf("Expected 2 services, got %d", len(base.Services))
		}
		expectedServices := []string{"lambda", "sqs"}
		for i, service := range base.Services {
			if service != expectedServices[i] {
				t.Errorf("Expected Services[%d] to be '%s', got '%s'", i, expectedServices[i], service)
			}
		}
	})

	t.Run("MergeWithNilBaseServices", func(t *testing.T) {
		base := &LocalstackConfig{
			Enabled: boolPtr(false),
		}
		overlay := &LocalstackConfig{
			Enabled:  boolPtr(true),
			Services: []string{"s3", "dynamodb", "lambda"},
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Expected Enabled to be true, got %v", *base.Enabled)
		}
		if len(base.Services) != 3 {
			t.Errorf("Expected 3 services, got %d", len(base.Services))
		}
		expectedServices := []string{"s3", "dynamodb", "lambda"}
		for i, service := range base.Services {
			if service != expectedServices[i] {
				t.Errorf("Expected Services[%d] to be '%s', got '%s'", i, expectedServices[i], service)
			}
		}
	})
}

// TestLocalstackConfig_Copy tests the Copy method of LocalstackConfig
func TestLocalstackConfig_Copy(t *testing.T) {
	t.Run("CopyNilConfig", func(t *testing.T) {
		var config *LocalstackConfig
		copied := config.DeepCopy()

		if copied != nil {
			t.Error("Expected nil copy for nil config")
		}
	})

	t.Run("CopyEmptyConfig", func(t *testing.T) {
		config := &LocalstackConfig{}
		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy of empty config")
		}
		if copied.Enabled != nil {
			t.Error("Expected Enabled to be nil in copy")
		}
		if copied.Services != nil {
			t.Error("Expected Services to be nil in copy")
		}
	})

	t.Run("CopyPopulatedConfig", func(t *testing.T) {
		config := &LocalstackConfig{
			Enabled:  boolPtr(true),
			Services: []string{"s3", "dynamodb", "lambda", "sqs", "sns"},
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
		if len(copied.Services) != len(config.Services) {
			t.Errorf("Expected Services length to be copied correctly")
		}
		for i, service := range copied.Services {
			if service != config.Services[i] {
				t.Errorf("Expected Services[%d] to be copied correctly", i)
			}
		}
	})

	t.Run("CopyWithPartialFields", func(t *testing.T) {
		config := &LocalstackConfig{
			Enabled: boolPtr(true),
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.Enabled == nil || *copied.Enabled != *config.Enabled {
			t.Errorf("Expected Enabled to be copied correctly")
		}
		if copied.Services != nil {
			t.Error("Expected Services to be nil in copy")
		}
	})

	t.Run("CopyWithOnlyServices", func(t *testing.T) {
		config := &LocalstackConfig{
			Services: []string{"s3", "dynamodb"},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.Enabled != nil {
			t.Error("Expected Enabled to be nil in copy")
		}
		if len(copied.Services) != len(config.Services) {
			t.Errorf("Expected Services length to be copied correctly")
		}
		for i, service := range copied.Services {
			if service != config.Services[i] {
				t.Errorf("Expected Services[%d] to be copied correctly", i)
			}
		}
	})

	t.Run("CopyWithIndependentValues", func(t *testing.T) {
		config := &LocalstackConfig{
			Enabled:  boolPtr(true),
			Services: []string{"s3", "dynamodb", "lambda"},
		}

		copied := config.DeepCopy()

		// Modify original to verify independence
		*config.Enabled = false
		config.Services[0] = "modified-service"
		config.Services = append(config.Services, "new-service")

		if *copied.Enabled != true {
			t.Error("Expected copied Enabled to remain independent")
		}
		if len(copied.Services) != 3 {
			t.Error("Expected copied Services length to remain independent")
		}
		if copied.Services[0] != "s3" {
			t.Error("Expected copied Services[0] to remain independent")
		}
		expectedServices := []string{"s3", "dynamodb", "lambda"}
		for i, service := range copied.Services {
			if service != expectedServices[i] {
				t.Errorf("Expected copied Services[%d] to remain independent", i)
			}
		}
	})

	t.Run("CopyWithEmptyServices", func(t *testing.T) {
		config := &LocalstackConfig{
			Enabled:  boolPtr(false),
			Services: []string{},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.Enabled == nil || *copied.Enabled != *config.Enabled {
			t.Errorf("Expected Enabled to be copied correctly")
		}
		if copied.Services == nil {
			t.Error("Expected Services to be initialized as empty slice")
		}
		if len(copied.Services) != 0 {
			t.Error("Expected Services to be empty slice")
		}
	})
}

func boolPtr(b bool) *bool {
	return &b
}
