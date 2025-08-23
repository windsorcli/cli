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

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	kustomize "github.com/fluxcd/pkg/apis/kustomize"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/goccy/go-yaml"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/shell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// =============================================================================
// Test Setup
// =============================================================================

// mockFileInfo implements os.FileInfo for testing
type mockFileInfo struct {
	name  string
	isDir bool
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return 0 }
func (m mockFileInfo) Mode() os.FileMode  { return 0644 }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return m.isDir }
func (m mockFileInfo) Sys() any           { return nil }

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

type mockDirEntry struct {
	name  string
	isDir bool
}

func (m *mockDirEntry) Name() string               { return m.name }
func (m *mockDirEntry) IsDir() bool                { return m.isDir }
func (m *mockDirEntry) Type() os.FileMode          { return 0 }
func (m *mockDirEntry) Info() (os.FileInfo, error) { return nil, nil }

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

	shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
		return nil
	}

	shims.Stat = func(name string) (os.FileInfo, error) {
		if strings.Contains(name, "blueprint.yaml") || strings.Contains(name, "blueprint.jsonnet") {
			return nil, nil
		}
		// Default: template directory does not exist (triggers default blueprint generation)
		return nil, os.ErrNotExist
	}

	shims.MkdirAll = func(name string, perm fs.FileMode) error {
		return nil
	}

	// Default: empty template directory (successful template processing)
	shims.ReadDir = func(name string) ([]os.DirEntry, error) {
		return []os.DirEntry{}, nil
	}

	shims.NewJsonnetVM = func() JsonnetVM {
		return NewMockJsonnetVM(func(filename, snippet string) (string, error) {
			return "", nil
		})
	}

	// Override timing shims for fast tests
	shims.TimeAfter = func(d time.Duration) <-chan time.Time {
		// Return a channel that never fires (no timeout for tests)
		return make(chan time.Time)
	}

	shims.NewTicker = func(d time.Duration) *time.Ticker {
		// Return a ticker that ticks immediately for tests
		ticker := time.NewTicker(1 * time.Millisecond)
		return ticker
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

	// Set up config handler - default to MockConfigHandler for easier testing
	var configHandler config.ConfigHandler
	if len(opts) > 0 && opts[0].ConfigHandler != nil {
		configHandler = opts[0].ConfigHandler
	} else {
		mockConfigHandler := config.NewMockConfigHandler()
		// Set up default mock behaviors with stateful context handling
		currentContext := "local" // Default context

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "cluster.platform":
				return "default"
			case "context":
				return currentContext
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}

		mockConfigHandler.GetContextFunc = func() string {
			// Check environment variable first, like the real ConfigHandler does
			if envContext := os.Getenv("WINDSOR_CONTEXT"); envContext != "" {
				return envContext
			}
			return currentContext
		}

		mockConfigHandler.SetContextFunc = func(context string) error {
			currentContext = context
			return nil
		}

		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		configHandler = mockConfigHandler
	}

	// Create mock shell and kubernetes manager
	mockShell := shell.NewMockShell()
	// Set default GetProjectRoot implementation
	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/mock/project", nil
	}

	mockKubernetesManager := kubernetes.NewMockKubernetesManager(nil)
	// Initialize safe default implementations for all mock functions
	mockKubernetesManager.DeleteKustomizationFunc = func(name, namespace string) error {
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
			// Return true for all kustomizations, including cleanup ones
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
	t.Run("CreatesHandlerWithMocks", func(t *testing.T) {
		// Given an injector with mocks
		mocks := setupMocks(t)

		// When creating a new blueprint handler
		handler := NewBlueprintHandler(mocks.Injector)

		// Then the handler should be properly initialized
		if handler == nil {
			t.Fatal("Expected non-nil handler")
		}

		// And basic fields should be set
		if handler.injector == nil {
			t.Error("Expected injector to be set")
		}
		if handler.shims == nil {
			t.Error("Expected shims to be set")
		}

		// And dependency fields should be nil until Initialize() is called
		if handler.configHandler != nil {
			t.Error("Expected configHandler to be nil before Initialize()")
		}
		if handler.shell != nil {
			t.Error("Expected shell to be nil before Initialize()")
		}
		if handler.kubernetesManager != nil {
			t.Error("Expected kubernetesManager to be nil before Initialize()")
		}

		// When Initialize is called
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Initialize() failed: %v", err)
		}

		// Then dependencies should be injected
		if handler.configHandler == nil {
			t.Error("Expected configHandler to be set after Initialize()")
		}
		if handler.shell == nil {
			t.Error("Expected shell to be set after Initialize()")
		}
		if handler.kubernetesManager == nil {
			t.Error("Expected kubernetesManager to be set after Initialize()")
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

	t.Run("ErrorResolvingKubernetesManager", func(t *testing.T) {
		// Given a handler with missing kubernetesManager
		handler, mocks := setup(t)

		// And an injector that registers nil for kubernetesManager
		mocks.Injector.Register("configHandler", mocks.ConfigHandler)
		mocks.Injector.Register("shell", mocks.Shell)
		mocks.Injector.Register("kubernetesManager", nil)

		// When calling Initialize
		err := handler.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving kubernetesManager") {
			t.Errorf("Expected kubernetesManager resolution error, got: %v", err)
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

		// Then an error should be returned since blueprint.yaml doesn't exist
		if err == nil {
			t.Errorf("Expected error when blueprint.yaml doesn't exist, got nil")
		}

		// And the error should indicate blueprint.yaml not found
		if !strings.Contains(err.Error(), "blueprint.yaml not found") {
			t.Errorf("Expected error about blueprint.yaml not found, got: %v", err)
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

	t.Run("ErrorReadingYamlFile", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock file system that finds yaml file but fails to read it
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, "blueprint.yaml") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, "blueprint.yaml") {
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

	t.Run("ErrorUnmarshallingYamlBlueprint", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock file system with a yaml file
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, "blueprint.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, "blueprint.yaml") {
				return []byte("invalid: yaml: content"), nil
			}
			return nil, os.ErrNotExist
		}

		// And a mock yaml unmarshaller that returns an error
		handler.shims.YamlUnmarshal = func(data []byte, obj any) error {
			return fmt.Errorf("error unmarshalling blueprint data")
		}

		// When loading the config
		err := handler.LoadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error unmarshalling blueprint data") {
			t.Errorf("Expected error containing 'error unmarshalling blueprint data', got: %v", err)
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

		// Then an error should be returned since blueprint.yaml doesn't exist
		if err == nil {
			t.Errorf("Expected error when blueprint.yaml doesn't exist, got nil")
		}

		// And the error should indicate blueprint.yaml not found
		if !strings.Contains(err.Error(), "blueprint.yaml not found") {
			t.Errorf("Expected error about blueprint.yaml not found, got: %v", err)
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
		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Path: "foo\\bar\\baz"},
		}
		ks := handler.GetKustomizations()
		if ks[0].Path != "kustomize/foo/bar/baz" {
			t.Errorf("expected normalized path, got %q", ks[0].Path)
		}
	})
}

func TestBlueprintHandler_Install(t *testing.T) {
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
		// Given a blueprint handler with repository, sources, and kustomizations
		handler, _ := setup(t)

		handler.blueprint.Repository = blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}

		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.blueprint.Sources = expectedSources

		expectedKustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name: "kustomization1",
			},
		}
		handler.blueprint.Kustomizations = expectedKustomizations

		// When installing the blueprint
		err := handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected successful installation, but got error: %v", err)
		}
	})

	t.Run("KustomizationDefaults", func(t *testing.T) {
		// Given a blueprint handler with repository and kustomizations
		handler, mocks := setup(t)

		handler.blueprint.Repository = blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}

		// And a blueprint with metadata name
		handler.blueprint.Metadata.Name = "test-blueprint"

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
		handler.blueprint.Kustomizations = kustomizations

		// And a mock that captures the applied kustomizations
		var appliedKustomizations []kustomizev1.Kustomization
		mocks.KubernetesManager.ApplyKustomizationFunc = func(k kustomizev1.Kustomization) error {
			appliedKustomizations = append(appliedKustomizations, k)
			return nil
		}

		// When installing the blueprint
		err := handler.Install()

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

		handler.blueprint.Repository = blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}

		sources := []blueprintv1alpha1.Source{
			{
				Name:       "source1",
				Url:        "git::https://example.com/source1.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				PathPrefix: "terraform",
			},
		}
		handler.blueprint.Sources = sources

		kustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name: "kustomization1",
			},
		}
		handler.blueprint.Kustomizations = kustomizations

		// Set up mock to return error for ApplyKustomization
		mocks.KubernetesManager.ApplyKustomizationFunc = func(kustomization kustomizev1.Kustomization) error {
			return fmt.Errorf("apply error")
		}

		// When installing the blueprint
		err := handler.Install()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply kustomization kustomization1") {
			t.Errorf("Expected error about failed kustomization apply, got: %v", err)
		}
	})

	t.Run("Error_CreateManagedNamespace", func(t *testing.T) {
		// Given a blueprint handler with namespace creation error
		handler, mocks := setup(t)

		// Override: CreateNamespace returns error
		mocks.KubernetesManager.CreateNamespaceFunc = func(name string) error {
			return fmt.Errorf("namespace creation error")
		}

		// When installing the blueprint
		err := handler.Install()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create namespace") {
			t.Errorf("Expected namespace creation error, got: %v", err)
		}
	})

	t.Run("Error_ApplyMainRepository", func(t *testing.T) {
		// Given a blueprint handler with main repository apply error
		handler, mocks := setup(t)

		handler.blueprint.Repository = blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}

		// Override: ApplyGitRepository returns error
		mocks.KubernetesManager.ApplyGitRepositoryFunc = func(repo *sourcev1.GitRepository) error {
			return fmt.Errorf("git repository apply error")
		}

		// When installing the blueprint
		err := handler.Install()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply blueprint repository") {
			t.Errorf("Expected main repository error, got: %v", err)
		}
	})

	t.Run("Error_ApplySourceRepository", func(t *testing.T) {
		// Given a blueprint handler with source repository apply error
		handler, mocks := setup(t)

		sources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.blueprint.Sources = sources

		// Override: ApplyGitRepository returns error for sources
		mocks.KubernetesManager.ApplyGitRepositoryFunc = func(repo *sourcev1.GitRepository) error {
			return fmt.Errorf("source repository apply error")
		}

		// When installing the blueprint
		err := handler.Install()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply source source1") {
			t.Errorf("Expected source repository error, got: %v", err)
		}
	})

	t.Run("Error_ApplyConfigMap", func(t *testing.T) {
		// Given a blueprint handler with configmap apply error
		handler, mocks := setup(t)

		// Override: ApplyConfigMap returns error
		mocks.KubernetesManager.ApplyConfigMapFunc = func(name, namespace string, data map[string]string) error {
			return fmt.Errorf("configmap apply error")
		}

		// When installing the blueprint
		err := handler.Install()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply values configmaps") {
			t.Errorf("Expected values configmaps error, got: %v", err)
		}
	})

	t.Run("Success_EmptyRepositoryUrl", func(t *testing.T) {
		// Given a blueprint handler with empty repository URL
		handler, _ := setup(t)

		// Repository with empty URL should be skipped
		handler.blueprint.Repository = blueprintv1alpha1.Repository{
			Url: "",
		}

		sources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		handler.blueprint.Sources = sources

		// When installing the blueprint
		err := handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("Success_NoSources", func(t *testing.T) {
		// Given a blueprint handler with no sources
		handler, _ := setup(t)

		handler.blueprint.Repository = blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}

		// No sources defined
		handler.blueprint.Sources = []blueprintv1alpha1.Source{}

		// When installing the blueprint
		err := handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("Success_NoKustomizations", func(t *testing.T) {
		// Given a blueprint handler with no kustomizations
		handler, _ := setup(t)

		handler.blueprint.Repository = blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}

		// No kustomizations defined
		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{}

		// When installing the blueprint
		err := handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("Success_WithSecretName", func(t *testing.T) {
		// Given a blueprint handler with repository that has secret name
		handler, _ := setup(t)

		handler.blueprint.Repository = blueprintv1alpha1.Repository{
			Url:        "git::https://example.com/private-repo.git",
			Ref:        blueprintv1alpha1.Reference{Branch: "main"},
			SecretName: "git-credentials",
		}

		sources := []blueprintv1alpha1.Source{
			{
				Name:       "source1",
				Url:        "https://example.com/private-source.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				SecretName: "source-credentials",
			},
		}
		handler.blueprint.Sources = sources

		// When installing the blueprint
		err := handler.Install()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})
}

func TestBlueprintHandler_WaitForKustomizations(t *testing.T) {
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

	// setupFastTiming sets up fast timing mocks for testing
	setupFastTiming := func(handler *BaseBlueprintHandler) {
		// Mock TimeAfter to return a channel that never fires (for non-timeout tests)
		// or fires immediately (for timeout tests)
		handler.shims.TimeAfter = func(d time.Duration) <-chan time.Time {
			if d <= 1*time.Millisecond {
				// For very short durations (timeout tests), fire immediately
				ch := make(chan time.Time, 1)
				ch <- time.Now()
				return ch
			}
			// For normal durations, return a channel that never fires
			return make(chan time.Time)
		}

		// Mock NewTicker to return a ticker that fires every 1ms
		handler.shims.NewTicker = func(d time.Duration) *time.Ticker {
			return time.NewTicker(1 * time.Millisecond)
		}

		// Keep the original TickerStop
		handler.shims.TickerStop = func(t *time.Ticker) { t.Stop() }
	}

	t.Run("Success_ImmediateReady", func(t *testing.T) {
		// Given a blueprint handler with kustomizations that are immediately ready
		handler, mocks := setup(t)
		setupFastTiming(handler)

		// Set up blueprint with kustomizations
		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "test-kustomization-1"},
			{Name: "test-kustomization-2"},
		}

		// Track method calls
		checkGitRepoStatusCalled := false
		getKustomizationStatusCalled := false

		// Override: return ready status immediately
		mocks.KubernetesManager.CheckGitRepositoryStatusFunc = func() error {
			checkGitRepoStatusCalled = true
			return nil
		}
		mocks.KubernetesManager.GetKustomizationStatusFunc = func(names []string) (map[string]bool, error) {
			getKustomizationStatusCalled = true
			status := make(map[string]bool)
			for _, name := range names {
				status[name] = true // All ready
			}
			return status, nil
		}

		// When waiting for kustomizations
		err := handler.WaitForKustomizations("Testing kustomizations")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And CheckGitRepositoryStatus should be called
		if !checkGitRepoStatusCalled {
			t.Error("Expected CheckGitRepositoryStatus to be called")
		}

		// And GetKustomizationStatus should be called
		if !getKustomizationStatusCalled {
			t.Error("Expected GetKustomizationStatus to be called")
		}
	})

	t.Run("Success_SpecificNames", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)
		setupFastTiming(handler)

		// Override: return ready status and verify specific names
		mocks.KubernetesManager.CheckGitRepositoryStatusFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.GetKustomizationStatusFunc = func(names []string) (map[string]bool, error) {
			// Verify specific names are passed
			expectedNames := []string{"custom-kustomization-1", "custom-kustomization-2"}
			if len(names) != len(expectedNames) {
				t.Errorf("Expected %d names, got %d", len(expectedNames), len(names))
			}
			for i, name := range names {
				if name != expectedNames[i] {
					t.Errorf("Expected name %s, got %s", expectedNames[i], name)
				}
			}

			status := make(map[string]bool)
			for _, name := range names {
				status[name] = true
			}
			return status, nil
		}

		// When waiting for specific kustomizations
		err := handler.WaitForKustomizations("Testing specific kustomizations", "custom-kustomization-1", "custom-kustomization-2")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("Success_AfterPolling", func(t *testing.T) {
		// Given a blueprint handler with kustomizations
		handler, mocks := setup(t)
		setupFastTiming(handler)

		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "test-kustomization"},
		}

		// Override: return not ready initially, then ready
		callCount := 0
		mocks.KubernetesManager.CheckGitRepositoryStatusFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.GetKustomizationStatusFunc = func(names []string) (map[string]bool, error) {
			callCount++
			status := make(map[string]bool)
			for _, name := range names {
				// Ready on second call
				status[name] = callCount >= 2
			}
			return status, nil
		}

		// When waiting for kustomizations
		err := handler.WaitForKustomizations("Testing polling")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And GetKustomizationStatus should be called multiple times
		if callCount < 2 {
			t.Errorf("Expected at least 2 calls to GetKustomizationStatus, got %d", callCount)
		}
	})

	t.Run("Error_GitRepositoryStatus", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)
		setupFastTiming(handler)

		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "test-kustomization"},
		}

		// Override: return git repository error
		mocks.KubernetesManager.CheckGitRepositoryStatusFunc = func() error {
			return fmt.Errorf("git repository not ready")
		}

		// When waiting for kustomizations
		err := handler.WaitForKustomizations("Testing git repo error")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "git repository error") {
			t.Errorf("Expected git repository error, got: %v", err)
		}
	})

	t.Run("Error_KustomizationStatus", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)
		setupFastTiming(handler)

		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "test-kustomization"},
		}

		// Override: return kustomization error
		mocks.KubernetesManager.CheckGitRepositoryStatusFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.GetKustomizationStatusFunc = func(names []string) (map[string]bool, error) {
			return nil, fmt.Errorf("kustomization status error")
		}

		// When waiting for kustomizations
		err := handler.WaitForKustomizations("Testing kustomization error")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomization error") {
			t.Errorf("Expected kustomization error, got: %v", err)
		}
	})

	t.Run("Error_ConsecutiveFailures", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)
		setupFastTiming(handler)

		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "test-kustomization"},
		}

		// Override: return errors consistently
		callCount := 0
		mocks.KubernetesManager.CheckGitRepositoryStatusFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.GetKustomizationStatusFunc = func(names []string) (map[string]bool, error) {
			callCount++
			return nil, fmt.Errorf("persistent error %d", callCount)
		}

		// When waiting for kustomizations
		err := handler.WaitForKustomizations("Testing consecutive failures")

		// Then an error should be returned mentioning consecutive failures
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "consecutive failures") {
			t.Errorf("Expected consecutive failures error, got: %v", err)
		}

		// And GetKustomizationStatus should be called multiple times (initial + 4 more failures = 5 total)
		expectedCalls := 5 // 1 initial + 4 more failures to reach max of 5 consecutive failures
		if callCount != expectedCalls {
			t.Errorf("Expected %d calls to GetKustomizationStatus, got %d", expectedCalls, callCount)
		}
	})

	t.Run("Error_RecoveryFromFailures", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)
		setupFastTiming(handler)

		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "test-kustomization"},
		}

		// Override: fail a few times then succeed
		callCount := 0
		mocks.KubernetesManager.CheckGitRepositoryStatusFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.GetKustomizationStatusFunc = func(names []string) (map[string]bool, error) {
			callCount++
			if callCount <= 3 {
				return nil, fmt.Errorf("temporary error %d", callCount)
			}
			// Success after 3 failures
			status := make(map[string]bool)
			for _, name := range names {
				status[name] = true
			}
			return status, nil
		}

		// When waiting for kustomizations
		err := handler.WaitForKustomizations("Testing recovery")

		// Then no error should be returned (it should recover)
		if err != nil {
			t.Errorf("Expected no error after recovery, got: %v", err)
		}

		// And GetKustomizationStatus should be called 4 times (1 initial + 3 failures + 1 success)
		expectedCalls := 4
		if callCount != expectedCalls {
			t.Errorf("Expected %d calls to GetKustomizationStatus, got %d", expectedCalls, callCount)
		}
	})

	t.Run("Timeout_ExceedsMaxWaitTime", func(t *testing.T) {
		// Given a blueprint handler with very short timeout
		handler, mocks := setup(t)

		// Set up kustomizations with very short timeout
		shortTimeout := &metav1.Duration{Duration: 1 * time.Millisecond}
		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "test-kustomization", Timeout: shortTimeout},
		}

		// Setup fast timing that will timeout immediately for short durations
		setupFastTiming(handler)

		// Override: never be ready
		mocks.KubernetesManager.CheckGitRepositoryStatusFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.GetKustomizationStatusFunc = func(names []string) (map[string]bool, error) {
			status := make(map[string]bool)
			for _, name := range names {
				status[name] = false // Never ready
			}
			return status, nil
		}

		// When waiting for kustomizations
		err := handler.WaitForKustomizations("Testing timeout")

		// Then a timeout error should be returned
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for kustomizations") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	})

	t.Run("EmptyKustomizationNames", func(t *testing.T) {
		// Given a blueprint handler with no kustomizations
		handler, mocks := setup(t)
		setupFastTiming(handler)

		// No kustomizations in blueprint
		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{}

		// Override: verify empty names list
		mocks.KubernetesManager.CheckGitRepositoryStatusFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.GetKustomizationStatusFunc = func(names []string) (map[string]bool, error) {
			if len(names) != 0 {
				t.Errorf("Expected empty names list, got %v", names)
			}
			return make(map[string]bool), nil
		}

		// When waiting for kustomizations
		err := handler.WaitForKustomizations("Testing empty")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error for empty kustomizations, got: %v", err)
		}
	})

	t.Run("EmptySpecificNames", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)
		setupFastTiming(handler)

		// Set up blueprint with kustomizations
		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "blueprint-kustomization"},
		}

		// Override: verify blueprint kustomizations are used when empty names provided
		mocks.KubernetesManager.CheckGitRepositoryStatusFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.GetKustomizationStatusFunc = func(names []string) (map[string]bool, error) {
			expectedNames := []string{"blueprint-kustomization"}
			if len(names) != len(expectedNames) {
				t.Errorf("Expected %d names, got %d", len(expectedNames), len(names))
			}
			if names[0] != expectedNames[0] {
				t.Errorf("Expected name %s, got %s", expectedNames[0], names[0])
			}

			status := make(map[string]bool)
			for _, name := range names {
				status[name] = true
			}
			return status, nil
		}

		// When waiting with empty string as name
		err := handler.WaitForKustomizations("Testing empty names", "")

		// Then no error should be returned and blueprint kustomizations should be used
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("PartialReadiness", func(t *testing.T) {
		// Given a blueprint handler with multiple kustomizations
		handler, mocks := setup(t)
		setupFastTiming(handler)

		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "kustomization-1"},
			{Name: "kustomization-2"},
			{Name: "kustomization-3"},
		}

		// Override: simulate gradual readiness
		callCount := 0
		mocks.KubernetesManager.CheckGitRepositoryStatusFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.GetKustomizationStatusFunc = func(names []string) (map[string]bool, error) {
			callCount++
			status := make(map[string]bool)

			// Simulate gradual readiness
			switch callCount {
			case 1:
				// First call: only kustomization-1 ready
				status["kustomization-1"] = true
				status["kustomization-2"] = false
				status["kustomization-3"] = false
			case 2:
				// Second call: kustomization-1 and kustomization-2 ready
				status["kustomization-1"] = true
				status["kustomization-2"] = true
				status["kustomization-3"] = false
			default:
				// Third call and beyond: all ready
				status["kustomization-1"] = true
				status["kustomization-2"] = true
				status["kustomization-3"] = true
			}

			return status, nil
		}

		// When waiting for kustomizations
		err := handler.WaitForKustomizations("Testing partial readiness")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And GetKustomizationStatus should be called at least 3 times
		if callCount < 3 {
			t.Errorf("Expected at least 3 calls to GetKustomizationStatus, got %d", callCount)
		}
	})

	t.Run("ImmediateReadyWithInitialError", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)
		setupFastTiming(handler)

		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "test-kustomization"},
		}

		// Override: fail on initial check but succeed immediately in polling
		initialCall := true
		mocks.KubernetesManager.CheckGitRepositoryStatusFunc = func() error {
			return nil
		}
		mocks.KubernetesManager.GetKustomizationStatusFunc = func(names []string) (map[string]bool, error) {
			if initialCall {
				initialCall = false
				return nil, fmt.Errorf("initial error")
			}

			// Ready on subsequent calls
			status := make(map[string]bool)
			for _, name := range names {
				status[name] = true
			}
			return status, nil
		}

		// When waiting for kustomizations
		err := handler.WaitForKustomizations("Testing initial error recovery")

		// Then no error should be returned (should recover quickly)
		if err != nil {
			t.Errorf("Expected no error after recovery, got: %v", err)
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

	t.Run("EmptyKustomizations", func(t *testing.T) {
		// Given a handler with no kustomizations
		handler, _ := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{}

		// When calling Down
		err := baseHandler.Down()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorDeletingCleanupKustomizations", func(t *testing.T) {
		// Given a handler with kustomizations that have cleanup paths
		handler, mocks := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Cleanup: []string{"cleanup/path"}},
		}

		// And a mock that fails to delete cleanup kustomizations
		mocks.KubernetesManager.DeleteKustomizationFunc = func(name, namespace string) error {
			if strings.Contains(name, "cleanup") {
				return fmt.Errorf("delete cleanup kustomization error")
			}
			return nil
		}

		// When calling Down
		err := baseHandler.Down()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete cleanup kustomization") {
			t.Errorf("Expected cleanup kustomization deletion error, got: %v", err)
		}
	})

	t.Run("CleanupPathNormalization", func(t *testing.T) {
		// Given a handler with kustomizations that have cleanup paths with backslashes
		handler, mocks := setup(t)
		baseHandler := handler
		baseHandler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Path: "ingress\\base", Cleanup: []string{"cleanup"}},
		}

		// Track the applied kustomization to verify path normalization
		var appliedKustomization kustomizev1.Kustomization
		mocks.KubernetesManager.ApplyKustomizationFunc = func(k kustomizev1.Kustomization) error {
			appliedKustomization = k
			return nil
		}

		// When calling Down
		err := baseHandler.Down()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// And the cleanup path should use forward slashes
		expectedPath := "kustomize/ingress/base/cleanup"
		if appliedKustomization.Spec.Path != expectedPath {
			t.Errorf("Expected cleanup path to be normalized to %s, got %s", expectedPath, appliedKustomization.Spec.Path)
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
		handler.blueprint.Repository = expectedRepo

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
		handler.blueprint.Repository = blueprintv1alpha1.Repository{}

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
		handler.blueprint.Sources = expectedSources

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
		handler.blueprint.Sources = sources

		// And a set of terraform components
		components := []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "path/to/module",
				Values: map[string]any{"key": "value"},
			},
		}
		handler.blueprint.TerraformComponents = components

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
		handler.blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{}

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
		handler.blueprint.Sources = sources

		// And a set of terraform components with backslashes in paths
		components := []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "path\\to\\module",
				Values: map[string]any{"key": "value"},
			},
		}
		handler.blueprint.TerraformComponents = components

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

	t.Run("OCISourceResolution", func(t *testing.T) {
		// Given a blueprint handler with OCI source and terraform component
		handler, _ := setup(t)

		// And a project root directory
		projectRoot := "/test/project"
		handler.projectRoot = projectRoot

		// And an OCI source
		sources := []blueprintv1alpha1.Source{
			{
				Name:       "oci-source",
				Url:        "oci://registry.example.com/modules:v1.0.0",
				Ref:        blueprintv1alpha1.Reference{Tag: "v1.0.0"},
				PathPrefix: "terraform",
			},
		}
		handler.blueprint.Sources = sources

		// And a terraform component using the OCI source
		components := []blueprintv1alpha1.TerraformComponent{
			{
				Source: "oci-source",
				Path:   "cluster/talos",
				Values: map[string]any{"key": "value"},
			},
		}
		handler.blueprint.TerraformComponents = components

		// When getting terraform components
		resolvedComponents := handler.GetTerraformComponents()

		// Then the components should be returned
		if len(resolvedComponents) != 1 {
			t.Fatalf("Expected 1 component, got %d", len(resolvedComponents))
		}

		// And the component should have the correct resolved OCI source
		expectedSource := "oci://registry.example.com/modules:v1.0.0//terraform/cluster/talos"
		if resolvedComponents[0].Source != expectedSource {
			t.Errorf("Expected source %q, got %q", expectedSource, resolvedComponents[0].Source)
		}

		// And the component should have the correct full path
		expectedPath := filepath.FromSlash(filepath.Join(projectRoot, ".windsor", ".tf_modules", "cluster/talos"))
		if resolvedComponents[0].FullPath != expectedPath {
			t.Errorf("Expected path %q, got %q", expectedPath, resolvedComponents[0].FullPath)
		}
	})
}

