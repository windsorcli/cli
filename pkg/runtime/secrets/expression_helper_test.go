package secrets

import (
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

func TestResolveReference(t *testing.T) {
	t.Run("ReturnsResolvedValueContainingExpressionSyntax", func(t *testing.T) {
		mockProvider := NewMockSecretsProvider(shell.NewMockShell())
		mockProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input != "${op.vault.item.field}" {
				t.Fatalf("expected provider input %q, got %q", "${op.vault.item.field}", input)
			}
			return "abc${x}", nil
		}

		value, err := ResolveReference("op.vault.item.field", []SecretsProvider{mockProvider})

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if value != "abc${x}" {
			t.Fatalf("expected value %q, got %q", "abc${x}", value)
		}
	})

	t.Run("ReturnsErrorWhenReferenceRemainsUnresolved", func(t *testing.T) {
		mockProvider := NewMockSecretsProvider(shell.NewMockShell())
		mockProvider.ParseSecretsFunc = func(input string) (string, error) {
			return input, nil
		}

		_, err := ResolveReference("op.vault.item.field", []SecretsProvider{mockProvider})

		if err == nil {
			t.Fatalf("expected unresolved reference error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to resolve secret reference: op.vault.item.field") {
			t.Fatalf("expected unresolved reference error, got %v", err)
		}
	})

	t.Run("DoesNotLoadSecretsDuringReferenceResolution", func(t *testing.T) {
		mockProvider := NewMockSecretsProvider(shell.NewMockShell())
		loadCalls := 0
		mockProvider.LoadSecretsFunc = func() error {
			loadCalls++
			return nil
		}
		mockProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "${op.vault.item.field}" {
				return "resolved-value", nil
			}
			return input, nil
		}

		value, err := ResolveReference("op.vault.item.field", []SecretsProvider{mockProvider})

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if value != "resolved-value" {
			t.Fatalf("expected resolved value, got %q", value)
		}
		if loadCalls != 0 {
			t.Fatalf("expected ResolveReference to avoid LoadSecrets side effects, got %d calls", loadCalls)
		}
	})
}

func TestRegisterSecretHelper(t *testing.T) {
	t.Run("NormalizesLegacyProviderSyntaxOutsideEvaluator", func(t *testing.T) {
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		eval := evaluator.NewExpressionEvaluator(mockConfig, "", "")

		RegisterSecretHelper(eval, func(ref string) (string, error) {
			return "resolved:" + ref, nil
		})

		result, err := eval.Evaluate("${op.platform.db.password}", "", nil, true)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !evaluator.ContainsSecretValue(result) {
			t.Fatalf("expected secret-qualified value, got %T", result)
		}
		if got := evaluator.UnwrapSecretValue(result); got != "resolved:op.platform.db.password" {
			t.Fatalf("expected normalized legacy secret expression result, got %v", got)
		}
	})

	t.Run("NormalizesUppercaseProviderPrefixOutsideEvaluator", func(t *testing.T) {
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		eval := evaluator.NewExpressionEvaluator(mockConfig, "", "")

		RegisterSecretHelper(eval, func(ref string) (string, error) {
			return "resolved:" + ref, nil
		})

		result, err := eval.Evaluate("${OP.platform.db.password}", "", nil, true)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !evaluator.ContainsSecretValue(result) {
			t.Fatalf("expected secret-qualified value, got %T", result)
		}
		if got := evaluator.UnwrapSecretValue(result); got != "resolved:OP.platform.db.password" {
			t.Fatalf("expected normalized uppercase provider expression result, got %v", got)
		}
	})

	t.Run("NormalizesBracketNotationWithSpacesOutsideEvaluator", func(t *testing.T) {
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		eval := evaluator.NewExpressionEvaluator(mockConfig, "", "")

		RegisterSecretHelper(eval, func(ref string) (string, error) {
			return "resolved:" + ref, nil
		})

		result, err := eval.Evaluate(`${op.personal["The Criterion Channel"]["password"]}`, "", nil, true)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !evaluator.ContainsSecretValue(result) {
			t.Fatalf("expected secret-qualified value, got %T", result)
		}
		if got := evaluator.UnwrapSecretValue(result); got != `resolved:op.personal["The Criterion Channel"]["password"]` {
			t.Fatalf("expected normalized bracket provider expression result, got %v", got)
		}
	})

	t.Run("KeepsExplicitSecretHelperCallsWorking", func(t *testing.T) {
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		eval := evaluator.NewExpressionEvaluator(mockConfig, "", "")

		RegisterSecretHelper(eval, func(ref string) (string, error) {
			return "resolved:" + ref, nil
		})

		result, err := eval.Evaluate(`${secret("secret.op.platform.db.password")}`, "", nil, true)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !evaluator.ContainsSecretValue(result) {
			t.Fatalf("expected secret-qualified value, got %T", result)
		}
		if got := evaluator.UnwrapSecretValue(result); got != "resolved:secret.op.platform.db.password" {
			t.Fatalf("expected explicit helper expression result, got %v", got)
		}
	})

	t.Run("DoesNotNormalizeNonSecretExpressionsStartingWithOp", func(t *testing.T) {
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"op": map[string]any{
					"flags": map[string]any{
						"enabled": true,
					},
				},
			}, nil
		}
		eval := evaluator.NewExpressionEvaluator(mockConfig, "", "")

		RegisterSecretHelper(eval, func(ref string) (string, error) {
			return "resolved:" + ref, nil
		})

		result, err := eval.Evaluate("${op.flags.enabled && true}", "", nil, true)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result != true {
			t.Fatalf("expected boolean expression result true, got %v", result)
		}
	})

	t.Run("DoesNotNormalizeNonSecretExpressionsStartingWithSops", func(t *testing.T) {
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"sops": map[string]any{
					"enabled": false,
				},
			}, nil
		}
		eval := evaluator.NewExpressionEvaluator(mockConfig, "", "")

		RegisterSecretHelper(eval, func(ref string) (string, error) {
			return "resolved:" + ref, nil
		})

		result, err := eval.Evaluate("${sops.enabled ?? true}", "", nil, true)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result != false {
			t.Fatalf("expected coalesce expression to return false, got %v", result)
		}
	})
}

func TestIsSecretReferenceExpression(t *testing.T) {
	t.Run("MatchesPlainReferenceForms", func(t *testing.T) {
		candidates := []string{
			"op.platform.db.password",
			`op["personal"]["item"]["field"]`,
			`op.personal["The Criterion Channel"]["password"]`,
			"OP.platform.db.password",
			"sops.platform.db.password",
			"secret.op.platform.db.password",
			"secrets.platform.db.password",
		}
		for _, candidate := range candidates {
			if !IsSecretReferenceExpression(candidate) {
				t.Fatalf("expected candidate %q to be recognized as secret reference", candidate)
			}
		}
	})

	t.Run("RejectsOperatorExpressions", func(t *testing.T) {
		candidates := []string{
			"op.flags.enabled && true",
			"sops.enabled ?? true",
			"secret.op.flags.enabled ? 1 : 0",
		}
		for _, candidate := range candidates {
			if IsSecretReferenceExpression(candidate) {
				t.Fatalf("expected candidate %q to be rejected", candidate)
			}
		}
	})
}
