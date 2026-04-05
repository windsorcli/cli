package flux

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// The MockStack is a test implementation of the Stack interface.
// It provides function fields that can be set to customize behavior in tests.
// The MockStack acts as a controllable test double for the Stack interface,
// enabling precise control over Plan behavior in unit tests.

// =============================================================================
// Types
// =============================================================================

// MockStack is a mock implementation of the Stack interface for testing.
type MockStack struct {
	PlanFunc                 func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanJSONFunc             func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanSummaryFunc          func(blueprint *blueprintv1alpha1.Blueprint) ([]KustomizePlan, []string)
	PlanComponentSummaryFunc func(blueprint *blueprintv1alpha1.Blueprint, name string) KustomizePlan
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

// Plan is a mock implementation of the Plan method.
func (m *MockStack) Plan(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if m.PlanFunc != nil {
		return m.PlanFunc(blueprint, componentID)
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

// PlanSummary is a mock implementation of the PlanSummary method.
func (m *MockStack) PlanSummary(blueprint *blueprintv1alpha1.Blueprint) ([]KustomizePlan, []string) {
	if m.PlanSummaryFunc != nil {
		return m.PlanSummaryFunc(blueprint)
	}
	return nil, nil
}

// PlanComponentSummary is a mock implementation of the PlanComponentSummary method.
func (m *MockStack) PlanComponentSummary(blueprint *blueprintv1alpha1.Blueprint, name string) KustomizePlan {
	if m.PlanComponentSummaryFunc != nil {
		return m.PlanComponentSummaryFunc(blueprint, name)
	}
	return KustomizePlan{Name: name}
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockStack implements Stack
var _ Stack = (*MockStack)(nil)
