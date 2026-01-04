package evaluator

import (
	"errors"
	"testing"
)

// The MockExpressionEvaluatorTest is a test suite for the MockExpressionEvaluator implementation.
// It provides comprehensive test coverage for mock evaluator operations,
// ensuring reliable testing of evaluator-dependent functionality.
// The MockExpressionEvaluatorTest validates the mock implementation's behavior.

// =============================================================================
// Test Setup
// =============================================================================

// setupMockEvaluatorMocks creates a new set of mocks for testing MockExpressionEvaluator
func setupMockEvaluatorMocks(t *testing.T) *MockExpressionEvaluator {
	t.Helper()

	mockEvaluator := NewMockExpressionEvaluator()

	return mockEvaluator
}

// =============================================================================
// Test Constructor
// =============================================================================

// TestMockExpressionEvaluator_NewMockExpressionEvaluator tests the constructor for MockExpressionEvaluator
func TestMockExpressionEvaluator_NewMockExpressionEvaluator(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockEvaluator := setupMockEvaluatorMocks(t)

		// Then the mock evaluator should be created successfully
		if mockEvaluator == nil {
			t.Errorf("Expected mockEvaluator, got nil")
		}
	})
}

// TestMockExpressionEvaluator tests that MockExpressionEvaluator implements ExpressionEvaluator interface
func TestMockExpressionEvaluator(t *testing.T) {
	t.Run("ImplementsInterface", func(t *testing.T) {
		// Given a mock evaluator
		mockEvaluator := setupMockEvaluatorMocks(t)

		// Then it should implement ExpressionEvaluator
		if mockEvaluator == nil {
			t.Error("Expected mock evaluator to be created")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestMockExpressionEvaluator_SetTemplateData tests the SetTemplateData method of MockExpressionEvaluator
func TestMockExpressionEvaluator_SetTemplateData(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock evaluator with SetTemplateDataFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)
		called := false
		expectedData := map[string][]byte{
			"test.jsonnet": []byte(`{"key": "value"}`),
		}
		mockEvaluator.SetTemplateDataFunc = func(templateData map[string][]byte) {
			called = true
			if len(templateData) != len(expectedData) {
				t.Errorf("Expected templateData length %d, got %d", len(expectedData), len(templateData))
			}
		}

		// When SetTemplateData is called
		mockEvaluator.SetTemplateData(expectedData)

		// Then the function should be called
		if !called {
			t.Error("Expected SetTemplateDataFunc to be called")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock evaluator without SetTemplateDataFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)

		// When SetTemplateData is called
		mockEvaluator.SetTemplateData(map[string][]byte{
			"test.jsonnet": []byte(`{"key": "value"}`),
		})

		// Then no error should occur (does nothing)
		if mockEvaluator == nil {
			t.Error("Expected mock evaluator to exist")
		}
	})
}

// TestMockExpressionEvaluator_Register tests the Register method of MockExpressionEvaluator
func TestMockExpressionEvaluator_Register(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock evaluator with RegisterFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)
		called := false
		expectedName := "test_helper"
		expectedHelper := func(params ...any) (any, error) {
			return "result", nil
		}
		expectedSignature := new(func(string) any)
		mockEvaluator.RegisterFunc = func(name string, helper func(params ...any) (any, error), signature any) {
			called = true
			if name != expectedName {
				t.Errorf("Expected name %s, got %s", expectedName, name)
			}
		}

		// When Register is called
		mockEvaluator.Register(expectedName, expectedHelper, expectedSignature)

		// Then the function should be called
		if !called {
			t.Error("Expected RegisterFunc to be called")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock evaluator without RegisterFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)

		// When Register is called
		mockEvaluator.Register("test_helper", func(params ...any) (any, error) {
			return nil, nil
		}, new(func(string) any))

		// Then no error should occur (does nothing)
		if mockEvaluator == nil {
			t.Error("Expected mock evaluator to exist")
		}
	})
}

