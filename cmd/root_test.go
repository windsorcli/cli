package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/mocks"
)

// Helper functions to create pointers for basic types
func ptrInt(i int) *int {
	return &i
}

// Helper function to capture stdout output
func captureStdout(f func()) string {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = oldStdout
	return buf.String()
}

// Helper function to capture stderr output
func captureStderr(f func()) string {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldStderr
	return buf.String()
}

// Mock exit function to capture exit code
var exitCode int

func mockExit(code int) {
	exitCode = code
}

func TestRoot_Execute(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("NoConfigHandlers", func(t *testing.T) {
		// Given no config handlers are registered
		injector := di.NewInjector()

		// When: Execute is called
		err := Execute(injector)

		// Then: it should return an error indicating configHandler resolution failure
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		expectedErrorMsg := "error resolving configHandler"
		if !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, err.Error())
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given: an injector that returns an error when resolving configHandler
		mockInjector := di.NewMockInjector()
		mocks := mocks.CreateSuperMocks(mockInjector)
		mockInjector.SetResolveError("configHandler", errors.New("error resolving configHandler"))

		// When: Execute is called
		err := Execute(mocks.Injector)

		// Then: the error message should indicate the error
		expectedError := "error resolving configHandler: error resolving configHandler"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorCastingConfigHandler", func(t *testing.T) {
		// Given: an injector that returns an instance that is not of type config.ConfigHandler
		mocks := mocks.CreateSuperMocks()
		mocks.Injector.Register("configHandler", "invalid")

		// When: Execute is called
		err := Execute(mocks.Injector)

		// Then: the error message should indicate the error
		expectedError := "resolved instance is not of type config.ConfigHandler"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorLoadingCLIConfig", func(t *testing.T) {
		// Given: a valid configHandler that returns an error when LoadConfig is called
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.LoadConfigFunc = func(path string) error {
			return errors.New("error loading CLI config")
		}

		// When: Execute is called
		err := Execute(mocks.Injector)

		// Then: the error message should indicate the error
		expectedError := "error loading CLI config: error loading CLI config"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})
}

func TestRoot_getCLIConfigPath(t *testing.T) {
	t.Run("UserHomeDirError", func(t *testing.T) {
		// Given osUserHomeDir is mocked to return an error
		originalUserHomeDir := osUserHomeDir
		osUserHomeDir = func() (string, error) {
			return "", errors.New("mock error")
		}
		defer func() { osUserHomeDir = originalUserHomeDir }()

		// Execute the function
		_, err := getCLIConfigPath()

		// Verify the error
		expectedError := "error retrieving user home directory: mock error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("expected error %q, got %v", expectedError, err)
		}
	})
}
