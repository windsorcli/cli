package composer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/composer/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
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
		Evaluator:     evaluator.NewExpressionEvaluator(configHandler, tmpDir, filepath.Join(tmpDir, "contexts", "_template")),
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
		if composer.shell != mocks.Shell {
			t.Error("Expected shell to be set")
		}

		if composer.configHandler != mocks.ConfigHandler {
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

	t.Run("ErrorFromWriteLocalGitignores", func(t *testing.T) {
		// Given mocks with writeLocalGitignores failing because ProjectRoot points
		// inside a regular file (MkdirAll of any child path returns ENOTDIR on all OSes)
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
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to create blocker file: %v", err)
		}
		mocks.Runtime.ProjectRoot = blocker
		composer := createComposerWithMocks(mocks)

		// When generating
		err := composer.Generate()

		// Then error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to write local gitignores") {
			t.Errorf("Expected error about writing local gitignores, got: %v", err)
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

// =============================================================================
// Test Private Methods
// =============================================================================

func TestComposer_writeLocalGitignores(t *testing.T) {
	t.Run("WritesWindsorMarkerWhenMissing", func(t *testing.T) {
		// Given a composer with no pre-existing .windsor/.gitignore
		mocks := setupComposerMocks(t)
		composer := createComposerWithMocks(mocks)

		// When writing local gitignores
		err := composer.writeLocalGitignores()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And .windsor/.gitignore should exist with self-ignore content
		path := filepath.Join(mocks.Runtime.ProjectRoot, ".windsor", ".gitignore")
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("Expected .windsor/.gitignore to be created, got error: %v", readErr)
		}
		if string(content) != "*\n" {
			t.Errorf("Expected self-ignore marker content, got: %q", string(content))
		}
	})

	t.Run("LeavesExistingWindsorGitignoreUntouched", func(t *testing.T) {
		// Given a composer with a pre-existing .windsor/.gitignore the user edited
		mocks := setupComposerMocks(t)
		windsorDir := filepath.Join(mocks.Runtime.ProjectRoot, ".windsor")
		if err := os.MkdirAll(windsorDir, 0750); err != nil {
			t.Fatalf("Failed to create .windsor: %v", err)
		}
		path := filepath.Join(windsorDir, ".gitignore")
		existing := "# user wrote this\n*.log\n"
		if err := os.WriteFile(path, []byte(existing), 0644); err != nil {
			t.Fatalf("Failed to seed .windsor/.gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When writing local gitignores
		err := composer.writeLocalGitignores()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the user's file should be unchanged
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("Failed to read .windsor/.gitignore: %v", readErr)
		}
		if string(content) != existing {
			t.Errorf("Expected user content preserved, got: %q", string(content))
		}
	})

	t.Run("SkipsVolumesWhenAbsent", func(t *testing.T) {
		// Given a composer with no .volumes/ directory
		mocks := setupComposerMocks(t)
		composer := createComposerWithMocks(mocks)

		// When writing local gitignores
		err := composer.writeLocalGitignores()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And .volumes/.gitignore should not be created
		path := filepath.Join(mocks.Runtime.ProjectRoot, ".volumes", ".gitignore")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("Expected .volumes/.gitignore to not exist, got stat err: %v", err)
		}
	})

	t.Run("WritesVolumesMarkerWhenPresent", func(t *testing.T) {
		// Given a composer with .volumes/ pre-existing
		mocks := setupComposerMocks(t)
		volumesDir := filepath.Join(mocks.Runtime.ProjectRoot, ".volumes")
		if err := os.MkdirAll(volumesDir, 0750); err != nil {
			t.Fatalf("Failed to create .volumes: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When writing local gitignores
		err := composer.writeLocalGitignores()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And .volumes/.gitignore should exist with self-ignore content
		path := filepath.Join(volumesDir, ".gitignore")
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("Expected .volumes/.gitignore to be created, got error: %v", readErr)
		}
		if string(content) != "*\n" {
			t.Errorf("Expected self-ignore marker content, got: %q", string(content))
		}
	})

	t.Run("WritesContextGitignoresForEachContext", func(t *testing.T) {
		// Given a composer with two contexts and a _template
		mocks := setupComposerMocks(t)
		root := mocks.Runtime.ProjectRoot
		for _, name := range []string{"local", "prod", "_template"} {
			if err := os.MkdirAll(filepath.Join(root, "contexts", name), 0750); err != nil {
				t.Fatalf("Failed to create context dir %s: %v", name, err)
			}
		}
		composer := createComposerWithMocks(mocks)

		// When writing local gitignores
		err := composer.writeLocalGitignores()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And each non-template context should have a .gitignore with cred dirs
		expected := ".kube/\n.talos/\n.omni/\n.aws/\n.azure/\n.gcp/\n.env\n"
		for _, name := range []string{"local", "prod"} {
			path := filepath.Join(root, "contexts", name, ".gitignore")
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("Expected contexts/%s/.gitignore, got error: %v", name, readErr)
			}
			if string(content) != expected {
				t.Errorf("contexts/%s/.gitignore content mismatch: got %q", name, string(content))
			}
		}

		// And _template should have no .gitignore created
		templatePath := filepath.Join(root, "contexts", "_template", ".gitignore")
		if _, err := os.Stat(templatePath); !os.IsNotExist(err) {
			t.Errorf("Expected contexts/_template/.gitignore to not exist, got stat err: %v", err)
		}
	})

	t.Run("BackfillsMissingLineIntoExistingContextGitignore", func(t *testing.T) {
		// Given a context whose .gitignore predates the ".env" line addition
		mocks := setupComposerMocks(t)
		root := mocks.Runtime.ProjectRoot
		contextDir := filepath.Join(root, "contexts", "local")
		if err := os.MkdirAll(contextDir, 0750); err != nil {
			t.Fatalf("Failed to create context dir: %v", err)
		}
		staleContent := ".kube/\n.talos/\n.omni/\n.aws/\n.azure/\n.gcp/\n"
		path := filepath.Join(contextDir, ".gitignore")
		if err := os.WriteFile(path, []byte(staleContent), 0644); err != nil {
			t.Fatalf("Failed to seed stale .gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When writing local gitignores
		err := composer.writeLocalGitignores()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the missing ".env" line should be appended
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("Failed to read .gitignore: %v", readErr)
		}
		if string(content) != staleContent+".env\n" {
			t.Errorf("Expected .env to be backfilled, got: %q", string(content))
		}
	})

	t.Run("PreservesUserLinesWhenBackfillingContextGitignore", func(t *testing.T) {
		// Given a context .gitignore with a user-added line and no ".env" line
		mocks := setupComposerMocks(t)
		root := mocks.Runtime.ProjectRoot
		contextDir := filepath.Join(root, "contexts", "local")
		if err := os.MkdirAll(contextDir, 0750); err != nil {
			t.Fatalf("Failed to create context dir: %v", err)
		}
		staleContent := ".kube/\n.talos/\n.omni/\n.aws/\n.azure/\n.gcp/\n*.bak\n"
		path := filepath.Join(contextDir, ".gitignore")
		if err := os.WriteFile(path, []byte(staleContent), 0644); err != nil {
			t.Fatalf("Failed to seed stale .gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When writing local gitignores
		if err := composer.writeLocalGitignores(); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the user's line should be preserved and ".env" appended
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("Failed to read .gitignore: %v", readErr)
		}
		if string(content) != staleContent+".env\n" {
			t.Errorf("Expected user line preserved and .env appended, got: %q", string(content))
		}
	})

	t.Run("IsIdempotentWhenEnvLineAlreadyPresent", func(t *testing.T) {
		// Given a context .gitignore that already has every current line
		mocks := setupComposerMocks(t)
		root := mocks.Runtime.ProjectRoot
		contextDir := filepath.Join(root, "contexts", "local")
		if err := os.MkdirAll(contextDir, 0750); err != nil {
			t.Fatalf("Failed to create context dir: %v", err)
		}
		path := filepath.Join(contextDir, ".gitignore")
		if err := os.WriteFile(path, []byte(contextIgnoreContent), 0644); err != nil {
			t.Fatalf("Failed to seed .gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When writing local gitignores
		if err := composer.writeLocalGitignores(); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the file should be unchanged
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("Failed to read .gitignore: %v", readErr)
		}
		if string(content) != contextIgnoreContent {
			t.Errorf("Expected .gitignore unchanged, got: %q", string(content))
		}
	})

	t.Run("SkipsContextsWhenDirectoryAbsent", func(t *testing.T) {
		// Given a composer with no contexts/ directory
		mocks := setupComposerMocks(t)
		composer := createComposerWithMocks(mocks)

		// When writing local gitignores
		err := composer.writeLocalGitignores()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("DoesNotTouchProjectRootGitignore", func(t *testing.T) {
		// Given a composer with a project-root .gitignore containing user content
		mocks := setupComposerMocks(t)
		rootGitignore := filepath.Join(mocks.Runtime.ProjectRoot, ".gitignore")
		userContent := "node_modules/\n*.log\n"
		if err := os.WriteFile(rootGitignore, []byte(userContent), 0644); err != nil {
			t.Fatalf("Failed to seed project root .gitignore: %v", err)
		}
		composer := createComposerWithMocks(mocks)

		// When writing local gitignores
		err := composer.writeLocalGitignores()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the project-root .gitignore should be byte-identical to what the user wrote
		content, readErr := os.ReadFile(rootGitignore)
		if readErr != nil {
			t.Fatalf("Failed to read project-root .gitignore: %v", readErr)
		}
		if string(content) != userContent {
			t.Errorf("Project-root .gitignore was modified; got: %q", string(content))
		}
	})

	t.Run("ErrorWhenWindsorParentUncreatable", func(t *testing.T) {
		// Given a composer whose ProjectRoot points inside a regular file, so
		// MkdirAll of any child path fails with ENOTDIR on every supported OS
		mocks := setupComposerMocks(t)
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to create blocker file: %v", err)
		}
		mocks.Runtime.ProjectRoot = blocker
		composer := createComposerWithMocks(mocks)

		// When writing local gitignores
		err := composer.writeLocalGitignores()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to ensure .windsor directory") {
			t.Errorf("Expected error about ensuring .windsor directory, got: %v", err)
		}
	})
}
