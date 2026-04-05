package terraform

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// The MockStack is a test implementation of the Stack interface.
// It provides function fields that can be set to customize behavior in tests,
// The MockStack acts as a controllable test double for the Stack interface,
// enabling precise control over Up, Down, Plan, and Apply behaviors in unit tests.

// =============================================================================
// Types
// =============================================================================

// MockStack is a mock implementation of the Stack interface for testing.
type MockStack struct {
	UpFunc                   func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error
	DownFunc                 func(blueprint *blueprintv1alpha1.Blueprint) error
	PlanFunc                 func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanAllFunc              func(blueprint *blueprintv1alpha1.Blueprint) error
	PlanAllJSONFunc          func(blueprint *blueprintv1alpha1.Blueprint) error
	PlanJSONFunc             func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	ApplyFunc                func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanSummaryFunc          func(blueprint *blueprintv1alpha1.Blueprint) []TerraformComponentPlan
	PlanComponentSummaryFunc func(blueprint *blueprintv1alpha1.Blueprint, componentID string) TerraformComponentPlan
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockStack creates a new mock stack.
func NewMockStack() *MockStack {
	return &MockStack{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Up is a mock implementation of the Up method.
func (m *MockStack) Up(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
	if m.UpFunc != nil {
		return m.UpFunc(blueprint, onApply...)
	}
	return nil
}

// Down is a mock implementation of the Down method.
func (m *MockStack) Down(blueprint *blueprintv1alpha1.Blueprint) error {
	if m.DownFunc != nil {
		return m.DownFunc(blueprint)
	}
	return nil
}

// Plan is a mock implementation of the Plan method.
func (m *MockStack) Plan(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if m.PlanFunc != nil {
		return m.PlanFunc(blueprint, componentID)
	}
	return nil
}

// PlanAll is a mock implementation of the PlanAll method.
func (m *MockStack) PlanAll(blueprint *blueprintv1alpha1.Blueprint) error {
	if m.PlanAllFunc != nil {
		return m.PlanAllFunc(blueprint)
	}
	return nil
}

// PlanAllJSON is a mock implementation of the PlanAllJSON method.
func (m *MockStack) PlanAllJSON(blueprint *blueprintv1alpha1.Blueprint) error {
	if m.PlanAllJSONFunc != nil {
		return m.PlanAllJSONFunc(blueprint)
	}
	return nil
}

// PlanJSON is a mock implementation of the PlanJSON method.
func (m *MockStack) PlanJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if m.PlanJSONFunc != nil {
		return m.PlanJSONFunc(blueprint, componentID)
	}
	return nil
}

// Apply is a mock implementation of the Apply method.
func (m *MockStack) Apply(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if m.ApplyFunc != nil {
		return m.ApplyFunc(blueprint, componentID)
	}
	return nil
}

// PlanSummary is a mock implementation of the PlanSummary method.
func (m *MockStack) PlanSummary(blueprint *blueprintv1alpha1.Blueprint) []TerraformComponentPlan {
	if m.PlanSummaryFunc != nil {
		return m.PlanSummaryFunc(blueprint)
	}
	return nil
}

// PlanComponentSummary is a mock implementation of the PlanComponentSummary method.
func (m *MockStack) PlanComponentSummary(blueprint *blueprintv1alpha1.Blueprint, componentID string) TerraformComponentPlan {
	if m.PlanComponentSummaryFunc != nil {
		return m.PlanComponentSummaryFunc(blueprint, componentID)
	}
	return TerraformComponentPlan{ComponentID: componentID}
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockStack implements Stack
var _ Stack = (*MockStack)(nil)
