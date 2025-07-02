package cmd

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/bundler"
	"github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Types
// =============================================================================

// Extend Mocks with additional fields needed for push command tests
type PushMocks struct {
	*Mocks
	ArtifactBuilder  *bundler.MockArtifact
	TemplateBundler  *bundler.MockBundler
	KustomizeBundler *bundler.MockBundler
}

// =============================================================================
// Helpers
// =============================================================================

func setupPushMocks(t *testing.T) *PushMocks {
	setup := func(t *testing.T) *PushMocks {
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
		artifactBuilder := bundler.NewMockArtifact()
		artifactBuilder.InitializeFunc = func(injector di.Injector) error { return nil }
		artifactBuilder.AddFileFunc = func(path string, content []byte) error { return nil }
		artifactBuilder.PushFunc = func(registry string, tag string) error { return nil }

		// Create mock template bundler
		templateBundler := bundler.NewMockBundler()
		templateBundler.InitializeFunc = func(injector di.Injector) error { return nil }
		templateBundler.BundleFunc = func(artifact bundler.Artifact) error { return nil }

		// Create mock kustomize bundler
		kustomizeBundler := bundler.NewMockBundler()
		kustomizeBundler.InitializeFunc = func(injector di.Injector) error { return nil }
		kustomizeBundler.BundleFunc = func(artifact bundler.Artifact) error { return nil }

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

		return &PushMocks{
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

func TestPushCmd(t *testing.T) {
	t.Run("SuccessWithTag", func(t *testing.T) {
		// Given a properly configured push environment
		mocks := setupPushMocks(t)

		// When executing the push command with registry and tag
		rootCmd.SetArgs([]string{"push", "registry.example.com/repo:v1.0.0"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithoutTag", func(t *testing.T) {
		// Given a properly configured push environment
		mocks := setupPushMocks(t)

		// When executing the push command without tag (relies on metadata.yaml)
		rootCmd.SetArgs([]string{"push", "registry.example.com/repo"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorMissingRegistry", func(t *testing.T) {
		// Given a properly configured push environment
		mocks := setupPushMocks(t)

		// When executing the push command without registry
		rootCmd.SetArgs([]string{"push"})
		err := Execute(mocks.Controller)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "registry is required: windsor push registry/repo[:tag]"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorInitializingController", func(t *testing.T) {
		// Given a push environment with failing controller initialization
		mocks := setupPushMocks(t)
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			return fmt.Errorf("failed to initialize controller")
		}

		// When executing the push command
		rootCmd.SetArgs([]string{"push", "registry.example.com/repo"})
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
		// Given a push environment with no artifact builder
		mocks := setupPushMocks(t)
		// Don't register the artifact builder to simulate it being unavailable
		mocks.Injector.Register("artifactBuilder", nil)

		// When executing the push command
		rootCmd.SetArgs([]string{"push", "registry.example.com/repo"})
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
		// Given a push environment with failing template bundler
		mocks := setupPushMocks(t)
		mocks.TemplateBundler.BundleFunc = func(artifact bundler.Artifact) error {
			return fmt.Errorf("template bundling failed")
		}

		// When executing the push command
		rootCmd.SetArgs([]string{"push", "registry.example.com/repo"})
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
		// Given a push environment with failing kustomize bundler
		mocks := setupPushMocks(t)
		mocks.KustomizeBundler.BundleFunc = func(artifact bundler.Artifact) error {
			return fmt.Errorf("kustomize bundling failed")
		}

		// When executing the push command
		rootCmd.SetArgs([]string{"push", "registry.example.com/repo"})
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

	t.Run("ErrorArtifactPushFails", func(t *testing.T) {
		// Given a push environment with failing artifact push
		mocks := setupPushMocks(t)
		mocks.ArtifactBuilder.PushFunc = func(registry string, tag string) error {
			return fmt.Errorf("push to registry failed")
		}

		// When executing the push command
		rootCmd.SetArgs([]string{"push", "registry.example.com/repo"})
		err := Execute(mocks.Controller)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "failed to push artifact: push to registry failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("VerifyBundlerRequirementPassed", func(t *testing.T) {
		// Given a push environment that validates requirements
		mocks := setupPushMocks(t)
		var receivedRequirements controller.Requirements
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			receivedRequirements = req
			return nil
		}

		// When executing the push command
		rootCmd.SetArgs([]string{"push", "registry.example.com/repo"})
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
		if receivedRequirements.CommandName != "push" {
			t.Errorf("Expected command name to be 'push', got %s", receivedRequirements.CommandName)
		}
	})

	t.Run("VerifyAllBundlersAreCalled", func(t *testing.T) {
		// Given a push environment that tracks bundler calls
		mocks := setupPushMocks(t)
		templateBundlerCalled := false
		kustomizeBundlerCalled := false

		mocks.TemplateBundler.BundleFunc = func(artifact bundler.Artifact) error {
			templateBundlerCalled = true
			return nil
		}
		mocks.KustomizeBundler.BundleFunc = func(artifact bundler.Artifact) error {
			kustomizeBundlerCalled = true
			return nil
		}

		// When executing the push command
		rootCmd.SetArgs([]string{"push", "registry.example.com/repo"})
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
		// Given a push environment that tracks artifact parameters
		mocks := setupPushMocks(t)
		var receivedRegistry, receivedTag string

		mocks.ArtifactBuilder.PushFunc = func(registry string, tag string) error {
			receivedRegistry = registry
			receivedTag = tag
			return nil
		}

		// When executing the push command with registry and tag
		rootCmd.SetArgs([]string{"push", "registry.example.com/myapp:v2.1.0"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the correct parameters should be passed
		if receivedRegistry != "registry.example.com/myapp" {
			t.Errorf("Expected registry 'registry.example.com/myapp', got %s", receivedRegistry)
		}
		if receivedTag != "v2.1.0" {
			t.Errorf("Expected tag 'v2.1.0', got %s", receivedTag)
		}
	})

	t.Run("VerifyEmptyTagHandling", func(t *testing.T) {
		// Given a push environment that tracks artifact parameters
		mocks := setupPushMocks(t)
		var receivedTag string

		mocks.ArtifactBuilder.PushFunc = func(registry string, tag string) error {
			receivedTag = tag
			return nil
		}

		// When executing the push command without tag (registry only)
		rootCmd.SetArgs([]string{"push", "registry.example.com/repo"})
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
		// Given a push environment with no bundlers
		mocks := setupPushMocks(t)
		// Don't register any bundlers to simulate empty list
		mocks.Injector.Register("templateBundler", nil)
		mocks.Injector.Register("kustomizeBundler", nil)

		// When executing the push command
		rootCmd.SetArgs([]string{"push", "registry.example.com/repo"})
		err := Execute(mocks.Controller)

		// Then no error should be returned (empty bundlers list is valid)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
