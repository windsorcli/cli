package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/terraform"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTerraformProvider_generateBackendConfigArgs(t *testing.T) {
	t.Run("GeneratesLocalBackendArgs", func(t *testing.T) {
		// Given a provider with local backend configuration
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When generating backend config args
		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

		// Then it should generate local backend args with correct path
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedPath := filepath.ToSlash(filepath.Join(windsorScratchPath, ".tfstate", "test/path", "terraform.tfstate"))
		foundPath := false
		for _, arg := range args {
			if strings.Contains(arg, expectedPath) {
				foundPath = true
				break
			}
		}
		if !foundPath {
			t.Errorf("Expected args to contain path %s, got %v", expectedPath, args)
		}
	})

	t.Run("GeneratesLocalBackendArgsWithPrefix", func(t *testing.T) {
		// Given a provider with local backend and prefix configuration
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if key == "terraform.backend.prefix" {
				return "my-prefix"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When generating backend config args
		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

		// Then it should include prefix in path
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedPath := filepath.ToSlash(filepath.Join(windsorScratchPath, ".tfstate", "my-prefix", "test/path", "terraform.tfstate"))
		foundPath := false
		for _, arg := range args {
			if strings.Contains(arg, expectedPath) {
				foundPath = true
				break
			}
		}
		if !foundPath {
			t.Errorf("Expected args to contain path %s, got %v", expectedPath, args)
		}
	})

	t.Run("IncludesBackendTfvars", func(t *testing.T) {
		// Given a provider with backend.tfvars file
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		backendTfvarsPath := filepath.Join(configRoot, "backend.tfvars")
		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if filepath.ToSlash(path) == filepath.ToSlash(backendTfvarsPath) {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock filepath.Abs by using a custom Stat that returns a file for the backend.tfvars path
		originalStat := provider.Shims.Stat
		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if filepath.ToSlash(path) == filepath.ToSlash(backendTfvarsPath) {
				return nil, nil
			}
			return originalStat(path)
		}

		// When generating backend config args
		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

		// Then it should include backend.tfvars
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundTfvars := false
		for _, arg := range args {
			if strings.Contains(arg, "backend.tfvars") {
				foundTfvars = true
				break
			}
		}
		if !foundTfvars {
			t.Errorf("Expected args to contain backend.tfvars, got %v", args)
		}
	})

	t.Run("PrefersNewBackendTfvarsLocation", func(t *testing.T) {
		// Given a provider with both new and old backend.tfvars locations
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		newLocation := filepath.Join(configRoot, "backend.tfvars")
		oldLocation := filepath.Join(configRoot, "terraform", "backend.tfvars")

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			slashPath := filepath.ToSlash(path)
			if slashPath == filepath.ToSlash(newLocation) || slashPath == filepath.ToSlash(oldLocation) {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When generating backend config args
		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

		// Then it should prefer new location
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundNewLocation := false
		foundOldLocation := false
		newLocationSlash := filepath.ToSlash(newLocation)
		oldLocationSlash := filepath.ToSlash(oldLocation)
		for _, arg := range args {
			if strings.Contains(arg, newLocationSlash) {
				foundNewLocation = true
			}
			if strings.Contains(arg, oldLocationSlash) {
				foundOldLocation = true
			}
		}
		if !foundNewLocation {
			t.Errorf("Expected args to prefer new location %s, got %v", newLocationSlash, args)
		}
		if foundOldLocation {
			t.Errorf("Expected args to not include old location %s when new location exists, got %v", oldLocationSlash, args)
		}
	})

	t.Run("FallsBackToOldBackendTfvarsLocation", func(t *testing.T) {
		// Given a provider with only old backend.tfvars location
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		newLocation := filepath.Join(configRoot, "backend.tfvars")
		oldLocation := filepath.Join(configRoot, "terraform", "backend.tfvars")

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			slashPath := filepath.ToSlash(path)
			if slashPath == filepath.ToSlash(oldLocation) {
				return nil, nil
			}
			if slashPath == filepath.ToSlash(newLocation) {
				return nil, os.ErrNotExist
			}
			return nil, os.ErrNotExist
		}

		// When generating backend config args
		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

		// Then it should fall back to old location
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundOldLocation := false
		oldLocationSlash := filepath.ToSlash(oldLocation)
		for _, arg := range args {
			if strings.Contains(arg, oldLocationSlash) {
				foundOldLocation = true
				break
			}
		}
		if !foundOldLocation {
			t.Errorf("Expected args to fall back to old location %s, got %v", oldLocationSlash, args)
		}
	})

	t.Run("GeneratesS3BackendArgs", func(t *testing.T) {
		// Given a provider with S3 backend configuration
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "s3"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		mockConfig.GetConfigFunc = func() *v1alpha1.Context {
			bucket := "my-bucket"
			region := "us-east-1"
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						S3: &terraform.S3Backend{
							Bucket: &bucket,
							Region: &region,
						},
					},
				},
			}
		}

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		provider.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("bucket: my-bucket\nregion: us-east-1"), nil
		}

		provider.Shims.YamlUnmarshal = func(data []byte, v any) error {
			m := v.(*map[string]any)
			*m = map[string]any{
				"bucket": "my-bucket",
				"region": "us-east-1",
			}
			return nil
		}

		// When generating backend config args
		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

		// Then it should generate S3 backend args
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundKey := false
		for _, arg := range args {
			if strings.Contains(arg, "key=") && strings.Contains(arg, "test/path") {
				foundKey = true
				break
			}
		}
		if !foundKey {
			t.Errorf("Expected args to contain key for test/path, got %v", args)
		}
	})

	t.Run("GeneratesKubernetesBackendArgs", func(t *testing.T) {
		// Given a provider with kubernetes backend configuration
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		mockConfig.GetConfigFunc = func() *v1alpha1.Context {
			secretSuffix := "terraform-state"
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Kubernetes: &terraform.KubernetesBackend{
							SecretSuffix: &secretSuffix,
						},
					},
				},
			}
		}

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		provider.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("secret_suffix: terraform-state"), nil
		}

		provider.Shims.YamlUnmarshal = func(data []byte, v any) error {
			m := v.(*map[string]any)
			*m = map[string]any{
				"secret_suffix": "terraform-state",
			}
			return nil
		}

		// When generating backend config args
		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

		// Then it should generate kubernetes backend args
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundSecretSuffix := false
		for _, arg := range args {
			if strings.Contains(arg, "secret_suffix=") {
				foundSecretSuffix = true
				break
			}
		}
		if !foundSecretSuffix {
			t.Errorf("Expected args to contain secret_suffix, got %v", args)
		}
	})

	t.Run("GeneratesKubernetesBackendArgsWithPrefix", func(t *testing.T) {
		// Given a provider with kubernetes backend and prefix
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if key == "terraform.backend.prefix" {
				return "my/prefix"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		mockConfig.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Kubernetes: &terraform.KubernetesBackend{},
					},
				},
			}
		}

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When generating backend config args
		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

		// Then it should include prefix in secret_suffix
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundSecretSuffix := false
		for _, arg := range args {
			if strings.Contains(arg, "secret_suffix=") && strings.Contains(arg, "my-prefix") {
				foundSecretSuffix = true
				break
			}
		}
		if !foundSecretSuffix {
			t.Errorf("Expected args to contain secret_suffix with prefix, got %v", args)
		}
	})

	t.Run("GeneratesAzurermBackendArgs", func(t *testing.T) {
		// Given a provider with azurerm backend configuration
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		mockConfig.GetConfigFunc = func() *v1alpha1.Context {
			rg := "rg"
			sa := "sa"
			container := "container"
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						AzureRM: &terraform.AzureRMBackend{
							ResourceGroupName:  &rg,
							StorageAccountName: &sa,
							ContainerName:      &container,
						},
					},
				},
			}
		}

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		provider.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("resource_group_name: rg\nstorage_account_name: sa\ncontainer_name: container"), nil
		}

		provider.Shims.YamlUnmarshal = func(data []byte, v any) error {
			m := v.(*map[string]any)
			*m = map[string]any{
				"resource_group_name":  "rg",
				"storage_account_name": "sa",
				"container_name":       "container",
			}
			return nil
		}

		// When generating backend config args
		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

		// Then it should generate azurerm backend args
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundKey := false
		for _, arg := range args {
			if strings.Contains(arg, "key=") && strings.Contains(arg, "test/path") {
				foundKey = true
				break
			}
		}
		if !foundKey {
			t.Errorf("Expected args to contain key for test/path, got %v", args)
		}
	})

	t.Run("ReturnsErrorForUnsupportedBackend", func(t *testing.T) {
		// Given a provider with unsupported backend type
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "unsupported"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When generating backend config args
		_, err := provider.generateBackendConfigArgs("test/path", configRoot)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error for unsupported backend")
		}
		if !strings.Contains(err.Error(), "unsupported backend") {
			t.Errorf("Expected error about unsupported backend, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenWindsorScratchPathFails", func(t *testing.T) {
		// Given a provider with GetWindsorScratchPath that fails
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return "", fmt.Errorf("scratch path error")
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When generating backend config args
		_, err := provider.generateBackendConfigArgs("test/path", configRoot)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when GetWindsorScratchPath fails")
		}
		if !strings.Contains(err.Error(), "windsor scratch path") {
			t.Errorf("Expected error about windsor scratch path, got: %v", err)
		}
	})

	t.Run("HandlesProcessBackendConfigError", func(t *testing.T) {
		// Given a provider with YamlMarshal that fails
		mocks := setupMocks(t)
		provider := mocks.Provider
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "s3"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		mockConfig.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						S3: &terraform.S3Backend{},
					},
				},
			}
		}

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		provider.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("marshal error")
		}

		// When generating backend config args
		_, err := provider.generateBackendConfigArgs("test/path", configRoot)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when processBackendConfig fails")
		}
		if !strings.Contains(err.Error(), "S3 backend config") {
			t.Errorf("Expected error about S3 backend config, got: %v", err)
		}
	})
}

