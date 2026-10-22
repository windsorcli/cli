package v1alpha1

import (
	"testing"

	"github.com/goccy/go-yaml"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBoolExpression_MarshalYAML(t *testing.T) {
	t.Run("PreservesExpressionInRoundTrip", func(t *testing.T) {
		yamlData := []byte(`destroy: "${shouldDestroy}"`)

		var component struct {
			Destroy *BoolExpression `yaml:"destroy,omitempty"`
		}
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Destroy == nil {
			t.Fatal("Expected Destroy to be set")
		}
		if !component.Destroy.IsExpr {
			t.Error("Expected IsExpr to be true")
		}
		if component.Destroy.Expr != "${shouldDestroy}" {
			t.Errorf("Expected Expr to be '${shouldDestroy}', got %q", component.Destroy.Expr)
		}
		if component.Destroy.Value != nil {
			t.Error("Expected Value to be nil for expression")
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip struct {
			Destroy *BoolExpression `yaml:"destroy,omitempty"`
		}
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Destroy == nil {
			t.Fatal("Expected Destroy to be preserved after round-trip")
		}
		if !roundTrip.Destroy.IsExpr {
			t.Error("Expected IsExpr to be true after round-trip")
		}
		if roundTrip.Destroy.Expr != "${shouldDestroy}" {
			t.Errorf("Expected Expr to be preserved as '${shouldDestroy}', got %q", roundTrip.Destroy.Expr)
		}
	})

	t.Run("PreservesBooleanValueInRoundTrip", func(t *testing.T) {
		yamlData := []byte(`destroy: true`)

		var component struct {
			Destroy *BoolExpression `yaml:"destroy,omitempty"`
		}
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Destroy == nil {
			t.Fatal("Expected Destroy to be set")
		}
		if component.Destroy.IsExpr {
			t.Error("Expected IsExpr to be false")
		}
		if component.Destroy.Value == nil || !*component.Destroy.Value {
			t.Error("Expected Value to be true")
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip struct {
			Destroy *BoolExpression `yaml:"destroy,omitempty"`
		}
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Destroy == nil {
			t.Fatal("Expected Destroy to be preserved after round-trip")
		}
		if roundTrip.Destroy.IsExpr {
			t.Error("Expected IsExpr to be false after round-trip")
		}
		if roundTrip.Destroy.Value == nil || !*roundTrip.Destroy.Value {
			t.Error("Expected Value to be true after round-trip")
		}
	})
}

func TestIntExpression_MarshalYAML(t *testing.T) {
	t.Run("PreservesExpressionInRoundTrip", func(t *testing.T) {
		yamlData := []byte(`parallelism: "${cluster.parallelism ?? 10}"`)

		var component struct {
			Parallelism *IntExpression `yaml:"parallelism,omitempty"`
		}
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Parallelism == nil {
			t.Fatal("Expected Parallelism to be set")
		}
		if !component.Parallelism.IsExpr {
			t.Error("Expected IsExpr to be true")
		}
		if component.Parallelism.Expr != "${cluster.parallelism ?? 10}" {
			t.Errorf("Expected Expr to be '${cluster.parallelism ?? 10}', got %q", component.Parallelism.Expr)
		}
		if component.Parallelism.Value != nil {
			t.Error("Expected Value to be nil for expression")
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip struct {
			Parallelism *IntExpression `yaml:"parallelism,omitempty"`
		}
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Parallelism == nil {
			t.Fatal("Expected Parallelism to be preserved after round-trip")
		}
		if !roundTrip.Parallelism.IsExpr {
			t.Error("Expected IsExpr to be true after round-trip")
		}
		if roundTrip.Parallelism.Expr != "${cluster.parallelism ?? 10}" {
			t.Errorf("Expected Expr to be preserved as '${cluster.parallelism ?? 10}', got %q", roundTrip.Parallelism.Expr)
		}
	})

	t.Run("PreservesIntegerValueInRoundTrip", func(t *testing.T) {
		yamlData := []byte(`parallelism: 5`)

		var component struct {
			Parallelism *IntExpression `yaml:"parallelism,omitempty"`
		}
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Parallelism == nil {
			t.Fatal("Expected Parallelism to be set")
		}
		if component.Parallelism.IsExpr {
			t.Error("Expected IsExpr to be false")
		}
		if component.Parallelism.Value == nil || *component.Parallelism.Value != 5 {
			t.Error("Expected Value to be 5")
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip struct {
			Parallelism *IntExpression `yaml:"parallelism,omitempty"`
		}
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Parallelism == nil {
			t.Fatal("Expected Parallelism to be preserved after round-trip")
		}
		if roundTrip.Parallelism.IsExpr {
			t.Error("Expected IsExpr to be false after round-trip")
		}
		if roundTrip.Parallelism.Value == nil || *roundTrip.Parallelism.Value != 5 {
			t.Error("Expected Value to be 5 after round-trip")
		}
	})
}

func TestBoolExpression_IsEnabled(t *testing.T) {
	t.Run("ReturnsTrueWhenNil", func(t *testing.T) {
		var expr *BoolExpression
		if !expr.IsEnabled() {
			t.Error("Expected IsEnabled to return true when nil")
		}
	})

	t.Run("ReturnsTrueWhenValueIsTrue", func(t *testing.T) {
		trueVal := true
		expr := &BoolExpression{Value: &trueVal, IsExpr: false}
		if !expr.IsEnabled() {
			t.Error("Expected IsEnabled to return true when value is true")
		}
	})

	t.Run("ReturnsFalseWhenValueIsFalse", func(t *testing.T) {
		falseVal := false
		expr := &BoolExpression{Value: &falseVal, IsExpr: false}
		if expr.IsEnabled() {
			t.Error("Expected IsEnabled to return false when value is false")
		}
	})

	t.Run("ReturnsTrueWhenIsExpression", func(t *testing.T) {
		expr := &BoolExpression{IsExpr: true, Expr: "${some.condition ?? true}"}
		if !expr.IsEnabled() {
			t.Error("Expected IsEnabled to return true when IsExpr is true")
		}
	})

	t.Run("ReturnsTrueWhenValueIsNilButNotExpression", func(t *testing.T) {
		expr := &BoolExpression{Value: nil, IsExpr: false}
		if !expr.IsEnabled() {
			t.Error("Expected IsEnabled to return true when Value is nil and not expression")
		}
	})
}

func TestBoolExpression_IsInstalled(t *testing.T) {
	t.Run("ReturnsFalseWhenNil", func(t *testing.T) {
		var expr *BoolExpression
		if expr.IsInstalled() {
			t.Error("Expected IsInstalled to return false when nil")
		}
	})

	t.Run("ReturnsTrueWhenValueIsTrue", func(t *testing.T) {
		trueVal := true
		expr := &BoolExpression{Value: &trueVal, IsExpr: false}
		if !expr.IsInstalled() {
			t.Error("Expected IsInstalled to return true when value is true")
		}
	})

	t.Run("ReturnsFalseWhenValueIsFalse", func(t *testing.T) {
		falseVal := false
		expr := &BoolExpression{Value: &falseVal, IsExpr: false}
		if expr.IsInstalled() {
			t.Error("Expected IsInstalled to return false when value is false")
		}
	})

	t.Run("ReturnsFalseWhenIsExpression", func(t *testing.T) {
		expr := &BoolExpression{IsExpr: true, Expr: "${some.condition ?? true}"}
		if expr.IsInstalled() {
			t.Error("Expected IsInstalled to return false when IsExpr is true")
		}
	})

	t.Run("ReturnsFalseWhenValueIsNilButNotExpression", func(t *testing.T) {
		expr := &BoolExpression{Value: nil, IsExpr: false}
		if expr.IsInstalled() {
			t.Error("Expected IsInstalled to return false when Value is nil and not expression")
		}
	})
}
