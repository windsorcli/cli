package blueprint

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// safeBlueprintYAML holds the "safe" blueprint yaml string
var safeBlueprintYAML = `
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
  description: A test blueprint
  authors:
    - John Doe
sources:
  - name: source1
    url: git::https://example.com/source1.git
    ref: v1.0.0
terraform:
  - source: source1
    path: path/to/code
    values:
      key1: value1
`

// safeBlueprintJsonnet holds the "safe" blueprint jsonnet string
var safeBlueprintJsonnet = `
local context = std.extVar("context");
{
  kind: "Blueprint",
  apiVersion: "v1alpha1",
  metadata: {
    name: "test-blueprint",
    description: "A test blueprint",
    authors: ["John Doe"]
  },
  sources: [
    {
      name: "source1",
      url: "git::https://example.com/source1.git",
      ref: "v1.0.0"
    }
  ],
  terraform: [
    {
      source: "source1",
      path: "path/to/code",
      values: {
        key1: "value1"
      }
    }
  ]
}
`

// compareYAML compares two YAML byte slices by unmarshaling them into interface{} and using DeepEqual.
func compareYAML(t *testing.T, actualYAML, expectedYAML []byte) {
	var actualData interface{}
	var expectedData interface{}

	// When unmarshaling actual YAML
	err := yaml.Unmarshal(actualYAML, &actualData)
	if err != nil {
		t.Fatalf("Failed to unmarshal actual YAML data: %v", err)
	}

	// When unmarshaling expected YAML
	err = yaml.Unmarshal(expectedYAML, &expectedData)
	if err != nil {
		t.Fatalf("Failed to unmarshal expected YAML data: %v", err)
	}

	// Then compare the data structures
	if !reflect.DeepEqual(actualData, expectedData) {
		actualFormatted, _ := yaml.Marshal(actualData)
		expectedFormatted, _ := yaml.Marshal(expectedData)
		t.Errorf("YAML mismatch.\nActual:\n%s\nExpected:\n%s", string(actualFormatted), string(expectedFormatted))
	}
}

type MockSafeComponents struct {
	Injector           di.Injector
	MockContextHandler *context.MockContext
	MockShell          *shell.MockShell
	MockConfigHandler  *config.MockConfigHandler
}

// setupSafeMocks function creates safe mocks for the blueprint handler
func setupSafeMocks(injector ...di.Injector) MockSafeComponents {
	// Mock the dependencies for the blueprint handler
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	// Create a new mock context handler
	mockContextHandler := context.NewMockContext()
	mockInjector.Register("contextHandler", mockContextHandler)

	// Create a new mock shell
	mockShell := shell.NewMockShell()
	mockInjector.Register("shell", mockShell)

	// Create a new mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockInjector.Register("configHandler", mockConfigHandler)

	// Mock the context handler methods
	mockContextHandler.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}

	// Mock the shell method to return a mock project root
	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/mock/project/root", nil
	}

	// Save original functions to restore later
	originalOsReadFile := osReadFile
	originalOsWriteFile := osWriteFile
	originalOsStat := osStat
	originalOsMkdirAll := osMkdirAll

	// Mock the osReadFile and osWriteFile functions
	osReadFile = func(_ string) ([]byte, error) {
		return []byte(safeBlueprintYAML), nil
	}
	osWriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
		return nil
	}
	osStat = func(_ string) (fs.FileInfo, error) {
		return nil, nil
	}
	osMkdirAll = func(_ string, _ fs.FileMode) error {
		return nil
	}

	// Defer restoring the original functions
	defer func() {
		osReadFile = originalOsReadFile
		osWriteFile = originalOsWriteFile
		osStat = originalOsStat
		osMkdirAll = originalOsMkdirAll
	}()

	return MockSafeComponents{
		Injector:           mockInjector,
		MockContextHandler: mockContextHandler,
		MockShell:          mockShell,
		MockConfigHandler:  mockConfigHandler,
	}
}

func TestBlueprintHandler_NewBlueprintHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// Then the BlueprintHandler should not be nil
		if blueprintHandler == nil {
			t.Errorf("Expected NewBlueprintHandler to return a non-nil value")
		}

		// And it should be of type BaseBlueprintHandler
		if _, ok := interface{}(blueprintHandler).(*BaseBlueprintHandler); !ok {
			t.Errorf("Expected NewBlueprintHandler to return a BaseBlueprintHandler")
		}
	})
}

