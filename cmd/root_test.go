package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	ctrl "github.com/windsor-hotel/cli/internal/controller"
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
		controller := ctrl.NewController(di.NewInjector())

		// When: Execute is called
		err := Execute(controller)

		// Then: it should return an error indicating configHandler resolution failure
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		expectedErrorMsg := "error resolving config handler: no instance registered with name configHandler"
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
		err := Execute(mocks.Controller)

		// Then: the error message should indicate the error
		expectedError := "error resolving config handler: error resolving configHandler"
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
		err := Execute(mocks.Controller)

		// Then: the error message should indicate the error
		expectedError := "Error loading config file: error loading CLI config"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})
}
