package config

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
)

// The MockConfigHandlerTest is a test suite for the MockConfigHandler implementation.
// It provides comprehensive test coverage for mock config handler operations,
// ensuring reliable testing of config-dependent functionality.
// The MockConfigHandlerTest validates the mock implementation's behavior.

// =============================================================================
// Test Setup
// =============================================================================

// stringPtr returns a pointer to the given string value
func stringPtr(s string) *string {
	return &s
}

// setupMockConfigHandlerMocks creates a new set of mocks for testing MockConfigHandler
func setupMockConfigHandlerMocks(t *testing.T) *MockConfigHandler {
	t.Helper()

	// Create mock config handler
	mockConfigHandler := NewMockConfigHandler()

	return mockConfigHandler
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestMockConfigHandler_NewMockConfigHandler tests the constructor for MockConfigHandler
func TestMockConfigHandler_NewMockConfigHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// Then the mock config handler should be created successfully
		if mockConfigHandler == nil {
			t.Errorf("Expected mockConfigHandler, got nil")
		}
	})
}

// TestMockConfigHandler tests that MockConfigHandler implements ConfigHandler interface
func TestMockConfigHandler(t *testing.T) {
	t.Run("ImplementsInterface", func(t *testing.T) {
		// Given a mock config handler
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// Then it should implement ConfigHandler
		if mockConfigHandler == nil {
			t.Error("Expected mock config handler to be created")
		}
	})
}

// TestMockConfigHandler_LoadConfig tests the LoadConfig method of MockConfigHandler
func TestMockConfigHandler_LoadConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with LoadConfigFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		mockConfigHandler.LoadConfigFunc = func() error {
			return nil
		}

		// When calling LoadConfig
		err := mockConfigHandler.LoadConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock config handler with LoadConfigFunc set to return an error
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock load config error")
		mockConfigHandler.LoadConfigFunc = func() error {
			return expectedError
		}

		// When calling LoadConfig
		err := mockConfigHandler.LoadConfig()

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with LoadConfigFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling LoadConfig
		err := mockConfigHandler.LoadConfig()

		// Then no error should be returned (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestMockConfigHandler_LoadConfigString tests the LoadConfigString method of MockConfigHandler
func TestMockConfigHandler_LoadConfigString(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with LoadConfigStringFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		mockConfigHandler.LoadConfigStringFunc = func(content string) error {
			return nil
		}

		// When calling LoadConfigString
		err := mockConfigHandler.LoadConfigString("test content")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock config handler with LoadConfigStringFunc set to return an error
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock load config string error")
		mockConfigHandler.LoadConfigStringFunc = func(content string) error {
			return expectedError
		}

		// When calling LoadConfigString
		err := mockConfigHandler.LoadConfigString("test content")

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with LoadConfigStringFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling LoadConfigString
		err := mockConfigHandler.LoadConfigString("test content")

		// Then no error should be returned (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestMockConfigHandler_IsLoaded tests the IsLoaded method of MockConfigHandler
func TestMockConfigHandler_IsLoaded(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with IsLoadedFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := true
		mockConfigHandler.IsLoadedFunc = func() bool {
			return expectedResult
		}

		// When calling IsLoaded
		result := mockConfigHandler.IsLoaded()

		// Then the expected result should be returned
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with IsLoadedFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling IsLoaded
		result := mockConfigHandler.IsLoaded()

		// Then false should be returned (default implementation)
		if result != false {
			t.Errorf("Expected false, got %v", result)
		}
	})
}

// TestMockConfigHandler_GetString tests the GetString method of MockConfigHandler
func TestMockConfigHandler_GetString(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with GetStringFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := "mock-string-value"
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return expectedResult
		}

		// When calling GetString
		result := mockConfigHandler.GetString("test-key")

		// Then the expected result should be returned
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		// Given a mock config handler with GetStringFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := "mock-string-value"
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return expectedResult
		}

		// When calling GetString with default value
		result := mockConfigHandler.GetString("test-key", "default-value")

		// Then the expected result should be returned
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with GetStringFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetString
		result := mockConfigHandler.GetString("test-key")

		// Then the default mock string should be returned
		expectedResult := "mock-string"
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("NotImplementedWithDefault", func(t *testing.T) {
		// Given a mock config handler with GetStringFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetString with default value
		result := mockConfigHandler.GetString("test-key", "custom-default")

		// Then the custom default should be returned
		expectedResult := "custom-default"
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})
}

