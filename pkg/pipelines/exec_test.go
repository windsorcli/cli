package pipelines

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// mockFileInfo is a simple mock implementation of os.FileInfo for testing
type mockFileInfo struct {
	name string
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() any           { return nil }

func TestNewExecPipeline(t *testing.T) {
	t.Run("CreatesWithDefaultConstructors", func(t *testing.T) {
		pipeline := NewExecPipeline()

		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}

		if pipeline.constructors.NewShell == nil {
			t.Error("Expected NewShell constructor to be set")
		}
	})

	t.Run("CreatesWithCustomConstructors", func(t *testing.T) {
		constructors := ExecConstructors{
			NewShell: func(di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
		}

		pipeline := NewExecPipeline(constructors)

		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}

		if pipeline.constructors.NewShell == nil {
			t.Error("Expected NewShell constructor to be set")
		}
	})
}

func TestExecPipeline_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()

		// Create mock shell
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return nil
		}

		constructors := ExecConstructors{
			NewShell: func(di.Injector) shell.Shell {
				return mockShell
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector, context.Background())

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ShellInitializationError", func(t *testing.T) {
		injector := di.NewInjector()

		// Create mock shell that fails initialization
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return fmt.Errorf("shell initialization failed")
		}

		constructors := ExecConstructors{
			NewShell: func(di.Injector) shell.Shell {
				return mockShell
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector, context.Background())

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "failed to initialize shell: shell initialization failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("UsesExistingShell", func(t *testing.T) {
		injector := di.NewInjector()

		// Register existing shell
		existingShell := shell.NewMockShell()
		injector.Register("shell", existingShell)

		pipeline := NewExecPipeline()
		err := pipeline.Initialize(injector, context.Background())

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if pipeline.shell != existingShell {
			t.Error("Expected pipeline to use existing shell")
		}
	})
}

func TestExecPipeline_Execute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()

		// Create mock shell
		mockShell := shell.NewMockShell()
		var capturedCommand string
		var capturedArgs []string
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			capturedCommand = command
			capturedArgs = args
			return "output", nil
		}

		constructors := ExecConstructors{
			NewShell: func(di.Injector) shell.Shell {
				return mockShell
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "command", "test-command")
		ctx = context.WithValue(ctx, "args", []string{"arg1", "arg2"})

		err = pipeline.Execute(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if capturedCommand != "test-command" {
			t.Errorf("Expected command 'test-command', got %q", capturedCommand)
		}

		if len(capturedArgs) != 2 || capturedArgs[0] != "arg1" || capturedArgs[1] != "arg2" {
			t.Errorf("Expected args ['arg1', 'arg2'], got %v", capturedArgs)
		}
	})

	t.Run("NoCommandInContext", func(t *testing.T) {
		injector := di.NewInjector()

		mockShell := shell.NewMockShell()
		constructors := ExecConstructors{
			NewShell: func(di.Injector) shell.Shell {
				return mockShell
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		ctx := context.Background()

		err = pipeline.Execute(ctx)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "no command provided in context"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("EmptyCommandInContext", func(t *testing.T) {
		injector := di.NewInjector()

		mockShell := shell.NewMockShell()
		constructors := ExecConstructors{
			NewShell: func(di.Injector) shell.Shell {
				return mockShell
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "command", "")

		err = pipeline.Execute(ctx)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "no command provided in context"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ShellExecutionError", func(t *testing.T) {
		injector := di.NewInjector()

		// Create mock shell that fails execution
		mockShell := shell.NewMockShell()
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("command execution failed")
		}

		constructors := ExecConstructors{
			NewShell: func(di.Injector) shell.Shell {
				return mockShell
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "command", "test-command")

		err = pipeline.Execute(ctx)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "command execution failed: command execution failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoArgsInContext", func(t *testing.T) {
		injector := di.NewInjector()

		// Create mock shell
		mockShell := shell.NewMockShell()
		var capturedCommand string
		var capturedArgs []string
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			capturedCommand = command
			capturedArgs = args
			return "output", nil
		}

		constructors := ExecConstructors{
			NewShell: func(di.Injector) shell.Shell {
				return mockShell
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "command", "test-command")

		err = pipeline.Execute(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if capturedCommand != "test-command" {
			t.Errorf("Expected command 'test-command', got %q", capturedCommand)
		}

		if len(capturedArgs) != 0 {
			t.Errorf("Expected no args, got %v", capturedArgs)
		}
	})
}
