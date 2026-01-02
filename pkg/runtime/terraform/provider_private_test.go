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
)

func TestTerraformProvider_generateBackendConfigArgs(t *testing.T) {
	t.Run("GeneratesLocalBackendArgs", func(t *testing.T) {
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

		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

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

		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

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

	t.Run("GeneratesLocalBackendArgsForEnvVar", func(t *testing.T) {
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

		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(args) == 0 {
			t.Error("Expected args to be generated")
		}

		for _, arg := range args {
			if strings.HasPrefix(arg, "-backend-config=\"") {
				t.Errorf("Expected raw args without quotes, got %v", args)
				break
			}
		}
	})

	t.Run("IncludesBackendTfvars", func(t *testing.T) {
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

		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

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

		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

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

		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

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

		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

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

		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

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

		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

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

		args, err := provider.generateBackendConfigArgs("test/path", configRoot)

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

		_, err := provider.generateBackendConfigArgs("test/path", configRoot)

		if err == nil {
			t.Error("Expected error for unsupported backend")
		}
		if !strings.Contains(err.Error(), "unsupported backend") {
			t.Errorf("Expected error about unsupported backend, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenWindsorScratchPathFails", func(t *testing.T) {
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

		_, err := provider.generateBackendConfigArgs("test/path", configRoot)

		if err == nil {
			t.Error("Expected error when GetWindsorScratchPath fails")
		}
		if !strings.Contains(err.Error(), "windsor scratch path") {
			t.Errorf("Expected error about windsor scratch path, got: %v", err)
		}
	})

	t.Run("HandlesProcessBackendConfigError", func(t *testing.T) {
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

		_, err := provider.generateBackendConfigArgs("test/path", configRoot)

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
		result := sanitizeForK8s("Test-String_123")

		expected := "test-string-123"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("HandlesInvalidCharacters", func(t *testing.T) {
		result := sanitizeForK8s("Test@String#123")

		if strings.Contains(result, "@") || strings.Contains(result, "#") {
			t.Errorf("Expected no invalid characters, got %s", result)
		}
		if !strings.Contains(result, "test") {
			t.Errorf("Expected lowercase, got %s", result)
		}
	})

	t.Run("TrimsTo63Characters", func(t *testing.T) {
		longString := strings.Repeat("a", 100)
		result := sanitizeForK8s(longString)

		if len(result) > 63 {
			t.Errorf("Expected max 63 characters, got %d: %s", len(result), result)
		}
	})

	t.Run("HandlesMultipleUnderscores", func(t *testing.T) {
		result := sanitizeForK8s("test___string")

		if strings.Contains(result, "_") {
			t.Errorf("Expected underscores to be replaced, got %s", result)
		}
	})

	t.Run("HandlesMultipleDashes", func(t *testing.T) {
		result := sanitizeForK8s("test---string")

		if strings.Contains(result, "---") {
			t.Errorf("Expected multiple dashes to be collapsed, got %s", result)
		}
	})

	t.Run("TrimsLeadingTrailingDashes", func(t *testing.T) {
		result := sanitizeForK8s("-test-string-")

		if strings.HasPrefix(result, "-") || strings.HasSuffix(result, "-") {
			t.Errorf("Expected no leading/trailing dashes, got %s", result)
		}
	})
}

func Test_processMap(t *testing.T) {
	t.Run("ProcessesStringValues", func(t *testing.T) {
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"bucket": "my-bucket",
			"region": "us-east-1",
		}

		processMap("", configMap, addArg)

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
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"insecure": true,
			"secure":   false,
		}

		processMap("", configMap, addArg)

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
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"port":    8080,
			"timeout": uint64(30),
		}

		processMap("", configMap, addArg)

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
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"allowed_account_ids": []any{"123", "456"},
		}

		processMap("", configMap, addArg)

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
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"nested": map[string]any{
				"key": "value",
			},
		}

		processMap("", configMap, addArg)

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
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"key": "value",
		}

		processMap("prefix", configMap, addArg)

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
		args := []string{}
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		configMap := map[string]any{
			"mixed_array": []any{"string", 123, true},
		}

		processMap("", configMap, addArg)

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

func TestTerraformProvider_RestoreEnvVar(t *testing.T) {
	t.Run("RestoresEnvVarWithValue", func(t *testing.T) {
		mocks := setupMocks(t)
		provider := mocks.Provider

		var setKey, setValue string
		provider.Shims.Setenv = func(key, value string) error {
			setKey = key
			setValue = value
			return nil
		}

		provider.restoreEnvVar("TEST_VAR", "original-value")

		if setKey != "TEST_VAR" {
			t.Errorf("Expected to set TEST_VAR, got %s", setKey)
		}

		if setValue != "original-value" {
			t.Errorf("Expected to set original-value, got %s", setValue)
		}
	})

	t.Run("UnsetsEnvVarWhenEmpty", func(t *testing.T) {
		mocks := setupMocks(t)
		provider := mocks.Provider

		var unsetKey string
		provider.Shims.Unsetenv = func(key string) error {
			unsetKey = key
			return nil
		}

		provider.restoreEnvVar("TEST_VAR", "")

		if unsetKey != "TEST_VAR" {
			t.Errorf("Expected to unset TEST_VAR, got %s", unsetKey)
		}
	})
}
