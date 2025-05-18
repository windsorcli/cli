package blueprint

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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

// =============================================================================
// Test Setup
// =============================================================================

type mockJsonnetVM struct {
	EvaluateFunc func(filename, snippet string) (string, error)
	TLACalls     []struct{ Key, Val string }
	ExtCalls     []struct{ Key, Val string }
}

var NewMockJsonnetVM = func(evaluateFunc func(filename, snippet string) (string, error)) JsonnetVM {
	return &mockJsonnetVM{
		EvaluateFunc: evaluateFunc,
		TLACalls:     make([]struct{ Key, Val string }, 0),
		ExtCalls:     make([]struct{ Key, Val string }, 0),
	}
}

func (m *mockJsonnetVM) TLACode(key, val string) {
	m.TLACalls = append(m.TLACalls, struct{ Key, Val string }{key, val})
}

func (m *mockJsonnetVM) ExtCode(key, val string) {
	m.ExtCalls = append(m.ExtCalls, struct{ Key, Val string }{key, val})
}

func (m *mockJsonnetVM) EvaluateAnonymousSnippet(filename, snippet string) (string, error) {
	if m.EvaluateFunc != nil {
		return m.EvaluateFunc(filename, snippet)
	}
	return "", nil
}

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
kustomize:
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

type Mocks struct {
	Injector      di.Injector
	Shell         *shell.MockShell
	ConfigHandler config.ConfigHandler
	Shims         *Shims
}

type SetupOptions struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

