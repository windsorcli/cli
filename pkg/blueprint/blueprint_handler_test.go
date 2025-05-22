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

	// Patch kubeClient and kubeClientResourceOperation to no-op by default
	origKubeClient := kubeClient
	kubeClient = func(string, KubeRequestConfig) error { return nil }
	origKubeClientResourceOperation := kubeClientResourceOperation
	kubeClientResourceOperation = func(string, ResourceOperationConfig) error { return nil }
	t.Cleanup(func() {
		kubeClient = origKubeClient
		kubeClientResourceOperation = origKubeClientResourceOperation
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
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".jsonnet") || strings.HasSuffix(name, ".yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			checkedPaths = append(checkedPaths, name)
			if strings.HasSuffix(name, ".jsonnet") {
				return []byte(safeBlueprintJsonnet), nil
			}
			if strings.HasSuffix(name, ".yaml") {
				return []byte(safeBlueprintYAML), nil
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

		// And only yaml path should be checked since it exists
		expectedPaths := []string{
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
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return nil, fmt.Errorf("error reading jsonnet file")
			}
			return nil, os.ErrNotExist
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
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".yaml") {
				return nil, fmt.Errorf("error reading yaml file")
			}
			return nil, os.ErrNotExist
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
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return []byte(safeBlueprintJsonnet), nil
			}
			return nil, os.ErrNotExist
		}
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
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return []byte(safeBlueprintJsonnet), nil
			}
			return nil, os.ErrNotExist
		}
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
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return []byte(safeBlueprintJsonnet), nil
			}
			return nil, os.ErrNotExist
		}
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
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return []byte(safeBlueprintJsonnet), nil
			}
			return nil, os.ErrNotExist
		}
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
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".jsonnet") {
				return nil, fmt.Errorf("error reading jsonnet file")
			}
			return nil, os.ErrNotExist
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
		// Patch Stat to simulate file does not exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
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
		// Patch Stat to simulate file does not exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
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
		// Patch Stat to simulate file does not exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

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
		// Patch Stat to simulate file does not exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

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
		// Patch Stat to simulate file does not exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
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
		// Patch Stat to simulate file does not exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
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
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		origKubeClientResourceOperation := kubeClientResourceOperation
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()
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
		origKubeClientResourceOperation := kubeClientResourceOperation
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()
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
		origKubeClientResourceOperation := kubeClientResourceOperation
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()
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

	t.Run("EmptySourceUrlError", func(t *testing.T) {
		origKubeClientResourceOperation := kubeClientResourceOperation
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()
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
		origKubeClientResourceOperation := kubeClientResourceOperation
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			return nil
		}

		// Given a blueprint handler with an empty repository URL
		handler, _ := setup(t)

		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		// When installing the blueprint
		err = handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error for empty repository URL, got: %v", err)
		}
	})

	t.Run("NoRepository", func(t *testing.T) {
		origKubeClientResourceOperation := kubeClientResourceOperation
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()
		gitRepoAttempted := false
		kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
			if config.ResourceName == "gitrepositories" {
				gitRepoAttempted = true
			}
			return nil
		}

		// And a blueprint handler
		handler, _ := setup(t)

		err := handler.Install()
		if err != nil {
			t.Errorf("Expected no error when no repository is defined, got: %v", err)
		}
		if gitRepoAttempted {
			t.Error("Expected no GitRepository to be applied when no repository is defined")
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
		err := handler.WaitForKustomizations("")

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
		err := handler.WaitForKustomizations("")

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
		err := handler.WaitForKustomizations("")

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
		err := handler.WaitForKustomizations("")

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
		err := handler.WaitForKustomizations("")

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
		err := handler.WaitForKustomizations("")

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
		err := handler.WaitForKustomizations("")

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
					{Name: "k1", Timeout: &metav1.Duration{Duration: 1 * time.Second}},
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
		err := handler.WaitForKustomizations("")

		// Then a kustomization error should be returned with failure count
		expectedMsg := fmt.Sprintf("after %d consecutive failures", constants.DEFAULT_KUSTOMIZATION_WAIT_MAX_FAILURES)
		if err == nil || !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("expected error with failure count, got %v", err)
		}
	})
}