func TestBlueprintHandler_GetDefaultTemplateData(t *testing.T) {
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

	t.Run("ReturnsDefaultTemplate", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// When getting default template data
		result, err := handler.GetDefaultTemplateData("local")

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should contain blueprint.jsonnet
		if len(result) != 1 {
			t.Fatalf("Expected 1 template file, got: %d", len(result))
		}

		if _, exists := result["blueprint.jsonnet"]; !exists {
			t.Error("Expected blueprint.jsonnet to exist in result")
		}

		if len(result["blueprint.jsonnet"]) == 0 {
			t.Error("Expected blueprint.jsonnet to have content")
		}
	})
}

func TestBlueprintHandler_GetLocalTemplateData(t *testing.T) {
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

	t.Run("ReturnsEmptyMapWhenTemplateDirectoryNotExists", func(t *testing.T) {
		// Given a blueprint handler with no template directory
		handler, mocks := setup(t)

		// Mock shell to return project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return filepath.Join("/mock", "project"), nil
		}

		// Mock shims to return error for template directory (doesn't exist)
		if baseHandler, ok := handler.(*BaseBlueprintHandler); ok {
			baseHandler.shims.Stat = func(path string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			}
		}

		// When getting local template data
		result, err := handler.GetLocalTemplateData()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should be empty map
		if len(result) != 0 {
			t.Errorf("Expected empty map, got: %d items", len(result))
		}
	})

	t.Run("CollectsJsonnetFilesFromTemplateDirectory", func(t *testing.T) {
		// Given a blueprint handler with template directory containing jsonnet files
		handler, mocks := setup(t)

		projectRoot := filepath.Join("/mock", "project")
		templateDir := filepath.Join(projectRoot, "contexts", "_template")

		// Mock shell to return project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}

		// Mock shims to simulate template directory with files
		if baseHandler, ok := handler.(*BaseBlueprintHandler); ok {
			baseHandler.shims.Stat = func(path string) (os.FileInfo, error) {
				if path == templateDir {
					return mockFileInfo{name: "_template"}, nil
				}
				return nil, os.ErrNotExist
			}

			baseHandler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
				if path == templateDir {
					return []os.DirEntry{
						&mockDirEntry{name: "blueprint.jsonnet", isDir: false},
						&mockDirEntry{name: "config.yaml", isDir: false}, // Should be ignored
						&mockDirEntry{name: "terraform", isDir: true},
					}, nil
				}
				if path == filepath.Join(templateDir, "terraform") {
					return []os.DirEntry{
						&mockDirEntry{name: "cluster.jsonnet", isDir: false},
						&mockDirEntry{name: "network.jsonnet", isDir: false},
						&mockDirEntry{name: "README.md", isDir: false}, // Should be ignored
					}, nil
				}
				return nil, fmt.Errorf("directory not found")
			}

			baseHandler.shims.ReadFile = func(path string) ([]byte, error) {
				switch path {
				case filepath.Join(templateDir, "blueprint.jsonnet"):
					return []byte("{ kind: 'Blueprint' }"), nil
				case filepath.Join(templateDir, "terraform", "cluster.jsonnet"):
					return []byte("{ cluster_name: 'test' }"), nil
				case filepath.Join(templateDir, "terraform", "network.jsonnet"):
					return []byte("{ vpc_cidr: '10.0.0.0/16' }"), nil
				default:
					return nil, fmt.Errorf("file not found: %s", path)
				}
			}
		}

		// When getting local template data
		result, err := handler.GetLocalTemplateData()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should contain only jsonnet files
		expectedFiles := []string{
			"blueprint.jsonnet",
			"terraform/cluster.jsonnet",
			"terraform/network.jsonnet",
		}

		if len(result) != len(expectedFiles) {
			t.Errorf("Expected %d files, got: %d", len(expectedFiles), len(result))
		}

		for _, expectedFile := range expectedFiles {
			if _, exists := result[expectedFile]; !exists {
				t.Errorf("Expected file %s to exist in result", expectedFile)
			}
		}

		// Verify non-jsonnet files are ignored
		ignoredFiles := []string{
			"config.yaml",
			"terraform/README.md",
		}

		for _, ignoredFile := range ignoredFiles {
			if _, exists := result[ignoredFile]; exists {
				t.Errorf("Expected file %s to be ignored", ignoredFile)
			}
		}

		// Verify file contents
		if string(result["blueprint.jsonnet"]) != "{ kind: 'Blueprint' }" {
			t.Errorf("Expected blueprint.jsonnet content to match")
		}
		if string(result["terraform/cluster.jsonnet"]) != "{ cluster_name: 'test' }" {
			t.Errorf("Expected terraform/cluster.jsonnet content to match")
		}
	})

	t.Run("ReturnsErrorWhenGetProjectRootFails", func(t *testing.T) {
		// Given a blueprint handler with shell that fails to get project root
		handler, mocks := setup(t)

		// Mock shell to return error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("failed to get project root")
		}

		// When getting local template data
		result, err := handler.GetLocalTemplateData()

		// Then error should occur
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error to contain 'failed to get project root', got: %v", err)
		}

		// And result should be nil
		if result != nil {
			t.Error("Expected result to be nil on error")
		}
	})

	t.Run("ReturnsErrorWhenWalkAndCollectTemplatesFails", func(t *testing.T) {
		// Given a blueprint handler with template directory that fails to read
		handler, mocks := setup(t)

		projectRoot := filepath.Join("/mock", "project")
		templateDir := filepath.Join(projectRoot, "contexts", "_template")

		// Mock shell to return project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}

		// Mock shims to simulate template directory exists but ReadDir fails
		if baseHandler, ok := handler.(*BaseBlueprintHandler); ok {
			baseHandler.shims.Stat = func(path string) (os.FileInfo, error) {
				if path == templateDir {
					return mockFileInfo{name: "_template"}, nil
				}
				return nil, os.ErrNotExist
			}

			baseHandler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
				return nil, fmt.Errorf("failed to read directory")
			}
		}

		// When getting local template data
		result, err := handler.GetLocalTemplateData()

		// Then error should occur
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to collect templates") {
			t.Errorf("Expected error to contain 'failed to collect templates', got: %v", err)
		}

		// And result should be nil
		if result != nil {
			t.Error("Expected result to be nil on error")
		}
	})
}

