package generators

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Mock Types
// =============================================================================

// setupKustomizeGeneratorMocks sets up a KustomizeGenerator and Mocks for testing
func setupKustomizeGeneratorMocks(t *testing.T) (*KustomizeGenerator, *Mocks) {
	mocks := setupMocks(t)
	generator := NewKustomizeGenerator(mocks.Injector)
	generator.shims = mocks.Shims
	return generator, mocks
}

// =============================================================================
// Constructor Tests
// =============================================================================

func TestNewKustomizeGenerator(t *testing.T) {
	// Given an injector
	injector := di.NewInjector()

	// When creating a new KustomizeGenerator
	generator := NewKustomizeGenerator(injector)

	// Then it should be properly initialized
	if generator == nil {
		t.Fatal("expected generator to be created")
	}
	if generator.injector != injector {
		t.Error("expected injector to be set")
	}
}

// =============================================================================
// Initialize Tests
// =============================================================================

func TestKustomizeGenerator_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		generator, _ := setupKustomizeGeneratorMocks(t)
		// Should succeed with default blueprint handler
		err := generator.Initialize()
		if err != nil {
			t.Fatalf("expected Initialize to succeed, got: %v", err)
		}
		if generator.blueprintHandler == nil {
			t.Error("expected blueprint handler to be set")
		}
	})

	t.Run("MissingBlueprintHandler", func(t *testing.T) {
		// Create injector without blueprint handler but with config handler
		injector := di.NewInjector()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		injector.Register("configHandler", configHandler)
		generator := NewKustomizeGenerator(injector)
		err := generator.Initialize()
		if err == nil {
			t.Fatal("expected Initialize to fail")
		}
		if !strings.Contains(err.Error(), "failed to resolve blueprint handler") {
			t.Errorf("expected error about blueprint handler, got: %v", err)
		}
	})

	t.Run("InvalidBlueprintHandlerType", func(t *testing.T) {
		// Create injector with wrong type but with config handler
		injector := di.NewInjector()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		injector.Register("configHandler", configHandler)
		injector.Register("blueprintHandler", "not a handler")
		generator := NewKustomizeGenerator(injector)
		err := generator.Initialize()
		if err == nil {
			t.Fatal("expected Initialize to fail")
		}
		if !strings.Contains(err.Error(), "failed to resolve blueprint handler") {
			t.Errorf("expected error about blueprint handler, got: %v", err)
		}
	})
}

// =============================================================================
// Generate Tests
// =============================================================================

func TestKustomizeGenerator_Generate(t *testing.T) {
	t.Run("NilData", func(t *testing.T) {
		generator, _ := setupKustomizeGeneratorMocks(t)
		_ = generator.Initialize()
		err := generator.Generate(nil)
		if err == nil {
			t.Fatal("expected Generate to fail with nil data")
		}
		if !strings.Contains(err.Error(), "data cannot be nil") {
			t.Errorf("expected error about nil data, got: %v", err)
		}
	})

	t.Run("EmptyData", func(t *testing.T) {
		generator, _ := setupKustomizeGeneratorMocks(t)
		_ = generator.Initialize()
		data := map[string]any{}
		err := generator.Generate(data)
		if err != nil {
			t.Fatalf("expected Generate to succeed with empty data, got: %v", err)
		}
	})

	t.Run("KustomizeData", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)
		_ = generator.Initialize()

		// Mock config handler
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// Mock shims for file operations
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: filepath.Base(name), isDir: false}, nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}

		data := map[string]any{
			"kustomize/test-patch": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
		}

		err := generator.Generate(data)
		if err != nil {
			t.Fatalf("expected Generate to succeed with kustomize data, got: %v", err)
		}
	})

	t.Run("ValuesData", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)
		_ = generator.Initialize()

		// Mock config handler
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// Mock shims for file operations
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: filepath.Base(name), isDir: false}, nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}

		data := map[string]any{
			"values/global": map[string]any{
				"domain":  "example.com",
				"port":    80,
				"enabled": true,
			},
		}

		err := generator.Generate(data)
		if err != nil {
			t.Fatalf("expected Generate to succeed with values data, got: %v", err)
		}
	})

	t.Run("ConfigRootError", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)
		_ = generator.Initialize()

		// Mock config handler to fail
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}

		data := map[string]any{"kustomize/test": "value"}
		err := generator.Generate(data)
		if err == nil {
			t.Fatal("expected Generate to fail with config root error")
		}
		if !strings.Contains(err.Error(), "failed to get config root") {
			t.Errorf("expected error about config root, got: %v", err)
		}
	})
}

