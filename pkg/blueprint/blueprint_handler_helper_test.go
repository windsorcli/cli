package blueprint

import (
	"fmt"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// =============================================================================
// Test Helper Functions
// =============================================================================

func TestTLACode(t *testing.T) {
	// Given a mock Jsonnet VM that returns an error about missing authors
	vm := NewMockJsonnetVM(func(filename, snippet string) (string, error) {
		return "", fmt.Errorf("blueprint has no authors")
	})

	// When evaluating an empty snippet
	_, err := vm.EvaluateAnonymousSnippet("test.jsonnet", "")

	// Then an error about missing authors should be returned
	if err == nil || !strings.Contains(err.Error(), "blueprint has no authors") {
		t.Errorf("expected error containing 'blueprint has no authors', got %v", err)
	}
}

func TestBaseBlueprintHandler_calculateMaxWaitTime(t *testing.T) {
	t.Run("EmptyKustomizations", func(t *testing.T) {
		// Given a blueprint handler with no kustomizations
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return 0 since there are no kustomizations
		if waitTime != 0 {
			t.Errorf("expected 0 duration, got %v", waitTime)
		}
	})

	t.Run("SingleKustomization", func(t *testing.T) {
		// Given a blueprint handler with a single kustomization
		customTimeout := 2 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "test-kustomization",
						Timeout: &metav1.Duration{
							Duration: customTimeout,
						},
					},
				},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the kustomization's timeout
		if waitTime != customTimeout {
			t.Errorf("expected timeout %v, got %v", customTimeout, waitTime)
		}
	})

	t.Run("LinearDependencies", func(t *testing.T) {
		// Given a blueprint handler with linear dependencies
		timeout1 := 1 * time.Minute
		timeout2 := 2 * time.Minute
		timeout3 := 3 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Timeout: &metav1.Duration{
							Duration: timeout1,
						},
						DependsOn: []string{"kustomization-2"},
					},
					{
						Name: "kustomization-2",
						Timeout: &metav1.Duration{
							Duration: timeout2,
						},
						DependsOn: []string{"kustomization-3"},
					},
					{
						Name: "kustomization-3",
						Timeout: &metav1.Duration{
							Duration: timeout3,
						},
					},
				},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the sum of all timeouts
		expectedTime := timeout1 + timeout2 + timeout3
		if waitTime != expectedTime {
			t.Errorf("expected timeout %v, got %v", expectedTime, waitTime)
		}
	})

	t.Run("BranchingDependencies", func(t *testing.T) {
		// Given a blueprint handler with branching dependencies
		timeout1 := 1 * time.Minute
		timeout2 := 2 * time.Minute
		timeout3 := 3 * time.Minute
		timeout4 := 4 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Timeout: &metav1.Duration{
							Duration: timeout1,
						},
						DependsOn: []string{"kustomization-2", "kustomization-3"},
					},
					{
						Name: "kustomization-2",
						Timeout: &metav1.Duration{
							Duration: timeout2,
						},
						DependsOn: []string{"kustomization-4"},
					},
					{
						Name: "kustomization-3",
						Timeout: &metav1.Duration{
							Duration: timeout3,
						},
						DependsOn: []string{"kustomization-4"},
					},
					{
						Name: "kustomization-4",
						Timeout: &metav1.Duration{
							Duration: timeout4,
						},
					},
				},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the longest path (1 -> 3 -> 4)
		expectedTime := timeout1 + timeout3 + timeout4
		if waitTime != expectedTime {
			t.Errorf("expected timeout %v, got %v", expectedTime, waitTime)
		}
	})

	t.Run("CircularDependencies", func(t *testing.T) {
		// Given a blueprint handler with circular dependencies
		timeout1 := 1 * time.Minute
		timeout2 := 2 * time.Minute
		timeout3 := 3 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Timeout: &metav1.Duration{
							Duration: timeout1,
						},
						DependsOn: []string{"kustomization-2"},
					},
					{
						Name: "kustomization-2",
						Timeout: &metav1.Duration{
							Duration: timeout2,
						},
						DependsOn: []string{"kustomization-3"},
					},
					{
						Name: "kustomization-3",
						Timeout: &metav1.Duration{
							Duration: timeout3,
						},
						DependsOn: []string{"kustomization-1"},
					},
				},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the sum of all timeouts in the cycle (1+2+3+3)
		expectedTime := timeout1 + timeout2 + timeout3 + timeout3
		if waitTime != expectedTime {
			t.Errorf("expected timeout %v, got %v", expectedTime, waitTime)
		}
	})
}

func TestBaseBlueprintHandler_loadFileData(t *testing.T) {
	t.Run("func", func(t *testing.T) {
		// Test cases will go here
	})
}
