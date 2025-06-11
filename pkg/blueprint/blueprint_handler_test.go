package blueprint

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/shell"
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
	Injector          di.Injector
	Shell             *shell.MockShell
	ConfigHandler     config.ConfigHandler
	Shims             *Shims
	KubernetesManager *kubernetes.MockKubernetesManager
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

	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Change to temporary directory
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Set environment variable
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	// Create injector
	injector := di.NewInjector()

	// Set up config handler
	var configHandler config.ConfigHandler
	if len(opts) > 0 && opts[0].ConfigHandler != nil {
		configHandler = opts[0].ConfigHandler
	} else {
		configHandler = config.NewYamlConfigHandler(injector)
	}

	// Create mock shell and kubernetes manager
	mockShell := shell.NewMockShell()
	mockKubernetesManager := kubernetes.NewMockKubernetesManager(nil)
	// Initialize safe default implementations for all mock functions
	mockKubernetesManager.DeleteKustomizationFunc = func(name, namespace string) error {
		return nil
	}
	mockKubernetesManager.WaitForKustomizationsDeletedFunc = func(message string, names ...string) error {
		return nil
	}
	mockKubernetesManager.ApplyKustomizationFunc = func(kustomization kustomizev1.Kustomization) error {
		return nil
	}
	mockKubernetesManager.SuspendKustomizationFunc = func(name, namespace string) error {
		return nil
	}
	mockKubernetesManager.GetKustomizationStatusFunc = func(names []string) (map[string]bool, error) {
		status := make(map[string]bool)
		for _, name := range names {
			status[name] = true
		}
		return status, nil
	}
	mockKubernetesManager.GetHelmReleasesForKustomizationFunc = func(name, namespace string) ([]helmv2.HelmRelease, error) {
		return nil, nil
	}
	mockKubernetesManager.SuspendHelmReleaseFunc = func(name, namespace string) error {
		return nil
	}
	mockKubernetesManager.CreateNamespaceFunc = func(name string) error {
		return nil
	}
	mockKubernetesManager.DeleteNamespaceFunc = func(name string) error {
		return nil
	}

	// Register components with injector
	injector.Register("shell", mockShell)
	injector.Register("configHandler", configHandler)
	injector.Register("kubernetesManager", mockKubernetesManager)

	// Set up default config
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
	if len(opts) > 0 && opts[0].ConfigStr != "" {
		if err := configHandler.LoadConfigString(opts[0].ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}

	// Create shims
	shims := setupShims(t)

	// Cleanup function
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		os.Unsetenv("WINDSOR_CONTEXT")
		os.Chdir(tmpDir)
	})

	return &Mocks{
		Injector:          injector,
		Shell:             mockShell,
		ConfigHandler:     configHandler,
		Shims:             shims,
		KubernetesManager: mockKubernetesManager,
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
		// Given a handler
		handler, _ := setup(t)

		// When calling Initialize
		err := handler.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a handler
		handler, mocks := setup(t)

		// And a shell that returns an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("get project root error")
		}

		// When calling Initialize
		err := handler.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "get project root error") {
			t.Errorf("Expected error about get project root error, got: %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given an injector with no config handler registered
		handler, mocks := setup(t)

		mocks.Injector.Register("configHandler", nil)

		// When calling Initialize
		err := handler.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given an injector with no shell registered
		handler, mocks := setup(t)
		mocks.Injector.Register("shell", nil)

		// When calling Initialize
		err := handler.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
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

		// When loading config
		err := handler.LoadConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And only yaml path should be checked since it exists
		expectedPaths := []string{
			"blueprint.yaml",
		}
		for _, expected := range expectedPaths {
			found := false
			for _, checked := range checkedPaths {
				if strings.HasSuffix(checked, expected) {
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

	t.Run("PathBackslashNormalization", func(t *testing.T) {
		handler, _ := setup(t)
		handler.SetKustomizations([]blueprintv1alpha1.Kustomization{
			{Name: "k1", Path: "foo\\bar\\baz"},
		})
		ks := handler.getKustomizations()
		if ks[0].Path != "kustomize/foo/bar/baz" {
			t.Errorf("expected normalized path, got %q", ks[0].Path)
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

	t.Run("ErrorGettingHelmReleases", func(t *testing.T) {
		// Given a handler with a kustomization
		handler, mocks := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1"},
		}

		// Set up mock Kubernetes manager to return error when getting helm releases
		mocks.KubernetesManager.GetHelmReleasesForKustomizationFunc = func(name, namespace string) ([]helmv2.HelmRelease, error) {
			return nil, fmt.Errorf("failed to get helm releases")
		}

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get helmreleases for kustomization k1") {
			t.Errorf("Expected error about failed helm releases, got: %v", err)
		}
	})

	t.Run("ErrorSuspendingHelmRelease", func(t *testing.T) {
		// Given a handler with a kustomization
		handler, mocks := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1"},
		}

		// Set up mock Kubernetes manager to return a helm release and error on suspend
		mocks.KubernetesManager.GetHelmReleasesForKustomizationFunc = func(name, namespace string) ([]helmv2.HelmRelease, error) {
			return []helmv2.HelmRelease{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-release",
						Namespace: "test-namespace",
					},
				},
			}, nil
		}
		mocks.KubernetesManager.SuspendHelmReleaseFunc = func(name, namespace string) error {
			return fmt.Errorf("failed to suspend helm release")
		}

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to suspend helmrelease test-release in namespace test-namespace") {
			t.Errorf("Expected error about failed helm release suspension, got: %v", err)
		}
	})

	t.Run("SuspendKustomizationError", func(t *testing.T) {
		// Given a handler with a kustomization
		handler, mocks := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1"},
		}

		// Set up mock Kubernetes manager to return error on suspend
		mocks.KubernetesManager.SuspendKustomizationFunc = func(name, namespace string) error {
			return fmt.Errorf("suspend error")
		}

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "suspend error") {
			t.Errorf("Expected error about suspend error, got: %v", err)
		}
	})

	t.Run("ErrorWaitingForKustomizationsDeleted", func(t *testing.T) {
		// Given a handler with a kustomization
		handler, mocks := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1"},
		}

		// Set up mock Kubernetes manager to return error on wait for deletion
		mocks.KubernetesManager.WaitForKustomizationsDeletedFunc = func(message string, names ...string) error {
			return fmt.Errorf("wait for deletion error")
		}

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed waiting for kustomizations to be deleted") {
			t.Errorf("Expected error about waiting for kustomizations to be deleted, got: %v", err)
		}
	})

	t.Run("ErrorDeletingCleanupKustomization", func(t *testing.T) {
		// Given a handler with a kustomization with a cleanup path
		handler, mocks := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup"}},
		}

		// Set up mock Kubernetes manager to return error on delete for cleanup
		mocks.KubernetesManager.DeleteKustomizationFunc = func(name, namespace string) error {
			return fmt.Errorf("delete cleanup error")
		}

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete kustomization") {
			t.Errorf("Expected error about failed to delete kustomization, got: %v", err)
		}
	})

	t.Run("ErrorWaitingForCleanupKustomizationsDeleted", func(t *testing.T) {
		// Given a handler with a kustomization with a cleanup path
		handler, mocks := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup"}},
		}

		// Set up mock Kubernetes manager to return error on wait for cleanup deletion
		mocks.KubernetesManager.WaitForKustomizationsDeletedFunc = func(message string, names ...string) error {
			return fmt.Errorf("wait for cleanup deletion error")
		}

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed waiting for kustomizations to be deleted") {
			t.Errorf("Expected error about failed waiting for kustomizations to be deleted, got: %v", err)
		}
	})
}

