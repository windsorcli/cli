package blueprint

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/smithy-go/ptr"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
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
repository:
  url: git::https://example.com/repo.git
  ref:
    branch: main
sources:
  - name: source1
    url: git::https://example.com/source1.git
    ref:
      branch: main
    pathPrefix: /source1
  - name: source2
    url: git::https://example.com/source2.git
    ref:
      branch: develop
    pathPrefix: /source2
terraform:
  - source: source1
    path: path/to/code
    values:
      key1: value1
kustomize::
  - name: kustomization1
    path: overlays/dev
    source: source1
    dependsOn:
      - kustomization2
    patches:
      - patch: |-
          apiVersion: apps/v1
          kind: Deployment
          metadata:
            name: example
          spec:
            replicas: 3
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
  repository: {
    url: "git::https://example.com/repo.git",
    ref: {
      branch: "main"
    }
  },
  sources: [
    {
      name: "source1",
      url: "git::https://example.com/source1.git",
      ref: {
        branch: "main"
      },
      pathPrefix: "/source1"
    },
    {
      name: "source2",
      url: "git::https://example.com/source2.git",
      ref: {
        branch: "develop"
      },
      pathPrefix: "/source2"
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
  ],
  kustomize:: [
    {
      name: "kustomization1",
      path: "overlays/dev",
      source: "source1",
      dependsOn: ["kustomization2"],
      patches: [
        {
          patch: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: example\nspec:\n  replicas: 3"
        }
      ]
    }
  ]
}
`

// mockJsonnetVM is a mock implementation of jsonnetVMInterface for testing
type mockJsonnetVM struct {
	EvaluateAnonymousSnippetFunc func(filename, snippet string) (string, error)
}

// TLACode is a mock implementation that does nothing
func (vm *mockJsonnetVM) TLACode(key, val string) {}

// ExtCode is a mock implementation that does nothing
func (vm *mockJsonnetVM) ExtCode(key, val string) {}

// EvaluateAnonymousSnippet is a mock implementation that calls the provided function
func (vm *mockJsonnetVM) EvaluateAnonymousSnippet(filename, snippet string) (string, error) {
	if vm.EvaluateAnonymousSnippetFunc != nil {
		return vm.EvaluateAnonymousSnippetFunc(filename, snippet)
	}
	return "", nil
}

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
	Injector          di.Injector
	MockShell         *shell.MockShell
	MockConfigHandler *config.MockConfigHandler
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

	// Create a new mock shell
	mockShell := shell.NewMockShell()
	mockInjector.Register("shell", mockShell)

	// Create a new mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockInjector.Register("configHandler", mockConfigHandler)

	// Mock the context handler methods
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}

	// Ensure DOMAIN, CONTEXT, and LOADBALANCER_IP_RANGE are defined
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "dns.domain":
			return "mock.domain.com"
		case "network.loadbalancer_ips.start":
			return "192.168.1.1"
		case "network.loadbalancer_ips.end":
			return "192.168.1.100"
		case "docker.registry_url":
			return "mock.registry.com"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}

	// Return mock volume paths
	mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
		if key == "cluster.workers.volumes" {
			return []string{"${WINDSOR_PROJECT_ROOT}/.volumes:/var/local"}
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return nil
	}

	mockConfigHandler.GetContextFunc = func() string {
		return "mock-context"
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
		Injector:          mockInjector,
		MockShell:         mockShell,
		MockConfigHandler: mockConfigHandler,
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

		// And the BlueprintHandler should have the correct shell
		if blueprintHandler.shell == nil {
			t.Errorf("Expected shell to be set, but got nil")
		}
	})

	t.Run("SetProjectNameInContext", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Track if SetContextValue was called with the correct values
		projectNameSet := false
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "projectName" && value == "root" {
				projectNameSet = true
			}
			return nil
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()

		// Then the initialization should succeed
		if err != nil {
			t.Errorf("Expected Initialize to succeed, but got error: %v", err)
		}

		// And SetContextValue should have been called with "projectName" = "root"
		if !projectNameSet {
			t.Errorf("Expected projectName to be set to 'root' in context, but it wasn't")
		}
	})

	t.Run("ErrorSettingProjectName", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// Mock SetContextValue to return an error
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "projectName" {
				return fmt.Errorf("error setting project name")
			}
			return nil
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()

		// Then the initialization should fail with the expected error
		if err == nil || err.Error() != "error setting project name in config: error setting project name" {
			t.Errorf("Expected error 'error setting project name in config: error setting project name', got: %v", err)
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
		mocks.MockConfigHandler.GetContextFunc = func() string { return "local" }

		// Mock loadFileData to simulate no data for local jsonnet
		originalLoadFileData := loadFileData
		defer func() { loadFileData = originalLoadFileData }()
		loadFileData = func(path string) ([]byte, error) {
			return nil, nil
		}

		// Mock yamlUnmarshal to simulate an error on unmarshalling local jsonnet data
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func(data []byte, obj interface{}) error {
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
		mocks.MockConfigHandler.GetConfigRootFunc = func() (string, error) {
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
		yamlUnmarshal = func(data []byte, obj interface{}) error {
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
		mocks.MockConfigHandler.GetContextFunc = func() string { return "local" }

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
		mocks.MockConfigHandler.GetContextFunc = func() string { return "local" }

		// Mock jsonnetMakeVM to return a VM that fails on EvaluateAnonymousSnippet
		originalJsonnetMakeVM := jsonnetMakeVM
		defer func() { jsonnetMakeVM = originalJsonnetMakeVM }()
		jsonnetMakeVM = func() jsonnetVMInterface {
			return &mockJsonnetVM{
				EvaluateAnonymousSnippetFunc: func(filename, snippet string) (string, error) {
					return "", fmt.Errorf("error evaluating snippet")
				},
			}
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
		mocks.MockConfigHandler.GetContextFunc = func() string { return "local" }

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
		// With the new implementation, this test case needs to be skipped or adapted
		// The original test is no longer relevant since the code path has changed
		t.Skip("This test is no longer relevant with the updated LoadConfig implementation")
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
		mocks.MockConfigHandler.GetContextFunc = func() string { return "local" }

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
		mocks.MockConfigHandler.GetContextFunc = func() string { return "local" }

		// Mock jsonnetMakeVM to simulate an error during Jsonnet evaluation
		originalJsonnetMakeVM := jsonnetMakeVM
		defer func() { jsonnetMakeVM = originalJsonnetMakeVM }()
		jsonnetMakeVM = func() jsonnetVMInterface {
			return &mockJsonnetVM{
				EvaluateAnonymousSnippetFunc: func(filename, snippet string) (string, error) {
					return "", fmt.Errorf("error evaluating snippet")
				},
			}
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

	t.Run("ExistingYamlFilePreference", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Mock osStat to return success for both YAML and Jsonnet files
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (fs.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/blueprint.yaml") ||
				name == filepath.FromSlash("/mock/config/root/blueprint.jsonnet") {
				return nil, nil // Both files exist
			}
			return nil, os.ErrNotExist
		}

		// Mock loadFileData to return both YAML and Jsonnet data
		originalLoadFileData := loadFileData
		defer func() { loadFileData = originalLoadFileData }()
		loadFileData = func(path string) ([]byte, error) {
			if path == filepath.FromSlash("/mock/config/root/blueprint.yaml") {
				// YAML file with a different name than the Jsonnet one
				return []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: yaml-blueprint
  description: A YAML blueprint
`), nil
			}
			if path == filepath.FromSlash("/mock/config/root/blueprint.jsonnet") {
				// Jsonnet file with a different name than the YAML one
				return []byte(`
{
  kind: "Blueprint",
  apiVersion: "v1alpha1",
  metadata: {
    name: "jsonnet-blueprint",
    description: "A Jsonnet blueprint"
  }
}
`), nil
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

		// Verify that the YAML file was used (not the Jsonnet file)
		metadata := blueprintHandler.GetMetadata()
		if metadata.Name != "yaml-blueprint" {
			t.Errorf("Expected metadata name to be 'yaml-blueprint' (from YAML), got '%s'", metadata.Name)
		}
		if metadata.Description != "A YAML blueprint" {
			t.Errorf("Expected metadata description to be 'A YAML blueprint' (from YAML), got '%s'", metadata.Description)
		}
	})

	t.Run("EmptyEvaluatedJsonnet", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Mock context to return a specific value
		testContext := "test-context"
		mocks.MockConfigHandler.GetContextFunc = func() string {
			return testContext
		}

		// Mock jsonnetMakeVM to return a VM that produces an empty string
		originalJsonnetMakeVM := jsonnetMakeVM
		defer func() { jsonnetMakeVM = originalJsonnetMakeVM }()
		jsonnetMakeVM = func() jsonnetVMInterface {
			return &mockJsonnetVM{
				EvaluateAnonymousSnippetFunc: func(filename, snippet string) (string, error) {
					return "", nil // Return empty string to trigger DefaultBlueprint path
				},
			}
		}

		// Mock osStat to simulate no existing files
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Initialize and load blueprint
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Initialization failed: %v", err)
		}
		if err := blueprintHandler.LoadConfig(); err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		// Validate that the DefaultBlueprint was used with context values
		metadata := blueprintHandler.GetMetadata()
		if metadata.Name != testContext {
			t.Errorf("Expected metadata name to be '%s', got '%s'", testContext, metadata.Name)
		}

		expectedDescription := fmt.Sprintf("This blueprint outlines resources in the %s context", testContext)
		if metadata.Description != expectedDescription {
			t.Errorf("Expected metadata description to be '%s', got '%s'", expectedDescription, metadata.Description)
		}

		// Verify that the DefaultBlueprint's other fields were also set correctly
		blueprint := DefaultBlueprint.DeepCopy()
		blueprint.Metadata.Name = testContext
		blueprint.Metadata.Description = expectedDescription

		// Check if the loaded blueprint matches the expected DefaultBlueprint
		// by verifying some key fields
		kustomizations := blueprintHandler.GetKustomizations()
		if len(kustomizations) != len(blueprint.Kustomizations) {
			t.Errorf("Expected %d kustomizations, got %d", len(blueprint.Kustomizations), len(kustomizations))
		}

		terraformComponents := blueprintHandler.GetTerraformComponents()
		if len(terraformComponents) != len(blueprint.TerraformComponents) {
			t.Errorf("Expected %d terraform components, got %d", len(blueprint.TerraformComponents), len(terraformComponents))
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
		mockTerraformComponents := []blueprintv1alpha1.TerraformComponent{
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
		var writtenBlueprint blueprintv1alpha1.Blueprint
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
		var writtenBlueprint blueprintv1alpha1.Blueprint
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
		originalGetConfigRootFunc := mocks.MockConfigHandler.GetConfigRootFunc
		defer func() { mocks.MockConfigHandler.GetConfigRootFunc = originalGetConfigRootFunc }()
		mocks.MockConfigHandler.GetConfigRootFunc = func() (string, error) {
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

	t.Run("CleanupEmptyPostBuild", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set a kustomization with an empty PostBuild
		emptyPostBuildKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name: "kustomization-empty-postbuild",
				Path: "path/to/kustomize",
				// Create an empty PostBuild object that should be cleaned up
				PostBuild: &blueprintv1alpha1.PostBuild{
					Substitute:     map[string]string{},
					SubstituteFrom: []blueprintv1alpha1.SubstituteReference{},
				},
			},
			{
				Name: "kustomization-with-substitutes",
				Path: "path/to/kustomize2",
				// Create a PostBuild object with substitutes that should be preserved
				PostBuild: &blueprintv1alpha1.PostBuild{
					SubstituteFrom: []blueprintv1alpha1.SubstituteReference{
						{
							Kind: "ConfigMap",
							Name: "test-config",
						},
					},
				},
			},
		}
		blueprintHandler.SetKustomizations(emptyPostBuildKustomizations)

		// Mock yamlMarshalNonNull to verify the blueprint being written
		originalYamlMarshalNonNull := yamlMarshalNonNull
		defer func() { yamlMarshalNonNull = originalYamlMarshalNonNull }()

		var capturedBlueprint *blueprintv1alpha1.Blueprint
		yamlMarshalNonNull = func(v interface{}) ([]byte, error) {
			if bp, ok := v.(*blueprintv1alpha1.Blueprint); ok {
				capturedBlueprint = bp
			}
			return originalYamlMarshalNonNull(v)
		}

		// Write the blueprint configuration
		err = blueprintHandler.WriteConfig()
		if err != nil {
			t.Fatalf("Failed to write blueprint configuration: %v", err)
		}

		// Verify that the empty PostBuild was removed
		if len(capturedBlueprint.Kustomizations) != 2 {
			t.Fatalf("Expected 2 kustomizations, got %d", len(capturedBlueprint.Kustomizations))
		}

		// First kustomization should have null PostBuild
		if capturedBlueprint.Kustomizations[0].PostBuild != nil {
			t.Errorf("Expected PostBuild to be nil for kustomization with empty PostBuild, got %v",
				capturedBlueprint.Kustomizations[0].PostBuild)
		}

		// Second kustomization should still have its PostBuild
		if capturedBlueprint.Kustomizations[1].PostBuild == nil {
			t.Errorf("Expected PostBuild to be preserved for kustomization with substitutes")
		} else if len(capturedBlueprint.Kustomizations[1].PostBuild.SubstituteFrom) != 1 {
			t.Errorf("Expected 1 SubstituteFrom entry, got %d",
				len(capturedBlueprint.Kustomizations[1].PostBuild.SubstituteFrom))
		}
	})
}

