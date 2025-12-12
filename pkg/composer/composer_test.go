package composer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/composer/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// ComposerTestMocks contains all the mock dependencies for testing the Composer
type ComposerTestMocks struct {
	ConfigHandler     config.ConfigHandler
	Shell             shell.Shell
	ArtifactBuilder   *artifact.MockArtifact
	BlueprintHandler  *blueprint.MockBlueprintHandler
	TerraformResolver *terraform.MockModuleResolver
	Runtime           *runtime.Runtime
}

// setupComposerMocks creates mock components for testing the Composer with optional overrides
func setupComposerMocks(t *testing.T, opts ...func(*ComposerTestMocks)) *ComposerTestMocks {
	t.Helper()

	// Create temporary directory for test
	tmpDir := t.TempDir()

	configHandler := config.NewMockConfigHandler()
	configHandler.GetConfigRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	configHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	// Create runtime
	rt := &runtime.Runtime{
		ContextName:   "test-context",
		ProjectRoot:   tmpDir,
		ConfigRoot:    filepath.Join(tmpDir, "contexts", "test-context"),
		TemplateRoot:  filepath.Join(tmpDir, "contexts", "_template"),
		ConfigHandler: configHandler,
		Shell:         mockShell,
	}

	// Create default mocks
	mocks := &ComposerTestMocks{
		ConfigHandler:     configHandler,
		Shell:             mockShell,
		ArtifactBuilder:   artifact.NewMockArtifact(),
		BlueprintHandler:  blueprint.NewMockBlueprintHandler(),
		TerraformResolver: terraform.NewMockModuleResolver(),
		Runtime:           rt,
	}

	// Apply any overrides
	for _, opt := range opts {
		opt(mocks)
	}

	return mocks
}

