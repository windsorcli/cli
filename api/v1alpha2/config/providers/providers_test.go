package providers

import (
	"testing"

	"github.com/windsorcli/cli/api/v1alpha2/config/providers/aws"
	"github.com/windsorcli/cli/api/v1alpha2/config/providers/azure"
)

// TestProvidersConfig_Merge tests the Merge method of ProvidersConfig
func TestProvidersConfig_Merge(t *testing.T) {
	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(true),
			},
			Azure: &azure.AzureConfig{
				Enabled: boolPtr(false),
			},
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if base.AWS == nil || *base.AWS.Enabled != *original.AWS.Enabled {
			t.Errorf("Expected AWS config to remain unchanged")
		}
		if base.Azure == nil || *base.Azure.Enabled != *original.Azure.Enabled {
			t.Errorf("Expected Azure config to remain unchanged")
		}
	})

	t.Run("MergeWithEmptyOverlay", func(t *testing.T) {
		base := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(true),
			},
			Azure: &azure.AzureConfig{
				Enabled: boolPtr(false),
			},
		}
		overlay := &ProvidersConfig{}

		base.Merge(overlay)

		if base.AWS == nil || !*base.AWS.Enabled {
			t.Errorf("Expected AWS config to remain enabled")
		}
		if base.Azure == nil || *base.Azure.Enabled {
			t.Errorf("Expected Azure config to remain disabled")
		}
	})

	t.Run("MergeWithPartialOverlay", func(t *testing.T) {
		base := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(true),
			},
			Azure: &azure.AzureConfig{
				Enabled: boolPtr(false),
			},
		}
		overlay := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(false),
			},
			Azure: &azure.AzureConfig{
				Enabled: boolPtr(true),
			},
		}

		base.Merge(overlay)

		if base.AWS == nil || *base.AWS.Enabled {
			t.Errorf("Expected AWS config to be disabled after merge")
		}
		if base.Azure == nil || !*base.Azure.Enabled {
			t.Errorf("Expected Azure config to be enabled after merge")
		}
	})

	t.Run("MergeWithOnlyAWS", func(t *testing.T) {
		base := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(true),
			},
			Azure: &azure.AzureConfig{
				Enabled: boolPtr(false),
			},
		}
		overlay := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(false),
			},
		}

		base.Merge(overlay)

		if base.AWS == nil || *base.AWS.Enabled {
			t.Errorf("Expected AWS config to be disabled after merge")
		}
		if base.Azure == nil || *base.Azure.Enabled {
			t.Errorf("Expected Azure config to remain disabled")
		}
	})

	t.Run("MergeWithOnlyAzure", func(t *testing.T) {
		base := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(true),
			},
			Azure: &azure.AzureConfig{
				Enabled: boolPtr(false),
			},
		}
		overlay := &ProvidersConfig{
			Azure: &azure.AzureConfig{
				Enabled: boolPtr(true),
			},
		}

		base.Merge(overlay)

		if base.AWS == nil || !*base.AWS.Enabled {
			t.Errorf("Expected AWS config to remain enabled")
		}
		if base.Azure == nil || !*base.Azure.Enabled {
			t.Errorf("Expected Azure config to be enabled after merge")
		}
	})

	t.Run("MergeWithNilBaseFields", func(t *testing.T) {
		base := &ProvidersConfig{}
		overlay := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(true),
			},
			Azure: &azure.AzureConfig{
				Enabled: boolPtr(false),
			},
		}

		base.Merge(overlay)

		if base.AWS == nil || !*base.AWS.Enabled {
			t.Errorf("Expected AWS config to be initialized and enabled")
		}
		if base.Azure == nil || *base.Azure.Enabled {
			t.Errorf("Expected Azure config to be initialized and disabled")
		}
	})

	t.Run("MergeWithNilBaseAWS", func(t *testing.T) {
		base := &ProvidersConfig{
			Azure: &azure.AzureConfig{
				Enabled: boolPtr(false),
			},
		}
		overlay := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(true),
			},
		}

		base.Merge(overlay)

		if base.AWS == nil || !*base.AWS.Enabled {
			t.Errorf("Expected AWS config to be initialized and enabled")
		}
		if base.Azure == nil || *base.Azure.Enabled {
			t.Errorf("Expected Azure config to remain disabled")
		}
	})

	t.Run("MergeWithNilBaseAzure", func(t *testing.T) {
		base := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(true),
			},
		}
		overlay := &ProvidersConfig{
			Azure: &azure.AzureConfig{
				Enabled: boolPtr(false),
			},
		}

		base.Merge(overlay)

		if base.AWS == nil || !*base.AWS.Enabled {
			t.Errorf("Expected AWS config to remain enabled")
		}
		if base.Azure == nil || *base.Azure.Enabled {
			t.Errorf("Expected Azure config to be initialized and disabled")
		}
	})
}

// TestProvidersConfig_Copy tests the Copy method of ProvidersConfig
func TestProvidersConfig_Copy(t *testing.T) {
	t.Run("CopyNilConfig", func(t *testing.T) {
		var config *ProvidersConfig
		copied := config.DeepCopy()

		if copied != nil {
			t.Error("Expected nil copy for nil config")
		}
	})

	t.Run("CopyEmptyConfig", func(t *testing.T) {
		config := &ProvidersConfig{}
		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy of empty config")
		}
		if copied.AWS != nil {
			t.Error("Expected AWS to be nil in copy")
		}
		if copied.Azure != nil {
			t.Error("Expected Azure to be nil in copy")
		}
	})

	t.Run("CopyPopulatedConfig", func(t *testing.T) {
		config := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(true),
			},
			Azure: &azure.AzureConfig{
				Enabled: boolPtr(false),
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied == config {
			t.Error("Expected copy to be a new instance")
		}
		if copied.AWS == nil || *copied.AWS.Enabled != *config.AWS.Enabled {
			t.Errorf("Expected AWS config to be copied correctly")
		}
		if copied.Azure == nil || *copied.Azure.Enabled != *config.Azure.Enabled {
			t.Errorf("Expected Azure config to be copied correctly")
		}
	})

	t.Run("CopyWithPartialFields", func(t *testing.T) {
		config := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(true),
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.AWS == nil || *copied.AWS.Enabled != *config.AWS.Enabled {
			t.Errorf("Expected AWS config to be copied correctly")
		}
		if copied.Azure != nil {
			t.Error("Expected Azure to be nil in copy")
		}
	})

	t.Run("CopyWithIndependentValues", func(t *testing.T) {
		config := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(true),
			},
			Azure: &azure.AzureConfig{
				Enabled: boolPtr(false),
			},
		}

		copied := config.DeepCopy()

		// Modify original to verify independence
		*config.AWS.Enabled = false
		*config.Azure.Enabled = true

		if *copied.AWS.Enabled != true {
			t.Error("Expected copied AWS config to remain independent")
		}
		if *copied.Azure.Enabled != false {
			t.Error("Expected copied Azure config to remain independent")
		}
	})

	t.Run("CopyWithSingleField", func(t *testing.T) {
		config := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				Enabled: boolPtr(true),
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.AWS == nil || *copied.AWS.Enabled != *config.AWS.Enabled {
			t.Errorf("Expected AWS config to be copied correctly")
		}
		if copied.Azure != nil {
			t.Error("Expected Azure to be nil in copy")
		}
	})
}

func boolPtr(b bool) *bool {
	return &b
}
