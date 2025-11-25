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
	"github.com/fluxcd/pkg/apis/kustomize"
	"github.com/goccy/go-yaml"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// mockSchemaContent provides a basic schema.yaml content for tests
func mockSchemaContent() string {
	return `$schema: https://schemas.windsorcli.dev/blueprint-config/v1alpha1
title: Test Configuration
description: Test configuration for Windsor blueprints
type: object
properties:
  external_domain:
    type: string
    default: "template.test"
  registry_url:
    type: string
    default: "registry.template.test"
  provider:
    type: string
    default: "local"
    enum: ["local", "aws", "azure"]
  template_only:
    type: string
    default: "template_value"
  template_key:
    type: string
    default: "template_value"
  nested:
    type: object
    properties:
      template_key:
        type: string
        default: "template_value"
    additionalProperties: true
  storage:
    type: object
    properties:
      driver:
        type: string
        default: "auto"
    additionalProperties: true
  substitutions:
    type: object
    properties:
      common:
        type: object
        properties:
          external_domain:
            type: string
            default: "template.test"
          registry_url:
            type: string
            default: "registry.template.test"
        additionalProperties: true
      template_sub:
        type: string
        default: "template_sub_value"
    additionalProperties: true
required: []
additionalProperties: true`
}

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

type BlueprintTestMocks struct {
	Shell             *shell.MockShell
	ConfigHandler     config.ConfigHandler
	Shims             *Shims
	KubernetesManager *kubernetes.MockKubernetesManager
	Runtime           *runtime.Runtime
}

func setupDefaultShims() *Shims {
	shims := NewShims()

	// Override only the functions needed for testing
	shims.ReadFile = func(name string) ([]byte, error) {
		switch {
		case strings.HasSuffix(name, "blueprint.jsonnet"):
			return []byte(safeBlueprintJsonnet), nil
		case strings.HasSuffix(name, "blueprint.yaml"):
			return []byte(safeBlueprintYAML), nil
		case strings.Contains(name, "_template/schema.yaml"):
			return []byte(mockSchemaContent()), nil
		case strings.Contains(name, "contexts") && strings.Contains(name, "values.yaml"):
			// Default context values for tests
			return []byte(`substitutions:
  common:
    external_domain: test.local`), nil
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
		if strings.Contains(name, "_template/schema.yaml") {
			return &mockFileInfo{name: "schema.yaml"}, nil
		}
		if strings.Contains(name, "contexts") && strings.Contains(name, "values.yaml") {
			return &mockFileInfo{name: "values.yaml"}, nil
		}
		if strings.Contains(name, "_template/metadata.yaml") {
			return nil, os.ErrNotExist
		}
		if strings.Contains(name, "_template") && !strings.Contains(name, "schema.yaml") {
			return &mockFileInfo{name: "_template", isDir: true}, nil
		}
		// Default: file does not exist
		return nil, os.ErrNotExist
	}

	shims.MkdirAll = func(name string, perm fs.FileMode) error {
		return nil
	}

	// Default: empty template directory (successful template processing)
	shims.ReadDir = func(name string) ([]os.DirEntry, error) {
		return []os.DirEntry{}, nil
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

func setupBlueprintMocks(t *testing.T, opts ...func(*BlueprintTestMocks)) *BlueprintTestMocks {
	t.Helper()

	// Unset BUILD_ID to ensure tests aren't affected by environment
	os.Unsetenv("BUILD_ID")

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

	// Set up config handler - default to MockConfigHandler for easier testing
	var configHandler config.ConfigHandler
	mockConfigHandler := config.NewMockConfigHandler()
	// Set up default mock behaviors with stateful context handling
	currentContext := "local" // Default context

	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
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

	configHandler = mockConfigHandler

	// Create mock shell and kubernetes manager
	mockShell := shell.NewMockShell()

	mockKubernetesManager := kubernetes.NewMockKubernetesManager()
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

	// Create shims
	shims := setupDefaultShims()

	// Set up default GetContextValues for mock config handler
	mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
		return make(map[string]any), nil
	}

	// Create runtime
	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		TemplateRoot:  filepath.Join(tmpDir, "contexts", "_template"),
		ConfigHandler: configHandler,
		Shell:         mockShell,
	}

	// Create mocks struct
	mocks := &BlueprintTestMocks{
		Shell:             mockShell,
		ConfigHandler:     configHandler,
		Shims:             shims,
		KubernetesManager: mockKubernetesManager,
		Runtime:           rt,
	}

	// Apply options
	for _, opt := range opts {
		opt(mocks)
	}

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

	mocks.ConfigHandler.SetContext("mock-context")

	if err := mocks.ConfigHandler.LoadConfigString(defaultConfigStr); err != nil {
		t.Fatalf("Failed to load default config string: %v", err)
	}

	// Cleanup function
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		os.Unsetenv("WINDSOR_CONTEXT")
		os.Unsetenv("BUILD_ID")
		os.Chdir(tmpDir)
	})

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBlueprintHandler_NewBlueprintHandler(t *testing.T) {
	t.Run("CreatesHandlerWithMocks", func(t *testing.T) {
		// Given mocks
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()

		// When creating a new blueprint handler
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}

		// Then the handler should be properly initialized
		if handler == nil {
			t.Fatal("Expected non-nil handler")
		}

		// And basic fields should be set
		if handler.shims == nil {
			t.Error("Expected shims to be set")
		}

		// And dependencies should be set
		if handler.runtime.ConfigHandler == nil {
			t.Error("Expected configHandler to be set")
		}
		if handler.runtime.Shell == nil {
			t.Error("Expected shell to be set")
		}
		if handler.artifactBuilder == nil {
			t.Error("Expected artifactBuilder to be set")
		}
	})

	t.Run("HandlesFeatureEvaluatorOverride", func(t *testing.T) {
		// Given mocks
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()

		// And a custom feature evaluator
		customEvaluator := NewFeatureEvaluator(mocks.Runtime)

		// When creating a handler with feature evaluator override
		overrideHandler := &BaseBlueprintHandler{
			featureEvaluator: customEvaluator,
		}
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder, overrideHandler)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}

		// Then the handler should use the custom feature evaluator
		if handler.featureEvaluator != customEvaluator {
			t.Error("Expected custom feature evaluator to be used")
		}
	})

	t.Run("HandlesNilOverride", func(t *testing.T) {
		// Given mocks
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()

		// When creating a handler with nil override
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder, nil)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}

		// Then it should succeed and use default feature evaluator
		if handler == nil {
			t.Fatal("Expected non-nil handler")
		}
		if handler.featureEvaluator == nil {
			t.Error("Expected default feature evaluator to be set")
		}
	})
}

func TestBlueprintHandler_NewBlueprintHandlerWithError(t *testing.T) {
	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a runtime with empty ProjectRoot
		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = ""

		// When creating a new blueprint handler
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)

		// Then it should succeed (ProjectRoot is set on runtime, not validated in constructor)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if handler == nil {
			t.Error("Expected non-nil handler")
		}
	})
}

