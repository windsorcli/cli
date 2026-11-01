package kubernetes

import (
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Test Setup
// =============================================================================

// kustomizationsPrivWithTimeout returns a kustomization with name, timeout d, and dependencies dependsOn.
func kustomizationsPrivWithTimeout(name string, d time.Duration, dependsOn ...string) blueprintv1alpha1.Kustomization {
	return blueprintv1alpha1.Kustomization{
		Name:      name,
		Timeout:   &blueprintv1alpha1.DurationString{Duration: d},
		DependsOn: dependsOn,
	}
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestBaseKubernetesManager_calculateTotalWaitTime(t *testing.T) {
	tests := []struct {
		name           string
		kustomizations []blueprintv1alpha1.Kustomization
		want           time.Duration
	}{
		{
			name:           "EmptyBlueprint",
			kustomizations: []blueprintv1alpha1.Kustomization{},
			want:           constants.DefaultKustomizationWaitTotalTimeout,
		},
		{
			name:           "SingleKustomizationNoDependencies",
			kustomizations: []blueprintv1alpha1.Kustomization{{Name: "k1"}},
			want:           constants.DefaultFluxKustomizationTimeout,
		},
		{
			name:           "SingleKustomizationWithCustomTimeout",
			kustomizations: []blueprintv1alpha1.Kustomization{kustomizationsPrivWithTimeout("k1", 10*time.Minute)},
			want:           10 * time.Minute,
		},
		{
			name:           "SingleKustomizationWithZeroTimeout",
			kustomizations: []blueprintv1alpha1.Kustomization{kustomizationsPrivWithTimeout("k1", 0)},
			want:           constants.DefaultFluxKustomizationTimeout,
		},
		{
			name: "TwoKustomizationsWithDependency",
			kustomizations: []blueprintv1alpha1.Kustomization{
				kustomizationsPrivWithTimeout("k1", 5*time.Minute),
				kustomizationsPrivWithTimeout("k2", 3*time.Minute, "k1"),
			},
			want: 8 * time.Minute,
		},
		{
			name: "TwoShortKustomizationsWithDependency",
			kustomizations: []blueprintv1alpha1.Kustomization{
				kustomizationsPrivWithTimeout("k1", 50*time.Millisecond),
				kustomizationsPrivWithTimeout("k2", 75*time.Millisecond, "k1"),
			},
			want: 125 * time.Millisecond,
		},
		{
			name: "LongDependencyChain",
			kustomizations: []blueprintv1alpha1.Kustomization{
				kustomizationsPrivWithTimeout("k1", 2*time.Minute),
				kustomizationsPrivWithTimeout("k2", 3*time.Minute, "k1"),
				kustomizationsPrivWithTimeout("k3", 4*time.Minute, "k2"),
				kustomizationsPrivWithTimeout("k4", 5*time.Minute, "k3"),
			},
			want: 14 * time.Minute,
		},
		{
			name: "MultipleIndependentChains",
			kustomizations: []blueprintv1alpha1.Kustomization{
				kustomizationsPrivWithTimeout("k1", 2*time.Minute),
				kustomizationsPrivWithTimeout("k2", 3*time.Minute, "k1"),
				kustomizationsPrivWithTimeout("k3", 10*time.Minute),
				kustomizationsPrivWithTimeout("k4", 1*time.Minute, "k3"),
			},
			want: 11 * time.Minute,
		},
		{
			name: "ForkedDependencyChain",
			kustomizations: []blueprintv1alpha1.Kustomization{
				kustomizationsPrivWithTimeout("k1", 2*time.Minute),
				kustomizationsPrivWithTimeout("k2", 3*time.Minute, "k1"),
				kustomizationsPrivWithTimeout("k3", 4*time.Minute, "k1"),
				kustomizationsPrivWithTimeout("k4", 5*time.Minute, "k2", "k3"),
			},
			want: 11 * time.Minute,
		},
		{
			name: "MixedDefaultAndCustomTimeouts",
			kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "k1"},
				kustomizationsPrivWithTimeout("k2", 7*time.Minute, "k1"),
			},
			want: constants.DefaultFluxKustomizationTimeout + 7*time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a manager and a blueprint with the case kustomizations
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: tt.kustomizations}

			// When the total wait time is calculated
			got := manager.calculateTotalWaitTime(blueprint)

			// Then it equals the longest chain total
			if got != tt.want {
				t.Errorf("Expected %v, got %v", tt.want, got)
			}
		})
	}
}