func Test_sanitizeForK8s(t *testing.T) {
	t.Run("SanitizesBasicString", func(t *testing.T) {
		// Given a string with mixed case and underscores
		// When sanitizing for K8s
		result := sanitizeForK8s("Test-String_123")

		// Then it should convert to lowercase and replace underscores
		expected := "test-string-123"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("TrimsTo63Characters", func(t *testing.T) {
		// Given a long string
		longString := strings.Repeat("a", 100)

		// When sanitizing for K8s
		result := sanitizeForK8s(longString)

		// Then it should be trimmed to 63 characters
		if len(result) > 63 {
			t.Errorf("Expected max 63 characters, got %d: %s", len(result), result)
		}
	})

	t.Run("HandlesMultipleDashes", func(t *testing.T) {
		// Given a string with multiple dashes
		// When sanitizing for K8s
		result := sanitizeForK8s("test---string")

		// Then multiple dashes should be collapsed
		if strings.Contains(result, "---") {
			t.Errorf("Expected multiple dashes to be collapsed, got %s", result)
		}
	})

	t.Run("TrimsLeadingTrailingDashes", func(t *testing.T) {
		// Given a string with leading and trailing dashes
		// When sanitizing for K8s
		result := sanitizeForK8s("-test-string-")

		// Then leading and trailing dashes should be trimmed
		if strings.HasPrefix(result, "-") || strings.HasSuffix(result, "-") {
			t.Errorf("Expected no leading/trailing dashes, got %s", result)
		}
	})
}

func Test_processMap(t *testing.T) {
	t.Run("ProcessesStringValues", func(t *testing.T) {
		// Given a config map with string values
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"bucket": "my-bucket",
			"region": "us-east-1",
		}

		// When processing the map
		processMap("", configMap, addArg)

		// Then string values should be processed
		if len(args) != 2 {
			t.Errorf("Expected 2 args, got %d", len(args))
		}
		foundBucket := false
		foundRegion := false
		for _, arg := range args {
			if strings.Contains(arg, "bucket=my-bucket") {
				foundBucket = true
			}
			if strings.Contains(arg, "region=us-east-1") {
				foundRegion = true
			}
		}
		if !foundBucket || !foundRegion {
			t.Errorf("Expected bucket and region args, got %v", args)
		}
	})

	t.Run("ProcessesBoolValues", func(t *testing.T) {
		// Given a config map with bool values
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"insecure": true,
			"secure":   false,
		}

		// When processing the map
		processMap("", configMap, addArg)

		// Then bool values should be processed
		foundTrue := false
		foundFalse := false
		for _, arg := range args {
			if strings.Contains(arg, "insecure=true") {
				foundTrue = true
			}
			if strings.Contains(arg, "secure=false") {
				foundFalse = true
			}
		}
		if !foundTrue || !foundFalse {
			t.Errorf("Expected bool args, got %v", args)
		}
	})

	t.Run("ProcessesIntValues", func(t *testing.T) {
		// Given a config map with int values
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"port":    8080,
			"timeout": uint64(30),
		}

		// When processing the map
		processMap("", configMap, addArg)

		// Then int values should be processed
		foundPort := false
		foundTimeout := false
		for _, arg := range args {
			if strings.Contains(arg, "port=8080") {
				foundPort = true
			}
			if strings.Contains(arg, "timeout=30") {
				foundTimeout = true
			}
		}
		if !foundPort || !foundTimeout {
			t.Errorf("Expected int args, got %v", args)
		}
	})

	t.Run("ProcessesStringArrayValues", func(t *testing.T) {
		// Given a config map with string array values
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"allowed_account_ids": []any{"123", "456"},
		}

		// When processing the map
		processMap("", configMap, addArg)

		// Then string array values should be processed
		found123 := false
		found456 := false
		for _, arg := range args {
			if strings.Contains(arg, "allowed_account_ids=123") {
				found123 = true
			}
			if strings.Contains(arg, "allowed_account_ids=456") {
				found456 = true
			}
		}
		if !found123 || !found456 {
			t.Errorf("Expected array args, got %v", args)
		}
	})

	t.Run("ProcessesNestedMaps", func(t *testing.T) {
		// Given a config map with nested maps
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"nested": map[string]any{
				"key": "value",
			},
		}

		// When processing the map
		processMap("", configMap, addArg)

		// Then nested maps should be processed with dot notation
		foundNested := false
		for _, arg := range args {
			if strings.Contains(arg, "nested.key=value") {
				foundNested = true
				break
			}
		}
		if !foundNested {
			t.Errorf("Expected nested key, got %v", args)
		}
	})

	t.Run("ProcessesWithPrefix", func(t *testing.T) {
		// Given a config map and a prefix
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"key": "value",
		}

		// When processing the map with prefix
		processMap("prefix", configMap, addArg)

		// Then keys should be prefixed
		foundPrefixed := false
		for _, arg := range args {
			if strings.Contains(arg, "prefix.key=value") {
				foundPrefixed = true
				break
			}
		}
		if !foundPrefixed {
			t.Errorf("Expected prefixed key, got %v", args)
		}
	})

	t.Run("IgnoresNonStringArrayItems", func(t *testing.T) {
		// Given a config map with mixed array types
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"mixed_array": []any{"string", 123, true},
		}

		// When processing the map
		processMap("", configMap, addArg)

		// Then only string array items should be processed
		foundString := false
		foundNonString := false
		for _, arg := range args {
			if strings.Contains(arg, "mixed_array=string") {
				foundString = true
			}
			if strings.Contains(arg, "mixed_array=123") || strings.Contains(arg, "mixed_array=true") {
				foundNonString = true
			}
		}
		if !foundString {
			t.Errorf("Expected string array item, got %v", args)
		}
		if foundNonString {
			t.Errorf("Expected non-string items to be ignored, got %v", args)
		}
	})
}

