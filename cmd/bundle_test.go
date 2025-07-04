package cmd

import (
	"fmt"
	"os"
	"testing"

	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Types
// =============================================================================

// Extend Mocks with additional fields needed for bundle command tests
type BundleMocks struct {
	*Mocks
	ArtifactBuilder  *artifact.MockArtifact
	TemplateBundler  *artifact.MockBundler
	KustomizeBundler *artifact.MockBundler
}

// =============================================================================
// Helpers
// =============================================================================

func setupBundleMocks(t *testing.T) *BundleMocks {
	setup := func(t *testing.T) *BundleMocks {
		t.Helper()
		opts := &SetupOptions{
			ConfigStr: `
contexts:
  default:
    bundler:
      enabled: true`,
		}
		mocks := setupMocks(t, opts)

		// Create mock artifact builder
		artifactBuilder := artifact.NewMockArtifact()
		artifactBuilder.InitializeFunc = func(injector di.Injector) error { return nil }
		artifactBuilder.AddFileFunc = func(path string, content []byte, mode os.FileMode) error { return nil }
		artifactBuilder.CreateFunc = func(outputPath string, tag string) (string, error) {
			if tag != "" {
				return "test-v1.0.0.tar.gz", nil
			}
			return "blueprint-v1.0.0.tar.gz", nil
		}

		// Create mock template bundler
		templateBundler := artifact.NewMockBundler()
		templateBundler.InitializeFunc = func(injector di.Injector) error { return nil }
		templateBundler.BundleFunc = func(art artifact.Artifact) error { return nil }

		// Create mock kustomize bundler
		kustomizeBundler := artifact.NewMockBundler()
		kustomizeBundler.InitializeFunc = func(injector di.Injector) error { return nil }
		kustomizeBundler.BundleFunc = func(art artifact.Artifact) error { return nil }

		// Set up controller mocks
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			if req.Bundler {
				return nil
			}
			return fmt.Errorf("bundler requirement not met")
		}

		// Register bundler components in the injector
		mocks.Injector.Register("artifactBuilder", artifactBuilder)
		mocks.Injector.Register("templateBundler", templateBundler)
		mocks.Injector.Register("kustomizeBundler", kustomizeBundler)

		return &BundleMocks{
			Mocks:            mocks,
			ArtifactBuilder:  artifactBuilder,
			TemplateBundler:  templateBundler,
			KustomizeBundler: kustomizeBundler,
		}
	}

	return setup(t)
}

// =============================================================================
// Tests
// =============================================================================