// createComposerWithMocks creates a Composer instance using the provided mocks
func createComposerWithMocks(mocks *ComposerTestMocks) *Composer {
	overrideComposer := &Composer{
		ArtifactBuilder:   mocks.ArtifactBuilder,
		BlueprintHandler:  mocks.BlueprintHandler,
		TerraformResolver: mocks.TerraformResolver,
	}
	return NewComposer(mocks.Runtime, overrideComposer)
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestComposer_NewComposer(t *testing.T) {
	t.Run("CreatesComposerWithDependencies", func(t *testing.T) {
		// Given mocks
		mocks := setupComposerMocks(t)

		// When creating a new composer
		composer := NewComposer(mocks.Runtime)

		// Then composer should be created
		if composer == nil {
			t.Fatal("Expected Composer to be created")
		}

		// And runtime dependencies should be set
		if composer.Runtime.Shell != mocks.Shell {
			t.Error("Expected shell to be set")
		}

		if composer.Runtime.ConfigHandler != mocks.ConfigHandler {
			t.Error("Expected config handler to be set")
		}

		// And all resource dependencies should be initialized
		if composer.ArtifactBuilder == nil {
			t.Error("Expected artifact builder to be initialized")
		}

		if composer.BlueprintHandler == nil {
			t.Error("Expected blueprint handler to be initialized")
		}

		if composer.TerraformResolver == nil {
			t.Error("Expected terraform resolver to be initialized")
		}
	})

	t.Run("UsesOverrideArtifactBuilder", func(t *testing.T) {
		// Given mocks with override artifact builder
		mocks := setupComposerMocks(t)
		customArtifactBuilder := artifact.NewMockArtifact()

		// When creating composer with override
		overrideComposer := &Composer{
			ArtifactBuilder: customArtifactBuilder,
		}
		composer := NewComposer(mocks.Runtime, overrideComposer)

		// Then composer should use the override
		if composer == nil {
			t.Fatal("Expected Composer to be created")
		}

		if composer.ArtifactBuilder != customArtifactBuilder {
			t.Error("Expected override artifact builder to be used")
		}

		// And other dependencies should still be initialized
		if composer.BlueprintHandler == nil {
			t.Error("Expected blueprint handler to be initialized")
		}

		if composer.TerraformResolver == nil {
			t.Error("Expected terraform resolver to be initialized")
		}
	})

	t.Run("UsesOverrideBlueprintHandler", func(t *testing.T) {
		// Given mocks with override blueprint handler
		mocks := setupComposerMocks(t)
		customBlueprintHandler := blueprint.NewMockBlueprintHandler()

		// When creating composer with override
		overrideComposer := &Composer{
			BlueprintHandler: customBlueprintHandler,
		}
		composer := NewComposer(mocks.Runtime, overrideComposer)

		// Then composer should use the override
		if composer == nil {
			t.Fatal("Expected Composer to be created")
		}

		if composer.BlueprintHandler != customBlueprintHandler {
			t.Error("Expected override blueprint handler to be used")
		}

		// And other dependencies should still be initialized
		if composer.ArtifactBuilder == nil {
			t.Error("Expected artifact builder to be initialized")
		}

		if composer.TerraformResolver == nil {
			t.Error("Expected terraform resolver to be initialized")
		}
	})

	t.Run("UsesOverrideTerraformResolver", func(t *testing.T) {
		// Given mocks with override terraform resolver
		mocks := setupComposerMocks(t)
		customTerraformResolver := terraform.NewMockModuleResolver()

		// When creating composer with override
		overrideComposer := &Composer{
			TerraformResolver: customTerraformResolver,
		}
		composer := NewComposer(mocks.Runtime, overrideComposer)

		// Then composer should use the override
		if composer == nil {
			t.Fatal("Expected Composer to be created")
		}

		if composer.TerraformResolver != customTerraformResolver {
			t.Error("Expected override terraform resolver to be used")
		}

		// And other dependencies should still be initialized
		if composer.ArtifactBuilder == nil {
			t.Error("Expected artifact builder to be initialized")
		}

		if composer.BlueprintHandler == nil {
			t.Error("Expected blueprint handler to be initialized")
		}
	})

	t.Run("UsesPartialOverrides", func(t *testing.T) {
		// Given mocks with partial overrides
		mocks := setupComposerMocks(t)
		customArtifactBuilder := artifact.NewMockArtifact()
		customTerraformResolver := terraform.NewMockModuleResolver()

		// When creating composer with partial overrides
		overrideComposer := &Composer{
			ArtifactBuilder:   customArtifactBuilder,
			TerraformResolver: customTerraformResolver,
		}
		composer := NewComposer(mocks.Runtime, overrideComposer)

		// Then composer should use overrides where provided
		if composer == nil {
			t.Fatal("Expected Composer to be created")
		}

		if composer.ArtifactBuilder != customArtifactBuilder {
			t.Error("Expected override artifact builder to be used")
		}

		if composer.TerraformResolver != customTerraformResolver {
			t.Error("Expected override terraform resolver to be used")
		}

		// And blueprint handler should be initialized (not overridden)
		if composer.BlueprintHandler == nil {
			t.Error("Expected blueprint handler to be initialized")
		}
	})

	t.Run("HandlesEmptyProjectRoot", func(t *testing.T) {
		// Given a runtime with empty ProjectRoot
		mocks := setupComposerMocks(t, func(m *ComposerTestMocks) {
			m.Runtime.ProjectRoot = ""
		})

		// When creating composer
		composer := NewComposer(mocks.Runtime)

		// Then composer should still be created (NewBlueprintHandler doesn't validate ProjectRoot)
		if composer == nil {
			t.Error("Expected Composer to be created even with empty ProjectRoot")
		}

		// And BlueprintHandler should be initialized
		if composer.BlueprintHandler == nil {
			t.Error("Expected BlueprintHandler to be initialized")
		}
	})

	t.Run("IgnoresNilOverride", func(t *testing.T) {
		// Given mocks
		mocks := setupComposerMocks(t)

		// When creating composer with nil override
		composer := NewComposer(mocks.Runtime, nil)

		// Then composer should be created with defaults
		if composer == nil {
			t.Fatal("Expected Composer to be created")
		}

		if composer.ArtifactBuilder == nil {
			t.Error("Expected artifact builder to be initialized")
		}

		if composer.BlueprintHandler == nil {
			t.Error("Expected blueprint handler to be initialized")
		}

		if composer.TerraformResolver == nil {
			t.Error("Expected terraform resolver to be initialized")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestComposer_Bundle(t *testing.T) {
	t.Run("SuccessWithExplicitPath", func(t *testing.T) {
		// Given mocks with artifact builder
		mocks := setupComposerMocks(t)
		expectedPath := "/tmp/bundle.tar.gz"
		mocks.ArtifactBuilder.WriteFunc = func(outputPath string, tag string) (string, error) {
			return expectedPath, nil
		}
		composer := createComposerWithMocks(mocks)

		// When bundling with explicit path
		result, err := composer.Bundle("/tmp/bundle.tar.gz", "v1.0.0")

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should match expected path
		if result != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, result)
		}
	})

	t.Run("SuccessWithTag", func(t *testing.T) {
		// Given mocks with artifact builder
		mocks := setupComposerMocks(t)
		expectedPath := "/tmp/bundle-v1.0.0.tar.gz"
		mocks.ArtifactBuilder.WriteFunc = func(outputPath string, tag string) (string, error) {
			if tag != "v1.0.0" {
				t.Errorf("Expected tag v1.0.0, got %s", tag)
			}
			return expectedPath, nil
		}
		composer := createComposerWithMocks(mocks)

		// When bundling with tag
		result, err := composer.Bundle("/tmp/bundle.tar.gz", "v1.0.0")

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should match expected path
		if result != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, result)
		}
	})

	t.Run("SuccessWithRelativePath", func(t *testing.T) {
		// Given mocks with artifact builder
		mocks := setupComposerMocks(t)
		expectedPath := "bundle.tar.gz"
		mocks.ArtifactBuilder.WriteFunc = func(outputPath string, tag string) (string, error) {
			return expectedPath, nil
		}
		composer := createComposerWithMocks(mocks)

		// When bundling with relative path
		result, err := composer.Bundle("bundle.tar.gz", "")

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should match expected path
		if result != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, result)
		}
	})

	t.Run("ErrorFromArtifactBuilderWrite", func(t *testing.T) {
		// Given mocks with artifact builder that returns error
		mocks := setupComposerMocks(t)
		expectedError := "write failed"
		mocks.ArtifactBuilder.WriteFunc = func(outputPath string, tag string) (string, error) {
			return "", fmt.Errorf("%s", expectedError)
		}
		composer := createComposerWithMocks(mocks)

		// When bundling
		result, err := composer.Bundle("/tmp/bundle.tar.gz", "")

		// Then error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got: %v", expectedError, err)
		}

		// And result should be empty
		if result != "" {
			t.Errorf("Expected empty result on error, got: %s", result)
		}
	})
}

