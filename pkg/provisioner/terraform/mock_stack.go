package terraform

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// The MockStack is a test implementation of the Stack interface.
// It provides function fields that can be set to customize behavior in tests,
// The MockStack acts as a controllable test double for the Stack interface,
// enabling precise control over Up, Down, Plan, Apply, and Destroy behaviors in unit tests.

// =============================================================================
// Types
// =============================================================================

// MockStack is a mock implementation of the Stack interface for testing.
type MockStack struct {
	UpFunc                    func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error
	MigrateStateFunc          func(blueprint *blueprintv1alpha1.Blueprint) ([]string, error)
	MigrateComponentStateFunc func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	HasRemoteStateFunc             func(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error)
	HasLocalStateWithResourcesFunc func(componentID string) (bool, error)
	InitComponentFunc              func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	RemoveLocalStateFunc           func(componentID string) error
	PostApplyFunc             func(fns ...func(id string) error)
	DestroyAllFunc            func(blueprint *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error)
	PlanFunc                  func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanAllFunc               func(blueprint *blueprintv1alpha1.Blueprint) error
	PlanJSONFunc              func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanAllJSONFunc           func(blueprint *blueprintv1alpha1.Blueprint) error
	ApplyFunc                 func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	DestroyFunc               func(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error)
	PlanSummaryFunc           func(blueprint *blueprintv1alpha1.Blueprint) []TerraformComponentPlan
	PlanComponentSummaryFunc  func(blueprint *blueprintv1alpha1.Blueprint, componentID string) TerraformComponentPlan
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

// MigrateState is a mock implementation of the MigrateState method.
func (m *MockStack) MigrateState(blueprint *blueprintv1alpha1.Blueprint) ([]string, error) {
	if m.MigrateStateFunc != nil {
		return m.MigrateStateFunc(blueprint)
	}
	return nil, nil
}

// MigrateComponentState is a mock implementation of the MigrateComponentState method.
func (m *MockStack) MigrateComponentState(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if m.MigrateComponentStateFunc != nil {
		return m.MigrateComponentStateFunc(blueprint, componentID)
	}
	return nil
}

// HasRemoteState is a mock implementation of the HasRemoteState method.
func (m *MockStack) HasRemoteState(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	if m.HasRemoteStateFunc != nil {
		return m.HasRemoteStateFunc(blueprint, componentID)
	}
	return false, nil
}

// HasLocalStateWithResources is a mock implementation of the HasLocalStateWithResources method.
func (m *MockStack) HasLocalStateWithResources(componentID string) (bool, error) {
	if m.HasLocalStateWithResourcesFunc != nil {
		return m.HasLocalStateWithResourcesFunc(componentID)
	}
	return false, nil
}

// InitComponent is a mock implementation of the InitComponent method.
func (m *MockStack) InitComponent(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if m.InitComponentFunc != nil {
		return m.InitComponentFunc(blueprint, componentID)
	}
	return nil
}

// RemoveLocalState is a mock implementation of the RemoveLocalState method.
func (m *MockStack) RemoveLocalState(componentID string) error {
	if m.RemoveLocalStateFunc != nil {
		return m.RemoveLocalStateFunc(componentID)
	}
	return nil
}

// PostApply is a mock implementation of the PostApply method.
func (m *MockStack) PostApply(fns ...func(id string) error) {
	if m.PostApplyFunc != nil {
		m.PostApplyFunc(fns...)
	}
}

// DestroyAll is a mock implementation of the DestroyAll method.
func (m *MockStack) DestroyAll(blueprint *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) {
	if m.DestroyAllFunc != nil {
		return m.DestroyAllFunc(blueprint, excludeIDs...)
	}
	return nil, nil
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

// Destroy is a mock implementation of the Destroy method.
func (m *MockStack) Destroy(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	if m.DestroyFunc != nil {
		return m.DestroyFunc(blueprint, componentID)
	}
	return false, nil
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