func TestBaseBlueprintHandler_Down(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		// Set fast poll interval and short timeout for all kustomizations
		handler.kustomizationWaitPollInterval = 1 * time.Millisecond
		for i := range handler.blueprint.Kustomizations {
			if handler.blueprint.Kustomizations[i].Timeout == nil {
				handler.blueprint.Kustomizations[i].Timeout = &metav1.Duration{Duration: 5 * time.Millisecond}
			} else {
				handler.blueprint.Kustomizations[i].Timeout.Duration = 5 * time.Millisecond
			}
		}
		return handler, mocks
	}

	t.Run("NoKustomizationsWithCleanup", func(t *testing.T) {
		// Given a handler with kustomizations that have no cleanup paths
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: nil},
			{Name: "k2", Cleanup: []string{}},
		}

		// Patch kubeClientResourceOperation to panic if called (simulate applyKustomization)
		origKubeClientResourceOperation := kubeClientResourceOperation
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()
		kubeClientResourceOperation = func(string, ResourceOperationConfig) error {
			panic("kubeClientResourceOperation should not be called")
		}

		// When calling Down
		err := baseHandler.Down()

		// Then no error should be returned
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("SingleKustomizationWithCleanup", func(t *testing.T) {
		// Given a handler with a single kustomization with a cleanup path
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup/path"}},
		}
		// Patch kubeClientResourceOperation to record calls
		var calledConfigs []ResourceOperationConfig
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			calledConfigs = append(calledConfigs, config)
			return nil
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()

		// Patch checkKustomizationStatus to always return ready
		origCheckKustomizationStatus := checkKustomizationStatus
		checkKustomizationStatus = func(_ string, names []string) (map[string]bool, error) {
			m := make(map[string]bool)
			for _, n := range names {
				m[n] = true
			}
			return m, nil
		}
		defer func() { checkKustomizationStatus = origCheckKustomizationStatus }()

		// Patch checkGitRepositoryStatus to no-op
		origCheckGitRepositoryStatus := checkGitRepositoryStatus
		checkGitRepositoryStatus = func(_ string) error { return nil }
		defer func() { checkGitRepositoryStatus = origCheckGitRepositoryStatus }()

		// When calling Down
		err := baseHandler.Down()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// And kubeClientResourceOperation should be called once
		if len(calledConfigs) != 1 {
			t.Fatalf("expected 1 call to kubeClientResourceOperation, got %d", len(calledConfigs))
		}

		// And the resource name should be k1-cleanup
		if calledConfigs[0].ResourceInstanceName != "k1-cleanup" {
			t.Errorf("expected ResourceInstanceName 'k1-cleanup', got '%s'", calledConfigs[0].ResourceInstanceName)
		}
	})

	t.Run("MultipleKustomizationsWithCleanup", func(t *testing.T) {
		// Given a handler with multiple kustomizations, some with cleanup paths
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup/path1"}},
			{Name: "k2", Cleanup: []string{"cleanup/path2"}},
			{Name: "k3", Cleanup: []string{"cleanup/path3"}},
			{Name: "k4", Cleanup: []string{}},
		}

		// Set fast poll interval and short timeout
		baseHandler.kustomizationWaitPollInterval = 1 * time.Millisecond
		for i := range baseHandler.blueprint.Kustomizations {
			if baseHandler.blueprint.Kustomizations[i].Timeout == nil {
				baseHandler.blueprint.Kustomizations[i].Timeout = &metav1.Duration{Duration: 5 * time.Millisecond}
			} else {
				baseHandler.blueprint.Kustomizations[i].Timeout.Duration = 5 * time.Millisecond
			}
		}

		// Patch kubeClientResourceOperation to record calls
		var calledConfigs []ResourceOperationConfig
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			calledConfigs = append(calledConfigs, config)
			return nil
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()

		// Patch checkKustomizationStatus to always return ready
		origCheckKustomizationStatus := checkKustomizationStatus
		checkKustomizationStatus = func(_ string, names []string) (map[string]bool, error) {
			m := make(map[string]bool)
			for _, n := range names {
				m[n] = true
			}
			return m, nil
		}
		defer func() { checkKustomizationStatus = origCheckKustomizationStatus }()

		// Patch checkGitRepositoryStatus to no-op
		origCheckGitRepositoryStatus := checkGitRepositoryStatus
		checkGitRepositoryStatus = func(_ string) error { return nil }
		defer func() { checkGitRepositoryStatus = origCheckGitRepositoryStatus }()

		// When calling Down
		err := baseHandler.Down()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// And kubeClientResourceOperation should be called for each kustomization with cleanup
		if len(calledConfigs) != 3 {
			t.Fatalf("expected 3 calls to kubeClientResourceOperation, got %d", len(calledConfigs))
		}

		// And the resource names should be k1-cleanup, k2-cleanup, k3-cleanup
		expectedNames := map[string]bool{"k1-cleanup": true, "k2-cleanup": true, "k3-cleanup": true}
		for _, config := range calledConfigs {
			if !expectedNames[config.ResourceInstanceName] {
				t.Errorf("unexpected ResourceInstanceName '%s'", config.ResourceInstanceName)
			}
			delete(expectedNames, config.ResourceInstanceName)
		}
		if len(expectedNames) != 0 {
			t.Errorf("expected ResourceInstanceNames not called: %v", expectedNames)
		}
	})

	t.Run("ApplyKustomizationError", func(t *testing.T) {
		// Given a handler with a kustomization with cleanup
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup/path1"}},
		}

		// Patch kubeClientResourceOperation to error on apply
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			if config.ResourceInstanceName == "k1-cleanup" {
				return fmt.Errorf("apply error")
			}
			return nil
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()

		// Patch checkKustomizationStatus to always return ready
		origCheckKustomizationStatus := checkKustomizationStatus
		checkKustomizationStatus = func(_ string, names []string) (map[string]bool, error) {
			m := make(map[string]bool)
			for _, n := range names {
				m[n] = true
			}
			return m, nil
		}
		defer func() { checkKustomizationStatus = origCheckKustomizationStatus }()

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "apply error") {
			t.Errorf("Expected error about apply error, got: %v", err)
		}
	})

	t.Run("WaitForKustomizationsError", func(t *testing.T) {
		// Given a handler with a kustomization with cleanup
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup/path1"}},
		}

		// Set fast poll interval and short timeout
		baseHandler.kustomizationWaitPollInterval = 1 * time.Millisecond
		for i := range baseHandler.blueprint.Kustomizations {
			if baseHandler.blueprint.Kustomizations[i].Timeout == nil {
				baseHandler.blueprint.Kustomizations[i].Timeout = &metav1.Duration{Duration: 5 * time.Millisecond}
			} else {
				baseHandler.blueprint.Kustomizations[i].Timeout.Duration = 5 * time.Millisecond
			}
		}

		// Patch kubeClientResourceOperation to succeed
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			return nil
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()

		// Patch checkKustomizationStatus to error
		origCheckKustomizationStatus := checkKustomizationStatus
		checkKustomizationStatus = func(_ string, names []string) (map[string]bool, error) {
			return nil, fmt.Errorf("wait error")
		}
		defer func() { checkKustomizationStatus = origCheckKustomizationStatus }()

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for kustomizations") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	})

	t.Run("ErrorApplyingCleanupKustomization", func(t *testing.T) {
		// Given a handler with kustomizations that need cleanup
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup/path1"}},
		}

		// And a mock that fails to apply cleanup kustomization
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			if config.ResourceInstanceName == "k1-cleanup" {
				return fmt.Errorf("failed to apply cleanup kustomization")
			}
			return nil
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply cleanup kustomization") {
			t.Errorf("Expected error about cleanup kustomization, got: %v", err)
		}
	})

	t.Run("ErrorDeletingKustomization", func(t *testing.T) {
		// Given a handler with kustomizations
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1"},
		}

		// Patch kubeClient to return error on DELETE
		origKubeClient := kubeClient
		kubeClient = func(kubeconfig string, req KubeRequestConfig) error {
			if req.Method == "DELETE" {
				return fmt.Errorf("delete error")
			}
			return nil
		}
		defer func() { kubeClient = origKubeClient }()

		// Patch kubeClientResourceOperation to no-op
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			return nil
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()

		// Patch checkKustomizationStatus to always return ready
		origCheckKustomizationStatus := checkKustomizationStatus
		checkKustomizationStatus = func(_ string, names []string) (map[string]bool, error) {
			m := make(map[string]bool)
			for _, n := range names {
				m[n] = true
			}
			return m, nil
		}
		defer func() { checkKustomizationStatus = origCheckKustomizationStatus }()

		// Patch checkGitRepositoryStatus to no-op
		origCheckGitRepositoryStatus := checkGitRepositoryStatus
		checkGitRepositoryStatus = func(_ string) error { return nil }
		defer func() { checkGitRepositoryStatus = origCheckGitRepositoryStatus }()

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "delete error") {
			t.Errorf("Expected error about delete error, got: %v", err)
		}
	})

	// ErrorApplyingCleanupKustomization
	t.Run("ErrorApplyingCleanupKustomization", func(t *testing.T) {
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup/path1"}},
		}
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			if config.ResourceInstanceName == "k1-cleanup" {
				return fmt.Errorf("failed to apply cleanup kustomization")
			}
			return nil
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()
		// Patch checkKustomizationStatus to always return ready
		origCheckKustomizationStatus := checkKustomizationStatus
		checkKustomizationStatus = func(_ string, names []string) (map[string]bool, error) {
			m := make(map[string]bool)
			for _, n := range names {
				m[n] = true
			}
			return m, nil
		}
		defer func() { checkKustomizationStatus = origCheckKustomizationStatus }()
		// Patch checkGitRepositoryStatus to no-op
		origCheckGitRepositoryStatus := checkGitRepositoryStatus
		checkGitRepositoryStatus = func(_ string) error { return nil }
		defer func() { checkGitRepositoryStatus = origCheckGitRepositoryStatus }()
		err := baseHandler.Down()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply cleanup kustomization") {
			t.Errorf("Expected error about cleanup kustomization, got: %v", err)
		}
	})

	// Error paths for WaitForKustomizationsError and ApplyKustomizationError
	t.Run("ApplyKustomizationError", func(t *testing.T) {
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup/path1"}},
		}
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			if config.ResourceInstanceName == "k1-cleanup" {
				return fmt.Errorf("apply error")
			}
			return nil
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()
		origCheckKustomizationStatus := checkKustomizationStatus
		checkKustomizationStatus = func(_ string, names []string) (map[string]bool, error) {
			m := make(map[string]bool)
			for _, n := range names {
				m[n] = true
			}
			return m, nil
		}
		defer func() { checkKustomizationStatus = origCheckKustomizationStatus }()
		origCheckGitRepositoryStatus := checkGitRepositoryStatus
		checkGitRepositoryStatus = func(_ string) error { return nil }
		defer func() { checkGitRepositoryStatus = origCheckGitRepositoryStatus }()
		err := baseHandler.Down()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "apply error") {
			t.Errorf("Expected error about apply error, got: %v", err)
		}
	})

	t.Run("WaitForKustomizationsError", func(t *testing.T) {
		// Given a handler with a kustomization with cleanup
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup/path1"}},
		}

		// Set fast poll interval and short timeout
		baseHandler.kustomizationWaitPollInterval = 1 * time.Millisecond
		for i := range baseHandler.blueprint.Kustomizations {
			if baseHandler.blueprint.Kustomizations[i].Timeout == nil {
				baseHandler.blueprint.Kustomizations[i].Timeout = &metav1.Duration{Duration: 5 * time.Millisecond}
			} else {
				baseHandler.blueprint.Kustomizations[i].Timeout.Duration = 5 * time.Millisecond
			}
		}

		// Patch kubeClientResourceOperation to succeed
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			return nil
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()

		// Patch checkKustomizationStatus to error
		origCheckKustomizationStatus := checkKustomizationStatus
		checkKustomizationStatus = func(_ string, names []string) (map[string]bool, error) {
			return nil, fmt.Errorf("wait error")
		}
		defer func() { checkKustomizationStatus = origCheckKustomizationStatus }()

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for kustomizations") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	})

	t.Run("MultipleKustomizationsWithCleanup", func(t *testing.T) {
		// Given a handler with multiple kustomizations, some with cleanup paths
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup/path1"}},
			{Name: "k2", Cleanup: []string{"cleanup/path2"}},
			{Name: "k3", Cleanup: []string{"cleanup/path3"}},
			{Name: "k4", Cleanup: []string{}},
		}

		// Set fast poll interval and short timeout
		baseHandler.kustomizationWaitPollInterval = 1 * time.Millisecond
		for i := range baseHandler.blueprint.Kustomizations {
			if baseHandler.blueprint.Kustomizations[i].Timeout == nil {
				baseHandler.blueprint.Kustomizations[i].Timeout = &metav1.Duration{Duration: 5 * time.Millisecond}
			} else {
				baseHandler.blueprint.Kustomizations[i].Timeout.Duration = 5 * time.Millisecond
			}
		}

		// Patch kubeClientResourceOperation to record calls
		var calledConfigs []ResourceOperationConfig
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			calledConfigs = append(calledConfigs, config)
			return nil
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()

		// Patch checkKustomizationStatus to always return ready
		origCheckKustomizationStatus := checkKustomizationStatus
		checkKustomizationStatus = func(_ string, names []string) (map[string]bool, error) {
			m := make(map[string]bool)
			for _, n := range names {
				m[n] = true
			}
			return m, nil
		}
		defer func() { checkKustomizationStatus = origCheckKustomizationStatus }()

		// Patch checkGitRepositoryStatus to no-op
		origCheckGitRepositoryStatus := checkGitRepositoryStatus
		checkGitRepositoryStatus = func(_ string) error { return nil }
		defer func() { checkGitRepositoryStatus = origCheckGitRepositoryStatus }()

		// When calling Down
		err := baseHandler.Down()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// And kubeClientResourceOperation should be called for each kustomization with cleanup
		if len(calledConfigs) != 3 {
			t.Fatalf("expected 3 calls to kubeClientResourceOperation, got %d", len(calledConfigs))
		}

		// And the resource names should be k1-cleanup, k2-cleanup, k3-cleanup
		expectedNames := map[string]bool{"k1-cleanup": true, "k2-cleanup": true, "k3-cleanup": true}
		for _, config := range calledConfigs {
			if !expectedNames[config.ResourceInstanceName] {
				t.Errorf("unexpected ResourceInstanceName '%s'", config.ResourceInstanceName)
			}
			delete(expectedNames, config.ResourceInstanceName)
		}
		if len(expectedNames) != 0 {
			t.Errorf("expected ResourceInstanceNames not called: %v", expectedNames)
		}
	})

	t.Run("ApplyKustomizationError", func(t *testing.T) {
		// Given a handler with a kustomization with cleanup
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup/path1"}},
		}

		// Patch kubeClientResourceOperation to error on apply
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			if config.ResourceInstanceName == "k1-cleanup" {
				return fmt.Errorf("apply error")
			}
			return nil
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()

		// Patch checkKustomizationStatus to always return ready
		origCheckKustomizationStatus := checkKustomizationStatus
		checkKustomizationStatus = func(_ string, names []string) (map[string]bool, error) {
			m := make(map[string]bool)
			for _, n := range names {
				m[n] = true
			}
			return m, nil
		}
		defer func() { checkKustomizationStatus = origCheckKustomizationStatus }()

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "apply error") {
			t.Errorf("Expected error about apply error, got: %v", err)
		}
	})

	t.Run("WaitForKustomizationsError", func(t *testing.T) {
		// Given a handler with a kustomization with cleanup
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup/path1"}},
		}

		// Set fast poll interval and short timeout
		baseHandler.kustomizationWaitPollInterval = 1 * time.Millisecond
		for i := range baseHandler.blueprint.Kustomizations {
			if baseHandler.blueprint.Kustomizations[i].Timeout == nil {
				baseHandler.blueprint.Kustomizations[i].Timeout = &metav1.Duration{Duration: 5 * time.Millisecond}
			} else {
				baseHandler.blueprint.Kustomizations[i].Timeout.Duration = 5 * time.Millisecond
			}
		}

		// Patch kubeClientResourceOperation to succeed
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, config ResourceOperationConfig) error {
			return nil
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()

		// Patch checkKustomizationStatus to error
		origCheckKustomizationStatus := checkKustomizationStatus
		checkKustomizationStatus = func(_ string, names []string) (map[string]bool, error) {
			return nil, fmt.Errorf("wait error")
		}
		defer func() { checkKustomizationStatus = origCheckKustomizationStatus }()

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for kustomizations") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	})
}