func TestKustomizeGenerator_generatePatchFile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock config handler
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// Mock shims
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: filepath.Base(name), isDir: false}, nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}

		key := "kustomize/test-patch"
		values := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}

		err := generator.generatePatchFile(key, values, "/test/config", false)
		if err != nil {
			t.Fatalf("expected generatePatchFile to succeed, got: %v", err)
		}
	})

	t.Run("InvalidKustomizationName", func(t *testing.T) {
		generator, _ := setupKustomizeGeneratorMocks(t)

		key := "kustomize/invalid@name"
		values := map[string]any{"test": "value"}

		err := generator.generatePatchFile(key, values, "/test/config", false)
		if err == nil {
			t.Fatal("expected generatePatchFile to fail with invalid name")
		}
		if !strings.Contains(err.Error(), "invalid kustomization name") {
			t.Errorf("expected error about invalid name, got: %v", err)
		}
	})

	t.Run("PathValidationError", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock config handler
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// Mock shims to fail path validation
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, fmt.Errorf("path error")
		}

		key := "kustomize/test-patch"
		values := map[string]any{"test": "value"}

		err := generator.generatePatchFile(key, values, "/test/config", false)
		if err == nil {
			t.Fatal("expected generatePatchFile to fail with path error")
		}
	})
}

func TestKustomizeGenerator_generateValuesFile(t *testing.T) {
	t.Run("SuccessGlobal", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock config handler
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// Mock shims
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: filepath.Base(name), isDir: false}, nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}

		key := "values/global"
		values := map[string]any{
			"domain":  "example.com",
			"port":    80,
			"enabled": true,
		}

		err := generator.generateValuesFile(key, values, "/test/config", false)
		if err != nil {
			t.Fatalf("expected generateValuesFile to succeed, got: %v", err)
		}
	})

	t.Run("SuccessComponent", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock config handler
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// Mock shims
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: filepath.Base(name), isDir: false}, nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}

		key := "values/ingress"
		values := map[string]any{
			"host": "example.com",
			"tls":  true,
		}

		err := generator.generateValuesFile(key, values, "/test/config", false)
		if err != nil {
			t.Fatalf("expected generateValuesFile to succeed, got: %v", err)
		}
	})

	t.Run("InvalidValuesName", func(t *testing.T) {
		generator, _ := setupKustomizeGeneratorMocks(t)

		key := "values/invalid@name"
		values := map[string]any{"test": "value"}

		err := generator.generateValuesFile(key, values, "/test/config", false)
		if err == nil {
			t.Fatal("expected generateValuesFile to fail with invalid name")
		}
		if !strings.Contains(err.Error(), "invalid values name") {
			t.Errorf("expected error about invalid name, got: %v", err)
		}
	})

	t.Run("InvalidValuesType", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock config handler
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		key := "values/global"
		values := "not a map"

		err := generator.generateValuesFile(key, values, "/test/config", false)
		if err == nil {
			t.Fatal("expected generateValuesFile to fail with invalid type")
		}
		if !strings.Contains(err.Error(), "must be a map") {
			t.Errorf("expected error about invalid type, got: %v", err)
		}
	})

	t.Run("InvalidValuesContent", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock config handler
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		key := "values/global"
		values := map[string]any{
			"valid":   "string",
			"invalid": map[string]any{"nested": "value"},
		}

		err := generator.generateValuesFile(key, values, "/test/config", false)
		if err == nil {
			t.Fatal("expected generateValuesFile to fail with invalid values")
		}
		if !strings.Contains(err.Error(), "complex types") {
			t.Errorf("expected error about complex types, got: %v", err)
		}
	})
}

func TestKustomizeGenerator_generatePatchFiles(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock shims
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}

		patchPath := "/test/patch.yaml"
		values := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}

		err := generator.generatePatchFiles(patchPath, values, false)
		if err != nil {
			t.Fatalf("expected generatePatchFiles to succeed, got: %v", err)
		}
	})

	t.Run("InvalidManifest", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock shims to avoid file system issues
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		patchPath := "/test/patch.yaml"
		values := map[string]any{
			"invalid": "manifest",
		}

		err := generator.generatePatchFiles(patchPath, values, false)
		if err == nil {
			t.Fatal("expected generatePatchFiles to fail with invalid manifest")
		}
		if !strings.Contains(err.Error(), "invalid Kubernetes manifest") {
			t.Errorf("expected error about invalid manifest, got: %v", err)
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock shims to fail
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir error")
		}

		patchPath := "/test/patch.yaml"
		values := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}

		err := generator.generatePatchFiles(patchPath, values, false)
		if err == nil {
			t.Fatal("expected generatePatchFiles to fail with mkdir error")
		}
		if !strings.Contains(err.Error(), "failed to create directory") {
			t.Errorf("expected error about directory creation, got: %v", err)
		}
	})

	t.Run("MarshalYAMLError", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock shims
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("marshal error")
		}

		patchPath := "/test/patch.yaml"
		values := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}

		err := generator.generatePatchFiles(patchPath, values, false)
		if err == nil {
			t.Fatal("expected generatePatchFiles to fail with marshal error")
		}
		if !strings.Contains(err.Error(), "failed to marshal content to YAML") {
			t.Errorf("expected error about YAML marshaling, got: %v", err)
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock shims
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write error")
		}

		patchPath := "/test/patch.yaml"
		values := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}

		err := generator.generatePatchFiles(patchPath, values, false)
		if err == nil {
			t.Fatal("expected generatePatchFiles to fail with write error")
		}
		if !strings.Contains(err.Error(), "failed to write patch file") {
			t.Errorf("expected error about file writing, got: %v", err)
		}
	})
}