func TestBlueprintHandler_LoadData(t *testing.T) {
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

		// And blueprint data
		blueprintData := map[string]any{
			"kind":       "Blueprint",
			"apiVersion": "v1alpha1",
			"metadata": map[string]any{
				"name":        "test-blueprint",
				"description": "A test blueprint from data",
				"authors":     []any{"John Doe"},
			},
			"sources": []any{
				map[string]any{
					"name": "test-source",
					"url":  "https://example.com/test-repo.git",
				},
			},
			"terraform": []any{
				map[string]any{
					"source": "test-source",
					"path":   "path/to/code",
					"values": map[string]any{
						"key1": "value1",
					},
				},
			},
		}

		// When loading the data
		err := handler.LoadData(blueprintData)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the metadata should be correctly loaded
		metadata := handler.GetMetadata()
		if metadata.Name != "test-blueprint" {
			t.Errorf("Expected name to be test-blueprint, got %s", metadata.Name)
		}
		if metadata.Description != "A test blueprint from data" {
			t.Errorf("Expected description to be 'A test blueprint from data', got %s", metadata.Description)
		}
		if len(metadata.Authors) != 1 || metadata.Authors[0] != "John Doe" {
			t.Errorf("Expected authors to be ['John Doe'], got %v", metadata.Authors)
		}

		// And the sources should be loaded
		sources := handler.GetSources()
		if len(sources) != 1 {
			t.Errorf("Expected 1 source, got %d", len(sources))
		}
		if sources[0].Name != "test-source" {
			t.Errorf("Expected source name to be 'test-source', got %s", sources[0].Name)
		}

		// And the terraform components should be loaded
		components := handler.GetTerraformComponents()
		if len(components) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(components))
		}
		if components[0].Path != "path/to/code" {
			t.Errorf("Expected component path to be 'path/to/code', got %s", components[0].Path)
		}

		// Note: The GetTerraformComponents() method resolves sources to full URLs,
		// so we can't easily test the raw source names without accessing private fields
	})

	t.Run("MarshalError", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock yaml marshaller that returns an error
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("simulated marshalling error")
		}

		// And blueprint data
		blueprintData := map[string]any{
			"kind": "Blueprint",
		}

		// When loading the data
		err := handler.LoadData(blueprintData)

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected LoadData to fail due to marshalling error, but it succeeded")
		}
		if !strings.Contains(err.Error(), "error marshalling blueprint data to yaml") {
			t.Errorf("Expected error message to contain 'error marshalling blueprint data to yaml', got %v", err)
		}
	})

	t.Run("ProcessBlueprintDataError", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And a mock yaml unmarshaller that returns an error
		handler.shims.YamlUnmarshal = func(data []byte, obj any) error {
			return fmt.Errorf("simulated unmarshalling error")
		}

		// And blueprint data
		blueprintData := map[string]any{
			"kind": "Blueprint",
		}

		// When loading the data
		err := handler.LoadData(blueprintData)

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected LoadData to fail due to unmarshalling error, but it succeeded")
		}
	})

	t.Run("WithOCIArtifactInfo", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)

		// And blueprint data
		blueprintData := map[string]any{
			"kind":       "Blueprint",
			"apiVersion": "v1alpha1",
			"metadata": map[string]any{
				"name":        "oci-blueprint",
				"description": "A blueprint from OCI artifact",
			},
		}

		// And OCI artifact info
		ociInfo := &artifact.OCIArtifactInfo{
			Name: "my-blueprint",
			URL:  "oci://registry.example.com/my-blueprint:v1.0.0",
		}

		// When loading the data with OCI info
		err := handler.LoadData(blueprintData, ociInfo)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the metadata should be correctly loaded
		metadata := handler.GetMetadata()
		if metadata.Name != "oci-blueprint" {
			t.Errorf("Expected name to be oci-blueprint, got %s", metadata.Name)
		}
		if metadata.Description != "A blueprint from OCI artifact" {
			t.Errorf("Expected description to be 'A blueprint from OCI artifact', got %s", metadata.Description)
		}
	})
}

