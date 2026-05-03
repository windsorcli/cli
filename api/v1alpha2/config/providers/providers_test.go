package providers

import (
	"testing"

	"github.com/windsorcli/cli/api/v1alpha2/config/providers/aws"
	"github.com/windsorcli/cli/api/v1alpha2/config/providers/azure"
)

// awsProfileA/B and azureTenantA/B stand in for the two distinct states the merge/copy tests
// previously expressed via aws.Enabled / azure.Enabled = true/false. The Enabled fields have
// been removed in both providers; any non-nil leaf field works as the "did the merge run"
// signal — these tests use AWSProfile and TenantID since they're the most operator-visible.
const (
	awsProfileA  = "profile-a"
	awsProfileB  = "profile-b"
	azureTenantA = "tenant-a"
	azureTenantB = "tenant-b"
)

// TestProvidersConfig_Merge tests the Merge method of ProvidersConfig
func TestProvidersConfig_Merge(t *testing.T) {
	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileA),
			},
			Azure: &azure.AzureConfig{
				TenantID: stringPtr(azureTenantA),
			},
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if base.AWS == nil || *base.AWS.AWSProfile != *original.AWS.AWSProfile {
			t.Errorf("Expected AWS config to remain unchanged")
		}
		if base.Azure == nil || *base.Azure.TenantID != *original.Azure.TenantID {
			t.Errorf("Expected Azure config to remain unchanged")
		}
	})

	t.Run("MergeWithEmptyOverlay", func(t *testing.T) {
		base := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileA),
			},
			Azure: &azure.AzureConfig{
				TenantID: stringPtr(azureTenantA),
			},
		}
		overlay := &ProvidersConfig{}

		base.Merge(overlay)

		if base.AWS == nil || *base.AWS.AWSProfile != awsProfileA {
			t.Errorf("Expected AWS config to remain at profile %q", awsProfileA)
		}
		if base.Azure == nil || *base.Azure.TenantID != azureTenantA {
			t.Errorf("Expected Azure config to remain at tenant %q", azureTenantA)
		}
	})

	t.Run("MergeWithPartialOverlay", func(t *testing.T) {
		base := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileA),
			},
			Azure: &azure.AzureConfig{
				TenantID: stringPtr(azureTenantA),
			},
		}
		overlay := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileB),
			},
			Azure: &azure.AzureConfig{
				TenantID: stringPtr(azureTenantB),
			},
		}

		base.Merge(overlay)

		if base.AWS == nil || *base.AWS.AWSProfile != awsProfileB {
			t.Errorf("Expected AWS profile to be %q after merge, got %q", awsProfileB, *base.AWS.AWSProfile)
		}
		if base.Azure == nil || *base.Azure.TenantID != azureTenantB {
			t.Errorf("Expected Azure tenant to be %q after merge, got %q", azureTenantB, *base.Azure.TenantID)
		}
	})

	t.Run("MergeWithOnlyAWS", func(t *testing.T) {
		base := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileA),
			},
			Azure: &azure.AzureConfig{
				TenantID: stringPtr(azureTenantA),
			},
		}
		overlay := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileB),
			},
		}

		base.Merge(overlay)

		if base.AWS == nil || *base.AWS.AWSProfile != awsProfileB {
			t.Errorf("Expected AWS profile to be %q after merge", awsProfileB)
		}
		if base.Azure == nil || *base.Azure.TenantID != azureTenantA {
			t.Errorf("Expected Azure tenant to remain %q", azureTenantA)
		}
	})

	t.Run("MergeWithOnlyAzure", func(t *testing.T) {
		base := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileA),
			},
			Azure: &azure.AzureConfig{
				TenantID: stringPtr(azureTenantA),
			},
		}
		overlay := &ProvidersConfig{
			Azure: &azure.AzureConfig{
				TenantID: stringPtr(azureTenantB),
			},
		}

		base.Merge(overlay)

		if base.AWS == nil || *base.AWS.AWSProfile != awsProfileA {
			t.Errorf("Expected AWS profile to remain %q", awsProfileA)
		}
		if base.Azure == nil || *base.Azure.TenantID != azureTenantB {
			t.Errorf("Expected Azure tenant to be %q after merge", azureTenantB)
		}
	})

	t.Run("MergeWithNilBaseFields", func(t *testing.T) {
		base := &ProvidersConfig{}
		overlay := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileA),
			},
			Azure: &azure.AzureConfig{
				TenantID: stringPtr(azureTenantA),
			},
		}

		base.Merge(overlay)

		if base.AWS == nil || *base.AWS.AWSProfile != awsProfileA {
			t.Errorf("Expected AWS config to be initialized with profile %q", awsProfileA)
		}
		if base.Azure == nil || *base.Azure.TenantID != azureTenantA {
			t.Errorf("Expected Azure config to be initialized with tenant %q", azureTenantA)
		}
	})

	t.Run("MergeWithNilBaseAWS", func(t *testing.T) {
		base := &ProvidersConfig{
			Azure: &azure.AzureConfig{
				TenantID: stringPtr(azureTenantA),
			},
		}
		overlay := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileA),
			},
		}

		base.Merge(overlay)

		if base.AWS == nil || *base.AWS.AWSProfile != awsProfileA {
			t.Errorf("Expected AWS config to be initialized with profile %q", awsProfileA)
		}
		if base.Azure == nil || *base.Azure.TenantID != azureTenantA {
			t.Errorf("Expected Azure tenant to remain %q", azureTenantA)
		}
	})

	t.Run("MergeWithNilBaseAzure", func(t *testing.T) {
		base := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileA),
			},
		}
		overlay := &ProvidersConfig{
			Azure: &azure.AzureConfig{
				TenantID: stringPtr(azureTenantA),
			},
		}

		base.Merge(overlay)

		if base.AWS == nil || *base.AWS.AWSProfile != awsProfileA {
			t.Errorf("Expected AWS profile to remain %q", awsProfileA)
		}
		if base.Azure == nil || *base.Azure.TenantID != azureTenantA {
			t.Errorf("Expected Azure config to be initialized with tenant %q", azureTenantA)
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
				AWSProfile: stringPtr(awsProfileA),
			},
			Azure: &azure.AzureConfig{
				TenantID: stringPtr(azureTenantA),
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied == config {
			t.Error("Expected copy to be a new instance")
		}
		if copied.AWS == nil || *copied.AWS.AWSProfile != *config.AWS.AWSProfile {
			t.Errorf("Expected AWS config to be copied correctly")
		}
		if copied.Azure == nil || *copied.Azure.TenantID != *config.Azure.TenantID {
			t.Errorf("Expected Azure config to be copied correctly")
		}
	})

	t.Run("CopyWithPartialFields", func(t *testing.T) {
		config := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileA),
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.AWS == nil || *copied.AWS.AWSProfile != *config.AWS.AWSProfile {
			t.Errorf("Expected AWS config to be copied correctly")
		}
		if copied.Azure != nil {
			t.Error("Expected Azure to be nil in copy")
		}
	})

	t.Run("CopyWithIndependentValues", func(t *testing.T) {
		config := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileA),
			},
			Azure: &azure.AzureConfig{
				TenantID: stringPtr(azureTenantA),
			},
		}

		copied := config.DeepCopy()

		// Modify original to verify independence
		*config.AWS.AWSProfile = awsProfileB
		*config.Azure.TenantID = azureTenantB

		if *copied.AWS.AWSProfile != awsProfileA {
			t.Error("Expected copied AWS config to remain independent")
		}
		if *copied.Azure.TenantID != azureTenantA {
			t.Error("Expected copied Azure config to remain independent")
		}
	})

	t.Run("CopyWithSingleField", func(t *testing.T) {
		config := &ProvidersConfig{
			AWS: &aws.AWSConfig{
				AWSProfile: stringPtr(awsProfileA),
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.AWS == nil || *copied.AWS.AWSProfile != *config.AWS.AWSProfile {
			t.Errorf("Expected AWS config to be copied correctly")
		}
		if copied.Azure != nil {
			t.Error("Expected Azure to be nil in copy")
		}
	})
}

func stringPtr(s string) *string {
	return &s
}