func TestBlueprintHandler_GetTerraformComponents(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *BlueprintTestMocks) {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
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
		handler.runtime.ProjectRoot = projectRoot

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
				Inputs: map[string]any{"key": "value"},
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
		if resolvedComponents[0].Inputs["key"] != "value" {
			t.Errorf("Expected value 'value' for key 'key', got %q", resolvedComponents[0].Inputs["key"])
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
		handler.runtime.ProjectRoot = projectRoot

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
				Inputs: map[string]any{"key": "value"},
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
		handler.runtime.ProjectRoot = projectRoot

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
				Inputs: map[string]any{"key": "value"},
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

func TestBlueprintHandler_GetLocalTemplateData(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *BlueprintTestMocks) {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("ReturnsEmptyMapWhenTemplateDirectoryNotExists", func(t *testing.T) {
		// Given a blueprint handler with no template directory
		handler, mocks := setup(t)

		// Mock shell to return project root
		mocks.Runtime.ProjectRoot = filepath.Join("/mock", "project")

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

	t.Run("ReturnsErrorWhenTemplateDirectoryReadFails", func(t *testing.T) {
		// Given a blueprint handler with template directory that fails to read
		handler, _ := setup(t)

		// Mock shims to return error when reading template directory
		if baseHandler, ok := handler.(*BaseBlueprintHandler); ok {
			baseHandler.shims.Stat = func(path string) (os.FileInfo, error) {
				if path == baseHandler.runtime.TemplateRoot {
					return nil, fmt.Errorf("failed to read template directory")
				}
				return nil, os.ErrNotExist
			}

			// Mock ReadDir to return error when trying to read the template directory
			baseHandler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
				if path == baseHandler.runtime.TemplateRoot {
					return nil, fmt.Errorf("failed to read template directory")
				}
				return nil, fmt.Errorf("directory not found")
			}
		}

		// When getting local template data
		result, err := handler.GetLocalTemplateData()

		// Then error should occur
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to read template directory") {
			t.Errorf("Expected error to contain 'failed to read template directory', got: %v", err)
		}

		// And result should be nil
		if result != nil {
			t.Error("Expected result to be nil on error")
		}
	})

	t.Run("ReturnsErrorWhenWalkAndCollectTemplatesFails", func(t *testing.T) {
		// Given a blueprint handler with template directory that fails to read
		projectRoot := filepath.Join("mock", "project")
		templateDir := filepath.Join(projectRoot, "contexts", "_template")

		// Set up mocks first, before initializing the handler
		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot
		mocks.Runtime.TemplateRoot = templateDir

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		// Mock shims to simulate template directory exists but ReadDir fails
		baseHandler := handler
		baseHandler.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == templateDir {
				return mockFileInfo{name: "_template"}, nil
			}
			return nil, os.ErrNotExist
		}

		baseHandler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			if path == templateDir {
				return nil, fmt.Errorf("failed to read directory")
			}
			return nil, os.ErrNotExist
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

	t.Run("MergesOCIArtifactValuesWithLocalContextValues", func(t *testing.T) {
		// Given a blueprint handler with OCI artifact values already in template data
		handler, mocks := setup(t)

		// Ensure the handler uses the mock shell and config handler
		baseHandler := handler.(*BaseBlueprintHandler)
		baseHandler.runtime.Shell = mocks.Shell
		baseHandler.runtime.ConfigHandler = mocks.ConfigHandler

		// Mock local context values
		projectRoot := filepath.Join("tmp", "test")
		mocks.Runtime.ProjectRoot = projectRoot
		baseHandler.shims.Stat = func(path string) (os.FileInfo, error) {
			// Normalize path separators for cross-platform compatibility
			normalizedPath := filepath.ToSlash(path)
			if strings.Contains(normalizedPath, "_template/schema.yaml") {
				return &mockFileInfo{isDir: false}, nil
			}
			if strings.Contains(normalizedPath, "test-context/values.yaml") {
				return &mockFileInfo{isDir: false}, nil
			}
			if normalizedPath == filepath.ToSlash(filepath.Join(projectRoot, "contexts", "_template")) {
				return &mockFileInfo{isDir: true}, nil
			}
			if strings.Contains(normalizedPath, "_template/metadata.yaml") {
				return nil, os.ErrNotExist
			}
			if strings.Contains(normalizedPath, "_template") && !strings.Contains(normalizedPath, "schema.yaml") {
				return &mockFileInfo{isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}
		baseHandler.shims.ReadFile = func(path string) ([]byte, error) {
			// Normalize path separators for cross-platform compatibility
			normalizedPath := filepath.ToSlash(path)
			if strings.Contains(normalizedPath, "_template/schema.yaml") {
				return []byte(`$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  external_domain:
    type: string
    default: "local.test"
  registry_url:
    type: string
    default: "registry.local.test"
  local_only:
    type: object
    properties:
      enabled:
        type: boolean
        default: true
  substitutions:
    type: object
    properties:
      common:
        type: object
        properties:
          external_domain:
            type: string
            default: "local.test"
          registry_url:
            type: string
            default: "registry.local.test"
        additionalProperties: true
      local_only:
        type: object
        properties:
          enabled:
            type: boolean
            default: true
        additionalProperties: true
    additionalProperties: true
required: []
additionalProperties: true`), nil
			}
			if strings.Contains(normalizedPath, "test-context/values.yaml") {
				return []byte(`external_domain: context.test
context_only:
  enabled: true
substitutions:
  common:
    external_domain: context.test
  context_only:
    enabled: true`), nil
			}
			return nil, os.ErrNotExist
		}

		// Cast to mock config handler to set the function
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mocks.Runtime.ConfigRoot = filepath.Join(projectRoot, "contexts", "test-context")
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"external_domain": "context.test",
				"context_only": map[string]any{
					"enabled": true,
				},
				"substitutions": map[string]any{
					"common": map[string]any{
						"external_domain": "context.test",
						"registry_url":    "registry.local.test",
					},
					"local_only": map[string]any{
						"enabled": true,
					},
					"context_only": map[string]any{
						"enabled": true,
					},
				},
			}, nil
		}

		// When GetLocalTemplateData is called
		result, err := handler.GetLocalTemplateData()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And result should contain merged values
		if result == nil {
			t.Fatal("Expected result to not be nil")
		}

		// This test doesn't include schema.yaml in the mock, so no schema key expected
		// Values processing is handled through the config context now

		// Values validation is now done at the schema level, not in templateData

		// Substitutions are now in Features, not in templateData
	})

	t.Run("MergesContextValuesWithTemplateData", func(t *testing.T) {
		// Given a blueprint handler with template directory and context values
		handler, mocks := setup(t)

		// Ensure the handler uses the mock shell and config handler
		baseHandler := handler.(*BaseBlueprintHandler)
		baseHandler.runtime.Shell = mocks.Shell
		baseHandler.runtime.ConfigHandler = mocks.ConfigHandler

		projectRoot := filepath.Join("mock", "project")

		// Mock shell to return project root
		mocks.Runtime.ProjectRoot = projectRoot

		// Mock config handler to return context
		if mockConfigHandler, ok := mocks.ConfigHandler.(*config.MockConfigHandler); ok {
			mockConfigHandler.GetContextFunc = func() string {
				return "test-context"
			}
			mocks.Runtime.ConfigRoot = filepath.Join(projectRoot, "contexts", "test-context")
			mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
				return map[string]any{
					"external_domain": "context.test",
					"context_only":    "context_value",
					"substitutions": map[string]any{
						"common": map[string]any{
							"registry_url": "registry.context.test",
						},
						"csi": map[string]any{
							"volume_path": "/context/volumes",
						},
					},
				}, nil
			}
			mockConfigHandler.LoadSchemaFromBytesFunc = func(data []byte) error {
				return nil
			}
		}

		// Mock shims to simulate template directory and schema files
		if baseHandler, ok := handler.(*BaseBlueprintHandler); ok {
			baseHandler.shims.Stat = func(path string) (os.FileInfo, error) {
				// Normalize path separators for cross-platform compatibility
				normalizedPath := filepath.ToSlash(path)
				if strings.Contains(normalizedPath, "_template/schema.yaml") ||
					strings.Contains(normalizedPath, "test-context/values.yaml") {
					return mockFileInfo{name: "template"}, nil
				}
				if strings.Contains(normalizedPath, "_template/metadata.yaml") {
					return nil, os.ErrNotExist
				}
				templateRoot := filepath.ToSlash(baseHandler.runtime.TemplateRoot)
				if normalizedPath == templateRoot {
					return mockFileInfo{name: "_template", isDir: true}, nil
				}
				if strings.Contains(normalizedPath, "_template") && !strings.Contains(normalizedPath, "schema.yaml") {
					return mockFileInfo{name: "_template", isDir: true}, nil
				}
				return nil, os.ErrNotExist
			}

			baseHandler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
				// Normalize path separators for cross-platform compatibility
				normalizedPath := filepath.ToSlash(path)
				if strings.Contains(normalizedPath, "_template") {
					return []os.DirEntry{
						&mockDirEntry{name: "schema.yaml", isDir: false},
					}, nil
				}
				return nil, fmt.Errorf("directory not found")
			}

			baseHandler.shims.ReadFile = func(path string) ([]byte, error) {
				// Normalize path separators for cross-platform compatibility
				normalizedPath := filepath.ToSlash(path)
				if strings.Contains(normalizedPath, "blueprint.jsonnet") {
					return []byte("{ kind: 'Blueprint' }"), nil
				}
				if strings.Contains(normalizedPath, "_template/schema.yaml") {
					return []byte(`$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  external_domain:
    type: string
    default: "template.test"
  template_only:
    type: string
    default: "template_value"
  substitutions:
    type: object
    properties:
      common:
        type: object
        properties:
          registry_url:
            type: string
            default: "registry.template.test"
        additionalProperties: true
    additionalProperties: true
required: []
additionalProperties: true`), nil
				}
				if strings.Contains(normalizedPath, "test-context/values.yaml") {
					return []byte(`
external_domain: context.test
context_only: context_value
substitutions:
  common:
    registry_url: registry.context.test
  csi:
    volume_path: /context/volumes
`), nil
				}
				return nil, fmt.Errorf("file not found: %s", path)
			}

			baseHandler.shims.YamlMarshal = func(v any) ([]byte, error) {
				return yaml.Marshal(v)
			}

			baseHandler.shims.YamlUnmarshal = func(data []byte, v any) error {
				return yaml.Unmarshal(data, v)
			}
		}

		// When getting local template data
		result, err := handler.GetLocalTemplateData()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should contain template files
		if len(result) == 0 {
			t.Fatal("Expected template data, got empty map")
		}

		// .jsonnet files are not collected in templateData; they are processed on-demand via jsonnet() function calls during feature evaluation
		// Schema.yaml should be collected
		if _, exists := result["_template/schema.yaml"]; !exists {
			t.Error("Expected _template/schema.yaml to be collected")
		}

		// Substitutions are now in Features, not in templateData
	})

	t.Run("HandlesContextValuesWithoutExistingValues", func(t *testing.T) {
		// Given a blueprint handler with only context values (no existing OCI values)
		projectRoot := filepath.Join("mock", "project")
		templateDir := filepath.Join(projectRoot, "contexts", "_template")

		// Set up mocks first, before initializing the handler
		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot
		mocks.Runtime.TemplateRoot = templateDir

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		// Ensure the handler uses the mock shell and config handler
		baseHandler := handler
		baseHandler.runtime.Shell = mocks.Shell
		baseHandler.runtime.ConfigHandler = mocks.ConfigHandler

		// Mock config handler to return context
		if mockConfigHandler, ok := mocks.ConfigHandler.(*config.MockConfigHandler); ok {
			mockConfigHandler.GetContextFunc = func() string {
				return "test-context"
			}
			mocks.Runtime.ConfigRoot = filepath.Join(projectRoot, "contexts", "test-context")
			mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
				return map[string]any{
					"external_domain": "context.test",
					"context_only":    "context_value",
					"substitutions": map[string]any{
						"common": map[string]any{
							"registry_url": "registry.context.test",
							"context_sub":  "context_sub_value",
						},
					},
				}, nil
			}
		}

		// Mock shims to simulate template directory and context values
		baseHandler.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == templateDir ||
				path == filepath.Join(projectRoot, "contexts", "test-context", "values.yaml") {
				return mockFileInfo{name: "template"}, nil
			}
			return nil, os.ErrNotExist
		}

		baseHandler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			if path == templateDir {
				return []os.DirEntry{
					&mockDirEntry{name: "blueprint.jsonnet", isDir: false},
				}, nil
			}
			return nil, fmt.Errorf("directory not found")
		}

		baseHandler.shims.ReadFile = func(path string) ([]byte, error) {
			switch path {
			case filepath.Join(templateDir, "blueprint.jsonnet"):
				return []byte("{ kind: 'Blueprint' }"), nil
			case filepath.Join(projectRoot, "contexts", "test-context", "values.yaml"):
				return []byte(`
external_domain: context.test
context_only: context_value
substitutions:
  common:
    registry_url: registry.context.test
    context_sub: context_sub_value
`), nil
			default:
				return nil, fmt.Errorf("file not found: %s", path)
			}
		}

		baseHandler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return yaml.Marshal(v)
		}

		baseHandler.shims.YamlUnmarshal = func(data []byte, v any) error {
			return yaml.Unmarshal(data, v)
		}

		// When getting local template data
		_, err = handler.GetLocalTemplateData()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Substitutions are now in Features, not in templateData
	})

	t.Run("HandlesErrorInLoadAndMergeContextValues", func(t *testing.T) {
		// Given a blueprint handler that fails to load context values
		projectRoot := filepath.Join("mock", "project")
		templateDir := filepath.Join(projectRoot, "contexts", "_template")

		// Set up mocks first, before initializing the handler
		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot
		mocks.Runtime.TemplateRoot = templateDir

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		// Mock config handler to return error when getting context values
		if mockConfigHandler, ok := mocks.ConfigHandler.(*config.MockConfigHandler); ok {
			mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
				return nil, fmt.Errorf("failed to load context values")
			}
		}

		// Mock shims to simulate template directory exists
		baseHandler := handler
		baseHandler.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == templateDir {
				return mockFileInfo{name: "template"}, nil
			}
			return nil, os.ErrNotExist
		}

		baseHandler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			if path == templateDir {
				return []os.DirEntry{
					&mockDirEntry{name: "blueprint.jsonnet", isDir: false},
				}, nil
			}
			return nil, fmt.Errorf("directory not found")
		}

		baseHandler.shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(templateDir, "blueprint.jsonnet") {
				return []byte("{ kind: 'Blueprint' }"), nil
			}
			return nil, fmt.Errorf("file not found: %s", path)
		}

		// When getting local template data
		_, err = handler.GetLocalTemplateData()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error when GetContextValues fails")
			return
		}

		if !strings.Contains(err.Error(), "failed to load context values") {
			t.Errorf("Expected error about context values, got: %v", err)
		}
	})
}