func TestComposer_Push(t *testing.T) {
	t.Run("SuccessWithFullURL", func(t *testing.T) {
		// Given mocks with artifact builder
		mocks := setupComposerMocks(t)
		registryBase := "ghcr.io"
		repoName := "test/repo"
		tag := "latest"
		mocks.ArtifactBuilder.BundleFunc = func() error {
			return nil
		}
		mocks.ArtifactBuilder.PushFunc = func(base string, repo string, tagValue string) error {
			if base != registryBase {
				t.Errorf("Expected registry base %s, got %s", registryBase, base)
			}
			if repo != repoName {
				t.Errorf("Expected repo name %s, got %s", repoName, repo)
			}
			if tagValue != tag {
				t.Errorf("Expected tag %s, got %s", tag, tagValue)
			}
			return nil
		}
		composer := createComposerWithMocks(mocks)

		// When pushing with full URL
		result, err := composer.Push("ghcr.io/test/repo:latest")

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should be correct URL
		expectedURL := "ghcr.io/test/repo:latest"
		if result != expectedURL {
			t.Errorf("Expected URL %s, got %s", expectedURL, result)
		}
	})

	t.Run("SuccessWithOCIPrefix", func(t *testing.T) {
		// Given mocks with artifact builder
		mocks := setupComposerMocks(t)
		mocks.ArtifactBuilder.BundleFunc = func() error {
			return nil
		}
		mocks.ArtifactBuilder.PushFunc = func(base string, repo string, tag string) error {
			return nil
		}
		composer := createComposerWithMocks(mocks)

		// When pushing with OCI prefix
		result, err := composer.Push("oci://ghcr.io/test/repo:v1.0.0")

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should be correct URL (without oci:// prefix)
		expectedURL := "ghcr.io/test/repo:v1.0.0"
		if result != expectedURL {
			t.Errorf("Expected URL %s, got %s", expectedURL, result)
		}
	})

	t.Run("SuccessWithoutTag", func(t *testing.T) {
		// Given mocks with artifact builder
		mocks := setupComposerMocks(t)
		mocks.ArtifactBuilder.BundleFunc = func() error {
			return nil
		}
		mocks.ArtifactBuilder.PushFunc = func(base string, repo string, tag string) error {
			if tag != "" {
				t.Errorf("Expected empty tag, got %s", tag)
			}
			return nil
		}
		composer := createComposerWithMocks(mocks)

		// When pushing without tag
		result, err := composer.Push("ghcr.io/test/repo")

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should be URL without tag
		expectedURL := "ghcr.io/test/repo"
		if result != expectedURL {
			t.Errorf("Expected URL %s, got %s", expectedURL, result)
		}
	})

	t.Run("ErrorFromParseRegistryURL", func(t *testing.T) {
		// Given mocks
		mocks := setupComposerMocks(t)
		composer := createComposerWithMocks(mocks)

		// When pushing with invalid URL
		result, err := composer.Push("invalid-url")

		// Then error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to parse registry URL") {
			t.Errorf("Expected error about parsing registry URL, got: %v", err)
		}

		// And result should be empty
		if result != "" {
			t.Errorf("Expected empty result on error, got: %s", result)
		}
	})

	t.Run("ErrorFromBundle", func(t *testing.T) {
		// Given mocks with artifact builder that fails on bundle
		mocks := setupComposerMocks(t)
		expectedError := "bundle failed"
		mocks.ArtifactBuilder.BundleFunc = func() error {
			return fmt.Errorf("%s", expectedError)
		}
		composer := createComposerWithMocks(mocks)

		// When pushing
		result, err := composer.Push("ghcr.io/test/repo:latest")

		// Then error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got: %v", expectedError, err)
		}

		// And result should be empty
		if result != "" {
			t.Errorf("Expected empty result on error, got: %s", result)
		}
	})

	t.Run("ErrorFromPush", func(t *testing.T) {
		// Given mocks with artifact builder that fails on push
		mocks := setupComposerMocks(t)
		expectedError := "push failed"
		mocks.ArtifactBuilder.BundleFunc = func() error {
			return nil
		}
		mocks.ArtifactBuilder.PushFunc = func(base string, repo string, tag string) error {
			return fmt.Errorf("%s", expectedError)
		}
		composer := createComposerWithMocks(mocks)

		// When pushing
		result, err := composer.Push("ghcr.io/test/repo:latest")

		// Then error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got: %v", expectedError, err)
		}

		// And result should be empty
		if result != "" {
			t.Errorf("Expected empty result on error, got: %s", result)
		}
	})
}