func TestBlueprintHandler_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()

		// Then the initialization should succeed
		if err != nil {
			t.Errorf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the BlueprintHandler should have the correct project root
		if blueprintHandler.projectRoot != "/mock/project/root" {
			t.Errorf("Expected project root to be '/mock/project/root', but got '%s'", blueprintHandler.projectRoot)
		}

		// And the BlueprintHandler should have the correct config handler
		if blueprintHandler.configHandler == nil {
			t.Errorf("Expected configHandler to be set, but got nil")
		}

		// And the BlueprintHandler should have the correct context handler
		if blueprintHandler.contextHandler == nil {
			t.Errorf("Expected contextHandler to be set, but got nil")
		}

		// And the BlueprintHandler should have the correct shell
		if blueprintHandler.shell == nil {
			t.Errorf("Expected shell to be set, but got nil")
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()
		mocks.Injector.Register("configHandler", nil)

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()

		// Then the initialization should fail with the expected error
		if err == nil || err.Error() != "error resolving configHandler" {
			t.Errorf("Expected Initialize to fail with 'error resolving configHandler', but got: %v", err)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()
		mocks.Injector.Register("contextHandler", nil)

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()

		// Then the initialization should fail with the expected error
		if err == nil || err.Error() != "error resolving contextHandler" {
			t.Errorf("Expected Initialize to fail with 'error resolving contextHandler', but got: %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()
		mocks.Injector.Register("shell", nil)

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()

		// Then the initialization should fail with the expected error
		if err == nil || err.Error() != "error resolving shell" {
			t.Errorf("Expected Initialize to fail with 'error resolving shell', but got: %v", err)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()
		mocks.Injector.Register("shell", mocks.MockShell)
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()

		// Then the initialization should fail with the expected error
		if err == nil || err.Error() != "error getting project root: error getting project root" {
			t.Errorf("Expected Initialize to fail with 'error getting project root: error getting project root', but got: %v", err)
		}
	})
}

func TestBlueprintHandler_LoadConfig(t *testing.T) {
	// Hoist the safe os level mocks to the top of the test runner
	originalOsStat := osStat
	defer func() { osStat = originalOsStat }()
	osStat = func(name string) (fs.FileInfo, error) {
		if name == filepath.FromSlash("/mock/config/root/blueprint.jsonnet") || name == filepath.FromSlash("/mock/config/root/blueprint.yaml") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	originalOsReadFile := osReadFile
	defer func() { osReadFile = originalOsReadFile }()
	osReadFile = func(name string) ([]byte, error) {
		switch name {
		case filepath.FromSlash("/mock/config/root/blueprint.jsonnet"):
			return []byte(safeBlueprintJsonnet), nil
		case filepath.FromSlash("/mock/config/root/blueprint.yaml"):
			return []byte(safeBlueprintYAML), nil
		default:
			return nil, fmt.Errorf("file not found")
		}
	}

	t.Run("Success", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Initialize and load blueprint
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Initialization failed: %v", err)
		}
		if err := blueprintHandler.LoadConfig(filepath.Join("/mock", "config", "root", "blueprint")); err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		// Validate blueprint metadata
		metadata := blueprintHandler.GetMetadata()
		if metadata.Name == "" {
			t.Errorf("Expected metadata name to be set, got empty string")
		}
		if metadata.Description == "" {
			t.Errorf("Expected metadata description to be set, got empty string")
		}

		// Validate sources
		sources := blueprintHandler.GetSources()
		if len(sources) == 0 {
			t.Errorf("Expected at least one source, got none")
		}

		// Validate Terraform components
		components := blueprintHandler.GetTerraformComponents()
		if len(components) == 0 {
			t.Errorf("Expected at least one Terraform component, got none")
		}
	})

	t.Run("DefaultBlueprint", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Mock loadFileData to simulate no Jsonnet or YAML data
		originalLoadFileData := loadFileData
		defer func() { loadFileData = originalLoadFileData }()
		loadFileData = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, ".jsonnet") || strings.HasSuffix(path, ".yaml") {
				return nil, nil
			}
			return originalLoadFileData(path)
		}

		// Initialize and load blueprint
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Initialization failed: %v", err)
		}
		if err := blueprintHandler.LoadConfig(filepath.Join("/mock", "config", "root", "blueprint")); err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		// Validate that the default blueprint is used
		metadata := blueprintHandler.GetMetadata()
		expectedName := "mock-context"
		if metadata.Name != expectedName {
			t.Errorf("Expected metadata name to be '%s', got '%s'", expectedName, metadata.Name)
		}
		expectedDescription := fmt.Sprintf("This blueprint outlines resources in the %s context", expectedName)
		if metadata.Description != expectedDescription {
			t.Errorf("Expected metadata description to be '%s', got '%s'", expectedDescription, metadata.Description)
		}
	})

	t.Run("ErrorUnmarshallingLocalJsonnet", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Mock context to return "local"
		mocks.MockContextHandler.GetContextFunc = func() string { return "local" }

		// Mock loadFileData to simulate no data for local jsonnet
		originalLoadFileData := loadFileData
		defer func() { loadFileData = originalLoadFileData }()
		loadFileData = func(path string) ([]byte, error) {
			return nil, nil
		}

		// Mock yamlUnmarshal to simulate an error on unmarshalling local jsonnet data
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func(data []byte, v interface{}) error {
			return fmt.Errorf("simulated unmarshalling error")
		}

		// Initialize and attempt to load blueprint
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Initialization failed: %v", err)
		}
		if err := blueprintHandler.LoadConfig(filepath.Join("/mock", "config", "root", "blueprint")); err == nil {
			t.Fatalf("Expected LoadConfig to fail due to unmarshalling error, but it succeeded")
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()
		mocks.MockContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting config root")
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()

		// Load the blueprint configuration
		err = blueprintHandler.LoadConfig()

		// Then the initialization should fail with the expected error
		if err == nil || err.Error() != "error getting config root: error getting config root" {
			t.Errorf("Expected Initialize to fail with 'error getting config root: error getting config root', but got: %v", err)
		}
	})

	t.Run("ErrorLoadingJsonnetFile", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock osStat to simulate the Jsonnet file exists
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock osReadFile to return an error for Jsonnet file
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return nil, fmt.Errorf("error reading jsonnet file")
			}
			return nil, nil
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()

		// Load the blueprint configuration
		err = blueprintHandler.LoadConfig()

		// Then the LoadConfig should fail with the expected error for Jsonnet file
		if err == nil || !strings.Contains(err.Error(), "error reading jsonnet file") {
			t.Errorf("Expected LoadConfig to fail with error containing 'error reading jsonnet file', but got: %v", err)
		}
	})

	t.Run("ErrorLoadingYamlFile", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock osStat to simulate the YAML file exists
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock osReadFile to return an error for YAML file
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".yaml") {
				return nil, fmt.Errorf("error reading yaml file")
			}
			return nil, nil
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()

		// Load the blueprint configuration
		err = blueprintHandler.LoadConfig()

		// Then the LoadConfig should fail with the expected error for YAML file
		if err == nil || !strings.Contains(err.Error(), "error reading yaml file") {
			t.Errorf("Expected LoadConfig to fail with error containing 'error reading yaml file', but got: %v", err)
		}
	})

	t.Run("ErrorUnmarshallingYamlForLocalBlueprint", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock osStat to simulate the presence of a YAML file
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			if filepath.Clean(name) == filepath.Clean("/mock/config/root/blueprint.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock osReadFile to return valid YAML data
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(name string) ([]byte, error) {
			if filepath.Clean(name) == filepath.Clean("/mock/config/root/blueprint.yaml") {
				return []byte("valid: yaml"), nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// Mock yamlUnmarshal to simulate an error
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func(data []byte, v interface{}) error {
			return fmt.Errorf("error unmarshalling yaml data: error unmarshalling yaml for local blueprint")
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Load the blueprint configuration
		err := blueprintHandler.LoadConfig()

		// Then the LoadConfig should fail with the expected error
		if err == nil {
			t.Errorf("Expected LoadConfig to fail with an error containing 'error unmarshalling yaml for local blueprint', but got: <nil>")
		} else {
			expectedMsg := "error unmarshalling yaml for local blueprint"
			if !strings.Contains(err.Error(), expectedMsg) {
				t.Errorf("Expected error to contain '%s', but got: %v", expectedMsg, err)
			}
		}
	})

	t.Run("ErrorMarshallingContextToJSON", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock context to return "local"
		mocks.MockContextHandler.GetContextFunc = func() string { return "local" }

		// Mock jsonMarshal to return an error
		originalJsonMarshal := jsonMarshal
		defer func() { jsonMarshal = originalJsonMarshal }()
		jsonMarshal = func(v interface{}) ([]byte, error) {
			return nil, fmt.Errorf("error marshalling context to JSON")
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Load the blueprint configuration
		err := blueprintHandler.LoadConfig()

		// Then the LoadConfig should fail with the expected error
		if err == nil {
			t.Errorf("Expected LoadConfig to fail with an error containing 'error marshalling context to JSON', but got: <nil>")
		} else {
			expectedMsg := "error marshalling context to JSON"
			if !strings.Contains(err.Error(), expectedMsg) {
				t.Errorf("Expected error to contain '%s', but got: %v", expectedMsg, err)
			}
		}
	})

	t.Run("ErrorEvaluatingJsonnet", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock context to return "local"
		mocks.MockContextHandler.GetContextFunc = func() string { return "local" }

		// Mock jsonnetMakeVM to return a VM that fails on EvaluateAnonymousSnippet
		originalJsonnetMakeVM := jsonnetMakeVM
		defer func() { jsonnetMakeVM = originalJsonnetMakeVM }()
		jsonnetMakeVM = func() jsonnetVMInterface {
			return &mockJsonnetVM{}
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Load the blueprint configuration
		err := blueprintHandler.LoadConfig()

		// Then the LoadConfig should fail with the expected error
		if err == nil {
			t.Errorf("Expected LoadConfig to fail with an error containing 'error evaluating jsonnet', but got: <nil>")
		} else {
			expectedMsg := "error evaluating snippet"
			if !strings.Contains(err.Error(), expectedMsg) {
				t.Errorf("Expected error to contain '%s', but got: %v", expectedMsg, err)
			}
		}
	})

	t.Run("ErrorUnmarshallingJsonnetData", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock context to return "local"
		mocks.MockContextHandler.GetContextFunc = func() string { return "local" }

		// Mock yamlUnmarshal to return an error
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func(data []byte, v interface{}) error {
			return fmt.Errorf("mock error unmarshalling jsonnet data")
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Load the blueprint configuration
		err := blueprintHandler.LoadConfig()

		// Then the LoadConfig should fail with the expected error
		if err == nil {
			t.Errorf("Expected LoadConfig to fail with an error containing 'error unmarshalling jsonnet data', but got: <nil>")
		} else {
			expectedMsg := "error unmarshalling jsonnet data"
			if !strings.Contains(err.Error(), expectedMsg) {
				t.Errorf("Expected error to contain '%s', but got: %v", expectedMsg, err)
			}
		}
	})

	t.Run("ErrorMarshallingLocalBlueprintYaml", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock yamlMarshal to return an error
		originalYamlMarshal := yamlMarshal
		defer func() { yamlMarshal = originalYamlMarshal }()
		yamlMarshal = func(v interface{}) ([]byte, error) {
			return nil, fmt.Errorf("mock error marshalling context config to YAML")
		}

		// Mock context to return "local"
		mocks.MockContextHandler.GetContextFunc = func() string { return "local" }

		// Mock loadFileData to return empty jsonnet data
		originalLoadFileData := loadFileData
		defer func() { loadFileData = originalLoadFileData }()
		loadFileData = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, ".jsonnet") {
				return []byte(""), nil // Return empty data for jsonnet
			}
			return originalLoadFileData(path)
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Load the blueprint configuration
		err := blueprintHandler.LoadConfig()

		// Then the LoadConfig should fail with the expected error
		if err == nil {
			t.Fatalf("Expected LoadConfig to fail with an error containing 'error marshalling context config to YAML', but got: <nil>")
		}

		expectedMsg := "error marshalling context config to YAML"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error to contain '%s', but got: %v", expectedMsg, err)
		}
	})

	t.Run("ErrorUnmarshallingYamlToJson", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock yamlUnmarshal to return an error only on the second call
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		callCount := 0
		yamlUnmarshal = func(data []byte, v interface{}) error {
			callCount++
			if callCount == 3 {
				return fmt.Errorf("mock error unmarshalling YAML to JSON")
			}
			return originalYamlUnmarshal(data, v)
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Load the blueprint configuration
		err := blueprintHandler.LoadConfig()

		// Then the LoadConfig should fail with the expected error
		if err == nil {
			t.Fatalf("Expected LoadConfig to fail with an error containing 'mock error unmarshalling YAML to JSON', but got: <nil>")
		}

		expectedMsg := "mock error unmarshalling YAML to JSON"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error to contain '%s', but got: %v", expectedMsg, err)
		}
	})

	t.Run("ErrorMarshallingLocalJson", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock jsonMarshal to return an error
		originalJsonMarshal := jsonMarshal
		defer func() { jsonMarshal = originalJsonMarshal }()
		jsonMarshal = func(v interface{}) ([]byte, error) {
			return nil, fmt.Errorf("mock error marshalling JSON data")
		}

		// Mock context to return "local"
		mocks.MockContextHandler.GetContextFunc = func() string { return "local" }

		// Mock loadFileData to return empty data for both jsonnet and yaml
		originalLoadFileData := loadFileData
		defer func() { loadFileData = originalLoadFileData }()
		loadFileData = func(path string) ([]byte, error) {
			return []byte(""), nil // Return empty data for both jsonnet and yaml
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Load the blueprint configuration
		err := blueprintHandler.LoadConfig()

		// Then the LoadConfig should fail with the expected error
		if err == nil {
			t.Fatalf("Expected LoadConfig to fail with an error containing 'mock error marshalling JSON data', but got: <nil>")
		}

		expectedMsg := "mock error marshalling JSON data"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error to contain '%s', but got: %v", expectedMsg, err)
		}
	})

	t.Run("ErrorGeneratingBlueprintFromLocalJsonnet", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock osStat to simulate the absence of a Jsonnet file
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Mock osReadFile to return an error for Jsonnet file
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(name string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}

		// Mock context to return "local"
		mocks.MockContextHandler.GetContextFunc = func() string { return "local" }

		// Mock jsonnetMakeVM to simulate an error during Jsonnet evaluation
		originalJsonnetMakeVM := jsonnetMakeVM
		defer func() { jsonnetMakeVM = originalJsonnetMakeVM }()
		jsonnetMakeVM = func() jsonnetVMInterface {
			return &mockJsonnetVM{}
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Load the blueprint configuration
		err := blueprintHandler.LoadConfig()

		// Then the LoadConfig should fail with the expected error
		if err == nil {
			t.Errorf("Expected LoadConfig to fail with an error containing 'error evaluating snippet', but got: <nil>")
		} else {
			expectedMsg := "error evaluating snippet"
			if !strings.Contains(err.Error(), expectedMsg) {
				t.Errorf("Expected error to contain '%s', but got: %v", expectedMsg, err)
			}
		}
	})

	t.Run("ErrorUnmarshallingJsonnetData", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock yamlUnmarshal to simulate an error only when unmarshalling Jsonnet data
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func(data []byte, v interface{}) error {
			if strings.Contains(string(data), "test-blueprint") {
				return fmt.Errorf("simulated unmarshalling error")
			}
			return originalYamlUnmarshal(data, v)
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Load the blueprint configuration
		err := blueprintHandler.LoadConfig()

		// Then the LoadConfig should fail with the expected error
		if err == nil {
			t.Errorf("Expected LoadConfig to fail due to unmarshalling error, but it succeeded")
		} else {
			expectedMsg := "error unmarshalling jsonnet data"
			if !strings.Contains(err.Error(), expectedMsg) {
				t.Errorf("Expected error to contain '%s', but got: %v", expectedMsg, err)
			}
		}
	})

	t.Run("ErrorUnmarshallingYamlDataWithEvaluatedJsonnet", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock yamlUnmarshal to simulate an error when unmarshalling YAML data
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func(data []byte, v interface{}) error {
			if strings.Contains(string(data), "test-blueprint") {
				return fmt.Errorf("simulated unmarshalling error for YAML")
			}
			return originalYamlUnmarshal(data, v)
		}

		// Mock loadFileData to return empty YAML data and valid Jsonnet data
		originalLoadFileData := loadFileData
		defer func() { loadFileData = originalLoadFileData }()
		loadFileData = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, ".jsonnet") {
				return []byte(`{"test-blueprint": "some data"}`), nil
			}
			if strings.HasSuffix(path, ".yaml") {
				return []byte{}, nil
			}
			return originalLoadFileData(path)
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Load the blueprint configuration
		err := blueprintHandler.LoadConfig(filepath.Join("/mock", "config", "root", "blueprint"))

		// Then the LoadConfig should fail with the expected error
		if err == nil {
			t.Errorf("Expected LoadConfig to fail due to YAML unmarshalling error, but it succeeded")
		} else {
			expectedMsg := "simulated unmarshalling error for YAML"
			if !strings.Contains(err.Error(), expectedMsg) {
				t.Errorf("Expected error to contain '%s', but got: %v", expectedMsg, err)
			}
		}
	})
}

