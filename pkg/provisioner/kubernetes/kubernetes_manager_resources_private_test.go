package kubernetes

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// =============================================================================
// Test Helpers
// =============================================================================

func TestValidateFields(t *testing.T) {
	tests := []struct {
		name    string
		object  map[string]any
		wantErr string
	}{
		{
			name:    "MissingSpec",
			object:  map[string]any{"apiVersion": "v1", "kind": "Deployment", "metadata": map[string]any{"name": "foo"}},
			wantErr: "spec is required",
		},
		{
			name:    "MissingMetadataName",
			object:  map[string]any{"apiVersion": "v1", "kind": "Deployment", "metadata": map[string]any{}, "spec": map[string]any{}},
			wantErr: "metadata.name is required",
		},
		{
			name:    "ConfigMapMissingData",
			object:  map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "foo"}},
			wantErr: "data is required for ConfigMap",
		},
		{
			name:    "ConfigMapDataNil",
			object:  map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "foo"}, "data": nil},
			wantErr: "data cannot be nil for ConfigMap",
		},
		{
			name:    "ConfigMapDataEmptyAnyMap",
			object:  map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "foo"}, "data": map[string]any{}},
			wantErr: "data cannot be empty for ConfigMap",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given an object that breaks one validation rule
			obj := &unstructured.Unstructured{Object: tt.object}

			// When the fields are validated
			err := validateFields(obj)

			// Then the error names the rule
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