func TestBlueprintHandler_Write(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *BlueprintTestMocks) {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a blueprint handler with a loaded blueprint
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"

		// Set up the blueprint with test data
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: blueprintv1alpha1.Metadata{
				Name:        "test-blueprint",
				Description: "A test blueprint",
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
		expectedPath := filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml")
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
		mocks.Runtime.ConfigRoot = "/test/config"

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
		expectedPath := filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml")
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
		// Given a blueprint handler with empty ConfigRoot
		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ConfigRoot = ""
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		// When Write is called
		err = handler.Write()

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected error from empty ConfigRoot, got nil")
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
					Inputs: map[string]any{
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
		if len(component.Inputs) != 0 {
			t.Errorf("Expected all inputs to be cleared, but got %d inputs: %v", len(component.Inputs), component.Inputs)
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

	t.Run("DoesNotWritePatchesToDisk", func(t *testing.T) {
		// Given a blueprint handler with kustomization containing strategic merge patches
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"
		mocks.Runtime.TemplateRoot = "/test/template"

		handler.blueprint = blueprintv1alpha1.Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Patches: []blueprintv1alpha1.BlueprintPatch{
						{
							Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: value`,
						},
					},
				},
			},
		}

		var writtenPatchPaths []string
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			normalizedName := filepath.ToSlash(name)
			if strings.Contains(normalizedName, "patches/") {
				writtenPatchPaths = append(writtenPatchPaths, name)
			}
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "blueprint.yaml") {
				return nil, os.ErrNotExist
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("kind: Blueprint\n"), nil
		}

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}

		// When Write is called
		err := handler.Write()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And no patch files should be written (patches are processed in memory only)
		if len(writtenPatchPaths) != 0 {
			t.Errorf("Expected no patch files to be written, but got: %v", writtenPatchPaths)
		}
	})

	t.Run("SkipsWritingWhenNoStrategicMergePatches", func(t *testing.T) {
		// Given a blueprint handler with kustomization containing only inline patches
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"

		handler.blueprint = blueprintv1alpha1.Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Patches: []blueprintv1alpha1.BlueprintPatch{
						{
							Target: &kustomize.Selector{
								Kind: "ConfigMap",
								Name: "test-config",
							},
							Patch: `[{"op": "replace", "path": "/data/key", "value": "newvalue"}]`,
						},
					},
				},
			},
		}

		var writtenPatchPaths []string
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			normalizedName := filepath.ToSlash(name)
			if strings.Contains(normalizedName, "patches/") {
				writtenPatchPaths = append(writtenPatchPaths, name)
			}
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("kind: Blueprint\n"), nil
		}

		// When Write is called
		err := handler.Write()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And no patch files should be written
		if len(writtenPatchPaths) != 0 {
			t.Errorf("Expected no patch files to be written, but got: %v", writtenPatchPaths)
		}
	})
}

func TestBlueprintHandler_LoadBlueprint(t *testing.T) {
	t.Run("LoadsBlueprintSuccessfullyWithLocalTemplates", func(t *testing.T) {
		// Given a blueprint handler with local templates
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		// Set up project root and template root
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		templateRootNormalized := filepath.ToSlash(mocks.Runtime.TemplateRoot)
		blueprintContent := `apiVersion: v1alpha1
kind: Blueprint
metadata:
  name: test-blueprint
  description: Test blueprint
sources: []
terraformComponents: []
kustomizations: []`

		originalReadFile := handler.shims.ReadFile
		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			normalizedPath := filepath.ToSlash(path)
			if normalizedPath == templateRootNormalized {
				return &mockFileInfo{name: "_template", isDir: true}, nil
			}
			if strings.Contains(normalizedPath, "_template/metadata.yaml") {
				return nil, os.ErrNotExist
			}
			if strings.Contains(normalizedPath, "_template/blueprint.yaml") {
				return &mockFileInfo{name: "blueprint.yaml", isDir: false}, nil
			}
			if strings.Contains(normalizedPath, "_template") {
				return &mockFileInfo{name: "_template", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			normalizedPath := filepath.ToSlash(path)
			if normalizedPath == templateRootNormalized {
				return []os.DirEntry{
					&mockDirEntry{name: "blueprint.yaml", isDir: false},
				}, nil
			}
			return []os.DirEntry{}, nil
		}
		handler.shims.ReadFile = func(path string) ([]byte, error) {
			normalizedPath := filepath.ToSlash(path)
			if strings.Contains(normalizedPath, "_template/blueprint.yaml") {
				return []byte(blueprintContent), nil
			}
			return originalReadFile(path)
		}

		// Mock config handler to return empty context values
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}

		// When loading blueprint
		err = handler.LoadBlueprint()

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And blueprint should be loaded with name from context
		metadata := handler.getMetadata()
		if metadata.Name != "test-context" {
			t.Errorf("Expected blueprint name 'test-context' (from context), got %s", metadata.Name)
		}
	})

	t.Run("HandlesGetLocalTemplateDataError", func(t *testing.T) {
		// Given a handler with template root that exists but GetLocalTemplateData fails
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")

		// Mock Stat to return success (template root exists)
		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == mocks.Runtime.TemplateRoot {
				return &mockFileInfo{name: "_template", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock ReadDir to return error (causes GetLocalTemplateData to fail)
		handler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			if path == mocks.Runtime.TemplateRoot {
				return nil, fmt.Errorf("readdir error")
			}
			return []os.DirEntry{}, nil
		}

		// When loading blueprint
		err = handler.LoadBlueprint()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when GetLocalTemplateData fails")
		}
		if !strings.Contains(err.Error(), "failed to get local template data") {
			t.Errorf("Expected error about local template data, got: %v", err)
		}
	})

	t.Run("HandlesEmptyTemplateDataWithEmptyConfigRoot", func(t *testing.T) {
		// Given a handler where template root exists, but config root is empty
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		// And the runtime roots set up appropriately
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = ""

		// And context configured on the mock config handler
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		// And FileInfo/ReadDir shims return empty data
		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == mocks.Runtime.TemplateRoot {
				return &mockFileInfo{name: "_template", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{}, nil
		}

		// When LoadBlueprint is called
		err = handler.LoadBlueprint()

		// Then it should succeed, with no error (empty blueprint with metadata created)
		if err != nil {
			t.Errorf("Expected no error when template root exists (creates empty blueprint with metadata), got %v", err)
		}
	})

	t.Run("HandlesEmptyTemplateDataWithBlueprintNotFound", func(t *testing.T) {
		// Given a handler where template root exists but blueprint.yaml is missing
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		// And project/config/template roots set up
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		// And context configured on the mock config handler
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		// And Stat/ReadDir shims such that blueprint.yaml doesn't exist, but template root does
		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == mocks.Runtime.TemplateRoot {
				return &mockFileInfo{name: "_template", isDir: true}, nil
			}
			if strings.HasSuffix(path, "blueprint.yaml") {
				return nil, os.ErrNotExist
			}
			return nil, os.ErrNotExist
		}

		handler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{}, nil
		}

		// When LoadBlueprint is called
		err = handler.LoadBlueprint()

		// Then it should succeed, with no error (empty blueprint with metadata created)
		if err != nil {
			t.Errorf("Expected no error when template root exists (creates empty blueprint with metadata), got %v", err)
		}
	})

	t.Run("HandlesGetTemplateDataError", func(t *testing.T) {
		// Given a handler with no template root and no blueprint.yaml, but GetTemplateData fails
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.GetTemplateDataFunc = func(url string) (map[string][]byte, error) {
			return nil, fmt.Errorf("get template data error")
		}
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		// Mock Stat to return errors (no template root, no blueprint.yaml)
		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When loading blueprint
		err = handler.LoadBlueprint()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when GetTemplateData fails")
		}
		if !strings.Contains(err.Error(), "failed to get template data from blueprint") {
			t.Errorf("Expected error about getting template data, got: %v", err)
		}
	})

	t.Run("HandlesArtifactBuilderNil", func(t *testing.T) {
		// Given a handler with no template root, no blueprint.yaml, and nil artifact builder
		mocks := setupBlueprintMocks(t)
		handler, err := NewBlueprintHandler(mocks.Runtime, nil)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		handler.artifactBuilder = nil

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		// Mock Stat to return errors (no template root, no blueprint.yaml)
		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Set valid blueprint URL
		os.Setenv("WINDSOR_BLUEPRINT_URL", "oci://registry.example.com/blueprint:latest")
		defer os.Unsetenv("WINDSOR_BLUEPRINT_URL")

		// When loading blueprint
		err = handler.LoadBlueprint()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when artifact builder is nil")
		}
		if !strings.Contains(err.Error(), "artifact builder not available") {
			t.Errorf("Expected error about artifact builder, got: %v", err)
		}
	})

	t.Run("HandlesLoadConfigWhenTemplateRootDoesNotExistButBlueprintYamlExists", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		blueprintPath := filepath.Join(tmpDir, "blueprint.yaml")
		blueprintContent := `apiVersion: v1alpha1
kind: Blueprint
metadata:
  name: test-blueprint
`
		if err := os.WriteFile(blueprintPath, []byte(blueprintContent), 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == mocks.Runtime.TemplateRoot {
				return nil, os.ErrNotExist
			}
			if path == blueprintPath {
				return &mockFileInfo{name: "blueprint.yaml", isDir: false}, nil
			}
			return nil, os.ErrNotExist
		}

		err = handler.LoadBlueprint()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("HandlesGetTemplateDataErrorWhenNoLocalBlueprint", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		os.Setenv("WINDSOR_BLUEPRINT_URL", "oci://registry.example.com/blueprint:latest")
		defer os.Unsetenv("WINDSOR_BLUEPRINT_URL")

		expectedError := fmt.Errorf("template data error")
		mockArtifactBuilder.GetTemplateDataFunc = func(url string) (map[string][]byte, error) {
			return nil, expectedError
		}

		err = handler.LoadBlueprint()

		if err == nil {
			t.Fatal("Expected error when GetTemplateData fails")
		}
		if !strings.Contains(err.Error(), "failed to get template data from blueprint") {
			t.Errorf("Expected error about template data, got: %v", err)
		}
	})

	t.Run("HandlesLoadDataErrorWhenNoLocalBlueprint", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		os.Setenv("WINDSOR_BLUEPRINT_URL", "oci://registry.example.com/blueprint:latest")
		defer os.Unsetenv("WINDSOR_BLUEPRINT_URL")

		mockArtifactBuilder.GetTemplateDataFunc = func(url string) (map[string][]byte, error) {
			return map[string][]byte{"_template/blueprint.yaml": []byte("invalid yaml")}, nil
		}

		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("yaml unmarshal error")
		}

		err = handler.LoadBlueprint()

		if err == nil {
			t.Fatal("Expected error when processing template data fails")
		}
		if !strings.Contains(err.Error(), "failed to process features") && !strings.Contains(err.Error(), "failed to load default blueprint data") {
			t.Errorf("Expected error about processing template data, got: %v", err)
		}
	})

	t.Run("HandlesEmptyTemplateDataWithEmptyConfigRoot", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = ""

		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == mocks.Runtime.TemplateRoot {
				return &mockFileInfo{name: "_template", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{}, nil
		}

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		err = handler.LoadBlueprint()

		if err != nil {
			t.Errorf("Expected no error when template root exists (creates empty blueprint with metadata), got %v", err)
		}
	})

	t.Run("HandlesLoadConfigErrorWhenTemplateRootDoesNotExist", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		blueprintPath := filepath.Join(tmpDir, "blueprint.yaml")
		invalidBlueprintContent := `invalid: yaml: [`
		if err := os.WriteFile(blueprintPath, []byte(invalidBlueprintContent), 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == mocks.Runtime.TemplateRoot {
				return nil, os.ErrNotExist
			}
			if path == blueprintPath {
				return &mockFileInfo{name: "blueprint.yaml", isDir: false}, nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("yaml unmarshal error")
		}

		err = handler.LoadBlueprint()

		if err == nil {
			t.Fatal("Expected error when loadConfig fails")
		}
		if !strings.Contains(err.Error(), "failed to load blueprint config") {
			t.Errorf("Expected error about loading blueprint config, got: %v", err)
		}
	})

	t.Run("HandlesPullOCISourcesError", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		blueprintContent := `apiVersion: v1alpha1
kind: Blueprint
metadata:
  name: test-blueprint
sources:
  - name: oci-source
    url: oci://ghcr.io/test/blueprint:latest
terraformComponents: []
kustomizations: []
`

		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == mocks.Runtime.TemplateRoot {
				return &mockFileInfo{name: "_template", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			if path == mocks.Runtime.TemplateRoot {
				return []os.DirEntry{
					&mockDirEntry{name: "blueprint.yaml", isDir: false},
				}, nil
			}
			return []os.DirEntry{}, nil
		}

		handler.shims.ReadFile = func(path string) ([]byte, error) {
			if strings.Contains(path, "blueprint.yaml") {
				return []byte(blueprintContent), nil
			}
			return nil, os.ErrNotExist
		}

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		expectedError := fmt.Errorf("pull error")
		mockArtifactBuilder.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			return nil, expectedError
		}

		err = handler.LoadBlueprint()

		if err == nil {
			t.Fatal("Expected error when Pull fails")
		}
		if !strings.Contains(err.Error(), "failed to load OCI sources") {
			t.Errorf("Expected error about loading OCI sources, got: %v", err)
		}
	})

	t.Run("HandlesLoadConfigErrorAfterProcessingSources", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		blueprintContent := `apiVersion: v1alpha1
kind: Blueprint
metadata:
  name: test-blueprint
sources: []
terraformComponents: []
kustomizations: []
`

		blueprintPath := filepath.Join(tmpDir, "blueprint.yaml")
		invalidBlueprintContent := `invalid: yaml: [`
		if err := os.WriteFile(blueprintPath, []byte(invalidBlueprintContent), 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == mocks.Runtime.TemplateRoot {
				return &mockFileInfo{name: "_template", isDir: true}, nil
			}
			if path == blueprintPath {
				return &mockFileInfo{name: "blueprint.yaml", isDir: false}, nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			if path == mocks.Runtime.TemplateRoot {
				return []os.DirEntry{
					&mockDirEntry{name: "blueprint.yaml", isDir: false},
				}, nil
			}
			return []os.DirEntry{}, nil
		}

		handler.shims.ReadFile = func(path string) ([]byte, error) {
			normalizedPath := filepath.ToSlash(path)
			if strings.Contains(normalizedPath, "_template/blueprint.yaml") || strings.HasSuffix(normalizedPath, "_template/blueprint.yaml") {
				return []byte(blueprintContent), nil
			}
			if path == blueprintPath {
				return []byte(invalidBlueprintContent), nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			if strings.Contains(string(data), "invalid") {
				return fmt.Errorf("yaml unmarshal error")
			}
			return yaml.Unmarshal(data, v)
		}

		err = handler.LoadBlueprint()

		if err == nil {
			t.Fatal("Expected error when loadConfig fails after processing sources")
		}
		if !strings.Contains(err.Error(), "failed to load blueprint config overrides") {
			t.Errorf("Expected error about loading blueprint config overrides, got: %v", err)
		}
	})

	t.Run("HandlesOCISourcesWithNonOCISources", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		blueprintContent := `apiVersion: v1alpha1
kind: Blueprint
metadata:
  name: test-blueprint
sources:
  - name: git-source
    url: git::https://example.com/repo.git
  - name: oci-source
    url: oci://ghcr.io/test/blueprint:latest
terraformComponents: []
kustomizations: []
`

		handler.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == mocks.Runtime.TemplateRoot {
				return &mockFileInfo{name: "_template", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			if path == mocks.Runtime.TemplateRoot {
				return []os.DirEntry{
					&mockDirEntry{name: "blueprint.yaml", isDir: false},
				}, nil
			}
			return []os.DirEntry{}, nil
		}

		handler.shims.ReadFile = func(path string) ([]byte, error) {
			if strings.Contains(path, "blueprint.yaml") {
				return []byte(blueprintContent), nil
			}
			return nil, os.ErrNotExist
		}

		mockArtifactBuilder.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			if len(ociRefs) != 1 || ociRefs[0] != "oci://ghcr.io/test/blueprint:latest" {
				t.Errorf("Expected single OCI URL, got: %v", ociRefs)
			}
			return nil, nil
		}

		err = handler.LoadBlueprint()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

}

func TestBaseBlueprintHandler_GetLocalTemplateData(t *testing.T) {
	t.Run("CollectsBlueprintAndFeatureFiles", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		contextName := "test-context"
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return contextName
		}

		projectRoot = os.Getenv("WINDSOR_PROJECT_ROOT")
		templateDir := filepath.Join(projectRoot, "contexts", "_template")
		featuresDir := filepath.Join(templateDir, "features")
		contextDir := filepath.Join(projectRoot, "contexts", contextName)

		if err := os.MkdirAll(featuresDir, 0755); err != nil {
			t.Fatalf("Failed to create features directory: %v", err)
		}
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		blueprintContent := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base-blueprint
`)
		if err := os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), blueprintContent, 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		awsFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
when: provider == "aws"
`)
		if err := os.WriteFile(filepath.Join(featuresDir, "aws.yaml"), awsFeature, 0644); err != nil {
			t.Fatalf("Failed to write aws feature: %v", err)
		}

		observabilityFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: observability-feature
`)
		if err := os.WriteFile(filepath.Join(featuresDir, "observability.yaml"), observabilityFeature, 0644); err != nil {
			t.Fatalf("Failed to write observability feature: %v", err)
		}

		jsonnetTemplate := []byte(`{
  terraform: {
    cluster: {
      node_count: 3
    }
  }
}`)
		if err := os.WriteFile(filepath.Join(templateDir, "terraform.jsonnet"), jsonnetTemplate, 0644); err != nil {
			t.Fatalf("Failed to write jsonnet template: %v", err)
		}

		templateData, err := handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if _, exists := templateData["_template/blueprint.yaml"]; !exists {
			t.Error("Expected blueprint.yaml to be collected")
		}

		if _, exists := templateData["_template/features/aws.yaml"]; !exists {
			t.Error("Expected features/aws.yaml to be collected")
		}

		if _, exists := templateData["_template/features/observability.yaml"]; !exists {
			t.Error("Expected features/observability.yaml to be collected")
		}

		// .jsonnet files are not collected in templateData; they are processed on-demand via jsonnet() function calls during feature evaluation

		if content, exists := templateData["_template/blueprint.yaml"]; exists {
			if !strings.Contains(string(content), contextName) {
				t.Errorf("Expected blueprint content to contain context name '%s', got: %s", contextName, string(content))
			}
		}

		if content, exists := templateData["_template/features/aws.yaml"]; exists {
			if !strings.Contains(string(content), "aws-feature") {
				t.Errorf("Expected aws feature content to contain 'aws-feature', got: %s", string(content))
			}
		}
	})

	t.Run("CollectsNestedFeatures", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		contextName := "test-context"
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return contextName
		}

		projectRoot = os.Getenv("WINDSOR_PROJECT_ROOT")
		templateDir := filepath.Join(projectRoot, "contexts", "_template")
		nestedFeaturesDir := filepath.Join(templateDir, "features", "aws")
		contextDir := filepath.Join(projectRoot, "contexts", contextName)

		if err := os.MkdirAll(nestedFeaturesDir, 0755); err != nil {
			t.Fatalf("Failed to create nested features directory: %v", err)
		}
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		nestedFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-eks-feature
`)
		if err := os.WriteFile(filepath.Join(nestedFeaturesDir, "eks.yaml"), nestedFeature, 0644); err != nil {
			t.Fatalf("Failed to write nested feature: %v", err)
		}

		templateData, err := handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if _, exists := templateData["_template/features/aws/eks.yaml"]; !exists {
			t.Error("Expected features/aws/eks.yaml to be collected")
		}
	})

	t.Run("HandlesYamlMarshalErrorWhenComposingBlueprint", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		if err := os.MkdirAll(mocks.Runtime.TemplateRoot, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		blueprintContent := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base-blueprint
terraform:
  - path: test/component
`)
		if err := os.WriteFile(filepath.Join(mocks.Runtime.TemplateRoot, "blueprint.yaml"), blueprintContent, 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		handler.shims.Stat = os.Stat
		handler.shims.ReadDir = os.ReadDir
		handler.shims.ReadFile = os.ReadFile
		handler.shims.YamlUnmarshal = yaml.Unmarshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}

		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("yaml marshal error")
		}

		_, err = handler.GetLocalTemplateData()

		if err == nil {
			t.Fatal("Expected error when YamlMarshal fails")
		}
		if !strings.Contains(err.Error(), "failed to marshal composed blueprint") {
			t.Errorf("Expected error about marshaling blueprint, got: %v", err)
		}
	})

	t.Run("HandlesGetContextValuesError", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		if err := os.MkdirAll(mocks.Runtime.TemplateRoot, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		handler.shims.Stat = os.Stat
		handler.shims.ReadDir = os.ReadDir
		handler.shims.ReadFile = os.ReadFile
		handler.shims.YamlUnmarshal = yaml.Unmarshal

		expectedError := fmt.Errorf("get context values error")
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return nil, expectedError
		}

		_, err = handler.GetLocalTemplateData()

		if err == nil {
			t.Fatal("Expected error when GetContextValues fails")
		}
		if !strings.Contains(err.Error(), "failed to load context values") {
			t.Errorf("Expected error about loading context values, got: %v", err)
		}
	})

	t.Run("MergesSubstitutionValuesWithExisting", func(t *testing.T) {
		t.Skip("Substitutions are now in Features, not in context values or files")
	})

	t.Run("HandlesSubstitutionUnmarshalErrorGracefully", func(t *testing.T) {
		t.Skip("Substitutions are now in Features, not in context values or files")
	})

	t.Run("IgnoresNonYAMLFilesInFeatures", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		contextName := "test-context"
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return contextName
		}

		projectRoot = os.Getenv("WINDSOR_PROJECT_ROOT")
		templateDir := filepath.Join(projectRoot, "contexts", "_template")
		featuresDir := filepath.Join(templateDir, "features")
		contextDir := filepath.Join(projectRoot, "contexts", contextName)

		if err := os.MkdirAll(featuresDir, 0755); err != nil {
			t.Fatalf("Failed to create features directory: %v", err)
		}
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		validFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: valid-feature
`)
		if err := os.WriteFile(filepath.Join(featuresDir, "valid.yaml"), validFeature, 0644); err != nil {
			t.Fatalf("Failed to write valid feature: %v", err)
		}

		if err := os.WriteFile(filepath.Join(featuresDir, "README.md"), []byte("# Features"), 0644); err != nil {
			t.Fatalf("Failed to write README: %v", err)
		}

		if err := os.WriteFile(filepath.Join(featuresDir, "config.json"), []byte("{}"), 0644); err != nil {
			t.Fatalf("Failed to write JSON: %v", err)
		}

		templateData, err := handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if _, exists := templateData["_template/features/valid.yaml"]; !exists {
			t.Error("Expected features/valid.yaml to be collected")
		}

		// All files from _template are now collected, including README.md and config.json
	})

	t.Run("ComposesFeaturesByEvaluatingConditions", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		contextName := "test-context"
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return contextName
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"provider": "aws",
				"observability": map[string]any{
					"enabled": true,
				},
			}, nil
		}

		templateDir := filepath.Join(projectRoot, "contexts", "_template")
		featuresDir := filepath.Join(templateDir, "features")
		contextDir := filepath.Join(projectRoot, "contexts", contextName)

		if err := os.MkdirAll(featuresDir, 0755); err != nil {
			t.Fatalf("Failed to create features directory: %v", err)
		}
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)
		if err := os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), baseBlueprint, 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		awsFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
when: provider == "aws"
terraform:
  - path: network/vpc
    values:
      cidr: 10.0.0.0/16
`)
		if err := os.WriteFile(filepath.Join(featuresDir, "aws.yaml"), awsFeature, 0644); err != nil {
			t.Fatalf("Failed to write aws feature: %v", err)
		}

		observabilityFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: observability
