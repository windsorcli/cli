// The ExpressionHelper is a secrets expression registration and resolution component.
// It provides evaluator helper wiring for secret() expressions and provider-backed resolution.
// The ExpressionHelper acts as the bridge between evaluator callbacks and secrets providers.
// It centralizes validation, deferred behavior, and reference resolution for secret expressions.
package secrets

import (
	"fmt"
	"strings"

	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Public Methods
// =============================================================================

// RegisterSecretHelper registers the secret() helper with the evaluator.
func RegisterSecretHelper(eval evaluator.ExpressionEvaluator, resolve func(string) (string, error)) {
	if eval == nil {
		return
	}
	if normalizerAware, ok := eval.(interface {
		RegisterExpressionNormalizer(func(string) string)
	}); ok {
		normalizerAware.RegisterExpressionNormalizer(normalizeSecretReferenceExpression)
	}
	eval.Register("secret", func(params []any, deferred bool) (any, error) {
		return evaluateSecretHelper(params, deferred, resolve)
	}, new(func(string) any))
}

// ResolveReference resolves a single secret reference through configured providers.
func ResolveReference(ref string, providers []SecretsProvider) (string, error) {
	if len(providers) == 0 {
		return "", fmt.Errorf("no secrets providers configured")
	}
	initialReference := fmt.Sprintf("${%s}", ref)
	value := initialReference
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		parsed, err := provider.ParseSecrets(value)
		if err == nil {
			value = parsed
		}
	}
	if value == initialReference {
		return "", fmt.Errorf("failed to resolve secret reference: %s", ref)
	}
	return value, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// evaluateSecretHelper executes the secret() evaluator helper and returns a secret-qualified value.
func evaluateSecretHelper(params []any, deferred bool, resolve func(string) (string, error)) (any, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("secret() requires exactly 1 argument, got %d", len(params))
	}
	ref, ok := params[0].(string)
	if !ok || strings.TrimSpace(ref) == "" {
		return nil, fmt.Errorf("secret() argument must be a non-empty string, got %T", params[0])
	}
	if !deferred {
		return evaluator.SecretValue{Value: fmt.Sprintf("${%s}", ref)}, nil
	}
	resolved, err := resolve(ref)
	if err != nil {
		return nil, err
	}
	return evaluator.SecretValue{Value: resolved}, nil
}

func normalizeSecretReferenceExpression(expression string) string {
	trimmed := strings.TrimSpace(expression)
	if isSecretReferenceExpression(trimmed) {
		escaped := strings.ReplaceAll(trimmed, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return fmt.Sprintf(`secret("%s")`, escaped)
	}
	return expression
}

func isSecretReferenceExpression(expression string) bool {
	if !(strings.HasPrefix(expression, "secret.op.") ||
		strings.HasPrefix(expression, "secret.sops.") ||
		strings.HasPrefix(expression, "op.") ||
		strings.HasPrefix(expression, "op[") ||
		strings.HasPrefix(expression, "sops.")) {
		return false
	}
	if strings.Contains(expression, "${") {
		return false
	}
	return !strings.ContainsAny(expression, " \t\n\r()+-*/%<>=!&|?:,")
}
