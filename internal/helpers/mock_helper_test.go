package helpers

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

func TestMockHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock helper with a GetComposeConfigFunc
		expectedConfig := &types.Config{
			Services: []types.ServiceConfig{
				{
					Name:  "service1",
					Image: "nginx:latest",
				},
			},
		}
		mockHelper := &MockHelper{
			GetComposeConfigFunc: func() (*types.Config, error) {
				return expectedConfig, nil
			},
		}

		// When: GetComposeConfig is called
		composeConfig, err := mockHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: the result should match the expected configuration
		if !reflect.DeepEqual(composeConfig, expectedConfig) {
			t.Errorf("expected %v, got %v", expectedConfig, composeConfig)
		}
	})

	t.Run("SuccessNoMock", func(t *testing.T) {
		// Given: a mock helper with no GetComposeConfigFunc
		mockHelper := NewMockHelper()

		// When: GetComposeConfig is called
		composeConfig, err := mockHelper.GetComposeConfig()

		// Then: no error should occur and the result should be nil
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		if composeConfig != nil {
			t.Errorf("expected nil, got %v", composeConfig)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given: a mock helper with a GetComposeConfigFunc that returns an error
		expectedError := fmt.Errorf("mock error getting compose config")
		mockHelper := &MockHelper{
			GetComposeConfigFunc: func() (*types.Config, error) {
				return nil, expectedError
			},
		}

		// When: GetComposeConfig is called
		_, err := mockHelper.GetComposeConfig()
		if err == nil {
			t.Fatalf("expected error %v, got nil", expectedError)
		}
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})
}

func TestMockHelper_SetGetComposeConfigFunc(t *testing.T) {
	t.Run("SetGetComposeConfigFunc", func(t *testing.T) {
		// Given: a mock helper
		mockHelper := NewMockHelper()

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
		mockHelper.SetGetComposeConfigFunc(mockGetComposeConfigFunc)

		// Then: the GetComposeConfigFunc should be set and return the expected configuration
		composeConfig, err := mockHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !reflect.DeepEqual(composeConfig, expectedConfig) {
			t.Errorf("expected %v, got %v", expectedConfig, composeConfig)
		}
	})
}