// TestMockExpressionEvaluator_Evaluate tests the Evaluate method of MockExpressionEvaluator
func TestMockExpressionEvaluator_Evaluate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock evaluator with EvaluateFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)
		expectedResult := 42
		expectedExpression := "value"
		expectedConfig := map[string]any{
			"value": 42,
		}
		expectedFeaturePath := "/test/feature.yaml"
		mockEvaluator.EvaluateFunc = func(expression string, config map[string]any, featurePath string) (any, error) {
			if expression != expectedExpression {
				t.Errorf("Expected expression %s, got %s", expectedExpression, expression)
			}
			if featurePath != expectedFeaturePath {
				t.Errorf("Expected featurePath %s, got %s", expectedFeaturePath, featurePath)
			}
			return expectedResult, nil
		}

		// When Evaluate is called
		result, err := mockEvaluator.Evaluate(expectedExpression, expectedConfig, expectedFeaturePath)

		// Then no error should be returned and the result should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock evaluator with EvaluateFunc set to return an error
		mockEvaluator := setupMockEvaluatorMocks(t)
		expectedError := errors.New("evaluation error")
		mockEvaluator.EvaluateFunc = func(expression string, config map[string]any, featurePath string) (any, error) {
			return nil, expectedError
		}

		// When Evaluate is called
		result, err := mockEvaluator.Evaluate("test", map[string]any{}, "")

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
		// Given a mock evaluator without EvaluateFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)

		// When Evaluate is called
		result, err := mockEvaluator.Evaluate("test", map[string]any{}, "")

		// Then no error should be returned and result should be nil (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
	})
}

// TestMockExpressionEvaluator_EvaluateDefaults tests the EvaluateDefaults method of MockExpressionEvaluator
func TestMockExpressionEvaluator_EvaluateDefaults(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock evaluator with EvaluateDefaultsFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)
		expectedResult := map[string]any{
			"key1": "value1",
			"key2": 42,
		}
		expectedDefaults := map[string]any{
			"key1": "${value1}",
			"key2": "${value2}",
		}
		expectedConfig := map[string]any{
			"value1": "value1",
			"value2": 42,
		}
		expectedFeaturePath := "/test/feature.yaml"
		mockEvaluator.EvaluateDefaultsFunc = func(defaults map[string]any, config map[string]any, featurePath string) (map[string]any, error) {
			if featurePath != expectedFeaturePath {
				t.Errorf("Expected featurePath %s, got %s", expectedFeaturePath, featurePath)
			}
			return expectedResult, nil
		}

		// When EvaluateDefaults is called
		result, err := mockEvaluator.EvaluateDefaults(expectedDefaults, expectedConfig, expectedFeaturePath)

		// Then no error should be returned and the result should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(result) != len(expectedResult) {
			t.Errorf("Expected result length %d, got %d", len(expectedResult), len(result))
		}
		if result["key1"] != expectedResult["key1"] {
			t.Errorf("Expected result[key1] %v, got %v", expectedResult["key1"], result["key1"])
		}
		if result["key2"] != expectedResult["key2"] {
			t.Errorf("Expected result[key2] %v, got %v", expectedResult["key2"], result["key2"])
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock evaluator with EvaluateDefaultsFunc set to return an error
		mockEvaluator := setupMockEvaluatorMocks(t)
		expectedError := errors.New("evaluation defaults error")
		mockEvaluator.EvaluateDefaultsFunc = func(defaults map[string]any, config map[string]any, featurePath string) (map[string]any, error) {
			return nil, expectedError
		}

		// When EvaluateDefaults is called
		result, err := mockEvaluator.EvaluateDefaults(map[string]any{}, map[string]any{}, "")

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
		// Given a mock evaluator without EvaluateDefaultsFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)

		// When EvaluateDefaults is called
		result, err := mockEvaluator.EvaluateDefaults(map[string]any{}, map[string]any{}, "")

		// Then no error should be returned and result should be an empty map (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result == nil {
			t.Error("Expected non-nil empty map, got nil")
		}
		if len(result) != 0 {
			t.Errorf("Expected empty map, got map with %d entries", len(result))
		}
	})
}

