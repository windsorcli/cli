package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kustomize "github.com/fluxcd/pkg/apis/kustomize"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mockFileInfo implements os.FileInfo for testing
type mockFileInfo struct {
	name string
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return 0 }
func (m mockFileInfo) Mode() os.FileMode  { return 0644 }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return false }
func (m mockFileInfo) Sys() interface{}   { return nil }

// =============================================================================
// Test Private Methods
// =============================================================================

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

func TestBlueprintHandler_resolveComponentSources(t *testing.T) {
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
		// Given a blueprint handler with repository and sources
		handler, _ := setup(t)

		// Mock Kubernetes manager
		mockK8sManager := &kubernetes.MockKubernetesManager{}
		mockK8sManager.ApplyGitRepositoryFunc = func(repo *sourcev1.GitRepository) error {
			return nil
		}
		handler.(*BaseBlueprintHandler).kubernetesManager = mockK8sManager

		// Set repository and sources directly on the blueprint
		baseHandler := handler.(*BaseBlueprintHandler)
		baseHandler.blueprint.Repository = blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}

		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "git::https://example.com/source1.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		baseHandler.blueprint.Sources = expectedSources

		// When resolving component sources
		handler.(*BaseBlueprintHandler).resolveComponentSources(&handler.(*BaseBlueprintHandler).blueprint)
	})

	t.Run("SourceURLWithoutDotGit", func(t *testing.T) {
		// Given a blueprint handler with repository and source without .git suffix
		handler, _ := setup(t)

		// Mock Kubernetes manager
		mockK8sManager := &kubernetes.MockKubernetesManager{}
		mockK8sManager.ApplyGitRepositoryFunc = func(repo *sourcev1.GitRepository) error {
			return nil
		}
		handler.(*BaseBlueprintHandler).kubernetesManager = mockK8sManager

		// Set repository and sources directly on the blueprint
		baseHandler := handler.(*BaseBlueprintHandler)
		baseHandler.blueprint.Repository = blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}

		expectedSources := []blueprintv1alpha1.Source{
			{
				Name: "source2",
				Url:  "https://example.com/source2",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		}
		baseHandler.blueprint.Sources = expectedSources

		// When resolving component sources
		handler.(*BaseBlueprintHandler).resolveComponentSources(&handler.(*BaseBlueprintHandler).blueprint)
	})

	t.Run("SourceWithSecretName", func(t *testing.T) {
		// Given a blueprint handler with repository and source with secret name
		handler, _ := setup(t)

		// Mock Kubernetes manager
		mockK8sManager := &kubernetes.MockKubernetesManager{}
		mockK8sManager.ApplyGitRepositoryFunc = func(repo *sourcev1.GitRepository) error {
			return nil
		}
		handler.(*BaseBlueprintHandler).kubernetesManager = mockK8sManager

		// Set repository and sources directly on the blueprint
		baseHandler := handler.(*BaseBlueprintHandler)
		baseHandler.blueprint.Repository = blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}

		expectedSources := []blueprintv1alpha1.Source{
			{
				Name:       "source3",
				Url:        "https://example.com/source3.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				SecretName: "git-credentials",
			},
		}
		baseHandler.blueprint.Sources = expectedSources

		// When resolving component sources
		handler.(*BaseBlueprintHandler).resolveComponentSources(&handler.(*BaseBlueprintHandler).blueprint)
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
		baseHandler.blueprint.TerraformComponents = expectedComponents

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
		baseHandler.blueprint.Sources = []blueprintv1alpha1.Source{{
			Name:       "test-source",
			Url:        "https://github.com/user/repo.git",
			PathPrefix: "terraform",
			Ref:        blueprintv1alpha1.Reference{Branch: "main"},
		}}

		// And a terraform component referencing that source
		baseHandler.blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{{
			Source: "test-source",
			Path:   "module/path",
		}}

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

func TestBlueprintHandler_processJsonnetTemplate(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("ErrorReadingTemplateFile", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile returns an error
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return nil, fmt.Errorf("read file error")
		}

		// When calling processJsonnetTemplate
		err := handler.processJsonnetTemplate("/template", "/template/test.jsonnet", "/context", false)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error reading template file") {
			t.Errorf("Expected 'error reading template file' in error, got: %v", err)
		}
	})

	t.Run("ErrorMarshallingContextToYAML", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds but YamlMarshal fails
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("{}"), nil
		}
		mocks.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("yaml marshal error")
		}

		// When calling processJsonnetTemplate
		err := handler.processJsonnetTemplate("/template", "/template/test.jsonnet", "/context", false)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error marshalling context to YAML") {
			t.Errorf("Expected 'error marshalling context to YAML' in error, got: %v", err)
		}
	})

	t.Run("ErrorUnmarshallingContextYAML", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds but YamlUnmarshal fails
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("{}"), nil
		}
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("yaml unmarshal error")
		}

		// When calling processJsonnetTemplate
		err := handler.processJsonnetTemplate("/template", "/template/test.jsonnet", "/context", false)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error unmarshalling context YAML") {
			t.Errorf("Expected 'error unmarshalling context YAML' in error, got: %v", err)
		}
	})

	t.Run("ErrorMarshallingContextMapToJSON", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds but JsonMarshal fails
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("{}"), nil
		}
		mocks.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("json marshal error")
		}

		// When calling processJsonnetTemplate
		err := handler.processJsonnetTemplate("/template", "/template/test.jsonnet", "/context", false)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error marshalling context map to JSON") {
			t.Errorf("Expected 'error marshalling context map to JSON' in error, got: %v", err)
		}
	})

	t.Run("ErrorEvaluatingJsonnetTemplate", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds but jsonnet evaluation fails
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("{}"), nil
		}
		mocks.Shims.NewJsonnetVM = func() JsonnetVM {
			return NewMockJsonnetVM(func(filename, snippet string) (string, error) {
				return "", fmt.Errorf("jsonnet evaluation error")
			})
		}

		// When calling processJsonnetTemplate
		err := handler.processJsonnetTemplate("/template", "/template/test.jsonnet", "/context", false)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error evaluating jsonnet template") {
			t.Errorf("Expected 'error evaluating jsonnet template' in error, got: %v", err)
		}
	})

	t.Run("ErrorGettingRelativePath", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("{}"), nil
		}

		// When calling processJsonnetTemplate with invalid paths
		err := handler.processJsonnetTemplate("", "/template/test.jsonnet", "/context", false)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting relative path") {
			t.Errorf("Expected 'error getting relative path' in error, got: %v", err)
		}
	})

	t.Run("BlueprintFileExtension", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("{}"), nil
		}

		// And output file doesn't exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		var writtenPath string
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenPath = name
			return nil
		}

		// When calling processJsonnetTemplate with blueprint file
		err := handler.processJsonnetTemplate("/template", "/template/blueprint.jsonnet", "/context", false)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And the output path should have .yaml extension
		if !strings.HasSuffix(writtenPath, "blueprint.yaml") {
			t.Errorf("Expected blueprint.yaml extension, got: %s", writtenPath)
		}
	})

	t.Run("TerraformFileExtension", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("{}"), nil
		}

		// And output file doesn't exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		var writtenPath string
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenPath = name
			return nil
		}

		// When calling processJsonnetTemplate with terraform file
		err := handler.processJsonnetTemplate("/template", "/template/terraform/main.jsonnet", "/context", false)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And the output path should have .tfvars extension
		if !strings.HasSuffix(writtenPath, "main.tfvars") {
			t.Errorf("Expected main.tfvars extension, got: %s", writtenPath)
		}
	})

	t.Run("DefaultYamlFileExtension", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("{}"), nil
		}

		// And output file doesn't exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		var writtenPath string
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenPath = name
			return nil
		}

		// When calling processJsonnetTemplate with regular file
		err := handler.processJsonnetTemplate("/template", "/template/config.jsonnet", "/context", false)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And the output path should have .yaml extension
		if !strings.HasSuffix(writtenPath, "config.yaml") {
			t.Errorf("Expected config.yaml extension, got: %s", writtenPath)
		}
	})

	t.Run("SkipsExistingFileWithoutReset", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("{}"), nil
		}

		// And output file already exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".yaml") {
				return mockFileInfo{name: "test.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		writeFileCalled := false
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writeFileCalled = true
			return nil
		}

		// When calling processJsonnetTemplate without reset
		err := handler.processJsonnetTemplate("/template", "/template/test.jsonnet", "/context", false)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And WriteFile should not be called
		if writeFileCalled {
			t.Error("WriteFile should not be called when file exists and reset is false")
		}
	})

	t.Run("OverwritesExistingFileWithReset", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("{}"), nil
		}

		// And output file already exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".yaml") {
				return mockFileInfo{name: "test.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		writeFileCalled := false
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writeFileCalled = true
			return nil
		}

		// When calling processJsonnetTemplate with reset
		err := handler.processJsonnetTemplate("/template", "/template/test.jsonnet", "/context", true)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And WriteFile should be called
		if !writeFileCalled {
			t.Error("WriteFile should be called when reset is true")
		}
	})

	t.Run("ErrorCreatingOutputDirectory", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("{}"), nil
		}

		// And MkdirAll fails
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir error")
		}

		// When calling processJsonnetTemplate
		err := handler.processJsonnetTemplate("/template", "/template/subdir/test.jsonnet", "/context", false)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error creating output directory") {
			t.Errorf("Expected 'error creating output directory' in error, got: %v", err)
		}
	})

	t.Run("ErrorWritingOutputFile", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("{}"), nil
		}

		// And WriteFile fails
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write file error")
		}

		// When calling processJsonnetTemplate
		err := handler.processJsonnetTemplate("/template", "/template/test.jsonnet", "/context", false)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing output file") {
			t.Errorf("Expected 'error writing output file' in error, got: %v", err)
		}
	})

	t.Run("SuccessfulProcessing", func(t *testing.T) {
		// Given a blueprint handler with mocked dependencies
		handler, mocks := setup(t)

		// And ReadFile succeeds
		templateContent := `{
			kind: "Blueprint",
			metadata: {
				name: std.extVar("context").name
			}
		}`
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(templateContent), nil
		}

		// And output file doesn't exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And jsonnet evaluation returns content
		mocks.Shims.NewJsonnetVM = func() JsonnetVM {
			return NewMockJsonnetVM(func(filename, snippet string) (string, error) {
				return `{"kind": "Blueprint", "metadata": {"name": "test-context"}}`, nil
			})
		}

		var writtenContent []byte
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// When calling processJsonnetTemplate
		err := handler.processJsonnetTemplate("/template", "/template/blueprint.jsonnet", "/context", false)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And the content should be processed
		if len(writtenContent) == 0 {
			t.Error("Expected content to be written")
		}
	})
}

