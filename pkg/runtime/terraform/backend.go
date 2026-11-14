// The backend functions generate the Terraform backend configuration of a component.
// They write the backend override file and add it to the terraform/.gitignore file of the project.
// They convert the backend settings of the context into -backend-config arguments.

package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// backendOverrideFileName is the generated Terraform backend override file.
const backendOverrideFileName = "backend_override.tf"

// unsupportedBackendRemediation tells the operator which backend types are valid.
const unsupportedBackendRemediation = "Set terraform.backend.type to none, local, s3, kubernetes, azurerm, or gcs."

// autoPrefixRemediation tells the operator how to restore the context ID.
const autoPrefixRemediation = "Run windsor init to generate the context ID, or set id in the context config."

// kubernetesSecretSuffixLimit is the longest secret_suffix that the kubernetes backend accepts.
const kubernetesSecretSuffixLimit = 63

// =============================================================================
// Public Methods
// =============================================================================

// GenerateBackendOverride writes or removes the backend_override.tf file in directory for the configured backend.
// For the none backend, it removes the file. For local, s3, kubernetes, azurerm, and gcs, it writes the backend block.
// Under <project>/terraform/, it also adds the file to terraform/.gitignore. A failure there is only a warning.
// An unsupported backend type returns an error with the valid types as the hint.
func (p *DefaultTerraformProvider) GenerateBackendOverride(directory string) error {
	backend := p.configHandler.GetTerraformBackendType()

	var backendConfig string
	switch backend {
	case "none":
		backendOverridePath := filepath.Join(directory, "backend_override.tf")
		if _, err := p.Shims.Stat(backendOverridePath); err == nil {
			if err := p.Shims.Remove(backendOverridePath); err != nil {
				return werror.Wrap(err, "error removing backend_override.tf")
			}
		}
		return nil
	case "local":
		backendConfig = `terraform {
  backend "local" {}
}`
	case "s3":
		backendConfig = `terraform {
  backend "s3" {}
}`
	case "kubernetes":
		backendConfig = `terraform {
  backend "kubernetes" {}
}`
	case "azurerm":
		backendConfig = `terraform {
  backend "azurerm" {}
}`
	case "gcs":
		backendConfig = `terraform {
  backend "gcs" {}
}`
	default:
		return werror.New(fmt.Sprintf("unsupported backend: %s", backend), unsupportedBackendRemediation, nil)
	}

	backendOverridePath := filepath.Join(directory, "backend_override.tf")
	err := p.Shims.WriteFile(backendOverridePath, []byte(backendConfig), os.ModePerm)
	if err != nil {
		return werror.Wrap(err, "error writing backend_override.tf")
	}

	if err := p.ensureBackendOverrideIgnored(directory); err != nil {
		fmt.Fprintf(p.warningWriter, "Warning: %v\n", err)
	}
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// ensureBackendOverrideIgnored adds backend_override.tf to <project>/terraform/.gitignore, creating
// the file when missing and appending the entry when absent. It does nothing when directory is outside
// the project's terraform/ folder, because Windsor scratch folders under .windsor/ are already ignored.
func (p *DefaultTerraformProvider) ensureBackendOverrideIgnored(directory string) error {
	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return nil
	}
	terraformRoot := filepath.Join(projectRoot, "terraform")
	rel, err := filepath.Rel(terraformRoot, directory)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}

	ignorePath := filepath.Join(terraformRoot, ".gitignore")
	existing, err := p.Shims.ReadFile(ignorePath)
	if err != nil && !os.IsNotExist(err) {
		return werror.Wrap(err, "error reading terraform/.gitignore")
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == backendOverrideFileName {
			return nil
		}
	}

	content := string(existing)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += backendOverrideFileName + "\n"
	if err := p.Shims.WriteFile(ignorePath, []byte(content), 0644); err != nil {
		return werror.Wrap(err, "error writing terraform/.gitignore")
	}
	return nil
}