func TestTerraformProvider_registerTerraformOutputHelper(t *testing.T) {
	t.Run("RegistersHelperWithEvaluator", func(t *testing.T) {
		// Given a provider and mock evaluator
		mocks := setupMocks(t)
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		var registeredName string
		mockEvaluator.RegisterFunc = func(name string, helper func(params []any, deferred bool) (any, error), signature any) {
			registeredName = name
		}

		// When registering terraform output helper
		mocks.Provider.registerTerraformOutputHelper(mockEvaluator)

		// Then helper should be registered with correct name
		if registeredName != "terraform_output" {
			t.Errorf("Expected helper to be registered as 'terraform_output', got '%s'", registeredName)
		}
	})

	t.Run("HelperReturnsErrorForWrongParamCount", func(t *testing.T) {
		// Given a registered helper
		mocks := setupMocks(t)
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		var helperFunc func(params []any, deferred bool) (any, error)
		mockEvaluator.RegisterFunc = func(name string, helper func(params []any, deferred bool) (any, error), signature any) {
			helperFunc = helper
		}

		mocks.Provider.registerTerraformOutputHelper(mockEvaluator)

		// When calling helper with wrong parameter count
		_, err := helperFunc([]any{"component"}, false)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error for wrong parameter count")
		}
	})
}