func TestKustomizeGenerator_generateValuesFiles(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock shims
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}

		valuesPath := "/test/values.yaml"
		values := map[string]any{
			"domain":  "example.com",
			"port":    80,
			"enabled": true,
		}

		err := generator.generateValuesFiles(valuesPath, values, false)
		if err != nil {
			t.Fatalf("expected generateValuesFiles to succeed, got: %v", err)
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock shims to fail
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir error")
		}

		valuesPath := "/test/values.yaml"
		values := map[string]any{"test": "value"}

		err := generator.generateValuesFiles(valuesPath, values, false)
		if err == nil {
			t.Fatal("expected generateValuesFiles to fail with mkdir error")
		}
		if !strings.Contains(err.Error(), "failed to create directory") {
			t.Errorf("expected error about directory creation, got: %v", err)
		}
	})

	t.Run("MarshalYAMLError", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock shims
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("marshal error")
		}

		valuesPath := "/test/values.yaml"
		values := map[string]any{"test": "value"}

		err := generator.generateValuesFiles(valuesPath, values, false)
		if err == nil {
			t.Fatal("expected generateValuesFiles to fail with marshal error")
		}
		if !strings.Contains(err.Error(), "failed to marshal content to YAML") {
			t.Errorf("expected error about YAML marshaling, got: %v", err)
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		generator, mocks := setupKustomizeGeneratorMocks(t)

		// Mock shims
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write error")
		}

		valuesPath := "/test/values.yaml"
		values := map[string]any{"test": "value"}

		err := generator.generateValuesFiles(valuesPath, values, false)
		if err == nil {
			t.Fatal("expected generateValuesFiles to fail with write error")
		}
		if !strings.Contains(err.Error(), "failed to write values file") {
			t.Errorf("expected error about file writing, got: %v", err)
		}
	})
}

// =============================================================================
// Validation Tests
// =============================================================================

func TestKustomizeGenerator_validateKustomizationName(t *testing.T) {
	// Given a generator
	generator, _ := setupKustomizeGeneratorMocks(t)

	testCases := []struct {
		name        string
		input       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "ValidSimpleName",
			input:       "test-kustomization",
			expectError: false,
		},
		{
			name:        "ValidWithHyphens",
			input:       "test-kustomization-name",
			expectError: false,
		},
		{
			name:        "ValidWithUnderscores",
			input:       "test_kustomization_name",
			expectError: false,
		},
		{
			name:        "ValidWithNumbers",
			input:       "test-kustomization-123",
			expectError: false,
		},
		{
			name:        "ValidSubdirectory",
			input:       "ingress/nginx",
			expectError: false,
		},
		{
			name:        "ValidNestedSubdirectory",
			input:       "ingress/nginx/controller",
			expectError: false,
		},
		{
			name:        "EmptyName",
			input:       "",
			expectError: true,
			errorMsg:    "cannot be empty",
		},
		{
			name:        "PathTraversal",
			input:       "test/../malicious",
			expectError: true,
			errorMsg:    "path traversal",
		},
		{
			name:        "BackslashPathTraversal",
			input:       "test\\..\\malicious",
			expectError: true,
			errorMsg:    "path traversal",
		},
		{
			name:        "EmptyPathComponent",
			input:       "test//component",
			expectError: true,
			errorMsg:    "empty path components",
		},
		{
			name:        "InvalidCharacter",
			input:       "test@kustomization",
			expectError: true,
			errorMsg:    "invalid character",
		},
		{
			name:        "InvalidCharacterInSubdirectory",
			input:       "ingress/nginx@controller",
			expectError: true,
			errorMsg:    "invalid character",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// When validating name
			err := generator.validateKustomizationName(tc.input)

			// Then check expected result
			if tc.expectError {
				if err == nil {
					t.Fatal("expected validation to fail")
				}
				if !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("expected error to contain '%s', got: %v", tc.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected validation to pass, got: %v", err)
				}
			}
		})
	}
}