// generateBackendConfigArgs returns the -backend-config arguments for the configured backend.
// For a remote backend, it adds backend.tfvars from the context root or its terraform/ subdirectory, if the file exists.
// It adds one argument for each field of the backend config block. The arguments have no shell quoting.
// An unsupported backend type returns an error with the valid types as the hint.
func (p *DefaultTerraformProvider) generateBackendConfigArgs(projectPath, configRoot string) ([]string, error) {
	var backendConfigArgs []string
	backend := p.configHandler.GetTerraformBackendType()

	addBackendConfigArg := func(key, value string) {
		if value != "" {
			backendConfigArgs = append(backendConfigArgs, fmt.Sprintf("-backend-config=%s=%s", key, filepath.ToSlash(value)))
		}
	}

	appendBackendTfvars := func() {
		if contextName, _, _, _ := p.providerScope(); contextName == "" {
			return
		}
		backendTfvarsPath := filepath.Join(configRoot, "backend.tfvars")
		if _, err := p.Shims.Stat(backendTfvarsPath); err != nil {
			backendTfvarsPath = filepath.Join(configRoot, "terraform", "backend.tfvars")
			if _, err := p.Shims.Stat(backendTfvarsPath); err != nil {
				return
			}
		}
		absBackendTfvarsPath, err := filepath.Abs(backendTfvarsPath)
		if err != nil {
			return
		}
		backendConfigArgs = append(backendConfigArgs, fmt.Sprintf("-backend-config=%s", filepath.ToSlash(absBackendTfvarsPath)))
	}

	segment, err := p.autoPrefixSegment()
	if err != nil {
		return nil, err
	}
	keyField, keyValue, err := p.backendKey(backend, projectPath, segment)
	if err != nil {
		return nil, err
	}

	switch backend {
	case "none":
		return []string{}, nil
	case "local":
		statePath, err := p.GetStatePath(projectPath)
		if err != nil {
			return nil, werror.Wrap(err, "error getting state path")
		}
		addBackendConfigArg("path", filepath.ToSlash(statePath))
	case "s3":
		appendBackendTfvars()
		addBackendConfigArg(keyField, keyValue)
		if backend, ok := p.configHandler.Get("terraform.backend.s3").(map[string]any); ok {
			if err := p.processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, werror.Wrap(err, "error processing S3 backend config")
			}
		}
	case "kubernetes":
		appendBackendTfvars()
		addBackendConfigArg(keyField, keyValue)
		if backend, ok := p.configHandler.Get("terraform.backend.kubernetes").(map[string]any); ok {
			if err := p.processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, werror.Wrap(err, "error processing Kubernetes backend config")
			}
		}
	case "azurerm":
		appendBackendTfvars()
		addBackendConfigArg(keyField, keyValue)
		if backend, ok := p.configHandler.Get("terraform.backend.azurerm").(map[string]any); ok {
			if err := p.processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, werror.Wrap(err, "error processing AzureRM backend config")
			}
		}
	case "gcs":
		appendBackendTfvars()
		addBackendConfigArg(keyField, keyValue)
		if backend, ok := p.configHandler.Get("terraform.backend.gcs").(map[string]any); ok {
			if err := p.processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, werror.Wrap(err, "error processing GCS backend config")
			}
		}
	default:
		return nil, werror.New(fmt.Sprintf("unsupported backend: %s", backend), unsupportedBackendRemediation, nil)
	}

	return backendConfigArgs, nil
}

// autoPrefixSegment returns the context segment that terraform.backend.auto_prefix adds to a remote
// backend key, such as "staging-w8k2x9q/". It returns "" when the option is off or the backend is
// local or none. It returns an error with a remediation when the context name or ID is empty.
func (p *DefaultTerraformProvider) autoPrefixSegment() (string, error) {
	switch p.configHandler.GetTerraformBackendType() {
	case "s3", "kubernetes", "azurerm", "gcs":
	default:
		return "", nil
	}
	if !p.configHandler.GetBool("terraform.backend.auto_prefix", false) {
		return "", nil
	}
	contextName := strings.ReplaceAll(p.configHandler.GetContext(), "/", "-")
	id := p.configHandler.GetString("id", "")
	if contextName == "" || id == "" {
		return "", werror.New("terraform.backend.auto_prefix needs a context name and a context ID", autoPrefixRemediation, nil)
	}
	return contextName + "-" + id + "/", nil
}

