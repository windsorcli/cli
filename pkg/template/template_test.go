package template

import (
	"os"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Injector      di.Injector
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
}

type SetupOptions struct {
	ConfigStr string
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	injector := di.NewInjector()
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/mock/project", nil
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetContextFunc = func() string {
		return "mock-context"
	}
	mockConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
		return []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com"), nil
	}

	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)

	defaultConfigStr := `
contexts:
  mock-context:
    dns:
      domain: mock.domain.com
`

	mockConfigHandler.Initialize()
	mockConfigHandler.SetContext("mock-context")

	if err := mockConfigHandler.LoadConfigString(defaultConfigStr); err != nil {
		t.Fatalf("Failed to load default config string: %v", err)
	}
	if len(opts) > 0 && opts[0].ConfigStr != "" {
		if err := mockConfigHandler.LoadConfigString(opts[0].ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		os.Chdir(tmpDir)
	})

	return &Mocks{
		Injector:      injector,
		Shell:         mockShell,
		ConfigHandler: mockConfigHandler,
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseTemplate_NewBaseTemplate(t *testing.T) {
	t.Run("CreatesTemplateWithInjector", func(t *testing.T) {
		// Given an injector
		mocks := setupMocks(t)

		// When creating a new base template
		template := NewBaseTemplate(mocks.Injector)

		// Then the template should be properly initialized
		if template == nil {
			t.Fatal("Expected non-nil template")
		}

		// And basic fields should be set
		if template.injector == nil {
			t.Error("Expected injector to be set")
		}

		// And dependency fields should be nil until Initialize() is called
		if template.configHandler != nil {
			t.Error("Expected configHandler to be nil before Initialize()")
		}
		if template.shell != nil {
			t.Error("Expected shell to be nil before Initialize()")
		}
	})
}

func TestBaseTemplate_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*BaseTemplate, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		template := NewBaseTemplate(mocks.Injector)
		return template, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a template
		template, _ := setup(t)

		// When calling Initialize
		err := template.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And dependencies should be injected
		if template.configHandler == nil {
			t.Error("Expected configHandler to be set after Initialize()")
		}
		if template.shell == nil {
			t.Error("Expected shell to be set after Initialize()")
		}
	})

	t.Run("HandlesNilInjector", func(t *testing.T) {
		// Given a template with nil injector
		template := NewBaseTemplate(nil)

		// When calling Initialize
		err := template.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And dependencies should remain nil
		if template.configHandler != nil {
			t.Error("Expected configHandler to remain nil")
		}
		if template.shell != nil {
			t.Error("Expected shell to remain nil")
		}
	})
}
