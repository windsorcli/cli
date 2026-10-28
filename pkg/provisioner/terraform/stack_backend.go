// The backend functions of the TerraformStack handle a change of the kubernetes backend namespace.
// A write operation moves the state of a component with terraform init -migrate-state.
// A plan reads the state from the namespace in the backend pointer and writes nothing to the cluster.

package terraform

import (
	"encoding/json"
	"slices"
	"strings"

	envvars "github.com/windsorcli/cli/pkg/runtime/env"
)

// =============================================================================
// Private Methods
// =============================================================================

// kubernetesNamespaceChange returns the namespace in the backend pointer of a component when it
// differs from the configured kubernetes backend namespace. The bool is false when the backend is
// not kubernetes, the config sets no namespace, the pointer is missing or names another backend
// type, or the namespaces match.
// A pointer with no namespace means "default", because terraform uses that namespace then.
func (s *TerraformStack) kubernetesNamespaceChange(terraformVars map[string]string, terraformArgs *envvars.TerraformArgs) (string, bool) {
	if terraformArgs == nil || terraformArgs.KubernetesNamespace == "" {
		return "", false
	}
	pointerPath := s.backendPointerPath(terraformVars)
	if pointerPath == "" {
		return "", false
	}
	data, err := s.shims.ReadFile(pointerPath)
	if err != nil {
		return "", false
	}
	var pointer struct {
		Backend struct {
			Type   string `json:"type"`
			Config struct {
				Namespace *string `json:"namespace"`
			} `json:"config"`
		} `json:"backend"`
	}
	if err := json.Unmarshal(data, &pointer); err != nil || pointer.Backend.Type != "kubernetes" {
		return "", false
	}
	previous := "default"
	if pointer.Backend.Config.Namespace != nil && *pointer.Backend.Config.Namespace != "" {
		previous = *pointer.Backend.Config.Namespace
	}
	if previous == terraformArgs.KubernetesNamespace {
		return "", false
	}
	return previous, true
}

// =============================================================================
// Helpers
// =============================================================================

// withBackendNamespace returns a copy of initArgs with the namespace backend-config argument set to namespace.
func withBackendNamespace(initArgs []string, namespace string) []string {
	out := slices.DeleteFunc(slices.Clone(initArgs), func(arg string) bool {
		return strings.HasPrefix(arg, "-backend-config=namespace=")
	})
	return append(out, "-backend-config=namespace="+namespace)
}