func TestKustomizeGenerator_validateValuesForSubstitution(t *testing.T) {
	// Given a generator
	generator, _ := setupKustomizeGeneratorMocks(t)

	testCases := []struct {
		name        string
		values      map[string]any
		expectError bool
		errorMsg    string
	}{
		{
			name: "ValidScalarTypes",
			values: map[string]any{
				"string":  "value",
				"int":     42,
				"int8":    int8(8),
				"int16":   int16(16),
				"int32":   int32(32),
				"int64":   int64(64),
				"uint":    uint(42),
				"uint8":   uint8(8),
				"uint16":  uint16(16),
				"uint32":  uint32(32),
				"uint64":  uint64(64),
				"float32": float32(3.14),
				"float64": 3.14,
				"bool":    true,
			},
			expectError: false,
		},
		{
			name: "InvalidMapType",
			values: map[string]any{
				"nested": map[string]any{"key": "value"},
			},
			expectError: true,
			errorMsg:    "complex types",
		},
		{
			name: "InvalidSliceType",
			values: map[string]any{
				"array": []any{1, 2, 3},
			},
			expectError: true,
			errorMsg:    "complex types",
		},
		{
			name: "MixedValidAndInvalid",
			values: map[string]any{
				"valid":   "string",
				"invalid": map[string]any{"nested": "value"},
			},
			expectError: true,
			errorMsg:    "complex types",
		},
		{
			name: "UnsupportedType",
			values: map[string]any{
				"unsupported": make(chan int),
			},
			expectError: true,
			errorMsg:    "unsupported type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// When validating values
			err := generator.validateValuesForSubstitution(tc.values)

			// Then check expected result
			if tc.expectError {
				if err == nil {
					t.Fatal("expected validation to fail")
				}
				if !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("expected error to contain '%s', got: %v", tc.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected validation to pass, got: %v", err)
				}
			}
		})
	}
}

func TestKustomizeGenerator_validatePath(t *testing.T) {
	// Given a generator
	generator, _ := setupKustomizeGeneratorMocks(t)

	testCases := []struct {
		name        string
		targetPath  string
		basePath    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "ValidPath",
			targetPath:  "/base/valid/path",
			basePath:    "/base",
			expectError: false,
		},
		{
			name:        "ValidSubPath",
			targetPath:  "/base/sub/path/file.yaml",
			basePath:    "/base",
			expectError: false,
		},
		{
			name:        "PathTraversal",
			targetPath:  "/base/../malicious/path",
			basePath:    "/base",
			expectError: true,
			errorMsg:    "outside base path",
		},
		{
			name:        "DifferentBase",
			targetPath:  "/other/path",
			basePath:    "/base",
			expectError: true,
			errorMsg:    "outside base path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// When validating path
			err := generator.validatePath(tc.targetPath, tc.basePath)

			// Then check expected result
			if tc.expectError {
				if err == nil {
					t.Fatal("expected validation to fail")
				}
				if !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("expected error to contain '%s', got: %v", tc.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected validation to pass, got: %v", err)
				}
			}
		})
	}
}

func TestKustomizeGenerator_validateKubernetesManifest(t *testing.T) {
	// Given a generator
	generator, _ := setupKustomizeGeneratorMocks(t)

	testCases := []struct {
		name        string
		content     any
		expectError bool
		errorMsg    string
	}{
		{
			name: "ValidManifest",
			content: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
			expectError: false,
		},
		{
			name:        "NotMap",
			content:     "not a map",
			expectError: true,
			errorMsg:    "must be a map",
		},
		{
			name: "MissingApiVersion",
			content: map[string]any{
				"kind": "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
			expectError: true,
			errorMsg:    "missing or invalid 'apiVersion'",
		},
		{
			name: "EmptyApiVersion",
			content: map[string]any{
				"apiVersion": "",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
			expectError: true,
			errorMsg:    "missing or invalid 'apiVersion'",
		},
		{
			name: "MissingKind",
			content: map[string]any{
				"apiVersion": "v1",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
			expectError: true,
			errorMsg:    "missing or invalid 'kind'",
		},
		{
			name: "EmptyKind",
			content: map[string]any{
				"apiVersion": "v1",
				"kind":       "",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
			expectError: true,
			errorMsg:    "missing or invalid 'kind'",
		},
		{
			name: "MissingMetadata",
			content: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
			},
			expectError: true,
			errorMsg:    "missing 'metadata'",
		},
		{
			name: "MissingName",
			content: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{},
			},
			expectError: true,
			errorMsg:    "missing or invalid 'metadata.name'",
		},
		{
			name: "EmptyName",
			content: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "",
				},
			},
			expectError: true,
			errorMsg:    "missing or invalid 'metadata.name'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// When validating manifest
			err := generator.validateKubernetesManifest(tc.content)

			// Then check expected result
			if tc.expectError {
				if err == nil {
					t.Fatal("expected validation to fail")
				}
				if !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("expected error to contain '%s', got: %v", tc.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected validation to pass, got: %v", err)
				}
			}
		})
	}
}