func TestBlueprintHandler_Write(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims

		// Override GetConfigRoot to return the expected path for Write tests
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "mock-config-root", nil
		}

		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a blueprint handler with a loaded blueprint
		handler, mocks := setup(t)

		// Set up the blueprint with test data
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: blueprintv1alpha1.Metadata{
				Name:        "test-blueprint",
				Description: "A test blueprint",
				Authors:     []string{"test-author"},
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/example/repo",
				Ref: blueprintv1alpha1.Reference{
					Branch: "main",
				},
			},
		}

		// And mock file operations
		var writtenPath string
		var writtenContent []byte
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenPath = name
			writtenContent = data
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist // File doesn't exist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("kind: Blueprint\napiVersion: v1alpha1\nmetadata:\n  name: test-blueprint\n"), nil
		}

		// When Write is called
		err := handler.Write()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the file should be written to the correct path
		expectedPath := filepath.Join("mock-config-root", "blueprint.yaml")
		if writtenPath != expectedPath {
			t.Errorf("Expected file path %s, got %s", expectedPath, writtenPath)
		}

		// And the content should be written
		if len(writtenContent) == 0 {
			t.Errorf("Expected content to be written, got empty content")
		}
	})

	t.Run("WithOverwriteTrue", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// Set up the blueprint with test data
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
		}

		// And mock file operations
		var writtenPath string
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenPath = name
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: "blueprint.yaml"}, nil // File exists
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("kind: Blueprint\napiVersion: v1alpha1\n"), nil
		}

		// When Write is called with overwrite true
		err := handler.Write(true)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the file should be written (overwrite)
		expectedPath := filepath.Join("mock-config-root", "blueprint.yaml")
		if writtenPath != expectedPath {
			t.Errorf("Expected file path %s, got %s", expectedPath, writtenPath)
		}
	})

	t.Run("WithOverwriteFalse", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// And mock file operations
		var writtenPath string
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenPath = name
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: "blueprint.yaml"}, nil // File exists
		}

		// When Write is called with overwrite false
		err := handler.Write(false)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the file should NOT be written (skip existing)
		if writtenPath != "" {
			t.Errorf("Expected no file to be written, but got %s", writtenPath)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a blueprint handler with a mock config handler that returns an error
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupMocks(t, opts)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		// When Write is called
		err = handler.Write()

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected error from GetConfigRoot, got nil")
		}
	})

	t.Run("ErrorCreatingDirectory", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// And MkdirAll returns an error
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir error")
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist // File doesn't exist
		}

		// When Write is called
		err := handler.Write()

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected error from MkdirAll, got nil")
		}
	})

	t.Run("ErrorMarshalingBlueprint", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// And YamlMarshal returns an error
		mocks.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("marshal error")
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist // File doesn't exist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		// When Write is called
		err := handler.Write()

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected error from YamlMarshal, got nil")
		}
	})

	t.Run("ClearsAllValues", func(t *testing.T) {
		// Given a blueprint handler with terraform components containing values
		handler, mocks := setup(t)

		// Set up a terraform component with both values and terraform variables
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Source: "core",
					Path:   "cluster/talos",
					Values: map[string]any{
						"cluster_name":     "test-cluster",      // Should be kept (not a terraform variable)
						"cluster_endpoint": "https://test:6443", // Should be filtered if it's a terraform variable
						"controlplanes":    []string{"node1"},   // Should be filtered if it's a terraform variable
						"custom_config":    "some-value",        // Should be kept (not a terraform variable)
					},
				},
			},
		}

		// Set up file system mocks
		var writtenContent []byte
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist // blueprint.yaml doesn't exist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return yaml.Marshal(v)
		}

		// When Write is called
		err := handler.Write()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the written content should have all values cleared
		if len(writtenContent) == 0 {
			t.Errorf("Expected content to be written, got empty content")
		}

		// Parse the written YAML to verify all values are cleared
		var writtenBlueprint blueprintv1alpha1.Blueprint
		err = yaml.Unmarshal(writtenContent, &writtenBlueprint)
		if err != nil {
			t.Errorf("Failed to unmarshal written YAML: %v", err)
		}

		// Verify that the terraform component exists
		if len(writtenBlueprint.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(writtenBlueprint.TerraformComponents))
		}

		component := writtenBlueprint.TerraformComponents[0]

		// Verify all values are cleared from the blueprint.yaml
		if len(component.Values) != 0 {
			t.Errorf("Expected all values to be cleared, but got %d values: %v", len(component.Values), component.Values)
		}

		// Also verify kustomizations have postBuild cleared
		if len(writtenBlueprint.Kustomizations) > 0 {
			for i, kustomization := range writtenBlueprint.Kustomizations {
				if kustomization.PostBuild != nil {
					t.Errorf("Expected PostBuild to be cleared for kustomization %d, but got %v", i, kustomization.PostBuild)
				}
			}
		}
	})

	t.Run("ErrorWritingFile", func(t *testing.T) {
		// Given a blueprint handler
		handler, mocks := setup(t)

		// And WriteFile returns an error
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write error")
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist // File doesn't exist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("test content"), nil
		}

		// When Write is called
		err := handler.Write()

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected error from WriteFile, got nil")
		}
	})
}

func TestTargetHandling(t *testing.T) {
	// Test case 1: Path with no existing Target - should create Target with namespace from patch
	patch1 := blueprintv1alpha1.BlueprintPatch{
		Path: "kustomize/ingress/nginx.yaml",
	}

	// Test case 2: Path with existing Target - should not override existing Target
	patch2 := blueprintv1alpha1.BlueprintPatch{
		Path: "kustomize/ingress/nginx.yaml",
		Target: &kustomize.Selector{
			Kind:      "Service",
			Name:      "nginx-ingress-controller",
			Namespace: "custom-namespace",
		},
	}

	// Test case 3: Flux format with Patch and Target - should use existing Target
	patch3 := blueprintv1alpha1.BlueprintPatch{
		Patch: "apiVersion: v1\nkind: Service\nmetadata:\n  name: nginx-ingress-controller\n  namespace: ingress-nginx",
		Target: &kustomize.Selector{
			Kind:      "Service",
			Name:      "nginx-ingress-controller",
			Namespace: "ingress-nginx",
		},
	}

	// Test case 4: Path with patch that has namespace in metadata - should use patch namespace
	patch4 := blueprintv1alpha1.BlueprintPatch{
		Path: "kustomize/ingress/nginx-with-namespace.yaml",
	}

	// Verify the patches have the expected structure
	if patch1.Path == "" {
		t.Error("Expected patch1 to have Path field")
	}
	if patch1.Target != nil {
		t.Error("Expected patch1 to have no Target field")
	}

	if patch2.Path == "" {
		t.Error("Expected patch2 to have Path field")
	}
	if patch2.Target == nil {
		t.Error("Expected patch2 to have Target field")
	}
	if patch2.Target.Kind != "Service" {
		t.Errorf("Expected patch2 Target Kind to be 'Service', got '%s'", patch2.Target.Kind)
	}
	if patch2.Target.Namespace != "custom-namespace" {
		t.Errorf("Expected patch2 Target Namespace to be 'custom-namespace', got '%s'", patch2.Target.Namespace)
	}

	if patch3.Patch == "" {
		t.Error("Expected patch3 to have Patch field")
	}
	if patch3.Target == nil {
		t.Error("Expected patch3 to have Target field")
	}
	if patch3.Target.Kind != "Service" {
		t.Errorf("Expected patch3 Target Kind to be 'Service', got '%s'", patch3.Target.Kind)
	}
	if patch3.Target.Namespace != "ingress-nginx" {
		t.Errorf("Expected patch3 Target Namespace to be 'ingress-nginx', got '%s'", patch3.Target.Namespace)
	}

	if patch4.Path == "" {
		t.Error("Expected patch4 to have Path field")
	}
	if patch4.Target != nil {
		t.Error("Expected patch4 to have no Target field (will be generated from patch content)")
	}
}

func TestNamespaceExtractionFromPatch(t *testing.T) {
	// Test that namespace is correctly extracted from patch content
	patchContent := `apiVersion: v1
kind: Service
metadata:
  name: nginx-ingress-controller
  namespace: ingress-nginx
spec:
  externalIPs:
  - 10.5.1.1
  type: LoadBalancer`

	var patchData map[string]any
	err := yaml.Unmarshal([]byte(patchContent), &patchData)
	if err != nil {
		t.Fatalf("Failed to unmarshal patch content: %v", err)
	}

	// Extract namespace from patch metadata
	patchNamespace := "default" // fallback
	if metadata, ok := patchData["metadata"].(map[string]any); ok {
		if ns, ok := metadata["namespace"].(string); ok {
			patchNamespace = ns
		}
	}

	if patchNamespace != "ingress-nginx" {
		t.Errorf("Expected namespace 'ingress-nginx', got '%s'", patchNamespace)
	}

	// Test with patch that has no namespace
	patchContentNoNS := `apiVersion: v1
kind: Service
metadata:
  name: nginx-ingress-controller
spec:
  externalIPs:
  - 10.5.1.1
  type: LoadBalancer`

	var patchDataNoNS map[string]any
	err = yaml.Unmarshal([]byte(patchContentNoNS), &patchDataNoNS)
	if err != nil {
		t.Fatalf("Failed to unmarshal patch content: %v", err)
	}

	// Extract namespace from patch metadata
	patchNamespaceNoNS := "default" // fallback
	if metadata, ok := patchDataNoNS["metadata"].(map[string]any); ok {
		if ns, ok := metadata["namespace"].(string); ok {
			patchNamespaceNoNS = ns
		}
	}

	if patchNamespaceNoNS != "default" {
		t.Errorf("Expected namespace 'default' (fallback), got '%s'", patchNamespaceNoNS)
	}
}

func TestToKubernetesKustomizationWithNamespace(t *testing.T) {
	// Create a mock config handler
	mockConfigHandler := &config.MockConfigHandler{}
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return "/tmp/test", nil
	}

	// Create a mock blueprint handler
	handler := &BaseBlueprintHandler{
		projectRoot:   "/tmp/test",
		configHandler: mockConfigHandler,
		shims:         NewShims(),
	}

	// Mock the ReadFile function to return a patch with namespace
	handler.shims.ReadFile = func(name string) ([]byte, error) {
		if strings.Contains(name, "nginx.yaml") {
			return []byte(`apiVersion: v1
kind: Service
metadata:
  name: nginx-ingress-controller
  namespace: ingress-nginx
spec:
  externalIPs:
  - 10.5.1.1
  type: LoadBalancer`), nil
		}
		return nil, fmt.Errorf("file not found")
	}

	// Create a kustomization with a patch that references a file
	kustomization := blueprintv1alpha1.Kustomization{
		Name: "ingress",
		Path: "ingress",
		Patches: []blueprintv1alpha1.BlueprintPatch{
			{
				Path: "kustomize/ingress/nginx.yaml",
			},
		},
		Interval:      &metav1.Duration{Duration: time.Minute},
		RetryInterval: &metav1.Duration{Duration: 2 * time.Minute},
		Timeout:       &metav1.Duration{Duration: 5 * time.Minute},
		Wait:          &[]bool{true}[0],
		Force:         &[]bool{false}[0],
		Prune:         &[]bool{true}[0],
		Destroy:       &[]bool{true}[0],
		Components:    []string{"nginx"},
		DependsOn:     []string{"pki-resources"},
	}

	// Convert to Kubernetes kustomization
	result := handler.toFluxKustomization(kustomization, "system-gitops")

	// Verify that the patch has the correct Target with namespace
	if len(result.Spec.Patches) != 1 {
		t.Fatalf("Expected 1 patch, got %d", len(result.Spec.Patches))
	}

	patch := result.Spec.Patches[0]
	if patch.Target == nil {
		t.Fatal("Expected Target to be set")
	}

	if patch.Target.Kind != "Service" {
		t.Errorf("Expected Target Kind to be 'Service', got '%s'", patch.Target.Kind)
	}

	if patch.Target.Name != "nginx-ingress-controller" {
		t.Errorf("Expected Target Name to be 'nginx-ingress-controller', got '%s'", patch.Target.Name)
	}

	if patch.Target.Namespace != "ingress-nginx" {
		t.Errorf("Expected Target Namespace to be 'ingress-nginx', got '%s'", patch.Target.Namespace)
	}
}

func TestToKubernetesKustomizationWithActualPatch(t *testing.T) {
	// Create a mock config handler
	mockConfigHandler := &config.MockConfigHandler{}
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return "/Users/ryanvangundy/Developer/windsorcli/core/contexts/colima", nil
	}

	// Create a mock blueprint handler with the actual project root
	handler := &BaseBlueprintHandler{
		projectRoot:   "/Users/ryanvangundy/Developer/windsorcli/core",
		configHandler: mockConfigHandler,
		shims:         NewShims(),
	}

	// Mock the ReadFile function to return the expected patch content
	handler.shims.ReadFile = func(path string) ([]byte, error) {
		// Normalize path for cross-platform comparison
		normalizedPath := filepath.ToSlash(path)
		if strings.Contains(normalizedPath, "kustomize/ingress/nginx.yaml") {
			return []byte(`apiVersion: v1
kind: Service
metadata:
  name: nginx-ingress-controller
  namespace: ingress-nginx
spec:
  externalIPs:
  - 10.5.1.1
  type: LoadBalancer`), nil
		}
		return nil, fmt.Errorf("file not found: %s", path)
	}

	// Create a kustomization with a patch that references the actual file
	kustomization := blueprintv1alpha1.Kustomization{
		Name: "ingress",
		Path: "ingress",
		Patches: []blueprintv1alpha1.BlueprintPatch{
			{
				Path: "kustomize/ingress/nginx.yaml",
			},
		},
		Interval:      &metav1.Duration{Duration: time.Minute},
		RetryInterval: &metav1.Duration{Duration: 2 * time.Minute},
		Timeout:       &metav1.Duration{Duration: 5 * time.Minute},
		Wait:          &[]bool{true}[0],
		Force:         &[]bool{false}[0],
		Prune:         &[]bool{true}[0],
		Destroy:       &[]bool{true}[0],
		Components:    []string{"nginx"},
		DependsOn:     []string{"pki-resources"},
	}

	// Convert to Kubernetes kustomization
	result := handler.toFluxKustomization(kustomization, "system-gitops")

	// Verify that the patch has the correct Target with namespace
	if len(result.Spec.Patches) != 1 {
		t.Fatalf("Expected 1 patch, got %d", len(result.Spec.Patches))
	}

	patch := result.Spec.Patches[0]
	if patch.Target == nil {
		t.Fatal("Expected Target to be set")
	}

	if patch.Target.Kind != "Service" {
		t.Errorf("Expected Target Kind to be 'Service', got '%s'", patch.Target.Kind)
	}

	if patch.Target.Name != "nginx-ingress-controller" {
		t.Errorf("Expected Target Name to be 'nginx-ingress-controller', got '%s'", patch.Target.Name)
	}

	if patch.Target.Namespace != "ingress-nginx" {
		t.Errorf("Expected Target Namespace to be 'ingress-nginx', got '%s'", patch.Target.Namespace)
	}

	// Also verify the patch content
	if !strings.Contains(patch.Patch, "namespace: ingress-nginx") {
		t.Errorf("Expected patch to contain 'namespace: ingress-nginx', got: %s", patch.Patch)
	}
}