func TestBlueprintHandler_Install(t *testing.T) {
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
		// Given a blueprint handler with repository, sources, and kustomizations
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
				Url:  "https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.SetSources(expectedSources)

		expectedKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name: "kustomization1",
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

	t.Run("KustomizationDefaults", func(t *testing.T) {
		// Given a blueprint handler with repository and kustomizations
		handler, mocks := setup(t)

		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		// And a blueprint with metadata name
		handler.(*BaseBlueprintHandler).blueprint.Metadata.Name = "test-blueprint"

		// And kustomizations with various configurations
		kustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name: "k1", // No source, should use blueprint name
			},
			{
				Name:   "k2",
				Source: "custom-source", // Explicit source
			},
			{
				Name: "k3", // No path, should default to "kustomize"
			},
			{
				Name: "k4",
				Path: "custom/path", // Custom path, should be prefixed with "kustomize/"
			},
			{
				Name: "k5", // No intervals/timeouts, should use defaults
			},
			{
				Name:          "k6",
				Interval:      &metav1.Duration{Duration: 2 * time.Minute},
				RetryInterval: &metav1.Duration{Duration: 30 * time.Second},
				Timeout:       &metav1.Duration{Duration: 5 * time.Minute},
			},
		}
		handler.SetKustomizations(kustomizations)

		// And a mock that captures the applied kustomizations
		var appliedKustomizations []kustomizev1.Kustomization
		mocks.KubernetesManager.ApplyKustomizationFunc = func(k kustomizev1.Kustomization) error {
			appliedKustomizations = append(appliedKustomizations, k)
			return nil
		}

		// When installing the blueprint
		err = handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected successful installation, but got error: %v", err)
		}

		// And the kustomizations should have the correct defaults
		if len(appliedKustomizations) != 6 {
			t.Fatalf("Expected 6 kustomizations to be applied, got %d", len(appliedKustomizations))
		}

		// Verify k1 (no source)
		if appliedKustomizations[0].Spec.SourceRef.Name != "test-blueprint" {
			t.Errorf("Expected k1 source to be 'test-blueprint', got '%s'", appliedKustomizations[0].Spec.SourceRef.Name)
		}

		// Verify k2 (explicit source)
		if appliedKustomizations[1].Spec.SourceRef.Name != "custom-source" {
			t.Errorf("Expected k2 source to be 'custom-source', got '%s'", appliedKustomizations[1].Spec.SourceRef.Name)
		}

		// Verify k3 (no path)
		if appliedKustomizations[2].Spec.Path != "kustomize" {
			t.Errorf("Expected k3 path to be 'kustomize', got '%s'", appliedKustomizations[2].Spec.Path)
		}

		// Verify k4 (custom path)
		if appliedKustomizations[3].Spec.Path != "kustomize/custom/path" {
			t.Errorf("Expected k4 path to be 'kustomize/custom/path', got '%s'", appliedKustomizations[3].Spec.Path)
		}

		// Verify k5 (default intervals/timeouts)
		if appliedKustomizations[4].Spec.Interval.Duration != constants.DEFAULT_FLUX_KUSTOMIZATION_INTERVAL {
			t.Errorf("Expected k5 interval to be %v, got %v", constants.DEFAULT_FLUX_KUSTOMIZATION_INTERVAL, appliedKustomizations[4].Spec.Interval.Duration)
		}
		if appliedKustomizations[4].Spec.RetryInterval.Duration != constants.DEFAULT_FLUX_KUSTOMIZATION_RETRY_INTERVAL {
			t.Errorf("Expected k5 retry interval to be %v, got %v", constants.DEFAULT_FLUX_KUSTOMIZATION_RETRY_INTERVAL, appliedKustomizations[4].Spec.RetryInterval.Duration)
		}
		if appliedKustomizations[4].Spec.Timeout.Duration != constants.DEFAULT_FLUX_KUSTOMIZATION_TIMEOUT {
			t.Errorf("Expected k5 timeout to be %v, got %v", constants.DEFAULT_FLUX_KUSTOMIZATION_TIMEOUT, appliedKustomizations[4].Spec.Timeout.Duration)
		}

		// Verify k6 (custom intervals/timeouts)
		if appliedKustomizations[5].Spec.Interval.Duration != 2*time.Minute {
			t.Errorf("Expected k6 interval to be 2m, got %v", appliedKustomizations[5].Spec.Interval.Duration)
		}
		if appliedKustomizations[5].Spec.RetryInterval.Duration != 30*time.Second {
			t.Errorf("Expected k6 retry interval to be 30s, got %v", appliedKustomizations[5].Spec.RetryInterval.Duration)
		}
		if appliedKustomizations[5].Spec.Timeout.Duration != 5*time.Minute {
			t.Errorf("Expected k6 timeout to be 5m, got %v", appliedKustomizations[5].Spec.Timeout.Duration)
		}
	})

	t.Run("ApplyKustomizationError", func(t *testing.T) {
		// Given a blueprint handler with repository, sources, and kustomizations
		handler, mocks := setup(t)

		err := handler.SetRepository(blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		})
		if err != nil {
			t.Fatalf("Failed to set repository: %v", err)
		}

		sources := []blueprintv1alpha1.Source{
			{
				Name:       "source1",
				Url:        "git::https://example.com/source1.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				PathPrefix: "terraform",
			},
		}
		handler.SetSources(sources)

		kustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name: "kustomization1",
			},
		}
		handler.SetKustomizations(kustomizations)

		// Set up mock to return error for ApplyKustomization
		mocks.KubernetesManager.ApplyKustomizationFunc = func(kustomization kustomizev1.Kustomization) error {
			return fmt.Errorf("apply error")
		}

		// When installing the blueprint
		err = handler.Install()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply kustomization kustomization1") {
			t.Errorf("Expected error about failed kustomization apply, got: %v", err)
		}
	})
}

