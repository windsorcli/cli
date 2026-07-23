// Package flux provides Flux kustomization stack management functionality.
// This file provides a mock Notifier for use in tests of packages that depend
// on the webhook notifier interface without touching a real cluster.

package flux

import (
	"context"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Types
// =============================================================================

// MockNotifier is a mock implementation of the Notifier interface for testing.
// Tests set NotifyFunc to control behavior; the default implementation returns
// nil to keep callers' "best-effort" semantics unchanged when no override is set.
type MockNotifier struct {
	NotifyFunc                  func(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error
	ReconcileKustomizationsFunc func(ctx context.Context, names []string) error
	ReconcileHelmReleasesFunc   func(ctx context.Context, refs []HelmReleaseRef, force bool) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockNotifier returns a MockNotifier with no overrides configured.
func NewMockNotifier() *MockNotifier {
	return &MockNotifier{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Notify implements the Notifier interface. Delegates to NotifyFunc when set
// and otherwise returns nil to match the Notifier's best-effort contract.
func (m *MockNotifier) Notify(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error {
	if m.NotifyFunc != nil {
		return m.NotifyFunc(ctx, blueprint)
	}
	return nil
}

// ReconcileKustomizations implements the Notifier interface. Delegates to ReconcileKustomizationsFunc when
// set and otherwise returns nil to match the Notifier's best-effort contract.
func (m *MockNotifier) ReconcileKustomizations(ctx context.Context, names []string) error {
	if m.ReconcileKustomizationsFunc != nil {
		return m.ReconcileKustomizationsFunc(ctx, names)
	}
	return nil
}

// ReconcileHelmReleases implements the Notifier interface. Delegates to ReconcileHelmReleasesFunc when set
// and otherwise returns nil to match the Notifier's best-effort contract.
func (m *MockNotifier) ReconcileHelmReleases(ctx context.Context, refs []HelmReleaseRef, force bool) error {
	if m.ReconcileHelmReleasesFunc != nil {
		return m.ReconcileHelmReleasesFunc(ctx, refs, force)
	}
	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockNotifier implements the Notifier interface
var _ Notifier = (*MockNotifier)(nil)
