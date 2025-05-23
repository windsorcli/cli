package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
)

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

func TestBaseBlueprintHandler_deleteKustomization(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		baseHandler := &BaseBlueprintHandler{}
		os.Setenv("KUBECONFIG", "test-kubeconfig")
		name := "test-kustomization"
		namespace := "test-namespace"

		var called bool
		var gotKubeconfig string
		var gotReq KubeRequestConfig
		origKubeClient := kubeClient
		kubeClient = func(kubeconfig string, req KubeRequestConfig) error {
			called = true
			gotKubeconfig = kubeconfig
			gotReq = req
			return nil
		}
		defer func() { kubeClient = origKubeClient }()

		err := baseHandler.deleteKustomization(name, namespace)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !called {
			t.Fatal("expected kubeClient to be called")
		}
		if gotKubeconfig != "test-kubeconfig" {
			t.Errorf("expected kubeconfig 'test-kubeconfig', got '%s'", gotKubeconfig)
		}
		if gotReq.Namespace != namespace {
			t.Errorf("expected namespace '%s', got '%s'", namespace, gotReq.Namespace)
		}
		if gotReq.Name != name {
			t.Errorf("expected resource name '%s', got '%s'", name, gotReq.Name)
		}
		if gotReq.Resource != "kustomizations" {
			t.Errorf("expected resource type 'kustomizations', got '%s'", gotReq.Resource)
		}
		if gotReq.Method != "DELETE" {
			t.Errorf("expected method 'DELETE', got '%s'", gotReq.Method)
		}
	})

	t.Run("DeleteError", func(t *testing.T) {
		baseHandler := &BaseBlueprintHandler{}
		os.Setenv("KUBECONFIG", "test-kubeconfig")
		name := "test-kustomization"
		namespace := "test-namespace"

		expectedErr := fmt.Errorf("delete error")
		origKubeClient := kubeClient
		kubeClient = func(kubeconfig string, req KubeRequestConfig) error {
			return expectedErr
		}
		defer func() { kubeClient = origKubeClient }()

		err := baseHandler.deleteKustomization(name, namespace)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

func TestBaseBlueprintHandler_applyConfigMap(t *testing.T) {
	t.Run("Error", func(t *testing.T) {
		baseHandler := &BaseBlueprintHandler{}
		os.Setenv("KUBECONFIG", "test-kubeconfig")

		mockConfigHandler := &config.MockConfigHandler{
			GetStringFunc:      func(key string, _ ...string) string { return "foo" },
			GetStringSliceFunc: func(key string, _ ...[]string) []string { return []string{"/tmp:/var/local"} },
			GetContextFunc:     func() string { return "mock-context" },
		}
		baseHandler.configHandler = mockConfigHandler

		errExpected := fmt.Errorf("apply configmap error")
		origKubeClientResourceOperation := kubeClientResourceOperation
		kubeClientResourceOperation = func(_ string, _ ResourceOperationConfig) error {
			return errExpected
		}
		defer func() { kubeClientResourceOperation = origKubeClientResourceOperation }()

		err := baseHandler.applyConfigMap()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err != errExpected {
			t.Errorf("expected error %v, got %v", errExpected, err)
		}
	})
}

// mockConfigHandler implements config.ConfigHandler for testing
// Only the methods used in applyConfigMap are implemented

type mockConfigHandler struct {
	getString      func(string) string
	getStringSlice func(string) []string
	getContext     func() string
}

func (m *mockConfigHandler) GetString(key string) string {
	return m.getString(key)
}
func (m *mockConfigHandler) GetStringSlice(key string) []string {
	return m.getStringSlice(key)
}
func (m *mockConfigHandler) GetContext() string {
	return m.getContext()
}
