// The expression types are the scalar blueprint fields that accept a literal value or a string.
// They provide DurationString, BoolExpression and IntExpression, with their YAML methods and their conversions.
// BoolExpression and IntExpression keep a string expression, so that facet processing can evaluate it later.

package v1alpha1

import (
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Types
// =============================================================================

// DurationString represents a duration that marshals/unmarshals as a string (e.g., "5m").
// It can be converted to metav1.Duration for use with Kubernetes APIs.
type DurationString struct {
	Duration time.Duration
}

// MarshalYAML implements yaml.Marshaler to write duration as a string.
func (d *DurationString) MarshalYAML() (any, error) {
	if d == nil || d.Duration == 0 {
		return nil, nil
	}
	return d.Duration.String(), nil
}

// UnmarshalYAML implements yaml.Unmarshaler to read duration from a string or object.
func (d *DurationString) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		if s == "" {
			d.Duration = 0
			return nil
		}
		dur, err := time.ParseDuration(s)
		if err != nil {
			return werror.Wrap(err, "invalid duration %q", s)
		}
		d.Duration = dur
		return nil
	}

	var obj map[string]any
	if err := unmarshal(&obj); err != nil {
		return err
	}
	if durationStr, ok := obj["duration"].(string); ok {
		if durationStr == "" {
			d.Duration = 0
			return nil
		}
		dur, err := time.ParseDuration(durationStr)
		if err != nil {
			return werror.Wrap(err, "invalid duration %q", durationStr)
		}
		d.Duration = dur
		return nil
	}
	return fmt.Errorf("duration must be a string or an object with a 'duration' field")
}

// ToMetaV1Duration converts DurationString to *metav1.Duration.
func (d *DurationString) ToMetaV1Duration() *metav1.Duration {
	if d == nil || d.Duration == 0 {
		return nil
	}
	return &metav1.Duration{Duration: d.Duration}
}

// FromMetaV1Duration creates a DurationString from *metav1.Duration.
func FromMetaV1Duration(d *metav1.Duration) *DurationString {
	if d == nil || d.Duration == 0 {
		return nil
	}
	return &DurationString{Duration: d.Duration}
}

// BoolExpression represents a boolean value that can be unmarshaled from a boolean or a string expression.
// String expressions are preserved for later evaluation during facet processing.
type BoolExpression struct {
	Value  *bool
	Expr   string
	IsExpr bool
}

// MarshalYAML implements yaml.Marshaler to write boolean as a bool or expression string.
func (b *BoolExpression) MarshalYAML() (any, error) {
	if b == nil {
		return nil, nil
	}
	if b.IsExpr && b.Expr != "" {
		return b.Expr, nil
	}
	if b.Value != nil {
		return *b.Value, nil
	}
	return nil, nil
}

// UnmarshalYAML implements yaml.Unmarshaler to read boolean from a bool or string expression.
func (b *BoolExpression) UnmarshalYAML(unmarshal func(any) error) error {
	var boolVal bool
	if err := unmarshal(&boolVal); err == nil {
		b.Value = &boolVal
		b.IsExpr = false
		b.Expr = ""
		return nil
	}

	var strVal string
	if err := unmarshal(&strVal); err == nil {
		if strings.Contains(strVal, "${") {
			b.Value = nil
			b.IsExpr = true
			b.Expr = strVal
		} else {
			boolVal := strVal == "true"
			b.Value = &boolVal
			b.IsExpr = false
			b.Expr = ""
		}
		return nil
	}

	return fmt.Errorf("bool expression must be a boolean or string")
}

// ToBool returns the boolean value, or nil if it's an expression.
func (b *BoolExpression) ToBool() *bool {
	if b == nil {
		return nil
	}
	return b.Value
}

// DeepCopy creates a deep copy of the BoolExpression.
func (b *BoolExpression) DeepCopy() *BoolExpression {
	if b == nil {
		return nil
	}
	copy := &BoolExpression{
		IsExpr: b.IsExpr,
		Expr:   b.Expr,
	}
	if b.Value != nil {
		valueCopy := *b.Value
		copy.Value = &valueCopy
	}
	return copy
}

// IsEnabled returns true if the BoolExpression is nil (default) or if its value is true.
// Returns false only if the value is explicitly set to false.
// This is used for fields like Enabled that default to true.
func (b *BoolExpression) IsEnabled() bool {
	if b == nil {
		return true
	}
	if b.IsExpr {
		return true
	}
	if b.Value == nil {
		return true
	}
	return *b.Value
}

// IsInstalled returns true only if the BoolExpression is explicitly set to true.
// Returns false if nil (default), if it's an expression, or if explicitly false.
// This is used for fields like Install that default to false.
func (b *BoolExpression) IsInstalled() bool {
	if b == nil {
		return false
	}
	if b.IsExpr {
		return false
	}
	if b.Value == nil {
		return false
	}
	return *b.Value
}

// IntExpression represents an integer value that can be unmarshaled from an integer or a string expression.
// String expressions are preserved for later evaluation during facet processing.
type IntExpression struct {
	Value  *int
	Expr   string
	IsExpr bool
}

// MarshalYAML implements yaml.Marshaler to write integer as an int or expression string.
func (i *IntExpression) MarshalYAML() (any, error) {
	if i == nil {
		return nil, nil
	}
	if i.IsExpr && i.Expr != "" {
		return i.Expr, nil
	}
	if i.Value != nil {
		return *i.Value, nil
	}
	return nil, nil
}

// UnmarshalYAML implements yaml.Unmarshaler to read integer from an int or string expression.
func (i *IntExpression) UnmarshalYAML(unmarshal func(any) error) error {
	var intVal int
	if err := unmarshal(&intVal); err == nil {
		i.Value = &intVal
		i.IsExpr = false
		i.Expr = ""
		return nil
	}

	var strVal string
	if err := unmarshal(&strVal); err == nil {
		if strings.Contains(strVal, "${") {
			i.Value = nil
			i.IsExpr = true
			i.Expr = strVal
		} else {
			var parsed int
			if _, err := fmt.Sscanf(strVal, "%d", &parsed); err != nil {
				return fmt.Errorf("invalid integer: %q", strVal)
			}
			i.Value = &parsed
			i.IsExpr = false
			i.Expr = ""
		}
		return nil
	}

	return fmt.Errorf("int expression must be an integer or string")
}

// ToInt returns the integer value, or nil if it's an expression.
func (i *IntExpression) ToInt() *int {
	if i == nil {
		return nil
	}
	return i.Value
}

// DeepCopy creates a deep copy of the IntExpression.
func (i *IntExpression) DeepCopy() *IntExpression {
	if i == nil {
		return nil
	}
	copy := &IntExpression{
		IsExpr: i.IsExpr,
		Expr:   i.Expr,
	}
	if i.Value != nil {
		valueCopy := *i.Value
		copy.Value = &valueCopy
	}
	return copy
}