func TestBlueprintHandler_WriteConfig(t *testing.T) {
	// Hoist the safe os level mocks to the top of the test runner
	originalOsMkdirAll := osMkdirAll
	defer func() { osMkdirAll = originalOsMkdirAll }()
	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	originalOsWriteFile := osWriteFile
	defer func() { osWriteFile = originalOsWriteFile }()
	osWriteFile = func(name string, data []byte, perm os.FileMode) error {
		return nil
	}

	originalOsReadFile := osReadFile
	defer func() { osReadFile = originalOsReadFile }()
	osReadFile = func(name string) ([]byte, error) {
		if filepath.Clean(name) == filepath.Clean("/mock/config/root/blueprint.yaml") {
			return []byte(safeBlueprintYAML), nil
		}
		return nil, fmt.Errorf("file not found")
	}

	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Mock the TerraformComponents to include in the blueprint
		mockTerraformComponents := []TerraformComponentV1Alpha1{
			{
				Source: "source1",
				Path:   "path/to/code",
				Values: map[string]interface{}{
					"key1": "value1",
				},
			},
		}
		blueprintHandler.SetTerraformComponents(mockTerraformComponents)

		// Write the blueprint configuration
		err = blueprintHandler.WriteConfig(filepath.FromSlash("/mock/config/root/blueprint.yaml"))
		if err != nil {
			t.Fatalf("Failed to write blueprint configuration: %v", err)
		}

		// Validate the written file
		data, err := osReadFile(filepath.FromSlash("/mock/config/root/blueprint.yaml"))
		if err != nil {
			t.Fatalf("Failed to read written blueprint file: %v", err)
		}

		// Unmarshal the written data to validate its content
		var writtenBlueprint BlueprintV1Alpha1
		err = yamlUnmarshal(data, &writtenBlueprint)
		if err != nil {
			t.Fatalf("Failed to unmarshal written blueprint data: %v", err)
		}

		// Validate the written blueprint content
		if writtenBlueprint.Metadata.Name != "test-blueprint" {
			t.Errorf("Expected written blueprint name to be 'test-blueprint', got '%s'", writtenBlueprint.Metadata.Name)
		}
		if writtenBlueprint.Metadata.Description != "A test blueprint" {
			t.Errorf("Expected written blueprint description to be 'A test blueprint', got '%s'", writtenBlueprint.Metadata.Description)
		}
		if len(writtenBlueprint.Metadata.Authors) != 1 || writtenBlueprint.Metadata.Authors[0] != "John Doe" {
			t.Errorf("Expected written blueprint authors to be ['John Doe'], got %v", writtenBlueprint.Metadata.Authors)
		}

		// Validate the Terraform components
		if len(writtenBlueprint.TerraformComponents) != 1 {
			t.Errorf("Expected 1 Terraform component, got %d", len(writtenBlueprint.TerraformComponents))
		} else {
			component := writtenBlueprint.TerraformComponents[0]
			if component.Source != "source1" {
				t.Errorf("Expected component source to be 'source1', got '%s'", component.Source)
			}
			if component.Path != "path/to/code" {
				t.Errorf("Expected component path to be 'path/to/code', got '%s'", component.Path)
			}
			if component.Values["key1"] != "value1" {
				t.Errorf("Expected component value for 'key1' to be 'value1', got '%v'", component.Values["key1"])
			}
		}
	})

	t.Run("WriteNoPath", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Write the blueprint configuration without specifying a path
		err = blueprintHandler.WriteConfig()
		if err != nil {
			t.Fatalf("Failed to write blueprint configuration: %v", err)
		}

		// Validate the written file
		data, err := osReadFile(filepath.FromSlash("/mock/config/root/blueprint.yaml"))
		if err != nil {
			t.Fatalf("Failed to read written blueprint file: %v", err)
		}

		// Unmarshal the written data to validate its content
		var writtenBlueprint BlueprintV1Alpha1
		err = yamlUnmarshal(data, &writtenBlueprint)
		if err != nil {
			t.Fatalf("Failed to unmarshal written blueprint data: %v", err)
		}

		// Validate the written blueprint content
		if writtenBlueprint.Metadata.Name != "test-blueprint" {
			t.Errorf("Expected written blueprint name to be 'test-blueprint', got '%s'", writtenBlueprint.Metadata.Name)
		}
		if writtenBlueprint.Metadata.Description != "A test blueprint" {
			t.Errorf("Expected written blueprint description to be 'A test blueprint', got '%s'", writtenBlueprint.Metadata.Description)
		}
		if len(writtenBlueprint.Metadata.Authors) != 1 || writtenBlueprint.Metadata.Authors[0] != "John Doe" {
			t.Errorf("Expected written blueprint authors to be ['John Doe'], got %v", writtenBlueprint.Metadata.Authors)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Override the GetConfigRootFunc to simulate an error
		originalGetConfigRootFunc := mocks.MockContextHandler.GetConfigRootFunc
		defer func() { mocks.MockContextHandler.GetConfigRootFunc = originalGetConfigRootFunc }()
		mocks.MockContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Attempt to load config and expect an error
		err = blueprintHandler.WriteConfig()
		if err == nil {
			t.Fatalf("Expected error when loading config, got nil")
		}
		if err.Error() != "error getting config root: mock error" {
			t.Errorf("Expected error message 'error getting config root: mock error', got '%v'", err)
		}
	})

	t.Run("ErrorCreatingDirectory", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Override the osMkdirAll function to simulate an error
		originalOsMkdirAll := osMkdirAll
		defer func() { osMkdirAll = originalOsMkdirAll }()
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Attempt to write config and expect an error
		err = blueprintHandler.WriteConfig()
		if err == nil {
			t.Fatalf("Expected error when writing config, got nil")
		}
		if err.Error() != "error creating directory: mock error creating directory" {
			t.Errorf("Expected error message 'error creating directory: mock error creating directory', got '%v'", err)
		}
	})

	t.Run("ErrorMarshallingYaml", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Override the yamlMarshalNonNull function to simulate an error
		originalYamlMarshalNonNull := yamlMarshalNonNull
		defer func() { yamlMarshalNonNull = originalYamlMarshalNonNull }()
		yamlMarshalNonNull = func(_ interface{}) ([]byte, error) {
			return nil, fmt.Errorf("mock error marshalling yaml")
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Attempt to write config and expect an error
		err = blueprintHandler.WriteConfig()
		if err == nil {
			t.Fatalf("Expected error when marshalling yaml, got nil")
		}
		if !strings.Contains(err.Error(), "error marshalling yaml") {
			t.Errorf("Expected error message to contain 'error marshalling yaml', got '%v'", err)
		}
	})

	t.Run("ErrorWritingFile", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Override the osWriteFile function to simulate an error
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Attempt to write config and expect an error
		err = blueprintHandler.WriteConfig()
		if err == nil {
			t.Fatalf("Expected error when writing file, got nil")
		}
		if !strings.Contains(err.Error(), "error writing blueprint file") {
			t.Errorf("Expected error message to contain 'error writing blueprint file', got '%v'", err)
		}
	})
}