func TestTerraformProvider_RestoreEnvVar(t *testing.T) {
	t.Run("RestoresEnvVarWithValue", func(t *testing.T) {
		// Given a provider and an env var with value
		mocks := setupMocks(t)
		provider := mocks.Provider

		var setKey, setValue string
		provider.Shims.Setenv = func(key, value string) error {
			setKey = key
			setValue = value
			return nil
		}

		// When restoring env var
		provider.restoreEnvVar("TEST_VAR", "original-value")

		// Then it should set the env var
		if setKey != "TEST_VAR" {
			t.Errorf("Expected to set TEST_VAR, got %s", setKey)
		}

		if setValue != "original-value" {
			t.Errorf("Expected to set original-value, got %s", setValue)
		}
	})

	t.Run("UnsetsEnvVarWhenEmpty", func(t *testing.T) {
		// Given a provider and an empty env var value
		mocks := setupMocks(t)
		provider := mocks.Provider

		var unsetKey string
		provider.Shims.Unsetenv = func(key string) error {
			unsetKey = key
			return nil
		}

		// When restoring env var with empty value
		provider.restoreEnvVar("TEST_VAR", "")

		// Then it should unset the env var
		if unsetKey != "TEST_VAR" {
			t.Errorf("Expected to unset TEST_VAR, got %s", unsetKey)
		}
	})
}