func TestComposer_Generate(t *testing.T) {
	t.Run("SuccessFullFlow", func(t *testing.T) {
		// Given mocks with all handlers succeeding
		mocks := setupComposerMocks(t)
		mocks.BlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return nil
		}
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			return nil
		}
		mocks.TerraformResolver.ProcessModulesFunc = func() error {
			return nil
		}
		composer := createComposerWithMocks(mocks)

		// When generating
		err := composer.Generate()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithOverwriteTrue", func(t *testing.T) {
		// Given mocks with all handlers succeeding
		mocks := setupComposerMocks(t)
		overwriteCalled := false
		mocks.BlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return nil
		}
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			if len(overwrite) > 0 && overwrite[0] {
				overwriteCalled = true
			}
			return nil
		}
		mocks.TerraformResolver.ProcessModulesFunc = func() error {
			return nil
		}
		composer := createComposerWithMocks(mocks)

		// When generating with overwrite true
		err := composer.Generate(true)

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And overwrite should be passed to Write
		if !overwriteCalled {
			t.Error("Expected overwrite to be passed to Write")
		}
	})

	t.Run("SuccessWithOverwriteFalse", func(t *testing.T) {
		// Given mocks with all handlers succeeding
		mocks := setupComposerMocks(t)
		overwriteValue := true
		mocks.BlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return nil
		}
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			if len(overwrite) > 0 {
				overwriteValue = overwrite[0]
			}
			return nil
		}
		mocks.TerraformResolver.ProcessModulesFunc = func() error {
			return nil
		}
		composer := createComposerWithMocks(mocks)

		// When generating with overwrite false
		err := composer.Generate(false)

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And overwrite should be false
		if overwriteValue {
			t.Error("Expected overwrite to be false")
		}
	})

	t.Run("SuccessWithTerraformEnabled", func(t *testing.T) {
		// Given mocks with terraform enabled
		mocks := setupComposerMocks(t)
		if mockConfigHandler, ok := mocks.ConfigHandler.(*config.MockConfigHandler); ok {
			mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
				if key == "terraform.enabled" {
					return true
				}
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return false
			}
		}
		generateTfvarsCalled := false
		mocks.BlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return nil
		}
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			return nil
		}
		mocks.TerraformResolver.ProcessModulesFunc = func() error {
			return nil
		}
		mocks.TerraformResolver.GenerateTfvarsFunc = func(overwrite bool) error {
			generateTfvarsCalled = true
			return nil
		}
		composer := createComposerWithMocks(mocks)

		// When generating
		err := composer.Generate()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And GenerateTfvars should be called
		if !generateTfvarsCalled {
			t.Error("Expected GenerateTfvars to be called when terraform is enabled")
		}
	})

	t.Run("SuccessWithTerraformDisabled", func(t *testing.T) {
		// Given mocks with terraform disabled
		mocks := setupComposerMocks(t)
		if mockConfigHandler, ok := mocks.ConfigHandler.(*config.MockConfigHandler); ok {
			mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
				if key == "terraform.enabled" {
					return false
				}
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return false
			}
		}
		generateTfvarsCalled := false
		mocks.BlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return nil
		}
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			return nil
		}
		mocks.TerraformResolver.ProcessModulesFunc = func() error {
			return nil
		}
		mocks.TerraformResolver.GenerateTfvarsFunc = func(overwrite bool) error {
			generateTfvarsCalled = true
			return nil
		}
		composer := createComposerWithMocks(mocks)

		// When generating
		err := composer.Generate()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And GenerateTfvars should not be called
		if generateTfvarsCalled {
			t.Error("Expected GenerateTfvars not to be called when terraform is disabled")
		}
	})

	t.Run("ErrorFromWrite", func(t *testing.T) {
		// Given mocks with Write failing
		mocks := setupComposerMocks(t)
		expectedError := "write failed"
		mocks.BlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return nil
		}
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			return fmt.Errorf("%s", expectedError)
		}
		composer := createComposerWithMocks(mocks)

		// When generating
		err := composer.Generate()

		// Then error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got: %v", expectedError, err)
		}
	})

	t.Run("ErrorFromProcessModules", func(t *testing.T) {
		// Given mocks with ProcessModules failing
		mocks := setupComposerMocks(t)
		expectedError := "process modules failed"
		mocks.BlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return nil
		}
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			return nil
		}
		mocks.TerraformResolver.ProcessModulesFunc = func() error {
			return fmt.Errorf("%s", expectedError)
		}
		composer := createComposerWithMocks(mocks)

		// When generating
		err := composer.Generate()

		// Then error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got: %v", expectedError, err)
		}
	})

	t.Run("ErrorFromGenerateGitignore", func(t *testing.T) {
		// Given mocks with generateGitignore failing (simulated via file system error)
		mocks := setupComposerMocks(t)
		mocks.BlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return nil
		}
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			return nil
		}
		mocks.TerraformResolver.ProcessModulesFunc = func() error {
			return nil
		}
		mocks.Runtime.ProjectRoot = "/nonexistent/path/that/cannot/be/written"
		composer := createComposerWithMocks(mocks)

		// When generating
		err := composer.Generate()

		// Then error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to generate .gitignore") {
			t.Errorf("Expected error about generating .gitignore, got: %v", err)
		}
	})

	t.Run("ErrorFromGenerateTfvars", func(t *testing.T) {
		// Given mocks with GenerateTfvars failing
		mocks := setupComposerMocks(t)
		if mockConfigHandler, ok := mocks.ConfigHandler.(*config.MockConfigHandler); ok {
			mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
				if key == "terraform.enabled" {
					return true
				}
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return false
			}
		}
		expectedError := "generate tfvars failed"
		mocks.BlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return nil
		}
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			return nil
		}
		mocks.TerraformResolver.ProcessModulesFunc = func() error {
			return nil
		}
		mocks.TerraformResolver.GenerateTfvarsFunc = func(overwrite bool) error {
			return fmt.Errorf("%s", expectedError)
		}
		composer := createComposerWithMocks(mocks)

		// When generating
		err := composer.Generate()

		// Then error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got: %v", expectedError, err)
		}
	})
}