func TestBlueprintHandler_GetMetadata(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the metadata for the blueprint
		expectedMetadata := MetadataV1Alpha1{
			Name:        "test-blueprint",
			Description: "A test blueprint",
			Authors:     []string{"John Doe"},
		}
		blueprintHandler.SetMetadata(expectedMetadata)

		// Retrieve the metadata
		actualMetadata := blueprintHandler.GetMetadata()

		// Then the metadata should match the expected metadata
		if !reflect.DeepEqual(actualMetadata, expectedMetadata) {
			t.Errorf("Expected metadata to be %v, but got %v", expectedMetadata, actualMetadata)
		}
	})
}

func TestBlueprintHandler_GetSources(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the sources for the blueprint
		expectedSources := []SourceV1Alpha1{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  "v1.0.0",
			},
		}
		blueprintHandler.SetSources(expectedSources)

		// Retrieve the sources
		actualSources := blueprintHandler.GetSources()

		// Then the sources should match the expected sources
		if !reflect.DeepEqual(actualSources, expectedSources) {
			t.Errorf("Expected sources to be %v, but got %v", expectedSources, actualSources)
		}
	})
}

func TestBlueprintHandler_GetTerraformComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the Terraform components for the blueprint
		expectedComponents := []TerraformComponentV1Alpha1{
			{
				Source:   "source1",
				Path:     "path/to/code",
				FullPath: filepath.FromSlash("/mock/project/root/terraform/path/to/code"),
				Values: map[string]interface{}{
					"key1": "value1",
				},
			},
		}
		blueprintHandler.SetTerraformComponents(expectedComponents)

		// Retrieve the Terraform components
		actualComponents := blueprintHandler.GetTerraformComponents()

		// Then the Terraform components should match the expected components
		if !reflect.DeepEqual(actualComponents, expectedComponents) {
			t.Errorf("Expected Terraform components to be %v, but got %v", expectedComponents, actualComponents)
		}
	})
}

