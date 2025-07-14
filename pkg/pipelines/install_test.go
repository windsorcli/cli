package pipelines

import (
	"context"
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
)

// =============================================================================
// Test Setup
// =============================================================================

type InstallMocks struct {
	*Mocks
	BlueprintHandler *blueprint.MockBlueprintHandler
}

func setupInstallMocks(t *testing.T, opts ...*SetupOptions) *InstallMocks {
	t.Helper()

	// Create setup options, preserving any provided options
	setupOptions := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		setupOptions = opts[0]
	}

	baseMocks := setupMocks(t, setupOptions)

	// Setup blueprint handler mock
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(baseMocks.Injector)
	mockBlueprintHandler.InitializeFunc = func() error { return nil }
	mockBlueprintHandler.InstallFunc = func() error { return nil }
	mockBlueprintHandler.WaitForKustomizationsFunc = func(message string, names ...string) error { return nil }
	baseMocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

	return &InstallMocks{
		Mocks:            baseMocks,
		BlueprintHandler: mockBlueprintHandler,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewInstallPipeline(t *testing.T) {
	t.Run("CreatesNewInstallPipeline", func(t *testing.T) {
		// When creating a new InstallPipeline
		pipeline := NewInstallPipeline()

		// Then it should not be nil
		if pipeline == nil {
			t.Error("Expected pipeline to not be nil")
		}

		// And it should be of the correct type
		if pipeline == nil {
			t.Error("Expected pipeline to be of type *InstallPipeline")
		}
	})
}

// =============================================================================
// Test Initialize
// =============================================================================

func TestInstallPipeline_Initialize(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InstallPipeline, *InstallMocks) {
		t.Helper()
		pipeline := NewInstallPipeline()
		mocks := setupInstallMocks(t, opts...)
		return pipeline, mocks
	}

	t.Run("InitializesSuccessfully", func(t *testing.T) {
		// Given a new InstallPipeline
		pipeline, mocks := setup(t)

		// When Initialize is called
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And blueprint handler should be set
		if pipeline.blueprintHandler == nil {
			t.Error("Expected blueprint handler to be set")
		}
	})

	t.Run("ReturnsErrorWhenBasePipelineInitializeFails", func(t *testing.T) {
		// Given a pipeline with failing base initialization
		pipeline, mocks := setup(t)

		// Override shell to return error during initialization
		mocks.Shell.InitializeFunc = func() error {
			return fmt.Errorf("shell init failed")
		}

		// When Initialize is called
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize shell: shell init failed" {
			t.Errorf("Expected shell init error, got %q", err.Error())
		}
	})

	t.Run("ReturnsErrorWhenBlueprintHandlerInitializeFails", func(t *testing.T) {
		// Given a pipeline with failing blueprint handler initialization
		pipeline, mocks := setup(t)

		// Override blueprint handler to return error during initialization
		mocks.BlueprintHandler.InitializeFunc = func() error {
			return fmt.Errorf("blueprint handler init failed")
		}

		// When Initialize is called
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize blueprint handler: blueprint handler init failed" {
			t.Errorf("Expected blueprint handler init error, got %q", err.Error())
		}
	})
}

// =============================================================================
// Test Execute
// =============================================================================

func TestInstallPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InstallPipeline, *InstallMocks) {
		t.Helper()
		pipeline := NewInstallPipeline()
		mocks := setupInstallMocks(t, opts...)

		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return pipeline, mocks
	}

	t.Run("ExecutesSuccessfully", func(t *testing.T) {
		// Given a properly initialized InstallPipeline
		pipeline, _ := setup(t)

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ExecutesWithWaitFlag", func(t *testing.T) {
		// Given a pipeline with wait flag set
		pipeline, mocks := setup(t)

		waitCalled := false
		mocks.BlueprintHandler.WaitForKustomizationsFunc = func(message string, names ...string) error {
			waitCalled = true
			return nil
		}

		ctx := context.WithValue(context.Background(), "wait", true)

		// When Execute is called
		err := pipeline.Execute(ctx)

		// Then no error should be returned and wait should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !waitCalled {
			t.Error("Expected blueprint wait to be called")
		}
	})

	t.Run("ReturnsErrorWhenConfigNotLoaded", func(t *testing.T) {
		// Given a mock config handler that returns not loaded
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.IsLoadedFunc = func() bool { return false }
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.GetContextFunc = func() string { return "mock-context" }
		mockConfigHandler.SetContextFunc = func(context string) error { return nil }

		// Setup with the not-loaded config handler
		pipeline, _ := setup(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Nothing to install. Have you run \033[1mwindsor init\033[0m?" {
			t.Errorf("Expected config not loaded error, got %q", err.Error())
		}
	})

	t.Run("ReturnsErrorWhenNoBlueprintHandler", func(t *testing.T) {
		// Given a pipeline with nil blueprint handler
		pipeline, _ := setup(t)

		// Set blueprint handler to nil
		pipeline.blueprintHandler = nil

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No blueprint handler found" {
			t.Errorf("Expected no blueprint handler error, got %q", err.Error())
		}
	})

	t.Run("ReturnsErrorWhenBlueprintInstallFails", func(t *testing.T) {
		// Given a pipeline with failing blueprint install
		pipeline, mocks := setup(t)

		// Override blueprint handler to return error during install
		mocks.BlueprintHandler.InstallFunc = func() error {
			return fmt.Errorf("blueprint install failed")
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error installing blueprint: blueprint install failed" {
			t.Errorf("Expected blueprint install error, got %q", err.Error())
		}
	})

	t.Run("ReturnsErrorWhenBlueprintWaitFails", func(t *testing.T) {
		// Given a pipeline with failing blueprint wait
		pipeline, mocks := setup(t)

		// Override blueprint handler to return error during wait
		mocks.BlueprintHandler.WaitForKustomizationsFunc = func(message string, names ...string) error {
			return fmt.Errorf("blueprint wait failed")
		}

		ctx := context.WithValue(context.Background(), "wait", true)

		// When Execute is called
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed waiting for kustomizations: blueprint wait failed" {
			t.Errorf("Expected blueprint wait error, got %q", err.Error())
		}
	})
}