// backendKey returns the backend-config field name and value that select the state of a component.
// The value is the terraform.backend.prefix, then segment, then the component path. The fields are
// key for s3 and azurerm, prefix for gcs, and secret_suffix for kubernetes. It returns empty strings
// for local and none. A kubernetes suffix longer than 63 characters with a segment returns an error.
func (p *DefaultTerraformProvider) backendKey(backend, projectPath, segment string) (string, string, error) {
	prefix := p.configHandler.GetString("terraform.backend.prefix", "")
	switch backend {
	case "s3", "azurerm":
		return "key", fmt.Sprintf("%s%s%s", prefix, segment, filepath.ToSlash(filepath.Join(projectPath, "terraform.tfstate"))), nil
	case "gcs":
		return "prefix", fmt.Sprintf("%s%s%s", prefix, segment, filepath.ToSlash(projectPath)), nil
	case "kubernetes":
		suffix := projectPath
		if segment != "" {
			suffix = fmt.Sprintf("%s-%s", strings.TrimSuffix(segment, "/"), suffix)
		}
		if prefix != "" {
			suffix = fmt.Sprintf("%s-%s", strings.ReplaceAll(prefix, "/", "-"), suffix)
		}
		if segment != "" {
			if full := cleanK8sName(suffix); len(full) > kubernetesSecretSuffixLimit {
				return "", "", werror.New(
					fmt.Sprintf("kubernetes secret suffix %q is longer than %d characters", full, kubernetesSecretSuffixLimit),
					"Shorten terraform.backend.prefix or the component path, or use a shorter context name.", nil)
			}
		}
		return "secret_suffix", sanitizeForK8s(suffix), nil
	}
	return "", "", nil
}

// legacyBackendKey returns the field and value of the backend key without the auto-prefix segment.
// It returns empty strings when terraform.backend.auto_prefix is off, because the key is then the legacy key.
func (p *DefaultTerraformProvider) legacyBackendKey(projectPath string) (string, string) {
	segment, err := p.autoPrefixSegment()
	if err != nil || segment == "" {
		return "", ""
	}
	field, value, err := p.backendKey(p.configHandler.GetTerraformBackendType(), projectPath, "")
	if err != nil {
		return "", ""
	}
	return field, value
}

// kubernetesBackendNamespace returns the configured namespace of the kubernetes backend.
// It returns "" when the backend type is not kubernetes or the config sets no namespace.
// Without a configured namespace, backend.tfvars or terraform's own default decides it.
func (p *DefaultTerraformProvider) kubernetesBackendNamespace() string {
	if p.configHandler.GetTerraformBackendType() != "kubernetes" {
		return ""
	}
	if block, ok := p.configHandler.Get("terraform.backend.kubernetes").(map[string]any); ok {
		if namespace, ok := block["namespace"].(string); ok {
			return namespace
		}
	}
	return ""
}

// processBackendConfig processes backend configuration and applies each key-value pair to the provided addArg function.
// It marshals the provided backendConfig to YAML, then unmarshals it into a map structure to normalize the format.
// It traverses the resulting map, applying each key-value pair to the addArg function. Nested configuration
// objects result in compound keys using dot notation (e.g., "s3.bucket" for a nested bucket field).
// Returns an error if marshalling, unmarshalling, or processing fails.
func (p *DefaultTerraformProvider) processBackendConfig(backendConfig any, addArg func(key, value string)) error {
	yamlData, err := p.Shims.YamlMarshal(backendConfig)
	if err != nil {
		return werror.Wrap(err, "error marshalling backend to YAML")
	}

	var configMap map[string]any
	if err := p.Shims.YamlUnmarshal(yamlData, &configMap); err != nil {
		return werror.Wrap(err, "error unmarshalling backend YAML")
	}

	processMap("", configMap, addArg)
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// processMap traverses the provided configMap, applying each key-value pair to the addArg function.
// Nested maps are handled recursively, forming compound keys with dot notation.
// Supports string, bool, int, uint64, []any (string slice only), and nested map[string]any types.
func processMap(prefix string, configMap map[string]any, addArg func(key, value string)) {
	keys := make([]string, 0, len(configMap))
	for key := range configMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		fullKey := key
		if prefix != "" {
			fullKey = fmt.Sprintf("%s.%s", prefix, key)
		}

		switch v := configMap[key].(type) {
		case string:
			addArg(fullKey, v)
		case bool:
			addArg(fullKey, fmt.Sprintf("%t", v))
		case int, uint64:
			addArg(fullKey, fmt.Sprintf("%d", v))
		case []any:
			for _, item := range v {
				if strItem, ok := item.(string); ok {
					addArg(fullKey, strItem)
				}
			}
		case map[string]any:
			processMap(fullKey, v, addArg)
		}
	}
}