func setupShims(t *testing.T) *Shims {
	t.Helper()
	shims := NewShims()

	// Override only the functions needed for testing
	shims.ReadFile = func(name string) ([]byte, error) {
		switch {
		case strings.HasSuffix(name, "blueprint.jsonnet"):
			return []byte(safeBlueprintJsonnet), nil
		case strings.HasSuffix(name, "blueprint.yaml"):
			return []byte(safeBlueprintYAML), nil
		default:
			return nil, fmt.Errorf("file not found")
		}
	}

	shims.WriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	shims.Stat = func(name string) (os.FileInfo, error) {
		if strings.Contains(name, "blueprint.yaml") || strings.Contains(name, "blueprint.jsonnet") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	shims.MkdirAll = func(name string, perm fs.FileMode) error {
		return nil
	}

	shims.NewJsonnetVM = func() JsonnetVM {
		return NewMockJsonnetVM(func(filename, snippet string) (string, error) {
			return "", nil
		})
	}

	return shims
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	options := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	var injector di.Injector
	if options.Injector == nil {
		injector = di.NewMockInjector()
	} else {
		injector = options.Injector
	}

	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewYamlConfigHandler(injector)
	} else {
		configHandler = options.ConfigHandler
	}

	mockShell := shell.NewMockShell()

	injector.Register("shell", mockShell)
	injector.Register("configHandler", configHandler)

	defaultConfigStr := `
contexts:
  mock-context:
    dns:
      domain: mock.domain.com
    network:
      loadbalancer_ips:
        start: 192.168.1.1
        end: 192.168.1.100
    docker:
      registry_url: mock.registry.com
    cluster:
      workers:
        volumes:
          - ${WINDSOR_PROJECT_ROOT}/.volumes:/var/local
`

	configHandler.Initialize()
	configHandler.SetContext("mock-context")

	if err := configHandler.LoadConfigString(defaultConfigStr); err != nil {
		t.Fatalf("Failed to load default config string: %v", err)
	}
	if options.ConfigStr != "" {
		if err := configHandler.LoadConfigString(options.ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}

	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	shims := setupShims(t)

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		os.Unsetenv("WINDSOR_CONTEXT")

		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	return &Mocks{
		Injector:      injector,
		Shell:         mockShell,
		ConfigHandler: configHandler,
		Shims:         shims,
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBlueprintHandler_NewBlueprintHandler(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new blueprint handler
		handler, _ := setup(t)

		// Then the handler should be created successfully
		if handler == nil {
			t.Fatalf("Expected BlueprintHandler to be non-nil")
		}
	})

	t.Run("HasCorrectComponents", func(t *testing.T) {
		// Given a new blueprint handler and mocks
		handler, mocks := setup(t)

		// Then the handler should be created successfully
		if handler == nil {
			t.Fatalf("Expected BlueprintHandler to be non-nil")
		}

		// And it should be of the correct type
		if _, ok := handler.(*BaseBlueprintHandler); !ok {
			t.Errorf("Expected NewBlueprintHandler to return a BaseBlueprintHandler")
		}

		// And it should have the correct injector
		if baseHandler, ok := handler.(*BaseBlueprintHandler); ok {
			if baseHandler.injector != mocks.Injector {
				t.Errorf("Expected handler to have the correct injector")
			}
		} else {
			t.Errorf("Failed to cast handler to BaseBlueprintHandler")
		}
	})
}

func TestBlueprintHandler_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *Mocks) {
		t.Helper()
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new blueprint handler
		handler, _ := setup(t)

		// When initializing the handler
		err := handler.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Initialize() failed: %v", err)
		}

		// And the handler should have the correct project root
		expectedRoot, _ := handler.shell.GetProjectRoot()
		if handler.projectRoot != expectedRoot {
			t.Errorf("projectRoot = %q, want %q", handler.projectRoot, expectedRoot)
		}
	})

	t.Run("SetProjectNameInContext", func(t *testing.T) {
		// Given a blueprint handler and mocks
		handler, mocks := setup(t)

		// And a mock config handler that tracks project name setting
		projectNameSet := false
		mockConfigHandler, ok := mocks.ConfigHandler.(*config.MockConfigHandler)
		if ok {
			mockConfigHandler.SetContextValueFunc = func(key string, value any) error {
				projectRoot, _ := mocks.Shell.GetProjectRootFunc()
				expectedName := filepath.Base(projectRoot)
				if key == "projectName" && value == expectedName {
					projectNameSet = true
				}
				return nil
			}
		} else {
			handler.Initialize()
			projectName := mocks.ConfigHandler.GetString("projectName")
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			if projectName == filepath.Base(projectRoot) {
				projectNameSet = true
			}
		}

		// When initializing the handler
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Initialize() failed: %v", err)
		}

		// Then the project name should be set in the context
		if !projectNameSet {
			t.Error("Expected project name to be set in context")
		}
	})

	t.Run("ErrorSettingProjectName", func(t *testing.T) {
		// Given a mock config handler that returns an error
		mocks := setupMocks(t, &SetupOptions{
			ConfigHandler: config.NewMockConfigHandler(),
		})
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "projectName" {
				return fmt.Errorf("error setting project name")
			}
			return nil
		}

		// And a new blueprint handler
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()

		// Then the appropriate error should be returned
		if err == nil {
			t.Fatal("Initialize() succeeded, want error")
		}
		if err.Error() != "error setting project name in config: error setting project name" {
			t.Errorf("error = %q, want %q", err.Error(), "error setting project name in config: error setting project name")
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a mock injector with no config handler
		mocks := setupMocks(t)
		mocks.Injector.Register("configHandler", nil)

		// And a new blueprint handler
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()

		// Then the appropriate error should be returned
		if err == nil {
			t.Fatal("Initialize() succeeded, want error")
		}
		if err.Error() != "error resolving configHandler" {
			t.Errorf("error = %q, want %q", err.Error(), "error resolving configHandler")
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a mock injector with no shell
		mocks := setupMocks(t)
		mocks.Injector.Register("shell", nil)

		// And a new blueprint handler
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims

		// When initializing the handler
		err := handler.Initialize()

		// Then the appropriate error should be returned
		if err == nil {
			t.Fatal("Initialize() succeeded, want error")
		}
		if err.Error() != "error resolving shell" {
			t.Errorf("error = %q, want %q", err.Error(), "error resolving shell")
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a mock shell that returns an error getting project root
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		// When initializing the handler
		err := handler.Initialize()

		// Then the appropriate error should be returned
		if err == nil {
			t.Fatal("Initialize() succeeded, want error")
		}
		if err.Error() != "error getting project root: error getting project root" {
			t.Errorf("error = %q, want %q", err.Error(), "error getting project root: error getting project root")
		}
	})
}

func TestBlueprintHandler_LoadConfig(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *Mocks) {
		t.Helper()
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// When loading the config
		err := handler.LoadConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the metadata should be correctly loaded
		metadata := handler.GetMetadata()
		if metadata.Name != "test-blueprint" {
			t.Errorf("Expected name to be test-blueprint, got %s", metadata.Name)
		}
	})

	t.Run("CustomPathOverride", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock file system that tracks checked paths
		var checkedPaths []string
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			checkedPaths = append(checkedPaths, name)
			if strings.HasSuffix(name, ".jsonnet") {
				return []byte(safeBlueprintJsonnet), nil
			}
			return nil, os.ErrNotExist
		}

		// When loading config with a custom path
		customPath := "/custom/path/blueprint"
		err := handler.LoadConfig(customPath)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And both jsonnet and yaml paths should be checked
		expectedPaths := []string{
			customPath + ".jsonnet",
			customPath + ".yaml",
		}
		for _, expected := range expectedPaths {
			found := false
			for _, checked := range checkedPaths {
				if checked == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected path %s to be checked, but it wasn't. Checked paths: %v", expected, checkedPaths)
			}
		}
	})

	t.Run("DefaultBlueprint", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock file system that returns no existing files
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		handler.shims.NewJsonnetVM = func() JsonnetVM {
			return NewMockJsonnetVM(func(filename, snippet string) (string, error) {
				return "", nil
			})
		}

		// And a local context
		originalContext := os.Getenv("WINDSOR_CONTEXT")
		os.Setenv("WINDSOR_CONTEXT", "local")
		defer func() { os.Setenv("WINDSOR_CONTEXT", originalContext) }()

		// When loading the config
		err := handler.LoadConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the default metadata should be set correctly
		metadata := handler.GetMetadata()
		if metadata.Name != "local" {
			t.Errorf("Expected name to be 'local', got %s", metadata.Name)
		}
		if metadata.Description != "This blueprint outlines resources in the local context" {
			t.Errorf("Expected description to be 'This blueprint outlines resources in the local context', got %s", metadata.Description)
		}
	})

	t.Run("ErrorUnmarshallingLocalJsonnet", func(t *testing.T) {
		// Given a blueprint handler with local context
		handler, mocks := setup(t)
		mocks.ConfigHandler.SetContext("local")

		// And a mock yaml unmarshaller that returns an error
		handler.shims.YamlUnmarshal = func(data []byte, obj any) error {
			return fmt.Errorf("simulated unmarshalling error")
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected LoadConfig to fail due to unmarshalling error, but it succeeded")
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a mock config handler that returns an error
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting config root")
		}
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupMocks(t, opts)

		// And a blueprint handler using that config handler
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error getting config root") {
			t.Errorf("Expected error containing 'error getting config root', got: %v", err)
		}
	})

	t.Run("ErrorLoadingJsonnetFile", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock file system that returns an error for jsonnet files
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return nil, fmt.Errorf("error reading jsonnet file")
			}
			return nil, nil
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error reading jsonnet file") {
			t.Errorf("Expected error containing 'error reading jsonnet file', got: %v", err)
		}
	})

	t.Run("ErrorLoadingYamlFile", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock file system that returns an error for yaml files
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".yaml") {
				return nil, fmt.Errorf("error reading yaml file")
			}
			return nil, nil
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error reading yaml file") {
			t.Errorf("Expected error containing 'error reading yaml file', got: %v", err)
		}
	})

	t.Run("ErrorUnmarshallingYamlForLocalBlueprint", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock file system with a yaml file
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if filepath.Clean(name) == filepath.Clean("/mock/config/root/blueprint.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if filepath.Clean(name) == filepath.Clean("/mock/config/root/blueprint.yaml") {
				return []byte("valid: yaml"), nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// And a mock yaml unmarshaller that returns an error
		handler.shims.YamlUnmarshal = func(data []byte, obj any) error {
			return fmt.Errorf("simulated unmarshalling error")
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "simulated unmarshalling error") {
			t.Errorf("Expected error containing 'simulated unmarshalling error', got: %v", err)
		}
	})

	t.Run("ErrorMarshallingContextToJSON", func(t *testing.T) {
		// Given a blueprint handler with local context
		handler, mocks := setup(t)
		mocks.ConfigHandler.SetContext("local")

		// And a mock json marshaller that returns an error
		handler.shims.JsonMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("simulated marshalling error")
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "simulated marshalling error") {
			t.Errorf("Expected error containing 'simulated marshalling error', got: %v", err)
		}
	})

	t.Run("ErrorEvaluatingJsonnet", func(t *testing.T) {
		// Given a blueprint handler with local context
		handler, mocks := setup(t)
		mocks.ConfigHandler.SetContext("local")

		// And a mock jsonnet VM that returns an error
		handler.shims.NewJsonnetVM = func() JsonnetVM {
			return NewMockJsonnetVM(func(filename, snippet string) (string, error) {
				return "", fmt.Errorf("simulated jsonnet evaluation error")
			})
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "simulated jsonnet evaluation error") {
			t.Errorf("Expected error containing 'simulated jsonnet evaluation error', got: %v", err)
		}
	})

	t.Run("ErrorMarshallingLocalBlueprintYaml", func(t *testing.T) {
		// Given a blueprint handler with local context
		handler, mocks := setup(t)
		mocks.ConfigHandler.SetContext("local")

		// And a mock yaml marshaller that returns an error
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("simulated yaml marshalling error")
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "simulated yaml marshalling error") {
			t.Errorf("Expected error containing 'simulated yaml marshalling error', got: %v", err)
		}
	})

	t.Run("ErrorMarshallingLocalJson", func(t *testing.T) {
		// Given a blueprint handler with local context
		handler, mocks := setup(t)
		mocks.ConfigHandler.SetContext("local")

		// And a mock json marshaller that returns an error
		handler.shims.JsonMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("simulated json marshalling error")
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "simulated json marshalling error") {
			t.Errorf("Expected error containing 'simulated json marshalling error', got: %v", err)
		}
	})

	t.Run("ErrorGeneratingBlueprintFromLocalJsonnet", func(t *testing.T) {
		// Given a blueprint handler with local context
		handler, mocks := setup(t)
		mocks.ConfigHandler.SetContext("local")

		// And a mock file system that returns an error for jsonnet files
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return nil, fmt.Errorf("error reading jsonnet file")
			}
			return nil, fmt.Errorf("file not found")
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error reading jsonnet file") {
			t.Errorf("Expected error containing 'error reading jsonnet file', got: %v", err)
		}
	})

	t.Run("ErrorUnmarshallingYamlDataWithEvaluatedJsonnet", func(t *testing.T) {
		// Given a blueprint handler with local context
		handler, mocks := setup(t)
		mocks.ConfigHandler.SetContext("local")

		// And a mock yaml unmarshaller that returns an error
		handler.shims.YamlUnmarshal = func(data []byte, obj any) error {
			return fmt.Errorf("simulated unmarshalling error")
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "simulated unmarshalling error") {
			t.Errorf("Expected error containing 'simulated unmarshalling error', got: %v", err)
		}
	})

	t.Run("ExistingYamlFilePreference", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock file system with a yaml file
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".yaml") {
				return []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
  description: A test blueprint
  authors:
    - John Doe
`), nil
			}
			return nil, os.ErrNotExist
		}

		// And a mock jsonnet VM that returns empty output
		handler.shims.NewJsonnetVM = func() JsonnetVM {
			return NewMockJsonnetVM(func(filename, snippet string) (string, error) {
				return "", nil
			})
		}

		// And a test context
		originalContext := os.Getenv("WINDSOR_CONTEXT")
		os.Setenv("WINDSOR_CONTEXT", "test")
		defer func() { os.Setenv("WINDSOR_CONTEXT", originalContext) }()

		// When loading the config
		err := handler.LoadConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// And the metadata should be loaded from the yaml file
		metadata := handler.GetMetadata()
		if metadata.Name != "test-blueprint" {
			t.Errorf("Expected name to be test-blueprint, got %s", metadata.Name)
		}
		if metadata.Description != "A test blueprint" {
			t.Errorf("Expected description to be 'A test blueprint', got %s", metadata.Description)
		}
		if len(metadata.Authors) != 1 || metadata.Authors[0] != "John Doe" {
			t.Errorf("Expected authors to be ['John Doe'], got %v", metadata.Authors)
		}
	})

	t.Run("EmptyEvaluatedJsonnet", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// And a mock config handler that returns local context
		mocks.ConfigHandler.SetContext("local")

		// And a mock file system with no files
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}

		// And a mock jsonnet VM that returns empty output
		handler.shims.NewJsonnetVM = func() JsonnetVM {
			return NewMockJsonnetVM(func(filename, snippet string) (string, error) {
				return "", nil
			})
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error for empty evaluated jsonnet, got: %v", err)
		}

		// And the metadata should be correctly loaded
		metadata := handler.GetMetadata()
		if metadata.Name != "local" {
			t.Errorf("Expected blueprint name to be 'local', got: %s", metadata.Name)
		}
		expectedDesc := "This blueprint outlines resources in the local context"
		if metadata.Description != expectedDesc {
			t.Errorf("Expected description '%s', got: %s", expectedDesc, metadata.Description)
		}
	})

	t.Run("ErrorEvaluatingDefaultJsonnet", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock file system with no files
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// And a mock jsonnet VM that returns an error for default template
		handler.shims.NewJsonnetVM = func() JsonnetVM {
			return NewMockJsonnetVM(func(filename, snippet string) (string, error) {
				if filename == "default.jsonnet" {
					return "", fmt.Errorf("error evaluating default jsonnet template")
				}
				return "", nil
			})
		}

		// And a local context
		originalContext := os.Getenv("WINDSOR_CONTEXT")
		os.Setenv("WINDSOR_CONTEXT", "local")
		defer func() { os.Setenv("WINDSOR_CONTEXT", originalContext) }()

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error generating blueprint from default jsonnet") {
			t.Errorf("Expected error containing 'error generating blueprint from default jsonnet', got: %v", err)
		}
	})

	t.Run("ErrorUnmarshallingContextYAML", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock yaml unmarshaller that returns an error for context YAML
		handler.shims.YamlUnmarshal = func(data []byte, obj any) error {
			if _, ok := obj.(map[string]any); ok {
				return fmt.Errorf("error unmarshalling context YAML")
			}
			return nil
		}

		// And a mock file system that returns a blueprint file
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// And a mock jsonnet VM that returns an error
		handler.shims.NewJsonnetVM = func() JsonnetVM {
			return NewMockJsonnetVM(func(filename, snippet string) (string, error) {
				return "", fmt.Errorf("error evaluating jsonnet")
			})
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error evaluating jsonnet") {
			t.Errorf("Expected error containing 'error evaluating jsonnet', got: %v", err)
		}
	})

	t.Run("EmptyEvaluatedJsonnet", func(t *testing.T) {
		// Given a blueprint handler with local context
		handler, mocks := setup(t)
		mocks.ConfigHandler.SetContext("local")

		// And a mock jsonnet VM that returns empty result
		handler.shims.NewJsonnetVM = func() JsonnetVM {
			return NewMockJsonnetVM(func(filename, snippet string) (string, error) {
				return "", nil
			})
		}

		// And a mock file system that returns no files
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}

		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error for empty evaluated jsonnet, got: %v", err)
		}

		// And the default metadata should be set correctly
		metadata := handler.GetMetadata()
		if metadata.Name != "local" {
			t.Errorf("Expected blueprint name to be 'local', got: %s", metadata.Name)
		}
		expectedDesc := "This blueprint outlines resources in the local context"
		if metadata.Description != expectedDesc {
			t.Errorf("Expected description '%s', got: %s", expectedDesc, metadata.Description)
		}
	})

	t.Run("ErrorMarshallingYamlNonNull", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// And a mock yaml marshaller that returns an error
		mocks.Shims.YamlMarshalNonNull = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("mock error marshalling yaml non null")
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error when marshalling yaml non null, got nil")
		}
		if !strings.Contains(err.Error(), "mock error marshalling yaml non null") {
			t.Errorf("Expected error containing 'mock error marshalling yaml non null', got: %v", err)
		}
	})
}

func TestBlueprintHandler_WriteConfig(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a blueprint handler with metadata
		handler, mocks := setup(t)
		expectedMetadata := blueprintv1alpha1.Metadata{
			Name:        "test-blueprint",
			Description: "A test blueprint",
			Authors:     []string{"John Doe"},
		}

		handler.SetMetadata(expectedMetadata)

		// And a mock file system that captures written data
		var capturedData []byte
		mocks.Shims.WriteFile = func(name string, data []byte, perm fs.FileMode) error {
			capturedData = data
			return nil
		}

		// When writing the config
		err := handler.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected WriteConfig to succeed, got error: %v", err)
		}

		// And data should be written
		if len(capturedData) == 0 {
			t.Error("Expected data to be written, but no data was captured")
		}

		// And the written data should match the expected blueprint
		var writtenBlueprint blueprintv1alpha1.Blueprint
		err = yaml.Unmarshal(capturedData, &writtenBlueprint)
		if err != nil {
			t.Fatalf("Failed to unmarshal captured blueprint data: %v", err)
		}

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

	t.Run("WriteNoPath", func(t *testing.T) {
		// Given a blueprint handler with metadata
		handler, mocks := setup(t)
		expectedMetadata := blueprintv1alpha1.Metadata{
			Name:        "test-blueprint",
			Description: "A test blueprint",
			Authors:     []string{"John Doe"},
		}
		handler.SetMetadata(expectedMetadata)

		// And a mock file system that captures written data
		var capturedData []byte
		mocks.Shims.WriteFile = func(name string, data []byte, perm fs.FileMode) error {
			capturedData = data
			return nil
		}

		// When writing the config without a path
		err := handler.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Failed to write blueprint configuration: %v", err)
		}

		// And data should be written
		if len(capturedData) == 0 {
			t.Error("Expected data to be written, but no data was captured")
		}

		// And the written data should match the expected blueprint
		var writtenBlueprint blueprintv1alpha1.Blueprint
		err = yaml.Unmarshal(capturedData, &writtenBlueprint)
		if err != nil {
			t.Fatalf("Failed to unmarshal captured blueprint data: %v", err)
		}

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
		// Given a mock config handler that returns an error
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error")
		}
		opts := &SetupOptions{
			ConfigHandler: configHandler,
		}
		mocks := setupMocks(t, opts)

		// And a blueprint handler using that config
		handler := NewBlueprintHandler(mocks.Injector)
		handler.Initialize()

		// When writing the config
		err := handler.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error when loading config, got nil")
		}
		if err.Error() != "error getting config root: mock error" {
			t.Errorf("error = %q, want %q", err.Error(), "error getting config root: mock error")
		}
	})

	t.Run("ErrorCreatingDirectory", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// And a mock file system that fails to create directories
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When writing the config
		err := handler.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error when writing config, got nil")
		}
		if err.Error() != "error creating directory: mock error creating directory" {
			t.Errorf("error = %q, want %q", err.Error(), "error creating directory: mock error creating directory")
		}
	})

	t.Run("ErrorMarshallingYaml", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// And a mock yaml marshaller that returns an error
		mocks.Shims.YamlMarshalNonNull = func(in any) ([]byte, error) {
			return nil, fmt.Errorf("mock error marshalling yaml")
		}

		// When writing the config
		err := handler.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error when marshalling yaml, got nil")
		}
		if !strings.Contains(err.Error(), "error marshalling yaml") {
			t.Errorf("Expected error message to contain 'error marshalling yaml', got '%v'", err)
		}
	})

	t.Run("ErrorWritingFile", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// And a mock file system that fails to write files
		mocks.Shims.WriteFile = func(name string, data []byte, perm fs.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}

		// When writing the config
		err := handler.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error when writing file, got nil")
		}
		if !strings.Contains(err.Error(), "error writing blueprint file") {
			t.Errorf("Expected error message to contain 'error writing blueprint file', got '%v'", err)
		}
	})

	t.Run("CleanupEmptyPostBuild", func(t *testing.T) {
		// Given a blueprint handler with kustomizations containing empty PostBuild
		handler, mocks := setup(t)
		emptyPostBuildKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name: "kustomization-empty-postbuild",
				Path: "path/to/kustomize",
				PostBuild: &blueprintv1alpha1.PostBuild{
					Substitute:     map[string]string{},
					SubstituteFrom: []blueprintv1alpha1.SubstituteReference{},
				},
			},
			{
				Name: "kustomization-with-substitutes",
				Path: "path/to/kustomize2",
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
		handler.SetKustomizations(emptyPostBuildKustomizations)

		// And a mock yaml marshaller that captures the blueprint
		var capturedBlueprint *blueprintv1alpha1.Blueprint
		mocks.Shims.YamlMarshalNonNull = func(v any) ([]byte, error) {
			if bp, ok := v.(*blueprintv1alpha1.Blueprint); ok {
				capturedBlueprint = bp
			}
			return []byte{}, nil
		}

		// When writing the config
		err := handler.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Failed to write blueprint configuration: %v", err)
		}

		// And the kustomizations should be properly cleaned up
		if capturedBlueprint == nil {
			t.Fatal("Expected blueprint to be captured, but it was nil")
		}

		if len(capturedBlueprint.Kustomizations) != 2 {
			t.Fatalf("Expected 2 kustomizations, got %d", len(capturedBlueprint.Kustomizations))
		}

		// And empty PostBuild should be removed
		if capturedBlueprint.Kustomizations[0].PostBuild != nil {
			t.Errorf("Expected PostBuild to be nil for kustomization with empty PostBuild, got %v",
				capturedBlueprint.Kustomizations[0].PostBuild)
		}

		// And non-empty PostBuild should be preserved
		if capturedBlueprint.Kustomizations[1].PostBuild == nil {
			t.Errorf("Expected PostBuild to be preserved for kustomization with substitutes")
		} else if len(capturedBlueprint.Kustomizations[1].PostBuild.SubstituteFrom) != 1 {
			t.Errorf("Expected 1 SubstituteFrom entry, got %d",
				len(capturedBlueprint.Kustomizations[1].PostBuild.SubstituteFrom))
		}
	})

	t.Run("ClearTerraformComponentsVariablesAndValues", func(t *testing.T) {
		// Given a blueprint handler with terraform components containing variables and values
		handler, mocks := setup(t)
		terraformComponents := []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "path/to/code",
				Values: map[string]any{
					"key1": "val1",
					"key2": true,
				},
			},
		}
		handler.SetTerraformComponents(terraformComponents)

		// And a mock yaml marshaller that captures the blueprint
		var capturedBlueprint *blueprintv1alpha1.Blueprint
		mocks.Shims.YamlMarshalNonNull = func(v any) ([]byte, error) {
			if bp, ok := v.(*blueprintv1alpha1.Blueprint); ok {
				capturedBlueprint = bp
			}
			return []byte{}, nil
		}

		// When writing the config
		err := handler.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Failed to write blueprint configuration: %v", err)
		}

		// And the blueprint should be captured
		if capturedBlueprint == nil {
			t.Fatal("Expected blueprint to be captured, but it was nil")
		}

		// And the terraform components should be properly cleaned up
		if len(capturedBlueprint.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(capturedBlueprint.TerraformComponents))
		}

		// And variables and values should be cleared
		component := capturedBlueprint.TerraformComponents[0]
		if component.Values != nil {
			t.Error("Expected Values to be nil after write")
		}

		// And other fields should be preserved
		if component.Source != "source1" {
			t.Errorf("Expected Source to be 'source1', got %s", component.Source)
		}
		if component.Path != "path/to/code" {
			t.Errorf("Expected Path to be 'path/to/code', got %s", component.Path)
		}
	})
}

func TestBlueprintHandler_Install(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a mock Kubernetes client that validates resource types
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
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

		// And a blueprint handler with repository, sources, and kustomizations
		handler, _ := setup(t)

		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.SetSources(expectedSources)

		expectedKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name:      "kustomization1",
				DependsOn: []string{"dependency1", "dependency2"},
			},
		}
		handler.SetKustomizations(expectedKustomizations)

		// When installing the blueprint
		err = handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected successful installation, but got error: %v", err)
		}
	})

	t.Run("SourceURLWithoutDotGit", func(t *testing.T) {
		// Given a mock Kubernetes client that accepts any resource
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			return nil
		}

		// And a blueprint handler with repository and source without .git suffix
		handler, _ := setup(t)

		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source2",
				Url:  "https://example.com/source2",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.SetSources(expectedSources)

		// When installing the blueprint
		err = handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected successful installation with .git URL, but got error: %v", err)
		}
	})

	t.Run("SourceWithSecretName", func(t *testing.T) {
		// Given a mock Kubernetes client that accepts any resource
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			return nil
		}

		// And a blueprint handler with repository and source with secret name
		handler, _ := setup(t)

		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		expectedSources := []blueprintv1alpha1.Source{
			{
				Name:       "source3",
				Url:        "https://example.com/source3.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				SecretName: "my-secret",
			},
		}
		handler.SetSources(expectedSources)

		// When installing the blueprint
		err = handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected successful installation with SecretName, but got error: %v", err)
		}
	})

	t.Run("EmptyLocalVolumePaths", func(t *testing.T) {
		// Given a mock Kubernetes client that validates ConfigMap data
		configMapApplied := false
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "configmaps" {
				configMapApplied = true

				configMap, ok := config.ResourceObject.(*corev1.ConfigMap)
				if !ok {
					return fmt.Errorf("unexpected resource object type")
				}

				if configMap.Data["LOCAL_VOLUME_PATH"] != "" {
					return fmt.Errorf("expected empty LOCAL_VOLUME_PATH value, but got: %s", configMap.Data["LOCAL_VOLUME_PATH"])
				}
			}
			return nil
		}

		// And a mock config handler that returns empty volume paths
		mockConfigHandler := config.NewMockConfigHandler()
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupMocks(t, opts)

		mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{}
			}
			return []string{"default value"}
		}

		// And a blueprint handler with sources
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}

		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.SetSources(expectedSources)

		// When installing the blueprint
		err = handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected successful installation, but got error: %v", err)
		}

		// And the ConfigMap should be applied
		if !configMapApplied {
			t.Fatalf("Expected ConfigMap to be applied, but it was not")
		}
	})

	t.Run("ApplyGitRepoError", func(t *testing.T) {
		// Given a mock Kubernetes client that returns an error for a specific GitRepository
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "gitrepositories" {
				gitRepo, ok := config.ResourceObject.(*sourcev1.GitRepository)
				if !ok {
					return fmt.Errorf("unexpected resource object type")
				}
				if gitRepo.Name == "primary-repo" {
					return fmt.Errorf("mock error applying primary GitRepository")
				}
			}
			return nil
		}

		// And a blueprint handler with sources
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		expectedRepository := blueprintv1alpha1.Repository{
			Url: "git::https://example.com/primary-repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}
		err := handler.SetSources([]blueprintv1alpha1.Source{
			{
				Name: "primary-repo",
				Url:  expectedRepository.Url,
				Ref:  expectedRepository.Ref,
			},
		})
		if err != nil {
			t.Fatalf("Failed to set sources: %v", err)
		}

		err = handler.Install()
		if err == nil || !strings.Contains(err.Error(), "mock error applying primary GitRepository") {
			t.Fatalf("Expected error when applying primary GitRepository, but got: %v", err)
		}
	})

	t.Run("EmptySourceUrlError", func(t *testing.T) {
		// Given a blueprint handler with a source that has an empty URL
		handler, _ := setup(t)

		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.SetSources(expectedSources)

		// When installing the blueprint
		err := handler.Install()

		// Then an error about empty source URL should be returned
		if err == nil || !strings.Contains(err.Error(), "source URL cannot be empty") {
			t.Fatalf("Expected error for empty source URL, but got: %v", err)
		}
	})

	t.Run("EmptyRepositoryURL", func(t *testing.T) {
		// Given a blueprint handler with an empty repository URL
		handler, _ := setup(t)

		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			return nil
		}

		// When installing the blueprint
		err = handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error for empty repository URL, got: %v", err)
		}
	})

	t.Run("ValidRepository", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock Kubernetes client that tracks GitRepository creation
		gitRepoApplied := false
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "gitrepositories" {
				gitRepo, ok := config.ResourceObject.(*sourcev1.GitRepository)
				if !ok {
					return fmt.Errorf("unexpected resource type")
				}
				if gitRepo.Name == "mock-context" {
					gitRepoApplied = true
				}
			}
			return nil
		}

		// And a valid repository configuration
		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		// When installing the blueprint
		err = handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And the GitRepository should be created
		if !gitRepoApplied {
			t.Error("Expected GitRepository to be applied, but it wasn't")
		}
	})

	t.Run("NoRepository", func(t *testing.T) {
		// Given a blueprint handler without a repository
		handler, _ := setup(t)

		// And a mock Kubernetes client that tracks GitRepository creation attempts
		gitRepoAttempted := false
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "gitrepositories" {
				gitRepoAttempted = true
			}
			return nil
		}

		// When installing the blueprint
		err := handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error when no repository is defined, got: %v", err)
		}

		// And no GitRepository should be created
		if gitRepoAttempted {
			t.Error("Expected no GitRepository to be applied when no repository is defined")
		}
	})

	t.Run("ErrorApplyingGitRepository", func(t *testing.T) {
		// Given a mock Kubernetes client that fails to apply GitRepository resources
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "gitrepositories" {
				return fmt.Errorf("mock error applying GitRepository")
			}
			return nil
		}

		// And a blueprint handler with a repository
		handler, _ := setup(t)

		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		// And sources and kustomizations are configured
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.SetSources(expectedSources)

		expectedKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name:      "kustomization1",
				DependsOn: []string{"dependency1", "dependency2"},
			},
		}
		handler.SetKustomizations(expectedKustomizations)

		// When installing the blueprint
		err = handler.Install()

		// Then an error about applying GitRepository should be returned
		if err == nil || !strings.Contains(err.Error(), "mock error applying GitRepository") {
			t.Fatalf("Expected error when applying GitRepository, but got: %v", err)
		}
	})

	t.Run("ErrorApplyingKustomization", func(t *testing.T) {
		// Given a mock Kubernetes client that fails to apply Kustomization resources
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "kustomizations" {
				return fmt.Errorf("mock error applying Kustomization")
			}
			return nil
		}

		// And a blueprint handler with repository, sources, and kustomizations
		handler, _ := setup(t)

		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.SetSources(expectedSources)

		expectedKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name:      "kustomization1",
				DependsOn: []string{"dependency1", "dependency2"},
			},
		}
		handler.SetKustomizations(expectedKustomizations)

		// When installing the blueprint
		err = handler.Install()

		// Then an error about applying Kustomization should be returned
		if err == nil || !strings.Contains(err.Error(), "mock error applying Kustomization") {
			t.Fatalf("Expected error when applying Kustomization, but got: %v", err)
		}
	})

	t.Run("ErrorApplyingConfigMap", func(t *testing.T) {
		// Given a mock Kubernetes client that fails to apply ConfigMap resources
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "configmaps" {
				return fmt.Errorf("mock error applying ConfigMap")
			}
			return nil
		}

		// And a blueprint handler with repository and sources
		handler, _ := setup(t)

		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.SetSources(expectedSources)

		// When installing the blueprint
		err = handler.Install()

		// Then an error about applying ConfigMap should be returned
		if err == nil || !strings.Contains(err.Error(), "mock error applying ConfigMap") {
			t.Fatalf("Expected error when applying ConfigMap, but got: %v", err)
		}
	})

	t.Run("SuccessApplyingConfigMap", func(t *testing.T) {
		// Given a mock Kubernetes client that validates ConfigMap data
		configMapApplied := false
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "configmaps" {
				configMapApplied = true

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

		// And a blueprint handler with repository and sources
		handler, _ := setup(t)

		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.SetSources(expectedSources)

		// When installing the blueprint
		err = handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected successful installation, but got error: %v", err)
		}

		// And the ConfigMap should be applied with correct values
		if !configMapApplied {
			t.Fatalf("Expected ConfigMap to be applied, but it was not")
		}
	})

	t.Run("EmptyLocalVolumePaths", func(t *testing.T) {
		// Given a mock Kubernetes client that validates ConfigMap data
		configMapApplied := false
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "configmaps" {
				configMapApplied = true
				configMap, ok := config.ResourceObject.(*corev1.ConfigMap)
				if !ok {
					return fmt.Errorf("unexpected resource object type")
				}
				if configMap.Data["LOCAL_VOLUME_PATH"] != "" {
					return fmt.Errorf("expected empty LOCAL_VOLUME_PATH value, but got: %s", configMap.Data["LOCAL_VOLUME_PATH"])
				}
			}
			return nil
		}

		// And a blueprint handler with empty volume paths
		handler, mocks := setup(t)
		mocks.ConfigHandler.LoadConfigString(`
contexts:
  mock-context:
    cluster:
      workers:
        volumes: []
`)

		// When installing the blueprint
		err := handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected successful installation, but got error: %v", err)
		}

		// And the ConfigMap should be applied with empty LOCAL_VOLUME_PATH
		if !configMapApplied {
			t.Fatalf("Expected ConfigMap to be applied, but it was not")
		}
	})

	t.Run("EmptySourceUrlError", func(t *testing.T) {
		// Given a blueprint handler with repository
		handler, _ := setup(t)

		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		// And a source with empty URL
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.SetSources(expectedSources)

		// When installing the blueprint
		err = handler.Install()

		// Then an error about empty source URL should be returned
		if err == nil || !strings.Contains(err.Error(), "source URL cannot be empty") {
			t.Fatalf("Expected error for empty source URL, but got: %v", err)
		}
	})
}

func TestBlueprintHandler_GetMetadata(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And metadata has been set
		expectedMetadata := blueprintv1alpha1.Metadata{
			Name:        "test-blueprint",
			Description: "A test blueprint",
			Authors:     []string{"John Doe"},
		}
		handler.SetMetadata(expectedMetadata)

		// When getting the metadata
		actualMetadata := handler.GetMetadata()

		// Then it should match the expected metadata
		if !reflect.DeepEqual(actualMetadata, expectedMetadata) {
			t.Errorf("Expected metadata to be %v, but got %v", expectedMetadata, actualMetadata)
		}
	})
}

func TestBlueprintHandler_GetSources(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)

		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And sources have been set
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.SetSources(expectedSources)

		// When getting the sources
		actualSources := handler.GetSources()

		// Then they should match the expected sources
		if !reflect.DeepEqual(actualSources, expectedSources) {
			t.Errorf("Expected sources to be %v, but got %v", expectedSources, actualSources)
		}
	})
}

func TestBlueprintHandler_GetTerraformComponents(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a blueprint handler and mocks
		handler, mocks := setup(t)

		// And a project root is available
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}

		// And terraform components have been set
		expectedComponents := []blueprintv1alpha1.TerraformComponent{
			{
				Source:   "source1",
				Path:     "path/to/code",
				FullPath: filepath.Join(projectRoot, "terraform", "path/to/code"),
				Values: map[string]any{
					"key1": "value1",
				},
			},
		}
		handler.SetTerraformComponents(expectedComponents)

		// When getting the terraform components
		actualComponents := handler.GetTerraformComponents()

		// Then they should match the expected components
		if !reflect.DeepEqual(actualComponents, expectedComponents) {
			t.Errorf("Expected Terraform components to be %v, but got %v", expectedComponents, actualComponents)
		}
	})
}

func TestBlueprintHandler_GetKustomizations(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And kustomizations have been set
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
		handler.SetKustomizations(inputKustomizations)

		// When getting the kustomizations
		actualKustomizations := handler.GetKustomizations()

		// Then they should match the expected kustomizations
		expectedKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name:          "kustomization1",
				Path:          "kustomize/overlays/dev",
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
		if !reflect.DeepEqual(actualKustomizations, expectedKustomizations) {
			t.Errorf("Expected Kustomizations to be %v, but got %v", expectedKustomizations, actualKustomizations)
		}
	})

	t.Run("NilKustomizations", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And kustomizations are set to nil
		handler.SetKustomizations(nil)

		// When getting the kustomizations
		actualKustomizations := handler.GetKustomizations()

		// Then they should be nil
		if actualKustomizations != nil {
			t.Errorf("Expected Kustomizations to be nil, but got %v", actualKustomizations)
		}
	})
}

func TestBlueprintHandler_GetRepository(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("DefaultRepository", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// When getting the repository
		repository := handler.GetRepository()

		// Then it should have default values
		if repository.Url != "" {
			t.Errorf("Expected empty URL, got %s", repository.Url)
		}
		if repository.Ref.Branch != "main" {
			t.Errorf("Expected branch 'main', got %s", repository.Ref.Branch)
		}
	})

	t.Run("CustomRepository", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// And a mock file system with a custom repository configuration
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".yaml") {
				return []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
  description: A test blueprint
  authors:
    - John Doe
repository:
  url: git::https://example.com/custom-repo.git
  ref:
    branch: develop
`), nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.NewJsonnetVM = func() JsonnetVM {
			return NewMockJsonnetVM(func(filename, snippet string) (string, error) {
				return "", nil
			})
		}

		originalContext := os.Getenv("WINDSOR_CONTEXT")
		os.Setenv("WINDSOR_CONTEXT", "test")
		defer func() { os.Setenv("WINDSOR_CONTEXT", originalContext) }()

		// And the config is loaded
		err := handler.LoadConfig()
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// When getting the repository
		repository := handler.GetRepository()

		// Then it should match the custom configuration
		if repository.Url != "git::https://example.com/custom-repo.git" {
			t.Errorf("Expected URL 'git::https://example.com/custom-repo.git', got %s", repository.Url)
		}
		if repository.Ref.Branch != "develop" {
			t.Errorf("Expected branch 'develop', got %s", repository.Ref.Branch)
		}
	})
}

