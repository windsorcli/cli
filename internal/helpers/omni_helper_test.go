package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

func TestOmniHelper(t *testing.T) {
	t.Run("NewOmniHelper", func(t *testing.T) {
		t.Run("ErrorResolvingContext", func(t *testing.T) {
			// Create DI container without registering context
			diContainer := di.NewContainer()
			mockConfigHandler := &config.MockConfigHandler{}
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Attempt to create OmniHelper
			_, err := NewOmniHelper(diContainer)
			if err == nil || !strings.Contains(err.Error(), "error resolving context") {
				t.Fatalf("expected error resolving context, got %v", err)
			}
		})

		t.Run("ErrorContextTypeAssertion", func(t *testing.T) {
			// Create DI container and register a wrong type for context
			diContainer := di.NewContainer()
			mockConfigHandler := &config.MockConfigHandler{}
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", "not a context interface")

			// Attempt to create OmniHelper
			_, err := NewOmniHelper(diContainer)
			if err == nil || !strings.Contains(err.Error(), "resolved context is not of type ContextInterface") {
				t.Fatalf("expected error for context type assertion, got %v", err)
			}
		})
	})

	t.Run("GetEnvVars", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given: a valid context path
			contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
			omniConfigPath := filepath.Join(contextPath, ".omni", "config")

			// Ensure the omni config file exists
			err := os.MkdirAll(filepath.Dir(omniConfigPath), 0755)
			if err != nil {
				t.Fatalf("Failed to create omni config directory: %v", err)
			}
			_, err = os.Create(omniConfigPath)
			if err != nil {
				t.Fatalf("Failed to create omni config file: %v", err)
			}

			// Mock context
			mockContext := &context.MockContext{
				GetConfigRootFunc: func() (string, error) {
					return contextPath, nil
				},
			}

			// Create OmniHelper
			container := di.NewContainer()
			container.Register("context", mockContext)
			omniHelper, err := NewOmniHelper(container)
			if err != nil {
				t.Fatalf("NewOmniHelper() error = %v", err)
			}

			// When: GetEnvVars is called
			envVars, err := omniHelper.GetEnvVars()
			if err != nil {
				t.Fatalf("GetEnvVars() error = %v", err)
			}

			// Then: the environment variables should be set correctly
			expectedEnvVars := map[string]string{
				"OMNICONFIG": omniConfigPath,
			}
			if !reflect.DeepEqual(envVars, expectedEnvVars) {
				t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
			}
		})

		t.Run("FileNotExist", func(t *testing.T) {
			// Given: a non-existent context path
			contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")
			omniConfigPath := ""

			// Mock context
			mockContext := &context.MockContext{
				GetConfigRootFunc: func() (string, error) {
					return contextPath, nil
				},
			}

			// Create OmniHelper
			container := di.NewContainer()
			container.Register("context", mockContext)
			omniHelper, err := NewOmniHelper(container)
			if err != nil {
				t.Fatalf("NewOmniHelper() error = %v", err)
			}

			// When: GetEnvVars is called
			envVars, err := omniHelper.GetEnvVars()
			if err != nil {
				t.Fatalf("GetEnvVars() error = %v", err)
			}

			// Then: the environment variables should be set correctly with an empty OMNICONFIG
			expectedEnvVars := map[string]string{
				"OMNICONFIG": omniConfigPath,
			}
			if !reflect.DeepEqual(envVars, expectedEnvVars) {
				t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
			}
		})

		t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
			// Given a mock context that returns an error for config root
			mockContext := &context.MockContext{
				GetConfigRootFunc: func() (string, error) {
					return "", errors.New("error retrieving config root")
				},
			}

			// Create OmniHelper
			container := di.NewContainer()
			container.Register("context", mockContext)
			omniHelper, err := NewOmniHelper(container)
			if err != nil {
				t.Fatalf("NewOmniHelper() error = %v", err)
			}

			// When calling GetEnvVars
			expectedError := "error retrieving config root"

			_, err = omniHelper.GetEnvVars()
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		})
	})

	t.Run("PostEnvExec", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a OmniHelper instance
			mockContext := &context.MockContext{
				GetContextFunc:    func() (string, error) { return "", nil },
				GetConfigRootFunc: func() (string, error) { return "", nil },
			}
			container := di.NewContainer()
			container.Register("context", mockContext)
			omniHelper, err := NewOmniHelper(container)
			if err != nil {
				t.Fatalf("NewOmniHelper() error = %v", err)
			}

			// When calling PostEnvExec
			err = omniHelper.PostEnvExec()

			// Then no error should be returned
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	})

	t.Run("SetConfig", func(t *testing.T) {
		t.Run("SetConfigStub", func(t *testing.T) {
			// Given a OmniHelper instance
			mockContext := &context.MockContext{}
			container := di.NewContainer()
			container.Register("context", mockContext)
			helper, err := NewOmniHelper(container)
			if err != nil {
				t.Fatalf("NewOmniHelper() error = %v", err)
			}

			// When: SetConfig is called
			err = helper.SetConfig("some_key", "some_value")

			// Then: it should return no error
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	})

	t.Run("GetContainerConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock context
			mockContext := &context.MockContext{}
			container := di.NewContainer()
			container.Register("context", mockContext)

			// Create OmniHelper
			omniHelper, err := NewOmniHelper(container)
			if err != nil {
				t.Fatalf("NewOmniHelper() error = %v", err)
			}

			// When: GetContainerConfig is called
			containerConfig, err := omniHelper.GetContainerConfig()
			if err != nil {
				t.Fatalf("GetContainerConfig() error = %v", err)
			}

			// Then: the result should be nil as per the stub implementation
			if containerConfig != nil {
				t.Errorf("expected nil, got %v", containerConfig)
			}
		})
	})
}