func TestBlueprintHandler_Down(t *testing.T) {
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

	t.Run("NoKustomizationsWithCleanup", func(t *testing.T) {
		// Given a handler with kustomizations that have no cleanup paths
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: nil},
			{Name: "k2", Cleanup: []string{}},
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
			{Name: "k1", Path: "", Cleanup: []string{"cleanup/path"}},
		}

		// When calling Down
		err := baseHandler.Down()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("MultipleKustomizationsWithCleanup", func(t *testing.T) {
		// Given a handler with multiple kustomizations, some with cleanup paths
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Path: "", Cleanup: []string{"cleanup"}},
			{Name: "k2", Path: "", Cleanup: []string{"cleanup"}},
			{Name: "k3", Path: "", Cleanup: []string{"cleanup"}},
			{Name: "k4", Path: "", Cleanup: []string{}},
		}

		// When calling Down
		err := baseHandler.Down()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("ErrorCases", func(t *testing.T) {
		testCases := []struct {
			name           string
			kustomizations []blueprintv1alpha1.Kustomization
			setupMock      func(*kubernetes.MockKubernetesManager)
			expectedError  string
		}{
			{
				name: "ApplyKustomizationError",
				kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1", Cleanup: []string{"cleanup/path1"}},
				},
				setupMock: func(m *kubernetes.MockKubernetesManager) {
					m.ApplyKustomizationFunc = func(kustomization kustomizev1.Kustomization) error {
						return fmt.Errorf("apply error")
					}
				},
				expectedError: "apply error",
			},
			{
				name: "ErrorDeletingKustomization",
				kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1"},
				},
				setupMock: func(m *kubernetes.MockKubernetesManager) {
					m.DeleteKustomizationFunc = func(name, namespace string) error {
						return fmt.Errorf("delete error")
					}
				},
				expectedError: "delete error",
			},
			{
				name: "SuspendKustomizationError",
				kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1"},
				},
				setupMock: func(m *kubernetes.MockKubernetesManager) {
					m.SuspendKustomizationFunc = func(name, namespace string) error {
						return fmt.Errorf("suspend error")
					}
				},
				expectedError: "suspend error",
			},
			{
				name: "ErrorWaitingForKustomizationsDeleted",
				kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1"},
				},
				setupMock: func(m *kubernetes.MockKubernetesManager) {
					m.WaitForKustomizationsDeletedFunc = func(message string, names ...string) error {
						return fmt.Errorf("wait for deletion error")
					}
				},
				expectedError: "failed waiting for kustomizations to be deleted",
			},
			{
				name: "ErrorWaitingForCleanupKustomizationsDeleted",
				kustomizations: []blueprintv1alpha1.Kustomization{
					{Name: "k1", Cleanup: []string{"cleanup"}},
				},
				setupMock: func(m *kubernetes.MockKubernetesManager) {
					m.WaitForKustomizationsDeletedFunc = func(message string, names ...string) error {
						return fmt.Errorf("wait for cleanup deletion error")
					}
				},
				expectedError: "failed waiting for kustomizations to be deleted",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Given a handler with the test case kustomizations
				handler, mocks := setup(t)
				baseHandler := handler
				baseHandler.blueprint.Kustomizations = tc.kustomizations

				// And the mock setup
				tc.setupMock(mocks.KubernetesManager)

				// When calling Down
				err := baseHandler.Down()

				// Then an error should be returned
				if err == nil {
					t.Error("Expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("Expected error containing %q, got: %v", tc.expectedError, err)
				}
			})
		}
	})
}