func TestBlueprintHandler_SetRepository(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a test repository configuration
		testRepo := blueprintv1alpha1.Repository{
			Url: "git::https://example.com/test-repo.git",
			Ref: blueprintv1alpha1.Reference{
				Branch: "feature/test",
			},
		}

		// When setting the repository
		err := handler.SetRepository(testRepo)

		// Then no error should be returned
		if err != nil {
			t.Errorf("SetRepository failed: %v", err)
		}

		// And the repository should match the test configuration
		repo := handler.GetRepository()
		if repo.Url != testRepo.Url {
			t.Errorf("Expected URL %s, got %s", testRepo.Url, repo.Url)
		}
		if repo.Ref.Branch != testRepo.Ref.Branch {
			t.Errorf("Expected branch %s, got %s", testRepo.Ref.Branch, repo.Ref.Branch)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestBlueprintHandler_resolveComponentSources(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock resource operation that tracks applied sources
		var appliedSources []string
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			if repo, ok := config.ResourceObject.(*sourcev1.GitRepository); ok {
				appliedSources = append(appliedSources, repo.Spec.URL)
			}
			return nil
		}

		// And sources have been set
		sources := []blueprintv1alpha1.Source{
			{
				Name:       "source1",
				Url:        "git::https://example.com/source1.git",
				PathPrefix: "terraform",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.SetSources(sources)

		// And terraform components have been set
		components := []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "path/to/code",
			},
		}
		handler.SetTerraformComponents(components)

		// When installing the components
		err := handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected successful installation, but got error: %v", err)
		}

		// And the source URL should be applied
		expectedURL := "git::https://example.com/source1.git"
		found := false
		for _, url := range appliedSources {
			if strings.TrimPrefix(url, "https://") == expectedURL {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected source URL %s to be applied, but it wasn't. Applied sources: %v", expectedURL, appliedSources)
		}
	})

	t.Run("DefaultPathPrefix", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)

		// And sources have been set without a path prefix
		handler.SetSources([]blueprintv1alpha1.Source{{
			Name: "test-source",
			Url:  "https://github.com/user/repo.git",
			Ref:  blueprintv1alpha1.Reference{Branch: "main"},
		}})

		// And terraform components have been set
		handler.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{{
			Source: "test-source",
			Path:   "module/path",
		}})

		// When resolving component sources
		blueprint := baseHandler.blueprint.DeepCopy()
		baseHandler.resolveComponentSources(blueprint)

		// Then the default path prefix should be used
		expectedSource := "https://github.com/user/repo.git//terraform/module/path?ref=main"
		if blueprint.TerraformComponents[0].Source != expectedSource {
			t.Errorf("Expected source URL %s, got %s", expectedSource, blueprint.TerraformComponents[0].Source)
		}
	})
}