func TestToKubernetesKustomizationWithMultiplePatches(t *testing.T) {
	// Create a mock config handler
	mockConfigHandler := &config.MockConfigHandler{}
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return "/tmp/test", nil
	}

	// Create a mock blueprint handler
	handler := &BaseBlueprintHandler{
		projectRoot:   "/tmp/test",
		configHandler: mockConfigHandler,
		shims:         NewShims(),
	}

	// Mock the ReadFile function to return a patch with multiple documents
	handler.shims.ReadFile = func(name string) ([]byte, error) {
		if strings.Contains(name, "multi-patch.yaml") {
			return []byte(`apiVersion: v1
kind: Service
metadata:
  name: nginx-ingress-controller
  namespace: ingress-nginx
spec:
  externalIPs:
  - 10.5.1.1
  type: LoadBalancer
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-config
  namespace: ingress-nginx
data:
  key: value`), nil
		}
		return nil, fmt.Errorf("file not found")
	}

	// Create a kustomization with a patch that references a file with multiple documents
	kustomization := blueprintv1alpha1.Kustomization{
		Name: "ingress",
		Path: "ingress",
		Patches: []blueprintv1alpha1.BlueprintPatch{
			{
				Path: "kustomize/ingress/multi-patch.yaml",
			},
		},
		Interval:      &metav1.Duration{Duration: time.Minute},
		RetryInterval: &metav1.Duration{Duration: 2 * time.Minute},
		Timeout:       &metav1.Duration{Duration: 5 * time.Minute},
		Wait:          &[]bool{true}[0],
		Force:         &[]bool{false}[0],
		Prune:         &[]bool{true}[0],
		Destroy:       &[]bool{true}[0],
		Components:    []string{"nginx"},
		DependsOn:     []string{"pki-resources"},
	}

	// Convert to Kubernetes kustomization
	result := handler.toFluxKustomization(kustomization, "system-gitops")

	// Verify that the patch has the correct Target with namespace from the first document
	if len(result.Spec.Patches) != 1 {
		t.Fatalf("Expected 1 patch, got %d", len(result.Spec.Patches))
	}

	patch := result.Spec.Patches[0]
	if patch.Target == nil {
		t.Fatal("Expected Target to be set")
	}

	if patch.Target.Kind != "Service" {
		t.Errorf("Expected Target Kind to be 'Service', got '%s'", patch.Target.Kind)
	}

	if patch.Target.Name != "nginx-ingress-controller" {
		t.Errorf("Expected Target Name to be 'nginx-ingress-controller', got '%s'", patch.Target.Name)
	}

	if patch.Target.Namespace != "ingress-nginx" {
		t.Errorf("Expected Target Namespace to be 'ingress-nginx', got '%s'", patch.Target.Namespace)
	}

	// Verify the patch content contains both documents
	if !strings.Contains(patch.Patch, "kind: Service") {
		t.Errorf("Expected patch to contain 'kind: Service', got: %s", patch.Patch)
	}
	if !strings.Contains(patch.Patch, "kind: ConfigMap") {
		t.Errorf("Expected patch to contain 'kind: ConfigMap', got: %s", patch.Patch)
	}
	if !strings.Contains(patch.Patch, "namespace: ingress-nginx") {
		t.Errorf("Expected patch to contain 'namespace: ingress-nginx', got: %s", patch.Patch)
	}
}

func TestToKubernetesKustomizationWithEdgeCases(t *testing.T) {
	// Create a mock config handler
	mockConfigHandler := &config.MockConfigHandler{}
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return "/tmp/test", nil
	}

	// Create a mock blueprint handler
	handler := &BaseBlueprintHandler{
		projectRoot:   "/tmp/test",
		configHandler: mockConfigHandler,
		shims:         NewShims(),
	}

	// Mock the ReadFile function to return a patch with edge cases
	handler.shims.ReadFile = func(name string) ([]byte, error) {
		if strings.Contains(name, "edge-case.yaml") {
			return []byte(`# Comment at the top
---
apiVersion: v1
kind: Service
metadata:
  name: nginx-ingress-controller
  namespace: ingress-nginx
spec:
  externalIPs:
  - 10.5.1.1
  type: LoadBalancer
---
# Comment between documents
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-config
  namespace: ingress-nginx
data:
  key: value
---
# Empty document
---
# Another comment
apiVersion: v1
kind: Secret
metadata:
  name: nginx-secret
  namespace: ingress-nginx
type: Opaque`), nil
		}
		return nil, fmt.Errorf("file not found")
	}

	// Create a kustomization with a patch that references a file with edge cases
	kustomization := blueprintv1alpha1.Kustomization{
		Name: "ingress",
		Path: "ingress",
		Patches: []blueprintv1alpha1.BlueprintPatch{
			{
				Path: "kustomize/ingress/edge-case.yaml",
			},
		},
		Interval:      &metav1.Duration{Duration: time.Minute},
		RetryInterval: &metav1.Duration{Duration: 2 * time.Minute},
		Timeout:       &metav1.Duration{Duration: 5 * time.Minute},
		Wait:          &[]bool{true}[0],
		Force:         &[]bool{false}[0],
		Prune:         &[]bool{true}[0],
		Destroy:       &[]bool{true}[0],
		Components:    []string{"nginx"},
		DependsOn:     []string{"pki-resources"},
	}

	// Convert to Kubernetes kustomization
	result := handler.toFluxKustomization(kustomization, "system-gitops")

	// Verify that the patch has the correct Target with namespace from the first valid document
	if len(result.Spec.Patches) != 1 {
		t.Fatalf("Expected 1 patch, got %d", len(result.Spec.Patches))
	}

	patch := result.Spec.Patches[0]
	if patch.Target == nil {
		t.Fatal("Expected Target to be set")
	}

	if patch.Target.Kind != "Service" {
		t.Errorf("Expected Target Kind to be 'Service', got '%s'", patch.Target.Kind)
	}

	if patch.Target.Name != "nginx-ingress-controller" {
		t.Errorf("Expected Target Name to be 'nginx-ingress-controller', got '%s'", patch.Target.Name)
	}

	if patch.Target.Namespace != "ingress-nginx" {
		t.Errorf("Expected Target Namespace to be 'ingress-nginx', got '%s'", patch.Target.Namespace)
	}

	// Verify the patch content contains all documents
	if !strings.Contains(patch.Patch, "kind: Service") {
		t.Errorf("Expected patch to contain 'kind: Service', got: %s", patch.Patch)
	}
	if !strings.Contains(patch.Patch, "kind: ConfigMap") {
		t.Errorf("Expected patch to contain 'kind: ConfigMap', got: %s", patch.Patch)
	}
	if !strings.Contains(patch.Patch, "kind: Secret") {
		t.Errorf("Expected patch to contain 'kind: Secret', got: %s", patch.Patch)
	}
	if !strings.Contains(patch.Patch, "namespace: ingress-nginx") {
		t.Errorf("Expected patch to contain 'namespace: ingress-nginx', got: %s", patch.Patch)
	}
}

// =============================================================================
// Values ConfigMap Tests
// =============================================================================

func TestBaseBlueprintHandler_applyValuesConfigMaps(t *testing.T) {
	// Given a handler with mocks
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		handler.configHandler = mocks.ConfigHandler
		handler.kubernetesManager = mocks.KubernetesManager
		handler.shell = mocks.Shell
		return handler
	}

	t.Run("SuccessWithGlobalValues", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And mock config root and other config methods
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "network.loadbalancer_ips.start":
				return "192.168.1.100"
			case "network.loadbalancer_ips.end":
				return "192.168.1.200"
			case "docker.registry_url":
				return "registry.example.com"
			case "id":
				return "test-id"
			default:
				return ""
			}
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{"/host/path:/container/path"}
			}
			return []string{}
		}

		// And mock kustomize directory with global config.yaml
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize") {
				return &mockFileInfo{name: "kustomize"}, nil
			}
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return &mockFileInfo{name: "config.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And mock file read for centralized values
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return []byte(`common:
  domain: example.com
  port: 80
  enabled: true`), nil
			}
			return nil, os.ErrNotExist
		}

		// And mock YAML unmarshal
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			values := v.(*map[string]any)
			*values = map[string]any{
				"common": map[string]any{
					"domain":  "example.com",
					"port":    80,
					"enabled": true,
				},
			}
			return nil
		}

		// And mock Kubernetes manager
		var appliedConfigMaps []string
		mockKubernetesManager := handler.kubernetesManager.(*kubernetes.MockKubernetesManager)
		mockKubernetesManager.ApplyConfigMapFunc = func(name, namespace string, data map[string]string) error {
			appliedConfigMaps = append(appliedConfigMaps, name)
			return nil
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should succeed
		if err != nil {
			t.Fatalf("expected applyValuesConfigMaps to succeed, got: %v", err)
		}

		// And it should apply the common values ConfigMap
		if len(appliedConfigMaps) != 1 {
			t.Errorf("expected 1 ConfigMap to be applied, got %d", len(appliedConfigMaps))
		}
		if appliedConfigMaps[0] != "values-common" {
			t.Errorf("expected ConfigMap name to be 'values-common', got '%s'", appliedConfigMaps[0])
		}
	})

	t.Run("SuccessWithComponentValues", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And mock config root and other config methods
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "network.loadbalancer_ips.start":
				return "192.168.1.100"
			case "network.loadbalancer_ips.end":
				return "192.168.1.200"
			case "docker.registry_url":
				return "registry.example.com"
			case "id":
				return "test-id"
			default:
				return ""
			}
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{"/host/path:/container/path"}
			}
			return []string{}
		}

		// And mock centralized values.yaml with component values
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize") {
				return &mockFileInfo{name: "kustomize"}, nil
			}
			if name == filepath.Join("/test/config", "kustomize", "values.yaml") {
				return &mockFileInfo{name: "values.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And mock file read for centralized values
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join("/test/config", "kustomize", "values.yaml") {
				return []byte(`common:
  domain: example.com
ingress:
  host: ingress.example.com
  ssl: true`), nil
			}
			return nil, os.ErrNotExist
		}

		// And mock YAML unmarshal
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			values := v.(*map[string]any)
			*values = map[string]any{
				"common": map[string]any{
					"domain": "example.com",
				},
				"ingress": map[string]any{
					"host": "ingress.example.com",
					"ssl":  true,
				},
			}
			return nil
		}

		// And mock Kubernetes manager
		var appliedConfigMaps []string
		mockKubernetesManager := handler.kubernetesManager.(*kubernetes.MockKubernetesManager)
		mockKubernetesManager.ApplyConfigMapFunc = func(name, namespace string, data map[string]string) error {
			appliedConfigMaps = append(appliedConfigMaps, name)
			return nil
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should succeed
		if err != nil {
			t.Fatalf("expected applyValuesConfigMaps to succeed, got: %v", err)
		}

		// And it should apply both common and component values ConfigMaps
		if len(appliedConfigMaps) != 2 {
			t.Errorf("expected 2 ConfigMaps to be applied, got %d: %v", len(appliedConfigMaps), appliedConfigMaps)
		}

		// Check that both ConfigMaps were applied (order may vary)
		commonFound := false
		ingressFound := false
		for _, name := range appliedConfigMaps {
			if name == "values-common" {
				commonFound = true
			}
			if name == "values-ingress" {
				ingressFound = true
			}
		}
		if !commonFound {
			t.Error("expected values-common ConfigMap to be applied")
		}
		if !ingressFound {
			t.Error("expected values-ingress ConfigMap to be applied")
		}
	})

	t.Run("NoKustomizeDirectory", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock that kustomize directory doesn't exist
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should succeed (no-op)
		if err != nil {
			t.Fatalf("expected applyValuesConfigMaps to succeed when no kustomize directory, got: %v", err)
		}
	})

	t.Run("ConfigRootError", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And mock config root that fails
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", os.ErrNotExist
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should fail
		if err == nil {
			t.Fatal("expected applyValuesConfigMaps to fail with config root error")
		}
		if !strings.Contains(err.Error(), "failed to get config root") {
			t.Errorf("expected error about config root, got: %v", err)
		}
	})

	t.Run("ReadFileError", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And mock config root and other config methods
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "network.loadbalancer_ips.start":
				return "192.168.1.100"
			case "network.loadbalancer_ips.end":
				return "192.168.1.200"
			case "docker.registry_url":
				return "registry.example.com"
			case "id":
				return "test-id"
			default:
				return ""
			}
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{"/host/path:/container/path"}
			}
			return []string{}
		}

		// And mock kustomize directory and config.yaml exists
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize") {
				return &mockFileInfo{name: "kustomize"}, nil
			}
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return &mockFileInfo{name: "config.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And mock ReadFile that fails
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return nil, os.ErrPermission
			}
			return nil, os.ErrNotExist
		}

		// Mock YAML marshal
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("test"), nil
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should still succeed since ReadFile errors are now ignored and rendered values take precedence
		if err != nil {
			t.Fatalf("expected applyValuesConfigMaps to succeed despite ReadFile error, got: %v", err)
		}
	})

	t.Run("ComponentConfigMapError", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And mock config root and other config methods
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "network.loadbalancer_ips.start":
				return "192.168.1.100"
			case "network.loadbalancer_ips.end":
				return "192.168.1.200"
			case "docker.registry_url":
				return "registry.example.com"
			case "id":
				return "test-id"
			default:
				return ""
			}
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{"/host/path:/container/path"}
			}
			return []string{}
		}

		// And mock centralized config.yaml with component values
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize") {
				return &mockFileInfo{name: "kustomize"}, nil
			}
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return &mockFileInfo{name: "config.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And mock file read for centralized values
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return []byte(`common:
  domain: example.com
ingress:
  host: ingress.example.com`), nil
			}
			return nil, os.ErrNotExist
		}

		// And mock YAML unmarshal
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			values := v.(*map[string]any)
			*values = map[string]any{
				"common": map[string]any{
					"domain": "example.com",
				},
				"ingress": map[string]any{
					"host": "ingress.example.com",
				},
			}
			return nil
		}

		// And mock Kubernetes manager that fails
		mockKubernetesManager := handler.kubernetesManager.(*kubernetes.MockKubernetesManager)
		mockKubernetesManager.ApplyConfigMapFunc = func(name, namespace string, data map[string]string) error {
			return os.ErrPermission
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should fail
		if err == nil {
			t.Fatal("expected applyValuesConfigMaps to fail with ConfigMap error")
		}
		if !strings.Contains(err.Error(), "failed to create merged common values ConfigMap") {
			t.Errorf("expected error about common ConfigMap creation, got: %v", err)
		}
	})
}