func TestBaseBlueprintHandler_isValidTerraformRemoteSource(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("ValidGitHTTPS", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// When checking a valid git HTTPS source
		source := "git::https://github.com/example/repo.git"
		valid := handler.isValidTerraformRemoteSource(source)

		// Then it should be valid
		if !valid {
			t.Errorf("Expected %s to be valid, got invalid", source)
		}
	})

	t.Run("ValidGitSSH", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// When checking a valid git SSH source
		source := "git@github.com:example/repo.git"
		valid := handler.isValidTerraformRemoteSource(source)

		// Then it should be valid
		if !valid {
			t.Errorf("Expected %s to be valid, got invalid", source)
		}
	})

	t.Run("ValidHTTPS", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// When checking a valid HTTPS source
		source := "https://github.com/example/repo.git"
		valid := handler.isValidTerraformRemoteSource(source)

		// Then it should be valid
		if !valid {
			t.Errorf("Expected %s to be valid, got invalid", source)
		}
	})

	t.Run("ValidZip", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// When checking a valid ZIP source
		source := "https://github.com/example/repo/archive/main.zip"
		valid := handler.isValidTerraformRemoteSource(source)

		// Then it should be valid
		if !valid {
			t.Errorf("Expected %s to be valid, got invalid", source)
		}
	})

	t.Run("ValidRegistry", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// When checking a valid registry source
		source := "registry.terraform.io/example/module"
		valid := handler.isValidTerraformRemoteSource(source)

		// Then it should be valid
		if !valid {
			t.Errorf("Expected %s to be valid, got invalid", source)
		}
	})

	t.Run("ValidCustomDomain", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// When checking a valid custom domain source
		source := "example.com/module"
		valid := handler.isValidTerraformRemoteSource(source)

		// Then it should be valid
		if !valid {
			t.Errorf("Expected %s to be valid, got invalid", source)
		}
	})

	t.Run("InvalidSource", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// When checking an invalid source
		source := "invalid-source"
		valid := handler.isValidTerraformRemoteSource(source)

		// Then it should be invalid
		if valid {
			t.Errorf("Expected %s to be invalid, got valid", source)
		}
	})

	t.Run("InvalidRegex", func(t *testing.T) {
		// Given a blueprint handler with a mock that returns error
		handler, mocks := setup(t)
		mocks.Shims.RegexpMatchString = func(pattern, s string) (bool, error) {
			return false, fmt.Errorf("mock regex error")
		}

		// When checking a source with regex error
		source := "git::https://github.com/example/repo.git"
		valid := handler.isValidTerraformRemoteSource(source)

		// Then it should be invalid
		if valid {
			t.Errorf("Expected %s to be invalid with regex error, got valid", source)
		}
	})
}

