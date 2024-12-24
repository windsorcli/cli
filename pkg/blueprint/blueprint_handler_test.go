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
local context = {
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
};
context
`

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

		// Mock loadFileData to return jsonnet data instead of yaml
		originalLoadFileData := loadFileData
		defer func() { loadFileData = originalLoadFileData }()
		loadFileData = func(path string) ([]byte, error) {
			if path == filepath.FromSlash("/mock/config/root/blueprint.jsonnet") {
				return []byte(safeBlueprintJsonnet), nil
			}
			if path == filepath.FromSlash("/mock/config/root/blueprint.yaml") {
				return []byte(safeBlueprintYAML), nil
			}
			return nil, fmt.Errorf("file not found")
		}

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

	t.Run("NoFileFound", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock osStat to return an error indicating the file does not exist
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()

		// Load the blueprint configuration
		err = blueprintHandler.LoadConfig()

		// Then the LoadConfig should not return an error
		if err != nil {
			t.Errorf("Expected LoadConfig to succeed, but got: %v", err)
		}
	})

	t.Run("ErrorReadingJsonnetFile", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock osStat to simulate the Jsonnet file exists
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
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

	t.Run("ErrorReadingYamlFile", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock osStat to simulate the YAML file exists
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
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

		// Mock generateBlueprintFromJsonnet to return an error
		originalGenerateBlueprintFromJsonnet := generateBlueprintFromJsonnet
		defer func() { generateBlueprintFromJsonnet = originalGenerateBlueprintFromJsonnet }()
		generateBlueprintFromJsonnet = func(contextConfig *config.Context, jsonnetTemplate string) (string, error) {
			return "", fmt.Errorf("error generating blueprint from local jsonnet")
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
			t.Errorf("Expected LoadConfig to fail with an error containing 'error generating blueprint from local jsonnet', but got: <nil>")
		} else {
			expectedMsg := "error generating blueprint from local jsonnet"
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
		blueprint := blueprintHandler.blueprint.deepCopy()
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
		blueprint := blueprintHandler.blueprint.deepCopy()
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

		blueprint := blueprintHandler.blueprint.deepCopy()
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

func TestBlueprintHandler_deepCopy(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		blueprint := &BlueprintV1Alpha1{
			Metadata: MetadataV1Alpha1{
				Name: "test-blueprint",
			},
			Sources: []SourceV1Alpha1{
				{
					Name:       "source1",
					Url:        "https://example.com/repo1.git",
					PathPrefix: "terraform",
					Ref:        "main",
				},
			},
			TerraformComponents: []TerraformComponentV1Alpha1{
				{
					Source: "source1",
					Path:   "module/path1",
				},
			},
		}
		copy := blueprint.deepCopy()
		if copy.Metadata.Name != "test-blueprint" {
			t.Errorf("Expected deep copy to have name %v, but got %v", "test-blueprint", copy.Metadata.Name)
		}
		if len(copy.Sources) != 1 || copy.Sources[0].Name != "source1" {
			t.Errorf("Expected deep copy to have source %v, but got %v", "source1", copy.Sources)
		}
		if len(copy.TerraformComponents) != 1 || copy.TerraformComponents[0].Source != "source1" {
			t.Errorf("Expected deep copy to have terraform component source %v, but got %v", "source1", copy.TerraformComponents)
		}
		// Additional test to ensure deep copy handles pointer fields correctly
		if copy.TerraformComponents[0].Path != "module/path1" {
			t.Errorf("Expected deep copy to have terraform component path %v, but got %v", "module/path1", copy.TerraformComponents[0].Path)
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

func TestBlueprintHandler_mergeBlueprints(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		dst := &BlueprintV1Alpha1{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: MetadataV1Alpha1{
				Name:        "original",
				Description: "original description",
				Authors:     []string{"author1"},
			},
			Sources: []SourceV1Alpha1{
				{
					Name: "source1",
					Url:  "http://example.com/source1",
					Ref:  "v1.0.0",
				},
			},
			TerraformComponents: []TerraformComponentV1Alpha1{
				{
					Source: "source1",
					Path:   "path1",
					Variables: map[string]TerraformVariableV1Alpha1{
						"var1": {Default: "default1"},
					},
					Values:   nil, // Set Values to nil to test initialization
					FullPath: "original/full/path",
				},
			},
		}

		src := &BlueprintV1Alpha1{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: MetadataV1Alpha1{
				Name:        "updated",
				Description: "updated description",
				Authors:     []string{"author2"},
			},
			Sources: []SourceV1Alpha1{
				{
					Name: "source2",
					Url:  "http://example.com/source2",
					Ref:  "v2.0.0",
				},
			},
			TerraformComponents: []TerraformComponentV1Alpha1{
				{
					Source: "source1",
					Path:   "path1",
					Variables: map[string]TerraformVariableV1Alpha1{
						"var2": {Default: "default2"},
					},
					Values: map[string]interface{}{
						"key2": "value2",
					},
					FullPath: "updated/full/path",
				},
				{
					Source: "source3",
					Path:   "path3",
					Variables: map[string]TerraformVariableV1Alpha1{
						"var3": {Default: "default3"},
					},
					Values: map[string]interface{}{
						"key3": "value3",
					},
					FullPath: "new/full/path",
				},
			},
		}

		mergeBlueprints(dst, src)

		if dst.Metadata.Name != "updated" {
			t.Errorf("Expected Metadata.Name to be 'updated', but got '%s'", dst.Metadata.Name)
		}
		if dst.Metadata.Description != "updated description" {
			t.Errorf("Expected Metadata.Description to be 'updated description', but got '%s'", dst.Metadata.Description)
		}
		if len(dst.Metadata.Authors) != 1 || dst.Metadata.Authors[0] != "author2" {
			t.Errorf("Expected Metadata.Authors to be ['author2'], but got %v", dst.Metadata.Authors)
		}
		if len(dst.Sources) != 1 || dst.Sources[0].Name != "source2" {
			t.Errorf("Expected Sources to be ['source2'], but got %v", dst.Sources)
		}
		if len(dst.TerraformComponents) != 2 {
			t.Fatalf("Expected 2 TerraformComponents, but got %d", len(dst.TerraformComponents))
		}
		component1 := dst.TerraformComponents[0]
		if len(component1.Variables) != 2 || component1.Variables["var1"].Default != "default1" || component1.Variables["var2"].Default != "default2" {
			t.Errorf("Expected Variables to be merged, but got %v", component1.Variables)
		}
		if component1.Values == nil || len(component1.Values) != 1 || component1.Values["key2"] != "value2" {
			t.Errorf("Expected Values to be initialized and contain 'key2', but got %v", component1.Values)
		}
		if component1.FullPath != "updated/full/path" {
			t.Errorf("Expected FullPath to be 'updated/full/path', but got '%s'", component1.FullPath)
		}
		component2 := dst.TerraformComponents[1]
		if len(component2.Variables) != 1 || component2.Variables["var3"].Default != "default3" {
			t.Errorf("Expected Variables to be ['var3'], but got %v", component2.Variables)
		}
		if component2.Values == nil || len(component2.Values) != 1 || component2.Values["key3"] != "value3" {
			t.Errorf("Expected Values to contain 'key3', but got %v", component2.Values)
		}
		if component2.FullPath != "new/full/path" {
			t.Errorf("Expected FullPath to be 'new/full/path', but got '%s'", component2.FullPath)
		}
	})

	t.Run("NoMergeWhenSrcIsNil", func(t *testing.T) {
		dst := &BlueprintV1Alpha1{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: MetadataV1Alpha1{
				Name:        "original",
				Description: "original description",
				Authors:     []string{"author1"},
			},
		}

		mergeBlueprints(dst, nil)

		if dst.Metadata.Name != "original" {
			t.Errorf("Expected Metadata.Name to remain 'original', but got '%s'", dst.Metadata.Name)
		}
		if dst.Metadata.Description != "original description" {
			t.Errorf("Expected Metadata.Description to remain 'original description', but got '%s'", dst.Metadata.Description)
		}
		if dst.Sources != nil {
			t.Errorf("Expected Sources to remain nil, but got %v", dst.Sources)
		}
		if dst.TerraformComponents != nil {
			t.Errorf("Expected TerraformComponents to remain nil, but got %v", dst.TerraformComponents)
		}
	})
}
