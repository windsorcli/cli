package evaluator

import "strings"

// =============================================================================
// Helpers
// =============================================================================

// IsDeferredValue reports whether value is a DeferredValue (value or pointer form).
func IsDeferredValue(value any) bool {
	switch value.(type) {
	case DeferredValue, *DeferredValue:
		return true
	default:
		return false
	}
}

// isSecretValue reports whether value is a SecretValue (value or pointer form).
func isSecretValue(value any) bool {
	switch value.(type) {
	case SecretValue, *SecretValue:
		return true
	default:
		return false
	}
}

// UnwrapSecretValue returns the wrapped value for SecretValue, or the value unchanged.
func UnwrapSecretValue(value any) any {
	switch v := value.(type) {
	case SecretValue:
		return v.Value
	case *SecretValue:
		if v == nil {
			return nil
		}
		return v.Value
	default:
		return value
	}
}

// ContainsSecretValue reports whether value or any nested field contains SecretValue.
func ContainsSecretValue(value any) bool {
	switch v := value.(type) {
	case SecretValue:
		return true
	case *SecretValue:
		return v != nil
	case map[string]any:
		for _, item := range v {
			if ContainsSecretValue(item) {
				return true
			}
		}
		return false
	case []any:
		for _, item := range v {
			if ContainsSecretValue(item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// DeferredExpression returns the preserved raw expression string when value is DeferredValue.
func DeferredExpression(value any) (string, bool) {
	switch v := value.(type) {
	case DeferredValue:
		return v.Expression, true
	case *DeferredValue:
		if v == nil {
			return "", false
		}
		return v.Expression, true
	default:
		return "", false
	}
}

// ContainsExpression determines whether the provided value contains an unresolved expression.
// An expression is identified by the pattern "${...}" in string values.
func ContainsExpression(value any) bool {
	switch v := value.(type) {
	case DeferredValue:
		return ContainsExpression(v.Expression)
	case *DeferredValue:
		if v == nil {
			return false
		}
		return ContainsExpression(v.Expression)
	case SecretValue:
		return ContainsExpression(v.Value)
	case *SecretValue:
		if v == nil {
			return false
		}
		return ContainsExpression(v.Value)
	case string:
		if !strings.Contains(v, "${") {
			return false
		}
		start := strings.Index(v, "${")
		if start == -1 {
			return false
		}
		return findExpressionEnd(v, start) != -1
	case map[string]any:
		for _, val := range v {
			if ContainsExpression(val) {
				return true
			}
		}
		return false
	case []any:
		for _, item := range v {
			if ContainsExpression(item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