func TestBaseBlueprintHandler_CreateManagedNamespace(t *testing.T) {
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
		handler, mocks := setup(t)

		// And a mock Kubernetes manager that tracks calls
		var createdNamespace string
		mocks.KubernetesManager.CreateNamespaceFunc = func(name string) error {
			createdNamespace = name
			return nil
		}

		// When creating a managed namespace
		err := handler.createManagedNamespace("test-namespace")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And the correct namespace should be created
		if createdNamespace != "test-namespace" {
			t.Errorf("Expected namespace 'test-namespace', got: %s", createdNamespace)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// And a mock Kubernetes manager that returns an error
		mocks.KubernetesManager.CreateNamespaceFunc = func(name string) error {
			return fmt.Errorf("mock create error")
		}

		// When creating a managed namespace
		err := handler.createManagedNamespace("test-namespace")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock create error") {
			t.Errorf("Expected error about create error, got: %v", err)
		}
	})
}

func TestBaseBlueprintHandler_DeleteNamespace(t *testing.T) {
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
		handler, mocks := setup(t)

		// And a mock Kubernetes manager that tracks calls
		var deletedNamespace string
		mocks.KubernetesManager.DeleteNamespaceFunc = func(name string) error {
			deletedNamespace = name
			return nil
		}

		// When deleting a namespace
		err := handler.deleteNamespace("test-namespace")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And the correct namespace should be deleted
		if deletedNamespace != "test-namespace" {
			t.Errorf("Expected namespace 'test-namespace', got: %s", deletedNamespace)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// And a mock Kubernetes manager that returns an error
		mocks.KubernetesManager.DeleteNamespaceFunc = func(name string) error {
			return fmt.Errorf("mock delete error")
		}

		// When deleting a namespace
		err := handler.deleteNamespace("test-namespace")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock delete error") {
			t.Errorf("Expected error about delete error, got: %v", err)
		}
	})
}