func TestBlueprintHandler_toKubernetesKustomization(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("BasicConversion", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And a basic blueprint kustomization
		blueprintKustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Source:        "test-source",
			Path:          "test/path",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{true}[0],
			Wait:          &[]bool{false}[0],
		}

		// When converting to kubernetes kustomization
		result := handler.toKubernetesKustomization(blueprintKustomization, "test-namespace")

		// Then the basic fields should be correctly mapped
		if result.Name != "test-kustomization" {
			t.Errorf("Expected name to be test-kustomization, got %s", result.Name)
		}
		if result.Namespace != "test-namespace" {
			t.Errorf("Expected namespace to be test-namespace, got %s", result.Namespace)
		}
		if result.Spec.SourceRef.Name != "test-source" {
			t.Errorf("Expected source name to be test-source, got %s", result.Spec.SourceRef.Name)
		}
		if result.Spec.SourceRef.Kind != "GitRepository" {
			t.Errorf("Expected source kind to be GitRepository, got %s", result.Spec.SourceRef.Kind)
		}
		if result.Spec.Path != "test/path" {
			t.Errorf("Expected path to be test/path, got %s", result.Spec.Path)
		}
		if result.Spec.Interval.Duration != 5*time.Minute {
			t.Errorf("Expected interval to be 5m, got %v", result.Spec.Interval.Duration)
		}
		if result.Spec.RetryInterval.Duration != 1*time.Minute {
			t.Errorf("Expected retry interval to be 1m, got %v", result.Spec.RetryInterval.Duration)
		}
		if result.Spec.Timeout.Duration != 10*time.Minute {
			t.Errorf("Expected timeout to be 10m, got %v", result.Spec.Timeout.Duration)
		}
		if result.Spec.Force != true {
			t.Errorf("Expected force to be true, got %v", result.Spec.Force)
		}
		if result.Spec.Wait != false {
			t.Errorf("Expected wait to be false, got %v", result.Spec.Wait)
		}
		if result.Spec.Prune != true {
			t.Errorf("Expected prune to be true (default), got %v", result.Spec.Prune)
		}
	})

	t.Run("WithPatches", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And a kustomization with patches
		blueprintKustomization := blueprintv1alpha1.Kustomization{
			Name:          "patched-kustomization",
			Source:        "test-source",
			Path:          "test/path",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{true}[0],
			Patches: []kustomize.Patch{
				{
					Patch: "patch content 1",
					Target: &kustomize.Selector{
						Kind: "Deployment",
						Name: "app-deployment",
					},
				},
				{
					Patch: "patch content 2",
					Target: &kustomize.Selector{
						Kind: "Service",
						Name: "app-service",
					},
				},
			},
		}

		// When converting to kubernetes kustomization
		result := handler.toKubernetesKustomization(blueprintKustomization, "test-namespace")

		// Then the patches should be correctly mapped
		if len(result.Spec.Patches) != 2 {
			t.Errorf("Expected 2 patches, got %d", len(result.Spec.Patches))
		}
		if result.Spec.Patches[0].Patch != "patch content 1" {
			t.Errorf("Expected first patch content to be 'patch content 1', got %s", result.Spec.Patches[0].Patch)
		}
		if result.Spec.Patches[0].Target.Kind != "Deployment" {
			t.Errorf("Expected first patch target kind to be Deployment, got %s", result.Spec.Patches[0].Target.Kind)
		}
		if result.Spec.Patches[0].Target.Name != "app-deployment" {
			t.Errorf("Expected first patch target name to be app-deployment, got %s", result.Spec.Patches[0].Target.Name)
		}
		if result.Spec.Patches[1].Patch != "patch content 2" {
			t.Errorf("Expected second patch content to be 'patch content 2', got %s", result.Spec.Patches[1].Patch)
		}
		if result.Spec.Patches[1].Target.Kind != "Service" {
			t.Errorf("Expected second patch target kind to be Service, got %s", result.Spec.Patches[1].Target.Kind)
		}
		if result.Spec.Patches[1].Target.Name != "app-service" {
			t.Errorf("Expected second patch target name to be app-service, got %s", result.Spec.Patches[1].Target.Name)
		}
	})

	t.Run("WithCustomPrune", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And a kustomization with custom prune setting
		customPrune := false
		blueprintKustomization := blueprintv1alpha1.Kustomization{
			Name:          "custom-prune-kustomization",
			Source:        "test-source",
			Path:          "test/path",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{true}[0],
			Prune:         &customPrune,
		}

		// When converting to kubernetes kustomization
		result := handler.toKubernetesKustomization(blueprintKustomization, "test-namespace")

		// Then the custom prune setting should be used
		if result.Spec.Prune != false {
			t.Errorf("Expected prune to be false (custom), got %v", result.Spec.Prune)
		}
	})

	t.Run("WithDependsOn", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And a kustomization with dependencies
		blueprintKustomization := blueprintv1alpha1.Kustomization{
			Name:          "dependent-kustomization",
			Source:        "test-source",
			Path:          "test/path",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{true}[0],
			DependsOn:     []string{"dependency-1", "dependency-2"},
		}

		// When converting to kubernetes kustomization
		result := handler.toKubernetesKustomization(blueprintKustomization, "test-namespace")

		// Then the dependencies should be correctly mapped
		if len(result.Spec.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(result.Spec.DependsOn))
		}
		if result.Spec.DependsOn[0].Name != "dependency-1" {
			t.Errorf("Expected first dependency name to be dependency-1, got %s", result.Spec.DependsOn[0].Name)
		}
		if result.Spec.DependsOn[0].Namespace != "test-namespace" {
			t.Errorf("Expected first dependency namespace to be test-namespace, got %s", result.Spec.DependsOn[0].Namespace)
		}
		if result.Spec.DependsOn[1].Name != "dependency-2" {
			t.Errorf("Expected second dependency name to be dependency-2, got %s", result.Spec.DependsOn[1].Name)
		}
		if result.Spec.DependsOn[1].Namespace != "test-namespace" {
			t.Errorf("Expected second dependency namespace to be test-namespace, got %s", result.Spec.DependsOn[1].Namespace)
		}
	})

	t.Run("WithPostBuild", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And a kustomization with postBuild configuration
		blueprintKustomization := blueprintv1alpha1.Kustomization{
			Name:          "postbuild-kustomization",
			Source:        "test-source",
			Path:          "test/path",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{true}[0],
			PostBuild: &blueprintv1alpha1.PostBuild{
				Substitute: map[string]string{
					"var1": "value1",
					"var2": "value2",
				},
				SubstituteFrom: []blueprintv1alpha1.SubstituteReference{
					{
						Kind:     "ConfigMap",
						Name:     "config-map-1",
						Optional: false,
					},
					{
						Kind:     "Secret",
						Name:     "secret-1",
						Optional: true,
					},
				},
			},
		}

		// When converting to kubernetes kustomization
		result := handler.toKubernetesKustomization(blueprintKustomization, "test-namespace")

		// Then the postBuild should be correctly mapped
		if result.Spec.PostBuild == nil {
			t.Error("Expected postBuild to be set, got nil")
		}
		if len(result.Spec.PostBuild.Substitute) != 2 {
			t.Errorf("Expected 2 substitute variables, got %d", len(result.Spec.PostBuild.Substitute))
		}
		if result.Spec.PostBuild.Substitute["var1"] != "value1" {
			t.Errorf("Expected var1 to be value1, got %s", result.Spec.PostBuild.Substitute["var1"])
		}
		if result.Spec.PostBuild.Substitute["var2"] != "value2" {
			t.Errorf("Expected var2 to be value2, got %s", result.Spec.PostBuild.Substitute["var2"])
		}
		if len(result.Spec.PostBuild.SubstituteFrom) != 2 {
			t.Errorf("Expected 2 substitute references, got %d", len(result.Spec.PostBuild.SubstituteFrom))
		}
		if result.Spec.PostBuild.SubstituteFrom[0].Kind != "ConfigMap" {
			t.Errorf("Expected first substitute reference kind to be ConfigMap, got %s", result.Spec.PostBuild.SubstituteFrom[0].Kind)
		}
		if result.Spec.PostBuild.SubstituteFrom[0].Name != "config-map-1" {
			t.Errorf("Expected first substitute reference name to be config-map-1, got %s", result.Spec.PostBuild.SubstituteFrom[0].Name)
		}
		if result.Spec.PostBuild.SubstituteFrom[0].Optional != false {
			t.Errorf("Expected first substitute reference optional to be false, got %v", result.Spec.PostBuild.SubstituteFrom[0].Optional)
		}
		if result.Spec.PostBuild.SubstituteFrom[1].Kind != "Secret" {
			t.Errorf("Expected second substitute reference kind to be Secret, got %s", result.Spec.PostBuild.SubstituteFrom[1].Kind)
		}
		if result.Spec.PostBuild.SubstituteFrom[1].Name != "secret-1" {
			t.Errorf("Expected second substitute reference name to be secret-1, got %s", result.Spec.PostBuild.SubstituteFrom[1].Name)
		}
		if result.Spec.PostBuild.SubstituteFrom[1].Optional != true {
			t.Errorf("Expected second substitute reference optional to be true, got %v", result.Spec.PostBuild.SubstituteFrom[1].Optional)
		}
	})

	t.Run("WithComponents", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And a kustomization with components
		blueprintKustomization := blueprintv1alpha1.Kustomization{
			Name:          "components-kustomization",
			Source:        "test-source",
			Path:          "test/path",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{true}[0],
			Components:    []string{"component-1", "component-2"},
		}

		// When converting to kubernetes kustomization
		result := handler.toKubernetesKustomization(blueprintKustomization, "test-namespace")

		// Then the components should be correctly mapped
		if len(result.Spec.Components) != 2 {
			t.Errorf("Expected 2 components, got %d", len(result.Spec.Components))
		}
		if result.Spec.Components[0] != "component-1" {
			t.Errorf("Expected first component to be component-1, got %s", result.Spec.Components[0])
		}
		if result.Spec.Components[1] != "component-2" {
			t.Errorf("Expected second component to be component-2, got %s", result.Spec.Components[1])
		}
	})

	t.Run("CompleteConversion", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And a kustomization with all features
		customPrune := false
		blueprintKustomization := blueprintv1alpha1.Kustomization{
			Name:          "complete-kustomization",
			Source:        "complete-source",
			Path:          "complete/path",
			Interval:      &metav1.Duration{Duration: 15 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 3 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 30 * time.Minute},
			Force:         &[]bool{true}[0],
			Wait:          &[]bool{false}[0],
			Prune:         &customPrune,
			DependsOn:     []string{"dep-1", "dep-2", "dep-3"},
			Components:    []string{"comp-1", "comp-2"},
			Patches: []kustomize.Patch{
				{
					Patch: "complete patch",
					Target: &kustomize.Selector{
						Kind: "StatefulSet",
						Name: "database",
					},
				},
			},
			PostBuild: &blueprintv1alpha1.PostBuild{
				Substitute: map[string]string{
					"env":    "production",
					"region": "us-west-2",
				},
				SubstituteFrom: []blueprintv1alpha1.SubstituteReference{
					{
						Kind:     "ConfigMap",
						Name:     "env-config",
						Optional: false,
					},
				},
			},
		}

		// When converting to kubernetes kustomization
		result := handler.toKubernetesKustomization(blueprintKustomization, "production-namespace")

		// Then all fields should be correctly converted
		if result.Name != "complete-kustomization" {
			t.Errorf("Expected name to be complete-kustomization, got %s", result.Name)
		}
		if result.Namespace != "production-namespace" {
			t.Errorf("Expected namespace to be production-namespace, got %s", result.Namespace)
		}
		if result.Kind != "Kustomization" {
			t.Errorf("Expected kind to be Kustomization, got %s", result.Kind)
		}
		if result.APIVersion != "kustomize.toolkit.fluxcd.io/v1" {
			t.Errorf("Expected apiVersion to be kustomize.toolkit.fluxcd.io/v1, got %s", result.APIVersion)
		}
		if result.Spec.SourceRef.Name != "complete-source" {
			t.Errorf("Expected source name to be complete-source, got %s", result.Spec.SourceRef.Name)
		}
		if result.Spec.Path != "complete/path" {
			t.Errorf("Expected path to be complete/path, got %s", result.Spec.Path)
		}
		if result.Spec.Interval.Duration != 15*time.Minute {
			t.Errorf("Expected interval to be 15m, got %v", result.Spec.Interval.Duration)
		}
		if result.Spec.Prune != false {
			t.Errorf("Expected prune to be false, got %v", result.Spec.Prune)
		}
		if len(result.Spec.DependsOn) != 3 {
			t.Errorf("Expected 3 dependencies, got %d", len(result.Spec.DependsOn))
		}
		if len(result.Spec.Components) != 2 {
			t.Errorf("Expected 2 components, got %d", len(result.Spec.Components))
		}
		if len(result.Spec.Patches) != 1 {
			t.Errorf("Expected 1 patch, got %d", len(result.Spec.Patches))
		}
		if result.Spec.PostBuild == nil {
			t.Error("Expected postBuild to be set, got nil")
		}
	})
}
