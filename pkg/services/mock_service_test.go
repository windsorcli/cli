package services

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type MockComponents struct {
	Injector          di.Injector
	MockContext       *context.MockContext
	MockShell         *shell.MockShell
	MockConfigHandler *config.MockConfigHandler
	MockService       *MockService
}

// Helper function to compare two maps
func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestMockService_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock service with an InitializeFunc
		mockService := NewMockService()
		mockService.InitializeFunc = func() error {
			return nil
		}

		// When: Initialize is called
		err := mockService.Initialize()

		// Then: no error should occur
		if err != nil {
			t.Errorf("Initialize() error = %v", err)
		}
	})

	t.Run("SuccessNoMock", func(t *testing.T) {
		// Given: a mock service with no InitializeFunc
		mockService := NewMockService()

		// When: Initialize is called
		err := mockService.Initialize()

		// Then: no error should occur
		if err != nil {
			t.Errorf("Initialize() error = %v", err)
		}
	})
}

func TestMockService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock service with a GetComposeConfigFunc
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

		// Initialize the service
		err := mockService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := mockService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: the result should match the expected configuration
		if !reflect.DeepEqual(composeConfig, expectedConfig) {
			t.Errorf("expected %v, got %v", expectedConfig, composeConfig)
		}
	})

	t.Run("SuccessNoMock", func(t *testing.T) {
		// Given: a mock service with no GetComposeConfigFunc
		mockService := NewMockService()

		// Initialize the service
		err := mockService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := mockService.GetComposeConfig()

		// Then: no error should occur and the result should be nil
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		if composeConfig != nil {
			t.Errorf("expected nil, got %v", composeConfig)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given: a mock service with a GetComposeConfigFunc that returns an error
		expectedError := fmt.Errorf("mock error getting compose config")
		mockService := NewMockService()
		mockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return nil, expectedError
		}

		// Initialize the service
		err := mockService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err = mockService.GetComposeConfig()
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
		// Given: a mock service
		mockService := NewMockService()

		// Initialize the service
		err := mockService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Define a mock GetComposeConfigFunc
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

		// When: GetComposeConfigFunc is called
		mockService.GetComposeConfigFunc = mockGetComposeConfigFunc

		// Then: the GetComposeConfigFunc should be set and return the expected configuration
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
		// Given: a mock service with a WriteConfigFunc
		mockService := NewMockService()
		mockService.WriteConfigFunc = func() error {
			return nil
		}

		// When: WriteConfig is called
		err := mockService.WriteConfig()

		// Then: no error should occur
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
	})

	t.Run("SuccessNoMock", func(t *testing.T) {
		// Given: a mock service with no WriteConfigFunc
		mockService := NewMockService()

		// When: WriteConfig is called
		err := mockService.WriteConfig()

		// Then: no error should occur
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
	})
}

func TestMockService_SetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock service
		mockService := NewMockService()
		expectedAddress := "127.0.0.1"

		// When: SetAddressFunc is called
		mockSetAddressFunc := func(address string) error {
			if address != expectedAddress {
				t.Errorf("expected address %v, got %v", expectedAddress, address)
			}
			return nil
		}
		mockService.SetAddressFunc = mockSetAddressFunc

		// Then: the SetAddressFunc should be set and called with the expected address
		err := mockService.SetAddress(expectedAddress)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessNoMock", func(t *testing.T) {
		// Given: a mock service with no SetAddressFunc
		mockService := NewMockService()
		expectedAddress := "127.0.0.1"

		// When: SetAddress is called
		err := mockService.SetAddress(expectedAddress)

		// Then: no error should occur
		if err != nil {
			t.Fatalf("SetAddress() error = %v", err)
		}
	})
}

func TestMockService_GetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock service
		mockService := NewMockService()
		expectedAddress := "127.0.0.1"

		// When: GetAddressFunc is called
		mockGetAddressFunc := func() string {
			return expectedAddress
		}
		mockService.GetAddressFunc = mockGetAddressFunc

		// Then: the GetAddressFunc should be set and return the expected address
		address := mockService.GetAddress()
		if address != expectedAddress {
			t.Errorf("expected address %v, got %v", expectedAddress, address)
		}
	})

	t.Run("SuccessNoMock", func(t *testing.T) {
		// Given: a mock service with no GetAddressFunc
		mockService := NewMockService()

		// When: GetAddress is called
		address := mockService.GetAddress()

		// Then: an empty string should be returned
		if address != "" {
			t.Errorf("expected empty address, got %v", address)
		}
	})
}

func TestMockService_SetName(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock service
		mockService := NewMockService()
		expectedName := "TestService"

		// When: SetNameFunc is called
		mockSetNameFunc := func(name string) {
			mockService.name = name
		}
		mockService.SetNameFunc = mockSetNameFunc
		mockService.SetName(expectedName)

		// Then: the SetNameFunc should be set and the name should be updated
		if mockService.name != expectedName {
			t.Errorf("expected name %v, got %v", expectedName, mockService.name)
		}
	})

	t.Run("SuccessNoMock", func(t *testing.T) {
		// Given: a mock service with no SetNameFunc
		mockService := NewMockService()
		expectedName := "TestService"

		// When: SetName is called
		mockService.SetName(expectedName)

		// Then: the name should be updated
		if mockService.name != expectedName {
			t.Errorf("expected name %v, got %v", expectedName, mockService.name)
		}
	})
}

func TestMockService_GetName(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock service
		mockService := NewMockService()
		expectedName := "TestService"

		// When: GetNameFunc is called
		mockGetNameFunc := func() string {
			return expectedName
		}
		mockService.GetNameFunc = mockGetNameFunc
		name := mockService.GetName()

		// Then: the GetNameFunc should be set and the name should be returned
		if name != expectedName {
			t.Errorf("expected name %v, got %v", expectedName, name)
		}
	})

	t.Run("SuccessNoMock", func(t *testing.T) {
		// Given: a mock service with no GetNameFunc
		mockService := NewMockService()

		// When: GetName is called
		name := mockService.GetName()

		// Then: an empty string should be returned
		if name != "" {
			t.Errorf("expected empty name, got %v", name)
		}
	})
}
