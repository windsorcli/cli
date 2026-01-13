package services

import (
	"testing"

	"github.com/windsorcli/cli/pkg/constants"
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

func TestLocalstackService_GetIncusConfig(t *testing.T) {
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

		// When GetIncusConfig is called
		incusConfig, err := service.GetIncusConfig()
		if err != nil {
			t.Fatalf("GetIncusConfig() error = %v", err)
		}

		// Then the Incus configuration should be correctly populated
		if incusConfig == nil {
			t.Fatalf("expected non-nil incusConfig, got %v", incusConfig)
		}
		if incusConfig.Type != "container" {
			t.Errorf("expected Type to be 'container', got %q", incusConfig.Type)
		}
		if incusConfig.Image != "docker:"+constants.DefaultAWSLocalstackImage {
			t.Errorf("expected Image to be 'docker:%s', got %q", constants.DefaultAWSLocalstackImage, incusConfig.Image)
		}
		if incusConfig.Config["environment.ENFORCE_IAM"] != "1" {
			t.Errorf("expected ENFORCE_IAM to be '1', got %q", incusConfig.Config["environment.ENFORCE_IAM"])
		}
		if incusConfig.Config["environment.SERVICES"] != "s3,dynamodb" {
			t.Errorf("expected SERVICES to be 's3,dynamodb', got %q", incusConfig.Config["environment.SERVICES"])
		}
	})

	t.Run("WithAuthToken", func(t *testing.T) {
		// Given a LocalstackService with auth token environment variable
		t.Setenv("LOCALSTACK_AUTH_TOKEN", "mock_token")
		service, mocks := setup(t)

		// Mock configuration for Localstack
		err := mocks.ConfigHandler.Set("aws.localstack.enabled", true)
		if err != nil {
			t.Fatalf("failed to set localstack enabled: %v", err)
		}

		// When GetIncusConfig is called
		incusConfig, err := service.GetIncusConfig()
		if err != nil {
			t.Fatalf("GetIncusConfig() error = %v", err)
		}

		// Then the Incus configuration should use pro image and include auth token
		if incusConfig.Image != "docker:"+constants.DefaultAWSLocalstackProImage {
			t.Errorf("expected Image to be 'docker:%s', got %q", constants.DefaultAWSLocalstackProImage, incusConfig.Image)
		}
		if incusConfig.Config["environment.LOCALSTACK_AUTH_TOKEN"] != "mock_token" {
			t.Errorf("expected LOCALSTACK_AUTH_TOKEN to be 'mock_token', got %q", incusConfig.Config["environment.LOCALSTACK_AUTH_TOKEN"])
		}
	})
}
