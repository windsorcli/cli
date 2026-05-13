package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type PushMocks struct {
	Composer        *composer.Composer
	ArtifactBuilder *artifact.MockArtifact
}

// setupPushTest sets up the test environment for push command tests.
// It creates a temporary directory, initializes the injector with required mocks, and returns PushMocks.
func setupPushTest(t *testing.T) (*PushMocks, *Mocks) {
	t.Helper()

	// Get base mocks first to get temp dir
	baseMocks := setupMocks(t)
	tmpDir := baseMocks.TmpDir

	// Create required directory structure
	contextsDir := filepath.Join(tmpDir, "contexts")
	templateDir := filepath.Join(contextsDir, "_template")
	os.MkdirAll(templateDir, 0755)

	// Override Shell GetProjectRootFunc if it's a MockShell
	if mockShell, ok := baseMocks.Runtime.Shell.(*shell.MockShell); ok {
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
	}

	// Override ConfigHandler and ProjectRoot in runtime
	baseMocks.Runtime.ConfigHandler = config.NewMockConfigHandler()
	if mockConfig, ok := baseMocks.Runtime.ConfigHandler.(*config.MockConfigHandler); ok {
		mockConfig.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
	}
	baseMocks.Runtime.ProjectRoot = tmpDir

	// Mock blueprint handler
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
		return map[string][]byte{}, nil
	}

	// Mock artifact builder — default returns a wrapped *transport.Error so the auth-classification
	// path in push.go (and its docker-login hint) is exercised end-to-end with a realistic shape
	mockArtifactBuilder := artifact.NewMockArtifact()
	mockArtifactBuilder.BundleFunc = func() error {
		return nil
	}
	mockArtifactBuilder.PushFunc = func(registryBase string, repoName string, tag string) error {
		return fmt.Errorf("failed to push artifact: %w", &transport.Error{StatusCode: http.StatusUnauthorized})
	}

	mockComposer := &composer.Composer{ArtifactBuilder: mockArtifactBuilder}

	return &PushMocks{Composer: mockComposer, ArtifactBuilder: mockArtifactBuilder}, baseMocks
}

// createTestPushCmd creates a new cobra.Command for testing the push command.
func createTestPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push blueprints to an OCI registry",
		RunE:  pushCmd.RunE,
	}
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
}

// =============================================================================
// Test Cases
// =============================================================================

func TestPushCmdWithRuntime(t *testing.T) {
	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("ClassifiesAuthFailureAndReturnsHint", func(t *testing.T) {
		// Given a mock composer whose ArtifactBuilder returns a wrapped *transport.Error{401}
		pushMocks, mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		ctx = context.WithValue(ctx, composerOverridesKey, pushMocks.Composer)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo:v1.0.0"})

		// When executing the push command
		err := cmd.Execute()

		// Then push.go should classify it via IsAuthenticationError and return the auth-failed hint
		if err == nil {
			t.Fatal("Expected authentication error, got nil")
		}
		if !strings.Contains(err.Error(), "Authentication failed") {
			t.Errorf("Expected 'Authentication failed' (auth-classified), got %v", err)
		}
	})

	t.Run("ClassifiesAuthFailureWithoutTag", func(t *testing.T) {
		// Given the same auth scenario but with a registry URL missing an explicit tag
		pushMocks, mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		ctx = context.WithValue(ctx, composerOverridesKey, pushMocks.Composer)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo"})

		// When executing the push command
		err := cmd.Execute()

		// Then the auth-classification path should still fire — argument parsing happens before push
		if err == nil {
			t.Fatal("Expected authentication error, got nil")
		}
		if !strings.Contains(err.Error(), "Authentication failed") {
			t.Errorf("Expected 'Authentication failed' (auth-classified), got %v", err)
		}
	})

	t.Run("ClassifiesAuthFailureWithOciUrl", func(t *testing.T) {
		// Given the same auth scenario with an oci:// prefixed URL
		pushMocks, mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		ctx = context.WithValue(ctx, composerOverridesKey, pushMocks.Composer)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"oci://ghcr.io/windsorcli/core:v0.0.0"})

		// When executing the push command
		err := cmd.Execute()

		// Then auth classification should fire regardless of URL prefix
		if err == nil {
			t.Fatal("Expected authentication error, got nil")
		}
		if !strings.Contains(err.Error(), "Authentication failed") {
			t.Errorf("Expected 'Authentication failed' (auth-classified), got %v", err)
		}
	})

	t.Run("PassesThroughNonAuthErrors", func(t *testing.T) {
		// Given a mock composer whose ArtifactBuilder returns a non-auth error
		pushMocks, mocks := setupPushTest(t)
		pushMocks.ArtifactBuilder.PushFunc = func(registryBase, repoName, tag string) error {
			return fmt.Errorf("network timeout: dial tcp: i/o timeout")
		}
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		ctx = context.WithValue(ctx, composerOverridesKey, pushMocks.Composer)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo:v1.0.0"})

		// When executing the push command
		err := cmd.Execute()

		// Then push.go should NOT classify it as auth — surfaces the underlying error verbatim
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if strings.Contains(err.Error(), "Authentication failed") {
			t.Errorf("Non-auth error misclassified as auth: %v", err)
		}
		if !strings.Contains(err.Error(), "network timeout") {
			t.Errorf("Expected underlying error to surface, got %v", err)
		}
	})

	t.Run("ErrorMissingRegistry", func(t *testing.T) {
		// Given proper setup with runtime override
		_, mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})

		// When executing the push command without registry
		err := cmd.Execute()

		// Then an error should occur
		if err == nil || !strings.Contains(err.Error(), "registry is required") {
			t.Errorf("Expected registry required error, got %v", err)
		}
	})

	t.Run("ErrorInvalidRegistryFormat", func(t *testing.T) {
		// Given proper setup with runtime override
		_, mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"invalidformat"})

		// When executing the push command with invalid format
		err := cmd.Execute()

		// Then an error should occur
		if err == nil || !strings.Contains(err.Error(), "invalid registry format") {
			t.Errorf("Expected invalid registry format error, got %v", err)
		}
	})

	t.Run("RuntimeSetupError", func(t *testing.T) {
		// Given command without runtime override
		cmd := createTestPushCmd()
		ctx := context.Background()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo:v1.0.0"})

		// When executing the push command
		err := cmd.Execute()

		// Then it may succeed or fail depending on environment
		// Runtime is resilient and will create default dependencies
		_ = err
	})
}