func TestBundleCmd(t *testing.T) {
	t.Run("SuccessWithDefaults", func(t *testing.T) {
		// Given a properly configured bundle environment
		mocks := setupBundleMocks(t)

		// When executing the bundle command with defaults
		rootCmd.SetArgs([]string{"bundle"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithTag", func(t *testing.T) {
		// Given a properly configured bundle environment
		mocks := setupBundleMocks(t)

		// When executing the bundle command with tag
		rootCmd.SetArgs([]string{"bundle", "--tag", "myproject:v2.0.0"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithCustomOutput", func(t *testing.T) {
		// Given a properly configured bundle environment
		mocks := setupBundleMocks(t)

		// When executing the bundle command with custom output path
		rootCmd.SetArgs([]string{"bundle", "--output", "/tmp/my-bundle.tar.gz", "--tag", "test:v1.0.0"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithShortFlags", func(t *testing.T) {
		// Given a properly configured bundle environment
		mocks := setupBundleMocks(t)

		// When executing the bundle command with short flags
		rootCmd.SetArgs([]string{"bundle", "-o", "output/", "-t", "myapp:v1.2.3"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorInitializingController", func(t *testing.T) {
		// Given a bundle environment with failing controller initialization
		mocks := setupBundleMocks(t)
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			return fmt.Errorf("failed to initialize controller")
		}

		// When executing the bundle command
		rootCmd.SetArgs([]string{"bundle"})
		err := Execute(mocks.Controller)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "failed to initialize controller: failed to initialize controller"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorArtifactBuilderNotAvailable", func(t *testing.T) {
		// Given a bundle environment with no artifact builder
		mocks := setupBundleMocks(t)
		// Don't register the artifact builder to simulate it being unavailable
		mocks.Injector.Register("artifactBuilder", nil)

		// When executing the bundle command
		rootCmd.SetArgs([]string{"bundle"})
		err := Execute(mocks.Controller)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "artifact builder not available"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorTemplateBundlerFails", func(t *testing.T) {
		// Given a bundle environment with failing template bundler
		mocks := setupBundleMocks(t)
		mocks.TemplateBundler.BundleFunc = func(artifact artifact.Artifact) error {
			return fmt.Errorf("template bundling failed")
		}

		// When executing the bundle command
		rootCmd.SetArgs([]string{"bundle"})
		err := Execute(mocks.Controller)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "bundling failed: template bundling failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorKustomizeBundlerFails", func(t *testing.T) {
		// Given a bundle environment with failing kustomize bundler
		mocks := setupBundleMocks(t)
		mocks.KustomizeBundler.BundleFunc = func(artifact artifact.Artifact) error {
			return fmt.Errorf("kustomize bundling failed")
		}

		// When executing the bundle command
		rootCmd.SetArgs([]string{"bundle"})
		err := Execute(mocks.Controller)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "bundling failed: kustomize bundling failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorArtifactCreationFails", func(t *testing.T) {
		// Given a bundle environment with failing artifact creation
		mocks := setupBundleMocks(t)
		mocks.ArtifactBuilder.CreateFunc = func(outputPath string, tag string) (string, error) {
			return "", fmt.Errorf("artifact creation failed")
		}

		// When executing the bundle command
		rootCmd.SetArgs([]string{"bundle"})
		err := Execute(mocks.Controller)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "failed to create artifact: artifact creation failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("VerifyBundlerRequirementPassed", func(t *testing.T) {
		// Given a bundle environment that validates requirements
		mocks := setupBundleMocks(t)
		var receivedRequirements controller.Requirements
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			receivedRequirements = req
			return nil
		}

		// When executing the bundle command
		rootCmd.SetArgs([]string{"bundle"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the bundler requirement should be set
		if !receivedRequirements.Bundler {
			t.Error("Expected bundler requirement to be true")
		}

		// And the command name should be correct
		if receivedRequirements.CommandName != "bundle" {
			t.Errorf("Expected command name to be 'bundle', got %s", receivedRequirements.CommandName)
		}
	})

	t.Run("VerifyAllBundlersAreCalled", func(t *testing.T) {
		// Given a bundle environment that tracks bundler calls
		mocks := setupBundleMocks(t)
		templateBundlerCalled := false
		kustomizeBundlerCalled := false

		mocks.TemplateBundler.BundleFunc = func(artifact artifact.Artifact) error {
			templateBundlerCalled = true
			return nil
		}
		mocks.KustomizeBundler.BundleFunc = func(artifact artifact.Artifact) error {
			kustomizeBundlerCalled = true
			return nil
		}

		// When executing the bundle command
		rootCmd.SetArgs([]string{"bundle"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And both bundlers should be called
		if !templateBundlerCalled {
			t.Error("Expected template bundler to be called")
		}
		if !kustomizeBundlerCalled {
			t.Error("Expected kustomize bundler to be called")
		}
	})

	t.Run("VerifyCorrectParametersPassedToArtifact", func(t *testing.T) {
		// Given a bundle environment that tracks artifact parameters
		mocks := setupBundleMocks(t)
		var receivedOutputPath, receivedTag string

		mocks.ArtifactBuilder.CreateFunc = func(outputPath string, tag string) (string, error) {
			receivedOutputPath = outputPath
			receivedTag = tag
			return "test-result.tar.gz", nil
		}

		// When executing the bundle command with specific parameters
		rootCmd.SetArgs([]string{"bundle", "--output", "/custom/path", "--tag", "myapp:v2.1.0"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the correct parameters should be passed
		if receivedOutputPath != "/custom/path" {
			t.Errorf("Expected output path '/custom/path', got %s", receivedOutputPath)
		}
		if receivedTag != "myapp:v2.1.0" {
			t.Errorf("Expected tag 'myapp:v2.1.0', got %s", receivedTag)
		}
	})

	t.Run("VerifyDefaultOutputPath", func(t *testing.T) {
		// Given a fresh bundle environment that tracks artifact parameters
		mocks := setupBundleMocks(t)
		var receivedOutputPath string

		// Reset command flags to avoid state contamination
		bundleCmd.ResetFlags()
		bundleCmd.Flags().StringP("output", "o", ".", "Output path for bundle archive (file or directory)")
		bundleCmd.Flags().StringP("tag", "t", "", "Tag in 'name:version' format (required if no metadata.yaml or missing name/version)")

		// Create a fresh artifact builder to avoid state contamination
		freshArtifactBuilder := artifact.NewMockArtifact()
		freshArtifactBuilder.InitializeFunc = func(injector di.Injector) error { return nil }
		freshArtifactBuilder.AddFileFunc = func(path string, content []byte, mode os.FileMode) error { return nil }
		freshArtifactBuilder.CreateFunc = func(outputPath string, tag string) (string, error) {
			receivedOutputPath = outputPath
			return "test-result.tar.gz", nil
		}
		mocks.Injector.Register("artifactBuilder", freshArtifactBuilder)

		// When executing the bundle command without output flag
		rootCmd.SetArgs([]string{"bundle", "--tag", "test:v1.0.0"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the default output path should be used
		if receivedOutputPath != "." {
			t.Errorf("Expected default output path '.', got %s", receivedOutputPath)
		}
	})

	t.Run("VerifyEmptyTagHandling", func(t *testing.T) {
		// Given a fresh bundle environment that tracks artifact parameters
		mocks := setupBundleMocks(t)
		var receivedTag string

		// Reset command flags to avoid state contamination
		bundleCmd.ResetFlags()
		bundleCmd.Flags().StringP("output", "o", ".", "Output path for bundle archive (file or directory)")
		bundleCmd.Flags().StringP("tag", "t", "", "Tag in 'name:version' format (required if no metadata.yaml or missing name/version)")

		// Create a fresh artifact builder to avoid state contamination
		freshArtifactBuilder := artifact.NewMockArtifact()
		freshArtifactBuilder.InitializeFunc = func(injector di.Injector) error { return nil }
		freshArtifactBuilder.AddFileFunc = func(path string, content []byte, mode os.FileMode) error { return nil }
		freshArtifactBuilder.CreateFunc = func(outputPath string, tag string) (string, error) {
			receivedTag = tag
			return "test-result.tar.gz", nil
		}
		mocks.Injector.Register("artifactBuilder", freshArtifactBuilder)

		// When executing the bundle command without tag flag
		rootCmd.SetArgs([]string{"bundle"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And an empty tag should be passed
		if receivedTag != "" {
			t.Errorf("Expected empty tag, got %s", receivedTag)
		}
	})

	t.Run("VerifyNoBundlersHandling", func(t *testing.T) {
		// Given a bundle environment with no bundlers
		mocks := setupBundleMocks(t)
		// Don't register any bundlers to simulate empty list
		mocks.Injector.Register("templateBundler", nil)
		mocks.Injector.Register("kustomizeBundler", nil)

		// When executing the bundle command
		rootCmd.SetArgs([]string{"bundle"})
		err := Execute(mocks.Controller)

		// Then no error should be returned (empty bundlers list is valid)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