func TestBlueprintHandler_SetMetadata(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the metadata for the blueprint
		expectedMetadata := MetadataV1Alpha1{
			Name:        "test-blueprint",
			Description: "A test blueprint",
			Authors:     []string{"John Doe"},
		}
		blueprintHandler.SetMetadata(expectedMetadata)

		// Retrieve the metadata
		actualMetadata := blueprintHandler.GetMetadata()

		// Then the metadata should match the expected metadata
		if !reflect.DeepEqual(actualMetadata, expectedMetadata) {
			t.Errorf("Expected metadata to be %v, but got %v", expectedMetadata, actualMetadata)
		}
	})
}

func TestBlueprintHandler_SetSources(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the sources for the blueprint
		expectedSources := []SourceV1Alpha1{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  "v1.0.0",
			},
		}
		blueprintHandler.SetSources(expectedSources)

		// Retrieve the sources
		actualSources := blueprintHandler.GetSources()

		// Then the sources should match the expected sources
		if !reflect.DeepEqual(actualSources, expectedSources) {
			t.Errorf("Expected sources to be %v, but got %v", expectedSources, actualSources)
		}
	})
}

func TestBlueprintHandler_SetTerraformComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the Terraform components for the blueprint
		expectedComponents := []TerraformComponentV1Alpha1{
			{
				Source:   "source1",
				Path:     "path/to/code",
				FullPath: filepath.FromSlash("/mock/project/root/terraform/path/to/code"),
				Values: map[string]interface{}{
					"key1": "value1",
				},
			},
		}
		blueprintHandler.SetTerraformComponents(expectedComponents)

		// Retrieve the Terraform components
		actualComponents := blueprintHandler.GetTerraformComponents()

		// Then the Terraform components should match the expected components
		if !reflect.DeepEqual(actualComponents, expectedComponents) {
			t.Errorf("Expected Terraform components to be %v, but got %v", expectedComponents, actualComponents)
		}
	})
}

