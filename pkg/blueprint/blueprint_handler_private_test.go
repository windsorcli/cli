package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
func (m mockFileInfo) Sys() any           { return nil }

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

func TestBlueprintHandler_toFluxKustomization(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		handler.configHandler = mocks.ConfigHandler

		// Set up mock shims to return expected patch content for the test
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			// Extract the relative path from the full path using filepath operations
			pathParts := strings.Split(path, string(filepath.Separator))
			var relativePath string
			for i, part := range pathParts {
				if part == "kustomize" {
					relativePath = strings.Join(pathParts[i:], "/") // Always use forward slashes for consistency
					break
				}
			}

			switch relativePath {
			case "kustomize/patch1.yaml":
				return []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: app-deployment
  namespace: test-namespace
spec:
  replicas: 3`), nil
			case "kustomize/patch2.yaml":
				return []byte(`apiVersion: v1
kind: Service
metadata:
  name: app-service
  namespace: test-namespace
spec:
  type: ClusterIP`), nil
			default:
				return nil, fmt.Errorf("file not found: %s", path)
			}
		}

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
		result := handler.toFluxKustomization(blueprintKustomization, "test-namespace")

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
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Path: "patch1.yaml",
				},
				{
					Path: "patch2.yaml",
				},
			},
		}

		// When converting to kubernetes kustomization
		result := handler.toFluxKustomization(blueprintKustomization, "test-namespace")

		// Then the patches should be correctly mapped
		if len(result.Spec.Patches) != 2 {
			t.Errorf("Expected 2 patches, got %d", len(result.Spec.Patches))
		}
		expectedPatch1 := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: app-deployment
  namespace: test-namespace
spec:
  replicas: 3`
		if result.Spec.Patches[0].Patch != expectedPatch1 {
			t.Errorf("Expected first patch content to be:\n%s\n\ngot:\n%s", expectedPatch1, result.Spec.Patches[0].Patch)
		}
		if result.Spec.Patches[0].Target.Kind != "Deployment" {
			t.Errorf("Expected first patch target kind to be Deployment, got %s", result.Spec.Patches[0].Target.Kind)
		}
		if result.Spec.Patches[0].Target.Name != "app-deployment" {
			t.Errorf("Expected first patch target name to be app-deployment, got %s", result.Spec.Patches[0].Target.Name)
		}
		expectedPatch2 := `apiVersion: v1
kind: Service
metadata:
  name: app-service
  namespace: test-namespace
spec:
  type: ClusterIP`
		if result.Spec.Patches[1].Patch != expectedPatch2 {
			t.Errorf("Expected second patch content to be:\n%s\n\ngot:\n%s", expectedPatch2, result.Spec.Patches[1].Patch)
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
		result := handler.toFluxKustomization(blueprintKustomization, "test-namespace")

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
		result := handler.toFluxKustomization(blueprintKustomization, "test-namespace")

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
		result := handler.toFluxKustomization(blueprintKustomization, "test-namespace")

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
		result := handler.toFluxKustomization(blueprintKustomization, "test-namespace")

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
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Path: "patch1.yaml",
				},
				{
					Path: "patch2.yaml",
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
		result := handler.toFluxKustomization(blueprintKustomization, "production-namespace")

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
		if len(result.Spec.Patches) != 2 {
			t.Errorf("Expected 2 patches, got %d", len(result.Spec.Patches))
		}
		if result.Spec.PostBuild == nil {
			t.Error("Expected postBuild to be set, got nil")
		}
	})
}

