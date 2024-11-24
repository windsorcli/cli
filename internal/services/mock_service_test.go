package services

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/compose-spec/compose-go/types"
)

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
		mockService := &MockService{
			GetComposeConfigFunc: func() (*types.Config, error) {
				return expectedConfig, nil
			},
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
		mockService := &MockService{
			GetComposeConfigFunc: func() (*types.Config, error) {
				return nil, expectedError
			},
		}

		// When: GetComposeConfig is called
		_, err := mockService.GetComposeConfig()
		if err == nil {
			t.Fatalf("expected error %v, got nil", expectedError)
		}
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})
}

func TestMockService_SetGetComposeConfigFunc(t *testing.T) {
	t.Run("SetGetComposeConfigFunc", func(t *testing.T) {
		// Given: a mock service
		mockService := NewMockService()

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

		// When: SetGetComposeConfigFunc is called
		mockService.SetGetComposeConfigFunc(mockGetComposeConfigFunc)

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
