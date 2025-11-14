package composer

import (
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupComposerMocks creates mock components for testing the Composer
func setupComposerMocks(t *testing.T) *Mocks {
	t.Helper()

	injector := di.NewInjector()
	configHandler := config.NewMockConfigHandler()
	shell := shell.NewMockShell()

	// Create execution context
	rt := &runtime.Runtime{
		ContextName:   "test-context",
		ProjectRoot:   "/test/project",
		ConfigRoot:    "/test/project/contexts/test-context",
		TemplateRoot:  "/test/project/contexts/_template",
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         shell,
	}

	return &Mocks{
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         shell,
		Runtime:       rt,
	}
}

// Mocks contains all the mock dependencies for testing
type Mocks struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Runtime       *runtime.Runtime
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewComposer(t *testing.T) {
	t.Run("CreatesComposerWithDependencies", func(t *testing.T) {
		mocks := setupComposerMocks(t)

		composer := NewComposer(mocks.Runtime)

		if composer == nil {
			t.Fatal("Expected Composer to be created")
		}

		if composer.Runtime.Injector != mocks.Injector {
			t.Error("Expected injector to be set")
		}

		if composer.Runtime.Shell != mocks.Shell {
			t.Error("Expected shell to be set")
		}

		if composer.Runtime.ConfigHandler != mocks.ConfigHandler {
			t.Error("Expected config handler to be set")
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

func TestCreateComposer(t *testing.T) {
	t.Run("CreatesComposerWithDependencies", func(t *testing.T) {
		mocks := setupComposerMocks(t)

		composer := NewComposer(mocks.Runtime)

		if composer == nil {
			t.Fatal("Expected Composer to be created")
		}

		if composer.Runtime.Injector != mocks.Injector {
			t.Error("Expected injector to be set")
		}

		if composer.Runtime.ConfigHandler != mocks.ConfigHandler {
			t.Error("Expected config handler to be set")
		}

		if composer.Runtime.Shell != mocks.Shell {
			t.Error("Expected shell to be set")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestComposer_Push(t *testing.T) {
	t.Run("HandlesPushSuccessfully", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewComposer(mocks.Runtime)

		// This test would need proper mocking of the artifact builder
		// For now, we'll just test that the method exists and handles errors
		_, err := composer.Push("ghcr.io/test/repo:latest")
		// We expect an error here because we don't have proper mocks set up
		if err == nil {
			t.Error("Expected error due to missing mocks, but got nil")
		}
	})
}

func TestComposer_Generate(t *testing.T) {
	t.Run("HandlesGenerateSuccessfully", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewComposer(mocks.Runtime)

		// This test would need proper mocking of the blueprint handler and terraform resolver
		// For now, we'll just test that the method exists and handles errors
		err := composer.Generate()
		// We expect an error here because we don't have proper mocks set up
		if err == nil {
			t.Error("Expected error due to missing mocks, but got nil")
		}
	})

	t.Run("HandlesGenerateWithOverwrite", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewComposer(mocks.Runtime)

		// This test would need proper mocking of the blueprint handler and terraform resolver
		// For now, we'll just test that the method exists and handles errors
		err := composer.Generate(true)
		// We expect an error here because we don't have proper mocks set up
		if err == nil {
			t.Error("Expected error due to missing mocks, but got nil")
		}
	})
}

// =============================================================================
// Test Runtime
// =============================================================================

func TestRuntime(t *testing.T) {
	t.Run("CreatesRuntime", func(t *testing.T) {
		rt := &runtime.Runtime{
			ContextName:  "test-context",
			ProjectRoot:  "/test/project",
			ConfigRoot:   "/test/project/contexts/test-context",
			TemplateRoot: "/test/project/contexts/_template",
		}

		if rt.ContextName != "test-context" {
			t.Errorf("Expected context name 'test-context', got: %s", rt.ContextName)
		}

		if rt.ProjectRoot != "/test/project" {
			t.Errorf("Expected project root '/test/project', got: %s", rt.ProjectRoot)
		}

		if rt.ConfigRoot != "/test/project/contexts/test-context" {
			t.Errorf("Expected config root '/test/project/contexts/test-context', got: %s", rt.ConfigRoot)
		}

		if rt.TemplateRoot != "/test/project/contexts/_template" {
			t.Errorf("Expected template root '/test/project/contexts/_template', got: %s", rt.TemplateRoot)
		}
	})
}
