package blueprint

import (
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"testing"

	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
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
    url: https://example.com/source1
    version: v1.0.0
terraform:
  - name: terraform1
    source: https://example.com/terraform1
    version: v1.0.0
    variables:
      - name: var1
        type: string
        default: default1
        description: A test variable
    values:
      key1: value1
`

type MockSafeComponents struct {
	Injector           di.Injector
	MockContextHandler *context.MockContext
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

	// Mock the context handler methods
	mockContextHandler.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}

	// Mock the osReadFile and osWriteFile functions
	osReadFile = func(filename string) ([]byte, error) {
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

	return MockSafeComponents{
		Injector:           mockInjector,
		MockContextHandler: mockContextHandler,
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
		injector := mocks.Injector

		// When a new BlueprintHandler is created
		blueprintHandler := NewBlueprintHandler(injector)

		// And the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()

		// Then the initialization should succeed
		if err != nil {
			t.Errorf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the default blueprint name and description should be correct
		metadata := blueprintHandler.GetMetadata()
		if metadata.Name != "mock-context" {
			t.Errorf("Expected default blueprint name to be 'mock-context', but got '%s'", metadata.Name)
		}
		expectedDescription := fmt.Sprintf("This blueprint outlines resources in the %s context", metadata.Name)
		if metadata.Description != expectedDescription {
			t.Errorf("Expected default blueprint description to be '%s', but got '%s'", expectedDescription, metadata.Description)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Given a mock injector that does not resolve contextHandler
		mocks := setupSafeMocks()
		mocks.Injector.Register("contextHandler", nil)

		// When a new BlueprintHandler is created
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// And the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()

		// Then the initialization should fail with an error
		if err == nil {
			t.Errorf("Expected Initialize to fail, but got no error")
		}
	})
}

func TestBlueprintHandler_LoadConfig(t *testing.T) {
	// validateBlueprint is a helper function to validate the blueprint metadata, sources, and Terraform components
	validateBlueprint := func(t *testing.T, blueprintHandler *BaseBlueprintHandler) {
		metadata := blueprintHandler.GetMetadata()
		if metadata.Name != "test-blueprint" {
			t.Errorf("Expected metadata name to be 'test-blueprint', but got '%s'", metadata.Name)
		}
		if metadata.Description != "A test blueprint" {
			t.Errorf("Expected metadata description to be 'A test blueprint', but got '%s'", metadata.Description)
		}
		if len(metadata.Authors) != 1 || metadata.Authors[0] != "John Doe" {
			t.Errorf("Expected metadata authors to be ['John Doe'], but got %v", metadata.Authors)
		}

		sources := blueprintHandler.GetSources()
		if len(sources) != 1 || sources[0].Name != "source1" {
			t.Errorf("Expected sources to contain one source with name 'source1', but got %v", sources)
		}

		terraformComponents := blueprintHandler.GetTerraformComponents()
		if len(terraformComponents) != 1 || terraformComponents[0].Name != "terraform1" {
			t.Errorf("Expected Terraform components to contain one component with name 'terraform1', but got %v", terraformComponents)
		}
	}

	t.Run("Success", func(t *testing.T) {
		// Given a mock injector and a valid blueprint path
		mocks := setupSafeMocks()
		path := "/mock/config/root/blueprint.yaml"
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the blueprint is loaded
		err = blueprintHandler.LoadConfig(path)
		if err != nil {
			t.Fatalf("Expected LoadConfig to succeed, but got error: %v", err)
		}

		// Then the blueprint should be validated successfully
		validateBlueprint(t, blueprintHandler)
	})

	t.Run("PathIsEmpty", func(t *testing.T) {
		// Given a mock injector and an empty path
		mocks := setupSafeMocks()
		path := ""
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the blueprint is loaded with an empty path
		err = blueprintHandler.LoadConfig(path)
		if err != nil {
			t.Fatalf("Expected LoadConfig to succeed, but got error: %v", err)
		}

		// Then the blueprint should be validated successfully
		validateBlueprint(t, blueprintHandler)
	})

	t.Run("PathSetFileDoesNotExist", func(t *testing.T) {
		// Given a mock injector and a path that does not exist
		mocks := setupSafeMocks()
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the osStat function is overridden to simulate a file not existing
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(string) (fs.FileInfo, error) {
			return nil, fmt.Errorf("mock error file does not exist")
		}

		// And the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// Then loading the blueprint should fail
		err = blueprintHandler.LoadConfig("/mock/config/root/nonexistent.yaml")
		if err == nil {
			t.Errorf("Expected LoadConfig to fail, but got no error")
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a mock injector and a context handler that returns an error
		mocks := setupSafeMocks()
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the context handler is mocked to return an error
		mocks.MockContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}

		// Then loading the blueprint should fail
		err = blueprintHandler.LoadConfig()
		if err == nil {
			t.Errorf("Expected LoadConfig to fail, but got no error")
		}
	})

	t.Run("PathNotSetFileDoesNotExist", func(t *testing.T) {
		// Given a mock injector and a path that does not exist
		mocks := setupSafeMocks()
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the osStat function is overridden to simulate a file not existing
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(string) (fs.FileInfo, error) {
			return nil, fmt.Errorf("mock error file does not exist")
		}

		// And the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// Then loading the blueprint should not return an error
		err = blueprintHandler.LoadConfig()
		if err != nil {
			t.Errorf("Expected LoadConfig to succeed, but got error: %v", err)
		}
	})

	t.Run("ErrorReadingFile", func(t *testing.T) {
		// Given a mock injector and an invalid file path
		mocks := setupSafeMocks()
		path := "/invalid/path/blueprint.yaml"
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the osReadFile function is overridden to simulate an error
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(string) ([]byte, error) {
			return nil, fmt.Errorf("mock error reading file")
		}

		// And the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// Then loading the blueprint should fail
		err = blueprintHandler.LoadConfig(path)
		if err == nil {
			t.Errorf("Expected LoadConfig to fail, but got no error")
		}
	})

	t.Run("ErrorUnmarshallingYAML", func(t *testing.T) {
		// Given a mock injector and a path to an invalid YAML file
		mocks := setupSafeMocks()
		path := "/mock/config/root/invalid.yaml"
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the yamlUnmarshal function is overridden to simulate an error
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func([]byte, interface{}) error {
			return fmt.Errorf("mock error unmarshalling yaml")
		}

		// And the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// Then loading the blueprint should fail
		err = blueprintHandler.LoadConfig(path)
		if err == nil {
			t.Errorf("Expected LoadConfig to fail, but got no error")
		}
	})
}

func TestBlueprintHandler_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector and a valid path
		mocks := setupSafeMocks()
		path := "/mock/config/root/blueprint.yaml"
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the blueprint is saved
		err = blueprintHandler.WriteConfig(path)
		// Then the save operation should succeed
		if err != nil {
			t.Errorf("Expected Save to succeed, but got error: %v", err)
		}
	})

	t.Run("PathIsEmpty", func(t *testing.T) {
		// Given a mock injector and an empty path
		mocks := setupSafeMocks()
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the blueprint is saved with an empty path
		err = blueprintHandler.WriteConfig()
		// Then the save operation should succeed
		if err != nil {
			t.Errorf("Expected Save to succeed with empty path, but got error: %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a mock injector and a failure in getting config root
		mocks := setupSafeMocks()
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the GetConfigRoot function is overridden to simulate an error
		mocks.MockContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}

		// And the blueprint is saved with an empty path
		err = blueprintHandler.WriteConfig()
		// Then the save operation should fail
		if err == nil {
			t.Errorf("Expected Save to fail, but got no error")
		}
	})

	t.Run("ErrorCreatingDirectory", func(t *testing.T) {
		// Given a mock injector and a failure in creating a directory
		mocks := setupSafeMocks()
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the osMkdirAll function is overridden to simulate an error
		originalOsMkdirAll := osMkdirAll
		defer func() { osMkdirAll = originalOsMkdirAll }()
		osMkdirAll = func(string, os.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// And the blueprint is saved
		err = blueprintHandler.WriteConfig("/mock/config/root/blueprint.yaml")
		// Then the save operation should fail
		if err == nil {
			t.Errorf("Expected Save to fail, but got no error")
		}
	})

	t.Run("ErrorMarshallingYAML", func(t *testing.T) {
		// Given a mock injector and a valid path
		mocks := setupSafeMocks()
		path := "/mock/config/root/blueprint.yaml"
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the yamlMarshalNonNull function is overridden to simulate an error
		originalYamlMarshalNonNull := yamlMarshalNonNull
		defer func() { yamlMarshalNonNull = originalYamlMarshalNonNull }()
		yamlMarshalNonNull = func(interface{}) ([]byte, error) {
			return nil, fmt.Errorf("mock error marshalling yaml")
		}

		// And the blueprint is saved
		err = blueprintHandler.WriteConfig(path)
		// Then the save operation should fail
		if err == nil {
			t.Errorf("Expected Save to fail, but got no error")
		}
	})

	t.Run("ErrorWritingFile", func(t *testing.T) {
		// Given a mock injector and a valid path
		mocks := setupSafeMocks()
		path := "/mock/config/root/blueprint.yaml"
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the osWriteFile function is overridden to simulate an error
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(string, []byte, os.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}

		// And the blueprint is saved
		err = blueprintHandler.WriteConfig(path)
		// Then the save operation should fail
		if err == nil {
			t.Errorf("Expected Save to fail, but got no error")
		}
	})
}

func TestBlueprintHandler_GetMetadata(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a valid blueprint handler
		mocks := setupSafeMocks()
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the metadata is set
		expectedMetadata := MetadataV1Alpha1{
			Name:        "test-blueprint",
			Description: "A test blueprint",
			Authors:     []string{"John Doe"},
		}

		blueprintHandler.LoadConfig()

		// Then the metadata should be retrieved successfully
		retrievedMetadata := blueprintHandler.GetMetadata()
		if retrievedMetadata.Name != expectedMetadata.Name || retrievedMetadata.Description != expectedMetadata.Description || !reflect.DeepEqual(retrievedMetadata.Authors, expectedMetadata.Authors) {
			t.Errorf("Expected metadata to be %v, but got %v", expectedMetadata, retrievedMetadata)
		}
	})
}

func TestBlueprintHandler_GetSources(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a valid blueprint handler
		mocks := setupSafeMocks()
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the sources are set
		expectedSources := []SourceV1Alpha1{
			{
				Name:    "source1",
				Url:     "https://example.com/source1",
				Version: "v1.0.0",
			},
		}

		err = blueprintHandler.LoadConfig()
		if err != nil {
			t.Fatalf("Expected LoadConfig to succeed, but got error: %v", err)
		}

		// Then the sources should be retrieved successfully
		retrievedSources := blueprintHandler.GetSources()
		if !reflect.DeepEqual(retrievedSources, expectedSources) {
			t.Errorf("Expected sources to be %v, but got %v", expectedSources, retrievedSources)
		}
	})
}

func TestBlueprintHandler_GetTerraformComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a valid blueprint handler
		mocks := setupSafeMocks()
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the Terraform components are set
		expectedTerraformComponents := []TerraformComponentV1Alpha1{
			{
				Name:    "terraform1",
				Source:  "https://example.com/terraform1",
				Version: "v1.0.0",
				Variables: []TerraformVariableV1Alpha1{
					{
						Name:        "var1",
						Type:        "string",
						Default:     "default1",
						Description: "A test variable",
					},
				},
				Values: map[string]interface{}{
					"key1": "value1",
				},
			},
		}

		err = blueprintHandler.LoadConfig()
		if err != nil {
			t.Fatalf("Expected LoadConfig to succeed, but got error: %v", err)
		}

		// Then the Terraform components should be retrieved successfully
		retrievedTerraformComponents := blueprintHandler.GetTerraformComponents()
		if !reflect.DeepEqual(retrievedTerraformComponents, expectedTerraformComponents) {
			t.Errorf("Expected Terraform components to be %v, but got %v", expectedTerraformComponents, retrievedTerraformComponents)
		}
	})
}

func TestBlueprintHandler_SetMetadata(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a valid blueprint handler
		mocks := setupSafeMocks()
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the metadata is set
		expectedMetadata := MetadataV1Alpha1{
			Name:        "test-blueprint",
			Description: "A test blueprint",
			Authors:     []string{"John Doe"},
		}

		err = blueprintHandler.SetMetadata(expectedMetadata)
		if err != nil {
			t.Fatalf("Expected SetMetadata to succeed, but got error: %v", err)
		}

		// Then the metadata should be retrieved successfully
		retrievedMetadata := blueprintHandler.GetMetadata()
		if !reflect.DeepEqual(retrievedMetadata, expectedMetadata) {
			t.Errorf("Expected metadata to be %v, but got %v", expectedMetadata, retrievedMetadata)
		}
	})
}

func TestBlueprintHandler_SetSources(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a valid blueprint handler
		mocks := setupSafeMocks()
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the sources are set
		expectedSources := []SourceV1Alpha1{
			{
				Name:    "source1",
				Url:     "https://example.com/source1",
				Version: "v1.0.0",
			},
		}

		err = blueprintHandler.SetSources(expectedSources)
		if err != nil {
			t.Fatalf("Expected SetSources to succeed, but got error: %v", err)
		}

		// Then the sources should be retrieved successfully
		retrievedSources := blueprintHandler.GetSources()
		if !reflect.DeepEqual(retrievedSources, expectedSources) {
			t.Errorf("Expected sources to be %v, but got %v", expectedSources, retrievedSources)
		}
	})
}

func TestBlueprintHandler_SetTerraformComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a valid blueprint handler
		mocks := setupSafeMocks()
		blueprintHandler := NewBlueprintHandler(mocks.Injector)

		// When the BlueprintHandler is initialized
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And the Terraform components are set
		expectedTerraformComponents := []TerraformComponentV1Alpha1{
			{
				Name:    "terraform1",
				Source:  "https://example.com/terraform1",
				Version: "v1.0.0",
				Variables: []TerraformVariableV1Alpha1{
					{
						Name:        "var1",
						Type:        "string",
						Default:     "default1",
						Description: "A test variable",
					},
				},
				Values: map[string]interface{}{
					"key1": "value1",
				},
			},
		}

		err = blueprintHandler.SetTerraformComponents(expectedTerraformComponents)
		if err != nil {
			t.Fatalf("Expected SetTerraformComponents to succeed, but got error: %v", err)
		}

		// Then the Terraform components should be retrieved successfully
		retrievedTerraformComponents := blueprintHandler.GetTerraformComponents()
		if !reflect.DeepEqual(retrievedTerraformComponents, expectedTerraformComponents) {
			t.Errorf("Expected Terraform components to be %v, but got %v", expectedTerraformComponents, retrievedTerraformComponents)
		}
	})
}
