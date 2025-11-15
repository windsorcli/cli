package services

import (
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

// TestLocalstackService_GetComposeConfig tests the GetComposeConfig method
func TestLocalstackService_GetComposeConfig(t *testing.T) {
	setup := func(t *testing.T) (*LocalstackService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupServicesMocks(t)
		service := NewLocalstackService(mocks.Runtime)
		service.shims = mocks.Shims
		service.SetName("aws")

		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Create mock injector with necessary mocks
		service, mocks := setup(t)

		// Mock configuration for Localstack
		err := mocks.ConfigHandler.Set("aws.localstack.enabled", true)
		if err != nil {
			t.Fatalf("failed to set localstack enabled: %v", err)
		}
		err = mocks.ConfigHandler.Set("aws.localstack.services", []string{"s3", "dynamodb"})
		if err != nil {
			t.Fatalf("failed to set localstack services: %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := service.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: the compose configuration should include the Localstack service
		if composeConfig == nil || len(composeConfig.Services) == 0 {
			t.Fatalf("expected non-nil composeConfig with services, got %v", composeConfig)
		}

		composeService, exists := composeConfig.Services["aws"]
		if !exists {
			t.Fatalf("expected service 'aws' to exist in compose config")
		}
		if composeService.Name != "aws" {
			t.Errorf("expected service name 'aws', got %v", composeService.Name)
		}
		if composeService.Environment["SERVICES"] == nil || *composeService.Environment["SERVICES"] != "s3,dynamodb" {
			t.Errorf("expected SERVICES environment variable to be 's3,dynamodb', got %v", composeService.Environment["SERVICES"])
		}
	})

	t.Run("LocalstackWithAuthToken", func(t *testing.T) {
		// Set the LOCALSTACK_AUTH_TOKEN environment variable
		t.Setenv("LOCALSTACK_AUTH_TOKEN", "mock_token")

		// Create mock injector with necessary mocks
		service, mocks := setup(t)

		// Mock configuration for Localstack
		err := mocks.ConfigHandler.Set("aws.localstack.enabled", true)
		if err != nil {
			t.Fatalf("failed to set localstack enabled: %v", err)
		}
		err = mocks.ConfigHandler.Set("aws.localstack.services", []string{"s3", "dynamodb"})
		if err != nil {
			t.Fatalf("failed to set localstack services: %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := service.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: the compose configuration should include the Localstack service with auth token
		if composeConfig == nil || len(composeConfig.Services) == 0 {
			t.Fatalf("expected non-nil composeConfig with services, got %v", composeConfig)
		}

		composeService, exists := composeConfig.Services["aws"]
		if !exists {
			t.Fatalf("expected service 'aws' to exist in compose config")
		}
		if len(composeService.Secrets) == 0 || composeService.Secrets[0].Source != "LOCALSTACK_AUTH_TOKEN" {
			t.Errorf("expected service to have LOCALSTACK_AUTH_TOKEN secret, got %v", composeService.Secrets)
		}
	})
}

// TestLocalstackService_SupportsWildcard tests the SupportsWildcard method
func TestLocalstackService_SupportsWildcard(t *testing.T) {
	setup := func(t *testing.T) (*LocalstackService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupServicesMocks(t)
		service := NewLocalstackService(mocks.Runtime)
		service.shims = mocks.Shims
		service.SetName("aws")

		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a LocalstackService with mock components
		service, _ := setup(t)

		// When SupportsWildcard is called
		supportsWildcard := service.SupportsWildcard()

		// Then SupportsWildcard should return true
		if !supportsWildcard {
			t.Errorf("Expected SupportsWildcard to return true, got false")
		}
	})
}
