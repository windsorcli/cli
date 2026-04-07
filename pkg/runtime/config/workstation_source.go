package config

import (
	"fmt"
	"path/filepath"
)

// The WorkstationSource is a configuration source for system-managed workstation state.
// It provides load, save, and delete operations for context-scoped workstation.yaml files,
// The WorkstationSource isolates ephemeral workstation persistence from user-authored values,
// and centralizes filtering/policy logic for workstation-managed keys and platform persistence.

// =============================================================================
// Types
// =============================================================================

// workstationStateManagedKeys lists top-level config keys that are system-managed via workstation.yaml.
var workstationStateManagedKeys = []string{"workstation"}

// workstationSource handles workstation state persistence and filtering rules.
type workstationSource struct {
	shims  *Shims
	policy *persistencePolicy
}

// =============================================================================
// Constructor
// =============================================================================

// newWorkstationSource creates a workstationSource with shims.
func newWorkstationSource(shims *Shims, policy *persistencePolicy) *workstationSource {
	if shims == nil {
		shims = NewShims()
	}
	if policy == nil {
		policy = newPersistencePolicy()
	}

	return &workstationSource{
		shims:  shims,
		policy: policy,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// StatePath returns the context-scoped workstation state file path.
func (s *workstationSource) StatePath(projectRoot, contextName string) string {
	return filepath.Join(projectRoot, ".windsor", "contexts", contextName, "workstation.yaml")
}

// Load loads workstation state for a context.
func (s *workstationSource) Load(projectRoot, contextName string) (map[string]any, bool, error) {
	statePath := s.StatePath(projectRoot, contextName)
	if _, err := s.shims.Stat(statePath); err != nil {
		return nil, false, nil
	}

	fileData, err := s.shims.ReadFile(statePath)
	if err != nil {
		return nil, false, fmt.Errorf("error reading workstation state: %w", err)
	}

	var wsState map[string]any
	if err := s.shims.YamlUnmarshal(fileData, &wsState); err != nil {
		return nil, false, fmt.Errorf("error unmarshalling workstation state: %w", err)
	}

	return wsState, true, nil
}

// Save writes workstation state for a context based on persistence policy.
func (s *workstationSource) Save(projectRoot, contextName string, data map[string]any, input persistencePolicyInput) error {
	statePath := s.StatePath(projectRoot, contextName)
	stateDir := filepath.Dir(statePath)
	if err := s.shims.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("error creating workstation state directory: %w", err)
	}

	partition := s.policy.Partition(data, input)
	if len(partition.Workstation) == 0 {
		return s.Delete(projectRoot, contextName)
	}

	marshaled, err := s.shims.YamlMarshal(partition.Workstation)
	if err != nil {
		return fmt.Errorf("error marshalling workstation state: %w", err)
	}
	if err := s.shims.WriteFile(statePath, marshaled, 0644); err != nil {
		return fmt.Errorf("error writing workstation state: %w", err)
	}

	return nil
}

// Delete removes workstation state for a context if present.
func (s *workstationSource) Delete(projectRoot, contextName string) error {
	statePath := s.StatePath(projectRoot, contextName)
	if _, err := s.shims.Stat(statePath); err == nil {
		if err := s.shims.RemoveAll(statePath); err != nil {
			return fmt.Errorf("error removing workstation state: %w", err)
		}
	}

	return nil
}