// =============================================================================
// toFluxKustomization ConfigMap Tests
// =============================================================================

func TestBaseBlueprintHandler_toFluxKustomization_WithValuesConfigMaps(t *testing.T) {
	// Given a handler with mocks
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		handler.configHandler = mocks.ConfigHandler
		handler.kubernetesManager = mocks.KubernetesManager
		return handler
	}

	t.Run("WithGlobalValuesConfigMap", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock that global config.yaml exists
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return &mockFileInfo{name: "config.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And a kustomization
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have PostBuild with ConfigMap references
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set")
		}

		// And it should have the blueprint ConfigMap reference
		if len(result.Spec.PostBuild.SubstituteFrom) < 1 {
			t.Fatal("expected at least 1 SubstituteFrom reference")
		}

		commonValuesFound := false
		for _, ref := range result.Spec.PostBuild.SubstituteFrom {
			if ref.Kind == "ConfigMap" && ref.Name == "values-common" {
				commonValuesFound = true
				if ref.Optional != false {
					t.Errorf("expected values-common ConfigMap to be Optional=false, got %v", ref.Optional)
				}
			}
		}

		if !commonValuesFound {
			t.Error("expected values-common ConfigMap reference to be present")
		}
	})

	t.Run("WithComponentValuesConfigMap", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock that global values.yaml exists
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize", "values.yaml") {
				return &mockFileInfo{name: "values.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And mock the values.yaml content with ingress component
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join("/test/config", "kustomize", "values.yaml") {
				return []byte(`ingress:
  key: value`), nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.YamlUnmarshal = func(data []byte, v interface{}) error {
			values := map[string]any{
				"ingress": map[string]any{
					"key": "value",
				},
			}
			reflect.ValueOf(v).Elem().Set(reflect.ValueOf(values))
			return nil
		}

		// And a kustomization with component name
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "ingress",
			Path:          "ingress/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have PostBuild with ConfigMap references
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set")
		}

		// And it should have the component-specific ConfigMap reference
		componentValuesFound := false
		for _, ref := range result.Spec.PostBuild.SubstituteFrom {
			if ref.Kind == "ConfigMap" && ref.Name == "values-ingress" {
				componentValuesFound = true
				if ref.Optional != false {
					t.Errorf("expected values-ingress ConfigMap to be Optional=false, got %v", ref.Optional)
				}
				break
			}
		}

		if !componentValuesFound {
			t.Error("expected values-ingress ConfigMap reference to be present")
		}
	})

	t.Run("WithExistingPostBuild", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock that global values.yaml exists
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize", "values.yaml") {
				return &mockFileInfo{name: "values.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And a kustomization with existing PostBuild
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			PostBuild: &blueprintv1alpha1.PostBuild{
				Substitute: map[string]string{
					"VAR1": "value1",
					"VAR2": "value2",
				},
				SubstituteFrom: []blueprintv1alpha1.SubstituteReference{
					{
						Kind:     "ConfigMap",
						Name:     "existing-config",
						Optional: true,
					},
				},
			},
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have PostBuild with both existing and new references
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set")
		}

		// And it should preserve existing Substitute values
		if len(result.Spec.PostBuild.Substitute) != 2 {
			t.Errorf("expected 2 Substitute values, got %d", len(result.Spec.PostBuild.Substitute))
		}
		if result.Spec.PostBuild.Substitute["VAR1"] != "value1" {
			t.Errorf("expected VAR1 to be 'value1', got '%s'", result.Spec.PostBuild.Substitute["VAR1"])
		}
		if result.Spec.PostBuild.Substitute["VAR2"] != "value2" {
			t.Errorf("expected VAR2 to be 'value2', got '%s'", result.Spec.PostBuild.Substitute["VAR2"])
		}

		// And it should have the correct SubstituteFrom references
		commonValuesFound := false
		existingConfigFound := false

		for _, ref := range result.Spec.PostBuild.SubstituteFrom {
			if ref.Kind == "ConfigMap" && ref.Name == "values-common" {
				commonValuesFound = true
			}
			if ref.Kind == "ConfigMap" && ref.Name == "existing-config" {
				existingConfigFound = true
				if ref.Optional != true {
					t.Errorf("expected existing-config to be Optional=true, got %v", ref.Optional)
				}
			}
		}

		if !commonValuesFound {
			t.Error("expected values-common ConfigMap reference to be present")
		}
		if !existingConfigFound {
			t.Error("expected existing-config ConfigMap reference to be preserved")
		}
	})

	t.Run("WithoutValuesConfigMaps", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock that no config.yaml files exist
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And a kustomization
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have PostBuild with only common ConfigMap reference
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set")
		}

		// And it should only have the common ConfigMap reference
		if len(result.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Errorf("expected 1 SubstituteFrom reference, got %d", len(result.Spec.PostBuild.SubstituteFrom))
		}

		ref := result.Spec.PostBuild.SubstituteFrom[0]
		if ref.Kind != "ConfigMap" {
			t.Errorf("expected Kind to be 'ConfigMap', got '%s'", ref.Kind)
		}
		if ref.Name != "values-common" {
			t.Errorf("expected Name to be 'values-common', got '%s'", ref.Name)
		}
		if ref.Optional != false {
			t.Errorf("expected Optional to be false, got %v", ref.Optional)
		}
	})

	t.Run("ConfigRootError", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root that fails
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", os.ErrNotExist
		}

		// And a kustomization
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should still have PostBuild with only blueprint ConfigMap reference
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set")
		}

		// And it should only have the blueprint ConfigMap reference (no values ConfigMaps due to error)
		if len(result.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Errorf("expected 1 SubstituteFrom reference, got %d", len(result.Spec.PostBuild.SubstituteFrom))
		}

		ref := result.Spec.PostBuild.SubstituteFrom[0]
		if ref.Kind != "ConfigMap" {
			t.Errorf("expected Kind to be 'ConfigMap', got '%s'", ref.Kind)
		}
		if ref.Name != "values-common" {
			t.Errorf("expected Name to be 'values-common', got '%s'", ref.Name)
		}
	})
}