when: observability.enabled == true
terraform:
  - path: observability/stack
`)
		if err := os.WriteFile(filepath.Join(featuresDir, "observability.yaml"), observabilityFeature, 0644); err != nil {
			t.Fatalf("Failed to write observability feature: %v", err)
		}

		gcpFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: gcp-feature
when: provider == "gcp"
terraform:
  - path: gcp/network
`)
		if err := os.WriteFile(filepath.Join(featuresDir, "gcp.yaml"), gcpFeature, 0644); err != nil {
			t.Fatalf("Failed to write gcp feature: %v", err)
		}

		templateData, err := handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		composedBlueprint, exists := templateData["_template/blueprint.yaml"]
		if !exists {
			t.Fatal("Expected composed blueprint in templateData")
		}

		if !strings.Contains(string(composedBlueprint), "network/vpc") {
			t.Error("Expected AWS VPC component to be merged")
		}
		if !strings.Contains(string(composedBlueprint), "observability/stack") {
			t.Error("Expected observability component to be merged")
		}
		if strings.Contains(string(composedBlueprint), "gcp/network") {
			t.Error("Did not expect GCP component to be merged (condition not met)")
		}
		if !strings.Contains(string(composedBlueprint), contextName) {
			t.Errorf("Expected blueprint metadata to include context name '%s'", contextName)
		}
	})

	t.Run("SetsMetadataFromContextName", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		contextName := "production"
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return contextName
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		templateDir := filepath.Join(projectRoot, "contexts", "_template")
		contextDir := filepath.Join(projectRoot, "contexts", contextName)

		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
terraform:
  - path: base/module
`)
		if err := os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), baseBlueprint, 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		templateData, err := handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		composedBlueprint, exists := templateData["_template/blueprint.yaml"]
		if !exists {
			t.Fatal("Expected composed blueprint in templateData")
		}

		var blueprint blueprintv1alpha1.Blueprint
		if err := yaml.Unmarshal(composedBlueprint, &blueprint); err != nil {
			t.Fatalf("Failed to unmarshal blueprint: %v", err)
		}

		if blueprint.Metadata.Name != contextName {
			t.Errorf("Expected metadata.name = '%s', got '%s'", contextName, blueprint.Metadata.Name)
		}
		expectedDesc := fmt.Sprintf("Blueprint for %s context", contextName)
		if blueprint.Metadata.Description != expectedDesc {
			t.Errorf("Expected metadata.description = '%s', got '%s'", expectedDesc, blueprint.Metadata.Description)
		}
	})

	t.Run("ReturnsNilWhenNoTemplateDirectory", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		templateData, err := handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if templateData != nil {
			t.Errorf("Expected nil templateData, got %v", templateData)
		}
	})

	t.Run("HandlesEmptyBlueprintWithOnlyFeatures", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		contextName := "test-context"
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return contextName
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"feature": "enabled",
			}, nil
		}

		templateDir := filepath.Join(projectRoot, "contexts", "_template")
		featuresDir := filepath.Join(templateDir, "features")
		contextDir := filepath.Join(projectRoot, "contexts", contextName)

		if err := os.MkdirAll(featuresDir, 0755); err != nil {
			t.Fatalf("Failed to create features directory: %v", err)
		}
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		feature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-feature
when: feature == "enabled"
terraform:
  - path: test/module
`)
		if err := os.WriteFile(filepath.Join(featuresDir, "test.yaml"), feature, 0644); err != nil {
			t.Fatalf("Failed to write feature: %v", err)
		}

		templateData, err := handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		composedBlueprint, exists := templateData["_template/blueprint.yaml"]
		if !exists {
			t.Fatal("Expected composed blueprint in templateData")
		}

		if !strings.Contains(string(composedBlueprint), "test/module") {
			t.Error("Expected feature component to be in blueprint")
		}
	})

	t.Run("HandlesKustomizationsInFeatures", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		contextName := "test-context"
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return contextName
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"gitops": map[string]any{
					"enabled": true,
				},
			}, nil
		}

		templateDir := filepath.Join(projectRoot, "contexts", "_template")
		featuresDir := filepath.Join(templateDir, "features")
		contextDir := filepath.Join(projectRoot, "contexts", contextName)

		if err := os.MkdirAll(featuresDir, 0755); err != nil {
			t.Fatalf("Failed to create features directory: %v", err)
		}
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)
		if err := os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), baseBlueprint, 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		fluxFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: flux