func TestBlueprintHandler_GetRepository(t *testing.T) {
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

	t.Run("ReturnsExpectedRepository", func(t *testing.T) {
		// Given a blueprint handler with a set repository
		handler, _ := setup(t)
		expectedRepo := blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}
		handler.SetRepository(expectedRepo)

		// When getting the repository
		repo := handler.GetRepository()

		// Then the expected repository should be returned
		if repo != expectedRepo {
			t.Errorf("Expected repository %+v, got %+v", expectedRepo, repo)
		}
	})

	t.Run("ReturnsDefaultValues", func(t *testing.T) {
		// Given a blueprint handler with an empty repository
		handler, _ := setup(t)
		handler.SetRepository(blueprintv1alpha1.Repository{})

		// When getting the repository
		repo := handler.GetRepository()

		// Then default values should be set
		expectedRepo := blueprintv1alpha1.Repository{
			Url: "",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}
		if repo != expectedRepo {
			t.Errorf("Expected repository %+v, got %+v", expectedRepo, repo)
		}
	})
}

func TestBlueprintHandler_GetSources(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{},
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("ReturnsExpectedSources", func(t *testing.T) {
		// Given a blueprint handler with a set of sources
		handler, _ := setup(t)
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name:       "source1",
				Url:        "git::https://example.com/source1.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				PathPrefix: "/source1",
			},
			{
				Name:       "source2",
				Url:        "git::https://example.com/source2.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "develop"},
				PathPrefix: "/source2",
			},
		}
		err := handler.SetSources(expectedSources)
		if err != nil {
			t.Fatalf("Failed to set sources: %v", err)
		}

		// When getting sources
		sources := handler.GetSources()

		// Then the returned sources should match the expected sources
		if len(sources) != len(expectedSources) {
			t.Fatalf("Expected %d sources, got %d", len(expectedSources), len(sources))
		}
		for i := range expectedSources {
			if sources[i] != expectedSources[i] {
				t.Errorf("Source[%d] = %+v, want %+v", i, sources[i], expectedSources[i])
			}
		}
	})
}