// TestMockExpressionEvaluator_InterpolateString tests the InterpolateString method of MockExpressionEvaluator
func TestMockExpressionEvaluator_InterpolateString(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock evaluator with InterpolateStringFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)
		expectedResult := "interpolated value: 42"
		expectedString := "interpolated value: ${value}"
		expectedConfig := map[string]any{
			"value": 42,
		}
		expectedFeaturePath := "/test/feature.yaml"
		mockEvaluator.InterpolateStringFunc = func(s string, config map[string]any, featurePath string) (string, error) {
			if s != expectedString {
				t.Errorf("Expected string %s, got %s", expectedString, s)
			}
			if featurePath != expectedFeaturePath {
				t.Errorf("Expected featurePath %s, got %s", expectedFeaturePath, featurePath)
			}
			return expectedResult, nil
		}

		// When InterpolateString is called
		result, err := mockEvaluator.InterpolateString(expectedString, expectedConfig, expectedFeaturePath)

		// Then no error should be returned and the result should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != expectedResult {
			t.Errorf("Expected result %s, got %s", expectedResult, result)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock evaluator with InterpolateStringFunc set to return an error
		mockEvaluator := setupMockEvaluatorMocks(t)
		expectedError := errors.New("interpolation error")
		mockEvaluator.InterpolateStringFunc = func(s string, config map[string]any, featurePath string) (string, error) {
			return "", expectedError
		}

		// When InterpolateString is called
		result, err := mockEvaluator.InterpolateString("test", map[string]any{}, "")

		// Then the expected error should be returned and result should be empty
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if result != "" {
			t.Errorf("Expected empty result, got %s", result)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock evaluator without InterpolateStringFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)
		expectedString := "test string"

		// When InterpolateString is called
		result, err := mockEvaluator.InterpolateString(expectedString, map[string]any{}, "")

		// Then no error should be returned and result should be the input string (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != expectedString {
			t.Errorf("Expected result %s, got %s", expectedString, result)
		}
	})
}

// TestMockExpressionEvaluator_EvaluateValue tests the EvaluateValue method of MockExpressionEvaluator
func TestMockExpressionEvaluator_EvaluateValue(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock evaluator with EvaluateValueFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)
		expectedResult := 42
		expectedString := "${value}"
		expectedConfig := map[string]any{
			"value": 42,
		}
		expectedFeaturePath := "/test/feature.yaml"
		mockEvaluator.EvaluateValueFunc = func(s string, config map[string]any, featurePath string) (any, error) {
			if s != expectedString {
				t.Errorf("Expected string %s, got %s", expectedString, s)
			}
			if featurePath != expectedFeaturePath {
				t.Errorf("Expected featurePath %s, got %s", expectedFeaturePath, featurePath)
			}
			return expectedResult, nil
		}

		// When EvaluateValue is called
		result, err := mockEvaluator.EvaluateValue(expectedString, expectedConfig, expectedFeaturePath)

		// Then no error should be returned and the result should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock evaluator with EvaluateValueFunc set to return an error
		mockEvaluator := setupMockEvaluatorMocks(t)
		expectedError := errors.New("evaluation value error")
		mockEvaluator.EvaluateValueFunc = func(s string, config map[string]any, featurePath string) (any, error) {
			return nil, expectedError
		}

		// When EvaluateValue is called
		result, err := mockEvaluator.EvaluateValue("test", map[string]any{}, "")

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
		// Given a mock evaluator without EvaluateValueFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)
		expectedString := "test string"

		// When EvaluateValue is called
		result, err := mockEvaluator.EvaluateValue(expectedString, map[string]any{}, "")

		// Then no error should be returned and result should be the input string (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != expectedString {
			t.Errorf("Expected result %s, got %v", expectedString, result)
		}
	})
}