func TestBaseBlueprintHandler_applySourceRepository(t *testing.T) {
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

	t.Run("GitSource", func(t *testing.T) {
		// Given a blueprint handler with a git source
		handler, mocks := setup(t)

		gitSource := blueprintv1alpha1.Source{
			Name: "git-source",
			Url:  "https://github.com/example/repo.git",
			Ref:  blueprintv1alpha1.Reference{Branch: "main"},
		}

		gitRepoApplied := false
		mocks.KubernetesManager.ApplyGitRepositoryFunc = func(repo *sourcev1.GitRepository) error {
			gitRepoApplied = true
			if repo.Name != "git-source" {
				t.Errorf("Expected repo name 'git-source', got %s", repo.Name)
			}
			if repo.Spec.URL != "https://github.com/example/repo.git" {
				t.Errorf("Expected URL 'https://github.com/example/repo.git', got %s", repo.Spec.URL)
			}
			return nil
		}

		// When applying the source repository
		err := handler.applySourceRepository(gitSource, "default")

		// Then it should call ApplyGitRepository
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !gitRepoApplied {
			t.Error("Expected ApplyGitRepository to be called")
		}
	})

	t.Run("OCISource", func(t *testing.T) {
		// Given a blueprint handler with an OCI source
		handler, mocks := setup(t)

		ociSource := blueprintv1alpha1.Source{
			Name: "oci-source",
			Url:  "oci://ghcr.io/example/repo:v1.0.0",
		}

		ociRepoApplied := false
		mocks.KubernetesManager.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			ociRepoApplied = true
			if repo.Name != "oci-source" {
				t.Errorf("Expected repo name 'oci-source', got %s", repo.Name)
			}
			if repo.Spec.URL != "oci://ghcr.io/example/repo" {
				t.Errorf("Expected URL 'oci://ghcr.io/example/repo', got %s", repo.Spec.URL)
			}
			if repo.Spec.Reference.Tag != "v1.0.0" {
				t.Errorf("Expected tag 'v1.0.0', got %s", repo.Spec.Reference.Tag)
			}
			return nil
		}

		// When applying the source repository
		err := handler.applySourceRepository(ociSource, "default")

		// Then it should call ApplyOCIRepository
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !ociRepoApplied {
			t.Error("Expected ApplyOCIRepository to be called")
		}
	})

	t.Run("GitSourceError", func(t *testing.T) {
		// Given a blueprint handler with git source that fails
		handler, mocks := setup(t)

		gitSource := blueprintv1alpha1.Source{
			Name: "git-source",
			Url:  "https://github.com/example/repo.git",
		}

		mocks.KubernetesManager.ApplyGitRepositoryFunc = func(repo *sourcev1.GitRepository) error {
			return fmt.Errorf("git repository error")
		}

		// When applying the source repository
		err := handler.applySourceRepository(gitSource, "default")

		// Then it should return the error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "git repository error") {
			t.Errorf("Expected git repository error, got: %v", err)
		}
	})

	t.Run("OCISourceError", func(t *testing.T) {
		// Given a blueprint handler with OCI source that fails
		handler, mocks := setup(t)

		ociSource := blueprintv1alpha1.Source{
			Name: "oci-source",
			Url:  "oci://ghcr.io/example/repo:v1.0.0",
		}

		mocks.KubernetesManager.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			return fmt.Errorf("oci repository error")
		}

		// When applying the source repository
		err := handler.applySourceRepository(ociSource, "default")

		// Then it should return the error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "oci repository error") {
			t.Errorf("Expected oci repository error, got: %v", err)
		}
	})
}