func TestBlueprintHandler_GetTerraformComponents(t *testing.T) {
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

	t.Run("ReturnsResolvedComponents", func(t *testing.T) {
		// Given a blueprint handler with terraform components and sources
		handler, _ := setup(t)

		// And a project root directory
		projectRoot := "/test/project"
		handler.projectRoot = projectRoot

		// And a set of sources
		sources := []blueprintv1alpha1.Source{
			{
				Name:       "source1",
				Url:        "https://example.com/source1.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				PathPrefix: "terraform",
			},
		}
		handler.SetSources(sources)

		// And a set of terraform components
		components := []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "path/to/module",
				Values: map[string]any{"key": "value"},
			},
		}
		handler.SetTerraformComponents(components)

		// When getting terraform components
		resolvedComponents := handler.GetTerraformComponents()

		// Then the components should be returned
		if len(resolvedComponents) != 1 {
			t.Fatalf("Expected 1 component, got %d", len(resolvedComponents))
		}

		// And the component should have the correct resolved source
		expectedSource := "https://example.com/source1.git//terraform/path/to/module?ref=main"
		if resolvedComponents[0].Source != expectedSource {
			t.Errorf("Expected source %q, got %q", expectedSource, resolvedComponents[0].Source)
		}

		// And the component should have the correct full path
		expectedPath := filepath.FromSlash(filepath.Join(projectRoot, ".windsor", ".tf_modules", "path/to/module"))
		if resolvedComponents[0].FullPath != expectedPath {
			t.Errorf("Expected path %q, got %q", expectedPath, resolvedComponents[0].FullPath)
		}

		// And the values should be preserved
		if resolvedComponents[0].Values["key"] != "value" {
			t.Errorf("Expected value 'value' for key 'key', got %q", resolvedComponents[0].Values["key"])
		}
	})

	t.Run("HandlesEmptyComponents", func(t *testing.T) {
		// Given a blueprint handler with no terraform components
		handler, _ := setup(t)

		// And an empty set of terraform components
		err := handler.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{})
		if err != nil {
			t.Fatalf("Failed to set empty components: %v", err)
		}

		// When getting terraform components
		components := handler.GetTerraformComponents()

		// Then an empty slice should be returned
		if components == nil {
			t.Error("Expected empty slice, got nil")
		}
		if len(components) != 0 {
			t.Errorf("Expected 0 components, got %d", len(components))
		}
	})

	t.Run("NormalizesPathsWithBackslashes", func(t *testing.T) {
		// Given a blueprint handler with terraform components and sources
		handler, _ := setup(t)

		// And a project root directory
		projectRoot := "/test/project"
		handler.projectRoot = projectRoot

		// And a set of sources
		sources := []blueprintv1alpha1.Source{
			{
				Name:       "source1",
				Url:        "https://example.com/source1.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				PathPrefix: "terraform",
			},
		}
		handler.SetSources(sources)

		// And a set of terraform components with backslashes in paths
		components := []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "path\\to\\module",
				Values: map[string]any{"key": "value"},
			},
		}
		handler.SetTerraformComponents(components)

		// When getting terraform components
		resolvedComponents := handler.GetTerraformComponents()

		// Then the components should be returned
		if len(resolvedComponents) != 1 {
			t.Fatalf("Expected 1 component, got %d", len(resolvedComponents))
		}

		// And the component should have the correct resolved source with backslashes preserved
		expectedSource := "https://example.com/source1.git//terraform/path\\to\\module?ref=main"
		if resolvedComponents[0].Source != expectedSource {
			t.Errorf("Expected source %q, got %q", expectedSource, resolvedComponents[0].Source)
		}

		// And the component should have the correct full path with backslashes preserved
		expectedPath := filepath.Join(projectRoot, ".windsor", ".tf_modules", "path\\to\\module")
		if resolvedComponents[0].FullPath != expectedPath {
			t.Errorf("Expected path %q, got %q", expectedPath, resolvedComponents[0].FullPath)
		}
	})
}

func TestBlueprintHandler_GetKustomizations(t *testing.T) {
	// Test removed - moved to private test file
}
