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
		expectedHelper := func(_ ...any) (any, error) {
			return "result", nil
		}
		expectedSignature := new(func(string) any)
		mockEvaluator.RegisterFunc = func(name string, helper func(params []any, deferred bool) (any, error), signature any) {
			called = true
			if name != expectedName {
				t.Errorf("Expected name %s, got %s", expectedName, name)
			}
		}

		// When Register is called
		mockEvaluator.Register(expectedName, func(params []any, deferred bool) (any, error) {
			return expectedHelper(params...)
		}, expectedSignature)

		// Then the function should be called
		if !called {
			t.Error("Expected RegisterFunc to be called")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock evaluator without RegisterFunc set
		mockEvaluator := setupMockEvaluatorMocks(t)

		// When Register is called
		mockEvaluator.Register("test_helper", func(params []any, deferred bool) (any, error) {
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
		expectedFacetPath := "/test/facet.yaml"
		mockEvaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression != expectedExpression {
				t.Errorf("Expected expression %s, got %s", expectedExpression, expression)
			}
			if facetPath != expectedFacetPath {
				t.Errorf("Expected facetPath %s, got %s", expectedFacetPath, facetPath)
			}
			return expectedResult, nil
		}

		// When Evaluate is called
		result, err := mockEvaluator.Evaluate(expectedExpression, expectedFacetPath, nil, false)

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
		mockEvaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return nil, expectedError
		}

		// When Evaluate is called
		result, err := mockEvaluator.Evaluate("test", "", nil, false)

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
		result, err := mockEvaluator.Evaluate("test", "", nil, false)

		// Then no error should be returned and result should be nil (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
	})
}