func TestBlueprintHandler_resolveComponentSources(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the sources for the blueprint
		expectedSources := []SourceV1Alpha1{
			{
				Name:       "source1",
				Url:        "git::https://example.com/source1.git",
				PathPrefix: "terraform",
				Ref:        "v1.0.0",
			},
		}
		blueprintHandler.SetSources(expectedSources)

		// Resolve the component sources
		blueprint := blueprintHandler.blueprint.Copy()
		blueprintHandler.resolveComponentSources(blueprint)

		// Then the resolved sources should match the expected sources
		for i, component := range blueprint.TerraformComponents {
			expectedSource := expectedSources[i].Url + "//" + expectedSources[i].PathPrefix + "/" + component.Path + "?ref=" + expectedSources[i].Ref
			if component.Source != expectedSource {
				t.Errorf("Expected component source to be %v, but got %v", expectedSource, component.Source)
			}
		}
	})
}

func TestBlueprintHandler_resolveComponentPaths(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the project root for the blueprint handler
		blueprintHandler.projectRoot = "/mock/project/root"

		// Set the Terraform components for the blueprint
		expectedComponents := []TerraformComponentV1Alpha1{
			{
				Source: "source1",
				Path:   "path/to/code",
			},
		}
		blueprintHandler.SetTerraformComponents(expectedComponents)

		// Resolve the component paths
		blueprint := blueprintHandler.blueprint.Copy()
		blueprintHandler.resolveComponentPaths(blueprint)

		// Then the resolved paths should match the expected paths
		for _, component := range blueprint.TerraformComponents {
			expectedPath := filepath.Join("/mock/project/root", "terraform", component.Path)
			if component.FullPath != expectedPath {
				t.Errorf("Expected component path to be %v, but got %v", expectedPath, component.FullPath)
			}
		}
	})

	t.Run("isValidTerraformRemoteSource", func(t *testing.T) {
		tests := []struct {
			name   string
			source string
			want   bool
		}{
			{"ValidLocalPath", "/absolute/path/to/module", false},
			{"ValidRelativePath", "./relative/path/to/module", false},
			{"InvalidLocalPath", "/invalid/path/to/module", false},
			{"ValidGitURL", "git::https://github.com/user/repo.git", true},
			{"ValidSSHGitURL", "git@github.com:user/repo.git", true},
			{"ValidHTTPURL", "https://github.com/user/repo.git", true},
			{"ValidHTTPZipURL", "https://example.com/archive.zip", true},
			{"InvalidHTTPURL", "https://example.com/not-a-zip", false},
			{"ValidTerraformRegistry", "registry.terraform.io/hashicorp/consul/aws", true},
			{"ValidGitHubReference", "github.com/hashicorp/terraform-aws-consul", true},
			{"InvalidSource", "invalid-source", false},
			{"VersionFileGitAtURL", "git@github.com:user/version.git", true},
			{"VersionFileGitAtURLWithPath", "git@github.com:user/version.git@v1.0.0", true},
			{"ValidGitLabURL", "git::https://gitlab.com/user/repo.git", true},
			{"ValidSSHGitLabURL", "git@gitlab.com:user/repo.git", true},
			{"ErrorCausingPattern", "[invalid-regex", false},
		}
		// Iterate over each test case
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := isValidTerraformRemoteSource(tt.source); got != tt.want {
					t.Errorf("isValidTerraformRemoteSource(%s) = %v, want %v", tt.source, got, tt.want)
				}
			})
		}
	})

	t.Run("ValidRemoteSourceWithFullPath", func(t *testing.T) {
		blueprintHandler := NewBlueprintHandler(setupSafeMocks().Injector)
		_ = blueprintHandler.Initialize()

		blueprintHandler.SetSources([]SourceV1Alpha1{{
			Name:       "test-source",
			Url:        "https://github.com/user/repo.git",
			PathPrefix: "terraform",
			Ref:        "main",
		}})

		blueprintHandler.SetTerraformComponents([]TerraformComponentV1Alpha1{{
			Source: "test-source",
			Path:   "module/path",
		}})

		blueprint := blueprintHandler.blueprint.Copy()
		blueprintHandler.resolveComponentSources(blueprint)
		blueprintHandler.resolveComponentPaths(blueprint)

		if blueprint.TerraformComponents[0].Source != "https://github.com/user/repo.git//terraform/module/path?ref=main" {
			t.Errorf("Unexpected resolved source: %v", blueprint.TerraformComponents[0].Source)
		}

		if blueprint.TerraformComponents[0].FullPath != filepath.Join("/mock/project/root", ".tf_modules", "module/path") {
			t.Errorf("Unexpected full path: %v", blueprint.TerraformComponents[0].FullPath)
		}
	})

	t.Run("RegexpMatchStringError", func(t *testing.T) {
		// Mock the regexpMatchString function to simulate an error for the specific test case
		originalRegexpMatchString := regexpMatchString
		defer func() { regexpMatchString = originalRegexpMatchString }()
		regexpMatchString = func(pattern, s string) (bool, error) {
			return false, fmt.Errorf("mocked error in regexpMatchString")
		}

		if got := isValidTerraformRemoteSource("[invalid-regex"); got != false {
			t.Errorf("isValidTerraformRemoteSource([invalid-regex) = %v, want %v", got, false)
		}
	})
}