func TestBaseBlueprintHandler_toFluxKustomization_Comprehensive(t *testing.T) {
	// Given a handler with mocks
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		handler.configHandler = mocks.ConfigHandler
		handler.kubernetesManager = mocks.KubernetesManager
		return handler
	}

	t.Run("BasicKustomizationConversion", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And a basic kustomization
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have correct basic fields
		if result.Name != "test-kustomization" {
			t.Errorf("expected Name to be 'test-kustomization', got '%s'", result.Name)
		}
		if result.Namespace != "test-namespace" {
			t.Errorf("expected Namespace to be 'test-namespace', got '%s'", result.Namespace)
		}
		if result.Spec.Path != "test/path" {
			t.Errorf("expected Path to be 'test/path', got '%s'", result.Spec.Path)
		}
		if result.Spec.SourceRef.Name != "test-source" {
			t.Errorf("expected SourceRef.Name to be 'test-source', got '%s'", result.Spec.SourceRef.Name)
		}
		if result.Spec.SourceRef.Kind != "GitRepository" {
			t.Errorf("expected SourceRef.Kind to be 'GitRepository', got '%s'", result.Spec.SourceRef.Kind)
		}
		if result.Spec.Interval.Duration != 5*time.Minute {
			t.Errorf("expected Interval to be 5 minutes, got %v", result.Spec.Interval.Duration)
		}
		if result.Spec.RetryInterval.Duration != 1*time.Minute {
			t.Errorf("expected RetryInterval to be 1 minute, got %v", result.Spec.RetryInterval.Duration)
		}
		if result.Spec.Timeout.Duration != 10*time.Minute {
			t.Errorf("expected Timeout to be 10 minutes, got %v", result.Spec.Timeout.Duration)
		}
		if result.Spec.Force != false {
			t.Errorf("expected Force to be false, got %v", result.Spec.Force)
		}
		if result.Spec.Wait != false {
			t.Errorf("expected Wait to be false, got %v", result.Spec.Wait)
		}
		if result.Spec.Prune != true {
			t.Errorf("expected Prune to be true, got %v", result.Spec.Prune)
		}
		if result.Spec.DeletionPolicy != "WaitForTermination" {
			t.Errorf("expected DeletionPolicy to be 'WaitForTermination', got '%s'", result.Spec.DeletionPolicy)
		}
	})

	t.Run("WithDependsOn", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And a kustomization with dependencies
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			DependsOn:     []string{"dependency1", "dependency2"},
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have correct dependencies
		if len(result.Spec.DependsOn) != 2 {
			t.Errorf("expected 2 dependencies, got %d", len(result.Spec.DependsOn))
		}

		expectedDeps := map[string]bool{
			"dependency1": false,
			"dependency2": false,
		}

		for _, dep := range result.Spec.DependsOn {
			if dep.Namespace != "test-namespace" {
				t.Errorf("expected dependency namespace to be 'test-namespace', got '%s'", dep.Namespace)
			}
			expectedDeps[dep.Name] = true
		}

		for depName, found := range expectedDeps {
			if !found {
				t.Errorf("expected dependency '%s' not found", depName)
			}
		}
	})

	t.Run("WithOCISource", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint with OCI repository
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "oci://registry.example.com/test/repo",
			},
		}

		// And a kustomization with OCI source
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "oci://registry.example.com/test/source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have OCI source type
		if result.Spec.SourceRef.Kind != "OCIRepository" {
			t.Errorf("expected SourceRef.Kind to be 'OCIRepository', got '%s'", result.Spec.SourceRef.Kind)
		}
		if result.Spec.SourceRef.Name != "oci://registry.example.com/test/source" {
			t.Errorf("expected SourceRef.Name to be 'oci://registry.example.com/test/source', got '%s'", result.Spec.SourceRef.Name)
		}
	})

	t.Run("WithDestroyPolicy", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And a kustomization with destroy policy
		destroy := true
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			Destroy:       &destroy,
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have WaitForTermination deletion policy
		if result.Spec.DeletionPolicy != "WaitForTermination" {
			t.Errorf("expected DeletionPolicy to be 'WaitForTermination', got '%s'", result.Spec.DeletionPolicy)
		}
	})

	t.Run("WithPatchFromFile", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock patch file content
		patchContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: test-namespace
data:
  key: value`

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join("/test/config", "kustomize", "patch.yaml") {
				return []byte(patchContent), nil
			}
			return nil, os.ErrNotExist
		}

		// And a kustomization with patch from file
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Path: "patch.yaml",
				},
			},
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have the patch
		if len(result.Spec.Patches) != 1 {
			t.Errorf("expected 1 patch, got %d", len(result.Spec.Patches))
		}

		patch := result.Spec.Patches[0]
		if patch.Patch != patchContent {
			t.Errorf("expected patch content to match, got '%s'", patch.Patch)
		}

		if patch.Target == nil {
			t.Error("expected patch target to be set")
		} else {
			if patch.Target.Kind != "ConfigMap" {
				t.Errorf("expected target kind to be 'ConfigMap', got '%s'", patch.Target.Kind)
			}
			if patch.Target.Name != "test-config" {
				t.Errorf("expected target name to be 'test-config', got '%s'", patch.Target.Name)
			}
			if patch.Target.Namespace != "test-namespace" {
				t.Errorf("expected target namespace to be 'test-namespace', got '%s'", patch.Target.Namespace)
			}
		}
	})

	t.Run("WithInlinePatch", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And a kustomization with inline patch
		inlinePatch := `apiVersion: v1
kind: ConfigMap
metadata:
  name: inline-config
data:
  inline: value`

		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: inlinePatch,
				},
			},
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have the inline patch
		if len(result.Spec.Patches) != 1 {
			t.Errorf("expected 1 patch, got %d", len(result.Spec.Patches))
		}

		patch := result.Spec.Patches[0]
		if patch.Patch != inlinePatch {
			t.Errorf("expected patch content to match, got '%s'", patch.Patch)
		}

		if patch.Target != nil {
			t.Error("expected patch target to be nil for inline patch")
		}
	})

	t.Run("WithPatchTarget", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And a kustomization with patch target
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: "patch content",
					Target: &kustomize.Selector{
						Kind:      "Deployment",
						Name:      "test-deployment",
						Namespace: "custom-namespace",
					},
				},
			},
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have the patch with target
		if len(result.Spec.Patches) != 1 {
			t.Errorf("expected 1 patch, got %d", len(result.Spec.Patches))
		}

		patch := result.Spec.Patches[0]
		if patch.Patch != "patch content" {
			t.Errorf("expected patch content to match, got '%s'", patch.Patch)
		}

		if patch.Target == nil {
			t.Error("expected patch target to be set")
		} else {
			if patch.Target.Kind != "Deployment" {
				t.Errorf("expected target kind to be 'Deployment', got '%s'", patch.Target.Kind)
			}
			if patch.Target.Name != "test-deployment" {
				t.Errorf("expected target name to be 'test-deployment', got '%s'", patch.Target.Name)
			}
			if patch.Target.Namespace != "custom-namespace" {
				t.Errorf("expected target namespace to be 'custom-namespace', got '%s'", patch.Target.Namespace)
			}
		}
	})

	t.Run("WithMultiplePatches", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And a kustomization with multiple patches
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: "patch1",
				},
				{
					Patch: "patch2",
					Target: &kustomize.Selector{
						Kind: "Service",
						Name: "test-service",
					},
				},
			},
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have both patches
		if len(result.Spec.Patches) != 2 {
			t.Errorf("expected 2 patches, got %d", len(result.Spec.Patches))
		}

		if result.Spec.Patches[0].Patch != "patch1" {
			t.Errorf("expected first patch content to be 'patch1', got '%s'", result.Spec.Patches[0].Patch)
		}

		if result.Spec.Patches[1].Patch != "patch2" {
			t.Errorf("expected second patch content to be 'patch2', got '%s'", result.Spec.Patches[1].Patch)
		}

		if result.Spec.Patches[1].Target == nil {
			t.Error("expected second patch target to be set")
		} else {
			if result.Spec.Patches[1].Target.Kind != "Service" {
				t.Errorf("expected second patch target kind to be 'Service', got '%s'", result.Spec.Patches[1].Target.Kind)
			}
			if result.Spec.Patches[1].Target.Name != "test-service" {
				t.Errorf("expected second patch target name to be 'test-service', got '%s'", result.Spec.Patches[1].Target.Name)
			}
		}
	})

	t.Run("WithComponents", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And a kustomization with components
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			Components:    []string{"component1", "component2"},
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have the components
		if len(result.Spec.Components) != 2 {
			t.Errorf("expected 2 components, got %d", len(result.Spec.Components))
		}

		expectedComponents := map[string]bool{
			"component1": false,
			"component2": false,
		}

		for _, component := range result.Spec.Components {
			expectedComponents[component] = true
		}

		for componentName, found := range expectedComponents {
			if !found {
				t.Errorf("expected component '%s' not found", componentName)
			}
		}
	})

	t.Run("WithCustomPrune", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And a kustomization with custom prune setting
		prune := false
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			Prune:         &prune,
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have the custom prune setting
		if result.Spec.Prune != false {
			t.Errorf("expected Prune to be false, got %v", result.Spec.Prune)
		}
	})
}

func TestBaseBlueprintHandler_applyConfigMap_WithBuildID(t *testing.T) {
	mocks := setupMocks(t, &SetupOptions{
		ConfigStr: `
contexts:
  test:
    id: "test-id"
    dns:
      domain: "test.com"
    network:
      loadbalancer_ips:
        start: "10.0.0.1"
        end: "10.0.0.10"
    docker:
      registry_url: "registry.test"
    cluster:
      workers:
        volumes: ["/tmp:/data"]
`,
	})

	handler := NewBlueprintHandler(mocks.Injector)
	if err := handler.Initialize(); err != nil {
		t.Fatalf("failed to initialize handler: %v", err)
	}

	// Set up build ID by mocking the file system
	testBuildID := "build-1234567890"
	projectRoot, err := mocks.Shell.GetProjectRoot()
	if err != nil {
		t.Fatalf("failed to get project root: %v", err)
	}
	buildIDPath := filepath.Join(projectRoot, ".windsor", ".build-id")

	// Mock the file system to return our test build ID
	handler.shims.Stat = func(path string) (os.FileInfo, error) {
		if path == buildIDPath {
			return mockFileInfo{name: ".build-id", isDir: false}, nil
		}
		return nil, os.ErrNotExist
	}
	handler.shims.ReadFile = func(path string) ([]byte, error) {
		if path == buildIDPath {
			return []byte(testBuildID), nil
		}
		return []byte{}, nil
	}

	// Mock the kubernetes manager to capture the ConfigMap data
	var capturedData map[string]string
	mocks.KubernetesManager.ApplyConfigMapFunc = func(name, namespace string, data map[string]string) error {
		capturedData = data
		return nil
	}

	// Call applyValuesConfigMaps
	if err := handler.applyValuesConfigMaps(); err != nil {
		t.Fatalf("failed to apply ConfigMap: %v", err)
	}

	// Verify BUILD_ID is included in the ConfigMap data
	if capturedData == nil {
		t.Fatal("ConfigMap data was not captured")
	}

	buildID, exists := capturedData["BUILD_ID"]
	if !exists {
		t.Fatal("BUILD_ID not found in ConfigMap data")
	}

	if buildID != testBuildID {
		t.Errorf("expected BUILD_ID to be %s, got %s", testBuildID, buildID)
	}

	// Verify other expected fields are present
	expectedFields := []string{"DOMAIN", "CONTEXT", "CONTEXT_ID", "LOADBALANCER_IP_RANGE", "REGISTRY_URL"}
	for _, field := range expectedFields {
		if _, exists := capturedData[field]; !exists {
			t.Errorf("expected field %s not found in ConfigMap data", field)
		}
	}
}

func TestBaseBlueprintHandler_applyConfigMap_WithoutBuildID(t *testing.T) {
	mocks := setupMocks(t, &SetupOptions{
		ConfigStr: `
contexts:
  test:
    id: "test-id"
    dns:
      domain: "test.com"
    network:
      loadbalancer_ips:
        start: "10.0.0.1"
        end: "10.0.0.10"
    docker:
      registry_url: "registry.test"
    cluster:
      workers:
        volumes: ["/tmp:/data"]
`,
	})

	handler := NewBlueprintHandler(mocks.Injector)
	if err := handler.Initialize(); err != nil {
		t.Fatalf("failed to initialize handler: %v", err)
	}

	// Mock the file system to simulate missing .build-id file
	projectRoot, err := mocks.Shell.GetProjectRoot()
	if err != nil {
		t.Fatalf("failed to get project root: %v", err)
	}
	buildIDPath := filepath.Join(projectRoot, ".windsor", ".build-id")

	// Mock the file system to return file not found for .build-id
	handler.shims.Stat = func(path string) (os.FileInfo, error) {
		if path == buildIDPath {
			return nil, os.ErrNotExist
		}
		return nil, os.ErrNotExist
	}
	handler.shims.ReadFile = func(path string) ([]byte, error) {
		if path == buildIDPath {
			return nil, os.ErrNotExist
		}
		return []byte{}, nil
	}

	// Mock the kubernetes manager to capture the ConfigMap data
	var capturedData map[string]string
	mocks.KubernetesManager.ApplyConfigMapFunc = func(name, namespace string, data map[string]string) error {
		capturedData = data
		return nil
	}

	// Call applyValuesConfigMaps - this should not cause an error
	if err := handler.applyValuesConfigMaps(); err != nil {
		t.Fatalf("failed to apply ConfigMap: %v", err)
	}

	// Verify BUILD_ID is not included in the ConfigMap data when file doesn't exist
	if capturedData == nil {
		t.Fatal("ConfigMap data was not captured")
	}

	buildID, exists := capturedData["BUILD_ID"]
	if exists {
		t.Errorf("expected BUILD_ID to not be present in ConfigMap data when file doesn't exist, but it was found with value '%s'", buildID)
	}

	// Verify other expected fields are present
	expectedFields := []string{"DOMAIN", "CONTEXT", "CONTEXT_ID", "LOADBALANCER_IP_RANGE", "REGISTRY_URL"}
	for _, field := range expectedFields {
		if _, exists := capturedData[field]; !exists {
			t.Errorf("expected field %s not found in ConfigMap data", field)
		}
	}
}

// =============================================================================
// New Functionality Tests
// =============================================================================

func TestBaseBlueprintHandler_resolvePatchFromPath(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewBlueprintHandler(injector)
		handler.shims = NewShims()
		handler.configHandler = config.NewMockConfigHandler()
		return handler
	}

	t.Run("WithRenderedDataOnly", func(t *testing.T) {
		// Given a handler with rendered patch data only
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "test-config",
					"namespace": "test-namespace",
				},
				"data": map[string]any{
					"key": "value",
				},
			},
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}
		// When resolving patch from path
		content, target := handler.resolvePatchFromPath("test", "default-namespace")
		// Then content should be returned and target should be extracted
		if content != "test yaml" {
			t.Errorf("Expected content = 'test yaml', got = '%s'", content)
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Kind != "ConfigMap" {
			t.Errorf("Expected target kind = 'ConfigMap', got = '%s'", target.Kind)
		}
		if target.Name != "test-config" {
			t.Errorf("Expected target name = 'test-config', got = '%s'", target.Name)
		}
		if target.Namespace != "test-namespace" {
			t.Errorf("Expected target namespace = 'test-namespace', got = '%s'", target.Namespace)
		}
	})

	t.Run("WithNoData", func(t *testing.T) {
		// Given a handler with no data
		handler := setup(t)
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}
		// When resolving patch from path
		content, target := handler.resolvePatchFromPath("test", "default-namespace")
		// Then empty content and nil target should be returned
		if content != "" {
			t.Errorf("Expected empty content, got = '%s'", content)
		}
		if target != nil {
			t.Error("Expected target to be nil")
		}
	})

	t.Run("WithYamlExtension", func(t *testing.T) {
		// Given a handler with patch path containing .yaml extension
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}
		// When resolving patch from path with .yaml extension
		content, target := handler.resolvePatchFromPath("test.yaml", "default-namespace")
		// Then content should be returned and target should be extracted
		if content != "test yaml" {
			t.Errorf("Expected content = 'test yaml', got = '%s'", content)
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
	})

	t.Run("WithYmlExtension", func(t *testing.T) {
		// Given a handler with patch path containing .yml extension
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}
		// When resolving patch from path with .yml extension
		content, target := handler.resolvePatchFromPath("test.yml", "default-namespace")
		// Then content should be returned and target should be extracted
		if content != "test yaml" {
			t.Errorf("Expected content = 'test yaml', got = '%s'", content)
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
	})

	t.Run("WithBothRenderedAndUserDataMerge", func(t *testing.T) {
		// Given a handler with both rendered and user data that can be merged
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "rendered-config",
					"namespace": "rendered-namespace",
				},
				"data": map[string]any{
					"rendered-key": "rendered-value",
				},
			},
		}
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: user-config
  namespace: user-namespace
data:
  user-key: user-value`), nil
		}
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			values := v.(*map[string]any)
			*values = map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "user-config",
					"namespace": "user-namespace",
				},
				"data": map[string]any{
					"user-key": "user-value",
				},
			}
			return nil
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("merged yaml"), nil
		}
		// When resolving patch from path
		content, target := handler.resolvePatchFromPath("test", "default-namespace")
		// Then merged content should be returned and target should be extracted from merged data
		if content != "merged yaml" {
			t.Errorf("Expected content = 'merged yaml', got = '%s'", content)
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Name != "user-config" {
			t.Errorf("Expected target name = 'user-config', got = '%s'", target.Name)
		}
		if target.Namespace != "user-namespace" {
			t.Errorf("Expected target namespace = 'user-namespace', got = '%s'", target.Namespace)
		}
	})
}

