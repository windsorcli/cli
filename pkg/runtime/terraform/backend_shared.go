// The shared backend functions find contexts of one project that would write state to the same keys.
// They read the config of the other contexts and compare the backend target and the prefix.
// They make no network call.

package terraform

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Public Methods
// =============================================================================

// SharedBackendContexts returns warnings for contexts that share the backend of the active context
// and write the same state keys. It returns nothing when terraform.backend.auto_prefix is on, or the
// backend is local or none. A context matches when it has the same backend type, the same target
// (bucket, storage account and container, or namespace), the same prefix, and auto_prefix off.
// A target that comes from backend.tfvars or a facet is not visible here, so the check can miss a match.
func (p *DefaultTerraformProvider) SharedBackendContexts() []string {
	backend := p.configHandler.GetTerraformBackendType()
	if backend == "local" || backend == "none" || backend == "" || p.configHandler.GetBool("terraform.backend.auto_prefix", false) {
		return nil
	}
	target := backendTarget(p.configHandler, backend)
	if target == "" {
		return nil
	}
	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return nil
	}
	entries, err := p.Shims.ReadDir(filepath.Join(projectRoot, "contexts"))
	if err != nil {
		return nil
	}

	current := p.configHandler.GetContext()
	prefix := p.configHandler.GetString("terraform.backend.prefix", "")
	var warnings []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == current || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			continue
		}
		other := config.NewConfigHandler(p.shell)
		if err := other.LoadConfigForContext(name); err != nil {
			continue
		}
		if other.GetTerraformBackendType() != backend ||
			other.GetBool("terraform.backend.auto_prefix", false) ||
			other.GetString("terraform.backend.prefix", "") != prefix ||
			backendTarget(other, backend) != target {
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"Contexts %q and %q use the %s backend %q with the same state keys. Their state can overwrite each other. Set terraform.backend.auto_prefix to true in both contexts. The next windsor up moves the state of each context.",
			current, name, backend, target))
	}
	sort.Strings(warnings)
	return warnings
}

// =============================================================================
// Helpers
// =============================================================================

// backendTarget returns the name of the shared store of a remote backend: the bucket for s3 and gcs,
// the storage account and container for azurerm, and the namespace for kubernetes. It returns ""
// when the config does not set the target.
func backendTarget(handler config.ConfigHandler, backend string) string {
	block, _ := handler.Get("terraform.backend." + backend).(map[string]any)
	get := func(key string) string {
		value, _ := block[key].(string)
		return value
	}
	switch backend {
	case "s3", "gcs":
		return get("bucket")
	case "azurerm":
		if account := get("storage_account_name"); account != "" {
			return account + "/" + get("container_name")
		}
	case "kubernetes":
		return get("namespace")
	}
	return ""
}