func TestBlueprintHandler_generateBlueprintFromJsonnet(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		contextConfig := &config.Context{} // Use an empty struct as the fields are unknown

		jsonnetTemplate := safeBlueprintJsonnet // Use the valid template from the context

		expectedYaml := safeBlueprintYAML // Use the valid YAML from the context

		result, err := generateBlueprintFromJsonnet(contextConfig, jsonnetTemplate)
		if err != nil {
			t.Fatalf("Expected no error, but got: %v", err)
		}

		// Unmarshal both YAML strings into maps for comparison
		var expectedMap, resultMap map[string]interface{}
		if err := yaml.Unmarshal([]byte(expectedYaml), &expectedMap); err != nil {
			t.Fatalf("Failed to unmarshal expected YAML: %v", err)
		}
		if err := yaml.Unmarshal([]byte(result), &resultMap); err != nil {
			t.Fatalf("Failed to unmarshal result YAML: %v", err)
		}

		// Compare the maps
		if !reflect.DeepEqual(expectedMap, resultMap) {
			t.Errorf("Expected generated YAML to be equivalent to expected YAML, but got differences")
		}
	})

	t.Run("YamlMarshalError", func(t *testing.T) {
		contextConfig := &config.Context{} // Use an empty struct as the fields are unknown

		jsonnetTemplate := safeBlueprintJsonnet // Use the valid template from the context

		// Mock yamlMarshal to return an error
		originalYamlMarshal := yamlMarshal
		defer func() { yamlMarshal = originalYamlMarshal }()
		yamlMarshal = func(v interface{}) ([]byte, error) {
			return nil, fmt.Errorf("error marshaling yaml")
		}

		_, err := generateBlueprintFromJsonnet(contextConfig, jsonnetTemplate)
		if err == nil || !strings.Contains(err.Error(), "error marshaling yaml") {
			t.Errorf("Expected generateBlueprintFromJsonnet to fail with error containing 'error marshaling yaml', but got: %v", err)
		}
	})

	t.Run("ErrorConvertYamlToJson", func(t *testing.T) {
		contextConfig := &config.Context{} // Use an empty struct as the fields are unknown

		jsonnetTemplate := safeBlueprintJsonnet // Use the valid template from the context

		// Mock yamlToJson to return an error
		originalYamlToJson := yamlToJson
		defer func() { yamlToJson = originalYamlToJson }()
		yamlToJson = func(yamlBytes []byte) ([]byte, error) {
			return nil, fmt.Errorf("error converting yaml to json")
		}

		_, err := generateBlueprintFromJsonnet(contextConfig, jsonnetTemplate)
		if err == nil || !strings.Contains(err.Error(), "error converting yaml to json") {
			t.Errorf("Expected generateBlueprintFromJsonnet to fail with error containing 'error converting yaml to json', but got: %v", err)
		}
	})

	t.Run("ErrorEvaluateSnippet", func(t *testing.T) {
		contextConfig := &config.Context{}      // Use an empty struct as the fields are unknown
		jsonnetTemplate := safeBlueprintJsonnet // Use the valid template from the context

		// Mock jsonnetMakeVM to return a VM that always errors on EvaluateAnonymousSnippet
		originalJsonnetMakeVM := jsonnetMakeVM
		defer func() { jsonnetMakeVM = originalJsonnetMakeVM }()
		jsonnetMakeVM = func() jsonnetVMInterface {
			return &mockJsonnetVM{}
		}

		_, err := generateBlueprintFromJsonnet(contextConfig, jsonnetTemplate)
		if err == nil || !strings.Contains(err.Error(), "error evaluating snippet") {
			t.Errorf("Expected generateBlueprintFromJsonnet to fail with error containing 'error evaluating snippet', but got: %v", err)
		}
	})

	t.Run("ErrorJsonToYaml", func(t *testing.T) {
		contextConfig := &config.Context{}      // Use an empty struct as the fields are unknown
		jsonnetTemplate := safeBlueprintJsonnet // Use the valid template from the context

		// Mock yamlJSONToYAML to return an error
		originalYamlJSONToYAML := yamlJSONToYAML
		defer func() { yamlJSONToYAML = originalYamlJSONToYAML }()
		yamlJSONToYAML = func(jsonBytes []byte) ([]byte, error) {
			return nil, fmt.Errorf("error converting json to yaml")
		}

		_, err := generateBlueprintFromJsonnet(contextConfig, jsonnetTemplate)
		if err == nil || !strings.Contains(err.Error(), "error converting json to yaml") {
			t.Errorf("Expected generateBlueprintFromJsonnet to fail with error containing 'error converting json to yaml', but got: %v", err)
		}
	})

	t.Run("ErrorUnmarshalYaml", func(t *testing.T) {
		contextConfig := &config.Context{}      // Use an empty struct as the fields are unknown
		jsonnetTemplate := safeBlueprintJsonnet // Use the valid template from the context

		// Mock yamlUnmarshal to return an error
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func(yamlBytes []byte, v interface{}) error {
			return fmt.Errorf("error unmarshaling yaml")
		}

		_, err := generateBlueprintFromJsonnet(contextConfig, jsonnetTemplate)
		if err == nil || !strings.Contains(err.Error(), "error unmarshaling yaml") {
			t.Errorf("Expected generateBlueprintFromJsonnet to fail with error containing 'error unmarshaling yaml', but got: %v", err)
		}
	})
}