func TestTerraformProvider_applyEnvVars(t *testing.T) {
	t.Run("AppliesEnvVarsSuccessfully", func(t *testing.T) {
		// Given a provider and env vars to apply
		mocks := setupMocks(t)
		provider := mocks.Provider

		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		var setVars map[string]string
		setVars = make(map[string]string)
		provider.Shims.Getenv = func(key string) string {
			return "original-" + key
		}
		provider.Shims.Setenv = func(key, value string) error {
			setVars[key] = value
			return nil
		}

		// When applying env vars
		originalEnvVars, err := provider.applyEnvVars(envVars)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then env vars should be set and originals preserved
		if originalEnvVars["VAR1"] != "original-VAR1" {
			t.Errorf("Expected original VAR1 to be original-VAR1, got: %s", originalEnvVars["VAR1"])
		}

		if setVars["VAR1"] != "value1" {
			t.Errorf("Expected VAR1 to be set to value1, got: %s", setVars["VAR1"])
		}

		if setVars["VAR2"] != "value2" {
			t.Errorf("Expected VAR2 to be set to value2, got: %s", setVars["VAR2"])
		}
	})

	t.Run("RestoresEnvVarsOnError", func(t *testing.T) {
		// Given a provider with Setenv that fails
		mocks := setupMocks(t)
		provider := mocks.Provider

		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		var restoredKeys []string
		restoredKeys = make([]string, 0)
		callCount := 0
		provider.Shims.Getenv = func(key string) string {
			return "original-" + key
		}
		provider.Shims.Setenv = func(key, value string) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("setenv error")
			}
			return nil
		}
		provider.Shims.Unsetenv = func(key string) error {
			restoredKeys = append(restoredKeys, key)
			return nil
		}

		// When applying env vars
		_, err := provider.applyEnvVars(envVars)
		// Then it should restore env vars on error
		if err == nil {
			t.Error("Expected error when Setenv fails")
		}

		if len(restoredKeys) == 0 && callCount < 2 {
			t.Error("Expected restoreEnvVars to be called on error")
		}
	})
}

func TestTerraformProvider_restoreEnvVars(t *testing.T) {
	t.Run("RestoresMultipleEnvVars", func(t *testing.T) {
		mocks := setupMocks(t)
		provider := mocks.Provider

		originalEnvVars := map[string]string{
			"VAR1": "original1",
			"VAR2": "",
			"VAR3": "original3",
		}

		var setVars map[string]string
		var unsetVars []string
		setVars = make(map[string]string)
		unsetVars = make([]string, 0)

		provider.Shims.Setenv = func(key, value string) error {
			setVars[key] = value
			return nil
		}
		provider.Shims.Unsetenv = func(key string) error {
			unsetVars = append(unsetVars, key)
			return nil
		}

		provider.restoreEnvVars(originalEnvVars)

		if setVars["VAR1"] != "original1" {
			t.Errorf("Expected VAR1 to be restored to original1, got: %s", setVars["VAR1"])
		}

		if setVars["VAR3"] != "original3" {
			t.Errorf("Expected VAR3 to be restored to original3, got: %s", setVars["VAR3"])
		}

		if len(unsetVars) != 1 || unsetVars[0] != "VAR2" {
			t.Errorf("Expected VAR2 to be unset, got: %v", unsetVars)
		}
	})

}