func TestBlueprintHandler_resolveComponentPaths(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)

		// And terraform components have been set
		expectedComponents := []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "path/to/code",
			},
		}
		handler.SetTerraformComponents(expectedComponents)

		// When resolving component paths
		blueprint := baseHandler.blueprint.DeepCopy()
		baseHandler.resolveComponentPaths(blueprint)

		// Then each component should have the correct full path
		for _, component := range blueprint.TerraformComponents {
			expectedPath := filepath.Join(baseHandler.projectRoot, "terraform", component.Path)
			if component.FullPath != expectedPath {
				t.Errorf("Expected component path to be %v, but got %v", expectedPath, component.FullPath)
			}
		}
	})

	t.Run("isValidTerraformRemoteSource", func(t *testing.T) {
		handler, _ := setup(t)

		// Given a set of test cases for terraform source validation
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

		// When validating each source
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Then the validation result should match the expected outcome
				if got := handler.(*BaseBlueprintHandler).isValidTerraformRemoteSource(tt.source); got != tt.want {
					t.Errorf("isValidTerraformRemoteSource(%s) = %v, want %v", tt.source, got, tt.want)
				}
			})
		}
	})

	t.Run("ValidRemoteSourceWithFullPath", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)

		// And a source with URL and path prefix
		handler.SetSources([]blueprintv1alpha1.Source{{
			Name:       "test-source",
			Url:        "https://github.com/user/repo.git",
			PathPrefix: "terraform",
			Ref:        blueprintv1alpha1.Reference{Branch: "main"},
		}})

		// And a terraform component referencing that source
		handler.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{{
			Source: "test-source",
			Path:   "module/path",
		}})

		// When resolving component sources and paths
		blueprint := baseHandler.blueprint.DeepCopy()
		baseHandler.resolveComponentSources(blueprint)
		baseHandler.resolveComponentPaths(blueprint)

		// Then the source should be properly resolved
		if blueprint.TerraformComponents[0].Source != "https://github.com/user/repo.git//terraform/module/path?ref=main" {
			t.Errorf("Unexpected resolved source: %v", blueprint.TerraformComponents[0].Source)
		}

		// And the full path should be correctly constructed
		expectedPath := filepath.Join(baseHandler.projectRoot, ".windsor", ".tf_modules", "module/path")
		if blueprint.TerraformComponents[0].FullPath != expectedPath {
			t.Errorf("Unexpected full path: %v", blueprint.TerraformComponents[0].FullPath)
		}
	})

	t.Run("RegexpMatchStringError", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)

		// And a mock regexp matcher that returns an error
		originalRegexpMatchString := baseHandler.shims.RegexpMatchString
		defer func() { baseHandler.shims.RegexpMatchString = originalRegexpMatchString }()
		baseHandler.shims.RegexpMatchString = func(pattern, s string) (bool, error) {
			return false, fmt.Errorf("mocked error in regexpMatchString")
		}

		// When validating an invalid regex pattern
		if got := baseHandler.isValidTerraformRemoteSource("[invalid-regex"); got != false {
			t.Errorf("isValidTerraformRemoteSource([invalid-regex) = %v, want %v", got, false)
		}
	})
}

