package evaluator

// MockExpressionEvaluator is a mock implementation of the ExpressionEvaluator interface for testing purposes.
// It provides a way to simulate expression evaluation without executing actual evaluation logic.
// It allows for controlled testing of evaluator-dependent functionality by providing mock implementations
// of all ExpressionEvaluator interface methods.
type MockExpressionEvaluator struct {
	SetTemplateDataFunc   func(templateData map[string][]byte)
	RegisterFunc          func(name string, helper func(params ...any) (any, error), signature any)
	EvaluateFunc          func(expression string, config map[string]any, featurePath string) (any, error)
	EvaluateDefaultsFunc  func(defaults map[string]any, config map[string]any, featurePath string) (map[string]any, error)
	EvaluateValueFunc     func(s string, config map[string]any, featurePath string) (any, error)
	InterpolateStringFunc func(s string, config map[string]any, featurePath string) (string, error)
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
func (m *MockExpressionEvaluator) Register(name string, helper func(params ...any) (any, error), signature any) {
	if m.RegisterFunc != nil {
		m.RegisterFunc(name, helper, signature)
	}
}

// Evaluate calls the mock EvaluateFunc if set, otherwise returns nil, nil.
func (m *MockExpressionEvaluator) Evaluate(expression string, config map[string]any, featurePath string) (any, error) {
	if m.EvaluateFunc != nil {
		return m.EvaluateFunc(expression, config, featurePath)
	}
	return nil, nil
}

// EvaluateDefaults calls the mock EvaluateDefaultsFunc if set, otherwise returns an empty map, nil.
func (m *MockExpressionEvaluator) EvaluateDefaults(defaults map[string]any, config map[string]any, featurePath string) (map[string]any, error) {
	if m.EvaluateDefaultsFunc != nil {
		return m.EvaluateDefaultsFunc(defaults, config, featurePath)
	}
	return make(map[string]any), nil
}

// EvaluateValue calls the mock EvaluateValueFunc if set, otherwise returns the input string, nil.
func (m *MockExpressionEvaluator) EvaluateValue(s string, config map[string]any, featurePath string) (any, error) {
	if m.EvaluateValueFunc != nil {
		return m.EvaluateValueFunc(s, config, featurePath)
	}
	return s, nil
}

// InterpolateString calls the mock InterpolateStringFunc if set, otherwise returns the input string, nil.
func (m *MockExpressionEvaluator) InterpolateString(s string, config map[string]any, featurePath string) (string, error) {
	if m.InterpolateStringFunc != nil {
		return m.InterpolateStringFunc(s, config, featurePath)
	}
	return s, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockExpressionEvaluator implements ExpressionEvaluator.
var _ ExpressionEvaluator = (*MockExpressionEvaluator)(nil)