// TestMockConfigHandler_GetInt tests the GetInt method of MockConfigHandler
func TestMockConfigHandler_GetInt(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with GetIntFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := 123
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int {
			return expectedResult
		}

		// When calling GetInt
		result := mockConfigHandler.GetInt("test-key")

		// Then the expected result should be returned
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		// Given a mock config handler with GetIntFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := 456
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int {
			return expectedResult
		}

		// When calling GetInt with default value
		result := mockConfigHandler.GetInt("test-key", 999)

		// Then the expected result should be returned
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with GetIntFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetInt
		result := mockConfigHandler.GetInt("test-key")

		// Then the default mock int should be returned
		expectedResult := 42
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("NotImplementedWithDefault", func(t *testing.T) {
		// Given a mock config handler with GetIntFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetInt with default value
		result := mockConfigHandler.GetInt("test-key", 999)

		// Then the custom default should be returned
		expectedResult := 999
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})
}

// TestMockConfigHandler_GetBool tests the GetBool method of MockConfigHandler
func TestMockConfigHandler_GetBool(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with GetBoolFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := false
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return expectedResult
		}

		// When calling GetBool
		result := mockConfigHandler.GetBool("test-key")

		// Then the expected result should be returned
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		// Given a mock config handler with GetBoolFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := false
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return expectedResult
		}

		// When calling GetBool with default value
		result := mockConfigHandler.GetBool("test-key", true)

		// Then the expected result should be returned
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with GetBoolFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetBool
		result := mockConfigHandler.GetBool("test-key")

		// Then the default mock bool should be returned
		expectedResult := true
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("NotImplementedWithDefault", func(t *testing.T) {
		// Given a mock config handler with GetBoolFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetBool with default value
		result := mockConfigHandler.GetBool("test-key", false)

		// Then the custom default should be returned
		expectedResult := false
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})
}

// TestMockConfigHandler_GetStringSlice tests the GetStringSlice method of MockConfigHandler
func TestMockConfigHandler_GetStringSlice(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with GetStringSliceFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := []string{"item1", "item2"}
		mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			return expectedResult
		}

		// When calling GetStringSlice
		result := mockConfigHandler.GetStringSlice("test-key")

		// Then the expected result should be returned
		if len(result) != len(expectedResult) {
			t.Errorf("Expected result length %v, got %v", len(expectedResult), len(result))
		}
		if result[0] != expectedResult[0] || result[1] != expectedResult[1] {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		// Given a mock config handler with GetStringSliceFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := []string{"custom1", "custom2"}
		mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			return expectedResult
		}

		// When calling GetStringSlice with default value
		result := mockConfigHandler.GetStringSlice("test-key", []string{"default1", "default2"})

		// Then the expected result should be returned
		if len(result) != len(expectedResult) {
			t.Errorf("Expected result length %v, got %v", len(expectedResult), len(result))
		}
		if result[0] != expectedResult[0] || result[1] != expectedResult[1] {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with GetStringSliceFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetStringSlice
		result := mockConfigHandler.GetStringSlice("test-key")

		// Then an empty slice should be returned (default implementation)
		if len(result) != 0 {
			t.Errorf("Expected empty slice, got %v", result)
		}
	})

	t.Run("NotImplementedWithDefault", func(t *testing.T) {
		// Given a mock config handler with GetStringSliceFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetStringSlice with default value
		result := mockConfigHandler.GetStringSlice("test-key", []string{"default1", "default2"})

		// Then the custom default should be returned
		expectedResult := []string{"default1", "default2"}
		if len(result) != len(expectedResult) {
			t.Errorf("Expected result length %v, got %v", len(expectedResult), len(result))
		}
		if result[0] != expectedResult[0] || result[1] != expectedResult[1] {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})
}

