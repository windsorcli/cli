package config

// The PersistencePolicy is a partitioning component for config persistence targets.
// It provides deterministic routing of merged config data into values and workstation maps,
// The PersistencePolicy centralizes conditional ownership rules for special keys like platform
// and the computed network.cidr_block, and removes policy-specific booleans from file source
// save method signatures.

// =============================================================================
// Types
// =============================================================================

// persistencePolicyInput provides runtime context for persistence ownership decisions.
type persistencePolicyInput struct {
	IsDevMode          bool
	WorkstationRuntime string
}

// persistencePartition contains split maps for each persistence destination.
type persistencePartition struct {
	Values      map[string]any
	Workstation map[string]any
}

// persistencePolicy partitions config data according to ownership rules.
type persistencePolicy struct{}

// =============================================================================
// Constructor
// =============================================================================

// newPersistencePolicy creates a persistencePolicy instance.
func newPersistencePolicy() *persistencePolicy {
	return &persistencePolicy{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Partition splits data into values and workstation persistence maps.
func (p *persistencePolicy) Partition(data map[string]any, input persistencePolicyInput) persistencePartition {
	values := make(map[string]any, len(data))
	workstation := make(map[string]any)

	persistPlatform := p.shouldPersistPlatform(input)
	persistNetwork := p.shouldPersistNetwork(input)
	for key, value := range data {
		if key == "provider" {
			continue
		}
		if p.isWorkstationManagedKey(key) {
			workstation[key] = value
			continue
		}
		if key == "platform" && persistPlatform {
			workstation[key] = value
			continue
		}
		if key == "network" && persistNetwork {
			if cidrBlock, ok := networkCIDRBlock(value); ok {
				workstation["network"] = map[string]any{"cidr_block": cidrBlock}
			}
		}
		values[key] = value
	}

	return persistencePartition{
		Values:      values,
		Workstation: workstation,
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// shouldPersistPlatform reports whether platform should be owned by workstation state.
func (p *persistencePolicy) shouldPersistPlatform(input persistencePolicyInput) bool {
	if input.IsDevMode {
		return true
	}

	return input.WorkstationRuntime != ""
}

// shouldPersistNetwork reports whether the computed network.cidr_block should be duplicated into
// workstation state. Same condition as shouldPersistPlatform today, kept as its own predicate so a
// future change to platform-persistence rules doesn't silently change network CIDR persistence too.
func (p *persistencePolicy) shouldPersistNetwork(input persistencePolicyInput) bool {
	if input.IsDevMode {
		return true
	}

	return input.WorkstationRuntime != ""
}

// isWorkstationManagedKey reports whether key is always workstation-managed.
func (p *persistencePolicy) isWorkstationManagedKey(key string) bool {
	for _, managedKey := range workstationStateManagedKeys {
		if key == managedKey {
			return true
		}
	}

	return false
}

// networkCIDRBlock extracts the cidr_block field from a network config map, if present.
func networkCIDRBlock(value any) (any, bool) {
	network, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	cidrBlock, ok := network["cidr_block"]
	if !ok {
		return nil, false
	}
	return cidrBlock, true
}