func TestBaseBlueprintHandler_extractTargetFromPatchData(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewBlueprintHandler(injector)
		return handler
	}

	t.Run("ValidPatchData", func(t *testing.T) {
		// Given valid patch data with all required fields
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "test-config",
				"namespace": "test-namespace",
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be extracted correctly
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Kind != "ConfigMap" {
			t.Errorf("Expected target kind = 'ConfigMap', got = '%s'", target.Kind)
		}
		if target.Name != "test-config" {
			t.Errorf("Expected target name = 'test-config', got = '%s'", target.Name)
		}
		if target.Namespace != "test-namespace" {
			t.Errorf("Expected target namespace = 'test-namespace', got = '%s'", target.Namespace)
		}
	})

	t.Run("WithCustomNamespace", func(t *testing.T) {
		// Given patch data with custom namespace
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "test-config",
				"namespace": "custom-namespace",
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then custom namespace should be used
		if target.Namespace != "custom-namespace" {
			t.Errorf("Expected target namespace = 'custom-namespace', got = '%s'", target.Namespace)
		}
	})

	t.Run("MissingKind", func(t *testing.T) {
		// Given patch data missing kind field
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when kind is missing")
		}
	})

	t.Run("MissingMetadata", func(t *testing.T) {
		// Given patch data missing metadata field
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when metadata is missing")
		}
	})

	t.Run("MissingName", func(t *testing.T) {
		// Given patch data missing name field
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when name is missing")
		}
	})

	t.Run("InvalidKindType", func(t *testing.T) {
		// Given patch data with invalid kind type
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       42,
			"metadata": map[string]any{
				"name": "test-config",
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when kind type is invalid")
		}
	})

	t.Run("InvalidMetadataType", func(t *testing.T) {
		// Given patch data with invalid metadata type
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   "not a map",
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when metadata type is invalid")
		}
	})

	t.Run("InvalidNameType", func(t *testing.T) {
		// Given patch data with invalid name type
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": 42,
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when name type is invalid")
		}
	})
}

func TestBaseBlueprintHandler_extractTargetFromPatchContent(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewBlueprintHandler(injector)
		return handler
	}

	t.Run("ValidYamlContent", func(t *testing.T) {
		// Given valid YAML content
		handler := setup(t)
		content := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: test-namespace`
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then target should be extracted correctly
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Name != "test-config" {
			t.Errorf("Expected target name = 'test-config', got = '%s'", target.Name)
		}
	})

	t.Run("MultipleDocuments", func(t *testing.T) {
		// Given YAML with multiple documents
		handler := setup(t)
		content := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: first-config
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: second-config`
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then first valid target should be extracted
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Name != "first-config" {
			t.Errorf("Expected target name = 'first-config', got = '%s'", target.Name)
		}
	})

	t.Run("InvalidYamlContent", func(t *testing.T) {
		// Given invalid YAML content
		handler := setup(t)
		content := `invalid: yaml: content: with: colons: everywhere`
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil for invalid YAML")
		}
	})

	t.Run("EmptyContent", func(t *testing.T) {
		// Given empty content
		handler := setup(t)
		content := ""
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil for empty content")
		}
	})

	t.Run("NoValidTargets", func(t *testing.T) {
		// Given YAML with no valid targets
		handler := setup(t)
		content := `apiVersion: v1
kind: ConfigMap
# Missing metadata.name`
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when no valid targets")
		}
	})
}

func TestBaseBlueprintHandler_hasComponentValues(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewBlueprintHandler(injector)
		handler.shims = NewShims()
		handler.configHandler = config.NewMockConfigHandler()
		return handler
	}

	t.Run("TemplateComponentExists", func(t *testing.T) {
		// Given handler with component in template data
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/values": map[string]any{
				"test-component": map[string]any{
					"key": "value",
				},
			},
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return true
		if !exists {
			t.Error("Expected component to exist in template data")
		}
	})

	t.Run("UserComponentExists", func(t *testing.T) {
		// Given handler with component in user file
		handler := setup(t)
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: "config.yaml", isDir: false}, nil
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`test-component:
  key: value`), nil
		}
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			values := v.(*map[string]any)
			*values = map[string]any{
				"test-component": map[string]any{
					"key": "value",
				},
			}
			return nil
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return true
		if !exists {
			t.Error("Expected component to exist in user file")
		}
	})

	t.Run("BothTemplateAndUserExist", func(t *testing.T) {
		// Given handler with component in both template and user data
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/values": map[string]any{
				"test-component": map[string]any{
					"template-key": "template-value",
				},
			},
		}
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: "config.yaml", isDir: false}, nil
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`test-component:
  user-key: user-value`), nil
		}
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			values := v.(*map[string]any)
			*values = map[string]any{
				"test-component": map[string]any{
					"user-key": "user-value",
				},
			}
			return nil
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return true
		if !exists {
			t.Error("Expected component to exist in both sources")
		}
	})

	t.Run("NoComponentExists", func(t *testing.T) {
		// Given handler with no component data
		handler := setup(t)
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return false
		if exists {
			t.Error("Expected component to not exist")
		}
	})

	t.Run("ConfigRootError", func(t *testing.T) {
		// Given handler with config root error
		handler := setup(t)
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return false
		if exists {
			t.Error("Expected component to not exist when config root fails")
		}
	})

	t.Run("FileNotExists", func(t *testing.T) {
		// Given handler with file not existing
		handler := setup(t)
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return false
		if exists {
			t.Error("Expected component to not exist when file doesn't exist")
		}
	})

	t.Run("InvalidValuesFile", func(t *testing.T) {
		// Given handler with invalid values file
		handler := setup(t)
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: "config.yaml", isDir: false}, nil
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("invalid yaml"), nil
		}
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("invalid yaml")
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return false
		if exists {
			t.Error("Expected component to not exist when values file is invalid")
		}
	})
}

func TestBaseBlueprintHandler_deepMergeMaps(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewBlueprintHandler(injector)
		return handler
	}

	t.Run("SimpleMerge", func(t *testing.T) {
		// Given base and overlay maps with simple values
		handler := setup(t)
		base := map[string]any{
			"key1": "base-value1",
			"key2": "base-value2",
		}
		overlay := map[string]any{
			"key2": "overlay-value2",
			"key3": "overlay-value3",
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then result should contain merged values
		if result["key1"] != "base-value1" {
			t.Errorf("Expected key1 = 'base-value1', got = '%v'", result["key1"])
		}
		if result["key2"] != "overlay-value2" {
			t.Errorf("Expected key2 = 'overlay-value2', got = '%v'", result["key2"])
		}
		if result["key3"] != "overlay-value3" {
			t.Errorf("Expected key3 = 'overlay-value3', got = '%v'", result["key3"])
		}
	})

	t.Run("NestedMapMerge", func(t *testing.T) {
		// Given base and overlay maps with nested maps
		handler := setup(t)
		base := map[string]any{
			"nested": map[string]any{
				"base-key": "base-value",
			},
		}
		overlay := map[string]any{
			"nested": map[string]any{
				"overlay-key": "overlay-value",
			},
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then nested maps should be merged
		nested := result["nested"].(map[string]any)
		if nested["base-key"] != "base-value" {
			t.Errorf("Expected nested.base-key = 'base-value', got = '%v'", nested["base-key"])
		}
		if nested["overlay-key"] != "overlay-value" {
			t.Errorf("Expected nested.overlay-key = 'overlay-value', got = '%v'", nested["overlay-key"])
		}
	})

	t.Run("OverlayPrecedence", func(t *testing.T) {
		// Given base and overlay maps with conflicting keys
		handler := setup(t)
		base := map[string]any{
			"key": "base-value",
		}
		overlay := map[string]any{
			"key": "overlay-value",
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then overlay value should take precedence
		if result["key"] != "overlay-value" {
			t.Errorf("Expected key = 'overlay-value', got = '%v'", result["key"])
		}
	})

	t.Run("DeepNestedMerge", func(t *testing.T) {
		// Given base and overlay maps with deeply nested maps
		handler := setup(t)
		base := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"base-key": "base-value",
				},
			},
		}
		overlay := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"overlay-key": "overlay-value",
				},
			},
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then deeply nested maps should be merged
		level1 := result["level1"].(map[string]any)
		level2 := level1["level2"].(map[string]any)
		if level2["base-key"] != "base-value" {
			t.Errorf("Expected level2.base-key = 'base-value', got = '%v'", level2["base-key"])
		}
		if level2["overlay-key"] != "overlay-value" {
			t.Errorf("Expected level2.overlay-key = 'overlay-value', got = '%v'", level2["overlay-key"])
		}
	})

	t.Run("EmptyMaps", func(t *testing.T) {
		// Given empty base and overlay maps
		handler := setup(t)
		base := map[string]any{}
		overlay := map[string]any{}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then result should be empty
		if len(result) != 0 {
			t.Errorf("Expected empty result, got %d items", len(result))
		}
	})

	t.Run("NonMapOverlay", func(t *testing.T) {
		// Given base map and non-map overlay value
		handler := setup(t)
		base := map[string]any{
			"key": map[string]any{
				"nested": "value",
			},
		}
		overlay := map[string]any{
			"key": "string-value",
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then overlay value should replace base value
		if result["key"] != "string-value" {
			t.Errorf("Expected key = 'string-value', got = '%v'", result["key"])
		}
	})

	t.Run("MixedTypes", func(t *testing.T) {
		// Given base and overlay maps with mixed types
		handler := setup(t)
		base := map[string]any{
			"string": "base-string",
			"number": 42,
			"nested": map[string]any{
				"key": "base-nested",
			},
		}
		overlay := map[string]any{
			"string": "overlay-string",
			"bool":   true,
			"nested": map[string]any{
				"overlay-key": "overlay-nested",
			},
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then all values should be merged correctly
		if result["string"] != "overlay-string" {
			t.Errorf("Expected string = 'overlay-string', got = '%v'", result["string"])
		}
		if result["number"] != 42 {
			t.Errorf("Expected number = 42, got = '%v'", result["number"])
		}
		if result["bool"] != true {
			t.Errorf("Expected bool = true, got = '%v'", result["bool"])
		}
		nested := result["nested"].(map[string]any)
		if nested["key"] != "base-nested" {
			t.Errorf("Expected nested.key = 'base-nested', got = '%v'", nested["key"])
		}
		if nested["overlay-key"] != "overlay-nested" {
			t.Errorf("Expected nested.overlay-key = 'overlay-nested', got = '%v'", nested["overlay-key"])
		}
	})
}

func TestBaseBlueprintHandler_SetRenderedKustomizeData(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewBlueprintHandler(injector)
		return handler
	}

	t.Run("SetData", func(t *testing.T) {
		// Given a handler with no existing data
		handler := setup(t)
		data := map[string]any{
			"key1": "value1",
			"key2": 42,
		}
		// When setting rendered kustomize data
		handler.SetRenderedKustomizeData(data)
		// Then data should be stored
		if !reflect.DeepEqual(handler.kustomizeData, data) {
			t.Errorf("Expected kustomizeData = %v, got = %v", data, handler.kustomizeData)
		}
	})

	t.Run("OverwriteData", func(t *testing.T) {
		// Given a handler with existing data
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"existing": "data",
		}
		newData := map[string]any{
			"new": "data",
		}
		// When setting new rendered kustomize data
		handler.SetRenderedKustomizeData(newData)
		// Then new data should overwrite existing data
		if !reflect.DeepEqual(handler.kustomizeData, newData) {
			t.Errorf("Expected kustomizeData = %v, got = %v", newData, handler.kustomizeData)
		}
	})

	t.Run("EmptyData", func(t *testing.T) {
		// Given a handler with existing data
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"existing": "data",
		}
		emptyData := map[string]any{}
		// When setting empty rendered kustomize data
		handler.SetRenderedKustomizeData(emptyData)
		// Then empty data should be stored
		if !reflect.DeepEqual(handler.kustomizeData, emptyData) {
			t.Errorf("Expected kustomizeData = %v, got = %v", emptyData, handler.kustomizeData)
		}
	})

	t.Run("ComplexData", func(t *testing.T) {
		// Given a handler with no existing data
		handler := setup(t)
		complexData := map[string]any{
			"nested": map[string]any{
				"level1": map[string]any{
					"level2": []any{
						"string1",
						123,
						map[string]any{"key": "value"},
					},
				},
			},
			"array": []any{
				"item1",
				456,
				map[string]any{"nested": "data"},
			},
		}
		// When setting complex rendered kustomize data
		handler.SetRenderedKustomizeData(complexData)
		// Then complex data should be stored
		if !reflect.DeepEqual(handler.kustomizeData, complexData) {
			t.Errorf("Expected kustomizeData = %v, got = %v", complexData, handler.kustomizeData)
		}
	})
}