func TestYamlMarshalWithDefinedPaths(t *testing.T) {
	t.Run("AllNonNilValues", func(t *testing.T) {
		testData := struct {
			Name    string                          `yaml:"name"`
			Age     int                             `yaml:"age"`
			Nested  struct{ FieldA, FieldB string } `yaml:"nested"`
			Numbers []int                           `yaml:"numbers"`
			MapData map[string]string               `yaml:"map_data"`
		}{
			Name: "Alice",
			Age:  30,
			Nested: struct{ FieldA, FieldB string }{
				FieldA: "ValueA",
				FieldB: "42",
			},
			Numbers: []int{1, 2, 3},
			MapData: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		}
		expectedYAML := "name: Alice\nage: 30\nnested:\n  FieldA: ValueA\n  FieldB: \"42\"\nnumbers:\n- 1\n- 2\n- 3\nmap_data:\n  key1: value1\n  key2: value2\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("NilPointerFields", func(t *testing.T) {
		testData := struct {
			Name    *string `yaml:"name"`
			Age     *int    `yaml:"age"`
			Comment *string `yaml:"comment"`
		}{
			Name:    nil,
			Age:     func() *int { i := 25; return &i }(),
			Comment: nil,
		}
		expectedYAML := "age: 25\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("ZeroValues", func(t *testing.T) {
		testData := struct {
			Name    string `yaml:"name"`
			Age     int    `yaml:"age"`
			Active  bool   `yaml:"active"`
			Comment string `yaml:"comment"`
		}{
			Name:    "",
			Age:     0,
			Active:  false,
			Comment: "",
		}
		expectedYAML := "name: \"\"\nage: 0\nactive: false\ncomment: \"\"\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("NilSlicesAndMaps", func(t *testing.T) {
		testData := struct {
			Numbers []int          `yaml:"numbers"`
			MapData map[string]int `yaml:"map_data"`
			Nested  *struct{}      `yaml:"nested"`
		}{
			Numbers: nil,
			MapData: nil,
			Nested:  nil,
		}
		expectedYAML := "numbers: []\nmap_data: {}\nnested: {}\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("EmptySlicesAndMaps", func(t *testing.T) {
		testData := struct {
			Numbers []int          `yaml:"numbers"`
			MapData map[string]int `yaml:"map_data"`
		}{
			Numbers: []int{},
			MapData: map[string]int{},
		}
		expectedYAML := "numbers: []\nmap_data: {}\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("UnexportedFields", func(t *testing.T) {
		testData := struct {
			ExportedField   string `yaml:"exported_field"`
			unexportedField string `yaml:"unexported_field"`
		}{
			ExportedField:   "Visible",
			unexportedField: "Hidden",
		}
		expectedYAML := "exported_field: Visible\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("OmittedFields", func(t *testing.T) {
		testData := struct {
			Name   string `yaml:"name"`
			Secret string `yaml:"-"`
		}{
			Name:   "Bob",
			Secret: "SuperSecret",
		}
		expectedYAML := "name: Bob\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("NestedPointers", func(t *testing.T) {
		testData := struct {
			Inner *struct{ Value *string } `yaml:"inner"`
		}{
			Inner: nil,
		}
		expectedYAML := "inner: {}\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("SliceWithNilElements", func(t *testing.T) {
		testData := struct {
			Items []interface{} `yaml:"items"`
		}{
			Items: []interface{}{"Item1", nil, "Item3"},
		}
		expectedYAML := "items:\n- \"Item1\"\n- null\n- \"Item3\"\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("MapWithNilValues", func(t *testing.T) {
		testData := struct {
			Data map[string]interface{} `yaml:"data"`
		}{
			Data: map[string]interface{}{
				"key1": "value1",
				"key2": nil,
			},
		}
		expectedYAML := "data:\n  key1: \"value1\"\n  key2: null\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("InterfaceFields", func(t *testing.T) {
		testData := struct {
			Info interface{} `yaml:"info"`
		}{
			Info: nil,
		}
		expectedYAML := "info: {}\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("InvalidInput", func(t *testing.T) {
		testData := func() {}
		expectedYAML := ""

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err == nil || err.Error() != "unsupported value type func" {
			t.Fatalf("Expected error 'unsupported value type func', but got: %v", err)
		}
		if string(data) != expectedYAML {
			t.Errorf("Expected empty YAML, but got: %s", string(data))
		}
	})

	t.Run("InvalidReflectValue", func(t *testing.T) {
		var testData interface{} = nil
		expectedError := "invalid input: nil value"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err == nil || err.Error() != expectedError {
			t.Fatalf("Expected error '%s', but got: %v", expectedError, err)
		}
		if data != nil {
			t.Errorf("Expected nil data, but got: %v", data)
		}
	})

	t.Run("NoYAMLTag", func(t *testing.T) {
		testData := struct {
			Name  string
			Age   int
			Email string
		}{
			Name:  "Alice",
			Age:   30,
			Email: "alice@example.com",
		}
		expectedYAML := "Name: Alice\nAge: 30\nEmail: alice@example.com\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("EmptyResult", func(t *testing.T) {
		testData := struct {
			Nested  *struct{ FieldA, FieldB string } `yaml:"nested"`
			Numbers []int                            `yaml:"numbers"`
			MapData map[string]string                `yaml:"map_data"`
		}{
			Nested:  nil,
			Numbers: nil,
			MapData: map[string]string{},
		}
		expectedYAML := "map_data: {}\nnested: {}\nnumbers: []\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("ErrorConvertingSliceElement", func(t *testing.T) {
		testData := []interface{}{1, "string", func() {}}
		_, err := yamlMarshalWithDefinedPaths(testData)
		if err == nil || err.Error() != "error converting slice element at index 2: unsupported value type func" {
			t.Fatalf("Expected error 'error converting slice element at index 2: unsupported value type func', but got: %v", err)
		}
	})

	t.Run("ErrorConvertingMapValue", func(t *testing.T) {
		testData := map[string]interface{}{
			"key1": 1,
			"key2": func() {},
		}
		_, err := yamlMarshalWithDefinedPaths(testData)
		if err == nil || err.Error() != "error converting map value for key key2: unsupported value type func" {
			t.Fatalf("Expected error 'error converting map value for key key2: unsupported value type func', but got: %v", err)
		}
	})

	t.Run("ErrorConvertingField", func(t *testing.T) {
		testData := struct {
			Name    string `yaml:"name"`
			Invalid func() `yaml:"invalid"`
		}{
			Name:    "Test",
			Invalid: func() {},
		}
		_, err := yamlMarshalWithDefinedPaths(testData)
		if err == nil || err.Error() != "error converting field Invalid: unsupported value type func" {
			t.Fatalf("Expected error 'error converting field Invalid: unsupported value type func', but got: %v", err)
		}
	})

	t.Run("EmptyStruct", func(t *testing.T) {
		testData := struct{}{}
		expectedYAML := "{}\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("IntSlice", func(t *testing.T) {
		testData := []int{1, 2, 3}
		expectedYAML := "- 1\n- 2\n- 3\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("UintSlice", func(t *testing.T) {
		testData := []uint{1, 2, 3}
		expectedYAML := "- 1\n- 2\n- 3\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("IntMap", func(t *testing.T) {
		testData := map[string]int{"key1": 1, "key2": 2}
		expectedYAML := "key1: 1\nkey2: 2\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("UintMap", func(t *testing.T) {
		testData := map[string]uint{"key1": 1, "key2": 2}
		expectedYAML := "key1: 1\nkey2: 2\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("FloatSlice", func(t *testing.T) {
		testData := []float64{1.1, 2.2, 3.3}
		expectedYAML := "- 1.1\n- 2.2\n- 3.3\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("FloatMap", func(t *testing.T) {
		testData := map[string]float64{"key1": 1.1, "key2": 2.2}
		expectedYAML := "key1: 1.1\nkey2: 2.2\n"

		data, err := yamlMarshalWithDefinedPaths(testData)
		if err != nil {
			t.Fatalf("yamlMarshalWithDefinedPaths() error: %v", err)
		}
		compareYAML(t, data, []byte(expectedYAML))
	})
}