when: gitops.enabled == true
kustomize:
  - name: flux-system
    path: gitops/flux
`)
		if err := os.WriteFile(filepath.Join(featuresDir, "flux.yaml"), fluxFeature, 0644); err != nil {
			t.Fatalf("Failed to write flux feature: %v", err)
		}

		templateData, err := handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		composedBlueprint, exists := templateData["_template/blueprint.yaml"]
		if !exists {
			t.Fatal("Expected composed blueprint in templateData")
		}

		if !strings.Contains(string(composedBlueprint), "flux-system") {
			t.Error("Expected kustomization to be in blueprint")
		}
		if !strings.Contains(string(composedBlueprint), "gitops/flux") {
			t.Error("Expected kustomization path to be in blueprint")
		}
	})

	t.Run("SkipsComposedBlueprintWhenEmpty", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}

		contextName := "test-context"
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return contextName
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		templateDir := filepath.Join(projectRoot, "contexts", "_template")
		contextDir := filepath.Join(projectRoot, "contexts", contextName)

		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: empty-base
`)
		if err := os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), baseBlueprint, 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		templateData, err := handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if composedBlueprint, exists := templateData["_template/blueprint.yaml"]; exists {
			var blueprint blueprintv1alpha1.Blueprint
			if err := yaml.Unmarshal(composedBlueprint, &blueprint); err != nil {
				t.Fatalf("Failed to unmarshal blueprint: %v", err)
			}
			if blueprint.Metadata.Name != contextName {
				t.Errorf("Expected metadata.name to be set to context name '%s' even when blueprint is empty, got '%s'", contextName, blueprint.Metadata.Name)
			}
			expectedDesc := fmt.Sprintf("Blueprint for %s context", contextName)
			if blueprint.Metadata.Description != expectedDesc {
				t.Errorf("Expected metadata.description to be '%s', got '%s'", expectedDesc, blueprint.Metadata.Description)
			}
		} else {
			t.Error("Expected blueprint to be generated even when empty if context name is set")
		}
	})

	t.Run("ValidatesCliVersionFromMetadataYaml", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}

		contextName := "test-context"
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return contextName
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		templateDir := filepath.Join(projectRoot, "contexts", "_template")

		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		metadataContent := []byte(`name: test-blueprint
version: 1.0.0
cliVersion: ">=1.0.0"
`)
		if err := os.WriteFile(filepath.Join(templateDir, "metadata.yaml"), metadataContent, 0644); err != nil {
			t.Fatalf("Failed to write metadata.yaml: %v", err)
		}

		blueprintContent := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base-blueprint