// TestMockConfigHandler_GetStringMap tests the GetStringMap method of MockConfigHandler
func TestMockConfigHandler_GetStringMap(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with GetStringMapFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := map[string]string{"key1": "value1", "key2": "value2"}
		mockConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			return expectedResult
		}

		// When calling GetStringMap
		result := mockConfigHandler.GetStringMap("test-key")

		// Then the expected result should be returned
		if len(result) != len(expectedResult) {
			t.Errorf("Expected result length %v, got %v", len(expectedResult), len(result))
		}
		if result["key1"] != expectedResult["key1"] || result["key2"] != expectedResult["key2"] {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		// Given a mock config handler with GetStringMapFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := map[string]string{"custom1": "custom2"}
		mockConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			return expectedResult
		}

		// When calling GetStringMap with default value
		result := mockConfigHandler.GetStringMap("test-key", map[string]string{"default1": "default2"})

		// Then the expected result should be returned
		if len(result) != len(expectedResult) {
			t.Errorf("Expected result length %v, got %v", len(expectedResult), len(result))
		}
		if result["custom1"] != expectedResult["custom1"] {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with GetStringMapFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetStringMap
		result := mockConfigHandler.GetStringMap("test-key")

		// Then an empty map should be returned (default implementation)
		if len(result) != 0 {
			t.Errorf("Expected empty map, got %v", result)
		}
	})

	t.Run("NotImplementedWithDefault", func(t *testing.T) {
		// Given a mock config handler with GetStringMapFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetStringMap with default value
		result := mockConfigHandler.GetStringMap("test-key", map[string]string{"default1": "default2"})

		// Then the custom default should be returned
		expectedResult := map[string]string{"default1": "default2"}
		if len(result) != len(expectedResult) {
			t.Errorf("Expected result length %v, got %v", len(expectedResult), len(result))
		}
		if result["default1"] != expectedResult["default1"] {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})
}

// TestMockConfigHandler_Set tests the Set method of MockConfigHandler
func TestMockConfigHandler_Set(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with SetFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		mockConfigHandler.SetFunc = func(key string, value any) error {
			return nil
		}

		// When calling Set
		err := mockConfigHandler.Set("test-key", "test-value")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock config handler with SetFunc set to return an error
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock set error")
		mockConfigHandler.SetFunc = func(key string, value any) error {
			return expectedError
		}

		// When calling Set
		err := mockConfigHandler.Set("test-key", "test-value")

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with SetFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling Set
		err := mockConfigHandler.Set("test-key", "test-value")

		// Then no error should be returned (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestMockConfigHandler_Get tests the Get method of MockConfigHandler
func TestMockConfigHandler_Get(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with GetFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := "mock-get-value"
		mockConfigHandler.GetFunc = func(key string) any {
			return expectedResult
		}

		// When calling Get
		result := mockConfigHandler.Get("test-key")

		// Then the expected result should be returned
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with GetFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling Get
		result := mockConfigHandler.Get("test-key")

		// Then the default mock value should be returned
		expectedResult := "mock-value"
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})
}

// TestMockConfigHandler_SaveConfig tests the SaveConfig method of MockConfigHandler
func TestMockConfigHandler_SaveConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with SaveConfigFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		mockConfigHandler.SaveConfigFunc = func(overwrite ...bool) error {
			return nil
		}

		// When calling SaveConfig
		err := mockConfigHandler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("WithOverwrite", func(t *testing.T) {
		// Given a mock config handler with SaveConfigFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		mockConfigHandler.SaveConfigFunc = func(overwrite ...bool) error {
			return nil
		}

		// When calling SaveConfig with overwrite
		err := mockConfigHandler.SaveConfig(true)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock config handler with SaveConfigFunc set to return an error
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock save config error")
		mockConfigHandler.SaveConfigFunc = func(overwrite ...bool) error {
			return expectedError
		}

		// When calling SaveConfig
		err := mockConfigHandler.SaveConfig()

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with SaveConfigFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling SaveConfig
		err := mockConfigHandler.SaveConfig()

		// Then no error should be returned (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestMockConfigHandler_SetDefault tests the SetDefault method of MockConfigHandler
func TestMockConfigHandler_SetDefault(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with SetDefaultFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		mockConfigHandler.SetDefaultFunc = func(context v1alpha1.Context) error {
			return nil
		}

		// When calling SetDefault
		err := mockConfigHandler.SetDefault(v1alpha1.Context{})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock config handler with SetDefaultFunc set to return an error
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock set default error")
		mockConfigHandler.SetDefaultFunc = func(context v1alpha1.Context) error {
			return expectedError
		}

		// When calling SetDefault
		err := mockConfigHandler.SetDefault(v1alpha1.Context{})

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with SetDefaultFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling SetDefault
		err := mockConfigHandler.SetDefault(v1alpha1.Context{})

		// Then no error should be returned (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestMockConfigHandler_GetConfig tests the GetConfig method of MockConfigHandler
func TestMockConfigHandler_GetConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with GetConfigFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := &v1alpha1.Context{ID: stringPtr("test-context")}
		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return expectedResult
		}

		// When calling GetConfig
		result := mockConfigHandler.GetConfig()

		// Then the expected result should be returned
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with GetConfigFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetConfig
		result := mockConfigHandler.GetConfig()

		// Then an empty context should be returned (default implementation)
		if result == nil {
			t.Error("Expected non-nil result, got nil")
		}
		if result.ID != nil {
			t.Errorf("Expected empty context, got %v", result)
		}
	})
}