func TestBaseBlueprintHandler_applyOCIRepository(t *testing.T) {
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

	t.Run("BasicOCIRepository", func(t *testing.T) {
		// Given a blueprint handler with basic OCI source
		handler, mocks := setup(t)

		source := blueprintv1alpha1.Source{
			Name: "basic-oci",
			Url:  "oci://registry.example.com/repo:v1.0.0",
		}

		var appliedRepo *sourcev1.OCIRepository
		mocks.KubernetesManager.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// When applying the OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then it should create the correct OCIRepository
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if appliedRepo == nil {
			t.Fatal("Expected OCIRepository to be applied")
		}
		if appliedRepo.Name != "basic-oci" {
			t.Errorf("Expected name 'basic-oci', got %s", appliedRepo.Name)
		}
		if appliedRepo.Namespace != "test-namespace" {
			t.Errorf("Expected namespace 'test-namespace', got %s", appliedRepo.Namespace)
		}
		if appliedRepo.Spec.URL != "oci://registry.example.com/repo" {
			t.Errorf("Expected URL 'oci://registry.example.com/repo', got %s", appliedRepo.Spec.URL)
		}
		if appliedRepo.Spec.Reference.Tag != "v1.0.0" {
			t.Errorf("Expected tag 'v1.0.0', got %s", appliedRepo.Spec.Reference.Tag)
		}
	})

	t.Run("OCIRepositoryWithoutTag", func(t *testing.T) {
		// Given an OCI source without embedded tag
		handler, mocks := setup(t)

		source := blueprintv1alpha1.Source{
			Name: "no-tag-oci",
			Url:  "oci://registry.example.com/repo",
		}

		var appliedRepo *sourcev1.OCIRepository
		mocks.KubernetesManager.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// When applying the OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then it should default to latest tag
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if appliedRepo.Spec.Reference.Tag != "latest" {
			t.Errorf("Expected default tag 'latest', got %s", appliedRepo.Spec.Reference.Tag)
		}
	})

	t.Run("OCIRepositoryWithRefField", func(t *testing.T) {
		// Given an OCI source with ref field instead of embedded tag
		handler, mocks := setup(t)

		source := blueprintv1alpha1.Source{
			Name: "ref-field-oci",
			Url:  "oci://registry.example.com/repo",
			Ref: blueprintv1alpha1.Reference{
				Tag: "v2.0.0",
			},
		}

		var appliedRepo *sourcev1.OCIRepository
		mocks.KubernetesManager.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// When applying the OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then it should use the ref field tag
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if appliedRepo.Spec.Reference.Tag != "v2.0.0" {
			t.Errorf("Expected tag 'v2.0.0', got %s", appliedRepo.Spec.Reference.Tag)
		}
	})

	t.Run("OCIRepositoryWithSemVer", func(t *testing.T) {
		// Given an OCI source with semver reference
		handler, mocks := setup(t)

		source := blueprintv1alpha1.Source{
			Name: "semver-oci",
			Url:  "oci://registry.example.com/repo",
			Ref: blueprintv1alpha1.Reference{
				SemVer: ">=1.0.0 <2.0.0",
			},
		}

		var appliedRepo *sourcev1.OCIRepository
		mocks.KubernetesManager.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// When applying the OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then it should use the semver reference
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if appliedRepo.Spec.Reference.SemVer != ">=1.0.0 <2.0.0" {
			t.Errorf("Expected semver '>=1.0.0 <2.0.0', got %s", appliedRepo.Spec.Reference.SemVer)
		}
	})

	t.Run("OCIRepositoryWithDigest", func(t *testing.T) {
		// Given an OCI source with commit/digest reference
		handler, mocks := setup(t)

		source := blueprintv1alpha1.Source{
			Name: "digest-oci",
			Url:  "oci://registry.example.com/repo",
			Ref: blueprintv1alpha1.Reference{
				Commit: "sha256:abc123",
			},
		}

		var appliedRepo *sourcev1.OCIRepository
		mocks.KubernetesManager.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// When applying the OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then it should use the digest reference
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if appliedRepo.Spec.Reference.Digest != "sha256:abc123" {
			t.Errorf("Expected digest 'sha256:abc123', got %s", appliedRepo.Spec.Reference.Digest)
		}
	})

	t.Run("OCIRepositoryWithSecret", func(t *testing.T) {
		// Given an OCI source with secret name
		handler, mocks := setup(t)

		source := blueprintv1alpha1.Source{
			Name:       "secret-oci",
			Url:        "oci://private-registry.example.com/repo:v1.0.0",
			SecretName: "registry-credentials",
		}

		var appliedRepo *sourcev1.OCIRepository
		mocks.KubernetesManager.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// When applying the OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then it should include the secret reference
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if appliedRepo.Spec.SecretRef == nil {
			t.Error("Expected SecretRef to be set")
		} else if appliedRepo.Spec.SecretRef.Name != "registry-credentials" {
			t.Errorf("Expected secret name 'registry-credentials', got %s", appliedRepo.Spec.SecretRef.Name)
		}
	})

	t.Run("OCIRepositoryWithPortInURL", func(t *testing.T) {
		// Given an OCI source with port in URL (should not be treated as tag)
		handler, mocks := setup(t)

		source := blueprintv1alpha1.Source{
			Name: "port-oci",
			Url:  "oci://registry.example.com:5000/repo",
			Ref: blueprintv1alpha1.Reference{
				Tag: "v1.0.0",
			},
		}

		var appliedRepo *sourcev1.OCIRepository
		mocks.KubernetesManager.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// When applying the OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then it should preserve the port and use ref field
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if appliedRepo.Spec.URL != "oci://registry.example.com:5000/repo" {
			t.Errorf("Expected URL with port 'oci://registry.example.com:5000/repo', got %s", appliedRepo.Spec.URL)
		}
		if appliedRepo.Spec.Reference.Tag != "v1.0.0" {
			t.Errorf("Expected tag 'v1.0.0', got %s", appliedRepo.Spec.Reference.Tag)
		}
	})

	t.Run("OCIRepositoryError", func(t *testing.T) {
		// Given an OCI source that fails to apply
		handler, mocks := setup(t)

		source := blueprintv1alpha1.Source{
			Name: "error-oci",
			Url:  "oci://registry.example.com/repo:v1.0.0",
		}

		mocks.KubernetesManager.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			return fmt.Errorf("failed to apply oci repository")
		}

		// When applying the OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then it should return the error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply oci repository") {
			t.Errorf("Expected oci repository error, got: %v", err)
		}
	})
}