func TestBlueprintHandler_processBlueprintData(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("ValidBlueprintData", func(t *testing.T) {
		// Given a blueprint handler and an empty blueprint
		handler, _ := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources:             []blueprintv1alpha1.Source{},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{},
			Kustomizations:      []blueprintv1alpha1.Kustomization{},
		}

		// And valid blueprint data
		data := []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
  description: A test blueprint
  authors:
    - John Doe
sources:
  - name: test-source
    url: git::https://example.com/test-repo.git
terraform:
  - source: test-source
    path: path/to/code
kustomize:
  - name: test-kustomization
    path: ./kustomize
repository:
  url: git::https://example.com/test-repo.git
  ref:
    branch: main
`)

		// When processing the blueprint data
		baseHandler := handler.(*BaseBlueprintHandler)
		err := baseHandler.processBlueprintData(data, blueprint)

		// Then no error should be returned
		if err != nil {
			t.Errorf("processBlueprintData failed: %v", err)
		}

		// And the metadata should be correctly set
		if blueprint.Metadata.Name != "test-blueprint" {
			t.Errorf("Expected name 'test-blueprint', got %s", blueprint.Metadata.Name)
		}
		if blueprint.Metadata.Description != "A test blueprint" {
			t.Errorf("Expected description 'A test blueprint', got %s", blueprint.Metadata.Description)
		}
		if len(blueprint.Metadata.Authors) != 1 || blueprint.Metadata.Authors[0] != "John Doe" {
			t.Errorf("Expected authors ['John Doe'], got %v", blueprint.Metadata.Authors)
		}

		// And the sources should be correctly set
		if len(blueprint.Sources) != 1 || blueprint.Sources[0].Name != "test-source" {
			t.Errorf("Expected one source named 'test-source', got %v", blueprint.Sources)
		}

		// And the terraform components should be correctly set
		if len(blueprint.TerraformComponents) != 1 || blueprint.TerraformComponents[0].Source != "test-source" {
			t.Errorf("Expected one component with source 'test-source', got %v", blueprint.TerraformComponents)
		}

		// And the kustomizations should be correctly set
		if len(blueprint.Kustomizations) != 1 || blueprint.Kustomizations[0].Name != "test-kustomization" {
			t.Errorf("Expected one kustomization named 'test-kustomization', got %v", blueprint.Kustomizations)
		}

		// And the repository should be correctly set
		if blueprint.Repository.Url != "git::https://example.com/test-repo.git" {
			t.Errorf("Expected repository URL 'git::https://example.com/test-repo.git', got %s", blueprint.Repository.Url)
		}
		if blueprint.Repository.Ref.Branch != "main" {
			t.Errorf("Expected repository branch 'main', got %s", blueprint.Repository.Ref.Branch)
		}
	})

	t.Run("MissingRequiredFields", func(t *testing.T) {
		// Given a blueprint handler and an empty blueprint
		handler, _ := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// And blueprint data with missing required fields
		data := []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: ""
  description: ""
`)

		// When processing the blueprint data
		baseHandler := handler.(*BaseBlueprintHandler)
		err := baseHandler.processBlueprintData(data, blueprint)

		// Then no error should be returned since validation is removed
		if err != nil {
			t.Errorf("Expected no error for missing required fields, got: %v", err)
		}
	})

	t.Run("InvalidYAML", func(t *testing.T) {
		// Given a blueprint handler and an empty blueprint
		handler, _ := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// And invalid YAML data
		data := []byte(`invalid yaml content`)

		// When processing the blueprint data
		baseHandler := handler.(*BaseBlueprintHandler)
		err := baseHandler.processBlueprintData(data, blueprint)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for invalid YAML, got nil")
		}
		if !strings.Contains(err.Error(), "error unmarshalling blueprint data") {
			t.Errorf("Expected error about unmarshalling, got: %v", err)
		}
	})

	t.Run("InvalidKustomization", func(t *testing.T) {
		// Given a blueprint handler and an empty blueprint
		handler, _ := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// And blueprint data with an invalid kustomization interval
		data := []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
  description: A test blueprint
  authors:
    - John Doe
kustomize:
  - name: test-kustomization
    interval: invalid-interval
    path: ./kustomize
`)

		// When processing the blueprint data
		baseHandler := handler.(*BaseBlueprintHandler)
		err := baseHandler.processBlueprintData(data, blueprint)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for invalid kustomization, got nil")
		}
		if !strings.Contains(err.Error(), "error unmarshalling kustomization YAML") {
			t.Errorf("Expected error about unmarshalling kustomization YAML, got: %v", err)
		}
	})

	t.Run("ErrorMarshallingKustomizationMap", func(t *testing.T) {
		// Given a blueprint handler and an empty blueprint
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// And a mock YAML marshaller that returns an error
		baseHandler.shims.YamlMarshalNonNull = func(v any) ([]byte, error) {
			if _, ok := v.(map[string]any); ok {
				return nil, fmt.Errorf("mock kustomization map marshal error")
			}
			return []byte{}, nil
		}

		// And valid blueprint data
		data := []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
  description: Test description
  authors:
    - Test Author
kustomize:
  - name: test-kustomization
    path: ./test
`)

		// When processing the blueprint data
		err := baseHandler.processBlueprintData(data, blueprint)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for kustomization map marshalling, got nil")
		}
		if !strings.Contains(err.Error(), "error marshalling kustomization map") {
			t.Errorf("Expected error about marshalling kustomization map, got: %v", err)
		}
	})

	t.Run("InvalidKustomizationIntervalZero", func(t *testing.T) {
		// Given a blueprint handler and an empty blueprint
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// And blueprint data with a zero kustomization interval
		data := []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
  description: Test description
  authors:
    - Test Author
kustomize:
  - apiVersion: kustomize.toolkit.fluxcd.io/v1
    kind: Kustomization
    metadata:
      name: test-kustomization
    spec:
      interval: 0s
      path: ./test
`)

		// When processing the blueprint data
		err := baseHandler.processBlueprintData(data, blueprint)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error for kustomization with zero interval, got: %v", err)
		}
	})

	t.Run("InvalidKustomizationIntervalValue", func(t *testing.T) {
		// Given a blueprint handler and an empty blueprint
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// And blueprint data with an invalid kustomization interval
		data := []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
  description: Test description
  authors:
    - Test Author
kustomize:
  - apiVersion: kustomize.toolkit.fluxcd.io/v1
    kind: Kustomization
    metadata:
      name: test-kustomization
    spec:
      interval: "invalid"
      path: ./test
`)
		// When processing the blueprint data
		err := baseHandler.processBlueprintData(data, blueprint)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error for invalid kustomization interval value, got: %v", err)
		}
	})

	t.Run("MissingDescription", func(t *testing.T) {
		// Given a blueprint handler and data with missing description
		handler, _ := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{}

		data := []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
  authors:
    - John Doe
`)

		// When processing the blueprint data
		baseHandler := handler.(*BaseBlueprintHandler)
		err := baseHandler.processBlueprintData(data, blueprint)

		// Then no error should be returned since validation is removed
		if err != nil {
			t.Errorf("Expected no error for missing description, got: %v", err)
		}
	})

	t.Run("MissingAuthors", func(t *testing.T) {
		// Given a blueprint handler and data with empty authors list
		handler, _ := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{}

		data := []byte(`
kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
  description: A test blueprint
  authors: []
`)

		// When processing the blueprint data
		baseHandler := handler.(*BaseBlueprintHandler)
		err := baseHandler.processBlueprintData(data, blueprint)

		// Then no error should be returned since validation is removed
		if err != nil {
			t.Errorf("Expected no error for empty authors list, got: %v", err)
		}
	})
}

// =============================================================================
// Test Helper Functions
// =============================================================================

func TestYamlMarshalWithDefinedPaths(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}
		return handler, mocks
	}

	t.Run("IgnoreYamlMinusTag", func(t *testing.T) {
		// Given a struct with a YAML minus tag
		type testStruct struct {
			Public  string `yaml:"public"`
			private string `yaml:"-"`
		}
		input := testStruct{Public: "value", private: "ignored"}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the public field should be included
		if !strings.Contains(string(result), "public: value") {
			t.Errorf("Expected 'public: value' in result, got: %s", string(result))
		}

		// And the ignored field should be excluded
		if strings.Contains(string(result), "ignored") {
			t.Errorf("Expected 'ignored' not to be in result, got: %s", string(result))
		}
	})

	t.Run("NilInput", func(t *testing.T) {
		// When marshalling nil input
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(nil)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for nil input, got nil")
		}

		// And the error message should be appropriate
		if !strings.Contains(err.Error(), "invalid input: nil value") {
			t.Errorf("Expected error about nil input, got: %v", err)
		}
	})

	t.Run("EmptySlice", func(t *testing.T) {
		// Given an empty slice
		input := []string{}

		// When marshalling the slice
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the result should be an empty array
		if string(result) != "[]\n" {
			t.Errorf("Expected '[]\n', got: %s", string(result))
		}
	})

	t.Run("NoYamlTag", func(t *testing.T) {
		// Given a struct with no YAML tags
		type testStruct struct {
			Field string
		}
		input := testStruct{Field: "value"}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the field name should be used as is
		if !strings.Contains(string(result), "Field: value") {
			t.Errorf("Expected 'Field: value' in result, got: %s", string(result))
		}
	})

	t.Run("CustomYamlTag", func(t *testing.T) {
		// Given a struct with a custom YAML tag
		type testStruct struct {
			Field string `yaml:"custom_field"`
		}
		input := testStruct{Field: "value"}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the custom field name should be used
		if !strings.Contains(string(result), "custom_field: value") {
			t.Errorf("Expected 'custom_field: value' in result, got: %s", string(result))
		}
	})

	t.Run("MapWithCustomTags", func(t *testing.T) {
		// Given a map with nested structs using custom YAML tags
		type nestedStruct struct {
			Value string `yaml:"custom_value"`
		}
		input := map[string]nestedStruct{
			"key": {Value: "test"},
		}

		// When marshalling the map
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the map key should be preserved
		if !strings.Contains(string(result), "key:") {
			t.Errorf("Expected 'key:' in result, got: %s", string(result))
		}

		// And the nested custom field name should be used
		if !strings.Contains(string(result), "  custom_value: test") {
			t.Errorf("Expected '  custom_value: test' in result, got: %s", string(result))
		}
	})

	t.Run("DefaultFieldName", func(t *testing.T) {
		// Given a struct with default field names
		data := struct {
			Field string
		}{
			Field: "value",
		}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(data)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the default field name should be used
		if !strings.Contains(string(result), "Field: value") {
			t.Errorf("Expected 'Field: value' in result, got: %s", string(result))
		}
	})

	t.Run("NilInput", func(t *testing.T) {
		// When marshalling nil input
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(nil)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for nil input, got nil")
		}

		// And the error message should be appropriate
		if !strings.Contains(err.Error(), "invalid input: nil value") {
			t.Errorf("Expected error about nil input, got: %v", err)
		}
	})

	t.Run("FuncType", func(t *testing.T) {
		// When marshalling a function type
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(func() {})

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for func type, got nil")
		}

		// And the error message should be appropriate
		if !strings.Contains(err.Error(), "unsupported value type func") {
			t.Errorf("Expected error about unsupported value type, got: %v", err)
		}
	})

	t.Run("UnsupportedType", func(t *testing.T) {
		// When marshalling an unsupported type
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(make(chan int))

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for unsupported type, got nil")
		}

		// And the error message should be appropriate
		if !strings.Contains(err.Error(), "unsupported value type") {
			t.Errorf("Expected error about unsupported value type, got: %v", err)
		}
	})

	t.Run("MapWithNilValues", func(t *testing.T) {
		// Given a map with nil values
		input := map[string]any{
			"key1": nil,
			"key2": "value2",
		}

		// When marshalling the map
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And nil values should be represented as null
		if !strings.Contains(string(result), "key1: null") {
			t.Errorf("Expected 'key1: null' in result, got: %s", string(result))
		}

		// And non-nil values should be preserved
		if !strings.Contains(string(result), "key2: value2") {
			t.Errorf("Expected 'key2: value2' in result, got: %s", string(result))
		}
	})

	t.Run("SliceWithNilValues", func(t *testing.T) {
		// Given a slice with nil values
		input := []any{nil, "value", nil}

		// When marshalling the slice
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And nil values should be represented as null
		if !strings.Contains(string(result), "- null") {
			t.Errorf("Expected '- null' in result, got: %s", string(result))
		}

		// And non-nil values should be preserved
		if !strings.Contains(string(result), "- value") {
			t.Errorf("Expected '- value' in result, got: %s", string(result))
		}
	})

	t.Run("StructWithPrivateFields", func(t *testing.T) {
		// Given a struct with both public and private fields
		type testStruct struct {
			Public  string
			private string
		}
		input := testStruct{Public: "value", private: "ignored"}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And public fields should be included
		if !strings.Contains(string(result), "Public: value") {
			t.Errorf("Expected 'Public: value' in result, got: %s", string(result))
		}

		// And private fields should be excluded
		if strings.Contains(string(result), "private") {
			t.Errorf("Expected 'private' not to be in result, got: %s", string(result))
		}
	})

	t.Run("StructWithYamlTag", func(t *testing.T) {
		// Given a struct with a YAML tag
		type testStruct struct {
			Field string `yaml:"custom_name"`
		}
		input := testStruct{Field: "value"}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the custom field name should be used
		if !strings.Contains(string(result), "custom_name: value") {
			t.Errorf("Expected 'custom_name: value' in result, got: %s", string(result))
		}
	})

	t.Run("NestedStructs", func(t *testing.T) {
		// Given nested structs
		type nested struct {
			Value string
		}
		type parent struct {
			Nested nested
		}
		input := parent{Nested: nested{Value: "test"}}

		// When marshalling the nested structs
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the parent field should be included
		if !strings.Contains(string(result), "Nested:") {
			t.Errorf("Expected 'Nested:' in result, got: %s", string(result))
		}

		// And the nested field should be properly indented
		if !strings.Contains(string(result), "  Value: test") {
			t.Errorf("Expected '  Value: test' in result, got: %s", string(result))
		}
	})

	t.Run("NumericTypes", func(t *testing.T) {
		// Given a struct with various numeric types
		type numbers struct {
			Int     int     `yaml:"int"`
			Int8    int8    `yaml:"int8"`
			Int16   int16   `yaml:"int16"`
			Int32   int32   `yaml:"int32"`
			Int64   int64   `yaml:"int64"`
			Uint    uint    `yaml:"uint"`
			Uint8   uint8   `yaml:"uint8"`
			Uint16  uint16  `yaml:"uint16"`
			Uint32  uint32  `yaml:"uint32"`
			Uint64  uint64  `yaml:"uint64"`
			Float32 float32 `yaml:"float32"`
			Float64 float64 `yaml:"float64"`
		}
		input := numbers{
			Int: 1, Int8: 2, Int16: 3, Int32: 4, Int64: 5,
			Uint: 6, Uint8: 7, Uint16: 8, Uint32: 9, Uint64: 10,
			Float32: 11.1, Float64: 12.2,
		}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And all numeric values should be correctly represented
		for _, expected := range []string{
			"int: 1", "int8: 2", "int16: 3", "int32: 4", "int64: 5",
			"uint: 6", "uint8: 7", "uint16: 8", "uint32: 9", "uint64: 10",
			"float32: 11.1", "float64: 12.2",
		} {
			if !strings.Contains(string(result), expected) {
				t.Errorf("Expected '%s' in result, got: %s", expected, string(result))
			}
		}
	})

	t.Run("BooleanType", func(t *testing.T) {
		// Given a struct with boolean fields
		type boolStruct struct {
			True  bool `yaml:"true"`
			False bool `yaml:"false"`
		}
		input := boolStruct{True: true, False: false}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the boolean values should be correctly represented
		if !strings.Contains(string(result), `"true": true`) {
			t.Errorf("Expected '\"true\": true' in result, got: %s", string(result))
		}
		if !strings.Contains(string(result), `"false": false`) {
			t.Errorf("Expected '\"false\": false' in result, got: %s", string(result))
		}
	})

	t.Run("NilPointerAndInterface", func(t *testing.T) {
		// Given a struct with nil pointers and interfaces
		type testStruct struct {
			NilPtr       *string              `yaml:"nil_ptr"`
			NilInterface any                  `yaml:"nil_interface"`
			NilMap       map[string]string    `yaml:"nil_map"`
			NilSlice     []string             `yaml:"nil_slice"`
			NilStruct    *struct{ Field int } `yaml:"nil_struct"`
		}
		input := testStruct{}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And nil interfaces should be represented as empty objects
		if !strings.Contains(string(result), "nil_interface: {}") {
			t.Errorf("Expected 'nil_interface: {}' in result, got: %s", string(result))
		}

		// And nil slices should be represented as empty arrays
		if !strings.Contains(string(result), "nil_slice: []") {
			t.Errorf("Expected 'nil_slice: []' in result, got: %s", string(result))
		}

		// And nil maps should be represented as empty objects
		if !strings.Contains(string(result), "nil_map: {}") {
			t.Errorf("Expected 'nil_map: {}' in result, got: %s", string(result))
		}

		// And nil structs should be represented as empty objects
		if !strings.Contains(string(result), "nil_struct: {}") {
			t.Errorf("Expected 'nil_struct: {}' in result, got: %s", string(result))
		}
	})

	t.Run("SliceWithNilElements", func(t *testing.T) {
		// Given a slice with nil elements
		type elem struct {
			Field string
		}
		input := []*elem{nil, {Field: "value"}, nil}

		// When marshalling the slice
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And non-nil elements should be correctly represented
		if !strings.Contains(string(result), "Field: value") {
			t.Errorf("Expected 'Field: value' in result, got: %s", string(result))
		}
	})

	t.Run("MapWithNilValues", func(t *testing.T) {
		// Given a map with nil and non-nil values
		input := map[string]any{
			"nil":    nil,
			"nonnil": "value",
			"nilptr": (*string)(nil),
		}

		// When marshalling the map to YAML
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And nil values should be represented as null
		if !strings.Contains(string(result), "nil: null") {
			t.Errorf("Expected 'nil: null' in result, got: %s", string(result))
		}

		// And non-nil values should be preserved
		if !strings.Contains(string(result), "nonnil: value") {
			t.Errorf("Expected 'nonnil: value' in result, got: %s", string(result))
		}
	})

	t.Run("UnsupportedType", func(t *testing.T) {
		// Given an unsupported channel type
		input := make(chan int)

		// When attempting to marshal the channel
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for unsupported type, got nil")
		}

		// And the error should indicate the unsupported type
		if !strings.Contains(err.Error(), "unsupported value type chan") {
			t.Errorf("Expected error about unsupported type, got: %v", err)
		}
	})

	t.Run("FunctionType", func(t *testing.T) {
		// Given a function type
		input := func() {}

		// When attempting to marshal the function
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for function type, got nil")
		}

		// And the error should indicate the unsupported type
		if !strings.Contains(err.Error(), "unsupported value type func") {
			t.Errorf("Expected error about unsupported type, got: %v", err)
		}
	})

	t.Run("ErrorInSliceConversion", func(t *testing.T) {
		// Given a slice containing an unsupported type
		input := []any{make(chan int)}

		// When attempting to marshal the slice
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for slice with unsupported type, got nil")
		}

		// And the error should indicate the slice conversion issue
		if !strings.Contains(err.Error(), "error converting slice element") {
			t.Errorf("Expected error about slice conversion, got: %v", err)
		}
	})

	t.Run("ErrorInMapConversion", func(t *testing.T) {
		// Given a map containing an unsupported type
		input := map[string]any{
			"channel": make(chan int),
		}

		// When attempting to marshal the map
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for map with unsupported type, got nil")
		}

		// And the error should indicate the map conversion issue
		if !strings.Contains(err.Error(), "error converting map value") {
			t.Errorf("Expected error about map conversion, got: %v", err)
		}
	})

	t.Run("ErrorInStructFieldConversion", func(t *testing.T) {
		// Given a struct containing an unsupported field type
		type testStruct struct {
			Channel chan int
		}
		input := testStruct{Channel: make(chan int)}

		// When attempting to marshal the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for struct with unsupported field type, got nil")
		}

		// And the error should indicate the field conversion issue
		if !strings.Contains(err.Error(), "error converting field") {
			t.Errorf("Expected error about field conversion, got: %v", err)
		}
	})

	t.Run("YamlMarshalError", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)

		// And a mock YAML marshaller that returns an error
		baseHandler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("mock yaml marshal error")
		}

		// And a simple struct to marshal
		input := struct{ Field string }{"value"}

		// When marshalling the struct
		_, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error from yaml marshal, got nil")
		}

		// And the error should indicate the YAML marshalling issue
		if !strings.Contains(err.Error(), "error marshalling yaml") {
			t.Errorf("Expected error about yaml marshalling, got: %v", err)
		}
	})
}

func TestTLACode(t *testing.T) {
	// Given a mock Jsonnet VM that returns an error about missing authors
	vm := NewMockJsonnetVM(func(filename, snippet string) (string, error) {
		return "", fmt.Errorf("blueprint has no authors")
	})

	// When evaluating an empty snippet
	_, err := vm.EvaluateAnonymousSnippet("test.jsonnet", "")

	// Then an error about missing authors should be returned
	if err == nil || !strings.Contains(err.Error(), "blueprint has no authors") {
		t.Errorf("expected error containing 'blueprint has no authors', got %v", err)
	}
}

func TestBaseBlueprintHandler_calculateMaxWaitTime(t *testing.T) {
	t.Run("EmptyKustomizations", func(t *testing.T) {
		// Given a blueprint handler with no kustomizations
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return 0 since there are no kustomizations
		if waitTime != 0 {
			t.Errorf("expected 0 duration, got %v", waitTime)
		}
	})

	t.Run("SingleKustomization", func(t *testing.T) {
		// Given a blueprint handler with a single kustomization
		customTimeout := 2 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "test-kustomization",
						Timeout: &metav1.Duration{
							Duration: customTimeout,
						},
					},
				},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the kustomization's timeout
		if waitTime != customTimeout {
			t.Errorf("expected timeout %v, got %v", customTimeout, waitTime)
		}
	})

	t.Run("LinearDependencies", func(t *testing.T) {
		// Given a blueprint handler with linear dependencies
		timeout1 := 1 * time.Minute
		timeout2 := 2 * time.Minute
		timeout3 := 3 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Timeout: &metav1.Duration{
							Duration: timeout1,
						},
						DependsOn: []string{"kustomization-2"},
					},
					{
						Name: "kustomization-2",
						Timeout: &metav1.Duration{
							Duration: timeout2,
						},
						DependsOn: []string{"kustomization-3"},
					},
					{
						Name: "kustomization-3",
						Timeout: &metav1.Duration{
							Duration: timeout3,
						},
					},
				},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the sum of all timeouts
		expectedTime := timeout1 + timeout2 + timeout3
		if waitTime != expectedTime {
			t.Errorf("expected timeout %v, got %v", expectedTime, waitTime)
		}
	})

	t.Run("BranchingDependencies", func(t *testing.T) {
		// Given a blueprint handler with branching dependencies
		timeout1 := 1 * time.Minute
		timeout2 := 2 * time.Minute
		timeout3 := 3 * time.Minute
		timeout4 := 4 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Timeout: &metav1.Duration{
							Duration: timeout1,
						},
						DependsOn: []string{"kustomization-2", "kustomization-3"},
					},
					{
						Name: "kustomization-2",
						Timeout: &metav1.Duration{
							Duration: timeout2,
						},
						DependsOn: []string{"kustomization-4"},
					},
					{
						Name: "kustomization-3",
						Timeout: &metav1.Duration{
							Duration: timeout3,
						},
						DependsOn: []string{"kustomization-4"},
					},
					{
						Name: "kustomization-4",
						Timeout: &metav1.Duration{
							Duration: timeout4,
						},
					},
				},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the longest path (1 -> 3 -> 4)
		expectedTime := timeout1 + timeout3 + timeout4
		if waitTime != expectedTime {
			t.Errorf("expected timeout %v, got %v", expectedTime, waitTime)
		}
	})

	t.Run("CircularDependencies", func(t *testing.T) {
		// Given a blueprint handler with circular dependencies
		timeout1 := 1 * time.Minute
		timeout2 := 2 * time.Minute
		timeout3 := 3 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Timeout: &metav1.Duration{
							Duration: timeout1,
						},
						DependsOn: []string{"kustomization-2"},
					},
					{
						Name: "kustomization-2",
						Timeout: &metav1.Duration{
							Duration: timeout2,
						},
						DependsOn: []string{"kustomization-3"},
					},
					{
						Name: "kustomization-3",
						Timeout: &metav1.Duration{
							Duration: timeout3,
						},
						DependsOn: []string{"kustomization-1"},
					},
				},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the sum of all timeouts in the cycle (1+2+3+3)
		expectedTime := timeout1 + timeout2 + timeout3 + timeout3
		if waitTime != expectedTime {
			t.Errorf("expected timeout %v, got %v", expectedTime, waitTime)
		}
	})
}

func TestBaseBlueprintHandler_WaitForKustomizations(t *testing.T) {
	t.Run("AllKustomizationsReady", func(t *testing.T) {
		// Given a blueprint handler with multiple kustomizations that are all ready
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1", Timeout: &metav1.Duration{Duration: 100 * time.Millisecond}},
					{Name: "k2", Timeout: &metav1.Duration{Duration: 100 * time.Millisecond}},
				},
			},
		}
		handler.kustomizationWaitPollInterval = 10 * time.Millisecond

		// And Git repository and kustomization status checks that return success
		origCheckGit := checkGitRepositoryStatus
		origCheckKustom := checkKustomizationStatus
		defer func() {
			checkGitRepositoryStatus = origCheckGit
			checkKustomizationStatus = origCheckKustom
		}()
		checkGitRepositoryStatus = func(string) error { return nil }
		checkKustomizationStatus = func(string, []string) (map[string]bool, error) {
			return map[string]bool{"k1": true, "k2": true}, nil
		}

		// When waiting for kustomizations to be ready
		err := handler.WaitForKustomizations()

		// Then no error should be returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("TimeoutWaitingForKustomizations", func(t *testing.T) {
		// Given a blueprint handler with kustomizations that never reach ready state
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1", Timeout: &metav1.Duration{Duration: 200 * time.Millisecond}},
					{Name: "k2", Timeout: &metav1.Duration{Duration: 200 * time.Millisecond}},
				},
			},
		}
		handler.kustomizationWaitPollInterval = 10 * time.Millisecond

		// And status checks that always return not ready
		origCheckGit := checkGitRepositoryStatus
		origCheckKustom := checkKustomizationStatus
		defer func() {
			checkGitRepositoryStatus = origCheckGit
			checkKustomizationStatus = origCheckKustom
		}()
		checkGitRepositoryStatus = func(string) error { return nil }
		checkKustomizationStatus = func(string, []string) (map[string]bool, error) {
			return map[string]bool{"k1": false, "k2": false}, nil
		}

		// When waiting for kustomizations to be ready
		err := handler.WaitForKustomizations()

		// Then a timeout error should be returned
		if err == nil || !strings.Contains(err.Error(), "timeout waiting for kustomizations") {
			t.Errorf("expected timeout error, got %v", err)
		}
	})

	t.Run("GitRepositoryStatusError", func(t *testing.T) {
		// Given a blueprint handler with a kustomization
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1", Timeout: &metav1.Duration{Duration: 100 * time.Millisecond}},
				},
			},
		}
		handler.kustomizationWaitPollInterval = 10 * time.Millisecond

		// And a Git repository status check that returns an error
		origCheckGit := checkGitRepositoryStatus
		origCheckKustom := checkKustomizationStatus
		defer func() {
			checkGitRepositoryStatus = origCheckGit
			checkKustomizationStatus = origCheckKustom
		}()
		checkGitRepositoryStatus = func(string) error { return fmt.Errorf("git repo error") }
		checkKustomizationStatus = func(string, []string) (map[string]bool, error) {
			return map[string]bool{"k1": true}, nil
		}

		// When waiting for kustomizations to be ready
		err := handler.WaitForKustomizations()

		// Then a Git repository error should be returned
		if err == nil || !strings.Contains(err.Error(), "git repository error") {
			t.Errorf("expected git repository error, got %v", err)
		}
	})

	t.Run("KustomizationStatusError", func(t *testing.T) {
		// Given a blueprint handler with a kustomization
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1", Timeout: &metav1.Duration{Duration: 100 * time.Millisecond}},
				},
			},
		}
		handler.kustomizationWaitPollInterval = 10 * time.Millisecond

		// And a kustomization status check that returns an error
		origCheckGit := checkGitRepositoryStatus
		origCheckKustom := checkKustomizationStatus
		defer func() {
			checkGitRepositoryStatus = origCheckGit
			checkKustomizationStatus = origCheckKustom
		}()
		checkGitRepositoryStatus = func(string) error { return nil }
		checkKustomizationStatus = func(string, []string) (map[string]bool, error) {
			return nil, fmt.Errorf("kustomization error")
		}

		// When waiting for kustomizations to be ready
		err := handler.WaitForKustomizations()

		// Then a kustomization error should be returned
		if err == nil || !strings.Contains(err.Error(), "kustomization error") {
			t.Errorf("expected kustomization error, got %v", err)
		}
	})

	t.Run("RecoverFromGitRepositoryError", func(t *testing.T) {
		// Given a blueprint handler with a kustomization
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1", Timeout: &metav1.Duration{Duration: 100 * time.Millisecond}},
				},
			},
		}
		handler.kustomizationWaitPollInterval = 10 * time.Millisecond

		// And a Git repository status check that fails twice then succeeds
		failCount := 0
		origCheckGit := checkGitRepositoryStatus
		origCheckKustom := checkKustomizationStatus
		defer func() {
			checkGitRepositoryStatus = origCheckGit
			checkKustomizationStatus = origCheckKustom
		}()
		checkGitRepositoryStatus = func(string) error {
			if failCount < 2 {
				failCount++
				return fmt.Errorf("git repo error")
			}
			return nil
		}
		checkKustomizationStatus = func(string, []string) (map[string]bool, error) {
			return map[string]bool{"k1": true}, nil
		}

		// When waiting for kustomizations to be ready
		err := handler.WaitForKustomizations()

		// Then no error should be returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("RecoverFromKustomizationError", func(t *testing.T) {
		// Given a blueprint handler with a kustomization
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1", Timeout: &metav1.Duration{Duration: 100 * time.Millisecond}},
				},
			},
		}
		handler.kustomizationWaitPollInterval = 10 * time.Millisecond

		// And a kustomization status check that fails twice then succeeds
		failCount := 0
		origCheckGit := checkGitRepositoryStatus
		origCheckKustom := checkKustomizationStatus
		defer func() {
			checkGitRepositoryStatus = origCheckGit
			checkKustomizationStatus = origCheckKustom
		}()
		checkGitRepositoryStatus = func(string) error { return nil }
		checkKustomizationStatus = func(string, []string) (map[string]bool, error) {
			if failCount < 2 {
				failCount++
				return nil, fmt.Errorf("kustomization error")
			}
			return map[string]bool{"k1": true}, nil
		}

		// When waiting for kustomizations to be ready
		err := handler.WaitForKustomizations()

		// Then no error should be returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("MaxGitRepositoryFailures", func(t *testing.T) {
		// Given a blueprint handler with a kustomization
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1", Timeout: &metav1.Duration{Duration: 100 * time.Millisecond}},
				},
			},
		}
		handler.kustomizationWaitPollInterval = 10 * time.Millisecond

		// And a Git repository status check that always fails
		origCheckGit := checkGitRepositoryStatus
		origCheckKustom := checkKustomizationStatus
		defer func() {
			checkGitRepositoryStatus = origCheckGit
			checkKustomizationStatus = origCheckKustom
		}()
		checkGitRepositoryStatus = func(string) error { return fmt.Errorf("git repo error") }
		checkKustomizationStatus = func(string, []string) (map[string]bool, error) {
			return map[string]bool{"k1": true}, nil
		}

		// When waiting for kustomizations to be ready
		err := handler.WaitForKustomizations()

		// Then a Git repository error should be returned with failure count
		expectedMsg := fmt.Sprintf("after %d consecutive failures", constants.DEFAULT_KUSTOMIZATION_WAIT_MAX_FAILURES)
		if err == nil || !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("expected error with failure count, got %v", err)
		}
	})

	t.Run("MaxKustomizationFailures", func(t *testing.T) {
		// Given a blueprint handler with a kustomization
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1", Timeout: &metav1.Duration{Duration: 100 * time.Millisecond}},
				},
			},
		}
		handler.kustomizationWaitPollInterval = 10 * time.Millisecond

		// And a kustomization status check that always fails
		origCheckGit := checkGitRepositoryStatus
		origCheckKustom := checkKustomizationStatus
		defer func() {
			checkGitRepositoryStatus = origCheckGit
			checkKustomizationStatus = origCheckKustom
		}()
		checkGitRepositoryStatus = func(string) error { return nil }
		checkKustomizationStatus = func(string, []string) (map[string]bool, error) {
			return nil, fmt.Errorf("kustomization error")
		}

		// When waiting for kustomizations to be ready
		err := handler.WaitForKustomizations()

		// Then a kustomization error should be returned with failure count
		expectedMsg := fmt.Sprintf("after %d consecutive failures", constants.DEFAULT_KUSTOMIZATION_WAIT_MAX_FAILURES)
		if err == nil || !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("expected error with failure count, got %v", err)
		}
	})
}
