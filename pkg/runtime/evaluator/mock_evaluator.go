package evaluator

// MockExpressionEvaluator is a mock implementation of the ExpressionEvaluator interface for testing purposes.
// It provides a way to simulate expression evaluation without executing actual evaluation logic.
// It allows for controlled testing of evaluator-dependent functionality by providing mock implementations
// of all ExpressionEvaluator interface methods.
type MockExpressionEvaluator struct {
	SetTemplateDataFunc func(templateData map[string][]byte)
	RegisterFunc        func(name string, helper func(params []any, deferred bool) (any, error), signature any)
	EvaluateFunc        func(expression string, featurePath string, evaluateDeferred bool) (any, error)
	EvaluateMapFunc     func(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockExpressionEvaluator creates a new instance of MockExpressionEvaluator.
func NewMockExpressionEvaluator() *MockExpressionEvaluator {
	return &MockExpressionEvaluator{}
}

// =============================================================================
// Public Methods
// =============================================================================

// SetTemplateData calls the mock SetTemplateDataFunc if set, otherwise does nothing.
func (m *MockExpressionEvaluator) SetTemplateData(templateData map[string][]byte) {
	if m.SetTemplateDataFunc != nil {
		m.SetTemplateDataFunc(templateData)
	}
}

// Register calls the mock RegisterFunc if set, otherwise does nothing.
func (m *MockExpressionEvaluator) Register(name string, helper func(params []any, deferred bool) (any, error), signature any) {
	if m.RegisterFunc != nil {
		m.RegisterFunc(name, helper, signature)
	}
}

// Evaluate calls the mock EvaluateFunc if set, otherwise returns nil, nil.
func (m *MockExpressionEvaluator) Evaluate(expression string, featurePath string, evaluateDeferred bool) (any, error) {
	if m.EvaluateFunc != nil {
		return m.EvaluateFunc(expression, featurePath, evaluateDeferred)
	}
	return nil, nil
}

// EvaluateMap calls the mock EvaluateMapFunc if set, otherwise returns an empty map and nil error.
func (m *MockExpressionEvaluator) EvaluateMap(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error) {
	if m.EvaluateMapFunc != nil {
		return m.EvaluateMapFunc(values, featurePath, evaluateDeferred)
	}
	return map[string]any{}, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockExpressionEvaluator implements ExpressionEvaluator.
var _ ExpressionEvaluator = (*MockExpressionEvaluator)(nil)