`)
		if err := os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), blueprintContent, 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		_, err = handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error when cliVersion is empty (validation skipped), got %v", err)
		}
	})

	t.Run("SkipsValidationWhenMetadataYamlDoesNotExist", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}

		contextName := "test-context"
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return contextName
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		templateDir := filepath.Join(projectRoot, "contexts", "_template")

		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		blueprintContent := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base-blueprint
`)
		if err := os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), blueprintContent, 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		templateData, err := handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error when metadata.yaml doesn't exist, got %v", err)
		}

		if templateData == nil {
			t.Fatal("Expected template data, got nil")
		}
	})

	t.Run("ReturnsErrorWhenMetadataYamlCannotBeRead", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}

		contextName := "test-context"
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return contextName
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		templateDir := filepath.Join(projectRoot, "contexts", "_template")

		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "metadata.yaml") {
				return &mockFileInfo{name: "metadata.yaml", isDir: false}, nil
			}
			return os.Stat(name)
		}

		handler.shims.ReadFile = func(path string) ([]byte, error) {
			if strings.Contains(path, "metadata.yaml") {
				return nil, fmt.Errorf("read error")
			}
			return os.ReadFile(path)
		}

		_, err = handler.GetLocalTemplateData()

		if err == nil {
			t.Fatal("Expected error when metadata.yaml cannot be read")
		}
		if !strings.Contains(err.Error(), "failed to read metadata.yaml") {
			t.Errorf("Expected error to contain 'failed to read metadata.yaml', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenMetadataYamlCannotBeParsed", func(t *testing.T) {
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = projectRoot

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}

		contextName := "test-context"
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextFunc = func() string {
			return contextName
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		templateDir := filepath.Join(projectRoot, "contexts", "_template")

		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		invalidMetadata := []byte(`name: test-blueprint
version: 1.0.0
invalid: yaml: content
`)
		if err := os.WriteFile(filepath.Join(templateDir, "metadata.yaml"), invalidMetadata, 0644); err != nil {
			t.Fatalf("Failed to write metadata.yaml: %v", err)
		}

		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			if strings.Contains(string(data), "invalid: yaml: content") {
				return fmt.Errorf("yaml parse error")
			}
			return yaml.Unmarshal(data, v)
		}

		_, err = handler.GetLocalTemplateData()

		if err == nil {
			t.Fatal("Expected error when metadata.yaml cannot be parsed")
		}
		if !strings.Contains(err.Error(), "failed to parse metadata.yaml") {
			t.Errorf("Expected error to contain 'failed to parse metadata.yaml', got: %v", err)
		}
	})

	t.Run("HandlesMetadataYamlReadFileError", func(t *testing.T) {
		tmpDir := t.TempDir()
		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}

		templateDir := mocks.Runtime.TemplateRoot
		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		metadataPath := filepath.Join(templateDir, "metadata.yaml")
		if err := os.WriteFile(metadataPath, []byte("name: test\n"), 0644); err != nil {
			t.Fatalf("Failed to write metadata.yaml: %v", err)
		}

		expectedError := fmt.Errorf("read file error")
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == metadataPath {
				return nil, expectedError
			}
			return os.ReadFile(name)
		}

		_, err = handler.GetLocalTemplateData()

		if err == nil {
			t.Fatal("Expected error when metadata.yaml cannot be read")
		}
		if !strings.Contains(err.Error(), "failed to read template file") && !strings.Contains(err.Error(), "failed to read metadata.yaml") {
			t.Errorf("Expected error to contain 'failed to read template file' or 'failed to read metadata.yaml', got: %v", err)
		}
	})

	t.Run("HandlesLoadSchemaFromBytesError", func(t *testing.T) {
		tmpDir := t.TempDir()
		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}

		templateDir := mocks.Runtime.TemplateRoot
		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		schemaPath := filepath.Join(templateDir, "schema.yaml")
		if err := os.WriteFile(schemaPath, []byte("invalid schema"), 0644); err != nil {
			t.Fatalf("Failed to write schema: %v", err)
		}

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		expectedError := fmt.Errorf("schema load error")
		mockConfigHandler.LoadSchemaFromBytesFunc = func(data []byte) error {
			return expectedError
		}

		_, err = handler.GetLocalTemplateData()

		if err == nil {
			t.Fatal("Expected error when schema cannot be loaded")
		}
		if !strings.Contains(err.Error(), "failed to load schema") {
			t.Errorf("Expected error to contain 'failed to load schema', got: %v", err)
		}
	})

	t.Run("HandlesGetContextValuesError", func(t *testing.T) {
		tmpDir := t.TempDir()
		mocks := setupBlueprintMocks(t)
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")

		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}

		templateDir := mocks.Runtime.TemplateRoot
		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		expectedError := fmt.Errorf("context values error")
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return nil, expectedError
		}

		_, err = handler.GetLocalTemplateData()

		if err == nil {
			t.Fatal("Expected error when context values cannot be loaded")
		}
		if !strings.Contains(err.Error(), "failed to load context values") {
			t.Errorf("Expected error to contain 'failed to load context values', got: %v", err)
		}
	})

	t.Run("HandlesSchemaFromTemplateSchemaYaml", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		if err := os.MkdirAll(mocks.Runtime.TemplateRoot, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		schemaContent := []byte("schema: test")
		if err := os.WriteFile(filepath.Join(mocks.Runtime.TemplateRoot, "schema.yaml"), schemaContent, 0644); err != nil {
			t.Fatalf("Failed to write schema.yaml: %v", err)
		}

		handler.shims.Stat = os.Stat
		handler.shims.ReadDir = os.ReadDir
		handler.shims.ReadFile = os.ReadFile
		handler.shims.YamlUnmarshal = yaml.Unmarshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).LoadSchemaFromBytesFunc = func(data []byte) error {
			return nil
		}

		_, err = handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("HandlesEmptyContextName", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		if err := os.MkdirAll(mocks.Runtime.TemplateRoot, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		blueprintContent := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base-blueprint
terraform:
  - path: test/component
`)
		if err := os.WriteFile(filepath.Join(mocks.Runtime.TemplateRoot, "blueprint.yaml"), blueprintContent, 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		handler.shims.Stat = os.Stat
		handler.shims.ReadDir = os.ReadDir
		handler.shims.ReadFile = os.ReadFile
		handler.shims.YamlUnmarshal = yaml.Unmarshal
		handler.shims.YamlMarshal = yaml.Marshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return ""
		}

		_, err = handler.GetLocalTemplateData()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("HandlesSubstitutionProcessing", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		if err := os.MkdirAll(mocks.Runtime.TemplateRoot, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		handler.shims.Stat = os.Stat
		handler.shims.ReadDir = os.ReadDir
		handler.shims.ReadFile = os.ReadFile
		handler.shims.YamlUnmarshal = yaml.Unmarshal
		handler.shims.YamlMarshal = yaml.Marshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitutions": map[string]any{
					"common": map[string]any{
						"key1": "value1",
						"key2": "value2",
					},
					"kustomization1": map[string]any{
						"key3": "value3",
					},
				},
			}, nil
		}

		_, err = handler.GetLocalTemplateData()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(handler.commonSubstitutions) == 0 {
			t.Error("Expected common substitutions to be processed")
		}
		if len(handler.featureSubstitutions) == 0 {
			t.Error("Expected feature substitutions to be processed")
		}
	})

	t.Run("HandlesSubstitutionUnmarshalError", func(t *testing.T) {
		t.Skip("Substitutions are now only in values.yaml (contextValues) or Features, not in files")
	})

	t.Run("HandlesSubstitutionMarshalError", func(t *testing.T) {
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.TemplateRoot = filepath.Join(tmpDir, "contexts", "_template")
		mocks.Runtime.ConfigRoot = tmpDir

		if err := os.MkdirAll(mocks.Runtime.TemplateRoot, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		handler.shims.Stat = os.Stat
		handler.shims.ReadDir = os.ReadDir
		handler.shims.ReadFile = os.ReadFile
		handler.shims.YamlUnmarshal = yaml.Unmarshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitutions": map[string]any{
					"common": map[string]any{
						"key1": "value1",
					},
				},
			}, nil
		}

		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			if _, ok := v.(map[string]any); ok {
				return nil, fmt.Errorf("marshal error")
			}
			return yaml.Marshal(v)
		}

		_, err = handler.GetLocalTemplateData()

		if err == nil {
			t.Fatal("Expected error when YamlMarshal fails for substitutions")
		}
		if !strings.Contains(err.Error(), "failed to marshal substitution values") {
			t.Errorf("Expected error about marshaling substitutions, got: %v", err)
		}
	})
}

func TestBaseBlueprintHandler_Generate(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *BlueprintTestMocks) {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("EmptyBlueprint", func(t *testing.T) {
		// Given a handler with empty blueprint
		handler, _ := setup(t)
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
		}

		// When generating blueprint
		generated := handler.Generate()

		// Then should return a copy of the blueprint
		if generated == nil {
			t.Fatal("Expected non-nil generated blueprint")
		}
		if generated.Metadata.Name != "test-blueprint" {
			t.Errorf("Expected name 'test-blueprint', got %s", generated.Metadata.Name)
		}
	})

	t.Run("KustomizationsWithDefaults", func(t *testing.T) {
		// Given a handler with kustomizations that need defaults
		handler, _ := setup(t)
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					// No Source, Path, or other fields set
				},
			},
		}

		// When generating blueprint
		generated := handler.Generate()

		// Then kustomizations should have defaults applied
		if len(generated.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(generated.Kustomizations))
		}

		kustomization := generated.Kustomizations[0]
		if kustomization.Name != "test-kustomization" {
			t.Errorf("Expected name 'test-kustomization', got %s", kustomization.Name)
		}
		if kustomization.Source != "test-blueprint" {
			t.Errorf("Expected source 'test-blueprint', got %s", kustomization.Source)
		}
		if kustomization.Path != "kustomize" {
			t.Errorf("Expected path 'kustomize', got %s", kustomization.Path)
		}
		if kustomization.Interval == nil || kustomization.Interval.Duration != constants.DefaultFluxKustomizationInterval {
			t.Errorf("Expected default interval, got %v", kustomization.Interval)
		}
		if kustomization.RetryInterval == nil || kustomization.RetryInterval.Duration != constants.DefaultFluxKustomizationRetryInterval {
			t.Errorf("Expected default retry interval, got %v", kustomization.RetryInterval)
		}
		if kustomization.Timeout == nil || kustomization.Timeout.Duration != constants.DefaultFluxKustomizationTimeout {
			t.Errorf("Expected default timeout, got %v", kustomization.Timeout)
		}
		if kustomization.Wait == nil || *kustomization.Wait != constants.DefaultFluxKustomizationWait {
			t.Errorf("Expected default wait, got %v", kustomization.Wait)
		}
		if kustomization.Force == nil || *kustomization.Force != constants.DefaultFluxKustomizationForce {
			t.Errorf("Expected default force, got %v", kustomization.Force)
		}
		if kustomization.Destroy == nil || *kustomization.Destroy != true {
			t.Errorf("Expected default destroy true, got %v", kustomization.Destroy)
		}
	})

	t.Run("KustomizationsWithCustomPath", func(t *testing.T) {
		// Given a handler with kustomization with custom path
		handler, _ := setup(t)
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Path: "custom/path",
				},
			},
		}

		// When generating blueprint
		generated := handler.Generate()

		// Then path should be prefixed with kustomize/
		if len(generated.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(generated.Kustomizations))
		}

		expectedPath := "kustomize/custom/path"
		if generated.Kustomizations[0].Path != expectedPath {
			t.Errorf("Expected path '%s', got '%s'", expectedPath, generated.Kustomizations[0].Path)
		}
	})

	t.Run("KustomizationsWithBackslashes", func(t *testing.T) {
		// Given a handler with kustomization with backslashes in path
		handler, _ := setup(t)
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Path: "custom\\path\\with\\backslashes",
				},
			},
		}

		// When generating blueprint
		generated := handler.Generate()

		// Then backslashes should be replaced with forward slashes
		if len(generated.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(generated.Kustomizations))
		}

		expectedPath := "kustomize/custom/path/with/backslashes"
		if generated.Kustomizations[0].Path != expectedPath {
			t.Errorf("Expected path '%s', got '%s'", expectedPath, generated.Kustomizations[0].Path)
		}
	})

	t.Run("TerraformComponentsWithSourceResolution", func(t *testing.T) {
		// Given a handler with terraform components
		handler, _ := setup(t)
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "test-source",
					Url:  "https://github.com/example/terraform-modules",
				},
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Source: "test-source",
					Path:   "modules/example",
				},
			},
		}

		// When generating blueprint
		generated := handler.Generate()

		// Then terraform components should be processed
		if len(generated.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(generated.TerraformComponents))
		}

		component := generated.TerraformComponents[0]
		expectedSource := "https://github.com/example/terraform-modules//terraform/modules/example?ref="
		if component.Source != expectedSource {
			t.Errorf("Expected source '%s', got %s", expectedSource, component.Source)
		}
		if component.Path != "modules/example" {
			t.Errorf("Expected path 'modules/example', got %s", component.Path)
		}
	})

	t.Run("PreservesOriginalBlueprint", func(t *testing.T) {
		// Given a handler with blueprint data
		handler, _ := setup(t)
		originalBlueprint := blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}
		handler.blueprint = originalBlueprint

		// When generating blueprint
		generated := handler.Generate()

		// Then original blueprint should be unchanged
		if handler.blueprint.Kustomizations[0].Source != "" {
			t.Error("Original blueprint should not be modified")
		}

		// And generated blueprint should have defaults
		if generated.Kustomizations[0].Source != "test-blueprint" {
			t.Error("Generated blueprint should have defaults applied")
		}
	})

	t.Run("WithCommonSubstitutions", func(t *testing.T) {
		handler, _ := setup(t)
		handler.commonSubstitutions = map[string]string{
			"domain": "example.com",
			"region": "us-west-2",
		}
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
		}

		generated := handler.Generate()

		if generated.ConfigMaps == nil {
			t.Fatal("Expected ConfigMaps to be set")
		}
		if len(generated.ConfigMaps) != 1 {
			t.Fatalf("Expected 1 ConfigMap, got %d", len(generated.ConfigMaps))
		}
		commonConfigMap, exists := generated.ConfigMaps["values-common"]
		if !exists {
			t.Fatal("Expected values-common ConfigMap")
		}
		if commonConfigMap["domain"] != "example.com" {
			t.Errorf("Expected domain 'example.com', got '%s'", commonConfigMap["domain"])
		}
		if commonConfigMap["region"] != "us-west-2" {
			t.Errorf("Expected region 'us-west-2', got '%s'", commonConfigMap["region"])
		}
	})

	t.Run("WithLegacySpecialVariables", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "test.example.com"
			case "id":
				return "test-id-123"
			case "network.loadbalancer_ips.start":
				return "192.168.1.1"
			case "network.loadbalancer_ips.end":
				return "192.168.1.100"
			case "docker.registry_url":
				return "registry.example.com"
			default:
				return ""
			}
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{"/host:/var/local"}
			}
			return []string{}
		}

		buildIDPath := filepath.Join(mocks.Runtime.ProjectRoot, ".windsor", ".build-id")
		if err := os.MkdirAll(filepath.Dir(buildIDPath), 0755); err != nil {
			t.Fatalf("Failed to create .windsor directory: %v", err)
		}
		if err := os.WriteFile(buildIDPath, []byte("build-123"), 0644); err != nil {
			t.Fatalf("Failed to write build ID file: %v", err)
		}

		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
		}

		generated := handler.Generate()

		if generated.ConfigMaps == nil {
			t.Fatal("Expected ConfigMaps to be set")
		}
		commonConfigMap, exists := generated.ConfigMaps["values-common"]
		if !exists {
			t.Fatal("Expected values-common ConfigMap")
		}
		if commonConfigMap["DOMAIN"] != "test.example.com" {
			t.Errorf("Expected DOMAIN 'test.example.com', got '%s'", commonConfigMap["DOMAIN"])
		}
		if commonConfigMap["CONTEXT"] != "test-context" {
			t.Errorf("Expected CONTEXT 'test-context', got '%s'", commonConfigMap["CONTEXT"])
		}
		if commonConfigMap["CONTEXT_ID"] != "test-id-123" {
			t.Errorf("Expected CONTEXT_ID 'test-id-123', got '%s'", commonConfigMap["CONTEXT_ID"])
		}
		if commonConfigMap["LOADBALANCER_IP_RANGE"] != "192.168.1.1-192.168.1.100" {
			t.Errorf("Expected LOADBALANCER_IP_RANGE '192.168.1.1-192.168.1.100', got '%s'", commonConfigMap["LOADBALANCER_IP_RANGE"])
		}
		if commonConfigMap["REGISTRY_URL"] != "registry.example.com" {
			t.Errorf("Expected REGISTRY_URL 'registry.example.com', got '%s'", commonConfigMap["REGISTRY_URL"])
		}
		if commonConfigMap["LOCAL_VOLUME_PATH"] != "/var/local" {
			t.Errorf("Expected LOCAL_VOLUME_PATH '/var/local', got '%s'", commonConfigMap["LOCAL_VOLUME_PATH"])
		}
		if commonConfigMap["BUILD_ID"] != "build-123" {
			t.Errorf("Expected BUILD_ID 'build-123', got '%s'", commonConfigMap["BUILD_ID"])
		}
	})

	t.Run("DoesNotWritePatchesDuringGeneration", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"
		mocks.Runtime.TemplateRoot = "/test/template"

		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Patches: []blueprintv1alpha1.BlueprintPatch{
						{
							Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: value`,
						},
					},
				},
			},
		}

		var writeFileCalls []string
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			normalizedName := filepath.ToSlash(name)
			if strings.Contains(normalizedName, "patches/") {
				writeFileCalls = append(writeFileCalls, name)
			}
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}

		generated := handler.Generate()

		if generated == nil {
			t.Fatal("Expected non-nil generated blueprint")
		}

		if len(writeFileCalls) > 0 {
			t.Errorf("Expected no patch files to be written during Generate(), but got %d writes: %v", len(writeFileCalls), writeFileCalls)
		}

		if len(generated.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(generated.Kustomizations))
		}

		if len(generated.Kustomizations[0].Patches) == 0 {
			t.Error("Expected patches to be preserved in generated blueprint")
		}
	})

	t.Run("DiscoversPatchesFromDisk", func(t *testing.T) {
		handler, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ConfigRoot = filepath.Join(tmpDir, "contexts", "test-context")
		patchesDir := filepath.Join(mocks.Runtime.ConfigRoot, "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("Failed to create patches directory: %v", err)
		}

		patchContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: discovered-config
data:
  key: value`
		patchFile := filepath.Join(patchesDir, "configmap-discovered-config.yaml")
		if err := os.WriteFile(patchFile, []byte(patchContent), 0644); err != nil {
			t.Fatalf("Failed to write patch file: %v", err)
		}

		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			normalizedName := filepath.ToSlash(name)
			if strings.Contains(normalizedName, "patches/test-kustomization") {
				return &mockFileInfo{name: "test-kustomization", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.Walk = filepath.Walk
		mocks.Shims.ReadFile = os.ReadFile

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}

		generated := handler.Generate()

		if generated == nil {
			t.Fatal("Expected non-nil generated blueprint")
		}

		if len(generated.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(generated.Kustomizations))
		}

		if len(generated.Kustomizations[0].Patches) == 0 {
			t.Error("Expected discovered patches to be added to kustomization")
		}

		found := false
		for _, patch := range generated.Kustomizations[0].Patches {
			if strings.Contains(patch.Patch, "discovered-config") {
				found = true
				if patch.Path != "" {
					t.Error("Expected discovered patch Path to be cleared")
				}
				break
			}
		}
		if !found {
			t.Error("Expected to find discovered patch in generated blueprint")
		}
	})

	t.Run("AppliesFeatureSubstitutions", func(t *testing.T) {
		handler, _ := setup(t)
		handler.featureSubstitutions = map[string]map[string]string{
			"test-kustomization": {
				"domain": "example.com",
				"region": "us-west-2",
			},
		}

		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		generated := handler.Generate()

		if generated == nil {
			t.Fatal("Expected non-nil generated blueprint")
		}

		if len(generated.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(generated.Kustomizations))
		}

		subs := generated.Kustomizations[0].Substitutions
		if subs == nil {
			t.Fatal("Expected substitutions to be set")
		}

		if subs["domain"] != "example.com" {
			t.Errorf("Expected domain 'example.com', got '%s'", subs["domain"])
		}

		if subs["region"] != "us-west-2" {
			t.Errorf("Expected region 'us-west-2', got '%s'", subs["region"])
		}
	})

	t.Run("DiscoversJSON6902Patches", func(t *testing.T) {
		handler, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ConfigRoot = filepath.Join(tmpDir, "contexts", "test-context")
		patchesDir := filepath.Join(mocks.Runtime.ConfigRoot, "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("Failed to create patches directory: %v", err)
		}

		json6902PatchContent := `apiVersion: v1
kind: Deployment
metadata:
  name: test-deployment
  namespace: default
patch:
- op: replace
  path: /spec/replicas
  value: 5`
		patchFile := filepath.Join(patchesDir, "deployment-patch.yaml")
		if err := os.WriteFile(patchFile, []byte(json6902PatchContent), 0644); err != nil {
			t.Fatalf("Failed to write patch file: %v", err)
		}

		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			normalizedName := filepath.ToSlash(name)
			if strings.Contains(normalizedName, "patches/test-kustomization") {
				return &mockFileInfo{name: "test-kustomization", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.Walk = filepath.Walk
		mocks.Shims.ReadFile = os.ReadFile

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}

		generated := handler.Generate()

		if generated == nil {
			t.Fatal("Expected non-nil generated blueprint")
		}

		if len(generated.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(generated.Kustomizations))
		}

		found := false
		for _, patch := range generated.Kustomizations[0].Patches {
			if patch.Target != nil {
				found = true
				if patch.Target.Kind != "Deployment" {
					t.Errorf("Expected target kind 'Deployment', got '%s'", patch.Target.Kind)
				}
				if patch.Target.Name != "test-deployment" {
					t.Errorf("Expected target name 'test-deployment', got '%s'", patch.Target.Name)
				}
				break
			}
		}
		if !found {
			t.Error("Expected to find JSON6902 patch with target in generated blueprint")
		}
	})

	t.Run("HandlesPatchDiscoveryErrors", func(t *testing.T) {
		handler, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ConfigRoot = filepath.Join(tmpDir, "contexts", "test-context")
		patchesDir := filepath.Join(mocks.Runtime.ConfigRoot, "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("Failed to create patches directory: %v", err)
		}

		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		walkError := fmt.Errorf("walk error")
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			normalizedName := filepath.ToSlash(name)
			if strings.Contains(normalizedName, "patches/test-kustomization") {
				return &mockFileInfo{name: "patches", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			return walkError
		}

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}

		generated := handler.Generate()

		if generated == nil {
			t.Fatal("Expected non-nil generated blueprint")
		}

		if len(generated.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(generated.Kustomizations))
		}

		if len(generated.Kustomizations[0].Patches) != 0 {
			t.Error("Expected no patches when Walk returns error")
		}
	})

	t.Run("SkipsNonYamlFilesInPatchDiscovery", func(t *testing.T) {
		handler, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ConfigRoot = filepath.Join(tmpDir, "contexts", "test-context")
		patchesDir := filepath.Join(mocks.Runtime.ConfigRoot, "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("Failed to create patches directory: %v", err)
		}

		patchContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config`
		if err := os.WriteFile(filepath.Join(patchesDir, "configmap-test-config.yaml"), []byte(patchContent), 0644); err != nil {
			t.Fatalf("Failed to write patch file: %v", err)
		}

		if err := os.WriteFile(filepath.Join(patchesDir, "not-a-patch.txt"), []byte("not yaml"), 0644); err != nil {
			t.Fatalf("Failed to write non-patch file: %v", err)
		}

		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			normalizedName := filepath.ToSlash(name)
			if strings.Contains(normalizedName, "patches/test-kustomization") {
				return &mockFileInfo{name: "test-kustomization", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.Walk = filepath.Walk
		mocks.Shims.ReadFile = os.ReadFile

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}

		generated := handler.Generate()

		if generated == nil {
			t.Fatal("Expected non-nil generated blueprint")
		}

		if len(generated.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(generated.Kustomizations))
		}

		if len(generated.Kustomizations[0].Patches) != 1 {
			t.Errorf("Expected 1 discovered patch (yaml file only), got %d", len(generated.Kustomizations[0].Patches))
		}
	})

	t.Run("HandlesEmptyConfigRoot", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = ""

		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		generated := handler.Generate()

		if generated == nil {
			t.Fatal("Expected non-nil generated blueprint")
		}

		if len(generated.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(generated.Kustomizations))
		}

		if len(generated.Kustomizations[0].Patches) != 0 {
			t.Error("Expected no patches when ConfigRoot is empty")
		}
	})
}
