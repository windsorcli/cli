// The state functions find the local Terraform state of the components.
// GetStatePath returns the state file path of one component.
// ListLocalStateComponentIDs walks the state directory of the context and returns the component IDs.
// The functions use the context name and the scratch path of the project.

package terraform

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Public Methods
// =============================================================================

// GetStatePath returns the path to the Terraform state file for the specified component ID.
// The state file is stored in .tfstate/<componentID>/terraform.tfstate within the Windsor scratch path.
// If a backend prefix is configured, it is included in the path to support multi-tenant or
// multi-environment deployments where state files need to be namespaced.
func (p *terraformProvider) GetStatePath(componentID string) (string, error) {
	_, _, windsorScratchPath, err := p.providerScope()
	if err != nil {
		return "", werror.Wrap(err, "error resolving provider scope")
	}

	component := p.GetTerraformComponent(componentID)
	actualComponentID := componentID
	if component != nil {
		actualComponentID = component.GetID()
	}

	prefix := p.configHandler.GetString("terraform.backend.prefix", "")
	path := filepath.Join(windsorScratchPath, ".tfstate")
	if prefix != "" {
		path = filepath.Join(path, prefix)
	}
	statePath := filepath.Join(path, actualComponentID, "terraform.tfstate")
	return statePath, nil
}

// ListLocalStateComponentIDs returns every componentID with a local Terraform state file on
// disk, whether or not that componentID is still declared in the active blueprint. Used by the
// bootstrap recovery sweep to find state stranded under a component's previous ID after a
// rename — recoverHalfMigratedComponents only iterates the current blueprint's componentIDs, so
// a renamed component's old state is otherwise invisible to it. Walks the same directory tree
// GetStatePath writes into, so the two stay in sync by construction. A missing .tfstate root is
// not an error: it just means nothing has ever applied locally.
func (p *terraformProvider) ListLocalStateComponentIDs() ([]string, error) {
	_, _, windsorScratchPath, err := p.providerScope()
	if err != nil {
		return nil, werror.Wrap(err, "error resolving provider scope")
	}

	root := filepath.Join(windsorScratchPath, ".tfstate")
	if prefix := p.configHandler.GetString("terraform.backend.prefix", ""); prefix != "" {
		root = filepath.Join(root, prefix)
	}

	var ids []string
	if err := p.walkLocalStateDir(root, "", &ids); err != nil {
		return nil, err
	}
	sort.Strings(ids)
	return ids, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// walkLocalStateDir recursively visits dir, collecting one componentID (its path relative to
// the .tfstate root, using "/" as the separator) per directory that directly contains a
// terraform.tfstate file. componentIDs may nest — a component's Path can itself contain
// slashes — so this walks the full tree rather than assuming a flat, one-level layout.
func (p *terraformProvider) walkLocalStateDir(dir, rel string, ids *[]string) error {
	entries, err := p.Shims.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return werror.Wrap(err, "error reading local state directory %s", dir)
	}

	hasState := false
	for _, entry := range entries {
		if !entry.IsDir() {
			if entry.Name() == "terraform.tfstate" {
				hasState = true
			}
			continue
		}
		childRel := entry.Name()
		if rel != "" {
			childRel = rel + "/" + entry.Name()
		}
		if err := p.walkLocalStateDir(filepath.Join(dir, entry.Name()), childRel, ids); err != nil {
			return err
		}
	}
	if hasState && rel != "" {
		*ids = append(*ids, rel)
	}
	return nil
}

// providerScope resolves the active context, config root, and Windsor scratch path.
// configHandler.GetContext() caches itself, so this needs no cache of its own.
func (p *terraformProvider) providerScope() (contextName, configRoot, windsorScratchPath string, err error) {
	contextName = p.configHandler.GetContext()

	configRoot, err = p.configHandler.GetConfigRoot()
	if err != nil {
		return contextName, "", "", err
	}

	windsorScratchPath, err = p.configHandler.GetWindsorScratchPath()
	return contextName, configRoot, windsorScratchPath, err
}
