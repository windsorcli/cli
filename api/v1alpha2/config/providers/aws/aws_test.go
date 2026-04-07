package aws

import (
	"testing"
)

func TestAWSConfig_Merge(t *testing.T) {
	t.Run("MergeWithNoNils", func(t *testing.T) {
		base := &AWSConfig{
			Enabled:        ptrBool(true),
			AWSEndpointURL: ptrString("https://base.aws.endpoint"),
			AWSProfile:     ptrString("base-profile"),
			S3Hostname:     ptrString("base-s3-hostname"),
			MWAAEndpoint:   ptrString("base-mwaa-endpoint"),
		}

		overlay := &AWSConfig{
			Enabled:        ptrBool(false),
			AWSEndpointURL: ptrString("https://overlay.aws.endpoint"),
			AWSProfile:     ptrString("overlay-profile"),
			S3Hostname:     ptrString("overlay-s3-hostname"),
			MWAAEndpoint:   ptrString("overlay-mwaa-endpoint"),
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Enabled mismatch: expected false, got %v", *base.Enabled)
		}
		if base.AWSEndpointURL == nil || *base.AWSEndpointURL != "https://overlay.aws.endpoint" {
			t.Errorf("AWSEndpointURL mismatch: expected 'https://overlay.aws.endpoint', got '%s'", *base.AWSEndpointURL)
		}
		if base.AWSProfile == nil || *base.AWSProfile != "overlay-profile" {
			t.Errorf("AWSProfile mismatch: expected 'overlay-profile', got '%s'", *base.AWSProfile)
		}
		if base.S3Hostname == nil || *base.S3Hostname != "overlay-s3-hostname" {
			t.Errorf("S3Hostname mismatch: expected 'overlay-s3-hostname', got '%s'", *base.S3Hostname)
		}
		if base.MWAAEndpoint == nil || *base.MWAAEndpoint != "overlay-mwaa-endpoint" {
			t.Errorf("MWAAEndpoint mismatch: expected 'overlay-mwaa-endpoint', got '%s'", *base.MWAAEndpoint)
		}
	})

	t.Run("MergeWithAllNils", func(t *testing.T) {
		base := &AWSConfig{
			Enabled:        nil,
			AWSEndpointURL: nil,
			AWSProfile:     nil,
			S3Hostname:     nil,
			MWAAEndpoint:   nil,
		}

		overlay := &AWSConfig{
			Enabled:        nil,
			AWSEndpointURL: nil,
			AWSProfile:     nil,
			S3Hostname:     nil,
			MWAAEndpoint:   nil,
		}

		base.Merge(overlay)

		if base.Enabled != nil {
			t.Errorf("Enabled mismatch: expected nil, got %v", base.Enabled)
		}
		if base.AWSEndpointURL != nil {
			t.Errorf("AWSEndpointURL mismatch: expected nil, got '%s'", *base.AWSEndpointURL)
		}
		if base.AWSProfile != nil {
			t.Errorf("AWSProfile mismatch: expected nil, got '%s'", *base.AWSProfile)
		}
		if base.S3Hostname != nil {
			t.Errorf("S3Hostname mismatch: expected nil, got '%s'", *base.S3Hostname)
		}
		if base.MWAAEndpoint != nil {
			t.Errorf("MWAAEndpoint mismatch: expected nil, got '%s'", *base.MWAAEndpoint)
		}
	})
}

func TestAWSConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &AWSConfig{
			Enabled:        ptrBool(true),
			AWSEndpointURL: ptrString("https://original.aws.endpoint"),
			AWSProfile:     ptrString("original-profile"),
			S3Hostname:     ptrString("original-s3-hostname"),
			MWAAEndpoint:   ptrString("original-mwaa-endpoint"),
		}

		copy := original.DeepCopy()

		if original.Enabled == nil || copy.Enabled == nil || *original.Enabled != *copy.Enabled {
			t.Errorf("Enabled mismatch: expected %v, got %v", *original.Enabled, *copy.Enabled)
		}
		if original.AWSEndpointURL == nil || copy.AWSEndpointURL == nil || *original.AWSEndpointURL != *copy.AWSEndpointURL {
			t.Errorf("AWSEndpointURL mismatch: expected %v, got %v", *original.AWSEndpointURL, *copy.AWSEndpointURL)
		}
		if original.AWSProfile == nil || copy.AWSProfile == nil || *original.AWSProfile != *copy.AWSProfile {
			t.Errorf("AWSProfile mismatch: expected %v, got %v", *original.AWSProfile, *copy.AWSProfile)
		}
		if original.S3Hostname == nil || copy.S3Hostname == nil || *original.S3Hostname != *copy.S3Hostname {
			t.Errorf("S3Hostname mismatch: expected %v, got %v", *original.S3Hostname, *copy.S3Hostname)
		}
		if original.MWAAEndpoint == nil || copy.MWAAEndpoint == nil || *original.MWAAEndpoint != *copy.MWAAEndpoint {
			t.Errorf("MWAAEndpoint mismatch: expected %v, got %v", *original.MWAAEndpoint, *copy.MWAAEndpoint)
		}

		// Modify the copy and ensure original is unchanged
		copy.Enabled = ptrBool(false)
		if original.Enabled == nil || *original.Enabled == *copy.Enabled {
			t.Errorf("Original Enabled was modified: expected %v, got %v", true, *copy.Enabled)
		}
	})

	t.Run("CopyNil", func(t *testing.T) {
		var original *AWSConfig = nil
		mockCopy := original.DeepCopy()
		if mockCopy != nil {
			t.Errorf("Mock copy should be nil, got %v", mockCopy)
		}
	})
}

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}
