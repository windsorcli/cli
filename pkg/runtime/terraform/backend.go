// The backend functions generate the Terraform backend configuration of a component.
// They write the backend override file and add it to the terraform/.gitignore file of the project.
// They convert the backend settings of the context into -backend-config arguments.
// BackendConfigComplete reports whether the backend has all the settings it needs.

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

// =============================================================================
// Public Methods
// =============================================================================

// GenerateBackendOverride creates or removes the backend_override.tf file for the specified directory
// based on the configured backend type. When it writes the file under <project>/terraform/, it also
// tries to ensure <project>/terraform/.gitignore ignores backend_override.tf; a failure there is a warning, not an error. This file is used to override Terraform backend configuration
// at runtime without modifying the original Terraform files. If the backend type is 'none', it removes
// the override file if it exists. Otherwise, it writes a backend_override.tf file with the appropriate
// backend stanza for local, s3, kubernetes, azurerm, or gcs backends. Returns an error for unsupported backend types.
func (p *terraformProvider) GenerateBackendOverride(directory string) error {
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

// BackendConfigComplete reports whether the configured remote backend has
// enough config — a backend.tfvars file or a populated nested config block —
// for `terraform init` to even attempt connecting. Local/empty backends are
// always complete. Lets Bootstrap's probe distinguish "we don't know where
// the remote is yet" (first-time bootstrap) from "we know where it is but
// can't reach it" (real warning).
//
// Default for unrecognized backend types is true: the predicate's job is to
// catch the well-understood first-time-bootstrap case for the backends
// Windsor knows about. For anything else (http, consul, remote, cos), we
// don't know what's required — let the probe run and surface real init
// failures via the warning path rather than silently disable detection.
func (p *terraformProvider) BackendConfigComplete() bool {
	backendType := p.configHandler.GetTerraformBackendType()
	if backendType == "" || backendType == "local" {
		return true
	}

	if _, configRoot, _, err := p.providerScope(); err == nil {
		for _, candidate := range []string{
			filepath.Join(configRoot, "backend.tfvars"),
			filepath.Join(configRoot, "terraform", "backend.tfvars"),
		} {
			if _, err := p.Shims.Stat(candidate); err == nil {
				return true
			}
		}
	}

	// Kubernetes uses the ambient kubeconfig; a nested config block is
	// optional. Reachable from here even when cfg.Backend is nil.
	if backendType == "kubernetes" {
		return true
	}

	cfg := p.configHandler.GetHeader().Terraform
	switch backendType {
	case "azurerm":
		return cfg != nil && cfg.Backend != nil && cfg.Backend.AzureRM != nil &&
			cfg.Backend.AzureRM.StorageAccountName != nil &&
			cfg.Backend.AzureRM.ContainerName != nil
	case "s3":
		return cfg != nil && cfg.Backend != nil && cfg.Backend.S3 != nil &&
			cfg.Backend.S3.Bucket != nil
	case "gcs":
		return cfg != nil && cfg.Backend != nil && cfg.Backend.GCS != nil &&
			cfg.Backend.GCS.Bucket != nil
	}

	return true
}

// =============================================================================
// Private Methods
// =============================================================================

// ensureBackendOverrideIgnored adds backend_override.tf to <project>/terraform/.gitignore, creating
// the file when missing and appending the entry when absent. It does nothing when directory is outside
// the project's terraform/ folder, because Windsor scratch folders under .windsor/ are already ignored.
func (p *terraformProvider) ensureBackendOverrideIgnored(directory string) error {
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

// generateBackendConfigArgs constructs the -backend-config CLI arguments for Terraform based on project configuration.
// This method determines the backend type from the configuration and assembles key-value argument pairs for supported
// backend types (local, s3, kubernetes, azurerm). It also detects the presence of backend.tfvars in the context root
// or a terraform/ fallback subdirectory, and includes a -backend-config pointing to that file if found.
// It passes every field of the backend's config block, read untyped, as one argument each.
// Returns raw CLI arguments without shell quoting; formatting for environment variables is handled by the calling context.
// Returns a slice of backend configuration arguments or an error if required configuration or paths are unavailable.
func (p *terraformProvider) generateBackendConfigArgs(projectPath, configRoot string) ([]string, error) {
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

	prefix := p.configHandler.GetString("terraform.backend.prefix", "")

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
		keyPath := fmt.Sprintf("%s%s", prefix, filepath.ToSlash(filepath.Join(projectPath, "terraform.tfstate")))
		addBackendConfigArg("key", keyPath)
		if backend, ok := p.configHandler.Get("terraform.backend.s3").(map[string]any); ok {
			if err := p.processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, werror.Wrap(err, "error processing S3 backend config")
			}
		}
	case "kubernetes":
		appendBackendTfvars()
		secretSuffix := projectPath
		if prefix != "" {
			secretSuffix = fmt.Sprintf("%s-%s", strings.ReplaceAll(prefix, "/", "-"), secretSuffix)
		}
		secretSuffix = sanitizeForK8s(secretSuffix)
		addBackendConfigArg("secret_suffix", secretSuffix)
		if backend, ok := p.configHandler.Get("terraform.backend.kubernetes").(map[string]any); ok {
			if err := p.processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, werror.Wrap(err, "error processing Kubernetes backend config")
			}
		}
	case "azurerm":
		appendBackendTfvars()
		keyPath := fmt.Sprintf("%s%s", prefix, filepath.ToSlash(filepath.Join(projectPath, "terraform.tfstate")))
		addBackendConfigArg("key", keyPath)
		if backend, ok := p.configHandler.Get("terraform.backend.azurerm").(map[string]any); ok {
			if err := p.processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, werror.Wrap(err, "error processing AzureRM backend config")
			}
		}
	case "gcs":
		appendBackendTfvars()
		prefixPath := fmt.Sprintf("%s%s", prefix, filepath.ToSlash(projectPath))
		addBackendConfigArg("prefix", prefixPath)
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

// processBackendConfig processes backend configuration and applies each key-value pair to the provided addArg function.
// It marshals the provided backendConfig to YAML, then unmarshals it into a map structure to normalize the format.
// It traverses the resulting map, applying each key-value pair to the addArg function. Nested configuration
// objects result in compound keys using dot notation (e.g., "s3.bucket" for a nested bucket field).
// Returns an error if marshalling, unmarshalling, or processing fails.
func (p *terraformProvider) processBackendConfig(backendConfig any, addArg func(key, value string)) error {
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