func TestComposer_GenerateBlueprint(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given mocks with blueprint handler
		mocks := setupComposerMocks(t)
		expectedBlueprint := &blueprintv1alpha1.Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: blueprintv1alpha1.Metadata{
				Name:        "test-blueprint",
				Description: "Test blueprint",
			},
		}
		mocks.BlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return nil
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return expectedBlueprint
		}
		composer := createComposerWithMocks(mocks)

		// When generating blueprint
		result, err := composer.GenerateBlueprint()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should match expected blueprint
		if result == nil {
			t.Fatal("Expected non-nil blueprint")
		}

		if result.Metadata.Name != expectedBlueprint.Metadata.Name {
			t.Errorf("Expected blueprint name %s, got %s", expectedBlueprint.Metadata.Name, result.Metadata.Name)
		}
	})

	t.Run("ErrorFromLoadBlueprint", func(t *testing.T) {
		// Given mocks with LoadBlueprint failing
		mocks := setupComposerMocks(t)
		expectedError := "load blueprint failed"
		mocks.BlueprintHandler.LoadBlueprintFunc = func(...string) error {
			return fmt.Errorf("%s", expectedError)
		}
		composer := createComposerWithMocks(mocks)

		// When generating blueprint
		result, err := composer.GenerateBlueprint()

		// Then error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got: %v", expectedError, err)
		}

		// And result should be nil
		if result != nil {
			t.Error("Expected nil result on error")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestComposer_generateGitignore(t *testing.T) {
	t.Run("CreatesNewFile", func(t *testing.T) {
		// Given a composer with temporary project root
		mocks := setupComposerMocks(t)
		composer := createComposerWithMocks(mocks)

		// When generating gitignore
		err := composer.generateGitignore()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And .gitignore file should be created
		gitignorePath := filepath.Join(mocks.Runtime.ProjectRoot, ".gitignore")
		content, readErr := os.ReadFile(gitignorePath)
		if readErr != nil {
			t.Fatalf("Expected .gitignore to be created, got error: %v", readErr)
		}

		// And file should contain Windsor entries
		contentStr := string(content)
		if !strings.Contains(contentStr, "# managed by windsor cli") {
			t.Error("Expected .gitignore to contain Windsor header")
		}

		if !strings.Contains(contentStr, ".windsor/") {
			t.Error("Expected .gitignore to contain .windsor/ entry")
		}
	})

	t.Run("UpdatesExistingFile", func(t *testing.T) {
		// Given a composer with existing .gitignore
		mocks := setupComposerMocks(t)
		existingContent := "existing-entry\n"
		gitignorePath := filepath.Join(mocks.Runtime.ProjectRoot, ".gitignore")
		if err := os.WriteFile(gitignorePath, []byte(existingContent), 0644); err != nil {
			t.Fatalf("Failed to create existing .gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When generating gitignore
		err := composer.generateGitignore()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And file should contain both existing and new entries
		content, readErr := os.ReadFile(gitignorePath)
		if readErr != nil {
			t.Fatalf("Failed to read .gitignore: %v", readErr)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "existing-entry") {
			t.Error("Expected .gitignore to preserve existing entry")
		}

		if !strings.Contains(contentStr, ".windsor/") {
			t.Error("Expected .gitignore to contain new Windsor entry")
		}
	})

	t.Run("PreservesUserEntries", func(t *testing.T) {
		// Given a composer with existing .gitignore with user entries
		mocks := setupComposerMocks(t)
		existingContent := "user-entry-1\nuser-entry-2\n"
		gitignorePath := filepath.Join(mocks.Runtime.ProjectRoot, ".gitignore")
		if err := os.WriteFile(gitignorePath, []byte(existingContent), 0644); err != nil {
			t.Fatalf("Failed to create existing .gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When generating gitignore
		err := composer.generateGitignore()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And file should contain all user entries
		content, readErr := os.ReadFile(gitignorePath)
		if readErr != nil {
			t.Fatalf("Failed to read .gitignore: %v", readErr)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "user-entry-1") {
			t.Error("Expected .gitignore to preserve user-entry-1")
		}

		if !strings.Contains(contentStr, "user-entry-2") {
			t.Error("Expected .gitignore to preserve user-entry-2")
		}
	})

	t.Run("HandlesCommentedEntries", func(t *testing.T) {
		// Given a composer with existing .gitignore with commented Windsor entry
		mocks := setupComposerMocks(t)
		existingContent := "# .windsor/\nuser-entry\n"
		gitignorePath := filepath.Join(mocks.Runtime.ProjectRoot, ".gitignore")
		if err := os.WriteFile(gitignorePath, []byte(existingContent), 0644); err != nil {
			t.Fatalf("Failed to create existing .gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When generating gitignore
		err := composer.generateGitignore()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And file should not duplicate the entry (commented entry stays commented)
		content, readErr := os.ReadFile(gitignorePath)
		if readErr != nil {
			t.Fatalf("Failed to read .gitignore: %v", readErr)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "# .windsor/") {
			t.Error("Expected commented entry # .windsor/ to remain in file")
		}
		lines := strings.Split(contentStr, "\n")
		uncommentedCount := 0
		for _, line := range lines {
			if strings.TrimSpace(line) == ".windsor/" {
				uncommentedCount++
			}
		}
		if uncommentedCount > 0 {
			t.Errorf("Expected .windsor/ to not be added when commented, got %d uncommented occurrences", uncommentedCount)
		}
	})

	t.Run("AddsMissingEntries", func(t *testing.T) {
		// Given a composer with existing .gitignore missing some Windsor entries
		mocks := setupComposerMocks(t)
		existingContent := "# managed by windsor cli\n.windsor/\n"
		gitignorePath := filepath.Join(mocks.Runtime.ProjectRoot, ".gitignore")
		if err := os.WriteFile(gitignorePath, []byte(existingContent), 0644); err != nil {
			t.Fatalf("Failed to create existing .gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When generating gitignore
		err := composer.generateGitignore()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And file should contain all Windsor entries
		content, readErr := os.ReadFile(gitignorePath)
		if readErr != nil {
			t.Fatalf("Failed to read .gitignore: %v", readErr)
		}

		contentStr := string(content)
		requiredEntries := []string{".windsor/", ".volumes/", "terraform/**/backend_override.tf", "terraform/**/providers_override.tf"}
		for _, entry := range requiredEntries {
			if !strings.Contains(contentStr, entry) {
				t.Errorf("Expected .gitignore to contain %s", entry)
			}
		}
	})

	t.Run("HandlesTrailingNewline", func(t *testing.T) {
		// Given a composer with existing .gitignore without trailing newline
		mocks := setupComposerMocks(t)
		existingContent := "user-entry"
		gitignorePath := filepath.Join(mocks.Runtime.ProjectRoot, ".gitignore")
		if err := os.WriteFile(gitignorePath, []byte(existingContent), 0644); err != nil {
			t.Fatalf("Failed to create existing .gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When generating gitignore
		err := composer.generateGitignore()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And file should end with newline
		content, readErr := os.ReadFile(gitignorePath)
		if readErr != nil {
			t.Fatalf("Failed to read .gitignore: %v", readErr)
		}

		if len(content) > 0 && content[len(content)-1] != '\n' {
			t.Error("Expected .gitignore to end with newline")
		}
	})

	t.Run("ErrorFromWriteFile", func(t *testing.T) {
		// Given a composer with existing .gitignore that can be read
		mocks := setupComposerMocks(t)
		existingContent := "existing-entry\n"
		gitignorePath := filepath.Join(mocks.Runtime.ProjectRoot, ".gitignore")
		if err := os.WriteFile(gitignorePath, []byte(existingContent), 0644); err != nil {
			t.Fatalf("Failed to create existing .gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// Make the .gitignore file itself read-only to simulate write failure
		if err := os.Chmod(gitignorePath, 0444); err != nil {
			t.Fatalf("Failed to make .gitignore read-only: %v", err)
		}
		defer func() {
			os.Chmod(gitignorePath, 0644)
		}()

		// When generating gitignore
		err := composer.generateGitignore()

		// Then error should be returned (write will fail due to read-only file)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to write to .gitignore") && !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("Expected error about writing .gitignore or permission denied, got: %v", err)
		}
	})

	t.Run("HandlesEmptyExistingFile", func(t *testing.T) {
		// Given a composer with empty existing .gitignore
		mocks := setupComposerMocks(t)
		gitignorePath := filepath.Join(mocks.Runtime.ProjectRoot, ".gitignore")
		if err := os.WriteFile(gitignorePath, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to create empty .gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When generating gitignore
		err := composer.generateGitignore()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And file should contain Windsor entries
		content, readErr := os.ReadFile(gitignorePath)
		if readErr != nil {
			t.Fatalf("Failed to read .gitignore: %v", readErr)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "# managed by windsor cli") {
			t.Error("Expected .gitignore to contain Windsor header")
		}
	})

	t.Run("HandlesFileWithOnlyComments", func(t *testing.T) {
		// Given a composer with existing .gitignore with only comments
		mocks := setupComposerMocks(t)
		existingContent := "# comment 1\n# comment 2\n"
		gitignorePath := filepath.Join(mocks.Runtime.ProjectRoot, ".gitignore")
		if err := os.WriteFile(gitignorePath, []byte(existingContent), 0644); err != nil {
			t.Fatalf("Failed to create existing .gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When generating gitignore
		err := composer.generateGitignore()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And file should contain both comments and Windsor entries
		content, readErr := os.ReadFile(gitignorePath)
		if readErr != nil {
			t.Fatalf("Failed to read .gitignore: %v", readErr)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "# comment 1") {
			t.Error("Expected .gitignore to preserve comment 1")
		}

		if !strings.Contains(contentStr, ".windsor/") {
			t.Error("Expected .gitignore to contain Windsor entry")
		}
	})
}

func TestComposer_normalizeGitignoreComment(t *testing.T) {
	t.Run("SimpleCommentFormat", func(t *testing.T) {
		// Given a simple comment line
		line := "# .windsor/"

		// When normalizing
		result := normalizeGitignoreComment(line)

		// Then result should be the uncommented entry
		expected := ".windsor/"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("MultipleHashSymbols", func(t *testing.T) {
		// Given a comment line with multiple hash symbols
		line := "## .windsor/"

		// When normalizing
		result := normalizeGitignoreComment(line)

		// Then result should be the uncommented entry
		expected := ".windsor/"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("CommentWithLeadingWhitespace", func(t *testing.T) {
		// Given a comment line with leading whitespace
		line := "  # .windsor/"

		// When normalizing
		result := normalizeGitignoreComment(line)

		// Then result should be the uncommented entry
		expected := ".windsor/"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("CommentWithTrailingWhitespace", func(t *testing.T) {
		// Given a comment line with trailing whitespace
		line := "# .windsor/  "

		// When normalizing
		result := normalizeGitignoreComment(line)

		// Then result should be the uncommented entry without trailing whitespace
		expected := ".windsor/"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("NonCommentLine", func(t *testing.T) {
		// Given a non-comment line
		line := ".windsor/"

		// When normalizing
		result := normalizeGitignoreComment(line)

		// Then result should be empty string
		if result != "" {
			t.Errorf("Expected empty string, got %s", result)
		}
	})

	t.Run("OnlyHashSymbol", func(t *testing.T) {
		// Given a line with only hash symbol
		line := "#"

		// When normalizing
		result := normalizeGitignoreComment(line)

		// Then result should be empty string
		if result != "" {
			t.Errorf("Expected empty string, got %s", result)
		}
	})

	t.Run("CommentWithMultipleSpaces", func(t *testing.T) {
		// Given a comment line with multiple spaces
		line := "#   .windsor/  "

		// When normalizing
		result := normalizeGitignoreComment(line)

		// Then result should be the uncommented entry
		expected := ".windsor/"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})
}