// TestMockConfigHandler_GetContext tests the GetContext method of MockConfigHandler
func TestMockConfigHandler_GetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with GetContextFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := "test-context"
		mockConfigHandler.GetContextFunc = func() string {
			return expectedResult
		}

		// When calling GetContext
		result := mockConfigHandler.GetContext()

		// Then the expected result should be returned
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with GetContextFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetContext
		result := mockConfigHandler.GetContext()

		// Then the default mock context should be returned
		expectedResult := "mock-context"
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})
}

// TestMockConfigHandler_SetContext tests the SetContext method of MockConfigHandler
func TestMockConfigHandler_SetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with SetContextFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		mockConfigHandler.SetContextFunc = func(context string) error {
			return nil
		}

		// When calling SetContext
		err := mockConfigHandler.SetContext("test-context")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock config handler with SetContextFunc set to return an error
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock set context error")
		mockConfigHandler.SetContextFunc = func(context string) error {
			return expectedError
		}

		// When calling SetContext
		err := mockConfigHandler.SetContext("test-context")

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with SetContextFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling SetContext
		err := mockConfigHandler.SetContext("test-context")

		// Then no error should be returned (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestMockConfigHandler_GetConfigRoot tests the GetConfigRoot method of MockConfigHandler
func TestMockConfigHandler_GetConfigRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with GetConfigRootFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := "/mock/config/root"
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return expectedResult, nil
		}

		// When calling GetConfigRoot
		result, err := mockConfigHandler.GetConfigRoot()

		// Then no error should be returned and the result should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock config handler with GetConfigRootFunc set to return an error
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock get config root error")
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", expectedError
		}

		// When calling GetConfigRoot
		result, err := mockConfigHandler.GetConfigRoot()

		// Then the expected error should be returned and result should be empty
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if result != "" {
			t.Errorf("Expected empty result, got %v", result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with GetConfigRootFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetConfigRoot
		result, err := mockConfigHandler.GetConfigRoot()

		// Then no error should be returned and result should be default (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expectedResult := "mock-config-root"
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})
}

// TestMockConfigHandler_GetWindsorScratchPath tests the GetWindsorScratchPath method of MockConfigHandler
func TestMockConfigHandler_GetWindsorScratchPath(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := "/mock/windsor/scratch/path"
		mockConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return expectedResult, nil
		}

		result, err := mockConfigHandler.GetWindsorScratchPath()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("Error", func(t *testing.T) {
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock get windsor scratch path error")
		mockConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", expectedError
		}

		result, err := mockConfigHandler.GetWindsorScratchPath()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if result != "" {
			t.Errorf("Expected empty result, got %v", result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		result, err := mockConfigHandler.GetWindsorScratchPath()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expectedResult := "mock-windsor-scratch-path"
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})
}