func TestBlueprintHandler_Install(t *testing.T) {
	// Common setup for all subtests
	mocks := setupSafeMocks()
	originalKubeClientResourceOperation := kubeClientResourceOperation
	defer func() { kubeClientResourceOperation = originalKubeClientResourceOperation }()

	t.Run("Success", func(t *testing.T) {
		// Mock the kubeClientResourceOperation function for success
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			// Verify the ResourceType is correct for Kustomization, GitRepository, and ConfigMap
			switch config.ResourceName {
			case "kustomizations":
				if _, ok := config.ResourceType().(*kustomizev1.Kustomization); !ok {
					return fmt.Errorf("unexpected resource type for Kustomization")
				}
			case "gitrepositories":
				if _, ok := config.ResourceType().(*sourcev1.GitRepository); !ok {
					return fmt.Errorf("unexpected resource type for GitRepository")
				}
			case "configmaps":
				if _, ok := config.ResourceType().(*corev1.ConfigMap); !ok {
					return fmt.Errorf("unexpected resource type for ConfigMap")
				}
			default:
				return fmt.Errorf("unexpected resource name: %s", config.ResourceName)
			}
			return nil
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the sources, repository, and kustomizations for the blueprint
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		blueprintHandler.SetSources(expectedSources)

		expectedRepository := blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}
		blueprintHandler.SetRepository(expectedRepository)

		expectedKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name:      "kustomization1",
				DependsOn: []string{"dependency1", "dependency2"},
			},
		}
		blueprintHandler.SetKustomizations(expectedKustomizations)

		// Attempt to install the blueprint components
		err = blueprintHandler.Install()
		if err != nil {
			t.Fatalf("Expected successful installation, but got error: %v", err)
		}
	})

	t.Run("SourceURLWithoutDotGit", func(t *testing.T) {
		// Mock the kubeClientResourceOperation function for success
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			return nil
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the sources with a URL ending in ".git"
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source2",
				Url:  "https://example.com/source2",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		blueprintHandler.SetSources(expectedSources)

		// Attempt to install the blueprint components
		err = blueprintHandler.Install()
		if err != nil {
			t.Fatalf("Expected successful installation with .git URL, but got error: %v", err)
		}
	})

	t.Run("SourceWithSecretName", func(t *testing.T) {
		// Mock the kubeClientResourceOperation function for success
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			return nil
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the sources with a SecretName
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name:       "source3",
				Url:        "https://example.com/source3.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				SecretName: "my-secret",
			},
		}
		blueprintHandler.SetSources(expectedSources)

		// Attempt to install the blueprint components
		err = blueprintHandler.Install()
		if err != nil {
			t.Fatalf("Expected successful installation with SecretName, but got error: %v", err)
		}
	})

	t.Run("ErrorApplyingPrimaryRepository", func(t *testing.T) {
		// Mock the kubeClientResourceOperation function to simulate an error when applying the primary GitRepository
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "gitrepositories" && config.ResourceInstanceName == "mock-context" {
				return fmt.Errorf("mock error applying primary GitRepository")
			}
			return nil
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the primary repository for the blueprint
		expectedRepository := blueprintv1alpha1.Repository{
			Url: "git::https://example.com/primary-repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}
		blueprintHandler.SetRepository(expectedRepository)

		// Attempt to install the blueprint components
		err = blueprintHandler.Install()
		if err == nil || !strings.Contains(err.Error(), "mock error applying primary GitRepository") {
			t.Fatalf("Expected error when applying primary GitRepository, but got: %v", err)
		}
	})

	t.Run("ErrorApplyingGitRepository", func(t *testing.T) {
		// Mock the kubeClientResourceOperation function for GitRepository error
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "gitrepositories" {
				return fmt.Errorf("mock error applying GitRepository")
			}
			return nil
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the sources and kustomizations for the blueprint
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		blueprintHandler.SetSources(expectedSources)

		expectedKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name:      "kustomization1",
				DependsOn: []string{"dependency1", "dependency2"},
			},
		}
		blueprintHandler.SetKustomizations(expectedKustomizations)

		// Attempt to install the blueprint components
		err = blueprintHandler.Install()
		if err == nil || !strings.Contains(err.Error(), "mock error applying GitRepository") {
			t.Fatalf("Expected error when applying GitRepository, but got: %v", err)
		}
	})

	t.Run("ErrorApplyingKustomization", func(t *testing.T) {
		// Mock the kubeClientResourceOperation function for Kustomization error
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "kustomizations" {
				return fmt.Errorf("mock error applying Kustomization")
			}
			return nil
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the sources and kustomizations for the blueprint
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		blueprintHandler.SetSources(expectedSources)

		expectedKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name:      "kustomization1",
				DependsOn: []string{"dependency1", "dependency2"},
			},
		}
		blueprintHandler.SetKustomizations(expectedKustomizations)

		// Attempt to install the blueprint components
		err = blueprintHandler.Install()
		if err == nil || !strings.Contains(err.Error(), "mock error applying Kustomization") {
			t.Fatalf("Expected error when applying Kustomization, but got: %v", err)
		}
	})

	t.Run("ErrorApplyingConfigMap", func(t *testing.T) {
		// Mock the kubeClientResourceOperation function for ConfigMap error
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "configmaps" {
				return fmt.Errorf("mock error applying ConfigMap")
			}
			return nil
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the sources for the blueprint
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		blueprintHandler.SetSources(expectedSources)

		// Attempt to install the blueprint components
		err = blueprintHandler.Install()
		if err == nil || !strings.Contains(err.Error(), "mock error applying ConfigMap") {
			t.Fatalf("Expected error when applying ConfigMap, but got: %v", err)
		}
	})

	t.Run("SuccessApplyingConfigMap", func(t *testing.T) {
		// Mock the kubeClientResourceOperation function for success
		configMapApplied := false
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "configmaps" {
				configMapApplied = true

				// Check that the DOMAIN, CONTEXT, and LOADBALANCER_IP_RANGE values are as expected
				configMap, ok := config.ResourceObject.(*corev1.ConfigMap)
				if !ok {
					return fmt.Errorf("unexpected resource object type")
				}
				if configMap.Data["DOMAIN"] != "mock.domain.com" {
					return fmt.Errorf("unexpected DOMAIN value: got %s, want %s", configMap.Data["DOMAIN"], "mock.domain.com")
				}
				if configMap.Data["CONTEXT"] != "mock-context" {
					return fmt.Errorf("unexpected CONTEXT value: got %s, want %s", configMap.Data["CONTEXT"], "mock-context")
				}
				if configMap.Data["LOADBALANCER_IP_RANGE"] != "192.168.1.1-192.168.1.100" {
					return fmt.Errorf("unexpected LOADBALANCER_IP_RANGE value: got %s, want %s", configMap.Data["LOADBALANCER_IP_RANGE"], "192.168.1.1-192.168.1.100")
				}
				if configMap.Data["REGISTRY_URL"] != "mock.registry.com" {
					return fmt.Errorf("unexpected REGISTRY_URL value: got %s, want %s", configMap.Data["REGISTRY_URL"], "mock.registry.com")
				}
				if configMap.Data["LOCAL_VOLUME_PATH"] != "/var/local" {
					return fmt.Errorf("unexpected LOCAL_VOLUME_PATH value: got %s, want %s", configMap.Data["LOCAL_VOLUME_PATH"], "/var/local")
				}
			}
			return nil
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the sources for the blueprint
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		blueprintHandler.SetSources(expectedSources)

		// Attempt to install the blueprint components
		err = blueprintHandler.Install()
		if err != nil {
			t.Fatalf("Expected successful installation, but got error: %v", err)
		}

		// Verify that the ConfigMap was applied
		if !configMapApplied {
			t.Fatalf("Expected ConfigMap to be applied, but it was not")
		}
	})

	t.Run("EmptyLocalVolumePaths", func(t *testing.T) {
		// Mock the kubeClientResourceOperation function to verify empty localVolumePath
		configMapApplied := false
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "configmaps" {
				configMapApplied = true

				// Check that the ConfigMap contains an empty LOCAL_VOLUME_PATH
				configMap, ok := config.ResourceObject.(*corev1.ConfigMap)
				if !ok {
					return fmt.Errorf("unexpected resource object type")
				}

				// Verify the empty LOCAL_VOLUME_PATH value
				if configMap.Data["LOCAL_VOLUME_PATH"] != "" {
					return fmt.Errorf("expected empty LOCAL_VOLUME_PATH value, but got: %s", configMap.Data["LOCAL_VOLUME_PATH"])
				}
			}
			return nil
		}

		// Create a mock ConfigHandler that returns empty localVolumePaths
		mocks := setupSafeMocks()
		mocks.MockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{} // Return empty slice for localVolumePaths
			}
			return []string{"default value"}
		}

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the sources for the blueprint
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		blueprintHandler.SetSources(expectedSources)

		// Attempt to install the blueprint components
		err = blueprintHandler.Install()
		if err != nil {
			t.Fatalf("Expected successful installation, but got error: %v", err)
		}

		// Verify that the ConfigMap was applied with empty LOCAL_VOLUME_PATH
		if !configMapApplied {
			t.Fatalf("Expected ConfigMap to be applied, but it was not")
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
		expectedMetadata := blueprintv1alpha1.Metadata{
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
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
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
		expectedComponents := []blueprintv1alpha1.TerraformComponent{
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

func TestBlueprintHandler_GetKustomizations(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the Kustomizations for the blueprint
		expectedKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name:          "kustomization1",
				Path:          "kustomization1", // Original path without "kustomize" prefix
				Interval:      &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_INTERVAL},
				RetryInterval: &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_RETRY_INTERVAL},
				Timeout:       &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_TIMEOUT},
				Wait:          ptr.Bool(constants.DEFAULT_FLUX_KUSTOMIZATION_WAIT),  // Use ptr.Bool to set default value
				Force:         ptr.Bool(constants.DEFAULT_FLUX_KUSTOMIZATION_FORCE), // Use ptr.Bool to set default value
				Components:    nil,                                                  // Expected default value from blueprint_handler.go
				PostBuild: &blueprintv1alpha1.PostBuild{
					SubstituteFrom: []blueprintv1alpha1.SubstituteReference{
						{
							Name:     "blueprint",
							Kind:     "ConfigMap",
							Optional: false,
						},
					},
				},
			},
		}
		blueprintHandler.SetKustomizations(expectedKustomizations)

		// Retrieve the Kustomizations
		actualKustomizations := blueprintHandler.GetKustomizations()

		// Adjust the expected Kustomizations to include the "kustomize" prefix
		expectedKustomizations[0].Path = filepath.Join("kustomize", expectedKustomizations[0].Path)

		// Then the Kustomizations should match the expected Kustomizations
		if !reflect.DeepEqual(actualKustomizations, expectedKustomizations) {
			t.Errorf("Expected Kustomizations to be %v, but got %v", expectedKustomizations, actualKustomizations)
		}
	})

	t.Run("NoKustomizations", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		if err := blueprintHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Retrieve the Kustomizations
		actualKustomizations := blueprintHandler.GetKustomizations()

		// Then the Kustomizations should be nil
		if actualKustomizations != nil {
			t.Errorf("Expected Kustomizations to be nil, but got %v", actualKustomizations)
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
		expectedMetadata := blueprintv1alpha1.Metadata{
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
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
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
		expectedComponents := []blueprintv1alpha1.TerraformComponent{
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

func TestBlueprintHandler_SetKustomizations(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the Kustomizations for the blueprint
		inputKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name:          "kustomization1",
				Path:          filepath.FromSlash("overlays/dev"),
				Source:        "source1",
				Interval:      &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_INTERVAL},
				RetryInterval: &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_RETRY_INTERVAL},
				Timeout:       &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_TIMEOUT},
				Wait:          ptr.Bool(constants.DEFAULT_FLUX_KUSTOMIZATION_WAIT),
				Force:         ptr.Bool(constants.DEFAULT_FLUX_KUSTOMIZATION_FORCE),
				PostBuild: &blueprintv1alpha1.PostBuild{
					SubstituteFrom: []blueprintv1alpha1.SubstituteReference{
						{
							Kind:     "ConfigMap",
							Name:     "blueprint",
							Optional: false,
						},
					},
				},
			},
		}
		blueprintHandler.SetKustomizations(inputKustomizations)

		// Retrieve the Kustomizations
		actualKustomizations := blueprintHandler.GetKustomizations()

		// Adjust the expected Kustomizations to match the internal representation
		expectedKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name:          "kustomization1",
				Path:          filepath.FromSlash("kustomize/overlays/dev"), // Prepend "kustomize" to the path
				Source:        "source1",
				Interval:      &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_INTERVAL},
				RetryInterval: &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_RETRY_INTERVAL},
				Timeout:       &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_TIMEOUT},
				Wait:          ptr.Bool(constants.DEFAULT_FLUX_KUSTOMIZATION_WAIT),
				Force:         ptr.Bool(constants.DEFAULT_FLUX_KUSTOMIZATION_FORCE),
				PostBuild: &blueprintv1alpha1.PostBuild{
					SubstituteFrom: []blueprintv1alpha1.SubstituteReference{
						{
							Kind:     "ConfigMap",
							Name:     "blueprint",
							Optional: false,
						},
					},
				},
			},
		}

		// Then the Kustomizations should match the expected Kustomizations
		if !reflect.DeepEqual(actualKustomizations, expectedKustomizations) {
			t.Errorf("Expected Kustomizations to be %v, but got %v", expectedKustomizations, actualKustomizations)
		}
	})

	t.Run("NilKustomizations", func(t *testing.T) {
		// Given a mock injector
		mocks := setupSafeMocks()

		// When a new BlueprintHandler is created and initialized
		blueprintHandler := NewBlueprintHandler(mocks.Injector)
		err := blueprintHandler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		// Set the Kustomizations to nil
		blueprintHandler.SetKustomizations(nil)

		// Retrieve the Kustomizations
		actualKustomizations := blueprintHandler.GetKustomizations()

		// Then the Kustomizations should be nil
		if actualKustomizations != nil {
			t.Errorf("Expected Kustomizations to be nil, but got %v", actualKustomizations)
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

		// Set the sources for the blueprint with an empty PathPrefix to test the default behavior
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name:       "source1",
				Url:        "git::https://example.com/source1.git",
				PathPrefix: "", // Intentionally left empty to test default pathPrefix
				Ref:        blueprintv1alpha1.Reference{Commit: "commit123"},
			},
			{
				Name:       "source2",
				Url:        "git::https://example.com/source2.git",
				PathPrefix: "", // Intentionally left empty to test default pathPrefix
				Ref:        blueprintv1alpha1.Reference{SemVer: "v1.0.0"},
			},
			{
				Name:       "source3",
				Url:        "git::https://example.com/source3.git",
				PathPrefix: "", // Intentionally left empty to test default pathPrefix
				Ref:        blueprintv1alpha1.Reference{Tag: "v1.0.1"},
			},
			{
				Name:       "source4",
				Url:        "git::https://example.com/source4.git",
				PathPrefix: "", // Intentionally left empty to test default pathPrefix
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		blueprintHandler.SetSources(expectedSources)

		// Set the Terraform components for the blueprint
		expectedComponents := []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "path/to/code1",
			},
			{
				Source: "source2",
				Path:   "path/to/code2",
			},
			{
				Source: "source3",
				Path:   "path/to/code3",
			},
			{
				Source: "source4",
				Path:   "path/to/code4",
			},
		}
		blueprintHandler.SetTerraformComponents(expectedComponents)

		// Resolve the component sources
		blueprint := blueprintHandler.blueprint.DeepCopy()
		blueprintHandler.resolveComponentSources(blueprint)

		// Then the resolved sources should match the expected sources with default pathPrefix
		for i, component := range blueprint.TerraformComponents {
			var expectedRef string
			if expectedSources[i].Ref.Commit != "" {
				expectedRef = expectedSources[i].Ref.Commit
			} else if expectedSources[i].Ref.SemVer != "" {
				expectedRef = expectedSources[i].Ref.SemVer
			} else if expectedSources[i].Ref.Tag != "" {
				expectedRef = expectedSources[i].Ref.Tag
			} else {
				expectedRef = expectedSources[i].Ref.Branch
			}
			expectedSource := expectedSources[i].Url + "//terraform/" + component.Path + "?ref=" + expectedRef
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
		expectedComponents := []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "path/to/code",
			},
		}
		blueprintHandler.SetTerraformComponents(expectedComponents)

		// Resolve the component paths
		blueprint := blueprintHandler.blueprint.DeepCopy()
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

		blueprintHandler.SetSources([]blueprintv1alpha1.Source{{
			Name:       "test-source",
			Url:        "https://github.com/user/repo.git",
			PathPrefix: "terraform",
			Ref:        blueprintv1alpha1.Reference{Branch: "main"},
		}})

		blueprintHandler.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{{
			Source: "test-source",
			Path:   "module/path",
		}})

		blueprint := blueprintHandler.blueprint.DeepCopy()
		blueprintHandler.resolveComponentSources(blueprint)
		blueprintHandler.resolveComponentPaths(blueprint)

		if blueprint.TerraformComponents[0].Source != "https://github.com/user/repo.git//terraform/module/path?ref=main" {
			t.Errorf("Unexpected resolved source: %v", blueprint.TerraformComponents[0].Source)
		}

		if blueprint.TerraformComponents[0].FullPath != filepath.Join("/mock/project/root", ".windsor", ".tf_modules", "module/path") {
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

func TestBlueprintHandler_processBlueprintData(t *testing.T) {
	t.Run("ValidBlueprintData", func(t *testing.T) {
		blueprintHandler := NewBlueprintHandler(setupSafeMocks().Injector)
		_ = blueprintHandler.Initialize()

		data := []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
kustomize:
  - name: test-kustomization
    path: ./kustomize
`)

		var blueprint blueprintv1alpha1.Blueprint
		if err := blueprintHandler.processBlueprintData(data, &blueprint); err != nil {
			t.Fatalf("processBlueprintData() error: %v", err)
		}

		if got := blueprint.Kind; got != "Blueprint" {
			t.Errorf("Expected kind 'Blueprint', got %s", got)
		}
		if got := blueprint.ApiVersion; got != "v1alpha1" {
			t.Errorf("Expected apiVersion 'v1alpha1', got %s", got)
		}
		if got := blueprint.Metadata.Name; got != "test-blueprint" {
			t.Errorf("Expected metadata name 'test-blueprint', got %s", got)
		}
		if got := len(blueprint.Kustomizations); got != 1 || blueprint.Kustomizations[0].Name != "test-kustomization" {
			t.Errorf("Expected kustomization name 'test-kustomization', got %v", blueprint.Kustomizations)
		}
	})

	t.Run("ErrorHandlingInYamlUnmarshal", func(t *testing.T) {
		// Mock the yamlUnmarshal function to simulate an error scenario
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func(data []byte, v interface{}) error {
			return fmt.Errorf("mocked error in yamlUnmarshal")
		}

		blueprintHandler := NewBlueprintHandler(setupSafeMocks().Injector)
		_ = blueprintHandler.Initialize()

		data := []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
kustomize::
  - name: test-kustomization
    path: ./kustomize
`)

		var blueprint blueprintv1alpha1.Blueprint
		if err := blueprintHandler.processBlueprintData(data, &blueprint); err == nil || !strings.Contains(err.Error(), "mocked error in yamlUnmarshal") {
			t.Fatalf("Expected mocked unmarshalling error, got %v", err)
		}
	})

	t.Run("ErrorHandlingInYamlMarshal", func(t *testing.T) {
		// Mock the yamlMarshal function to simulate an error scenario
		originalYamlMarshal := yamlMarshal
		defer func() { yamlMarshal = originalYamlMarshal }()
		yamlMarshal = func(v interface{}) ([]byte, error) {
			return nil, fmt.Errorf("mocked error in yamlMarshal")
		}

		blueprintHandler := NewBlueprintHandler(setupSafeMocks().Injector)
		_ = blueprintHandler.Initialize()

		data := []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
kustomize:
  - name: test-kustomization
    path: ./kustomize
`)

		var blueprint blueprintv1alpha1.Blueprint
		if err := blueprintHandler.processBlueprintData(data, &blueprint); err == nil || !strings.Contains(err.Error(), "mocked error in yamlMarshal") {
			t.Fatalf("Expected mocked marshalling error, got %v", err)
		}
	})

	t.Run("ErrorHandlingInK8sYamlUnmarshal", func(t *testing.T) {
		// Mock the k8sYamlUnmarshal function to simulate an error scenario
		originalK8sYamlUnmarshal := k8sYamlUnmarshal
		defer func() { k8sYamlUnmarshal = originalK8sYamlUnmarshal }()
		k8sYamlUnmarshal = func(data []byte, v interface{}) error {
			return fmt.Errorf("mocked error in k8sYamlUnmarshal")
		}

		blueprintHandler := NewBlueprintHandler(setupSafeMocks().Injector)
		_ = blueprintHandler.Initialize()

		data := []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
kustomize:
  - name: test-kustomization
    path: ./kustomize
`)

		var blueprint blueprintv1alpha1.Blueprint
		if err := blueprintHandler.processBlueprintData(data, &blueprint); err == nil || !strings.Contains(err.Error(), "mocked error in k8sYamlUnmarshal") {
			t.Fatalf("Expected mocked k8s unmarshalling error, got %v", err)
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
