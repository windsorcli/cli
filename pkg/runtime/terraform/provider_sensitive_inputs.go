// The ProviderSensitiveInputDiscovery is a terraform provider metadata component.
// It provides HCL parsing and cache-backed discovery of sensitive input declarations,
// The ProviderSensitiveInputDiscovery supports TerraformProvider env rendering decisions,
// and isolates sensitive variable introspection from broader provider orchestration flows.

package terraform

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// =============================================================================
// Types
// =============================================================================

type providerSensitiveInputCacheEntry struct {
	signature string
	inputs    map[string]bool
}

type defaultProviderSensitiveInputDiscovery struct {
	shims *Shims
	cache map[string]providerSensitiveInputCacheEntry
	mu    sync.RWMutex
}

// =============================================================================
// Interfaces
// =============================================================================

type providerSensitiveInputDiscovery interface {
	GetSensitiveTerraformInputs(modulePath string) (map[string]bool, error)
	ClearCache()
}

// =============================================================================
// Constructor
// =============================================================================

// newProviderSensitiveInputDiscovery creates a sensitive terraform input discovery service for TerraformProvider.
func newProviderSensitiveInputDiscovery(shims *Shims) providerSensitiveInputDiscovery {
	return &defaultProviderSensitiveInputDiscovery{
		shims: shims,
		cache: make(map[string]providerSensitiveInputCacheEntry),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetSensitiveTerraformInputs discovers terraform variable names declared with sensitive=true.
func (d *defaultProviderSensitiveInputDiscovery) GetSensitiveTerraformInputs(modulePath string) (map[string]bool, error) {
	inputs := make(map[string]bool)
	pattern := filepath.Join(modulePath, "*.tf")
	matches, err := d.shims.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to find terraform files in module %s: %w", modulePath, err)
	}
	sort.Strings(matches)
	signature, err := d.getSensitiveInputsSignature(matches)
	if err != nil {
		return nil, err
	}
	d.mu.RLock()
	cached, ok := d.cache[modulePath]
	d.mu.RUnlock()
	if ok && cached.signature == signature {
		return cloneProviderSensitiveInputMap(cached.inputs), nil
	}

	for _, tfFile := range matches {
		content, err := d.shims.ReadFile(tfFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read terraform file %s: %w", tfFile, err)
		}
		parsedFile, diags := d.shims.HclParseConfig(content, tfFile, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return nil, fmt.Errorf("failed to parse terraform file %s: %s", tfFile, strings.TrimSpace(diags.Error()))
		}
		for _, block := range parsedFile.Body().Blocks() {
			if block.Type() != "variable" || len(block.Labels()) == 0 {
				continue
			}
			attr := block.Body().GetAttribute("sensitive")
			if attr == nil {
				continue
			}
			exprBytes := attr.Expr().BuildTokens(nil).Bytes()
			parsedExpr, diags := hclsyntax.ParseExpression(exprBytes, "sensitive", hcl.Pos{Line: 1, Column: 1})
			if diags.HasErrors() {
				return nil, fmt.Errorf("failed to parse sensitive attribute in variable %s from %s: %s", block.Labels()[0], tfFile, strings.TrimSpace(diags.Error()))
			}
			val, diags := parsedExpr.Value(nil)
			if diags.HasErrors() {
				return nil, fmt.Errorf("failed to evaluate sensitive attribute in variable %s from %s: %s", block.Labels()[0], tfFile, strings.TrimSpace(diags.Error()))
			}
			if val.Type() != cty.Bool || !val.True() {
				continue
			}
			inputs[block.Labels()[0]] = true
		}
	}

	d.mu.Lock()
	d.cache[modulePath] = providerSensitiveInputCacheEntry{
		signature: signature,
		inputs:    cloneProviderSensitiveInputMap(inputs),
	}
	d.mu.Unlock()
	return inputs, nil
}

// ClearCache clears cached sensitive variable discovery results.
func (d *defaultProviderSensitiveInputDiscovery) ClearCache() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache = make(map[string]providerSensitiveInputCacheEntry)
}

// =============================================================================
// Private Methods
// =============================================================================

// getSensitiveInputsSignature computes a stable cache signature from terraform file metadata.
func (d *defaultProviderSensitiveInputDiscovery) getSensitiveInputsSignature(tfFiles []string) (string, error) {
	var signature strings.Builder
	for _, tfFile := range tfFiles {
		info, err := d.shims.Stat(tfFile)
		if err != nil {
			return "", fmt.Errorf("failed to stat terraform file %s: %w", tfFile, err)
		}
		signature.WriteString(tfFile)
		signature.WriteString("|")
		signature.WriteString(fmt.Sprintf("%d|%d|", info.Size(), info.ModTime().UnixNano()))
	}
	return signature.String(), nil
}

// =============================================================================
// Helpers
// =============================================================================

// cloneProviderSensitiveInputMap returns a deep copy of the provided sensitivity map.
func cloneProviderSensitiveInputMap(inputs map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(inputs))
	for key, value := range inputs {
		cloned[key] = value
	}
	return cloned
}