// TestMockConfigHandler_Clean tests the Clean method of MockConfigHandler
func TestMockConfigHandler_Clean(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with CleanFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		mockConfigHandler.CleanFunc = func() error {
			return nil
		}

		// When calling Clean
		err := mockConfigHandler.Clean()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock config handler with CleanFunc set to return an error
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock clean error")
		mockConfigHandler.CleanFunc = func() error {
			return expectedError
		}

		// When calling Clean
		err := mockConfigHandler.Clean()

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with CleanFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling Clean
		err := mockConfigHandler.Clean()

		// Then no error should be returned (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestMockConfigHandler_GenerateContextID tests the GenerateContextID method of MockConfigHandler
func TestMockConfigHandler_GenerateContextID(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with GenerateContextIDFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		mockConfigHandler.GenerateContextIDFunc = func() error {
			return nil
		}

		// When calling GenerateContextID
		err := mockConfigHandler.GenerateContextID()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock config handler with GenerateContextIDFunc set to return an error
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock generate context id error")
		mockConfigHandler.GenerateContextIDFunc = func() error {
			return expectedError
		}

		// When calling GenerateContextID
		err := mockConfigHandler.GenerateContextID()

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with GenerateContextIDFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GenerateContextID
		err := mockConfigHandler.GenerateContextID()

		// Then no error should be returned (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestMockConfigHandler_LoadSchema tests the LoadSchema method of MockConfigHandler
func TestMockConfigHandler_LoadSchema(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with LoadSchemaFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		mockConfigHandler.LoadSchemaFunc = func(schemaPath string) error {
			return nil
		}

		// When calling LoadSchema
		err := mockConfigHandler.LoadSchema("/mock/schema.yaml")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock config handler with LoadSchemaFunc set to return an error
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock load schema error")
		mockConfigHandler.LoadSchemaFunc = func(schemaPath string) error {
			return expectedError
		}

		// When calling LoadSchema
		err := mockConfigHandler.LoadSchema("/mock/schema.yaml")

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with LoadSchemaFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling LoadSchema
		err := mockConfigHandler.LoadSchema("/mock/schema.yaml")

		// Then an error should be returned (default implementation)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "LoadSchemaFunc not set"
		if err.Error() != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})
}

// TestMockConfigHandler_LoadSchemaFromBytes tests the LoadSchemaFromBytes method of MockConfigHandler
func TestMockConfigHandler_LoadSchemaFromBytes(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with LoadSchemaFromBytesFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		mockConfigHandler.LoadSchemaFromBytesFunc = func(schemaContent []byte) error {
			return nil
		}

		// When calling LoadSchemaFromBytes
		err := mockConfigHandler.LoadSchemaFromBytes([]byte("schema content"))

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock config handler with LoadSchemaFromBytesFunc set to return an error
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock load schema from bytes error")
		mockConfigHandler.LoadSchemaFromBytesFunc = func(schemaContent []byte) error {
			return expectedError
		}

		// When calling LoadSchemaFromBytes
		err := mockConfigHandler.LoadSchemaFromBytes([]byte("schema content"))

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with LoadSchemaFromBytesFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling LoadSchemaFromBytes
		err := mockConfigHandler.LoadSchemaFromBytes([]byte("schema content"))

		// Then an error should be returned (default implementation)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "LoadSchemaFromBytesFunc not set"
		if err.Error() != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})
}

// TestMockConfigHandler_GetContextValues tests the GetContextValues method of MockConfigHandler
func TestMockConfigHandler_GetContextValues(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler with GetContextValuesFunc set
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedResult := map[string]any{"key1": "value1", "key2": 42}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return expectedResult, nil
		}

		// When calling GetContextValues
		result, err := mockConfigHandler.GetContextValues()

		// Then no error should be returned and the result should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(result) != len(expectedResult) {
			t.Errorf("Expected result length %v, got %v", len(expectedResult), len(result))
		}
		if result["key1"] != expectedResult["key1"] || result["key2"] != expectedResult["key2"] {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock config handler with GetContextValuesFunc set to return an error
		mockConfigHandler := setupMockConfigHandlerMocks(t)
		expectedError := fmt.Errorf("mock get context values error")
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return nil, expectedError
		}

		// When calling GetContextValues
		result, err := mockConfigHandler.GetContextValues()

		// Then the expected error should be returned and result should be nil
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock config handler with GetContextValuesFunc not set
		mockConfigHandler := setupMockConfigHandlerMocks(t)

		// When calling GetContextValues
		result, err := mockConfigHandler.GetContextValues()

		// Then an error should be returned and result should be nil (default implementation)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "GetContextValuesFunc not set"
		if err.Error() != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
	})
}
