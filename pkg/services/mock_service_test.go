package services

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The MockServiceTest is a test suite for the MockService implementation
// It provides comprehensive test coverage for mock service behavior and function fields
// The MockServiceTest ensures proper mock service functionality across different scenarios
// enabling reliable service mocking and testing in the Windsor CLI

// =============================================================================
// Test Setup
// =============================================================================

type MockComponents struct {
	Injector          di.Injector
	MockShell         *shell.MockShell
	MockConfigHandler *config.MockConfigHandler
	MockService       *MockService
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockService_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock service with an InitializeFunc
		mockService := NewMockService()
		mockService.InitializeFunc = func() error {
			return nil
		}

		// When Initialize is called
		err := mockService.Initialize()

		// Then no error should occur
		if err != nil {
			t.Errorf("Initialize() error = %v", err)
		}
	})

	t.Run("SuccessNoMock", func(t *testing.T) {
		// Given a mock service with no InitializeFunc
		mockService := NewMockService()

		// When Initialize is called
		err := mockService.Initialize()

		// Then no error should occur
		if err != nil {
			t.Errorf("Initialize() error = %v", err)
		}
	})
}

func TestMockService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock service with a GetComposeConfigFunc
		expectedConfig := &types.Config{
			Services: []types.ServiceConfig{
				{
					Name:  "service1",
					Image: "nginx:latest",
				},
			},
		}
		mockService := NewMockService()
		mockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return expectedConfig, nil
		}

		// And the service is initialized
		err := mockService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When GetComposeConfig is called
		composeConfig, err := mockService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then the result should match the expected configuration
		if !reflect.DeepEqual(composeConfig, expectedConfig) {
			t.Errorf("expected %v, got %v", expectedConfig, composeConfig)
		}
	})

	t.Run("SuccessNoMock", func(t *testing.T) {
		// Given a mock service with no GetComposeConfigFunc
		mockService := NewMockService()

		// And the service is initialized
		err := mockService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When GetComposeConfig is called
		composeConfig, err := mockService.GetComposeConfig()

		// Then no error should occur and the result should be nil
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		if composeConfig != nil {
			t.Errorf("expected nil, got %v", composeConfig)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock service with a GetComposeConfigFunc that returns an error
		expectedError := fmt.Errorf("mock error getting compose config")
		mockService := NewMockService()
		mockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return nil, expectedError
		}

		// And the service is initialized
		err := mockService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When GetComposeConfig is called
		_, err = mockService.GetComposeConfig()

		// Then the expected error should be returned
		if err == nil {
			t.Fatalf("expected error %v, got nil", expectedError)
		}
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})
}

func TestMockService_GetComposeConfigFunc(t *testing.T) {
	t.Run("GetComposeConfigFunc", func(t *testing.T) {
		// Given a mock service
		mockService := NewMockService()

		// And the service is initialized
		err := mockService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// And a mock GetComposeConfigFunc is defined
		expectedConfig := &types.Config{
			Services: []types.ServiceConfig{
				{
					Name:  "service1",
					Image: "nginx:latest",
				},
			},
		}
		mockGetComposeConfigFunc := func() (*types.Config, error) {
			return expectedConfig, nil
		}

		// When GetComposeConfigFunc is set
		mockService.GetComposeConfigFunc = mockGetComposeConfigFunc

		// Then the GetComposeConfigFunc should return the expected configuration
		composeConfig, err := mockService.GetComposeConfig()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !reflect.DeepEqual(composeConfig, expectedConfig) {
			t.Errorf("expected %v, got %v", expectedConfig, composeConfig)
		}
	})
}

func TestMockService_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock service with a WriteConfigFunc
		mockService := NewMockService()
		mockService.WriteConfigFunc = func() error {
			return nil
		}

		// When WriteConfig is called
		err := mockService.WriteConfig()

		// Then no error should occur
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
	})

	t.Run("SuccessNoMock", func(t *testing.T) {
		// Given a mock service with no WriteConfigFunc
		mockService := NewMockService()

		// When WriteConfig is called
		err := mockService.WriteConfig()

		// Then no error should occur
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
	})
}

func TestMockService_SetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock service
		mockService := NewMockService()
		expectedAddress := "127.0.0.1"

		// And a mock SetAddressFunc is defined
		mockSetAddressFunc := func(address string) error {
			if address != expectedAddress {
				t.Errorf("expected address %v, got %v", expectedAddress, address)
			}
			return nil
		}
		mockService.SetAddressFunc = mockSetAddressFunc

		// When SetAddress is called
		err := mockService.SetAddress(expectedAddress)

		// Then no error should occur
		if err != nil {
			t.Fatalf("SetAddress() error = %v", err)
		}
	})
}

func TestMockService_GetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock service
		mockService := NewMockService()
		expectedAddress := "127.0.0.1"

		// And a mock GetAddressFunc is defined
		mockGetAddressFunc := func() string {
			return expectedAddress
		}
		mockService.GetAddressFunc = mockGetAddressFunc

		// When GetAddress is called
		address := mockService.GetAddress()

		// Then the expected address should be returned
		if address != expectedAddress {
			t.Errorf("expected address %v, got %v", expectedAddress, address)
		}
	})
}

func TestMockService_SetName(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock service
		mockService := NewMockService()
		expectedName := "test-service"

		// And a mock SetNameFunc is defined
		mockSetNameFunc := func(name string) {
			if name != expectedName {
				t.Errorf("expected name %v, got %v", expectedName, name)
			}
		}
		mockService.SetNameFunc = mockSetNameFunc

		// When SetName is called
		mockService.SetName(expectedName)
	})
}

func TestMockService_GetName(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock service
		mockService := NewMockService()
		expectedName := "test-service"

		// And a mock GetNameFunc is defined
		mockGetNameFunc := func() string {
			return expectedName
		}
		mockService.GetNameFunc = mockGetNameFunc

		// When GetName is called
		name := mockService.GetName()

		// Then the expected name should be returned
		if name != expectedName {
			t.Errorf("expected name %v, got %v", expectedName, name)
		}
	})
}

func TestMockService_GetHostname(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock service
		mockService := NewMockService()
		expectedHostname := "test-hostname"

		// And a mock GetHostnameFunc is defined
		mockGetHostnameFunc := func() string {
			return expectedHostname
		}
		mockService.GetHostnameFunc = mockGetHostnameFunc

		// When GetHostname is called
		hostname := mockService.GetHostname()

		// Then the expected hostname should be returned
		if hostname != expectedHostname {
			t.Errorf("expected hostname %v, got %v", expectedHostname, hostname)
		}
	})
}

func TestMockService_SupportsWildcard(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock service
		mockService := NewMockService()
		expectedSupportsWildcard := true

		// And a mock SupportsWildcardFunc is defined
		mockSupportsWildcardFunc := func() bool {
			return expectedSupportsWildcard
		}
		mockService.SupportsWildcardFunc = mockSupportsWildcardFunc

		// When SupportsWildcard is called
		supportsWildcard := mockService.SupportsWildcard()

		// Then the expected value should be returned
		if supportsWildcard != expectedSupportsWildcard {
			t.Errorf("expected supportsWildcard %v, got %v", expectedSupportsWildcard, supportsWildcard)
		}
	})
}
