package pipelines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupBasePipeline(t *testing.T) (*BasePipeline, di.Injector) {
	t.Helper()

	injector := di.NewInjector()
	pipeline := NewBasePipeline()

	return pipeline, injector
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewBasePipeline(t *testing.T) {
	t.Run("CreatesBasePipeline", func(t *testing.T) {
		// Given a new base pipeline is created
		// When creating a new base pipeline
		pipeline := NewBasePipeline()

		// Then the pipeline should be created successfully
		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBasePipeline_Initialize(t *testing.T) {
	t.Run("InitializeReturnsNilByDefault", func(t *testing.T) {
		// Given a base pipeline
		pipeline, injector := setupBasePipeline(t)

		// When initializing the pipeline
		err := pipeline.Initialize(injector)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestBasePipeline_Execute(t *testing.T) {
	t.Run("ExecuteReturnsNilByDefault", func(t *testing.T) {
		// Given a base pipeline
		pipeline, injector := setupBasePipeline(t)

		// When initializing and executing the pipeline
		err := pipeline.Initialize(injector)
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		ctx := context.Background()
		err = pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// =============================================================================
// Test Protected Methods
// =============================================================================

func TestBasePipeline_handleSessionReset(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *shell.MockShell) {
		t.Helper()
		pipeline := NewBasePipeline()
		mockShell := shell.NewMockShell()
		pipeline.shell = mockShell

		origOsGetenv := osGetenv
		origOsSetenv := osSetenv
		t.Cleanup(func() {
			osGetenv = origOsGetenv
			osSetenv = origOsSetenv
		})

		osGetenv = func(key string) string {
			return ""
		}
		osSetenv = func(key, value string) error {
			return nil
		}

		return pipeline, mockShell
	}

	t.Run("ReturnsNilWhenShellIsNil", func(t *testing.T) {
		// Given a pipeline with nil shell
		pipeline := &BasePipeline{}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ResetsWhenNoSessionToken", func(t *testing.T) {
		// Given a pipeline with no session token
		pipeline, mockShell := setup(t)

		osGetenv = func(key string) string {
			return ""
		}
		osSetenv = func(key, value string) error {
			return nil
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		resetCalled := false
		mockShell.ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then reset should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !resetCalled {
			t.Error("Expected shell reset to be called")
		}
	})

	t.Run("ResetsWhenResetFlagsTrue", func(t *testing.T) {
		// Given a pipeline with reset flags true
		pipeline, mockShell := setup(t)

		osGetenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "test-token"
			}
			return ""
		}
		osSetenv = func(key, value string) error {
			return nil
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}
		resetCalled := false
		mockShell.ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then reset should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !resetCalled {
			t.Error("Expected shell reset to be called")
		}
	})

	t.Run("DoesNotResetWhenSessionTokenExistsAndResetFlagsFalse", func(t *testing.T) {
		// Given a pipeline with session token and reset flags false
		pipeline, mockShell := setup(t)

		osGetenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "test-token"
			}
			return ""
		}
		osSetenv = func(key, value string) error {
			return nil
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		resetCalled := false
		mockShell.ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then reset should not be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if resetCalled {
			t.Error("Expected shell reset to not be called")
		}
	})

	t.Run("ReturnsErrorWhenCheckResetFlagsFails", func(t *testing.T) {
		// Given a pipeline where check reset flags fails
		pipeline, mockShell := setup(t)

		osGetenv = func(key string) string {
			return ""
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("check reset flags error")
		}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "check reset flags error" {
			t.Errorf("Expected check reset flags error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSetenvFails", func(t *testing.T) {
		// Given a pipeline where setenv fails
		pipeline, mockShell := setup(t)

		osGetenv = func(key string) string {
			return ""
		}
		osSetenv = func(key, value string) error {
			return fmt.Errorf("setenv error")
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}
		mockShell.ResetFunc = func(...bool) {}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "setenv error" {
			t.Errorf("Expected setenv error, got: %v", err)
		}
	})
}

func TestBasePipeline_loadConfig(t *testing.T) {
	t.Run("ReturnsErrorWhenShellIsNil", func(t *testing.T) {
		// Given a BasePipeline with nil shell
		pipeline := NewBasePipeline()

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when shell is nil")
		}
		if err.Error() != "shell not initialized" {
			t.Errorf("Expected 'shell not initialized' error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a BasePipeline with shell but nil config handler
		pipeline := NewBasePipeline()
		pipeline.shell = shell.NewMockShell()

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when config handler is nil")
		}
		if err.Error() != "config handler not initialized" {
			t.Errorf("Expected 'config handler not initialized' error, got %v", err)
		}
	})

	t.Run("LoadsConfigSuccessfully", func(t *testing.T) {
		// Given a BasePipeline with shell and config handler
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		projectRoot := t.TempDir()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}
		pipeline.shell = mockShell

		mockConfigHandler := config.NewMockConfigHandler()
		loadConfigCalled := false
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			loadConfigCalled = true
			expectedPath := filepath.Join(projectRoot, "windsor.yaml")
			if path != expectedPath {
				t.Errorf("Expected config path %q, got %q", expectedPath, path)
			}
			return nil
		}
		pipeline.configHandler = mockConfigHandler

		// Create a test config file
		configPath := filepath.Join(projectRoot, "windsor.yaml")
		if err := os.WriteFile(configPath, []byte("test: config"), 0644); err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then no error should be returned and config should be loaded
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !loadConfigCalled {
			t.Error("Expected loadConfig to be called on config handler")
		}
	})

	t.Run("ReturnsErrorWhenGetProjectRootFails", func(t *testing.T) {
		// Given a BasePipeline with failing shell
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}
		pipeline.shell = mockShell

		mockConfigHandler := config.NewMockConfigHandler()
		pipeline.configHandler = mockConfigHandler

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}
		if !strings.Contains(err.Error(), "error retrieving project root") {
			t.Errorf("Expected 'error retrieving project root' in error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenLoadConfigFails", func(t *testing.T) {
		// Given a BasePipeline with config handler that fails to load
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		projectRoot := t.TempDir()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}
		pipeline.shell = mockShell

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			return fmt.Errorf("load config error")
		}
		pipeline.configHandler = mockConfigHandler

		// Create a test config file
		configPath := filepath.Join(projectRoot, "windsor.yaml")
		if err := os.WriteFile(configPath, []byte("test: config"), 0644); err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when loadConfig fails")
		}
		if !strings.Contains(err.Error(), "error loading config file") {
			t.Errorf("Expected 'error loading config file' in error, got %v", err)
		}
	})

	t.Run("SkipsLoadingWhenNoConfigFileExists", func(t *testing.T) {
		// Given a BasePipeline with no config file
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		projectRoot := t.TempDir()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}
		pipeline.shell = mockShell

		mockConfigHandler := config.NewMockConfigHandler()
		loadConfigCalled := false
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			loadConfigCalled = true
			return nil
		}
		pipeline.configHandler = mockConfigHandler

		// When loadConfig is called (no config file exists)
		err := pipeline.loadConfig()

		// Then no error should be returned and loadConfig should not be called
		if err != nil {
			t.Errorf("Expected no error when no config file exists, got %v", err)
		}
		if loadConfigCalled {
			t.Error("Expected loadConfig not to be called when no config file exists")
		}
	})
}