func TestBaseBlueprintHandler_isOCISource(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler
	}

	t.Run("MainRepositoryOCI", func(t *testing.T) {
		// Given a blueprint with OCI main repository
		handler := setup(t)
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Repository: blueprintv1alpha1.Repository{
				Url: "oci://ghcr.io/example/blueprint:v1.0.0",
			},
		}

		// When checking if main repository is OCI
		result := handler.isOCISource("test-blueprint")

		// Then it should return true
		if !result {
			t.Error("Expected main repository to be identified as OCI source")
		}
	})

	t.Run("MainRepositoryGit", func(t *testing.T) {
		// Given a blueprint with Git main repository
		handler := setup(t)
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/example/blueprint.git",
			},
		}

		// When checking if main repository is OCI
		result := handler.isOCISource("test-blueprint")

		// Then it should return false
		if result {
			t.Error("Expected main repository to not be identified as OCI source")
		}
	})

	t.Run("AdditionalSourceOCI", func(t *testing.T) {
		// Given a blueprint with OCI additional source
		handler := setup(t)
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/example/blueprint.git",
			},
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "oci-source",
					Url:  "oci://ghcr.io/example/source:latest",
				},
				{
					Name: "git-source",
					Url:  "https://github.com/example/source.git",
				},
			},
		}

		// When checking if additional source is OCI
		result := handler.isOCISource("oci-source")

		// Then it should return true
		if !result {
			t.Error("Expected additional source to be identified as OCI source")
		}
	})

	t.Run("AdditionalSourceGit", func(t *testing.T) {
		// Given a blueprint with Git additional source
		handler := setup(t)
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "git-source",
					Url:  "https://github.com/example/source.git",
				},
			},
		}

		// When checking if additional source is OCI
		result := handler.isOCISource("git-source")

		// Then it should return false
		if result {
			t.Error("Expected additional source to not be identified as OCI source")
		}
	})
}
